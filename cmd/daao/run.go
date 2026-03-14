//go:build windows

// Package main provides the daao CLI tool for running agent sessions.
package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/daao/nexus/internal/satellite"
	"github.com/daao/nexus/internal/session"
	"github.com/daao/nexus/pkg/proc"
	"github.com/daao/nexus/pkg/pty"
	"github.com/google/uuid"
	"golang.org/x/term"
)

// runCommand handles the daao run process for spawning agent sessions
func runCommand(ctx context.Context, args []string) error {
	// Check for --list flag first
	if len(args) > 0 && (args[0] == "--list" || args[0] == "-l") {
		return listSessions(ctx)
	}

	// Validate we have an agent command
	if len(args) < 1 {
		return fmt.Errorf("agent command required: daao run <agent-command> [args...]")
	}

	agentCommand := args[0]
	agentArgs := args[1:]

	log.Printf("Starting agent session with command: %s %v", agentCommand, agentArgs)

	// Ensure daemon is running
	if err := ensureDaemonRunning(ctx); err != nil {
		return fmt.Errorf("failed to ensure daemon is running: %w", err)
	}

	// Get satellite info
	satID, _, err := getSatelliteInfo()
	if err != nil {
		return fmt.Errorf("failed to get satellite info: %w", err)
	}

	// Create session in PROVISIONING state
	sessionID := uuid.New()
	now := time.Now().UTC()

	// Default terminal size
	cols, rows, err := term.GetSize(int(os.Stdin.Fd()))
	if err != nil {
		cols, rows = 80, 24
	}

	sess := &session.Session{
		ID:             sessionID,
		SatelliteID:    satID,
		UserID:         uuid.MustParse("00000000-0000-0000-0000-000000000001"), // Default user
		Name:           fmt.Sprintf("session-%d", now.Unix()),
		AgentBinary:    agentCommand,
		AgentArgs:      agentArgs,
		State:          session.SessionState(session.StateProvisioning),
		Cols:           int16(cols),
		Rows:           int16(rows),
		LastActivityAt: now,
		CreatedAt:      now,
	}

	log.Printf("Created session %s in PROVISIONING state", sessionID)

	// Allocate PTY
	ptyObj, err := pty.NewPty(uint16(cols), uint16(rows))
	if err != nil {
		sess.State = session.SessionState(session.StateTerminated)
		return fmt.Errorf("failed to allocate PTY: %w", err)
	}
	defer ptyObj.Close()

	log.Printf("PTY allocated for session %s", sessionID)

	// Transition to RUNNING state after PTY is ready
	sess.State = session.SessionState(session.StateRunning)
	now = time.Now().UTC()
	sess.StartedAt = &now

	log.Printf("Session %s transitioned to RUNNING state", sessionID)

	// Spawn the agent process with PTY using detach flags
	proc, err := spawnAgentWithPTY(agentCommand, agentArgs, ptyObj)
	if err != nil {
		sess.State = session.SessionState(session.StateTerminated)
		return fmt.Errorf("failed to spawn agent process: %w", err)
	}

	// Store the OS PID
	pid := proc.Pid
	sess.OSPID = &pid

	log.Printf("Agent process started with PID %d", pid)

	// Start terminal I/O forwarding
	var wg sync.WaitGroup
	wg.Add(2)

	// Forward terminal input to PTY
	go func() {
		defer wg.Done()
		_, err := io.Copy(ptyObj, os.Stdin)
		if err != nil {
			log.Printf("Error copying stdin to PTY: %v", err)
		}
	}()

	// Forward PTY output to terminal
	go func() {
		defer wg.Done()
		_, err := io.Copy(os.Stdout, ptyObj)
		if err != nil {
			log.Printf("Error copying PTY to stdout: %v", err)
		}
	}()

	// Wait for interrupt signal to handle graceful detach
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	// Wait for either process exit or signal
	done := make(chan struct{})
	go func() {
		proc.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Printf("Agent process exited")
	case <-sigCh:
		log.Printf("Received interrupt signal")
	}

	// Graceful detach on terminal close (Ctrl+D or terminal close)
	// Transition session to DETACHED state instead of terminating
	sess.State = session.SessionState(session.StateDetached)
	detachedAt := time.Now().UTC()
	sess.DetachedAt = &detachedAt

	log.Printf("Session %s transitioned to DETACHED state - agent keeps running", sessionID)

	// Wait for I/O to complete
	wg.Wait()

	log.Printf("Terminal I/O detached for session %s", sessionID)

	return nil
}

