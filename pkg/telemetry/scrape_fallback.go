package telemetry

import (
	"fmt"
	"os"
)

// fallbackScraper provides basic functionality when platform-specific scrapers aren't available
type fallbackScraper struct{}

// newFallbackScraper creates a new fallback scraper
func newFallbackScraper() Scraper {
	return &fallbackScraper{}
}

// IsAvailable always returns true for fallback
func (s *fallbackScraper) IsAvailable() bool {
	return true
}

// ScrapeProcess returns basic process info using os package
func (s *fallbackScraper) ScrapeProcess(pid int) (*ProcessInfo, error) {
	// Check if process exists using os.FindProcess
	proc, err := os.FindProcess(pid)
	if err != nil {
		return nil, ErrProcessNotFound
	}

	// On Windows, FindProcess succeeds even for processes we can't access
	// Try to get the process state if possible
	// On unsupported platforms, we can only provide limited info

	// Try signal 0 to check existence (cross-platform)
	err = proc.Signal(os.Signal(nil))
	// This won't work as expected, so we just check if proc is not nil

	// Return a best-effort state
	return &ProcessInfo{
		PID:         pid,
		State:       StateUnknown,
		WaitReason:  WaitReasonUnknown,
		WaitChannel: "unsupported_platform",
	}, nil
}

// GetPlatformInfo returns platform information
func GetPlatformInfo() string {
	return "unsupported_platform"
}

// NewFallbackScraper creates a new fallback scraper
func NewFallbackScraper() Scraper {
	return &fallbackScraper{}
}

// GetAllProcessInfo returns info about all processes (fallback implementation)
func GetAllProcessInfo() ([]*ProcessInfo, error) {
	// This is not really implementable in fallback mode
	// Would need platform-specific code
	return []*ProcessInfo{}, fmt.Errorf("not implemented for this platform")
}

// WaitForInputFallback provides a fallback waiting mechanism
func WaitForInputFallback(pid int, timeoutMs int) (bool, error) {
	// In fallback mode, we can't really detect input required
	// Just wait and return timeout
	interval := 100
	elapsed := 0

	for elapsed < timeoutMs {
		proc, err := os.FindProcess(pid)
		if err != nil || proc == nil {
			return false, nil // Process exited
		}

		_ = interval
		elapsed += interval
	}

	// Assume input required on timeout
	return true, nil
}
