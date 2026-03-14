package satellite

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ============================================================================
// Agent Run Benchmarking & Validation Framework
//
// This framework provides:
// 1. AgentRunMetrics — structured metrics from an agent session
// 2. AgentRunValidator — validates metrics against pass/fail thresholds
// 3. ContextFileValidator — validates output context files for completeness
// 4. BenchmarkReport — human-readable pass/fail summary
//
// Usage:
//   go test ./internal/satellite/... -run TestValidateAgentRun -v
//
// To benchmark a real agent run:
//   1. Deploy the discovery agent via Cockpit
//   2. Export the session stats from the DAAO events log
//   3. Point DAAO_BENCH_METRICS_FILE to the exported JSON
//   4. Point DAAO_BENCH_CONTEXT_DIR to the satellite's context directory
//   5. Run: go test ./internal/satellite/... -run TestBenchmarkLiveRun -v
// ============================================================================

// AgentRunMetrics captures the key performance indicators from an agent session.
// Populated from Pi RPC session_stats and DAAO event stream.
type AgentRunMetrics struct {
	// Identity
	AgentSlug   string `json:"agent_slug"`
	SatelliteID string `json:"satellite_id"`
	SessionID   string `json:"session_id"`
	RunDate     string `json:"run_date"`

	// Performance
	TurnCount       int     `json:"turn_count"`
	ToolCallCount   int     `json:"tool_call_count"`
	TotalTokens     int     `json:"total_tokens"`
	InputTokens     int     `json:"input_tokens"`
	OutputTokens    int     `json:"output_tokens"`
	ExecutionTimeMs int64   `json:"execution_time_ms"`
	CostUSD         float64 `json:"cost_usd"`

	// Quality indicators (set by validation)
	ContextFilesWritten  []string `json:"context_files_written"`
	HasYAMLFrontmatter   bool     `json:"has_yaml_frontmatter"`
	HasGPUInfo           bool     `json:"has_gpu_info"`
	HasDiskUtilization   bool     `json:"has_disk_utilization"`
	HasMachineClasses    bool     `json:"has_machine_classes"`
	HasRecommendedAgents bool     `json:"has_recommended_agents"`
	HasDriftDetection    bool     `json:"has_drift_detection"`
}

// Thresholds defines pass/fail criteria for agent benchmarking.
type Thresholds struct {
	MaxTurns       int     `json:"max_turns"`
	MaxTokens      int     `json:"max_tokens"`
	MaxToolCalls   int     `json:"max_tool_calls"`
	MaxTimeSeconds int     `json:"max_time_seconds"`
	MaxCostUSD     float64 `json:"max_cost_usd"`
}

// DefaultDiscoveryThresholds returns the target thresholds for the optimized
// infrastructure-discovery agent (the "after" benchmark).
func DefaultDiscoveryThresholds() Thresholds {
	return Thresholds{
		MaxTurns:       7,     // Target: 5, allow 2 buffer
		MaxTokens:      12000, // Target: ~6K, allow 2x buffer
		MaxToolCalls:   20,    // Target: ~10, allow 2x buffer
		MaxTimeSeconds: 180,   // 3 minutes max
		MaxCostUSD:     0.05,  // $0.05 max per run
	}
}

// BaselineDiscoveryMetrics returns the metrics from the FIRST unoptimized run
// (2026-03-09, MONOLITH). Used as the comparison baseline.
func BaselineDiscoveryMetrics() AgentRunMetrics {
	return AgentRunMetrics{
		AgentSlug:       "infrastructure-discovery",
		SatelliteID:     "MONOLITH",
		SessionID:       "baseline-2026-03-09",
		RunDate:         "2026-03-09",
		TurnCount:       16,
		ToolCallCount:   23,
		TotalTokens:     33000,
		ExecutionTimeMs: 139000, // 2m19s
	}
}

// ValidationResult represents a single check in the benchmark.
type ValidationResult struct {
	Name     string `json:"name"`
	Passed   bool   `json:"passed"`
	Actual   string `json:"actual"`
	Expected string `json:"expected"`
	Category string `json:"category"` // "performance", "quality", "security"
}

// BenchmarkReport contains all validation results and a summary.
type BenchmarkReport struct {
	AgentSlug     string             `json:"agent_slug"`
	RunDate       string             `json:"run_date"`
	Baseline      AgentRunMetrics    `json:"baseline"`
	Current       AgentRunMetrics    `json:"current"`
	Thresholds    Thresholds         `json:"thresholds"`
	Results       []ValidationResult `json:"results"`
	PassCount     int                `json:"pass_count"`
	FailCount     int                `json:"fail_count"`
	OverallPassed bool               `json:"overall_passed"`
}

