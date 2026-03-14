//go:build darwin

package sysmetrics

import (
	"bufio"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

// prevCPU values for delta calculation
var (
	prevIdle  uint64
	prevTotal uint64
	hasPrev   bool
)

func collectCPU() (float64, error) {
	// Use sysctl to get CPU load on macOS
	// vm.loadavg gives 1/5/15 min averages — for a percentage we look at
	// the processor info via `top` in logging mode
	out, err := exec.Command("sh", "-c",
		`top -l 1 -n 0 | grep "CPU usage" | awk '{print $3}' | tr -d '%'`).Output()
	if err != nil {
		return 0, fmt.Errorf("failed to get CPU: %v", err)
	}

	s := strings.TrimSpace(string(out))
	if s == "" {
		return 0, fmt.Errorf("empty CPU output")
	}

	val, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, err
	}
	return val, nil
}

func collectMemory() (int64, int64, error) {
	// Use sysctl for total memory
	totalOut, err := exec.Command("sysctl", "-n", "hw.memsize").Output()
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get total memory: %v", err)
	}
	total, err := strconv.ParseInt(strings.TrimSpace(string(totalOut)), 10, 64)
	if err != nil {
		return 0, 0, err
	}

	// Use vm_stat for page statistics
	vmOut, err := exec.Command("vm_stat").Output()
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get vm_stat: %v", err)
	}

	pageSize := int64(4096) // Default macOS page size
	var free, inactive, speculative int64

	scanner := bufio.NewScanner(strings.NewReader(string(vmOut)))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "Pages free:") {
			free = parseVMStatLine(line)
		} else if strings.HasPrefix(line, "Pages inactive:") {
			inactive = parseVMStatLine(line)
		} else if strings.HasPrefix(line, "Pages speculative:") {
			speculative = parseVMStatLine(line)
		}
	}

	available := (free + inactive + speculative) * pageSize
	used := total - available
	if used < 0 {
		used = 0
	}

	return used, total, nil
}

func parseVMStatLine(line string) int64 {
	parts := strings.Split(line, ":")
	if len(parts) < 2 {
		return 0
	}
	s := strings.TrimSpace(parts[1])
	s = strings.TrimSuffix(s, ".")
	val, _ := strconv.ParseInt(s, 10, 64)
	return val
}

func collectDisk() (int64, int64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs("/", &stat); err != nil {
		return 0, 0, err
	}

	total := int64(stat.Blocks) * int64(stat.Bsize)
	free := int64(stat.Bavail) * int64(stat.Bsize)
	used := total - free
	return used, total, nil
}
