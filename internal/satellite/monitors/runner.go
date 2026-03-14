// Package monitors provides the embedded monitor script runner for the
// Infrastructure Discovery Agent. It detects the host OS, executes the
// appropriate platform-specific monitor scripts, and returns validated
// structured JSON output.
//
// Scripts are embedded in the binary via Go's embed package and executed
// as temporary files to avoid filesystem dependency.
package monitors

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

//go:embed scripts/linux/*.sh scripts/windows/*.ps1
var scriptsFS embed.FS

// SnapshotType identifies the kind of data a monitor script collects.
type SnapshotType string

const (
	SnapshotOSProfile         SnapshotType = "os_profile"
	SnapshotServiceCatalog    SnapshotType = "service_catalog"
	SnapshotNetworkTopology   SnapshotType = "network_topology"
	SnapshotHardwareInventory SnapshotType = "hardware_inventory"
)

// AllSnapshotTypes returns all available snapshot types in recommended execution order.
func AllSnapshotTypes() []SnapshotType {
	return []SnapshotType{
		SnapshotOSProfile,
		SnapshotServiceCatalog,
		SnapshotNetworkTopology,
		SnapshotHardwareInventory,
	}
}

// scriptMapping maps snapshot types to script filenames.
var scriptMapping = map[SnapshotType]string{
	SnapshotOSProfile:         "os-profile",
	SnapshotServiceCatalog:    "service-catalog",
	SnapshotNetworkTopology:   "network-topology",
	SnapshotHardwareInventory: "hardware-inventory",
}

// ScriptResult holds the parsed output from a monitor script execution.
type ScriptResult struct {
	SchemaVersion string          `json:"schema_version"`
	SnapshotType  string          `json:"snapshot_type"`
	OS            string          `json:"os"`
	CollectedAt   string          `json:"collected_at"`
	Status        string          `json:"status"` // "complete", "partial", "error"
	Data          json.RawMessage `json:"data"`
	Warnings      []string        `json:"warnings,omitempty"`

	// Metadata added by the runner (not from script output)
	Duration time.Duration `json:"duration_ms,omitempty"`
	Error    string        `json:"error,omitempty"`
}

// Runner executes embedded monitor scripts on the host system.
type Runner struct {
	// Timeout for individual script execution.
	ScriptTimeout time.Duration
}

// NewRunner creates a new monitor script runner with sensible defaults.
func NewRunner() *Runner {
	return &Runner{
		ScriptTimeout: 30 * time.Second,
	}
}

// DetectOS returns the current operating system family.
func DetectOS() string {
	switch runtime.GOOS {
	case "linux":
		// Check for Unraid
		if _, err := os.Stat("/boot/config/plugins"); err == nil {
			return "unraid"
		}
		return "linux"
	case "darwin":
		return "darwin"
	case "windows":
		return "windows"
	default:
		return runtime.GOOS
	}
}

// RunAll executes all monitor scripts and returns results for each.
func (r *Runner) RunAll(ctx context.Context) []ScriptResult {
	var results []ScriptResult
	for _, st := range AllSnapshotTypes() {
		result := r.Run(ctx, st)
		results = append(results, result)
	}
	return results
}

// Run executes a single monitor script and returns the parsed result.
func (r *Runner) Run(ctx context.Context, snapshotType SnapshotType) ScriptResult {
	start := time.Now()

	scriptName, ok := scriptMapping[snapshotType]
	if !ok {
		return ScriptResult{
			SnapshotType: string(snapshotType),
			Status:       "error",
			Error:        fmt.Sprintf("unknown snapshot type: %s", snapshotType),
			Duration:     time.Since(start),
		}
	}

	osFamily := DetectOS()
	result, err := r.executeScript(ctx, osFamily, scriptName)
	if err != nil {
		return ScriptResult{
			SnapshotType: string(snapshotType),
			OS:           osFamily,
			Status:       "error",
			Error:        err.Error(),
			Duration:     time.Since(start),
		}
	}

	result.Duration = time.Since(start)
	return result
}

