package main

import "fmt"

// Version is set at build time via -ldflags="-X main.Version=..."
// Defaults to "dev" for local builds without explicit version injection.
var Version = "dev"

// versionCommand prints the satellite daemon version.
func versionCommand() {
	fmt.Printf("daao version %s\n", Version)
}
