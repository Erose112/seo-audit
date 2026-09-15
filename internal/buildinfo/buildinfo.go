package buildinfo

import (
	"fmt"
	"runtime/debug"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// Version returns the release version string (e.g. v0.1.0 or a git describe output).
func Version() string {
	if version != "dev" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return version
}

// Commit returns the short git commit hash baked in at build time.
func Commit() string {
	return commit
}

// Date returns the build timestamp (RFC 3339) baked in at build time.
func Date() string {
	return date
}

// String returns a human-readable version line for --version output.
func String() string {
	return fmt.Sprintf("%s (%s, %s)", Version(), commit, date)
}
