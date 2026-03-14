package satellite

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExtractFrontmatter(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		wantYAML string
		wantOK   bool
	}{
		{
			name: "valid-frontmatter",
			content: `---
daao_schema: "1.1"
os_family: "windows"
---
# System Info
Body text`,
			wantYAML: "daao_schema: \"1.1\"\nos_family: \"windows\"",
			wantOK:   true,
		},
		{
			name:     "no-frontmatter",
			content:  "# Just markdown\nNo frontmatter here.",
			wantYAML: "",
			wantOK:   false,
		},
		{
			name: "unclosed-frontmatter",
			content: `---
daao_schema: "1.1"
# Never closed`,
			wantYAML: "",
			wantOK:   false,
		},
		{
			name:     "empty-content",
			content:  "",
			wantYAML: "",
			wantOK:   false,
		},
		{
			name: "frontmatter-with-nested-yaml",
			content: `---
daao_schema: "1.1"
hardware:
  cpu: "AMD Ryzen 9800X3D"
  cores: 8
---
# Body`,
			wantYAML: "daao_schema: \"1.1\"\nhardware:\n  cpu: \"AMD Ryzen 9800X3D\"\n  cores: 8",
			wantOK:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := extractFrontmatter(tt.content)
			if ok != tt.wantOK {
				t.Errorf("extractFrontmatter() ok = %v, want %v", ok, tt.wantOK)
			}
			if got != tt.wantYAML {
				t.Errorf("extractFrontmatter() =\n%q\nwant:\n%q", got, tt.wantYAML)
			}
		})
	}
}

func TestParseContextFrontmatter(t *testing.T) {
	// Create temp file with full frontmatter
	content := `---
daao_schema: "1.1"
last_discovered: "2026-03-09T17:00:00Z"
os_family: "windows"
machine_classes: ["Dev Workstation", "Container Host"]
recommended_agents: ["security-scanner", "log-analyzer"]
hardware:
  cpu: "AMD Ryzen 7 9800X3D"
  cores: 8
  ram_gb: 64
  gpu: ["NVIDIA GeForce RTX 4090"]
  highest_disk_utilization_pct: 82.4
services:
  listening_ports: [22, 5432, 11434]
  containers_running: 4
---
# System Information
This is the body text.

## Operator Notes
Human-written notes here.
`
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "systeminfo.md")
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}

	fm, err := ParseContextFrontmatter(tmpFile)
	if err != nil {
		t.Fatalf("ParseContextFrontmatter() error = %v", err)
	}

	// Verify all fields
	if fm.DAAOSchema != "1.1" {
		t.Errorf("DAAOSchema = %q, want %q", fm.DAAOSchema, "1.1")
	}
	if fm.OSFamily != "windows" {
		t.Errorf("OSFamily = %q, want %q", fm.OSFamily, "windows")
	}
	if len(fm.MachineClasses) != 2 {
		t.Errorf("MachineClasses = %v, want 2 items", fm.MachineClasses)
	}
	if len(fm.RecommendedAgents) != 2 {
		t.Errorf("RecommendedAgents = %v, want 2 items", fm.RecommendedAgents)
	}
	if fm.Hardware.CPU != "AMD Ryzen 7 9800X3D" {
		t.Errorf("Hardware.CPU = %q, want %q", fm.Hardware.CPU, "AMD Ryzen 7 9800X3D")
	}
	if fm.Hardware.Cores != 8 {
		t.Errorf("Hardware.Cores = %d, want 8", fm.Hardware.Cores)
	}
	if fm.Hardware.RAMGB != 64 {
		t.Errorf("Hardware.RAMGB = %f, want 64", fm.Hardware.RAMGB)
	}
	if len(fm.Hardware.GPU) != 1 || fm.Hardware.GPU[0] != "NVIDIA GeForce RTX 4090" {
		t.Errorf("Hardware.GPU = %v, want [NVIDIA GeForce RTX 4090]", fm.Hardware.GPU)
	}
	if fm.Hardware.HighestDiskUtilizationPct != 82.4 {
		t.Errorf("Hardware.DiskUtil = %f, want 82.4", fm.Hardware.HighestDiskUtilizationPct)
	}
	if len(fm.Services.ListeningPorts) != 3 {
		t.Errorf("Services.ListeningPorts = %v, want 3 items", fm.Services.ListeningPorts)
	}
	if fm.Services.ContainersRunning != 4 {
		t.Errorf("Services.ContainersRunning = %d, want 4", fm.Services.ContainersRunning)
	}
}

