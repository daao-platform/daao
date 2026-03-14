//go:build !windows

package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/daao/nexus/internal/satellite"
)

// buildDaemonConfig creates a DaemonConfig from the satellite key pair and environment
func buildDaemonConfig() (DaemonConfig, error) {
	publicKeyPath, privateKeyPath, err := satellite.GetDefaultKeyPaths()
	if err != nil {
		return DaemonConfig{}, fmt.Errorf("failed to get key paths: %w", err)
	}

	keyPair, err := satellite.LoadKeyPair(publicKeyPath, privateKeyPath)
	if err != nil {
		return DaemonConfig{}, fmt.Errorf("failed to load key pair: %w", err)
	}

	nexusURL := os.Getenv("NEXUS_URL")
	grpcAddr := os.Getenv("NEXUS_GRPC_ADDR")

	if grpcAddr == "" {
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
		SatelliteID: keyPair.Fingerprint[:16],
		Fingerprint: keyPair.Fingerprint,
		PrivateKey:  privateKeyPath,
	}, nil
}

// runStart handles the "start" command on non-Windows platforms
func runStart(ctx context.Context, args []string) error {
	config, err := buildDaemonConfig()
	if err != nil {
		return fmt.Errorf("failed to build config: %w", err)
	}

	daemon := NewDaemon(config)
	if err := daemon.Start(); err != nil {
		return fmt.Errorf("failed to start daemon: %w", err)
	}

	log.Printf("Satellite running (gRPC: %s, ID: %s). Press Ctrl+C to exit.", config.NexusAddr, config.SatelliteID)

	select {
	case <-ctx.Done():
		daemon.Stop()
	case <-make(chan struct{}):
	}
	return nil
}
