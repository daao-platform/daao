// Package sysmetrics provides cross-platform system metrics collection.
// It collects CPU, memory, disk, and GPU utilisation for satellite telemetry.
package sysmetrics

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Metrics contains a snapshot of system metrics.
type Metrics struct {
	CPUPercent       float64     `json:"cpu_percent"`
	MemoryPercent    float64     `json:"memory_percent"`
	MemoryUsedBytes  int64       `json:"memory_used_bytes"`
	MemoryTotalBytes int64       `json:"memory_total_bytes"`
	DiskPercent      float64     `json:"disk_percent"`
	DiskUsedBytes    int64       `json:"disk_used_bytes"`
	DiskTotalBytes   int64       `json:"disk_total_bytes"`
	GPUs             []GPUMetric `json:"gpus,omitempty"`
	Timestamp        int64       `json:"timestamp"`
}

// GPUMetric contains per-GPU utilisation data.
type GPUMetric struct {
	Index              int     `json:"index"`
	Name               string  `json:"name"`
	UtilizationPercent float64 `json:"utilization_percent"`
	MemoryUsedBytes    int64   `json:"memory_used_bytes"`
	MemoryTotalBytes   int64   `json:"memory_total_bytes"`
	TemperatureCelsius float64 `json:"temperature_celsius"`
}

// Collect gathers current system metrics (cross-platform).
// Platform-specific functions collectCPU, collectMemory, collectDisk
// are implemented in collector_windows.go and collector_linux.go.
func Collect() (*Metrics, error) {
	m := &Metrics{
		Timestamp: time.Now().UnixMilli(),
	}

	// CPU
	cpu, err := collectCPU()
	if err == nil {
		m.CPUPercent = cpu
	}

	// Memory
	memUsed, memTotal, err := collectMemory()
	if err == nil {
		m.MemoryUsedBytes = memUsed
		m.MemoryTotalBytes = memTotal
		if memTotal > 0 {
			m.MemoryPercent = float64(memUsed) / float64(memTotal) * 100
		}
	}

	// Disk
	diskUsed, diskTotal, err := collectDisk()
	if err == nil {
		m.DiskUsedBytes = diskUsed
		m.DiskTotalBytes = diskTotal
		if diskTotal > 0 {
			m.DiskPercent = float64(diskUsed) / float64(diskTotal) * 100
		}
	}

	// GPU (optional, nvidia-smi based — works on all platforms)
	gpus, _ := collectGPU()
	m.GPUs = gpus

	return m, nil
}

// ---- GPU (nvidia-smi) — cross platform ----

func collectGPU() ([]GPUMetric, error) {
	// Try nvidia-smi with CSV output
	cmd := exec.Command("nvidia-smi",
		"--query-gpu=name,utilization.gpu,memory.used,memory.total,temperature.gpu",
		"--format=csv,noheader,nounits")

	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var gpus []GPUMetric
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for i, line := range lines {
		fields := strings.Split(line, ", ")
		if len(fields) < 5 {
			continue
		}

		gpu := GPUMetric{
			Index: i,
			Name:  strings.TrimSpace(fields[0]),
		}

		fmt.Sscanf(strings.TrimSpace(fields[1]), "%f", &gpu.UtilizationPercent)

		var memUsedMB, memTotalMB float64
		fmt.Sscanf(strings.TrimSpace(fields[2]), "%f", &memUsedMB)
		fmt.Sscanf(strings.TrimSpace(fields[3]), "%f", &memTotalMB)
		gpu.MemoryUsedBytes = int64(memUsedMB * 1024 * 1024)
		gpu.MemoryTotalBytes = int64(memTotalMB * 1024 * 1024)

		fmt.Sscanf(strings.TrimSpace(fields[4]), "%f", &gpu.TemperatureCelsius)

		gpus = append(gpus, gpu)
	}

	return gpus, nil
}

// GPUsToJSON returns a JSON representation of GPU metrics for storage.
func (m *Metrics) GPUsToJSON() ([]byte, error) {
	if len(m.GPUs) == 0 {
		return []byte("[]"), nil
	}
	return json.Marshal(m.GPUs)
}
