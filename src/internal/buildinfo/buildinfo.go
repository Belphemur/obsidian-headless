// Package buildinfo holds compile-time build metadata injected via ldflags.
package buildinfo

// Build-time variables (injected via -X ldflags).
var (
	Version = "dev"
	Commit  = "unknown"
	Date    = "unknown"
)
