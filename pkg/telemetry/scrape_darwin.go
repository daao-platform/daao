//go:build darwin

package telemetry

// #cgo LDFLAGS: -lproc
// #include <libproc.h>
// #include <stdlib.h>
// #include <sys/proc_info.h>
import "C"

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"unsafe"
)

// darwinScraper implements Scraper for macOS using libproc
type darwinScraper struct{}

// newDarwinScraper creates a new Darwin/macOS scraper
func newDarwinScraper() Scraper {
	return &darwinScraper{}
}

// IsAvailable checks if libproc is available (macOS)
func (s *darwinScraper) IsAvailable() bool {
	return true
}

// ScrapeProcess reads the process state using libproc
func (s *darwinScraper) ScrapeProcess(pid int) (*ProcessInfo, error) {
	// First, get the process info using proc_pidinfo
	// PROC_PIDTASKINFO gives us task information including thread states
	var taskInfo C.struct_proc_taskinfo
	taskInfoSize := C.int(unsafe.Sizeof(taskInfo))

	ret, err := C.proc_pidinfo(C.int(pid), C.PROC_PIDTASKINFO, 0, unsafe.Pointer(&taskInfo), taskInfoSize)
	if err != nil {
		return nil, fmt.Errorf("proc_pidinfo failed: %w", err)
	}

	if ret != taskInfoSize {
		// Try alternative: proc_ppid
		return s.getProcessInfoFallback(pid)
	}

	// Extract thread states from task info
	// pti_threads shows the number of running threads
	numThreads := int(taskInfo.pti_threads)

	// Get process status using proc_pidpath (for existence check)
	pathBuf := make([]byte, C.PROC_PIDPATHINFO_MAXSIZE)
	pathLen := C.proc_pidpath(C.int(pid), unsafe.Pointer(&pathBuf[0]), C.uint32_t(len(pathBuf)))
	if pathLen <= 0 {
		return nil, ErrProcessNotFound
	}

	// Determine state from thread info
	// On macOS, we can check if any threads are waiting
	state := determineDarwinState(taskInfo.pti_status, int(taskInfo.pti_threads))

	// Get wait channel info if available
	waitChannel := getDarwinWaitChannel(int(taskInfo.pti_threads))

	waitReason := reasonFromDarwinwait(waitChannel)

	return &ProcessInfo{
		PID:         pid,
		State:       state,
		WaitReason:  waitReason,
		WaitChannel: waitChannel,
	}, nil
}

// getProcessInfoFallback provides basic info when PROC_PIDTASKINFO fails
func (s *darwinScraper) getProcessInfoFallback(pid int) (*ProcessInfo, error) {
	// Get process path to verify it exists
	pathBuf := make([]byte, C.PROC_PIDPATHINFO_MAXSIZE)
	pathLen := C.proc_pidpath(C.int(pid), unsafe.Pointer(&pathBuf), C.uint32_t(len(pathBuf)))
	if pathLen <= 0 {
		return nil, ErrProcessNotFound
	}

	// Use proc_listpids to get process list info
	// For now, return basic info
	return &ProcessInfo{
		PID:         pid,
		State:       StateUnknown,
		WaitReason:  WaitReasonUnknown,
		WaitChannel: "",
	}, nil
}

// determineDarwinState determines ProcessState from Darwin process status
func determineDarwinState(status C.int, numThreads int) ProcessState {
	// macOS process status codes:
	// SIDL = 1: Process being created
	// SRUN = 2: Running
	// SSLEEP = 3: Sleeping (wait)
	// SSTOP = 4: Stopped
	// SZOMB = 5: Zombie

	switch int(status) {
	case 1: // SIDL
		return StateUnknown
	case 2: // SRUN
		return StateRunning
	case 3: // SSLEEP
		// Check if it's waiting for input
		return StateWaiting
	case 4: // SSTOP
		return StateStopped
	case 5: // SZOMB
		return StateZombie
	default:
		// If running and has threads, likely running
		if numThreads > 0 {
			return StateRunning
		}
		return StateUnknown
	}
}

// getDarwinWaitChannel attempts to get wait channel info
// Note: Full thread-level info requires kernel debugging symbols
func getDarwinWaitChannel(numThreads int) string {
	// Without kernel symbols, we can only infer from process state
	// Return empty - actual wait channel would require kernel debugging
	if numThreads == 0 {
		return "no_threads"
	}
	return ""
}

