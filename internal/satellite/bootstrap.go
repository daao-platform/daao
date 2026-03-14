// Package satellite provides satellite runtime configuration and Pi RPC bridge functionality.
package satellite

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// nodeVersion is the Node.js LTS version used by DAAO runtime.
const nodeVersion = "22.14.0"

// ProgressFunc is called with (step, message) during bootstrap progress.
type ProgressFunc func(step, message string)

// platformConfig encodes all OS-specific provisioning knowledge — the "runbook"
// for each platform. This is DAAO's equivalent of an Ansible playbook, executed
// over gRPC instead of SSH, compiled into the satellite binary instead of YAML.
type platformConfig struct {
	GOOS        string
	GOARCH      string
	RuntimeDir  string // e.g. /opt/daao/runtime/
	NodeDir     string // e.g. /opt/daao/runtime/node/
	NodeBinary  string // e.g. /opt/daao/runtime/node/bin/node
	PiBinary    string // e.g. /opt/daao/runtime/node/bin/pi
	NpmBinary   string // e.g. /opt/daao/runtime/node/bin/npm
	NodeArch    string // Node.js arch name: x64, arm64
	ArchiveExt  string // .tar.xz, .tar.gz, .zip
	ExtractFunc func(ctx context.Context, archive, dest string) error
}

// nodeArch maps Go's GOARCH to Node.js distribution naming.
func nodeArch(goarch string) string {
	switch goarch {
	case "amd64":
		return "x64"
	case "arm64":
		return "arm64"
	case "386":
		return "x86"
	default:
		return goarch
	}
}

// GetPlatformConfig returns the provisioning configuration for the current OS/arch.
// Exported for testing.
func GetPlatformConfig() platformConfig {
	arch := nodeArch(runtime.GOARCH)

	switch runtime.GOOS {
	case "darwin":
		home, _ := os.UserHomeDir()
		if home == "" {
			home = "/tmp"
		}
		base := filepath.Join(home, "Library", "Application Support", "daao", "runtime")
		nodeDir := filepath.Join(base, "node")
		return platformConfig{
			GOOS:        "darwin",
			GOARCH:      runtime.GOARCH,
			RuntimeDir:  base,
			NodeDir:     nodeDir,
			NodeBinary:  filepath.Join(nodeDir, "bin", "node"),
			PiBinary:    filepath.Join(nodeDir, "bin", "pi"),
			NpmBinary:   filepath.Join(nodeDir, "bin", "npm"),
			NodeArch:    arch,
			ArchiveExt:  ".tar.gz",
			ExtractFunc: extractTarGZ,
		}
	case "windows":
		programFiles := os.Getenv("ProgramFiles")
		if programFiles == "" {
			programFiles = `C:\Program Files`
		}
		base := filepath.Join(programFiles, "daao", "runtime")
		nodeDir := filepath.Join(base, "node")
		return platformConfig{
			GOOS:        "windows",
			GOARCH:      runtime.GOARCH,
			RuntimeDir:  base,
			NodeDir:     nodeDir,
			NodeBinary:  filepath.Join(nodeDir, "node.exe"),
			PiBinary:    filepath.Join(nodeDir, "pi.cmd"),
			NpmBinary:   filepath.Join(nodeDir, "npm.cmd"),
			NodeArch:    arch,
			ArchiveExt:  ".zip",
			ExtractFunc: extractZip,
		}
	default: // linux
		return platformConfig{
			GOOS:        "linux",
			GOARCH:      runtime.GOARCH,
			RuntimeDir:  "/opt/daao/runtime/",
			NodeDir:     "/opt/daao/runtime/node/",
			NodeBinary:  "/opt/daao/runtime/node/bin/node",
			PiBinary:    "/opt/daao/runtime/node/bin/pi",
			NpmBinary:   "/opt/daao/runtime/node/bin/npm",
			NodeArch:    arch,
			ArchiveExt:  ".tar.xz",
			ExtractFunc: extractTarXZ,
		}
	}
}

// RuntimeDir returns the platform-specific runtime base directory.
func RuntimeDir() string {
	return GetPlatformConfig().RuntimeDir
}

