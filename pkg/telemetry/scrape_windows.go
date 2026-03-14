//go:build windows

package telemetry

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

// Windows NT status codes
const (
	winSTATUS_SUCCESS              = 0x00000000
	winSTATUS_INFO_LENGTH_MISMATCH = 0xC0000004
	winSTATUS_ACCESS_DENIED        = 0xC0000022
	winSTATUS_INVALID_PARAMETER    = 0xC000000D
)

// System process information classes
const (
	winSystemProcessInformationClass = 5
	winSystemThreadInformationClass  = 5
)

// Windows internal process state values
const (
	winStateInitialized = 0
	winStateReady       = 2
	winStateRunning     = 3
	winStateWaiting     = 5
	winStateTerminated  = 6
)

// windowsScraper implements Scraper for Windows using NtQuerySystemInformation
type windowsScraper struct{}

// newWindowsScraper creates a new Windows scraper
func newWindowsScraper() Scraper {
	return &windowsScraper{}
}

// IsAvailable checks if Windows API is available
func (s *windowsScraper) IsAvailable() bool {
	return true
}

// NTSTATUS is the return type for NT functions
type NTSTATUS uint32

// unicodeString for NT API
type unicodeString struct {
	Length        uint16
	MaximumLength uint16
	Buffer        *uint16
}

// clientId identifies a process or thread
type clientId struct {
	UniqueProcess uintptr
	UniqueThread  uintptr
}

// systemProcessInformation contains process info from NtQuerySystemInformation
type systemProcessInformation struct {
	NextEntryOffset          uint32
	NumberOfThreads          uint32
	WorkingSetPrivateSize    int64
	SpareVoid1               [3]uint64
	PeakVirtualSize          uint64
	VirtualSize              uint64
	PageFaultCount           uint32
	PeakWorkingSetSize       uint64
	WorkingSetSize           uint64
	QuotaPeakPagedPoolUsage  uint64
	QuotaPagedPoolUsage      uint64
	QuotaPeakNonPagedPoolUsage uint64
	QuotaNonPagedPoolUsage   uint64
	PagefileUsage            uint64
	PeakPagefileUsage        uint64
	PrivatePageCount         int64
	LastSystemTime           int64
	ElapsedTime              int64
	ImageName                unicodeString
	BasePriority             int32
	UniqueProcessId          uintptr
	InheritedFromUniqueProcessId uintptr
	HandleCount              uint32
	SessionId                uint32
	UniqueProcessId2         uintptr
	SpareVoid3               [4]uintptr
}

// systemThreadInformation contains thread info from NtQuerySystemInformation
type systemThreadInformation struct {
	NextEntryOffset       uint32
	SpareVoid1            [3]uint64
	ClientId              clientId
	SpareVoid2            [3]uint64
	WaitReason            uint32
	State                 uint32
	Priority              int32
	BasePriority          int32
	TotalExecutedTime      int64
	SpareVoid3            [3]uint64
}

// NtQuerySystemInformation is the Windows NT function
// syscall convention for this varies, using stdcall
var (
	modkernel32 = syscall.MustLoadDLL("kernel32.dll")
	modntdll    = syscall.MustLoadDLL("ntdll.dll")

	procNtQuerySystemInformation = modntdll.MustFindProc("NtQuerySystemInformation")
)

// ntQuerySystemInformation calls the Windows NT function
func ntQuerySystemInformation(infoClass uint32, buffer unsafe.Pointer, length uint32, returnedLength *uint32) NTSTATUS {
	r1, _, _ := syscall.Syscall6(
		procNtQuerySystemInformation.Addr(),
		4,
		uintptr(infoClass),
		uintptr(buffer),
		uintptr(length),
		uintptr(unsafe.Pointer(returnedLength)),
		0,
		0,
	)
	return NTSTATUS(r1)
}

// ScrapeProcess queries Windows for process state using NtQuerySystemInformation
func (s *windowsScraper) ScrapeProcess(pid int) (*ProcessInfo, error) {
	// Allocate initial buffer
	bufSize := uint32(64 * 1024)
	buffer := make([]byte, bufSize)

	var returnedLength uint32

	// Query system process information
	for {
		status := ntQuerySystemInformation(
			winSystemProcessInformationClass,
			unsafe.Pointer(&buffer[0]),
			bufSize,
			&returnedLength,
		)

		if status == winSTATUS_SUCCESS {
			break
		}
		if status == winSTATUS_INFO_LENGTH_MISMATCH {
			// Buffer too small, resize
			bufSize = returnedLength + 4096
			buffer = make([]byte, bufSize)
			continue
		}
		if status == winSTATUS_ACCESS_DENIED {
			return nil, ErrPermissionDenied
		}
		return nil, fmt.Errorf("NtQuerySystemInformation failed: 0x%08x", status)
	}

	// Parse the process information
	offset := uint32(0)
	for {
		if offset >= returnedLength {
			break
		}

		// Cast buffer to systemProcessInformation
		procInfo := (*systemProcessInformation)(unsafe.Pointer(&buffer[offset]))

		// Check if this is our process
		if int(procInfo.UniqueProcessId) == pid {
			return s.parseProcessInfo(procInfo)
		}

		// Move to next entry
		if procInfo.NextEntryOffset == 0 {
			break
		}
		offset += procInfo.NextEntryOffset
	}

	return nil, ErrProcessNotFound
}