// ValidateAgentRun checks metrics against thresholds and context file quality.
func ValidateAgentRun(metrics AgentRunMetrics, thresholds Thresholds, contextDir string) BenchmarkReport {
	report := BenchmarkReport{
		AgentSlug:  metrics.AgentSlug,
		RunDate:    metrics.RunDate,
		Baseline:   BaselineDiscoveryMetrics(),
		Current:    metrics,
		Thresholds: thresholds,
	}

	// ── Performance checks ──────────────────────────────────────────
	report.addCheck("Turn count", "performance",
		metrics.TurnCount <= thresholds.MaxTurns,
		fmt.Sprintf("%d", metrics.TurnCount),
		fmt.Sprintf("<= %d", thresholds.MaxTurns))

	report.addCheck("Token usage", "performance",
		metrics.TotalTokens <= thresholds.MaxTokens,
		fmt.Sprintf("%d", metrics.TotalTokens),
		fmt.Sprintf("<= %d", thresholds.MaxTokens))

	report.addCheck("Tool call count", "performance",
		metrics.ToolCallCount <= thresholds.MaxToolCalls,
		fmt.Sprintf("%d", metrics.ToolCallCount),
		fmt.Sprintf("<= %d", thresholds.MaxToolCalls))

	if metrics.ExecutionTimeMs > 0 {
		timeSec := float64(metrics.ExecutionTimeMs) / 1000.0
		report.addCheck("Execution time", "performance",
			int(timeSec) <= thresholds.MaxTimeSeconds,
			fmt.Sprintf("%.1fs", timeSec),
			fmt.Sprintf("<= %ds", thresholds.MaxTimeSeconds))
	}

	if metrics.CostUSD > 0 {
		report.addCheck("Cost per run", "performance",
			metrics.CostUSD <= thresholds.MaxCostUSD,
			fmt.Sprintf("$%.4f", metrics.CostUSD),
			fmt.Sprintf("<= $%.4f", thresholds.MaxCostUSD))
	}

	// ── Improvement over baseline ──────────────────────────────
	baseline := report.Baseline
	if baseline.TurnCount > 0 {
		turnReduction := float64(baseline.TurnCount-metrics.TurnCount) / float64(baseline.TurnCount) * 100
		report.addCheck("Turn reduction vs baseline", "performance",
			metrics.TurnCount < baseline.TurnCount,
			fmt.Sprintf("%d → %d (%.0f%% reduction)", baseline.TurnCount, metrics.TurnCount, turnReduction),
			fmt.Sprintf("< %d (baseline)", baseline.TurnCount))
	}
	if baseline.TotalTokens > 0 {
		tokenReduction := float64(baseline.TotalTokens-metrics.TotalTokens) / float64(baseline.TotalTokens) * 100
		report.addCheck("Token reduction vs baseline", "performance",
			metrics.TotalTokens < baseline.TotalTokens,
			fmt.Sprintf("%d → %d (%.0f%% reduction)", baseline.TotalTokens, metrics.TotalTokens, tokenReduction),
			fmt.Sprintf("< %d (baseline)", baseline.TotalTokens))
	}

	// ── Quality checks ──────────────────────────────────────────
	if contextDir != "" {
		qualityResults := ValidateContextFiles(contextDir)
		report.Results = append(report.Results, qualityResults...)
	}

	// Manual quality indicators from metrics
	report.addCheck("YAML frontmatter present", "quality",
		metrics.HasYAMLFrontmatter,
		fmt.Sprintf("%v", metrics.HasYAMLFrontmatter), "true")

	report.addCheck("GPU information discovered", "quality",
		metrics.HasGPUInfo,
		fmt.Sprintf("%v", metrics.HasGPUInfo), "true")

	report.addCheck("Disk utilization %% included", "quality",
		metrics.HasDiskUtilization,
		fmt.Sprintf("%v", metrics.HasDiskUtilization), "true")

	// ── Compute summary ─────────────────────────────────────────
	for _, r := range report.Results {
		if r.Passed {
			report.PassCount++
		} else {
			report.FailCount++
		}
	}
	report.OverallPassed = report.FailCount == 0

	return report
}