// PiBinaryPath returns the full path where Pi should be installed.
func PiBinaryPath() string {
	return GetPlatformConfig().PiBinary
}

// IsRuntimeInstalled returns true if both Node.js and Pi are installed
// either in the DAAO runtime directory or available in the system PATH.
func IsRuntimeInstalled() bool {
	cfg := GetPlatformConfig()

	// Check DAAO runtime directory first
	if _, err := os.Stat(cfg.NodeBinary); err == nil {
		if _, err := os.Stat(cfg.PiBinary); err == nil {
			return true
		}
	}

	// Check system PATH (e.g. Alpine apk-installed Node.js)
	if _, err := exec.LookPath("node"); err == nil {
		if _, err := exec.LookPath("pi"); err == nil {
			return true
		}
	}

	return false
}

// NodeTarballURL returns the download URL for the Node.js binary distribution.
// It uses DAAO_NODE_MIRROR env var if set (for air-gapped environments),
// otherwise downloads from the official nodejs.org CDN.
func NodeTarballURL(version, goos, goarch string) string {
	base := os.Getenv("DAAO_NODE_MIRROR")
	if base == "" {
		base = "https://nodejs.org/dist"
	}
	arch := nodeArch(goarch)

	switch goos {
	case "windows":
		return fmt.Sprintf("%s/v%s/node-v%s-win-%s.zip", base, version, version, arch)
	case "darwin":
		return fmt.Sprintf("%s/v%s/node-v%s-darwin-%s.tar.gz", base, version, version, arch)
	default: // linux
		return fmt.Sprintf("%s/v%s/node-v%s-linux-%s.tar.xz", base, version, version, arch)
	}
}

