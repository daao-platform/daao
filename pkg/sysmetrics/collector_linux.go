//go:build linux

package sysmetrics

import (
	"bufio"
	"fmt"
	"os"
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
	f, err := os.Open("/proc/stat")
	if err != nil {
		return 0, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "cpu ") {
			fields := strings.Fields(line)
			if len(fields) < 5 {
				return 0, fmt.Errorf("unexpected /proc/stat format")
			}

			var values [10]uint64
			for i := 1; i < len(fields) && i <= 10; i++ {
				values[i-1], _ = strconv.ParseUint(fields[i], 10, 64)
			}

			// idle = idle + iowait
			idle := values[3] + values[4]
			var total uint64
			for i := 0; i < len(fields)-1 && i < 10; i++ {
				total += values[i]
			}

			if !hasPrev {
				prevIdle = idle
				prevTotal = total
				hasPrev = true
				return 0, nil // first call, no delta
			}

			idleDelta := float64(idle - prevIdle)
			totalDelta := float64(total - prevTotal)
			prevIdle = idle
			prevTotal = total

			if totalDelta == 0 {
				return 0, nil
			}
			return (1.0 - idleDelta/totalDelta) * 100, nil
		}
	}
	return 0, fmt.Errorf("cpu line not found in /proc/stat")
}

func collectMemory() (int64, int64, error) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()

	var memTotal, memAvailable int64

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "MemTotal:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				v, _ := strconv.ParseInt(fields[1], 10, 64)
				memTotal = v * 1024 // kB to bytes
			}
		} else if strings.HasPrefix(line, "MemAvailable:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				v, _ := strconv.ParseInt(fields[1], 10, 64)
				memAvailable = v * 1024
			}
		}
	}

	if memTotal == 0 {
		return 0, 0, fmt.Errorf("could not read MemTotal from /proc/meminfo")
	}

	used := memTotal - memAvailable
	return used, memTotal, nil
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
