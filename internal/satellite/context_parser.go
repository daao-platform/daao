package satellite

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// ContextFrontmatter represents the YAML frontmatter schema for DAAO context files.
// Written by the infrastructure-discovery agent and parsed by Nexus for ecosystem integration.
type ContextFrontmatter struct {
	DAAOSchema        string       `yaml:"daao_schema" json:"daao_schema"`
	LastDiscovered    string       `yaml:"last_discovered" json:"last_discovered"`
	OSFamily          string       `yaml:"os_family" json:"os_family"`
	MachineClasses    []string     `yaml:"machine_classes" json:"machine_classes"`
	RecommendedAgents []string     `yaml:"recommended_agents" json:"recommended_agents"`
	Hardware          HardwareInfo `yaml:"hardware" json:"hardware"`
	Services          ServicesInfo `yaml:"services" json:"services"`
}

// HardwareInfo contains hardware discovery results.
type HardwareInfo struct {
	CPU                       string   `yaml:"cpu" json:"cpu"`
	Cores                     int      `yaml:"cores" json:"cores"`
	RAMGB                     float64  `yaml:"ram_gb" json:"ram_gb"`
	RAMSpeedMHz               int      `yaml:"ram_speed_mhz,omitempty" json:"ram_speed_mhz,omitempty"`
	RAMType                   string   `yaml:"ram_type,omitempty" json:"ram_type,omitempty"`
	GPU                       []string `yaml:"gpu" json:"gpu"`
	Storage                   []string `yaml:"storage,omitempty" json:"storage,omitempty"`
	HighestDiskUtilizationPct float64  `yaml:"highest_disk_utilization_pct" json:"highest_disk_utilization_pct"`
}

// ServicesInfo contains discovered service information.
type ServicesInfo struct {
	ListeningPorts    []int `yaml:"listening_ports" json:"listening_ports"`
	ContainersRunning int   `yaml:"containers_running" json:"containers_running"`
}

// AgentRecommendation represents a recommended agent deployment with rationale.
type AgentRecommendation struct {
	AgentSlug      string   `json:"agent_slug"`
	Reason         string   `json:"reason"`
	MachineClasses []string `json:"machine_classes"`
}

// ParseContextFrontmatter reads a .md file from the context directory and extracts
// the YAML frontmatter between the first pair of --- delimiters.
// Returns an empty ContextFrontmatter (not an error) if no frontmatter exists,
// so callers can safely check fields without error handling.
func ParseContextFrontmatter(filePath string) (ContextFrontmatter, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return ContextFrontmatter{}, fmt.Errorf("read context file: %w", err)
	}

	yamlBlock, ok := extractFrontmatter(string(data))
	if !ok {
		return ContextFrontmatter{}, nil // No frontmatter is valid — agent hasn't run yet
	}

	var fm ContextFrontmatter
	if err := yaml.Unmarshal([]byte(yamlBlock), &fm); err != nil {
		return ContextFrontmatter{}, fmt.Errorf("parse YAML frontmatter: %w", err)
	}

	return fm, nil
}

// ParseSystemInfoFile is a convenience function that reads and parses
// systeminfo.md from the standard context directory.
func ParseSystemInfoFile() (ContextFrontmatter, error) {
	return ParseContextFrontmatter(filepath.Join(ContextDir(), "systeminfo.md"))
}

// GetAgentRecommendations generates agent deployment recommendations by analyzing
// the parsed context frontmatter. Used by Nexus to show actionable alerts in Cockpit.
func GetAgentRecommendations(fm ContextFrontmatter) []AgentRecommendation {
	var recs []AgentRecommendation

	// Use the agent's own recommendations if available
	for _, agent := range fm.RecommendedAgents {
		rec := AgentRecommendation{
			AgentSlug:      agent,
			MachineClasses: fm.MachineClasses,
		}
		switch agent {
		case "security-scanner":
			rec.Reason = "Detected exposed services — security scan recommended"
		case "log-analyzer":
			rec.Reason = "Running services detected — log analysis recommended"
		case "system-monitor":
			rec.Reason = "Infrastructure detected — continuous monitoring recommended"
		case "deployment-assistant":
			rec.Reason = "CI/CD environment detected — deployment assistance recommended"
		default:
			rec.Reason = fmt.Sprintf("Discovery agent recommended %s", agent)
		}
		recs = append(recs, rec)
	}

	// Add high-priority alerts based on hardware
	if fm.Hardware.HighestDiskUtilizationPct >= 90.0 {
		recs = append(recs, AgentRecommendation{
			AgentSlug:      "system-monitor",
			Reason:         fmt.Sprintf("CRITICAL: Disk utilization at %.1f%% — monitoring urgently needed", fm.Hardware.HighestDiskUtilizationPct),
			MachineClasses: fm.MachineClasses,
		})
	}

	return recs
}

// extractFrontmatter finds the YAML block between the first pair of "---" lines.
// Returns the YAML content (without delimiters) and true if found.
func extractFrontmatter(content string) (string, bool) {
	scanner := bufio.NewScanner(strings.NewReader(content))
	var (
		inFrontmatter bool
		lines         []string
	)

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if trimmed == "---" {
			if !inFrontmatter {
				inFrontmatter = true
				continue
			}
			// Second --- found — return collected lines
			return strings.Join(lines, "\n"), true
		}

		if inFrontmatter {
			lines = append(lines, line)
		}
	}

	return "", false
}

// DriftReport represents changes detected between two discovery runs.
type DriftReport struct {
	NewPorts     []int    `json:"new_ports,omitempty"`
	RemovedPorts []int    `json:"removed_ports,omitempty"`
	NewClasses   []string `json:"new_classes,omitempty"`
	DiskDelta    float64  `json:"disk_delta,omitempty"` // positive = more full
	HasDrift     bool     `json:"has_drift"`
}

// DetectDrift compares two ContextFrontmatter snapshots and returns a DriftReport.
// Used to detect infrastructure changes between discovery runs.
func DetectDrift(previous, current ContextFrontmatter) DriftReport {
	report := DriftReport{}

	// Detect new/removed listening ports
	prevPorts := toIntSet(previous.Services.ListeningPorts)
	currPorts := toIntSet(current.Services.ListeningPorts)

	for port := range currPorts {
		if !prevPorts[port] {
			report.NewPorts = append(report.NewPorts, port)
			report.HasDrift = true
		}
	}
	for port := range prevPorts {
		if !currPorts[port] {
			report.RemovedPorts = append(report.RemovedPorts, port)
			report.HasDrift = true
		}
	}

	// Detect new machine classes
	prevClasses := toStringSet(previous.MachineClasses)
	for _, class := range current.MachineClasses {
		if !prevClasses[class] {
			report.NewClasses = append(report.NewClasses, class)
			report.HasDrift = true
		}
	}

	// Detect disk utilization change
	if previous.Hardware.HighestDiskUtilizationPct > 0 && current.Hardware.HighestDiskUtilizationPct > 0 {
		report.DiskDelta = current.Hardware.HighestDiskUtilizationPct - previous.Hardware.HighestDiskUtilizationPct
		if report.DiskDelta > 5.0 || report.DiskDelta < -5.0 { // 5% threshold
			report.HasDrift = true
		}
	}

	return report
}

func toIntSet(items []int) map[int]bool {
	s := make(map[int]bool, len(items))
	for _, i := range items {
		s[i] = true
	}
	return s
}

func toStringSet(items []string) map[string]bool {
	s := make(map[string]bool, len(items))
	for _, i := range items {
		s[i] = true
	}
	return s
}