// BootstrapRuntime downloads Node.js and installs Pi CLI into the DAAO
// runtime directory. This is the "Ansible-over-gRPC" provisioner — it
// executes the platform-specific runbook compiled into the binary.
//
// If Node.js is already available in the system PATH (e.g., Alpine `apk`),
// the download step is skipped and Pi CLI is installed using the system npm.
//
// The progressFn callback is called with (step, message) at each phase
// so callers can stream status back to Nexus via gRPC.
func BootstrapRuntime(ctx context.Context, progressFn ProgressFunc) error {
	if progressFn == nil {
		progressFn = func(_, _ string) {}
	}

	cfg := GetPlatformConfig()
	slog.Info("BootstrapRuntime: starting", "component", "satellite",
		"goos", cfg.GOOS, "goarch", cfg.GOARCH, "runtimeDir", cfg.RuntimeDir)

	// Step 1: Check if already installed
	progressFn("check", "Checking existing runtime...")
	if IsRuntimeInstalled() {
		progressFn("complete", "Runtime already installed")
		slog.Info("BootstrapRuntime: runtime already installed, skipping", "component", "satellite")
		return nil
	}

	// Check if Node.js is in system PATH (e.g. Alpine apk)
	systemNode, _ := exec.LookPath("node")
	systemNpm, _ := exec.LookPath("npm")
	hasSystemNode := systemNode != "" && systemNpm != ""

	if hasSystemNode {
		slog.Info("BootstrapRuntime: using system Node.js", "component", "satellite",
			"node", systemNode, "npm", systemNpm)
		progressFn("install_pi", "Installing Pi CLI via system npm...")

		// Install Pi CLI using the system npm, into the DAAO runtime dir
		// so the satellite service (running as SYSTEM) can find it reliably.
		if err := os.MkdirAll(cfg.NodeDir, 0755); err != nil {
			return fmt.Errorf("failed to create runtime dir %s: %w", cfg.NodeDir, err)
		}
		cmd := exec.CommandContext(ctx, systemNpm,
			"install", "-g", "@mariozechner/pi-coding-agent",
			"--prefix", cfg.NodeDir,
		)
		cmd.Env = os.Environ()
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("npm install failed: %w\nOutput: %s", err, string(output))
		}
		slog.Info("BootstrapRuntime: Pi CLI installed via system npm", "component", "satellite",
			"output", string(output))
	} else {
		// No system Node.js — download and install into DAAO runtime dir
		// Step 2: Create runtime directory
		progressFn("prepare", fmt.Sprintf("Creating runtime directory %s", cfg.RuntimeDir))
		if err := os.MkdirAll(cfg.RuntimeDir, 0755); err != nil {
			return fmt.Errorf("failed to create runtime directory %s: %w", cfg.RuntimeDir, err)
		}

		// Step 3: Download Node.js
		url := NodeTarballURL(nodeVersion, cfg.GOOS, cfg.GOARCH)
		progressFn("download_node", fmt.Sprintf("Downloading Node.js %s from %s", nodeVersion, url))
		slog.Info("BootstrapRuntime: downloading Node.js", "component", "satellite", "url", url)

		archivePath := filepath.Join(cfg.RuntimeDir, "node"+cfg.ArchiveExt)
		if err := downloadFile(ctx, url, archivePath); err != nil {
			return fmt.Errorf("failed to download Node.js: %w", err)
		}
		defer os.Remove(archivePath) // clean up archive after extraction

		// Step 4: Extract Node.js
		progressFn("extract_node", "Extracting Node.js...")
		slog.Info("BootstrapRuntime: extracting", "component", "satellite", "archive", archivePath)
		if err := cfg.ExtractFunc(ctx, archivePath, cfg.RuntimeDir); err != nil {
			return fmt.Errorf("failed to extract Node.js: %w", err)
		}

		// The extracted directory has a versioned name like "node-v22.14.0-linux-x64".
		// Rename it to just "node" for consistent paths.
		extractedName := fmt.Sprintf("node-v%s-%s-%s", nodeVersion, cfg.GOOS, cfg.NodeArch)
		if cfg.GOOS == "windows" {
			extractedName = fmt.Sprintf("node-v%s-win-%s", nodeVersion, cfg.NodeArch)
		}
		extractedDir := filepath.Join(cfg.RuntimeDir, extractedName)
		targetDir := cfg.NodeDir

		// Remove existing target if it exists (force re-provision)
		if _, err := os.Stat(targetDir); err == nil {
			os.RemoveAll(targetDir)
		}

		if err := os.Rename(extractedDir, targetDir); err != nil {
			// On some filesystems (cross-device), rename fails. Fall back to copy.
			slog.Warn("BootstrapRuntime: rename failed, trying copy",
				"component", "satellite", "from", extractedDir, "to", targetDir, "err", err)
			if cpErr := copyDir(extractedDir, targetDir); cpErr != nil {
				return fmt.Errorf("failed to move Node.js to %s: rename=%w, copy=%v", targetDir, err, cpErr)
			}
			os.RemoveAll(extractedDir)
		}

		// Step 5: Verify Node.js
		progressFn("verify_node", "Verifying Node.js installation...")
		if _, err := os.Stat(cfg.NodeBinary); err != nil {
			return fmt.Errorf("Node.js binary not found at %s after extraction: %w", cfg.NodeBinary, err)
		}
		slog.Info("BootstrapRuntime: Node.js installed", "component", "satellite", "binary", cfg.NodeBinary)

		// Step 6: Install Pi CLI via npm
		progressFn("install_pi", "Installing Pi CLI via npm...")
		slog.Info("BootstrapRuntime: installing Pi CLI", "component", "satellite")
		if err := installPiCLI(ctx, cfg); err != nil {
			return fmt.Errorf("failed to install Pi CLI: %w", err)
		}
	}

	// Step 7: Verify Pi
	progressFn("verify_pi", "Verifying Pi installation...")
	// Check both DAAO runtime dir and system PATH
	piFound := false
	if _, err := os.Stat(cfg.PiBinary); err == nil {
		piFound = true
		slog.Info("BootstrapRuntime: Pi CLI installed", "component", "satellite", "binary", cfg.PiBinary)
	} else if piPath, err := exec.LookPath("pi"); err == nil {
		piFound = true
		slog.Info("BootstrapRuntime: Pi CLI installed (system)", "component", "satellite", "binary", piPath)
	}
	if !piFound {
		return fmt.Errorf("Pi binary not found after install (checked %s and system PATH)", cfg.PiBinary)
	}

	// Step 8: Ensure extensions directory exists
	progressFn("extensions", "Checking extensions directory...")
	extDir := ExtensionsDir()
	if err := os.MkdirAll(extDir, 0755); err != nil {
		slog.Warn("BootstrapRuntime: could not create extensions dir",
			"component", "satellite", "dir", extDir, "err", err)
	}

	progressFn("complete", fmt.Sprintf("Runtime ready — Node.js + Pi CLI installed"))
	slog.Info("BootstrapRuntime: complete", "component", "satellite")
	return nil
}

