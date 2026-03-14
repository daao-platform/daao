package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// updateCommand checks for and applies satellite binary updates.
// It downloads the latest binary from the Nexus cockpit's /releases/ endpoint,
// performs a rename-swap (safe on Windows since you can't overwrite a running exe),
// and keeps the old binary as a .bak for rollback.
func updateCommand(args []string) error {
	nexusURL := os.Getenv("NEXUS_URL")
	if nexusURL == "" {
		nexusURL = "http://localhost:8081"
	}
	nexusURL = strings.TrimRight(nexusURL, "/")

	// Check --force flag
	force := false
	for _, a := range args {
		if a == "--force" || a == "-f" {
			force = true
		}
	}

	log.Printf("Current version: %s", Version)
	log.Printf("Checking for updates at %s ...", nexusURL)

	// Step 1: Check latest version
	latestVersion, err := fetchLatestVersion(nexusURL)
	if err != nil {
		return fmt.Errorf("failed to check for updates: %w", err)
	}

	log.Printf("Latest version: %s", latestVersion)

	if latestVersion == Version && !force {
		fmt.Println("Already up to date.")
		return nil
	}

	if latestVersion == Version && force {
		log.Println("Force flag set — re-downloading current version")
	}

	// Step 2: Determine binary name for this platform
	binaryName := fmt.Sprintf("daao-%s-%s", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}

	downloadURL := fmt.Sprintf("%s/releases/%s", nexusURL, binaryName)
	log.Printf("Downloading %s ...", downloadURL)

	// Step 3: Download to temp file
	tmpPath, size, err := downloadBinary(downloadURL)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer os.Remove(tmpPath) // Clean up on failure

	// Sanity check — real binaries are > 1MB
	if size < 1024*1024 {
		return fmt.Errorf("downloaded file too small (%d bytes) — may be an HTML error page", size)
	}

	log.Printf("Downloaded %s (%.1f MB)", binaryName, float64(size)/(1024*1024))

	// Step 4: Locate current executable
	currentExe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot determine executable path: %w", err)
	}
	currentExe, err = filepath.EvalSymlinks(currentExe)
	if err != nil {
		return fmt.Errorf("cannot resolve executable path: %w", err)
	}

	// Step 5: Rename-swap (safe on all platforms including Windows)
	bakPath := currentExe + ".bak"

	// Remove old backup if it exists
	_ = os.Remove(bakPath)

	// Rename current → .bak
	if err := os.Rename(currentExe, bakPath); err != nil {
		return fmt.Errorf("failed to create backup of current binary: %w", err)
	}

	// Rename temp → current
	if err := os.Rename(tmpPath, currentExe); err != nil {
		// Rollback: restore backup
		_ = os.Rename(bakPath, currentExe)
		return fmt.Errorf("failed to install new binary (rolled back): %w", err)
	}

	// Make executable on Unix
	if runtime.GOOS != "windows" {
		_ = os.Chmod(currentExe, 0755)
	}

	fmt.Printf("Updated from %s to %s\n", Version, latestVersion)
	fmt.Printf("Old binary saved as %s\n", bakPath)
	fmt.Println("Restart the daemon to apply the update:")
	if runtime.GOOS == "windows" {
		fmt.Println("  sc stop DAAOSatellite && sc start DAAOSatellite")
	} else {
		fmt.Println("  sudo systemctl restart daao-satellite")
	}

	return nil
}

// rollbackCommand restores the previous binary from .bak
func rollbackCommand() error {
	currentExe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot determine executable path: %w", err)
	}
	currentExe, err = filepath.EvalSymlinks(currentExe)
	if err != nil {
		return fmt.Errorf("cannot resolve executable path: %w", err)
	}

	bakPath := currentExe + ".bak"
	if _, err := os.Stat(bakPath); os.IsNotExist(err) {
		return fmt.Errorf("no backup found at %s — nothing to roll back", bakPath)
	}

	// Swap: current → .new, .bak → current, .new → .bak
	newPath := currentExe + ".new"
	_ = os.Remove(newPath)

	if err := os.Rename(currentExe, newPath); err != nil {
		return fmt.Errorf("failed to move current binary: %w", err)
	}

	if err := os.Rename(bakPath, currentExe); err != nil {
		// Rollback the rollback
		_ = os.Rename(newPath, currentExe)
		return fmt.Errorf("failed to restore backup: %w", err)
	}

	// The .new file becomes the new backup
	_ = os.Rename(newPath, bakPath)

	fmt.Println("Rolled back to previous version.")
	fmt.Println("Restart the daemon to apply.")
	return nil
}

// fetchLatestVersion reads /releases/version.txt from the cockpit.
func fetchLatestVersion(nexusURL string) (string, error) {
	url := nexusURL + "/releases/version.txt"

	tlsCfg, err := nexusTLSConfig()
	if err != nil {
		return "", fmt.Errorf("TLS setup failed: %w", err)
	}
	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: tlsCfg,
		},
	}

	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("version endpoint returned HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(body)), nil
}

// downloadBinary downloads a binary to a temp file and returns the path and size.
func downloadBinary(url string) (path string, size int64, err error) {
	tlsCfg, err := nexusTLSConfig()
	if err != nil {
		return "", 0, fmt.Errorf("TLS setup failed: %w", err)
	}
	client := &http.Client{
		Timeout: 5 * time.Minute, // Large binary download
		Transport: &http.Transport{
			TLSClientConfig: tlsCfg,
		},
	}

	resp, err := client.Get(url)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", 0, fmt.Errorf("download returned HTTP %d", resp.StatusCode)
	}

	tmpFile, err := os.CreateTemp("", "daao-update-*")
	if err != nil {
		return "", 0, fmt.Errorf("failed to create temp file: %w", err)
	}

	n, err := io.Copy(tmpFile, resp.Body)
	if err != nil {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		return "", 0, fmt.Errorf("failed to save download: %w", err)
	}

	tmpFile.Close()
	return tmpFile.Name(), n, nil
}

// updateInfo holds update metadata received from Nexus via gRPC.
type updateInfo struct {
	LatestVersion string `json:"latest_version"`
	DownloadURL   string `json:"download_url"`
	Force         bool   `json:"force"`
}

// saveUpdateNotification saves an update notification from Nexus for later processing.
func saveUpdateNotification(info *updateInfo) error {
	homeDir, _ := os.UserHomeDir()
	configDir := filepath.Join(homeDir, ".config", "daao")
	_ = os.MkdirAll(configDir, 0700)

	data, _ := json.MarshalIndent(info, "", "  ")
	return os.WriteFile(filepath.Join(configDir, "update-available.json"), data, 0600)
}
