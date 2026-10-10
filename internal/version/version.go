// Package version carries build-time version metadata.
package version

// These are set via -ldflags at build time.
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)
