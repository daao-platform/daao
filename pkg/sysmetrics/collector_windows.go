//go:build windows

package sysmetrics

import (
	"fmt"
	"syscall"
	"unsafe"
)

// prevIdle and prevTotal for CPU delta calculation
var (
	prevIdleTime   uint64
	prevKernelTime uint64
	prevUserTime   uint64
	hasPrevCPU     bool
)

func collectCPU() (float64, error) {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	getSystemTimes := kernel32.NewProc("GetSystemTimes")

	var idleTime, kernelTime, userTime syscall.Filetime
	ret, _, err := getSystemTimes.Call(
		uintptr(unsafe.Pointer(&idleTime)),
		uintptr(unsafe.Pointer(&kernelTime)),
		uintptr(unsafe.Pointer(&userTime)),
	)
	if ret == 0 {
		return 0, fmt.Errorf("GetSystemTimes failed: %v", err)
	}

	idle := uint64(idleTime.HighDateTime)<<32 | uint64(idleTime.LowDateTime)
	kernel := uint64(kernelTime.HighDateTime)<<32 | uint64(kernelTime.LowDateTime)
	user := uint64(userTime.HighDateTime)<<32 | uint64(userTime.LowDateTime)

	if !hasPrevCPU {
		prevIdleTime = idle
		prevKernelTime = kernel
		prevUserTime = user
		hasPrevCPU = true
		return 0, nil // first call, no delta yet
	}

	idleDelta := float64(idle - prevIdleTime)
	totalDelta := float64((kernel - prevKernelTime) + (user - prevUserTime))

	prevIdleTime = idle
	prevKernelTime = kernel
	prevUserTime = user

	if totalDelta == 0 {
		return 0, nil
	}
	return (1.0 - idleDelta/totalDelta) * 100, nil
}

type memoryStatusEx struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

func collectMemory() (int64, int64, error) {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	globalMemoryStatusEx := kernel32.NewProc("GlobalMemoryStatusEx")

	var mse memoryStatusEx
	mse.Length = uint32(unsafe.Sizeof(mse))

	ret, _, err := globalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&mse)))
	if ret == 0 {
		return 0, 0, fmt.Errorf("GlobalMemoryStatusEx failed: %v", err)
	}

	total := int64(mse.TotalPhys)
	avail := int64(mse.AvailPhys)
	used := total - avail
	return used, total, nil
}

func collectDisk() (int64, int64, error) {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	getDiskFreeSpaceEx := kernel32.NewProc("GetDiskFreeSpaceExW")

	var freeBytesAvailable, totalBytes, totalFreeBytes int64
	rootPath, _ := syscall.UTF16PtrFromString("C:\\")

	ret, _, err := getDiskFreeSpaceEx.Call(
		uintptr(unsafe.Pointer(rootPath)),
		uintptr(unsafe.Pointer(&freeBytesAvailable)),
		uintptr(unsafe.Pointer(&totalBytes)),
		uintptr(unsafe.Pointer(&totalFreeBytes)),
	)
	if ret == 0 {
		return 0, 0, fmt.Errorf("GetDiskFreeSpaceExW failed: %v", err)
	}

	used := totalBytes - totalFreeBytes
	return used, totalBytes, nil
}
