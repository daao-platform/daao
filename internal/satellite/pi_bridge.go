// Package satellite provides satellite runtime configuration and Pi RPC bridge functionality.
package satellite

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/daao/nexus/proto"
	goproto "google.golang.org/protobuf/proto"
)

// PiBridge manages a Pi process running in RPC mode.
type PiBridge struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	stdout  io.ReadCloser
	events  chan AgentEvent
	timeout *time.Timer

	mu       sync.Mutex
	done     chan struct{} // closed when the process exits (cmd.Wait returns)
	closed   bool
	exitErr  error // exit error from cmd.Wait
	exitCode int   // process exit code (-1 if unknown)
}

// AgentEvent represents an event from the Pi agent
type AgentEvent struct {
	SessionID string
	EventType string
	Payload   []byte
	Timestamp int64
	Provider  string
	Model     string
}

// PiEvent types emitted by pi's RPC mode.
const (
	EventAgentStart          = "agent_start"
	EventAgentEnd            = "agent_end"
	EventMessageStart        = "message_start"
	EventMessageEnd          = "message_end"
	EventMessageUpdate       = "message_update"
	EventToolExecutionUpdate = "tool_execution_update"
	// Legacy aliases kept for compatibility
	EventTurnStart          = "turn_start"
	EventTurnEnd            = "turn_end"
	EventToolExecutionStart = "tool_execution_start"
	EventToolExecutionEnd   = "tool_execution_end"
)

// Valid event types — all events forwarded to Nexus.
var validEventTypes = map[string]bool{
	EventAgentStart:          true,
	EventAgentEnd:            true,
	EventMessageStart:        true,
	EventMessageEnd:          true,
	EventMessageUpdate:       true,
	EventToolExecutionUpdate: true,
	// Legacy
	EventTurnStart:          true,
	EventTurnEnd:            true,
	EventToolExecutionStart: true,
	EventToolExecutionEnd:   true,
}

// piEventPayload represents the JSON payload from Pi RPC
type piEventPayload struct {
	Type      string          `json:"type"`
	Timestamp int64           `json:"timestamp"`
	Provider  string          `json:"provider,omitempty"`
	Model     string          `json:"model,omitempty"`
	Message   json.RawMessage `json:"message,omitempty"`
	Turn      json.RawMessage `json:"turn,omitempty"`
	Tool      json.RawMessage `json:"tool,omitempty"`
	Error     json.RawMessage `json:"error,omitempty"`
}

// piMessage represents a message in the RPC protocol.
// pi's RPC protocol uses "message" (not "prompt") as the prompt field name.
type piMessage struct {
	Type    string `json:"type"`
	Message string `json:"message,omitempty"`
	Abort   bool   `json:"abort,omitempty"`
}

// NewPiBridge creates a new PiBridge instance
func NewPiBridge() *PiBridge {
	return &PiBridge{
		events:   make(chan AgentEvent, 100),
		done:     make(chan struct{}),
		exitCode: -1,
	}
}

// promptVars holds the runtime values injected into system prompt templates.
type promptVars struct {
	GOOS        string
	GOARCH      string
	CONTEXT_DIR string
	TEMP_DIR    string
}

// ExpandSystemPrompt replaces Go text/template variables in a system prompt
// with runtime values. If the prompt contains no template markers ({{ }}),
// it is returned unchanged. If template parsing or execution fails, the raw
// prompt is returned with a log warning.
// Exported for testing.
func ExpandSystemPrompt(raw string) string {
	// Fast path: no template markers → return as-is
	if !strings.Contains(raw, "{{") {
		return raw
	}

	vars := promptVars{
		GOOS:        runtime.GOOS,
		GOARCH:      runtime.GOARCH,
		CONTEXT_DIR: ContextDir(),
		TEMP_DIR:    os.TempDir(),
	}

	tmpl, err := template.New("sysprompt").Parse(raw)
	if err != nil {
		slog.Error(fmt.Sprintf("ExpandSystemPrompt: template parse error (using raw prompt): %v", err), "component", "satellite")
		return raw
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, vars); err != nil {
		slog.Error(fmt.Sprintf("ExpandSystemPrompt: template exec error (using raw prompt): %v", err), "component", "satellite")
		return raw
	}

	result := buf.String()
	slog.Info("template expanded", "component", "satellite", "GOOS", vars.GOOS, "GOARCH", vars.GOARCH, "CONTEXT_DIR", vars.CONTEXT_DIR)
	return result
}

