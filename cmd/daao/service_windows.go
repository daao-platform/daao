//go:build windows

package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/daao/nexus/internal/satellite"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/debug"
	"golang.org/x/sys/windows/svc/eventlog"
)

const svcName = "DAAOSatellite"

// setupLogFile configures log output to write to both a file and stderr.
// The log file is placed next to the executable at satellite.log.
// Returns a closer function that should be deferred.
func setupLogFile() func() {
	exePath, err := os.Executable()
	if err != nil {
		return func() {}
	}
	logPath := filepath.Join(filepath.Dir(exePath), "satellite.log")

	// Rotate: if existing log is > 5 MB, rename to .log.old
	if info, err := os.Stat(logPath); err == nil && info.Size() > 5*1024*1024 {
		os.Rename(logPath, logPath+".old")
	}

	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		log.Printf("Failed to open log file %s: %v", logPath, err)
		return func() {}
	}

	// Write to both stderr and the file
	log.SetOutput(io.MultiWriter(os.Stderr, f))
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Printf("=== Satellite log started at %s ===", time.Now().Format(time.RFC3339))

	return func() { f.Close() }
}

// loadDaemonEnv loads environment variables from daemon.env file next to the binary.
// Essential for Windows Service mode where LocalSystem has no user environment.
func loadDaemonEnv() {
	exePath, err := os.Executable()
	if err != nil {
		return
	}
	envFile := filepath.Join(filepath.Dir(exePath), "daemon.env")
	f, err := os.Open(envFile)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			val := strings.TrimSpace(parts[1])
			os.Setenv(key, val)
		}
	}
}

// buildDaemonConfig creates a DaemonConfig from the satellite key pair and environment
func buildDaemonConfig() (DaemonConfig, error) {
	// Load satellite key pair
	publicKeyPath, privateKeyPath, err := satellite.GetDefaultKeyPaths()
	if err != nil {
		return DaemonConfig{}, fmt.Errorf("failed to get key paths: %w", err)
	}

	keyPair, err := satellite.LoadKeyPair(publicKeyPath, privateKeyPath)
	if err != nil {
		return DaemonConfig{}, fmt.Errorf("failed to load key pair: %w", err)
	}

	// Determine satellite ID — prefer the real UUID saved by 'daao login'
	satelliteID := keyPair.Fingerprint[:16] // fallback: short fingerprint prefix
	if reg := loadLocalRegistration(); reg != nil && isValidUUID(reg.ID) {
		satelliteID = reg.ID
	}

	// Determine gRPC address: NEXUS_GRPC_ADDR > derived from NEXUS_URL > default
	grpcAddr := os.Getenv("NEXUS_GRPC_ADDR")
	if grpcAddr == "" {
		nexusURL := os.Getenv("NEXUS_URL")
		if nexusURL != "" {
			host := nexusURL
			for _, prefix := range []string{"https://", "http://"} {
				host = strings.TrimPrefix(host, prefix)
			}
			if idx := strings.Index(host, "/"); idx > 0 {
				host = host[:idx]
			}
			if idx := strings.Index(host, ":"); idx > 0 {
				host = host[:idx]
			}
			grpcAddr = host + ":8444"
		} else {
			grpcAddr = "localhost:8444"
		}
	}

	return DaemonConfig{
		NexusAddr:   grpcAddr,
		SatelliteID: satelliteID,
		Fingerprint: keyPair.Fingerprint,
		PrivateKey:  privateKeyPath,
	}, nil
}

// daaoService implements svc.Handler for Windows Service Control Manager
type daaoService struct{}

// Execute is the main service loop called by the Windows SCM
func (s *daaoService) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (svcSpecificEC bool, errno uint32) {
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown

	changes <- svc.Status{State: svc.StartPending}

	// Load environment from daemon.env
	loadDaemonEnv()

	// Set up file logging (must be after loadDaemonEnv for correct paths)
	closeLog := setupLogFile()
	defer closeLog()

	config, err := buildDaemonConfig()
	if err != nil {
		log.Printf("Failed to build daemon config: %v", err)
		changes <- svc.Status{State: svc.StopPending}
		return true, 1
	}

	// Create and start the real Daemon (gRPC connection to Nexus)
	daemon := NewDaemon(config)
	if err := daemon.Start(); err != nil {
		log.Printf("Failed to start daemon: %v", err)
		changes <- svc.Status{State: svc.StopPending}
		return true, 2
	}

	changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}
	log.Printf("DAAO Satellite Service running (gRPC: %s, ID: %s)", config.NexusAddr, config.SatelliteID)

	// Main service loop
	for {
		select {
		case c := <-r:
			switch c.Cmd {
			case svc.Stop, svc.Shutdown:
				log.Println("Service stop requested")
				changes <- svc.Status{State: svc.StopPending}
				daemon.Stop()
				time.Sleep(1 * time.Second)
				return false, 0
			case svc.Interrogate:
				changes <- c.CurrentStatus
			default:
				log.Printf("Unexpected service control request #%d", c)
			}
		}
	}
}

// runAsService runs the daemon under the Windows SCM
func runAsService() error {
	elog, err := eventlog.Open(svcName)
	if err == nil {
		defer elog.Close()
		elog.Info(1, fmt.Sprintf("%s service starting", svcName))
	}

	err = svc.Run(svcName, &daaoService{})
	if err != nil {
		return fmt.Errorf("service failed: %w", err)
	}
	return nil
}

// runAsDebugService runs the service in debug mode
func runAsDebugService() error {
	elog := debug.New(svcName)
	defer elog.Close()
	elog.Info(1, fmt.Sprintf("starting %s in debug mode", svcName))

	err := debug.Run(svcName, &daaoService{})
	if err != nil {
		return fmt.Errorf("service debug failed: %w", err)
	}
	return nil
}

// isWindowsService detects if running under the SCM
func isWindowsService() bool {
	isSvc, err := svc.IsWindowsService()
	if err != nil {
		return false
	}
	return isSvc
}

// runStart handles the "start" command
func runStart(ctx context.Context, args []string) error {
	// Running as Windows Service — use SCM handler
	if isWindowsService() {
		return runAsService()
	}

	// Debug mode
	for _, a := range args {
		if strings.EqualFold(a, "--debug") || strings.EqualFold(a, "-debug") {
			return runAsDebugService()
		}
	}

	// Foreground mode — set up file logging alongside stderr
	loadDaemonEnv()
	closeLog := setupLogFile()
	defer closeLog()

	// Foreground mode — use the real Daemon with gRPC
	config, err := buildDaemonConfig()
	if err != nil {
		return fmt.Errorf("failed to build config: %w", err)
	}

	daemon := NewDaemon(config)
	if err := daemon.Start(); err != nil {
		return fmt.Errorf("failed to start daemon: %w", err)
	}

	log.Printf("Satellite running (gRPC: %s, ID: %s). Press Ctrl+C to exit.", config.NexusAddr, config.SatelliteID)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	select {
	case <-ctx.Done():
		daemon.Stop()
	case <-sigCh:
		log.Println("Shutting down satellite...")
		daemon.Stop()
	}
	return nil
}