// executeScript writes the embedded script to a temp file and executes it.
func (r *Runner) executeScript(ctx context.Context, osFamily, scriptName string) (ScriptResult, error) {
	// Determine script path and interpreter
	var scriptPath string
	var cmd *exec.Cmd

	switch osFamily {
	case "linux", "darwin", "unraid":
		scriptPath = fmt.Sprintf("scripts/linux/%s.sh", scriptName)
		scriptContent, err := scriptsFS.ReadFile(scriptPath)
		if err != nil {
			return ScriptResult{}, fmt.Errorf("script not found: %s: %w", scriptPath, err)
		}

		// Write to temp file
		tmpFile, err := os.CreateTemp("", "daao-monitor-*.sh")
		if err != nil {
			return ScriptResult{}, fmt.Errorf("create temp file: %w", err)
		}
		defer os.Remove(tmpFile.Name())

		if _, err := tmpFile.Write(scriptContent); err != nil {
			tmpFile.Close()
			return ScriptResult{}, fmt.Errorf("write script: %w", err)
		}
		tmpFile.Close()

		if err := os.Chmod(tmpFile.Name(), 0700); err != nil {
			return ScriptResult{}, fmt.Errorf("chmod: %w", err)
		}

		ctx, cancel := context.WithTimeout(ctx, r.ScriptTimeout)
		defer cancel()
		cmd = exec.CommandContext(ctx, "sh", tmpFile.Name())

	case "windows":
		scriptPath = fmt.Sprintf("scripts/windows/%s.ps1", scriptName)
		scriptContent, err := scriptsFS.ReadFile(scriptPath)
		if err != nil {
			return ScriptResult{}, fmt.Errorf("script not found: %s: %w", scriptPath, err)
		}

		// Write to temp file
		tmpFile, err := os.CreateTemp("", "daao-monitor-*.ps1")
		if err != nil {
			return ScriptResult{}, fmt.Errorf("create temp file: %w", err)
		}
		defer os.Remove(tmpFile.Name())

		if _, err := tmpFile.Write(scriptContent); err != nil {
			tmpFile.Close()
			return ScriptResult{}, fmt.Errorf("write script: %w", err)
		}
		tmpFile.Close()

		ctx, cancel := context.WithTimeout(ctx, r.ScriptTimeout)
		defer cancel()
		cmd = exec.CommandContext(ctx, "powershell", "-ExecutionPolicy", "Bypass", "-File", tmpFile.Name())

	default:
		return ScriptResult{}, fmt.Errorf("unsupported OS: %s", osFamily)
	}

	// Execute and capture output
	output, err := cmd.Output()
	if err != nil {
		// Try to parse stderr for better error messages
		if exitErr, ok := err.(*exec.ExitError); ok {
			return ScriptResult{}, fmt.Errorf("script failed (exit %d): %s", exitErr.ExitCode(), strings.TrimSpace(string(exitErr.Stderr)))
		}
		return ScriptResult{}, fmt.Errorf("execute script: %w", err)
	}

	// Parse JSON output
	var result ScriptResult
	if err := json.Unmarshal(output, &result); err != nil {
		return ScriptResult{}, fmt.Errorf("parse script output: %w (raw: %s)", err, truncate(string(output), 200))
	}

	return result, nil
}

// DiscoveryReport generates a comprehensive markdown discovery report
// from a set of snapshot results, suitable for writing as a context file.
func DiscoveryReport(results []ScriptResult) string {
	var sb strings.Builder

	sb.WriteString("# Infrastructure Discovery Report\n\n")
	sb.WriteString(fmt.Sprintf("**Generated**: %s\n", time.Now().UTC().Format(time.RFC3339)))
	sb.WriteString("<!-- DAAO Discovery Agent: Auto-generated -->\n\n")

	// Summary
	complete := 0
	errors := 0
	for _, r := range results {
		if r.Status == "complete" {
			complete++
		} else {
			errors++
		}
	}
	sb.WriteString(fmt.Sprintf("**Snapshots**: %d complete, %d errors\n\n", complete, errors))
	sb.WriteString("---\n\n")

	// Each snapshot section
	for _, r := range results {
		title := strings.ReplaceAll(r.SnapshotType, "_", " ")
		title = strings.Title(title)
		sb.WriteString(fmt.Sprintf("## %s\n\n", title))
		sb.WriteString(fmt.Sprintf("- **OS**: %s\n", r.OS))
		sb.WriteString(fmt.Sprintf("- **Status**: %s\n", r.Status))
		sb.WriteString(fmt.Sprintf("- **Collected**: %s\n", r.CollectedAt))
		sb.WriteString(fmt.Sprintf("- **Duration**: %s\n", r.Duration))

		if r.Error != "" {
			sb.WriteString(fmt.Sprintf("- **Error**: %s\n", r.Error))
		}

		if len(r.Data) > 0 && string(r.Data) != "{}" && string(r.Data) != "null" {
			sb.WriteString("\n```json\n")
			// Pretty-print the data
			var pretty json.RawMessage
			if err := json.Unmarshal(r.Data, &pretty); err == nil {
				formatted, err := json.MarshalIndent(pretty, "", "  ")
				if err == nil {
					sb.Write(formatted)
				} else {
					sb.Write(r.Data)
				}
			} else {
				sb.Write(r.Data)
			}
			sb.WriteString("\n```\n")
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// truncate shortens a string to maxLen, adding "..." if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// TempScriptDir returns the directory used for temporary script files.
// Useful for cleanup in tests.
func TempScriptDir() string {
	return filepath.Join(os.TempDir(), "daao-monitors")
}
