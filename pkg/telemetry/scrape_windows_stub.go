//go:build !windows

package telemetry

// windowsScraperStub implements Scraper stub for non-Windows platforms
// This provides the type definition but returns ErrNotAvailable on use
type windowsScraperStub struct{}

// newWindowsScraper returns a stub that indicates Windows is not available
func newWindowsScraper() Scraper {
	return &windowsScraperStub{}
}

// IsAvailable returns false on non-Windows platforms
func (s *windowsScraperStub) IsAvailable() bool {
	return false
}

// ScrapeProcess returns ErrNotAvailable on non-Windows platforms
func (s *windowsScraperStub) ScrapeProcess(pid int) (*ProcessInfo, error) {
	return nil, ErrNotAvailable
}