// ensureDaemonRunning checks if the satellite daemon is running and starts it if not
func ensureDaemonRunning(ctx context.Context) error {
	// Check if daemon process is already running
	// In production, this would check for a daemon socket or pid file
	daemonRunning, err := isDaemonRunning()
	if err != nil {
		log.Printf("Error checking daemon status: %v", err)
	}

	if daemonRunning {
		log.Println("Satellite daemon is already running")
		return nil
	}

	// Spawn the daemon if not running
	log.Println("Satellite daemon not running, spawning daemon...")

	daemonPath, err := findDaemonPath()
	if err != nil {
		return fmt.Errorf("failed to find daemon binary: %w", err)
	}

	// Start daemon process with detach flags
	detachFlags := proc.DetachFlags()
	procAttr := &os.ProcAttr{
		Dir:   "",
		Env:   os.Environ(),
		Files: []*os.File{os.Stdin, os.Stdout, os.Stderr},
		Sys: &syscall.SysProcAttr{
			CreationFlags: detachFlags,
		},
	}

	_, err = os.StartProcess(daemonPath, []string{daemonPath, "daemon"}, procAttr)
	if err != nil {
		return fmt.Errorf("failed to spawn daemon: %w", err)
	}

	// Wait for daemon to be ready
	time.Sleep(2 * time.Second)

	log.Println("Satellite daemon spawned successfully")
	return nil
}

// isDaemonRunning checks if the satellite daemon is currently running
func isDaemonRunning(ctx ...context.Context) (bool, error) {
	// Check for daemon socket or pid file
	// For now, check if a process named "daao" is running
	// In production, this would check a proper daemon socket
	daemonSocketPath := getDaemonSocketPath()
	if _, err := os.Stat(daemonSocketPath); err == nil {
		return true, nil
	}

	// Check if daemon process is running by checking process list
	// This is a simplified check - production would use proper daemon management
	proc, err := os.FindProcess(daemonPID())
	if err == nil && proc != nil {
		err = proc.Signal(syscall.Signal(0))
		if err == nil {
			return true, nil
		}
	}

	return false, nil
}

// spawnAgentWithPTY spawns an agent process attached to a PTY
func spawnAgentWithPTY(command string, args []string, ptyObj pty.Pty) (*os.Process, error) {
	// Get detach flags for the process
	detachFlags := proc.DetachFlags()

	// Determine the shell/binary to run
	binary := command
	if runtime.GOOS == "windows" {
		// On Windows, we may need to handle .exe vs other binaries
		if !strings.Contains(command, ".exe") && !strings.HasSuffix(command, ".exe") {
			binary = command + ".exe"
		}
	}

	// Build the process arguments
	procArgs := []string{binary}
	procArgs = append(procArgs, args...)

	// Create procAttr with the PTY as the controlling terminal
	// The PTY pipes will be used for stdin/stdout/stderr
	procAttr := &os.ProcAttr{
		Dir:   os.Getenv("USERPROFILE"),
		Env:   os.Environ(),
		Files: []*os.File{os.Stdin, os.Stdout, os.Stderr},
		Sys: &syscall.SysProcAttr{
			CreationFlags: detachFlags,
		},
	}

	// Start the process
	proc, err := os.StartProcess(binary, procArgs, procAttr)
	if err != nil {
		return nil, fmt.Errorf("failed to start process %s: %w", binary, err)
	}

	return proc, nil
}

// getSatelliteInfo returns the satellite ID and fingerprint
func getSatelliteInfo() (uuid.UUID, string, error) {
	// Try to load registration from registration.json
	reg := loadLocalRegistration()
	if reg != nil && isValidUUID(reg.ID) {
		id, err := uuid.Parse(reg.ID)
		if err != nil {
			return uuid.Nil, "", fmt.Errorf("failed to parse satellite ID from registration: %w", err)
		}

		// Load key pair for fingerprint
		publicKeyPath, privateKeyPath, err := satellite.GetDefaultKeyPaths()
		if err != nil {
			return uuid.Nil, "", fmt.Errorf("failed to get key paths: %w", err)
		}

		keyPair, err := satellite.LoadKeyPair(publicKeyPath, privateKeyPath)
		if err != nil {
			return uuid.Nil, "", fmt.Errorf("failed to load key pair: %w", err)
		}

		return id, keyPair.Fingerprint, nil
	}

	// No valid registration found
	return uuid.Nil, "", fmt.Errorf("satellite not registered; run 'daao login' first")
}

// listSessions lists active sessions
func listSessions(ctx context.Context) error {
	log.Println("Listing active sessions...")

	// In production, this would query the database or daemon for active sessions
	// For now, we just print a message
	fmt.Println("Active sessions:")
	fmt.Println("  (no active sessions)")

	return nil
}

// getDaemonSocketPath returns the path to the daemon socket
func getDaemonSocketPath() string {
	homeDir, _ := os.UserHomeDir()
	return fmt.Sprintf("%s/.config/daao/daemon.sock", homeDir)
}

// daemonPID returns the PID of the daemon process if known
func daemonPID() int {
	// In production, this would read from a pid file
	// For now, return 0 (no known PID)
	return 0
}

// findDaemonPath returns the path to the daemon binary
func findDaemonPath() (string, error) {
	// First check if daemon binary exists alongside daao binary
	exePath, err := os.Executable()
	if err != nil {
		return "", err
	}

	// Try to find daemon in the same directory
	daemonPath := strings.Replace(exePath, "daao.exe", "daao-daemon.exe", 1)
	daemonPath = strings.Replace(daemonPath, "daao", "daao-daemon", 1)

	if _, err := os.Stat(daemonPath); err == nil {
		return daemonPath, nil
	}

	// Try daao-daemon in PATH
	return "daao-daemon", nil
}