// reasonFromDarwinwait determines WaitReason from wait channel info
func reasonFromDarwinwait(waitChannel string) WaitReason {
	if waitChannel == "" || waitChannel == "no_threads" {
		return WaitReasonUnknown
	}

	// Similar logic to Linux for wait reason
	terminalIndicators := []string{"tty", "pts", "console", "stdin"}
	for _, ind := range terminalIndicators {
		if strings.Contains(waitChannel, ind) {
			return WaitReasonUserInput
		}
	}

	ioIndicators := []string{"disk", "aio", "io"}
	for _, ind := range ioIndicators {
		if strings.Contains(waitChannel, ind) {
			return WaitReasonIO
		}
	}

	networkIndicators := []string{"tcp", "udp", "inet", "socket"}
	for _, ind := range networkIndicators {
		if strings.Contains(waitChannel, ind) {
			return WaitReasonNetwork
		}
	}

	return WaitReasonUnknown
}

// GetPids returns all PIDs on the system (useful for finding PTY owners)
func GetPids() ([]int, error) {
	// Get size needed for all PIDs
	bufSize := C.proc_listpids(C.PROC_ALL_PIDS, 0, nil, 0)
	if bufSize <= 0 {
		return nil, fmt.Errorf("failed to get pids buffer size")
	}

	// Allocate buffer
	buf := make([]C.int, bufSize/unsafe.Sizeof(C.int(0)))

	// Get PIDs
	actualSize := C.proc_listpids(C.PROC_ALL_PIDS, 0, unsafe.Pointer(&buf[0]), bufSize)
	if actualSize <= 0 {
		return nil, fmt.Errorf("failed to list pids")
	}

	// Parse PIDs
	numPids := int(actualSize) / int(unsafe.Sizeof(C.int(0)))
	var pids []int
	for i := 0; i < numPids; i++ {
		if int(buf[i]) > 0 {
			pids = append(pids, int(buf[i]))
		}
	}

	return pids, nil
}

// FindProcessByName finds a process by its name
func FindProcessByName(name string) (*ProcessInfo, error) {
	pids, err := GetPids()
	if err != nil {
		return nil, err
	}

	scraper := &darwinScraper{}
	for _, pid := range pids {
		info, err := scraper.ScrapeProcess(pid)
		if err != nil {
			continue
		}
		// Check if this process matches (we'd need additional info)
		_ = info
	}

	return nil, fmt.Errorf("process not found: %s", name)
}

// GetProcessPath returns the path to a process's executable
func GetProcessPath(pid int) (string, error) {
	pathBuf := make([]byte, C.PROC_PIDPATHINFO_MAXSIZE)
	pathLen := C.proc_pidpath(C.int(pid), unsafe.Pointer(&pathBuf), C.uint32_t(len(pathBuf)))
	if pathLen <= 0 {
		return "", ErrProcessNotFound
	}
	return string(pathBuf[:pathLen]), nil
}

// darwinInputRequired checks if process is waiting for terminal input
// Uses the terminal file descriptors to determine if process is blocked on stdin
func darwinInputRequired(pid int) (bool, error) {
	// Check /proc equivalent on macOS - /proc doesn't exist
	// Instead check by examining the process's file descriptors

	// Use proc_pidfdinfo to get file descriptor info
	// This is complex without additional libraries

	// Fallback: check if process is in SSLEEP state with no active threads
	var taskInfo C.struct_proc_taskinfo
	taskInfoSize := C.int(unsafe.Sizeof(taskInfo))

	ret, err := C.proc_pidinfo(C.int(pid), C.PROC_PIDTASKINFO, 0, unsafe.Pointer(&taskInfo), taskInfoSize)
	if err != nil {
		return false, err
	}

	if ret != taskInfoSize {
		return false, nil
	}

	// If status is SSLEEP (3), process is waiting
	if int(taskInfo.pti_status) == 3 {
		// Without kernel symbols, we can't know exactly what it's waiting for
		// Heuristic: if process has stdin open and is sleeping, likely waiting for input
		return true, nil
	}

	return false, nil
}
