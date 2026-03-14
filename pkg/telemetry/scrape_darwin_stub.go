//go:build !darwin

package telemetry

// darwinScraperStub implements Scraper stub for non-Darwin platforms
// This provides the type definition but returns ErrNotAvailable on use
type darwinScraperStub struct{}

// newDarwinScraper returns a stub that indicates Darwin is not available
func newDarwinScraper() Scraper {
	return &darwinScraperStub{}
}

// IsAvailable returns false on non-Darwin platforms
func (s *darwinScraperStub) IsAvailable() bool {
	return false
}

// ScrapeProcess returns ErrNotAvailable on non-Darwin platforms
func (s *darwinScraperStub) ScrapeProcess(pid int) (*ProcessInfo, error) {
	return nil, ErrNotAvailable
}

// Stub functions for darwin - return errors on non-Darwin
func GetPids() ([]int, error) {
	return nil, ErrNotAvailable
}

func FindProcessByName(name string) (*ProcessInfo, error) {
	return nil, ErrNotAvailable
}

func GetProcessPath(pid int) (string, error) {
	return "", ErrNotAvailable
}
