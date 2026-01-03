package main

import "runtime/debug"

// version is set at build time via ldflags:
//
//	go build -ldflags "-X main.ldflagsVersion=v1.0.0"
//
// For `go install`, the version is automatically read from module info
var version = getVersion()

func getVersion() string {
	// Check if version was set via ldflags (for release binaries)
	if ldflagsVersion != "" {
		return ldflagsVersion
	}

	// Fall back to module version info (for go install)
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}

	return "dev"
}

// ldflagsVersion is set at build time via -ldflags "-X main.ldflagsVersion=v1.0.0"
var ldflagsVersion string
