// Package version contains version information.
package version

// Version information for DLIA
var (
	Version   = "dev"
	BuildDate = "unknown"
	GitCommit = "unknown"
)

// GetVersion returns the full version string
func GetVersion() string {
	return Version
}

// GetFullVersion returns version with build metadata
func GetFullVersion() string {
	return Version + " (build: " + BuildDate + ", commit: " + GitCommit + ")"
}