// AllowedWriteDirs returns the directories that agents are permitted to write to.
// Currently: the DAAO context directory and the OS temp directory.
func AllowedWriteDirs() []string {
	return []string{
		ContextDir(),
		os.TempDir(),
	}
}

// ValidateWritePath checks whether targetPath is inside one of the allowed
// write directories. Returns nil if allowed, or an error describing the
// violation. Uses filepath.Abs + filepath.Clean to resolve symlinks and
// relative paths before comparison.
// Exported for use by guardrails, context watcher, and tests.
func ValidateWritePath(targetPath string) error {
	cleaned, err := filepath.Abs(filepath.Clean(targetPath))
	if err != nil {
		return fmt.Errorf("SECURITY DENIED: cannot resolve path %q: %w", targetPath, err)
	}

	for _, dir := range AllowedWriteDirs() {
		allowedAbs, err := filepath.Abs(filepath.Clean(dir))
		if err != nil {
			continue
		}
		// Append separator to prevent prefix false positives
		// (e.g., /etc/daao/context-evil matching /etc/daao/context)
		if strings.HasPrefix(cleaned, allowedAbs+string(filepath.Separator)) || cleaned == allowedAbs {
			return nil
		}
	}

	return fmt.Errorf("SECURITY DENIED: write to %q blocked — only allowed in %v", targetPath, AllowedWriteDirs())
}

// BuildPiArgs constructs the Pi CLI arguments from an agent definition.
// It builds base args (pi --mode rpc --provider X --model Y) and appends
// guardrails flags if guardrails config is present in agentDef.Config.
//
// NOTE: The system prompt is NOT passed as a CLI argument. It is injected
// via stdin alongside the initial prompt to avoid shell escaping issues,
// Windows command-line length limits, and problems with multiline strings.
// Exported for testing.
func BuildPiArgs(agentDef *proto.AgentDefinitionProto) []string {
	// Extract provider and model from config
	provider := agentDef.Config["provider"]
	model := agentDef.Config["model"]

	// Build base command arguments — no leading "pi" since we exec the binary directly.
	// --no-skills and --no-prompt-templates prevent pi's own stored context from
	// overriding the agent's system prompt with unrelated project files.
	args := []string{
		"--mode", "rpc",
		"--no-session",
		"--no-skills",
		"--no-prompt-templates",
	}

	// Add provider/model if set
	if provider != "" {
		args = append(args, "--provider", provider)
	}
	if model != "" {
		args = append(args, "--model", model)
	}

	// System prompt is injected via stdin, NOT as a CLI arg.
	// This avoids Windows command-line length limits and shell escaping issues
	// with multiline prompts containing special characters.

	// Add tools config if specified (Pi native --tools flag)
	if toolsAllow, ok := agentDef.Config["tools.allow"]; ok && toolsAllow != "" {
		args = append(args, "--tools", toolsAllow)
	}

	// Check if any guardrails config keys exist
	hasGuardrailsConfig := false
	for key := range agentDef.Config {
		if strings.HasPrefix(key, "guardrails.") {
			hasGuardrailsConfig = true
			break
		}
	}

	// If guardrails config exists, add the extension
	if hasGuardrailsConfig {
		extDir := ExtensionsDir()
		guardrailsPath := filepath.Join(extDir, "daao-guardrails")
		if _, err := os.Stat(guardrailsPath); err == nil {
			args = append(args, "--extension", guardrailsPath)
		}
	}

	if readOnly, ok := agentDef.Config["guardrails.read_only"]; ok && readOnly == "true" {
		args = append(args, "--read-only")
	}

	if maxToolCalls, ok := agentDef.Config["guardrails.max_tool_calls"]; ok && maxToolCalls != "" {
		if calls, err := strconv.Atoi(maxToolCalls); err == nil && calls > 0 {
			args = append(args, "--max-tool-calls", maxToolCalls)
		}
	}

	// Handle guardrails.hitl (Human-In-The-Loop) flag
	if hitl, ok := agentDef.Config["guardrails.hitl"]; ok && hitl == "true" {
		args = append(args, "--guardrails")
	}

	// Path jailing: Pi doesn't have a native --allowed-dirs flag. Write restrictions
	// are enforced by three layers:
	// 1. cmd.Dir is set to ContextDir() in Start() — limits relative path resolution
	// 2. The system prompt instructs agents to only write to the context and temp dirs
	// 3. The daao-guardrails Pi extension intercepts tool_call events for "write"
	//    and blocks paths outside DAAO_ALLOWED_DIRS (see extensions/daao-guardrails/)
	// DAAO_ALLOWED_DIRS is injected as an env var in Start() for the extension to read.

	// Add system prompt via --system-prompt flag if present.
	// This is the reliable way to set the system prompt for Pi RPC mode.
	// Template variables ({{.GOOS}}, {{.GOARCH}}, etc.) are expanded at deploy-time.
	if sysPrompt, ok := agentDef.Config["system_prompt"]; ok && sysPrompt != "" {
		args = append(args, "--system-prompt", ExpandSystemPrompt(sysPrompt))
	}

	return args
}

