// Package version contains version information.
package version

import (
	"strings"
	"testing"
)

func TestGetVersion(t *testing.T) {
	// Save original values
	originalVersion := Version
	defer func() { Version = originalVersion }()

	tests := []struct {
		name    string
		version string
		want    string
	}{
		{
			name:    "default dev version",
			version: "dev",
			want:    "dev",
		},
		{
			name:    "semantic version",
			version: "1.0.0",
			want:    "1.0.0",
		},
		{
			name:    "version with v prefix",
			version: "v2.1.3",
			want:    "v2.1.3",
		},
		{
			name:    "pre-release version",
			version: "1.0.0-beta.1",
			want:    "1.0.0-beta.1",
		},
		{
			name:    "empty version",
			version: "",
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			Version = tt.version
			if got := GetVersion(); got != tt.want {
				t.Errorf("GetVersion() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGetFullVersion(t *testing.T) {
	// Save original values
	originalVersion := Version
	originalBuildDate := BuildDate
	originalGitCommit := GitCommit
	defer func() {
		Version = originalVersion
		BuildDate = originalBuildDate
		GitCommit = originalGitCommit
	}()

	tests := []struct {
		name      string
		version   string
		buildDate string
		gitCommit string
		want      string
	}{
		{
			name:      "default values",
			version:   "dev",
			buildDate: "unknown",
			gitCommit: "unknown",
			want:      "dev (build: unknown, commit: unknown)",
		},
		{
			name:      "production release",
			version:   "1.0.0",
			buildDate: "2024-01-15T10:30:00Z",
			gitCommit: "abc123def",
			want:      "1.0.0 (build: 2024-01-15T10:30:00Z, commit: abc123def)",
		},
		{
			name:      "full git commit hash",
			version:   "2.0.0",
			buildDate: "2024-06-20",
			gitCommit: "abc123def456789012345678901234567890abcd",
			want:      "2.0.0 (build: 2024-06-20, commit: abc123def456789012345678901234567890abcd)",
		},
		{
			name:      "empty values",
			version:   "",
			buildDate: "",
			gitCommit: "",
			want:      " (build: , commit: )",
		},
		{
			name:      "pre-release with metadata",
			version:   "1.0.0-rc.1",
			buildDate: "2024-03-10",
			gitCommit: "deadbeef",
			want:      "1.0.0-rc.1 (build: 2024-03-10, commit: deadbeef)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			Version = tt.version
			BuildDate = tt.buildDate
			GitCommit = tt.gitCommit

			got := GetFullVersion()
			if got != tt.want {
				t.Errorf("GetFullVersion() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGetFullVersion_ContainsComponents(t *testing.T) {
	// Save original values
	originalVersion := Version
	originalBuildDate := BuildDate
	originalGitCommit := GitCommit
	defer func() {
		Version = originalVersion
		BuildDate = originalBuildDate
		GitCommit = originalGitCommit
	}()

	Version = "3.2.1"
	BuildDate = "2024-12-01"
	GitCommit = "1234567"

	fullVersion := GetFullVersion()

	// Verify all components are present
	if !strings.Contains(fullVersion, Version) {
		t.Errorf("GetFullVersion() = %q, should contain version %q", fullVersion, Version)
	}

	if !strings.Contains(fullVersion, BuildDate) {
		t.Errorf("GetFullVersion() = %q, should contain build date %q", fullVersion, BuildDate)
	}

	if !strings.Contains(fullVersion, GitCommit) {
		t.Errorf("GetFullVersion() = %q, should contain git commit %q", fullVersion, GitCommit)
	}

	if !strings.Contains(fullVersion, "build:") {
		t.Error("GetFullVersion() should contain 'build:' label")
	}

	if !strings.Contains(fullVersion, "commit:") {
		t.Error("GetFullVersion() should contain 'commit:' label")
	}
}

func TestGetVersion_ReturnsVersionVariable(t *testing.T) {
	// Save original value
	originalVersion := Version
	defer func() { Version = originalVersion }()

	testVersion := "test-version-12345"
	Version = testVersion

	if got := GetVersion(); got != testVersion {
		t.Errorf("GetVersion() = %q, should return Version variable value %q", got, testVersion)
	}
}

func TestDefaultValues(t *testing.T) {
	// Test that the package initializes with expected default values
	// These tests verify the hardcoded defaults in the package

	// Note: These may be overridden during build with ldflags,
	// so we only check if they're non-empty or match expected dev defaults
	if Version == "" && BuildDate == "" && GitCommit == "" {
		t.Error("At least one version variable should have a default value")
	}
}

func TestGetFullVersion_Format(t *testing.T) {
	// Save original values
	originalVersion := Version
	originalBuildDate := BuildDate
	originalGitCommit := GitCommit
	defer func() {
		Version = originalVersion
		BuildDate = originalBuildDate
		GitCommit = originalGitCommit
	}()

	Version = "1.0.0"
	BuildDate = "date"
	GitCommit = "commit"

	got := GetFullVersion()

	// Verify format: "VERSION (build: DATE, commit: COMMIT)"
	if !strings.HasPrefix(got, "1.0.0 ") {
		t.Errorf("GetFullVersion() = %q, should start with version", got)
	}

	if !strings.HasSuffix(got, ")") {
		t.Errorf("GetFullVersion() = %q, should end with closing parenthesis", got)
	}

	if !strings.Contains(got, "(") {
		t.Errorf("GetFullVersion() = %q, should contain opening parenthesis", got)
	}
}
