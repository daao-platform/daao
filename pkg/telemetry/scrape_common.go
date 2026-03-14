//go:build linux || darwin || windows

package telemetry

// DetectInputRequired determines if a process is waiting for input
// based on its wait state and wait channel
func DetectInputRequired(info *ProcessInfo) bool {
	if info == nil {
		return false
	}

	// Direct state check
	if info.State == StateInputRequired {
		return true
	}

	// Check wait reason for input
	if info.WaitReason == WaitReasonUserInput {
		return true
	}

	// On Linux, check common terminal wait channels
	// These symbols indicate reading from terminal/PTY
	terminalWaitChannels := []string{
		"tty_read",
		"n_tty_read",
		"con_read",  // console read
		"pty_read",
		"read_chan",
		"file_read",
	}

	for _, ch := range terminalWaitChannels {
		if containsString(info.WaitChannel, ch) {
			return true
		}
	}

	return false
}

// containsString checks if a string contains a substring
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
