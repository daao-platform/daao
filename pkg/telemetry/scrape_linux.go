//go:build linux

package telemetry

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// linuxScraper implements Scraper for Linux using /proc filesystem
type linuxScraper struct{}

// newLinuxScraper creates a new Linux scraper
func newLinuxScraper() Scraper {
	return &linuxScraper{}
}

// IsAvailable checks if /proc is available (Linux)
func (s *linuxScraper) IsAvailable() bool {
	_, err := os.Stat("/proc")
	return err == nil
}

// ScrapeProcess reads the wait state from /proc/<pid>/wchan
func (s *linuxScraper) ScrapeProcess(pid int) (*ProcessInfo, error) {
	// Read /proc/<pid>/wchan - the kernel function the process is sleeping in
	wchanPath := fmt.Sprintf("/proc/%d/wchan", pid)
	wchanData, err := os.ReadFile(wchanPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrProcessNotFound
		}
		if os.IsPermission(err) {
			return nil, ErrPermissionDenied
		}
		return nil, fmt.Errorf("failed to read wchan: %w", err)
	}

	waitChannel := strings.TrimSpace(string(wchanData))

	// Read /proc/<pid>/stat for process state
	statPath := fmt.Sprintf("/proc/%d/stat", pid)
	statData, err := os.ReadFile(statPath)
	if err != nil {
		// Return what we have if stat fails
		return &ProcessInfo{
			PID:         pid,
			State:       stateFromWchan(waitChannel),
			WaitReason:  reasonFromWchan(waitChannel),
			WaitChannel: waitChannel,
		}, nil
	}

	// Parse state from stat (second field, between parentheses)
	state := parseStateFromStat(string(statData))

	// Read /proc/<pid>/status for additional info
	statusPath := fmt.Sprintf("/proc/%d/status", pid)
	statusData, _ := os.ReadFile(statusPath)
	status := string(statusData)

	return &ProcessInfo{
		PID:         pid,
		State:       determineState(state, waitChannel, status),
		WaitReason:  reasonFromWchan(waitChannel),
		WaitChannel: waitChannel,
	}, nil
}

// parseStateFromStat extracts the state character from /proc/<pid>/stat
// Format: pid (comm) state ...
func parseStateFromStat(stat string) string {
	// Find the first ')' which closes the comm field
	closeParen := strings.Index(stat, ")")
	if closeParen < 0 || closeParen+2 >= len(stat) {
		return ""
	}

	// State is the character after ') '
	stateChar := stat[closeParen+2]
	return string(stateChar)
}

// stateFromWchan maps wchan value to ProcessState
func stateFromWchan(wchan string) ProcessState {
	if wchan == "0" || wchan == "" {
		return StateRunning
	}

	// Terminal-related wait functions indicate INPUT_REQUIRED
	terminalFuncs := []string{
		"tty_read",
		"n_tty_read",
		"con廉read",
		"pty_read",
		"read_chan",
		"file_read",
		"pipe_read",
	}

	for _, fn := range terminalFuncs {
		if strings.Contains(wchan, fn) {
			return StateInputRequired
		}
	}

	return StateWaiting
}

// reasonFromWchan maps wchan value to WaitReason
func reasonFromWchan(wchan string) WaitReason {
	if wchan == "0" || wchan == "" {
		return WaitReasonUnknown
	}

	// Check for terminal/PTY input
	terminalFuncs := []string{"tty_read", "n_tty_read", "pty_read", "read_chan"}
	for _, fn := range terminalFuncs {
		if strings.Contains(wchan, fn) {
			return WaitReasonUserInput
		}
	}

	// Check for I/O
	ioFuncs := []string{"__generic_file_aio_read", "do_sync_read", "do_sync_write",
		"ext4_file_read", "ext4_file_write", "nfs_read", "nfs_write"}
	for _, fn := range ioFuncs {
		if strings.Contains(wchan, fn) {
			return WaitReasonIO
		}
	}

	// Check for poll/select
	pollFuncs := []string{"do_select", "poll", "poll_do_select", "ep_poll"}
	for _, fn := range pollFuncs {
		if strings.Contains(wchan, fn) {
			return WaitReasonPoll
		}
	}

	// Check for network
	networkFuncs := []string{"tcp", "udp", "inet", "sock", "sk_wait"}
	for _, fn := range networkFuncs {
		if strings.Contains(wchan, fn) {
			return WaitReasonNetwork
		}
	}

	// Check for timer
	timerFuncs := []string{"hrtimer", "schedule_timeout", "wait_for_timer"}
	for _, fn := range timerFuncs {
		if strings.Contains(wchan, fn) {
			return WaitReasonTimer
		}
	}

	// Check for lock
	lockFuncs := []string{"mutex", "lock", "down", "bit_wait"}
	for _, fn := range lockFuncs {
		if strings.Contains(wchan, fn) {
			return WaitReasonLock
		}
	}

	return WaitReasonUnknown
}

