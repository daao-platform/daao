//go:build !linux

package telemetry

// linuxScraperStub implements Scraper stub for non-Linux platforms
// This provides the type definition but returns ErrNotAvailable on use
type linuxScraperStub struct{}

// newLinuxScraper returns a stub that indicates Linux is not available
func newLinuxScraper() Scraper {
	return &linuxScraperStub{}
}

// IsAvailable returns false on non-Linux platforms
func (s *linuxScraperStub) IsAvailable() bool {
	return false
}

// ScrapeProcess returns ErrNotAvailable on non-Linux platforms
func (s *linuxScraperStub) ScrapeProcess(pid int) (*ProcessInfo, error) {
	return nil, ErrNotAvailable
}

// Linux-specific functions that aren't available on other platforms
func FindPtyProcess() (*ProcessInfo, error) {
	return nil, ErrNotAvailable
}

func GetProcessWaitInfo(pid int) (string, ProcessState, WaitReason, error) {
	return "", StateUnknown, WaitReasonUnknown, ErrNotAvailable
}

func WaitForInput(pid int, timeoutMs int) (bool, error) {
	return false, ErrNotAvailable
}

func GetInputRequiredPids() ([]int, error) {
	return nil, ErrNotAvailable
}