// resolvePiCommand builds the exec.Cmd for running Pi.
// On Windows, .cmd and .bat wrappers run through cmd.exe, which can corrupt
// stdio pipes. If piPath is a .cmd/.bat wrapper that invokes node.js, we read
// the wrapper to find the actual node binary and script, then exec node directly.
// Also handles .ps1 files by redirecting to the corresponding .cmd wrapper.
func resolvePiCommand(piPath string, args []string) *exec.Cmd {
	ext := strings.ToLower(filepath.Ext(piPath))

	// PowerShell's Get-Command may return pi.ps1 — redirect to pi.cmd
	if ext == ".ps1" {
		cmdPath := strings.TrimSuffix(piPath, filepath.Ext(piPath)) + ".cmd"
		if _, err := os.Stat(cmdPath); err == nil {
			slog.Info(fmt.Sprintf("PiBridge: redirecting .ps1 → .cmd: %s", cmdPath), "component", "satellite")
			piPath = cmdPath
			ext = ".cmd"
		} else {
			// Try the extensionless unix shim (npm also creates these)
			noExt := strings.TrimSuffix(piPath, filepath.Ext(piPath))
			if _, err := os.Stat(noExt); err == nil {
				slog.Info(fmt.Sprintf("PiBridge: redirecting .ps1 → extensionless shim: %s", noExt), "component", "satellite")
				piPath = noExt
				ext = ""
			}
		}
	}

	if ext == ".cmd" || ext == ".bat" {
		if node, script, ok := extractNodeFromCmdWrapper(piPath); ok {
			slog.Info(fmt.Sprintf("PiBridge: unwrapped %s → node %s", piPath, script), "component", "satellite")
			return exec.Command(node, append([]string{script}, args...)...)
		}
	}
	return exec.Command(piPath, args...)
}

