//go:build unix || linux || darwin

package proc

import (
	"syscall"
)

// DetachAttrs returns the process attributes for detaching a process
// from the parent terminal on Unix-like systems (Linux, macOS).
// Setsid: true creates a new session, preventing SIGHUP propagation
// when the parent terminal closes.
func DetachAttrs() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		Setsid: true,
	}
}
