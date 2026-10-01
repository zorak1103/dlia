package cmd

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zorak1103/dlia/internal/state"
)

func emptyStateForRetryEnv(t *testing.T) *state.State {
	t.Helper()
	st, err := state.Load(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatalf("Failed to load state: %v", err)
	}
	return st
}

func TestProcessContainers_FirstScanFailureKeepsWindow(t *testing.T) {
	withScanLLMMock(t, &fakeScanLLM{analysis: "x", failAll: true})
	_, mockDocker, containers, c := retryTestEnv(t, time.Now())
	st := emptyStateForRetryEnv(t)
	scanCfg := newTestScanConfig()
	scanCfg.dryRun = false

	before := time.Now()
	_, stats := processContainers(context.Background(), mockDocker, st, containers, c, scanCfg, 0)
	after := time.Now()

	got, ok := st.GetLastScan(retryContainerID)
	if !ok {
		t.Fatal("Expected a cursor after first-scan failure")
	}
	if got.Before(before.Add(-time.Hour)) || got.After(after.Add(-time.Hour)) {
		t.Errorf("Cursor = %v, want ~now-1h (between %v and %v)", got, before.Add(-time.Hour), after.Add(-time.Hour))
	}
	if stats.failedContainers != 1 {
		t.Errorf("Expected 1 failed container, got %d", stats.failedContainers)
	}
}

func TestProcessContainers_FirstScanFailureDryRunNoCursor(t *testing.T) {
	withScanLLMMock(t, &fakeScanLLM{analysis: "x", failAll: true})
	_, mockDocker, containers, c := retryTestEnv(t, time.Now())
	st := emptyStateForRetryEnv(t)
	scanCfg := newTestScanConfig()
	scanCfg.dryRun = true

	processContainers(context.Background(), mockDocker, st, containers, c, scanCfg, 0)

	if _, ok := st.GetLastScan(retryContainerID); ok {
		t.Error("Dry run must not create a cursor")
	}
}

func TestProcessContainers_ReadErrorNoGapNoStateChange(t *testing.T) {
	oldCursor := time.Now().Add(-48 * time.Hour)
	st, mockDocker, containers, c := retryTestEnv(t, oldCursor)
	mockDocker.logsErr = errors.New("logs error")
	scanCfg := newTestScanConfig()
	scanCfg.dryRun = false
	read := captureStdout(t)

	processContainers(context.Background(), mockDocker, st, containers, c, scanCfg, 0)
	out := read()

	if strings.Contains(out, "Skipped log gap") {
		t.Errorf("Gap must not be printed on read error:\n%s", out)
	}
	if got, _ := st.GetLastScan(retryContainerID); !got.Equal(oldCursor) {
		t.Errorf("Cursor moved to %v, want %v", got, oldCursor)
	}
}

func TestProcessContainers_EmptyWindowGapPrintedOnce(t *testing.T) {
	oldCursor := time.Now().Add(-48 * time.Hour)
	st, mockDocker, containers, c := retryTestEnv(t, oldCursor)
	mockDocker.logs = nil
	scanCfg := newTestScanConfig()
	scanCfg.dryRun = false

	before := time.Now()
	read := captureStdout(t)
	processContainers(context.Background(), mockDocker, st, containers, c, scanCfg, 0)
	out := read()
	after := time.Now()

	if n := strings.Count(out, "Skipped log gap"); n != 1 {
		t.Errorf("Expected gap printed once, got %d:\n%s", n, out)
	}
	if !strings.Contains(out, "No new logs") {
		t.Errorf("Expected 'No new logs':\n%s", out)
	}
	got, _ := st.GetLastScan(retryContainerID)
	if got.Before(before.Add(-24*time.Hour)) || got.After(after.Add(-24*time.Hour)) {
		t.Errorf("Cursor = %v, want floor (now-24h)", got)
	}

	read = captureStdout(t)
	processContainers(context.Background(), mockDocker, st, containers, c, scanCfg, 0)
	if out := read(); strings.Contains(out, "Skipped log gap") {
		t.Errorf("Second scan must not print gap:\n%s", out)
	}
}

func TestProcessContainers_EmptyWindowGapDryRunKeepsCursor(t *testing.T) {
	oldCursor := time.Now().Add(-48 * time.Hour)
	st, mockDocker, containers, c := retryTestEnv(t, oldCursor)
	mockDocker.logs = nil
	scanCfg := newTestScanConfig()
	scanCfg.dryRun = true

	processContainers(context.Background(), mockDocker, st, containers, c, scanCfg, 0)

	if got, _ := st.GetLastScan(retryContainerID); !got.Equal(oldCursor) {
		t.Errorf("Dry run moved cursor to %v, want %v", got, oldCursor)
	}
}