func (r *BenchmarkReport) addCheck(name, category string, passed bool, actual, expected string) {
	r.Results = append(r.Results, ValidationResult{
		Name:     name,
		Passed:   passed,
		Actual:   actual,
		Expected: expected,
		Category: category,
	})
}

// ValidateContextFiles checks that all required context files exist and have
// the expected structure. Returns validation results for each check.
func ValidateContextFiles(contextDir string) []ValidationResult {
	var results []ValidationResult

	// Required context files
	requiredFiles := map[string]string{
		"systeminfo.md":       "System information with YAML frontmatter",
		"topology.md":         "Network topology",
		"dependencies.md":     "Service dependencies",
		"discovery-report.md": "Executive summary with machine classification",
	}

	for filename, desc := range requiredFiles {
		filePath := filepath.Join(contextDir, filename)
		_, err := os.Stat(filePath)
		results = append(results, ValidationResult{
			Name:     fmt.Sprintf("Context file: %s", filename),
			Passed:   err == nil,
			Actual:   boolToPresent(err == nil),
			Expected: fmt.Sprintf("present (%s)", desc),
			Category: "quality",
		})
	}

	// Validate systeminfo.md has YAML frontmatter
	sysInfoPath := filepath.Join(contextDir, "systeminfo.md")
	if _, err := os.Stat(sysInfoPath); err == nil {
		fm, parseErr := ParseContextFrontmatter(sysInfoPath)

		results = append(results, ValidationResult{
			Name:     "systeminfo.md: YAML frontmatter parseable",
			Passed:   parseErr == nil && fm.DAAOSchema != "",
			Actual:   fmt.Sprintf("schema=%q", fm.DAAOSchema),
			Expected: "daao_schema present",
			Category: "quality",
		})

		results = append(results, ValidationResult{
			Name:     "systeminfo.md: os_family populated",
			Passed:   fm.OSFamily != "",
			Actual:   fmt.Sprintf("os_family=%q", fm.OSFamily),
			Expected: "non-empty (windows/linux/darwin)",
			Category: "quality",
		})

		results = append(results, ValidationResult{
			Name:     "systeminfo.md: machine_classes populated",
			Passed:   len(fm.MachineClasses) > 0,
			Actual:   fmt.Sprintf("%d classes", len(fm.MachineClasses)),
			Expected: ">= 1 machine class",
			Category: "quality",
		})

		results = append(results, ValidationResult{
			Name:     "systeminfo.md: recommended_agents populated",
			Passed:   len(fm.RecommendedAgents) > 0,
			Actual:   fmt.Sprintf("%d agents", len(fm.RecommendedAgents)),
			Expected: ">= 1 recommended agent",
			Category: "quality",
		})

		results = append(results, ValidationResult{
			Name:     "systeminfo.md: CPU info present",
			Passed:   fm.Hardware.CPU != "",
			Actual:   fmt.Sprintf("cpu=%q", fm.Hardware.CPU),
			Expected: "non-empty CPU model",
			Category: "quality",
		})

		results = append(results, ValidationResult{
			Name:     "systeminfo.md: GPU info present",
			Passed:   len(fm.Hardware.GPU) > 0,
			Actual:   fmt.Sprintf("%d GPUs", len(fm.Hardware.GPU)),
			Expected: ">= 1 GPU",
			Category: "quality",
		})

		results = append(results, ValidationResult{
			Name:     "systeminfo.md: disk utilization present",
			Passed:   fm.Hardware.HighestDiskUtilizationPct > 0,
			Actual:   fmt.Sprintf("%.1f%%", fm.Hardware.HighestDiskUtilizationPct),
			Expected: "> 0%",
			Category: "quality",
		})

		results = append(results, ValidationResult{
			Name:     "systeminfo.md: listening ports detected",
			Passed:   len(fm.Services.ListeningPorts) > 0,
			Actual:   fmt.Sprintf("%d ports", len(fm.Services.ListeningPorts)),
			Expected: ">= 1 listening port",
			Category: "quality",
		})
	}

	return results
}

func boolToPresent(b bool) string {
	if b {
		return "present"
	}
	return "missing"
}