// installPiCLI runs npm install -g @mariozechner/pi-coding-agent using the private Node.js.
func installPiCLI(ctx context.Context, cfg platformConfig) error {
	// Use the private npm to install pi globally within the DAAO prefix.
	// --prefix ensures packages go into our runtime dir, not system-wide.
	npmBin := cfg.NpmBinary

	// Fall back to system npm if the DAAO-private npm isn't found
	if _, err := os.Stat(npmBin); err != nil {
		if sysNpm, lookErr := exec.LookPath("npm"); lookErr == nil {
			npmBin = sysNpm
			slog.Info("BootstrapRuntime: using system npm as fallback", "component", "satellite", "npm", npmBin)
		}
	}

	cmd := exec.CommandContext(ctx, npmBin,
		"install", "-g", "@mariozechner/pi-coding-agent",
		"--prefix", cfg.NodeDir,
	)
	cmd.Dir = cfg.NodeDir
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("PATH=%s%c%s", filepath.Dir(cfg.NodeBinary), os.PathListSeparator, os.Getenv("PATH")),
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("npm install failed: %w\nOutput: %s", err, string(output))
	}
	slog.Info("BootstrapRuntime: npm install output", "component", "satellite", "output", string(output))
	return nil
}

// downloadFile downloads a URL to a local path.
func downloadFile(ctx context.Context, url, dest string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}

	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, resp.Body)
	return err
}

// extractTarXZ extracts a .tar.xz archive using the system tar command.
// Used on Linux where xz-compressed tarballs are the standard Node.js distribution.
func extractTarXZ(ctx context.Context, archive, dest string) error {
	cmd := exec.CommandContext(ctx, "tar", "xJf", archive, "-C", dest)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("tar xJf failed: %w\nOutput: %s", err, string(output))
	}
	return nil
}

// extractTarGZ extracts a .tar.gz archive using the system tar command.
// Used on macOS where gzip-compressed tarballs are the standard Node.js distribution.
func extractTarGZ(ctx context.Context, archive, dest string) error {
	cmd := exec.CommandContext(ctx, "tar", "xzf", archive, "-C", dest)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("tar xzf failed: %w\nOutput: %s", err, string(output))
	}
	return nil
}

// extractZip extracts a .zip archive using Go's archive/zip.
// Used on Windows where there's no standard tar command.
func extractZip(ctx context.Context, archive, dest string) error {
	r, err := zip.OpenReader(archive)
	if err != nil {
		return fmt.Errorf("failed to open zip: %w", err)
	}
	defer r.Close()

	for _, f := range r.File {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		fpath := filepath.Join(dest, f.Name)

		// Security: prevent zip slip (path traversal)
		if !strings.HasPrefix(filepath.Clean(fpath), filepath.Clean(dest)+string(os.PathSeparator)) {
			return fmt.Errorf("zip slip detected: %s", f.Name)
		}

		if f.FileInfo().IsDir() {
			os.MkdirAll(fpath, 0755)
			continue
		}

		// Create parent directories
		if err := os.MkdirAll(filepath.Dir(fpath), 0755); err != nil {
			return err
		}

		outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			outFile.Close()
			return err
		}

		_, err = io.Copy(outFile, rc)
		rc.Close()
		outFile.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

// copyDir recursively copies a directory tree. Used as fallback when
// os.Rename fails (cross-device moves in containers).
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)

		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}

		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()

		out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode())
		if err != nil {
			return err
		}
		defer out.Close()

		_, err = io.Copy(out, in)
		return err
	})
}
