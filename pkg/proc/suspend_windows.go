//go:build windows

package proc

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

// SuspendableProcess defines the interface for suspending and resuming a process
type SuspendableProcess interface {
	// Suspend stops the process
	Suspend() error
	// Resume continues the process
	Resume() error
	// PID returns the process ID
	PID() int
}

// WindowsProcess implements SuspendableProcess for Windows using NtSuspendProcess/NtResumeProcess
type WindowsProcess struct {
	pid int
}

// NewWindowsProcess creates a new WindowsProcess wrapper
func NewWindowsProcess(pid int) *WindowsProcess {
	return &WindowsProcess{pid: pid}
}

// PID returns the process ID
func (p *WindowsProcess) PID() int {
	return p.pid
}

// Suspend stops the process using NtSuspendProcess
func (p *WindowsProcess) Suspend() error {
	return SuspendProcess(p.pid)
}

// Resume continues the process using NtResumeProcess
func (p *WindowsProcess) Resume() error {
	return ResumeProcess(p.pid)
}

// ntSuspendProcess is the Windows API function to suspend a process
// It returns 0 on success, or an error code on failure
// syscall.SYS_NTSUSPENDPROCESS is not directly available, so we use stdcall inline
func ntSuspendProcess(handle windows.Handle) (uintptr, error) {
	// NtSuspendProcess is exported from ntdll.dll
	// We use the syscold calling convention via golang.org/x/sys/windows
	// The function takes a process handle and returns STATUS_SUCCESS (0x00000000) on success
	var ret uintptr
	var err error

	// Try to get the address of NtSuspendProcess
	kernel32 := windows.MustLoadDLL("kernel32.dll")
	ntdll := windows.MustLoadDLL("ntdll.dll")

	// NtSuspendProcess is in ntdll.dll
	ntSuspendProc, err := ntdll.FindProc("NtSuspendProcess")
	if err != nil {
		return 0, fmt.Errorf("failed to find NtSuspendProcess: %w", err)
	}

	// Call NtSuspendProcess(handle)
	ret, _, _ = ntSuspendProc.Call(uintptr(handle))
	if ret != 0 {
		return ret, fmt.Errorf("NtSuspendProcess failed with status: 0x%x", ret)
	}

	// Also keep a reference to kernel32 to prevent it from being GC'd
	_ = kernel32
	return ret, nil
}

// ntResumeProcess is the Windows API function to resume a process
func ntResumeProcess(handle windows.Handle) (uintptr, error) {
	ntdll := windows.MustLoadDLL("ntdll.dll")
	ntResumeProc, err := ntdll.FindProc("NtResumeProcess")
	if err != nil {
		return 0, fmt.Errorf("failed to find NtResumeProcess: %w", err)
	}

	ret, _, _ := ntResumeProc.Call(uintptr(handle))
	if ret != 0 {
		return ret, fmt.Errorf("NtResumeProcess failed with status: 0x%x", ret)
	}

	return ret, nil
}

// SuspendProcess suspends a process by PID using NtSuspendProcess
func SuspendProcess(pid int) error {
	// Open the process with PROCESS_SUSPEND_RESUME access rights
	handle, err := windows.OpenProcess(windows.PROCESS_SUSPEND_RESUME, false, uint32(pid))
	if err != nil {
		return fmt.Errorf("failed to open process %d: %w", pid, err)
	}
	defer windows.CloseHandle(handle) // nolint:errcheck

	// Call NtSuspendProcess
	_, err = ntSuspendProcess(handle)
	if err != nil {
		return fmt.Errorf("failed to suspend process %d: %w", pid, err)
	}

	return nil
}

// ResumeProcess resumes a process by PID using NtResumeProcess
func ResumeProcess(pid int) error {
	// Open the process with PROCESS_SUSPEND_RESUME access rights
	handle, err := windows.OpenProcess(windows.PROCESS_SUSPEND_RESUME, false, uint32(pid))
	if err != nil {
		return fmt.Errorf("failed to open process %d: %w", pid, err)
	}
	defer windows.CloseHandle(handle) // nolint:errcheck

	// Call NtResumeProcess
	_, err = ntResumeProcess(handle)
	if err != nil {
		return fmt.Errorf("failed to resume process %d: %w", pid, err)
	}

	return nil
}

// SuspendSelf suspends the current process
// This is useful for the process to suspend itself
func SuspendSelf() error {
	return SuspendProcess(os.Getpid())
}

// ResumeSelf resumes the current process
func ResumeSelf() error {
	return ResumeProcess(os.Getpid())
}