// PrintReport outputs a human-readable benchmark report to stdout.
func PrintReport(report BenchmarkReport) string {
	var sb strings.Builder

	sb.WriteString("╔═══════════════════════════════════════════════════════════════╗\n")
	sb.WriteString("║        DAAO Agent Run Benchmark Report                      ║\n")
	sb.WriteString("╠═══════════════════════════════════════════════════════════════╣\n")
	sb.WriteString(fmt.Sprintf("║ Agent: %-20s  Date: %-22s ║\n", report.AgentSlug, report.RunDate))
	sb.WriteString("╠═══════════════════════════════════════════════════════════════╣\n")

	categories := []string{"performance", "quality", "security"}
	for _, cat := range categories {
		hasCategory := false
		for _, r := range report.Results {
			if r.Category == cat {
				if !hasCategory {
					sb.WriteString(fmt.Sprintf("║ ── %s ──\n", strings.ToUpper(cat)))
					hasCategory = true
				}
				icon := "✅"
				if !r.Passed {
					icon = "❌"
				}
				sb.WriteString(fmt.Sprintf("║ %s %-35s %s\n", icon, r.Name, r.Actual))
			}
		}
	}

	sb.WriteString("╠═══════════════════════════════════════════════════════════════╣\n")
	overallIcon := "✅ ALL CHECKS PASSED"
	if !report.OverallPassed {
		overallIcon = fmt.Sprintf("❌ %d/%d CHECKS FAILED", report.FailCount, report.PassCount+report.FailCount)
	}
	sb.WriteString(fmt.Sprintf("║ %s\n", overallIcon))
	sb.WriteString("╚═══════════════════════════════════════════════════════════════╝\n")

	return sb.String()
}

// ============================================================================
// Test Cases
// ============================================================================

// TestValidateAgentRun_Synthetic tests the validation framework with synthetic data.
func TestValidateAgentRun_Synthetic(t *testing.T) {
	t.Run("optimized-run-passes", func(t *testing.T) {
		metrics := AgentRunMetrics{
			AgentSlug:            "infrastructure-discovery",
			RunDate:              "2026-03-09",
			TurnCount:            5,
			ToolCallCount:        10,
			TotalTokens:          6000,
			ExecutionTimeMs:      90000, // 1.5 minutes
			HasYAMLFrontmatter:   true,
			HasGPUInfo:           true,
			HasDiskUtilization:   true,
			HasMachineClasses:    true,
			HasRecommendedAgents: true,
		}
		thresholds := DefaultDiscoveryThresholds()
		report := ValidateAgentRun(metrics, thresholds, "")

		t.Log("\n" + PrintReport(report))

		if !report.OverallPassed {
			for _, r := range report.Results {
				if !r.Passed {
					t.Errorf("FAIL: %s — got %s, want %s", r.Name, r.Actual, r.Expected)
				}
			}
		}
	})

	t.Run("unoptimized-run-fails", func(t *testing.T) {
		// Baseline metrics should FAIL against optimized thresholds
		metrics := BaselineDiscoveryMetrics()
		metrics.HasYAMLFrontmatter = false // Old prompt didn't emit this
		metrics.HasGPUInfo = false
		metrics.HasDiskUtilization = false
		thresholds := DefaultDiscoveryThresholds()
		report := ValidateAgentRun(metrics, thresholds, "")

		t.Log("\n" + PrintReport(report))

		if report.OverallPassed {
			t.Error("Baseline metrics should FAIL against optimized thresholds")
		}
		if report.FailCount == 0 {
			t.Error("Expected at least one failure for baseline metrics")
		}
	})
}

// TestValidateContextFiles_Synthetic creates a synthetic context directory and validates it.
func TestValidateContextFiles_Synthetic(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a complete synthetic context directory
	sysinfo := `---
daao_schema: "1.1"
last_discovered: "2026-03-09T17:00:00Z"
os_family: "windows"
machine_classes: ["Dev Workstation", "GPU Workstation"]
recommended_agents: ["security-scanner", "system-monitor"]
hardware:
  cpu: "AMD Ryzen 7 9800X3D"
  cores: 8
  ram_gb: 64
  gpu: ["NVIDIA GeForce RTX 4090"]
  highest_disk_utilization_pct: 72.3
services:
  listening_ports: [22, 5432, 11434, 3000]
  containers_running: 6
---
# System Information
Full system info markdown body.
`
	files := map[string]string{
		"systeminfo.md":       sysinfo,
		"topology.md":         "# Network Topology\n\nNetwork details.",
		"dependencies.md":     "# Dependencies\n\nService dependencies.",
		"discovery-report.md": "# Discovery Report\n\n## Machine Classification\nDev Workstation + GPU Workstation",
	}

	for name, content := range files {
		if err := os.WriteFile(filepath.Join(tmpDir, name), []byte(content), 0644); err != nil {
			t.Fatalf("Failed to create %s: %v", name, err)
		}
	}

	results := ValidateContextFiles(tmpDir)

	var failures []string
	for _, r := range results {
		if !r.Passed {
			failures = append(failures, fmt.Sprintf("FAIL: %s — got %s, want %s", r.Name, r.Actual, r.Expected))
		}
	}

	if len(failures) > 0 {
		t.Errorf("Context file validation failures:\n%s", strings.Join(failures, "\n"))
	}

	t.Logf("Validated %d checks, %d passed", len(results), len(results)-len(failures))
}