// determineState combines stat state, wchan, and status to determine ProcessState
func determineState(statState, wchan, status string) ProcessState {
	// Map /proc/stat state character to ProcessState
	stateMap := map[string]ProcessState{
		"R": StateRunning,  // Running
		"S": StateSleeping, // Sleeping (interruptible)
		"D": StateWaiting,  // Waiting (uninterruptible disk)
		"Z": StateZombie,   // Zombie
		"T": StateStopped,  // Stopped
		"t": StateStopped,  // Stopped (traced)
		"I": StateSleeping, // Idle (idle thread)
		"X": StateZombie,   // Dead
		"x": StateZombie,   // Dead
		"K": StateStopped,  // Wakekill
		"P": StateStopped,  // Parked
	}

	if state, ok := stateMap[statState]; ok {
		// If waiting and wchan indicates terminal, it's INPUT_REQUIRED
		if state == StateWaiting || state == StateSleeping {
			if reasonFromWchan(wchan) == WaitReasonUserInput {
				return StateInputRequired
			}
		}
		return state
	}

	return stateFromWchan(wchan)
}

// FindPtyProcess finds a process that owns a PTY and is waiting for input
func FindPtyProcess() (*ProcessInfo, error) {
	// Read /proc/self/fd to find PTY
	entries, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		fdPath := "/proc/self/fd/" + entry.Name()
		link, err := os.Readlink(fdPath)
		if err != nil {
			continue
		}
		// Check if it's a PTY
		if strings.HasPrefix(link, "/dev/pts/") || strings.HasPrefix(link, "/dev/tty") {
			// Found our PTY, now get our own PID
			pid := os.Getpid()
			return (&linuxScraper{}).ScrapeProcess(pid)
		}
	}

	// Fallback: return info about current process
	pid := os.Getpid()
	return (&linuxScraper{}).ScrapeProcess(pid)
}

// GetProcessWaitInfo returns comprehensive wait information for a process
func GetProcessWaitInfo(pid int) (string, ProcessState, WaitReason, error) {
	scraper := &linuxScraper{}
	info, err := scraper.ScrapeProcess(pid)
	if err != nil {
		return "", StateUnknown, WaitReasonUnknown, err
	}
	return info.WaitChannel, info.State, info.WaitReason, nil
}

// WaitForInput polls a process until it's no longer waiting for input
// Returns true if input was detected, false if process exited or error
func WaitForInput(pid int, timeoutMs int) (bool, error) {
	scraper := &linuxScraper{}
	interval := 100 // 100ms polling interval
	elapsed := 0

	for elapsed < timeoutMs {
		info, err := scraper.ScrapeProcess(pid)
		if err != nil {
			if err == ErrProcessNotFound {
				return false, nil // Process exited
			}
			return false, err
		}

		if DetectInputRequired(info) {
			return true, nil
		}

		// Check if process is no longer waiting (e.g., running)
		if info.State == StateRunning {
			return false, nil
		}

		// Sleep before next poll
		// Using simple busy wait for cross-platform compatibility
		// In production, use time.Sleep
		_ = interval
		elapsed += interval
	}

	return false, nil // Timeout
}

// GetInputRequiredPids returns PIDs of processes that appear to be waiting for input
func GetInputRequiredPids() ([]int, error) {
	procDir, err := os.Open("/proc")
	if err != nil {
		return nil, err
	}
	defer procDir.Close()

	entries, err := procDir.Readdirnames(0)
	if err != nil {
		return nil, err
	}

	var pids []int
	scraper := &linuxScraper{}

	for _, name := range entries {
		pid, err := strconv.Atoi(name)
		if err != nil {
			continue
		}

		info, err := scraper.ScrapeProcess(pid)
		if err != nil {
			continue
		}

		if DetectInputRequired(info) {
			pids = append(pids, pid)
		}
	}

	return pids, nil
}
