package satellite

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestNodeArch(t *testing.T) {
	tests := []struct {
		goarch string
		want   string
	}{
		{"amd64", "x64"},
		{"arm64", "arm64"},
		{"386", "x86"},
		{"mips", "mips"},
	}
	for _, tt := range tests {
		t.Run(tt.goarch, func(t *testing.T) {
			got := nodeArch(tt.goarch)
			if got != tt.want {
				t.Errorf("nodeArch(%q) = %q, want %q", tt.goarch, got, tt.want)
			}
		})
	}
}

func TestGetPlatformConfig(t *testing.T) {
	cfg := GetPlatformConfig()

	// Should match current platform
	if cfg.GOOS != runtime.GOOS {
		t.Errorf("GetPlatformConfig().GOOS = %q, want %q", cfg.GOOS, runtime.GOOS)
	}
	if cfg.GOARCH != runtime.GOARCH {
		t.Errorf("GetPlatformConfig().GOARCH = %q, want %q", cfg.GOARCH, runtime.GOARCH)
	}

	// RuntimeDir should be non-empty
	if cfg.RuntimeDir == "" {
		t.Error("GetPlatformConfig().RuntimeDir is empty")
	}

	// NodeBinary should be a full path
	if !filepath.IsAbs(cfg.NodeBinary) {
		t.Errorf("GetPlatformConfig().NodeBinary = %q, want absolute path", cfg.NodeBinary)
	}

	// PiBinary should be a full path
	if !filepath.IsAbs(cfg.PiBinary) {
		t.Errorf("GetPlatformConfig().PiBinary = %q, want absolute path", cfg.PiBinary)
	}

	// ExtractFunc should not be nil
	if cfg.ExtractFunc == nil {
		t.Error("GetPlatformConfig().ExtractFunc is nil")
	}

	// Archive extension should be appropriate for platform
	switch runtime.GOOS {
	case "windows":
		if cfg.ArchiveExt != ".zip" {
			t.Errorf("GetPlatformConfig().ArchiveExt = %q, want .zip on Windows", cfg.ArchiveExt)
		}
	case "darwin":
		if cfg.ArchiveExt != ".tar.gz" {
			t.Errorf("GetPlatformConfig().ArchiveExt = %q, want .tar.gz on macOS", cfg.ArchiveExt)
		}
	case "linux":
		if cfg.ArchiveExt != ".tar.xz" {
			t.Errorf("GetPlatformConfig().ArchiveExt = %q, want .tar.xz on Linux", cfg.ArchiveExt)
		}
	}
}

func TestNodeTarballURL(t *testing.T) {
	tests := []struct {
		name    string
		version string
		goos    string
		goarch  string
		want    string
	}{
		{
			name:    "linux-amd64",
			version: "22.14.0",
			goos:    "linux",
			goarch:  "amd64",
			want:    "https://nodejs.org/dist/v22.14.0/node-v22.14.0-linux-x64.tar.xz",
		},
		{
			name:    "linux-arm64",
			version: "22.14.0",
			goos:    "linux",
			goarch:  "arm64",
			want:    "https://nodejs.org/dist/v22.14.0/node-v22.14.0-linux-arm64.tar.xz",
		},
		{
			name:    "darwin-arm64",
			version: "22.14.0",
			goos:    "darwin",
			goarch:  "arm64",
			want:    "https://nodejs.org/dist/v22.14.0/node-v22.14.0-darwin-arm64.tar.gz",
		},
		{
			name:    "darwin-amd64",
			version: "22.14.0",
			goos:    "darwin",
			goarch:  "amd64",
			want:    "https://nodejs.org/dist/v22.14.0/node-v22.14.0-darwin-x64.tar.gz",
		},
		{
			name:    "windows-amd64",
			version: "22.14.0",
			goos:    "windows",
			goarch:  "amd64",
			want:    "https://nodejs.org/dist/v22.14.0/node-v22.14.0-win-x64.zip",
		},
		{
			name:    "windows-arm64",
			version: "22.14.0",
			goos:    "windows",
			goarch:  "arm64",
			want:    "https://nodejs.org/dist/v22.14.0/node-v22.14.0-win-arm64.zip",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear the mirror env var so we test against the default
			os.Unsetenv("DAAO_NODE_MIRROR")
			got := NodeTarballURL(tt.version, tt.goos, tt.goarch)
			if got != tt.want {
				t.Errorf("NodeTarballURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNodeTarballURL_Mirror(t *testing.T) {
	os.Setenv("DAAO_NODE_MIRROR", "https://internal-mirror.example.com/node")
	defer os.Unsetenv("DAAO_NODE_MIRROR")

	got := NodeTarballURL("22.14.0", "linux", "amd64")
	want := "https://internal-mirror.example.com/node/v22.14.0/node-v22.14.0-linux-x64.tar.xz"
	if got != want {
		t.Errorf("NodeTarballURL(mirror) = %q, want %q", got, want)
	}
}

func TestIsRuntimeInstalled(t *testing.T) {
	// Without any files, runtime should not be installed
	if IsRuntimeInstalled() {
		t.Skip("Runtime appears to be actually installed on this machine — skipping negative test")
	}
}

func TestRuntimeDir(t *testing.T) {
	dir := RuntimeDir()
	if dir == "" {
		t.Error("RuntimeDir() returned empty string")
	}
}

func TestPiBinaryPath(t *testing.T) {
	path := PiBinaryPath()
	if path == "" {
		t.Error("PiBinaryPath() returned empty string")
	}
	// Should be an absolute path
	if !filepath.IsAbs(path) {
		t.Errorf("PiBinaryPath() = %q, want absolute path", path)
	}
}