// TestBenchmarkLiveRun validates a REAL agent run from exported metrics.
// Set DAAO_BENCH_METRICS_FILE and DAAO_BENCH_CONTEXT_DIR to enable.
func TestBenchmarkLiveRun(t *testing.T) {
	metricsFile := os.Getenv("DAAO_BENCH_METRICS_FILE")
	contextDir := os.Getenv("DAAO_BENCH_CONTEXT_DIR")

	if metricsFile == "" {
		t.Skip("DAAO_BENCH_METRICS_FILE not set — skipping live benchmark. " +
			"To run: export DAAO_BENCH_METRICS_FILE=/path/to/metrics.json DAAO_BENCH_CONTEXT_DIR=/path/to/context")
	}

	// Load metrics from file
	data, err := os.ReadFile(metricsFile)
	if err != nil {
		t.Fatalf("Failed to read metrics file %s: %v", metricsFile, err)
	}

	var metrics AgentRunMetrics
	if err := json.Unmarshal(data, &metrics); err != nil {
		t.Fatalf("Failed to parse metrics JSON: %v", err)
	}

	// If context dir not specified, try the default
	if contextDir == "" {
		contextDir = ContextDir()
	}

	// Populate quality indicators from context files
	if _, err := os.Stat(contextDir); err == nil {
		sysInfoPath := filepath.Join(contextDir, "systeminfo.md")
		if fm, parseErr := ParseContextFrontmatter(sysInfoPath); parseErr == nil {
			metrics.HasYAMLFrontmatter = fm.DAAOSchema != ""
			metrics.HasGPUInfo = len(fm.Hardware.GPU) > 0
			metrics.HasDiskUtilization = fm.Hardware.HighestDiskUtilizationPct > 0
			metrics.HasMachineClasses = len(fm.MachineClasses) > 0
			metrics.HasRecommendedAgents = len(fm.RecommendedAgents) > 0
		}

		// List context files
		entries, _ := os.ReadDir(contextDir)
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
				metrics.ContextFilesWritten = append(metrics.ContextFilesWritten, e.Name())
			}
		}
	}

	// Run validation
	thresholds := DefaultDiscoveryThresholds()
	report := ValidateAgentRun(metrics, thresholds, contextDir)

	// Print the report
	reportStr := PrintReport(report)
	t.Log("\n" + reportStr)

	// Save report to file
	reportJSON, _ := json.MarshalIndent(report, "", "  ")
	reportPath := filepath.Join(contextDir, "benchmark-report.json")
	if err := os.WriteFile(reportPath, reportJSON, 0644); err != nil {
		t.Logf("Warning: could not save report to %s: %v", reportPath, err)
	} else {
		t.Logf("Benchmark report saved to %s", reportPath)
	}

	// Fail test if any checks failed
	if !report.OverallPassed {
		for _, r := range report.Results {
			if !r.Passed {
				t.Errorf("FAIL: %s — got %s, want %s", r.Name, r.Actual, r.Expected)
			}
		}
	}
}

// TestBenchmarkReport_Format verifies the report renders correctly.
func TestBenchmarkReport_Format(t *testing.T) {
	report := BenchmarkReport{
		AgentSlug: "test-agent",
		RunDate:   time.Now().Format("2006-01-02"),
		Results: []ValidationResult{
			{Name: "Turn count", Passed: true, Actual: "5", Expected: "<= 7", Category: "performance"},
			{Name: "Token usage", Passed: false, Actual: "15000", Expected: "<= 12000", Category: "performance"},
			{Name: "GPU detected", Passed: true, Actual: "true", Expected: "true", Category: "quality"},
		},
		PassCount:     2,
		FailCount:     1,
		OverallPassed: false,
	}

	output := PrintReport(report)
	t.Log("\n" + output)

	if !strings.Contains(output, "FAILED") {
		t.Error("Report should contain FAILED for failing run")
	}
	if !strings.Contains(output, "Turn count") {
		t.Error("Report should contain check names")
	}
}