// parseProcessInfo converts Windows process info to our ProcessInfo
func (s *windowsScraper) parseProcessInfo(windowsProc *systemProcessInformation) (*ProcessInfo, error) {
	state := determineWindowsState(int(windowsProc.NumberOfThreads), 0) // WaitReason would need thread iteration
	waitChannel := ""

	// If we have threads, get thread details
	if windowsProc.NumberOfThreads > 0 {
		// For more detailed info, we'd need to iterate threads
		// For now, use process-level info
		waitChannel = fmt.Sprintf("pid:%d threads:%d", windowsProc.UniqueProcessId, windowsProc.NumberOfThreads)
	}

	return &ProcessInfo{
		PID:         int(windowsProc.UniqueProcessId),
		State:       state,
		WaitReason:  WaitReasonUnknown,
		WaitChannel: waitChannel,
	}, nil
}

// determineWindowsState determines ProcessState from Windows thread states
func determineWindowsState(numThreads int, waitReason uint32) ProcessState {
	if numThreads == 0 {
		return StateZombie
	}

	// WaitReason values (from winnt.h):
	// 0 = Executive
	// 1 = FreePage
	// 2 = PageIn
	// 3 = PoolAllocation
	// 4 = DelayExecution
	// 5 = Suspended
	// 6 = UserRequest
	// 7 = EventPairHigh
	// 8 = EventPairLow
	// 9 = LpcReceive
	// 10 = LpcReply
	// 11 = VirtualMemory
	// 12 = PageOut
	// ... many more

	// If waitReason indicates waiting
	if waitReason > 0 && waitReason < 12 {
		// Check if it's user input related
		switch waitReason {
		case 6: // UserRequest - could be waiting for input
			// Would need more context to determine if terminal input
			return StateInputRequired
		case 9: // LpcReceive - could be waiting for IPC
			return StateWaiting
		default:
			return StateWaiting
		}
	}

	return StateRunning
}

// ProcessStateFromNTStatus converts NT wait reason to our WaitReason
func ProcessStateFromNTStatus(waitReason uint32) WaitReason {
	switch waitReason {
	case 0: // Executive
		return WaitReasonUnknown
	case 1: // FreePage
		return WaitReasonIO
	case 2: // PageIn
		return WaitReasonIO
	case 3: // PoolAllocation
		return WaitReasonUnknown
	case 4: // DelayExecution
		return WaitReasonTimer
	case 5: // Suspended
		return WaitReasonUnknown
	case 6: // UserRequest
		// Could be terminal input - we treat as user_input since PTY would be UserRequest
		return WaitReasonUserInput
	case 7, 8: // EventPair
		return WaitReasonUnknown
	case 9: // LpcReceive
		return WaitReasonUnknown
	case 10: // LpcReply
		return WaitReasonUnknown
	case 11: // VirtualMemory
		return WaitReasonIO
	case 12: // PageOut
		return WaitReasonIO
	default:
		return WaitReasonUnknown
	}
}

// GetCurrentProcessInfo returns info about the current process
func GetCurrentProcessInfo() (*ProcessInfo, error) {
	pid := os.Getpid()
	return (&windowsScraper{}).ScrapeProcess(pid)
}

// WaitForInputWithTimeout waits for a process to no longer be waiting for input
func WaitForInputWithTimeout(pid int, timeoutMs int) (bool, error) {
	scraper := &windowsScraper{}
	interval := 100
	elapsed := 0

	for elapsed < timeoutMs {
		info, err := scraper.ScrapeProcess(pid)
		if err != nil {
			if err == ErrProcessNotFound {
				return false, nil
			}
			return false, err
		}

		if DetectInputRequired(info) {
			return true, nil
		}

		if info.State == StateRunning {
			return false, nil
		}

		_ = interval
		elapsed += interval
	}

	return false, nil
}

// GetWindowTitles returns window titles for processes (if available)
func GetWindowTitles(pid int) ([]string, error) {
	// This would require additional Windows API calls (EnumWindows, GetWindowThreadProcessId)
	// Return empty for now as it's complex
	return []string{}, nil
}