func TestParseContextFrontmatter_NoFrontmatter(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "systeminfo.md")
	if err := os.WriteFile(tmpFile, []byte("# Just markdown\nNo frontmatter."), 0644); err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}

	fm, err := ParseContextFrontmatter(tmpFile)
	if err != nil {
		t.Fatalf("ParseContextFrontmatter() error = %v (should be nil for no frontmatter)", err)
	}
	if fm.DAAOSchema != "" {
		t.Errorf("DAAOSchema = %q, want empty for no frontmatter", fm.DAAOSchema)
	}
}

func TestParseContextFrontmatter_MissingFile(t *testing.T) {
	_, err := ParseContextFrontmatter("/nonexistent/path/systeminfo.md")
	if err == nil {
		t.Error("ParseContextFrontmatter() should error for missing file")
	}
}

func TestDetectDrift(t *testing.T) {
	tests := []struct {
		name             string
		previous         ContextFrontmatter
		current          ContextFrontmatter
		wantDrift        bool
		wantNewPorts     int
		wantRemovedPorts int
	}{
		{
			name: "no-drift",
			previous: ContextFrontmatter{
				Services: ServicesInfo{ListeningPorts: []int{22, 80, 443}},
				Hardware: HardwareInfo{HighestDiskUtilizationPct: 50.0},
			},
			current: ContextFrontmatter{
				Services: ServicesInfo{ListeningPorts: []int{22, 80, 443}},
				Hardware: HardwareInfo{HighestDiskUtilizationPct: 51.0},
			},
			wantDrift: false,
		},
		{
			name: "new-port-detected",
			previous: ContextFrontmatter{
				Services: ServicesInfo{ListeningPorts: []int{22, 80}},
			},
			current: ContextFrontmatter{
				Services: ServicesInfo{ListeningPorts: []int{22, 80, 11434}},
			},
			wantDrift:    true,
			wantNewPorts: 1,
		},
		{
			name: "port-removed",
			previous: ContextFrontmatter{
				Services: ServicesInfo{ListeningPorts: []int{22, 80, 443}},
			},
			current: ContextFrontmatter{
				Services: ServicesInfo{ListeningPorts: []int{22, 80}},
			},
			wantDrift:        true,
			wantRemovedPorts: 1,
		},
		{
			name: "disk-spike",
			previous: ContextFrontmatter{
				Hardware: HardwareInfo{HighestDiskUtilizationPct: 50.0},
			},
			current: ContextFrontmatter{
				Hardware: HardwareInfo{HighestDiskUtilizationPct: 92.0},
			},
			wantDrift: true,
		},
		{
			name: "new-machine-class",
			previous: ContextFrontmatter{
				MachineClasses: []string{"Dev Workstation"},
			},
			current: ContextFrontmatter{
				MachineClasses: []string{"Dev Workstation", "Container Host"},
			},
			wantDrift: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := DetectDrift(tt.previous, tt.current)
			if report.HasDrift != tt.wantDrift {
				t.Errorf("DetectDrift().HasDrift = %v, want %v", report.HasDrift, tt.wantDrift)
			}
			if len(report.NewPorts) != tt.wantNewPorts {
				t.Errorf("NewPorts = %v (len %d), want %d", report.NewPorts, len(report.NewPorts), tt.wantNewPorts)
			}
			if len(report.RemovedPorts) != tt.wantRemovedPorts {
				t.Errorf("RemovedPorts = %v (len %d), want %d", report.RemovedPorts, len(report.RemovedPorts), tt.wantRemovedPorts)
			}
		})
	}
}

func TestGetAgentRecommendations(t *testing.T) {
	fm := ContextFrontmatter{
		MachineClasses:    []string{"Container Host"},
		RecommendedAgents: []string{"security-scanner", "system-monitor"},
		Hardware: HardwareInfo{
			HighestDiskUtilizationPct: 95.0, // Critical — should add extra rec
		},
	}

	recs := GetAgentRecommendations(fm)

	// Should have 3: security-scanner, system-monitor, + critical disk alert
	if len(recs) != 3 {
		t.Fatalf("GetAgentRecommendations() = %d recs, want 3", len(recs))
	}

	// Check critical disk alert
	var hasCritical bool
	for _, rec := range recs {
		if rec.AgentSlug == "system-monitor" && rec.Reason != "" {
			if rec.Reason != "Infrastructure detected — continuous monitoring recommended" {
				hasCritical = true
			}
		}
	}
	if !hasCritical {
		t.Error("Expected critical disk utilization alert in recommendations")
	}
}