// extractNodeFromCmdWrapper parses a standard npm-generated .cmd wrapper and
// returns the node binary path and the JS entry-point script path.
// Returns ok=false if the wrapper doesn't match the expected pattern.
func extractNodeFromCmdWrapper(cmdPath string) (nodeBin, jsScript string, ok bool) {
	data, err := os.ReadFile(cmdPath)
	if err != nil {
		return "", "", false
	}
	content := string(data)
	// npm wrappers end with a line like:
	//   "%_prog%"  "C:\path\to\script.js" %*
	// or:
	//   "%dp0%\node.exe"  "%dp0%\node_modules\...\cli.js" %*
	dir := filepath.Dir(cmdPath)
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if !strings.Contains(line, ".js") {
			continue
		}
		// Extract quoted paths
		var paths []string
		rest := line
		for {
			start := strings.Index(rest, "\"")
			if start < 0 {
				break
			}
			end := strings.Index(rest[start+1:], "\"")
			if end < 0 {
				break
			}
			p := rest[start+1 : start+1+end]
			// Expand %dp0% → wrapper directory
			p = strings.ReplaceAll(p, "%dp0%\\", dir+"\\")
			p = strings.ReplaceAll(p, "%dp0%/", dir+"/")
			paths = append(paths, p)
			rest = rest[start+1+end+1:]
		}
		if len(paths) >= 2 && strings.HasSuffix(strings.ToLower(paths[1]), ".js") {
			// paths[0] = node binary (possibly %_prog% which resolves to node)
			node := paths[0]
			if strings.Contains(strings.ToLower(node), "_prog") || node == "" {
				// Bare "node" won't work under Windows Service (LocalSystem has no user PATH).
				// Resolve to full absolute path so it works in any execution context.
				if resolved, err := exec.LookPath("node"); err == nil {
					node = resolved
				} else {
					// Fallback: check well-known Node install locations on Windows
					for _, candidate := range []string{
						filepath.Join(os.Getenv("ProgramFiles"), "nodejs", "node.exe"),
						filepath.Join(os.Getenv("ProgramFiles(x86)"), "nodejs", "node.exe"),
						filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "nodejs", "node.exe"),
						filepath.Join(os.Getenv("APPDATA"), "nvm", "current", "node.exe"),
					} {
						if candidate != "" {
							if _, statErr := os.Stat(candidate); statErr == nil {
								node = candidate
								break
							}
						}
					}
					if node == "" || strings.Contains(strings.ToLower(node), "_prog") {
						node = "node" // last resort — caller will get a clear exec error
					}
				}
				slog.Info(fmt.Sprintf("PiBridge: resolved node binary → %s", node), "component", "satellite")
			}
			return node, paths[1], true
		}
	}
	return "", "", false
}

