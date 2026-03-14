// Package satellite provides satellite runtime configuration and availability checking.
package satellite

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"gopkg.in/yaml.v3"
)

// SatelliteConfig represents the full satellite configuration
type SatelliteConfig struct {
	AgentRuntime *RuntimeConfig `yaml:"agent_runtime"`
}

// RuntimeConfig represents the runtime configuration for the agent
type RuntimeConfig struct {
	Enabled bool `yaml:"enabled"`
}

// DefaultRuntimeConfigPath returns the default path for satellite config
func DefaultRuntimeConfigPath() string {
	// Check environment variable first
	if configPath := os.Getenv("DAAO_SATELLITE_CONFIG"); configPath != "" {
		return configPath
	}

	// Default locations based on OS
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	switch runtime.GOOS {
	case "linux":
		return "/etc/daao/satellite/config.yaml"
	case "windows":
		return filepath.Join(homeDir, ".config", "daao", "satellite", "config.yaml")
	default:
		return filepath.Join(homeDir, ".config", "daao", "satellite", "config.yaml")
	}
}

// LoadRuntimeConfig loads runtime configuration from the satellite config file
func LoadRuntimeConfig(configPath string) (*RuntimeConfig, error) {
	if configPath == "" {
		configPath = DefaultRuntimeConfigPath()
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Config file doesn't exist, return default (enabled)
			return &RuntimeConfig{Enabled: true}, nil
		}
		return nil, fmt.Errorf("failed to read satellite config: %w", err)
	}

	// Parse YAML and extract runtime config
	config, err := parseRuntimeConfig(data)
	if err != nil {
		return nil, err
	}

	return config, nil
}

// parseRuntimeConfig parses the YAML and extracts runtime config
func parseRuntimeConfig(data []byte) (*RuntimeConfig, error) {
	var config SatelliteConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse satellite config: %w", err)
	}

	// If agent_runtime is not specified, default to enabled
	if config.AgentRuntime == nil {
		return &RuntimeConfig{Enabled: true}, nil
	}

	return config.AgentRuntime, nil
}

// DefaultRuntimeConfig returns a default runtime configuration
func DefaultRuntimeConfig() *RuntimeConfig {
	return &RuntimeConfig{Enabled: true}
}

// Runtime holds the singleton runtime configuration
var runtimeConfig *RuntimeConfig

// InitRuntime initializes the runtime configuration
func InitRuntime(configPath string) error {
	config, err := LoadRuntimeConfig(configPath)
	if err != nil {
		return err
	}
	runtimeConfig = config
	return nil
}

// GetRuntimeConfig returns the current runtime configuration
func GetRuntimeConfig() *RuntimeConfig {
	if runtimeConfig == nil {
		return DefaultRuntimeConfig()
	}
	return runtimeConfig
}

// IsRuntimeAvailable checks if the Node.js runtime is available
// It checks for the Node.js binary at the expected location
func IsRuntimeAvailable() bool {
	nodePath := NodeBinaryPath()
	if nodePath == "" {
		return false
	}

	// Check if the binary exists and is executable
	info, err := os.Stat(nodePath)
	if err != nil {
		return false
	}

	// Check if it's a regular file (not a directory)
	if !info.Mode().IsRegular() {
		return false
	}

	// On Windows, also check for .exe variant if the original path doesn't exist
	if runtime.GOOS == "windows" && os.IsNotExist(err) {
		exePath := nodePath + ".exe"
		if info, err := os.Stat(exePath); err == nil && info.Mode().IsRegular() {
			return true
		}
	}

	return true
}

// ValidateDeployment validates that the runtime can be used on this satellite
// Returns an error if the runtime is disabled
func ValidateDeployment() error {
	config := GetRuntimeConfig()
	if !config.Enabled {
		return errors.New("Agent runtime is disabled on this satellite")
	}
	return nil
}

// NodeBinaryPath returns the OS-appropriate path to the Node.js binary
func NodeBinaryPath() string {
	switch runtime.GOOS {
	case "linux":
		return "/opt/daao/runtime/node/bin/node"
	case "windows":
		// On Windows, check common locations
		programFiles := os.Getenv("ProgramFiles")
		if programFiles == "" {
			programFiles = "C:\\Program Files"
		}
		return filepath.Join(programFiles, "daao", "runtime", "node", "node.exe")
	default:
		// Fallback to Linux path for unknown OSes (likely dev environments)
		return "/opt/daao/runtime/node/bin/node"
	}
}

// LinuxNodeBinaryPath returns the Linux-specific Node.js binary path
func LinuxNodeBinaryPath() string {
	return "/opt/daao/runtime/node/bin/node"
}

// WindowsNodeBinaryPath returns the Windows-specific Node.js binary path
func WindowsNodeBinaryPath() string {
	programFiles := os.Getenv("ProgramFiles")
	if programFiles == "" {
		programFiles = "C:\\Program Files"
	}
	return filepath.Join(programFiles, "daao", "runtime", "node", "node.exe")
}

// ExtensionsDir returns the OS-appropriate path to the DAAO extensions directory
func ExtensionsDir() string {
	var platformPath string

	switch runtime.GOOS {
	case "linux", "darwin":
		platformPath = "/opt/daao/extensions/"
	case "windows":
		platformPath = "C:\\ProgramData\\daao\\extensions\\"
	default:
		// Fallback for unknown OSes (likely dev environments)
		platformPath = "/opt/daao/extensions/"
	}

	// Check if the platform path exists
	if _, err := os.Stat(platformPath); err == nil {
		return platformPath
	}

	// Check for extensions next to the binary (portable / Windows LOCALAPPDATA install)
	if exePath, err := os.Executable(); err == nil {
		portablePath := filepath.Join(filepath.Dir(exePath), "extensions")
		if _, err := os.Stat(portablePath); err == nil {
			return portablePath + string(filepath.Separator)
		}
	}

	// Fallback to ./extensions/ for development
	return "./extensions/"
}
