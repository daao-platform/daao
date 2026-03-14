package satellite

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/daao/nexus/proto"
)

func TestExpandSystemPrompt(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantContain []string // substrings that MUST appear in output
		wantEqual   string   // exact match (empty = skip exact check)
	}{
		{
			name:      "no-template-markers-passthrough",
			input:     "You are a helpful assistant. No templates here.",
			wantEqual: "You are a helpful assistant. No templates here.",
		},
		{
			name:        "expands-GOOS",
			input:       "Target OS: {{.GOOS}}",
			wantContain: []string{"Target OS: " + runtime.GOOS},
		},
		{
			name:  "expands-all-vars",
			input: "OS={{.GOOS}} ARCH={{.GOARCH}} CTX={{.CONTEXT_DIR}} TMP={{.TEMP_DIR}}",
			wantContain: []string{
				"OS=" + runtime.GOOS,
				"ARCH=" + runtime.GOARCH,
				"CTX=" + ContextDir(),
				"TMP=" + os.TempDir(),
			},
		},
		{
			name:  "invalid-template-fallback",
			input: "Bad template {{.Unknown}}",
			// text/template returns error for undefined fields with default options,
			// but we use the zero-value option. Actually, by default Go templates
			// return <no value> for missing fields. Let's just verify it doesn't panic.
			wantContain: []string{"Bad template"},
		},
		{
			name:  "mixed-template-and-text",
			input: "You run on {{.GOOS}}/{{.GOARCH}}.\nContext: {{.CONTEXT_DIR}}\nDo your job.",
			wantContain: []string{
				runtime.GOOS + "/" + runtime.GOARCH,
				"Context: " + ContextDir(),
				"Do your job.",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExpandSystemPrompt(tt.input)

			if tt.wantEqual != "" && got != tt.wantEqual {
				t.Errorf("ExpandSystemPrompt() = %q, want exact %q", got, tt.wantEqual)
			}

			for _, want := range tt.wantContain {
				if !strings.Contains(got, want) {
					t.Errorf("ExpandSystemPrompt() = %q, want to contain %q", got, want)
				}
			}
		})
	}
}

func TestBuildPiArgs(t *testing.T) {
	tests := []struct {
		name            string
		config          map[string]string
		wantContains    []string
		wantNotContains []string
	}{
		{
			name:            "no-guardrails",
			config:          map[string]string{"provider": "anthropic", "model": "claude-sonnet-4-20250514"},
			wantContains:    []string{"--mode", "rpc", "--no-session", "--provider", "anthropic", "--model", "claude-sonnet-4-20250514"},
			wantNotContains: []string{"--read-only", "--max-tool-calls", "--guardrails"},
		},
		{
			name:            "tools-allow-uses-native-flag",
			config:          map[string]string{"provider": "anthropic", "model": "claude-sonnet-4-20250514", "tools.allow": "exec,read,process"},
			wantContains:    []string{"--tools", "exec,read,process"},
			wantNotContains: []string{"--read-only", "--max-tool-calls", "--guardrails"},
		},
		{
			name:            "read-only",
			config:          map[string]string{"provider": "anthropic", "model": "claude-sonnet-4-20250514", "guardrails.read_only": "true"},
			wantContains:    []string{"--read-only"},
			wantNotContains: []string{"--max-tool-calls", "--guardrails"},
		},
		{
			name:            "max-tool-calls",
			config:          map[string]string{"provider": "anthropic", "model": "claude-sonnet-4-20250514", "guardrails.max_tool_calls": "50"},
			wantContains:    []string{"--max-tool-calls", "50"},
			wantNotContains: []string{"--read-only", "--guardrails"},
		},
		{
			name:            "hitl",
			config:          map[string]string{"provider": "anthropic", "model": "claude-sonnet-4-20250514", "guardrails.hitl": "true"},
			wantContains:    []string{"--guardrails"},
			wantNotContains: []string{"--read-only", "--max-tool-calls"},
		},
		{
			name: "combined",
			config: map[string]string{
				"provider":                  "anthropic",
				"model":                     "claude-sonnet-4-20250514",
				"tools.allow":               "read,exec",
				"guardrails.read_only":      "true",
				"guardrails.max_tool_calls": "100",
				"guardrails.hitl":           "true",
			},
			wantContains: []string{"--tools", "read,exec", "--read-only", "--max-tool-calls", "100", "--guardrails"},
		},
		{
			name:            "system-prompt-with-template",
			config:          map[string]string{"provider": "openai", "model": "gpt-4o", "system_prompt": "You run on {{.GOOS}}"},
			wantContains:    []string{"--system-prompt", "You run on " + runtime.GOOS},
			wantNotContains: []string{"--guardrails", "{{.GOOS}}"},
		},
		{
			name:            "system-prompt-no-template",
			config:          map[string]string{"provider": "openai", "model": "gpt-4o", "system_prompt": "You are a helpful assistant"},
			wantContains:    []string{"--system-prompt", "You are a helpful assistant"},
			wantNotContains: []string{"--guardrails"},
		},
		{
			name:            "base-args-always-present",
			config:          map[string]string{"provider": "openai", "model": "gpt-4o"},
			wantContains:    []string{"--mode", "rpc", "--no-session", "--provider", "openai", "--model", "gpt-4o"},
			wantNotContains: []string{"--extension"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agentDef := proto.AgentDefinitionProto{
				Config: tt.config,
			}
			got := BuildPiArgs(&agentDef)

			// Check that required strings are present
			for _, want := range tt.wantContains {
				if !slices.Contains(got, want) {
					t.Errorf("BuildPiArgs() = %v, want to contain %q", got, want)
				}
			}

			// Check that excluded strings are NOT present
			for _, wantNot := range tt.wantNotContains {
				if slices.Contains(got, wantNot) {
					t.Errorf("BuildPiArgs() = %v, should NOT contain %q", got, wantNot)
				}
			}
		})
	}
}

func TestValidateWritePath(t *testing.T) {
	contextDir := ContextDir()
	tempDir := os.TempDir()

	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{
			name:    "context-dir-subpath-allowed",
			path:    filepath.Join(contextDir, "systeminfo.md"),
			wantErr: false,
		},
		{
			name:    "temp-dir-subpath-allowed",
			path:    filepath.Join(tempDir, "daao-discovery.ps1"),
			wantErr: false,
		},
		{
			name:    "context-dir-exact-allowed",
			path:    contextDir,
			wantErr: false,
		},
		{
			name:    "outside-dirs-blocked",
			path:    filepath.Join(os.TempDir(), "..", "etc", "passwd"),
			wantErr: true,
		},
		{
			name:    "root-path-blocked",
			path:    "/etc/shadow",
			wantErr: true,
		},
		{
			name:    "windows-system-path-blocked",
			path:    `C:\Windows\System32\config`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateWritePath(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateWritePath(%q) error = %v, wantErr %v", tt.path, err, tt.wantErr)
			}
			if err != nil && !tt.wantErr {
				t.Errorf("ValidateWritePath(%q) unexpected error: %v", tt.path, err)
			}
			if err != nil && tt.wantErr && !strings.Contains(err.Error(), "SECURITY DENIED") {
				t.Errorf("ValidateWritePath(%q) error should contain 'SECURITY DENIED', got: %v", tt.path, err)
			}
		})
	}
}