// Start launches the Pi process in RPC mode with the given agent definition and secrets.
// The agent definition should contain provider and model information in its config.
// Secrets are injected as environment variables, not written to files.
func (p *PiBridge) Start(agentDef *proto.AgentDefinitionProto, secrets map[string]string) error {
	p.mu.Lock()
	if p.cmd != nil {
		p.mu.Unlock()
		return errors.New("PiBridge already started")
	}
	p.mu.Unlock()

	// Find the Pi binary — prefer DAAO_PI_PATH env, then PATH, then DAAO runtime dir
	piPath := os.Getenv("DAAO_PI_PATH")
	if piPath == "" {
		var err error
		piPath, err = exec.LookPath("pi")
		if err != nil {
			// Check DAAO private runtime directory (bootstrapped Node.js + Pi)
			runtimePi := PiBinaryPath()
			if _, statErr := os.Stat(runtimePi); statErr == nil {
				piPath = runtimePi
			} else {
				return fmt.Errorf("pi binary not found in PATH or DAAO runtime (%s) — install from pi.dev, set DAAO_PI_PATH, or bootstrap runtime: %w", runtimePi, err)
			}
		}
	}
	slog.Info(fmt.Sprintf("PiBridge: using Pi binary at %s", piPath), "component", "satellite")

	// Build command arguments using BuildPiArgs
	args := BuildPiArgs(agentDef) // agentDef is already a pointer
	slog.Info(fmt.Sprintf("PiBridge: args = %v", args), "component", "satellite")

	// Create the command.
	// On Windows, .cmd/.bat wrappers redirect stdio in ways that break our pipes.
	// Resolve the wrapper to the underlying node + script and invoke directly.
	cmd := resolvePiCommand(piPath, args)

	// Set working directory to the DAAO context directory.
	// This ensures agents start in a safe location and relative paths resolve
	// within the context directory rather than wherever the daemon was launched.
	contextDir := ContextDir()
	if err := os.MkdirAll(contextDir, 0755); err != nil {
		slog.Warn(fmt.Sprintf("PiBridge: warning: could not create context dir %s: %v", contextDir, err), "component", "satellite")
	}
	cmd.Dir = contextDir
	slog.Info(fmt.Sprintf("PiBridge: working directory set to %s", contextDir), "component", "satellite")

	// Build environment - start with existing environment
	env := os.Environ()

	// Prepend the DAAO Node.js bin dir to PATH so Pi CLI's #!/usr/bin/env node
	// shebang resolves correctly (especially under launchd which has minimal PATH).
	cfg := GetPlatformConfig()
	nodeBinDir := filepath.Dir(cfg.NodeBinary)
	for i, e := range env {
		if strings.HasPrefix(e, "PATH=") {
			env[i] = fmt.Sprintf("PATH=%s%c%s", nodeBinDir, os.PathListSeparator, e[5:])
			break
		}
	}

	// Inject DAAO-specific env vars for the guardrails extension.
	// The daao-guardrails extension reads DAAO_ALLOWED_DIRS to enforce
	// write path restrictions via Pi's tool_call event blocking.
	env = append(env, fmt.Sprintf("DAAO_CONTEXT_DIR=%s", contextDir))
	env = append(env, fmt.Sprintf("DAAO_ALLOWED_DIRS=%s", strings.Join(AllowedWriteDirs(), string(filepath.ListSeparator))))

	// Add secrets as environment variables
	for key, value := range secrets {
		env = append(env, fmt.Sprintf("%s=%s", key, value))
	}

	// Also add common API keys if present with different naming conventions
	for key, value := range secrets {
		upperKey := strings.ToUpper(key)
		if !strings.HasPrefix(upperKey, "ANTHROPIC") && !strings.HasPrefix(upperKey, "OPENAI") &&
			!strings.HasPrefix(upperKey, "GOOGLE") && !strings.HasPrefix(upperKey, "AZURE") {
			// Map common secret names to expected env vars
			switch upperKey {
			case "API_KEY", "ANTHROPIC_API_KEY":
				env = append(env, fmt.Sprintf("ANTHROPIC_API_KEY=%s", value))
			case "OPENAI_API_KEY":
				env = append(env, fmt.Sprintf("OPENAI_API_KEY=%s", value))
			}
		}
	}

	cmd.Env = env

	// Set up stdio pipes
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	// Pi emits all RPC JSON events on stdout.
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		stdin.Close()
		stdout.Close()
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// Start the process
	if err := cmd.Start(); err != nil {
		stdin.Close()
		stdout.Close()
		stderr.Close()
		return fmt.Errorf("failed to start Pi process (pid would not start): %w", err)
	}

	slog.Info(fmt.Sprintf("PiBridge: Pi process started (PID: %d)", cmd.Process.Pid), "component", "satellite")

	// Store references
	p.mu.Lock()
	p.cmd = cmd
	p.stdin = stdin
	p.stdout = stdout // RPC events come on stdout
	p.mu.Unlock()

	// WaitGroup to track stdout and stderr reader goroutines.
	// The process lifecycle goroutine waits for these before closing done.
	var ioWg sync.WaitGroup

	// Drain stderr to log — captures startup errors and warnings.
	ioWg.Add(1)
	go func() {
		defer ioWg.Done()
		scanner := bufio.NewScanner(stderr)
		scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024) // 10MB for large stack traces
		for scanner.Scan() {
			slog.Info(fmt.Sprintf("PiBridge stderr: %s", scanner.Text()), "component", "satellite")
		}
		if err := scanner.Err(); err != nil {
			slog.Error(fmt.Sprintf("PiBridge: stderr scanner error: %v", err), "component", "satellite")
		}
	}()

	// Launch goroutine to read RPC events line-by-line from stdout.
	ioWg.Add(1)
	go func() {
		defer ioWg.Done()
		p.readStdoutLoop()
	}()

	// Process lifecycle goroutine: waits for the process to exit, then
	// ensures all IO is drained before signaling completion via done channel.
	go func() {
		// Wait for the process to exit.
		waitErr := cmd.Wait()

		// Log the exit status.
		p.mu.Lock()
		p.exitErr = waitErr
		if cmd.ProcessState != nil {
			p.exitCode = cmd.ProcessState.ExitCode()
		}
		p.mu.Unlock()

		if waitErr != nil {
			slog.Error(fmt.Sprintf("PiBridge: Pi process exited with error: %v (exit code: %d)", waitErr, p.exitCode), "component", "satellite")
		} else {
			slog.Info(fmt.Sprintf("PiBridge: Pi process exited cleanly (exit code: 0)"), "component", "satellite")
		}

		// Wait for stdout and stderr goroutines to finish draining.
		// This ensures all buffered events are pushed to the events channel
		// before we close done.
		ioWg.Wait()

		// Now safe to close the done channel — all events have been read.
		p.mu.Lock()
		if !p.closed {
			close(p.done)
			p.closed = true
		}
		p.mu.Unlock()

		// Close the events channel so the consumer knows no more events are coming.
		close(p.events)
	}()

	// Set up timeout enforcement if configured
	if timeoutMinutes, ok := agentDef.Config["guardrails.timeout_minutes"]; ok {
		if minutes, err := strconv.Atoi(timeoutMinutes); err == nil && minutes > 0 {
			p.timeout = time.AfterFunc(time.Duration(minutes)*time.Minute, func() {
				slog.Info(fmt.Sprintf("PiBridge: timeout reached (%d min), stopping", minutes), "component", "satellite")
				p.Stop()
			})
		}
	}

	return nil
}

