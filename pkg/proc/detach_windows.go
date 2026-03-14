//go:build windows

package proc

import (
	"syscall"
)

// DetachFlags returns the process creation flags for detaching a process
// from the parent terminal on Windows.
// - CREATE_NEW_PROCESS_GROUP: Creates the process in a new process group
// - DETACHED_PROCESS: Prevents the process from attaching to the console
// - CREATE_NO_WINDOW: Creates the process without a console window
func DetachFlags() uint32 {
	// These are standard Windows process creation flags
	// CREATE_NEW_PROCESS_GROUP = 0x00000200
	// DETACHED_PROCESS = 0x00000008
	// CREATE_NO_WINDOW = 0x08000000
	const (
		CREATE_NEW_PROCESS_GROUP uint32 = 0x00000200
		DETACHED_PROCESS         uint32 = 0x00000008
		CREATE_NO_WINDOW         uint32 = 0x08000000
	)
	return CREATE_NEW_PROCESS_GROUP | DETACHED_PROCESS | CREATE_NO_WINDOW
}

// DetachSysProcAttr returns the SysProcAttr for detaching a process on Windows.
func DetachSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		CreationFlags: DetachFlags(),
	}
}
