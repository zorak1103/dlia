//go:build windows

package cmd

import (
	"testing"

	"golang.org/x/sys/windows"
)

// lockStateFileForTest opens the state file WITHOUT FILE_SHARE_DELETE so any
// rename-over attempt fails deterministically — the only reliable way to make
// state.Save() error in tests on this toolchain (read-only files get removed).
func lockStateFileForTest(t *testing.T, path string) {
	t.Helper()
	handle, err := windows.CreateFile(
		windows.StringToUTF16Ptr(path),
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_ATTRIBUTE_NORMAL,
		0,
	)
	if err != nil {
		t.Fatalf("could not lock state file: %v", err)
	}
	t.Cleanup(func() { _ = windows.CloseHandle(handle) })
}
