//go:build windows

package main

import "github.com/daao/nexus/pkg/proc"

// newSuspendableProcess creates a platform-specific suspendable process wrapper.
func newSuspendableProcess(pid int) proc.SuspendableProcess {
	return proc.NewWindowsProcess(pid)
}
