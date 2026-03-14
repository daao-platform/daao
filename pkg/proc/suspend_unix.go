//go:build unix || linux || darwin

package proc

import (
	"fmt"
	"os"
	"syscall"
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

// UnixProcess implements SuspendableProcess for Unix-like systems using SIGSTOP/SIGCONT
type UnixProcess struct {
	pid int
}

// NewUnixProcess creates a new UnixProcess wrapper
func NewUnixProcess(pid int) *UnixProcess {
	return &UnixProcess{pid: pid}
}

// PID returns the process ID
func (p *UnixProcess) PID() int {
	return p.pid}

// Suspend stops the process using SIGSTOP
func (p *UnixProcess) Suspend() error {
	process, err := os.FindProcess(p.pid)
	if err != nil {
		return fmt.Errorf("failed to find process %d: %w", p.pid, err)
	}

	// Send SIGSTOP to suspend the process
	if err := process.Signal(syscall.SIGSTOP); err != nil {
		return fmt.Errorf("failed to send SIGSTOP to process %d: %w", p.pid, err)
	}

	return nil
}

// Resume continues the process using SIGCONT
func (p *UnixProcess) Resume() error {
	process, err := os.FindProcess(p.pid)
	if err != nil {
		return fmt.Errorf("failed to find process %d: %w", p.pid, err)
	}

	// Send SIGCONT to resume the process
	if err := process.Signal(syscall.SIGCONT); err != nil {
		return fmt.Errorf("failed to send SIGCONT to process %d: %w", p.pid, err)
	}

	return nil
}

// SuspendProcess suspends a process by PID using SIGSTOP
func SuspendProcess(pid int) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("failed to find process %d: %w", pid, err)
	}

	// Send SIGSTOP to suspend the process
	if err := process.Signal(syscall.SIGSTOP); err != nil {
		return fmt.Errorf("failed to send SIGSTOP to process %d: %w", pid, err)
	}

	return nil
}

// ResumeProcess resumes a process by PID using SIGCONT
func ResumeProcess(pid int) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("failed to find process %d: %w", pid, err)
	}

	// Send SIGCONT to resume the process
	if err := process.Signal(syscall.SIGCONT); err != nil {
		return fmt.Errorf("failed to send SIGCONT to process %d: %w", pid, err)
	}

	return nil
}
