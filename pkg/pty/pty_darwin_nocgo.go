//go:build darwin && !cgo

package pty

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

// darwinPty implements the Pty interface using pure-Go syscalls (no cgo).
// This implementation uses posix_openpt for cross-compilation support.
type darwinPty struct {
	master    *os.File
	slave     *os.File
	closeOnce sync.Once
}

// NewPty creates a new PTY using pure-Go syscalls on macOS.
func NewPty(cols, rows uint16) (Pty, error) {
	// Open the master PTY — O_RDWR | O_NOCTTY
	masterFd, err := syscall.Open("/dev/ptmx", syscall.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		return nil, fmt.Errorf("open /dev/ptmx: %w", err)
	}

	// grantpt — on macOS this is a no-op, but we call ioctl TIOCPTYGRANT
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(masterFd), uintptr(0x20007454), 0); errno != 0 {
		// TIOCPTYGRANT = 0x20007454
		syscall.Close(masterFd)
		return nil, fmt.Errorf("grantpt: %v", errno)
	}

	// unlockpt — ioctl TIOCPTYUNLK
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(masterFd), uintptr(0x20007452), 0); errno != 0 {
		// TIOCPTYUNLK = 0x20007452
		syscall.Close(masterFd)
		return nil, fmt.Errorf("unlockpt: %v", errno)
	}

	// ptsname — ioctl TIOCPTYGNAME
	var slaveName [128]byte
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(masterFd), uintptr(0x40807453), uintptr(unsafe.Pointer(&slaveName[0]))); errno != 0 {
		// TIOCPTYGNAME = 0x40807453
		syscall.Close(masterFd)
		return nil, fmt.Errorf("ptsname: %v", errno)
	}

	// Find null terminator
	slaveNameStr := ""
	for i, b := range slaveName {
		if b == 0 {
			slaveNameStr = string(slaveName[:i])
			break
		}
	}

	// Open slave
	slaveFd, err := syscall.Open(slaveNameStr, syscall.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		syscall.Close(masterFd)
		return nil, fmt.Errorf("open slave %s: %w", slaveNameStr, err)
	}

	master := os.NewFile(uintptr(masterFd), "/dev/ptmx")
	slave := os.NewFile(uintptr(slaveFd), slaveNameStr)

	// Set initial terminal size
	ws := &unix.Winsize{Col: cols, Row: rows}
	if err := unix.IoctlSetWinsize(masterFd, unix.TIOCSWINSZ, ws); err != nil {
		master.Close()
		slave.Close()
		return nil, fmt.Errorf("set winsize: %w", err)
	}

	return &darwinPty{master: master, slave: slave}, nil
}

// Start spawns a process attached to this PTY.
func (p *darwinPty) Start(binary string, args []string, dir string, env []string, detachFlags uint32) (*os.Process, error) {
	fullPath, err := exec.LookPath(binary)
	if err != nil {
		return nil, fmt.Errorf("failed to find binary %s: %w", binary, err)
	}

	cmd := exec.Command(fullPath, args[1:]...)
	cmd.Dir = dir
	cmd.Env = env
	cmd.Stdin = p.slave
	cmd.Stdout = p.slave
	cmd.Stderr = p.slave

	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid:  true,
		Setctty: true,
		Ctty:    0,
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start process: %w", err)
	}

	p.slave.Close()
	p.slave = nil

	return cmd.Process, nil
}

func (p *darwinPty) Read(b []byte) (int, error)  { return p.master.Read(b) }
func (p *darwinPty) Write(b []byte) (int, error) { return p.master.Write(b) }

func (p *darwinPty) Resize(cols, rows uint16) error {
	ws := &unix.Winsize{Col: cols, Row: rows}
	return unix.IoctlSetWinsize(int(p.master.Fd()), unix.TIOCSWINSZ, ws)
}

func (p *darwinPty) Fd() uintptr                       { return p.master.Fd() }
func (p *darwinPty) SetReadDeadline(t time.Time) error { return p.master.SetReadDeadline(t) }

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
