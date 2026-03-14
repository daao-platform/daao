//go:build linux

package pty

import (
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// unixPty implements the Pty interface using POSIX openpty.
type unixPty struct {
	master    *os.File
	slave     *os.File
	closeOnce sync.Once
}

// NewPty creates a new PTY using POSIX openpty.
func NewPty(cols, rows uint16) (Pty, error) {
	masterFd, slaveFd, err := openpty()
	if err != nil {
		return nil, fmt.Errorf("openpty: %w", err)
	}

	master := os.NewFile(uintptr(masterFd), "/dev/ptmx")
	slave := os.NewFile(uintptr(slaveFd), "slave-pty")

	// Set initial terminal size
	ws := &unix.Winsize{Col: cols, Row: rows}
	if err := unix.IoctlSetWinsize(masterFd, unix.TIOCSWINSZ, ws); err != nil {
		master.Close()
		slave.Close()
		return nil, fmt.Errorf("set winsize: %w", err)
	}

	return &unixPty{master: master, slave: slave}, nil
}

// openpty creates a PTY master/slave pair.
func openpty() (master int, slave int, err error) {
	master, err = unix.Open("/dev/ptmx", unix.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		return -1, -1, fmt.Errorf("open /dev/ptmx: %w", err)
	}

	// Grant and unlock the slave
	if _, err := unix.IoctlGetInt(master, unix.TIOCGPTN); err != nil {
		unix.Close(master)
		return -1, -1, fmt.Errorf("TIOCGPTN: %w", err)
	}

	unlock := 0
	if err := unix.IoctlSetPointerInt(master, unix.TIOCSPTLCK, unlock); err != nil {
		unix.Close(master)
		return -1, -1, fmt.Errorf("TIOCSPTLCK: %w", err)
	}

	// Get the slave name
	ptsNum, err := unix.IoctlGetInt(master, unix.TIOCGPTN)
	if err != nil {
		unix.Close(master)
		return -1, -1, fmt.Errorf("TIOCGPTN: %w", err)
	}

	slaveName := fmt.Sprintf("/dev/pts/%d", ptsNum)
	slave, err = unix.Open(slaveName, unix.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		unix.Close(master)
		return -1, -1, fmt.Errorf("open %s: %w", slaveName, err)
	}

	return master, slave, nil
}

// Start spawns a process attached to this PTY.
func (p *unixPty) Start(binary string, args []string, dir string, env []string, detachFlags uint32) (*os.Process, error) {
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
	// it to fd 0 in the child via dup2. The parent's p.slave.Fd() (e.g. fd 5)
	// would be invalid in the child, causing "Ctty not valid in child".
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
func (p *unixPty) Read(b []byte) (int, error) {
	return p.master.Read(b)
}

// Write writes to the PTY master (child input).
func (p *unixPty) Write(b []byte) (int, error) {
	return p.master.Write(b)
}

// Resize changes the terminal dimensions.
func (p *unixPty) Resize(cols, rows uint16) error {
	ws := &unix.Winsize{Col: cols, Row: rows}
	return unix.IoctlSetWinsize(int(p.master.Fd()), unix.TIOCSWINSZ, ws)
}

// Fd returns the master file descriptor.
func (p *unixPty) Fd() uintptr {
	return p.master.Fd()
}

// SetReadDeadline sets the read deadline on the master.
func (p *unixPty) SetReadDeadline(t time.Time) error {
	return p.master.SetReadDeadline(t)
}

// Close releases all PTY resources. Safe to call multiple times.
func (p *unixPty) Close() error {
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
