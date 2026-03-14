//go:build windows

// Package main provides the daao CLI commands for local session management.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	"golang.org/x/term"
)

// sessionsCommand lists active sessions on the local satellite daemon.
func sessionsCommand(ctx context.Context, args []string) error {
	conn, err := controlDial(getDaemonSocketPath())
	if err != nil {
		return fmt.Errorf("cannot connect to daemon: %w\n\nIs the satellite daemon running? Start it with: daao start", err)
	}
	defer conn.Close()

	// Send list_sessions request
	req := ControlRequest{Method: "list_sessions"}
	reqBytes, _ := json.Marshal(req)
	if _, err := conn.Write(append(reqBytes, '\n')); err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}

	// Read response
	reader := bufio.NewReader(conn)
	line, err := reader.ReadBytes('\n')
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	var resp ControlResponse
	if err := json.Unmarshal(line, &resp); err != nil {
		return fmt.Errorf("invalid response: %w", err)
	}
	if !resp.OK {
		return fmt.Errorf("daemon error: %s", resp.Error)
	}

	var sessions []SessionInfo
	if err := json.Unmarshal(resp.Data, &sessions); err != nil {
		return fmt.Errorf("invalid session data: %w", err)
	}

	if len(sessions) == 0 {
		fmt.Print("\n  No active sessions on this satellite.\n\n")
		return nil
	}

	// Sort by state (RUNNING first) then by ID
	sort.Slice(sessions, func(i, j int) bool {
		if sessions[i].State != sessions[j].State {
			return stateOrder(sessions[i].State) < stateOrder(sessions[j].State)
		}
		return sessions[i].ID < sessions[j].ID
	})

	// Print table
	fmt.Println()
	fmt.Printf("  %-36s  %-12s  %-15s  %-8s  %s\n",
		"SESSION ID", "STATE", "AGENT", "PID", "BUFFER")
	fmt.Printf("  %-36s  %-12s  %-15s  %-8s  %s\n",
		strings.Repeat("─", 36), strings.Repeat("─", 12),
		strings.Repeat("─", 15), strings.Repeat("─", 8),
		strings.Repeat("─", 10))

	for _, s := range sessions {
		agent := s.AgentBinary
		if agent == "" {
			agent = "(unknown)"
		}
		// Truncate agent name for display
		if len(agent) > 15 {
			agent = agent[:12] + "..."
		}
		pidStr := fmt.Sprintf("%d", s.PID)
		if s.PID == 0 {
			pidStr = "-"
		}
		bufStr := formatBytes(s.BufferLen)
		stateIcon := stateIcon(s.State)

		fmt.Printf("  %-36s  %s %-10s  %-15s  %-8s  %s\n",
			s.ID, stateIcon, s.State, agent, pidStr, bufStr)
	}
	fmt.Println()

	// Hint
	fmt.Println("  Attach to a session:  daao attach <session-id>")
	fmt.Println()

	return nil
}

