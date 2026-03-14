// Package pty provides pseudo-terminal functionality
package pty

import (
	"os"
	"time"
)

// Pty represents a pseudo-terminal interface
type Pty interface {
	Read(b []byte) (int, error)
	Write(b []byte) (int, error)
	Close() error
	Resize(cols, rows uint16) error
	Fd() uintptr
	SetReadDeadline(t time.Time) error
	Start(binary string, args []string, dir string, env []string, detachFlags uint32) (*os.Process, error)
}