// readStdoutLoop continuously reads stdout and parses JSON events.
// It runs until stdout is closed (process exit or pipe close).
// Events are pushed to p.events; the lifecycle goroutine in Start() handles
// closing the done channel and events channel after this returns.
func (p *PiBridge) readStdoutLoop() {
	scanner := bufio.NewScanner(p.stdout)
	// Increase buffer for potentially large JSON payloads (e.g. tool results)
	// 10MB max line length — Pi can emit very large JSON payloads for tool results
	// (file reads, chain-debug arrays). Default 64KB limit would silently kill the loop.
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)

	lineCount := 0
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		lineCount++

		// Parse just enough to identify the event type — we use a minimal
		// struct so that all original fields are preserved in the raw JSON.
		var event piEventPayload
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			slog.Info(fmt.Sprintf("PiBridge stdout (non-JSON, line %d): %.200s", lineCount, line), "component", "satellite")
			continue
		}

		// Check if it's a valid event type
		if !validEventTypes[event.Type] {
			slog.Info(fmt.Sprintf("PiBridge: unrecognised event type %q (line: %.120s)", event.Type, line), "component", "satellite")
			continue
		}

		// Use the RAW JSON line as the payload — do NOT re-marshal through
		// piEventPayload. Pi's events have many top-level fields (toolCallId,
		// toolName, args, etc.) that the struct doesn't capture. Re-marshaling
		// would silently drop them.
		payload := []byte(line)

		// Create AgentEvent
		agentEvent := AgentEvent{
			EventType: event.Type,
			Timestamp: event.Timestamp,
			Provider:  event.Provider,
			Model:     event.Model,
			Payload:   payload,
		}

		// Send to channel — don't block on done here since the lifecycle
		// goroutine will close done AFTER this function returns.
		select {
		case p.events <- agentEvent:
		default:
			slog.Info(fmt.Sprintf("PiBridge: events channel full, dropping event type=%s", event.Type), "component", "satellite")
		}
	}

	// Log scanner completion
	if err := scanner.Err(); err != nil {
		slog.Error(fmt.Sprintf("PiBridge: stdout scanner error: %v", err), "component", "satellite")
	}
	slog.Info(fmt.Sprintf("PiBridge: stdout scanner finished after %d lines", lineCount), "component", "satellite")
}