// attachCommand attaches the local terminal to a running session.
func attachCommand(ctx context.Context, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: daao attach <session-id>\n\nUse 'daao sessions' to list available sessions")
	}

	sessionID := args[0]

	conn, err := controlDial(getDaemonSocketPath())
	if err != nil {
		return fmt.Errorf("cannot connect to daemon: %w\n\nIs the satellite daemon running? Start it with: daao start", err)
	}
	defer conn.Close()

	// Send attach_session request
	params, _ := json.Marshal(AttachParams{SessionID: sessionID})
	req := ControlRequest{Method: "attach_session", Params: params}
	reqBytes, _ := json.Marshal(req)
	if _, err := conn.Write(append(reqBytes, '\n')); err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}

	// Read response
	reader := bufio.NewReader(conn)
	line, err := reader.ReadBytes('\n')
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	var resp ControlResponse
	if err := json.Unmarshal(line, &resp); err != nil {
		return fmt.Errorf("invalid response: %w", err)
	}
	if !resp.OK {
		return fmt.Errorf("attach failed: %s", resp.Error)
	}

	// Parse attach metadata
	var meta struct {
		SessionID    string `json:"session_id"`
		HydrationLen int    `json:"hydration_len"`
	}
	if err := json.Unmarshal(resp.Data, &meta); err != nil {
		return fmt.Errorf("invalid attach metadata: %w", err)
	}

	// Put terminal in raw mode
	fd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return fmt.Errorf("failed to set raw terminal mode: %w", err)
	}
	defer term.Restore(fd, oldState)

	// Print banner (before raw mode takes full effect)
	fmt.Fprintf(os.Stderr, "\r\n\033[1;36m⚡ Attached to session %s\033[0m\r\n", meta.SessionID)
	fmt.Fprintf(os.Stderr, "\033[90m   Press Ctrl+\\ to detach\033[0m\r\n\r\n")

	// Read hydration data (scrollback)
	if meta.HydrationLen > 0 {
		hydration := make([]byte, meta.HydrationLen)
		if _, err := io.ReadFull(reader, hydration); err != nil {
			log.Printf("Warning: partial hydration read: %v", err)
		} else {
			os.Stdout.Write(hydration)
		}
	}

	// Set up signal handling for clean detach
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	// Bidirectional I/O
	done := make(chan struct{})

	// PTY output → stdout
	go func() {
		defer close(done)
		buf := make([]byte, 4096)
		for {
			n, err := reader.Read(buf)
			if n > 0 {
				os.Stdout.Write(buf[:n])
			}
			if err != nil {
				return
			}
		}
	}()

	// stdin → PTY (in the control connection)
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := os.Stdin.Read(buf)
			if n > 0 {
				// Check for Ctrl+\ (0x1c) to detach
				for i := 0; i < n; i++ {
					if buf[i] == 0x1c { // SIGQUIT / Ctrl+backslash
						conn.Close()
						return
					}
				}
				if _, werr := conn.Write(buf[:n]); werr != nil {
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()

	// Wait for disconnect
	select {
	case <-done:
	case <-sigCh:
		// Clean exit on signal
	}

	// Restore terminal and print detach message
	term.Restore(fd, oldState)
	fmt.Fprintf(os.Stderr, "\r\n\033[1;33m⚡ Detached from session %s\033[0m\r\n", meta.SessionID)
	fmt.Fprintf(os.Stderr, "\033[90m   Session is still running. Use 'daao attach %s' to reconnect.\033[0m\r\n", meta.SessionID[:8])

	return nil
}

// stateOrder returns a sort key for session states.
func stateOrder(state string) int {
	switch state {
	case "RUNNING":
		return 0
	case "RE_ATTACHING":
		return 1
	case "DETACHED":
		return 2
	case "SUSPENDED":
		return 3
	case "PROVISIONING":
		return 4
	case "TERMINATED":
		return 5
	default:
		return 9
	}
}

// stateIcon returns a colored icon for the session state.
func stateIcon(state string) string {
	switch state {
	case "RUNNING":
		return "\033[32m●\033[0m" // green dot
	case "DETACHED":
		return "\033[33m◐\033[0m" // yellow half
	case "SUSPENDED":
		return "\033[90m◌\033[0m" // gray circle
	case "TERMINATED":
		return "\033[31m✕\033[0m" // red x
	default:
		return "\033[90m◌\033[0m"
	}
}

// formatBytes formats byte count for display.
func formatBytes(n int) string {
	switch {
	case n >= 1024*1024:
		return fmt.Sprintf("%.1f MB", float64(n)/(1024*1024))
	case n >= 1024:
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	default:
		return fmt.Sprintf("%d B", n)
	}
}

// addScheduledResize sends a resize command to the PTY when the terminal resizes.
// This is called during attach to keep the PTY dimensions in sync.
func addScheduledResize(conn *bufio.Writer, cols, rows int) {
	// For future use: send resize commands over the control socket
	_ = conn
	_ = cols
	_ = rows
}

// ensureRunningTime formats a duration since a given time.
func ensureRunningTime(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
	default:
		return fmt.Sprintf("%dd%dh", int(d.Hours())/24, int(d.Hours())%24)
	}
}
