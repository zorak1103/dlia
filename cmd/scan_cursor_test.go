package cmd

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zorak1103/dlia/internal/config"
	"github.com/zorak1103/dlia/internal/docker"
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

// withClock swaps nowFunc for a controllable clock so tests never depend on wall-clock progress.
func withClock(t *testing.T, start time.Time) *time.Time {
	t.Helper()
	now := start
	orig := nowFunc
	nowFunc = func() time.Time { return now }
	t.Cleanup(func() { nowFunc = orig })
	return &now
}

func scanEmpty(t *testing.T, st *state.State, mockDocker *MockDockerClient, containers []docker.Container, c *config.Config, scanCfg *scanConfig, lookback time.Duration) string {
	t.Helper()
	read := captureStdout(t)
	processContainers(context.Background(), mockDocker, st, containers, c, scanCfg, lookback)
	return read()
}

func TestProcessContainers_EmptyReadGapPrintedOnceAndCursorFollowsReadTime(t *testing.T) {
	t1 := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	clock := withClock(t, t1)
	st, mockDocker, containers, c := retryTestEnv(t, t1.Add(-48*time.Hour))
	mockDocker.logs = nil
	scanCfg := newTestScanConfig()
	scanCfg.dryRun = false

	out := scanEmpty(t, st, mockDocker, containers, c, scanCfg, 0)
	if n := strings.Count(out, "Skipped log gap"); n != 1 {
		t.Errorf("Expected gap printed once, got %d:\n%s", n, out)
	}
	if !strings.Contains(out, "No new logs") {
		t.Errorf("Expected 'No new logs':\n%s", out)
	}
	if got, _ := st.GetLastScan(retryContainerID); !got.Equal(t1) {
		t.Errorf("Cursor = %v, want %v", got, t1)
	}

	t2 := t1.Add(time.Hour)
	*clock = t2
	out = scanEmpty(t, st, mockDocker, containers, c, scanCfg, 0)
	if strings.Contains(out, "Skipped log gap") {
		t.Errorf("Second scan must not print gap:\n%s", out)
	}
	if got, _ := st.GetLastScan(retryContainerID); !got.Equal(t2) {
		t.Errorf("Cursor = %v, want %v", got, t2)
	}
}

func TestProcessContainers_EmptyReadWithinWindowAdvancesCursor(t *testing.T) {
	t1 := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	withClock(t, t1)
	st, mockDocker, containers, c := retryTestEnv(t, t1.Add(-2*time.Hour))
	mockDocker.logs = nil
	scanCfg := newTestScanConfig()
	scanCfg.dryRun = false

	out := scanEmpty(t, st, mockDocker, containers, c, scanCfg, 0)

	if strings.Contains(out, "Skipped log gap") {
		t.Errorf("No gap expected within window:\n%s", out)
	}
	if got, _ := st.GetLastScan(retryContainerID); !got.Equal(t1) {
		t.Errorf("Cursor = %v, want %v", got, t1)
	}
}

func TestProcessContainers_EmptyReadWithoutCursorSetsCursor(t *testing.T) {
	t1 := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	withClock(t, t1)
	_, mockDocker, containers, c := retryTestEnv(t, t1)
	mockDocker.logs = nil
	st := emptyStateForRetryEnv(t)
	scanCfg := newTestScanConfig()
	scanCfg.dryRun = false

	out := scanEmpty(t, st, mockDocker, containers, c, scanCfg, 0)

	if strings.Contains(out, "Skipped log gap") {
		t.Errorf("No gap expected without cursor:\n%s", out)
	}
	if got, ok := st.GetLastScan(retryContainerID); !ok || !got.Equal(t1) {
		t.Errorf("Cursor = %v (exists=%v), want %v", got, ok, t1)
	}
}

func TestProcessContainers_EmptyReadDryRunAndLookbackKeepState(t *testing.T) {
	t1 := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	oldCursor := t1.Add(-48 * time.Hour)
	cases := []struct {
		name     string
		dryRun   bool
		lookback time.Duration
	}{
		{"dry-run", true, 0},
		{"lookback", false, 2 * time.Hour},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			withClock(t, t1)
			st, mockDocker, containers, c := retryTestEnv(t, oldCursor)
			mockDocker.logs = nil
			scanCfg := newTestScanConfig()
			scanCfg.dryRun = tc.dryRun

			scanEmpty(t, st, mockDocker, containers, c, scanCfg, tc.lookback)

			if got, _ := st.GetLastScan(retryContainerID); !got.Equal(oldCursor) {
				t.Errorf("Cursor moved to %v, want %v", got, oldCursor)
			}
		})
	}
}
