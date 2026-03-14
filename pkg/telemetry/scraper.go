//go:build linux || darwin || windows

// Package telemetry provides PTY wait-state detection for non-compliant AI agents.
// It scrapes kernel-level telemetry to detect when a process is waiting for input.
package telemetry

import (
	"errors"
	"runtime"
)

// ProcessState represents the wait state of a process
type ProcessState string

const (
	// StateUnknown indicates the process state could not be determined
	StateUnknown ProcessState = "UNKNOWN"
	// StateRunning indicates the process is actively running
	StateRunning ProcessState = "RUNNING"
	// StateWaiting indicates the process is waiting (e.g., blocked on I/O)
	StateWaiting ProcessState = "WAITING"
	// StateSleeping indicates the process is sleeping
	StateSleeping ProcessState = "SLEEPING"
	// StateInputRequired indicates the process (PTY) is waiting for user input
	StateInputRequired ProcessState = "INPUT_REQUIRED"
	// StateStopped indicates the process is stopped
	StateStopped ProcessState = "STOPPED"
	// StateZombie indicates the process is a zombie
	StateZombie ProcessState = "ZOMBIE"
)

// WaitReason provides additional context about why a process is waiting
type WaitReason string

const (
	// WaitReasonUnknown - unknown wait reason
	WaitReasonUnknown WaitReason = "unknown"
	// WaitReasonUserInput - waiting for user input (terminal read)
	WaitReasonUserInput WaitReason = "user_input"
	// WaitReasonIO - waiting for I/O completion
	WaitReasonIO WaitReason = "io"
	// WaitReasonTimer - waiting on a timer
	WaitReasonTimer WaitReason = "timer"
	// WaitReasonPoll - waiting on poll/select
	WaitReasonPoll WaitReason = "poll"
	// WaitReasonNetwork - waiting for network
	WaitReasonNetwork WaitReason = "network"
	// WaitReasonLock - waiting on a lock
	WaitReasonLock WaitReason = "lock"
)

// ProcessInfo contains information about a scraped process
type ProcessInfo struct {
	PID         int
	State       ProcessState
	WaitReason  WaitReason
	WaitChannel string // kernel symbol or file descriptor
}

// Scraper defines the interface for platform-specific wait-state scrapers
type Scraper interface {
	// ScrapeProcess reads kernel telemetry for a specific process
	ScrapeProcess(pid int) (*ProcessInfo, error)
	// IsAvailable checks if the scraper is available on this platform
	IsAvailable() bool
}

// Errors for scraper operations
var (
	ErrProcessNotFound  = errors.New("process not found")
	ErrPermissionDenied = errors.New("permission denied")
	ErrNotAvailable     = errors.New("scraper not available on this platform")
)

// NewScraper creates a platform-appropriate scraper
//go:generate go run .
func NewScraper() Scraper {
	switch runtime.GOOS {
	case "linux":
		return newLinuxScraper()
	case "darwin":
		return newDarwinScraper()
	case "windows":
		return newWindowsScraper()
	default:
		return newFallbackScraper()
	}
}