// SendPrompt sends a prompt to the Pi process via stdin
func (p *PiBridge) SendPrompt(prompt string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.stdin == nil {
		return errors.New("PiBridge not started")
	}

	// Create the message
	msg := piMessage{
		Type:    "prompt",
		Message: prompt,
	}

	// Marshal to JSON
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal prompt: %w", err)
	}

	// Write to stdin with newline
	_, err = p.stdin.Write(append(data, '\n'))
	if err != nil {
		return fmt.Errorf("failed to write prompt: %w", err)
	}

	return nil
}

// Events returns the channel of agent events.
// The channel is closed when the Pi process exits and all events are drained.
func (p *PiBridge) Events() <-chan AgentEvent {
	return p.events
}

// Stop stops the Pi process gracefully by sending abort, waiting, then killing if needed
func (p *PiBridge) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.cmd == nil {
		return nil
	}

	// Check if already closed
	if p.closed {
		return nil
	}

	// Send abort message via stdin
	if p.stdin != nil {
		msg := piMessage{
			Type:  "abort",
			Abort: true,
		}
		data, _ := json.Marshal(msg)
		p.stdin.Write(append(data, '\n'))
		p.stdin.Close()
		p.stdin = nil
	}

	// Wait for the process lifecycle goroutine to signal completion.
	// It handles cmd.Wait(), IO draining, and closing done/events.
	p.mu.Unlock()
	select {
	case <-p.done:
		p.mu.Lock()
		p.cleanup()
		return p.exitErr
	case <-time.After(10 * time.Second):
		p.mu.Lock()
		// Timeout - kill the process
		if p.cmd != nil && p.cmd.Process != nil {
			slog.Info(fmt.Sprintf("PiBridge: killing Pi process (PID: %d) after timeout", p.cmd.Process.Pid), "component", "satellite")
			p.cmd.Process.Kill()
		}
		// Wait for done again after kill — the lifecycle goroutine will fire
		p.mu.Unlock()
		select {
		case <-p.done:
		case <-time.After(5 * time.Second):
			slog.Info(fmt.Sprintf("PiBridge: process did not exit after kill, force cleanup"), "component", "satellite")
		}
		p.mu.Lock()
		p.cleanup()
		return errors.New("process killed after timeout")
	}
}

// cleanup cleans up resources.
// Must be called with p.mu held.
func (p *PiBridge) cleanup() {
	// Stop the timeout timer if it was set
	if p.timeout != nil {
		p.timeout.Stop()
		p.timeout = nil
	}
	if p.stdout != nil {
		p.stdout.Close()
		p.stdout = nil
	}
	if !p.closed {
		close(p.done)
		p.closed = true
	}
	p.cmd = nil
	p.stdin = nil
}

// IsRunning returns true if the Pi process is running
func (p *PiBridge) IsRunning() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.cmd != nil && p.cmd.Process != nil && !p.closed
}

// Done returns a channel that is closed when the Pi process exits.
func (p *PiBridge) Done() <-chan struct{} {
	return p.done
}

// ExitCode returns the process exit code (-1 if not yet exited or unknown).
func (p *PiBridge) ExitCode() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.exitCode
}

// GetAgentEventProto converts an AgentEvent to the protobuf AgentEvent
func GetAgentEventProto(sessionID string, event AgentEvent) *proto.AgentEvent {
	return &proto.AgentEvent{
		SessionId: sessionID,
		EventType: event.EventType,
		Payload:   event.Payload,
		Timestamp: event.Timestamp,
	}
}

// MarshalAgentEvent marshals an AgentEvent to protobuf bytes
func MarshalAgentEvent(sessionID string, event AgentEvent) ([]byte, error) {
	protoEvent := GetAgentEventProto(sessionID, event)
	return goproto.Marshal(protoEvent)
}
