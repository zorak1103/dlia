//go:build !windows

package cmd

import "testing"

// lockStateFileForTest is a no-op outside Windows; tests that rely on it skip there.
func lockStateFileForTest(_ *testing.T, _ string) {}
