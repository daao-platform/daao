//go:build darwin && cgo

package pty

/*
#include <stdlib.h>
#include <util.h>
*/
import "C"

import (
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

// darwinPty implements the Pty interface using macOS openpty via cgo.
type darwinPty struct {
	master    *os.File
	slave     *os.File
	closeOnce sync.Once
}

// NewPty creates a new PTY using macOS openpty.
func NewPty(cols, rows uint16) (Pty, error) {
	var masterFd, slaveFd C.int

	// openpty() from <util.h> creates a master/slave PTY pair.
	// It handles grantpt/unlockpt/ptsname internally on macOS.
	if rc := C.openpty(&masterFd, &slaveFd, nil, nil, nil); rc != 0 {
		return nil, fmt.Errorf("openpty failed with rc=%d", int(rc))
	}

	master := os.NewFile(uintptr(masterFd), "/dev/ptmx")
	slave := os.NewFile(uintptr(slaveFd), "slave-pty")

	// Set initial terminal size
	ws := &unix.Winsize{Col: cols, Row: rows}
	if err := unix.IoctlSetWinsize(int(masterFd), unix.TIOCSWINSZ, ws); err != nil {
		master.Close()
		slave.Close()
		return nil, fmt.Errorf("set winsize: %w", err)
	}

	return &darwinPty{master: master, slave: slave}, nil
}

// Start spawns a process attached to this PTY.
func (p *darwinPty) Start(binary string, args []string, dir string, env []string, detachFlags uint32) (*os.Process, error) {
	// Resolve binary
	fullPath, err := exec.LookPath(binary)
	if err != nil {
		return nil, fmt.Errorf("failed to find binary %s: %w", binary, err)
	}

	// Build command line
	cmd := exec.Command(fullPath, args[1:]...)
	cmd.Dir = dir
	cmd.Env = env

	// Attach to PTY slave
	cmd.Stdin = p.slave
	cmd.Stdout = p.slave
	cmd.Stderr = p.slave

	// Start a new session so the child gets the PTY as its controlling terminal.
	// Ctty must be a fd valid IN THE CHILD — since cmd.Stdin = p.slave, Go maps
	// it to fd 0 in the child via dup2.
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid:  true,
		Setctty: true,
		Ctty:    0, // fd 0 = stdin in the child = the slave PTY
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start process: %w", err)
	}

	// Close the slave in the parent — the child has its own copy.
	// This ensures master.Read() returns EOF when the child exits.
	p.slave.Close()
	p.slave = nil

	return cmd.Process, nil
}

// Read reads from the PTY master (child output).
func (p *darwinPty) Read(b []byte) (int, error) {
	return p.master.Read(b)
}

// Write writes to the PTY master (child input).
func (p *darwinPty) Write(b []byte) (int, error) {
	return p.master.Write(b)
}

// Resize changes the terminal dimensions.
func (p *darwinPty) Resize(cols, rows uint16) error {
	ws := &unix.Winsize{Col: cols, Row: rows}
	return unix.IoctlSetWinsize(int(p.master.Fd()), unix.TIOCSWINSZ, ws)
}

// Fd returns the master file descriptor.
func (p *darwinPty) Fd() uintptr {
	return p.master.Fd()
}

// SetReadDeadline sets the read deadline on the master.
func (p *darwinPty) SetReadDeadline(t time.Time) error {
	return p.master.SetReadDeadline(t)
}

// Close releases all PTY resources. Safe to call multiple times.
func (p *darwinPty) Close() error {
	var firstErr error
	p.closeOnce.Do(func() {
		if p.slave != nil {
			if err := p.slave.Close(); err != nil && firstErr == nil {
				firstErr = err
			}
			p.slave = nil
		}
		if p.master != nil {
			if err := p.master.Close(); err != nil && firstErr == nil {
				firstErr = err
			}
			p.master = nil
		}
	})
	return firstErr
}

// Ensure cgo types are referenced to prevent "imported and not used" errors.
var _ = unsafe.Sizeof(C.int(0))
