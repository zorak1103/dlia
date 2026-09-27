package cmd

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zorak1103/dlia/internal/config"
	"github.com/zorak1103/dlia/internal/docker"
	"github.com/zorak1103/dlia/internal/llm"
	"github.com/zorak1103/dlia/internal/llmlogger"
	"github.com/zorak1103/dlia/internal/state"
)

// scanRunEnv holds the paths of the temp-based test environment created by
// setupScanRunTest so tests can assert on files the scan wrote.
type scanRunEnv struct {
	tmpDir     string
	configFile string
	stateFile  string
	kbDir      string
	reportsDir string
}

// setupScanRunTest installs a valid config pointing at temp directories and
// saves/restores the cfg global, mirroring setupCleanupRunTest.
func setupScanRunTest(t *testing.T) *scanRunEnv {
	t.Helper()
	tmpDir := t.TempDir()

	reportsDir := filepath.Join(tmpDir, "reports")
	kbDir := filepath.Join(tmpDir, "knowledge_base")
	servicesDir := filepath.Join(kbDir, "services")
	for _, dir := range []string{reportsDir, servicesDir} {
		require.NoError(t, os.MkdirAll(dir, 0o750))
	}

	configFile := filepath.Join(tmpDir, "config.yaml")
	require.NoError(t, os.WriteFile(configFile, []byte("test: value"), 0o600))

	viper.Reset()
	viper.SetConfigFile(configFile)
	require.NoError(t, viper.ReadInConfig())

	originalCfg := cfg
	cfg = &config.Config{
		ConfigFilePath: configFile,
		LLM: config.LLMConfig{
			APIKey:    "test-key",
			Model:     "test-model",
			BaseURL:   "http://localhost",
			MaxTokens: 4000,
		},
		Docker: config.DockerConfig{
			SocketPath: "unix:///var/run/docker.sock",
		},
		Output: config.OutputConfig{
			ReportsDir:       reportsDir,
			KnowledgeBaseDir: kbDir,
			StateFile:        filepath.Join(tmpDir, "state.json"),
			LLMLogDir:        filepath.Join(tmpDir, "logs", "llm"),
			LLMLogEnabled:    false,
		},
		Notification: config.NotificationConfig{
			Enabled:    false,
			ShoutrrURL: "",
		},
	}
	t.Cleanup(func() {
		cfg = originalCfg
		viper.Reset()
	})

	return &scanRunEnv{
		tmpDir:     tmpDir,
		configFile: configFile,
		stateFile:  cfg.Output.StateFile,
		kbDir:      kbDir,
		reportsDir: reportsDir,
	}
}

// withScanDockerMock swaps in a MockDockerClient for the scan run tests.
func withScanDockerMock(t *testing.T, mock *MockDockerClient, factoryErr error) {
	t.Helper()
	original := newDockerClient
	newDockerClient = func(string) (docker.Client, error) {
		if factoryErr != nil {
			return nil, factoryErr
		}
		return mock, nil
	}
	t.Cleanup(func() { newDockerClient = original })
}

// fakeScanLLM implements llm.Client for runScan orchestration tests.
// failAfter > 0 makes the first failAfter Analyze calls succeed and every
// later call fail — used to separate container analysis from the
// cross-container executive summary, which share the Analyze method.
type fakeScanLLM struct {
	analysis  string
	failAfter int
	calls     int
}

func (f *fakeScanLLM) Analyze(_ context.Context, _, _, _ string) (string, *llm.TokenUsage, error) {
	f.calls++
	if f.failAfter > 0 && f.calls > f.failAfter {
		return "", nil, errors.New("llm exploded")
	}
	return f.analysis, &llm.TokenUsage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15}, nil
}

func (f *fakeScanLLM) SummarizeChunk(_ context.Context, _, _, _ string) (string, error) {
	return "chunk summary", nil
}

func (f *fakeScanLLM) ChatCompletion(_ context.Context, _ []llm.ChatMessage, _ float64, _ int) (*llm.ChatResponse, error) {
	return &llm.ChatResponse{
		Choices: []llm.Choice{{Message: llm.ChatMessage{Role: "assistant", Content: f.analysis}}},
	}, nil
}

func (f *fakeScanLLM) SetLogger(_ *llmlogger.Logger) {}

func withScanLLMMock(t *testing.T, fake *fakeScanLLM) {
	t.Helper()
	original := newLLMClient
	newLLMClient = func(string, string, string) llm.Client { return fake }
	t.Cleanup(func() { newLLMClient = original })
}

// setVerbose enables verbose mode for the test's duration.
func setVerbose(t *testing.T) {
	t.Helper()
	original := verbose
	verbose = true
	t.Cleanup(func() { verbose = original })
}

// captureStdout redirects os.Stdout into a buffer; the returned func drains it.
func captureStdout(t *testing.T) func() string {
	t.Helper()
	original := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = original })
	return func() string {
		require.NoError(t, w.Close())
		data, err := io.ReadAll(r)
		require.NoError(t, err)
		return string(data)
	}
}

// newScanRunCmd builds a fresh scan command with the flags runScan expects,
// without mutating the global scanCmd flag values.
func newScanRunCmd() *cobra.Command {
	c := &cobra.Command{Use: "scan"}
	c.Flags().Bool("dry-run", false, "")
	c.Flags().String("filter", "", "")
	c.Flags().String("lookback", "", "")
	c.Flags().Bool("llmlog", false, "")
	c.Flags().Bool("filter-stats", false, "")
	return c
}

// scanLogs is a minimal valid log batch: RFC3339 timestamps so
// GetLatestLogTime parses and state update succeeds.
var scanLogs = []docker.LogEntry{
	{Timestamp: "2026-01-01T10:00:00Z", Stream: "stdout", Message: "service started"},
	{Timestamp: "2026-01-01T10:00:05Z", Stream: "stdout", Message: "request handled"},
}

func scanContainer() docker.Container {
	return docker.Container{ID: "abc123456789def0", Name: "web", State: "running"}
}

func TestRunScan_ConfigNotLoaded(t *testing.T) {
	env := setupScanRunTest(t)
	cfg = nil // runScan re-reads the global; nil must fail validation

	err := runScan(newScanRunCmd(), []string{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "configuration not loaded")
	assert.NotEmpty(t, env.tmpDir) // keep env referenced for symmetry
}

func TestRunScan_InvalidLookback(t *testing.T) {
	setupScanRunTest(t)

	cmd := newScanRunCmd()
	require.NoError(t, cmd.Flags().Set("lookback", "bogus"))

	err := runScan(cmd, []string{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid lookback duration")
}

func TestRunScan_DockerClientFactoryFails(t *testing.T) {
	setupScanRunTest(t)
	withScanDockerMock(t, nil, errors.New("no socket"))

	err := runScan(newScanRunCmd(), []string{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create Docker client")
	assert.Contains(t, err.Error(), "no socket")
}

func TestRunScan_PingFails(t *testing.T) {
	setupScanRunTest(t)
	withScanDockerMock(t, &MockDockerClient{pingErr: errors.New("daemon down")}, nil)

	err := runScan(newScanRunCmd(), []string{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to connect to Docker daemon")
	assert.Contains(t, err.Error(), "daemon down")
}

func TestRunScan_StateLoadFails(t *testing.T) {
	env := setupScanRunTest(t)
	withScanDockerMock(t, &MockDockerClient{}, nil)
	require.NoError(t, os.WriteFile(env.stateFile, []byte("{invalid json"), 0o600))

	err := runScan(newScanRunCmd(), []string{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load state")
}

func TestRunScan_ListContainersFails(t *testing.T) {
	setupScanRunTest(t)
	withScanDockerMock(t, &MockDockerClient{listErr: errors.New("api broken")}, nil)

	err := runScan(newScanRunCmd(), []string{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to list containers")
	assert.Contains(t, err.Error(), "api broken")
}

func TestRunScan_NoContainers(t *testing.T) {
	setupScanRunTest(t)
	read := captureStdout(t)
	withScanDockerMock(t, &MockDockerClient{containers: []docker.Container{}}, nil)

	err := runScan(newScanRunCmd(), []string{})

	require.NoError(t, err)
	assert.Contains(t, read(), "No containers found")
}

func TestRunScan_HappyPath(t *testing.T) {
	env := setupScanRunTest(t)
	read := captureStdout(t)
	setVerbose(t)
	withScanDockerMock(t, &MockDockerClient{
		containers: []docker.Container{scanContainer()},
		logs:       map[string][]docker.LogEntry{scanContainer().ID: scanLogs},
	}, nil)
	withScanLLMMock(t, &fakeScanLLM{analysis: "All systems nominal"})

	err := runScan(newScanRunCmd(), []string{})

	require.NoError(t, err)
	out := read()
	assert.Contains(t, out, "Found 1 container(s) to scan")
	assert.Contains(t, out, "Scan complete")
	assert.Contains(t, out, "State: Updated")
	assert.Contains(t, out, "Loaded state with") // verbose init branch

	// State file written with the scanned container.
	st, loadErr := state.Load(env.stateFile)
	require.NoError(t, loadErr)
	_, exists := st.GetLastScan(scanContainer().ID)
	assert.True(t, exists, "container should be recorded in state")

	// Report, service KB file and global summary written.
	entries, readErr := os.ReadDir(env.reportsDir)
	require.NoError(t, readErr)
	assert.NotEmpty(t, entries, "report should be saved")
	_, statErr := os.Stat(filepath.Join(env.kbDir, "global_summary.md"))
	assert.NoError(t, statErr, "global summary should be written")
}

func TestRunScan_DryRun(t *testing.T) {
	env := setupScanRunTest(t)
	read := captureStdout(t)
	setVerbose(t)
	withScanDockerMock(t, &MockDockerClient{
		containers: []docker.Container{scanContainer()},
		logs:       map[string][]docker.LogEntry{scanContainer().ID: scanLogs},
	}, nil)

	cmd := newScanRunCmd()
	require.NoError(t, cmd.Flags().Set("dry-run", "true"))

	err := runScan(cmd, []string{})

	require.NoError(t, err)
	out := read()
	assert.Contains(t, out, "DRY RUN MODE")
	assert.Contains(t, out, "Would update state")
	assert.Contains(t, out, "State: Not modified (dry-run)")
	// Dry-run without lookback is state-tracking disabled, not lookback mode.
	assert.NotContains(t, out, "Using lookback mode")

	// Dry run must not touch state, reports or knowledge base.
	_, statErr := os.Stat(env.stateFile)
	assert.True(t, os.IsNotExist(statErr), "state file must not be created in dry-run")
	entries, readErr := os.ReadDir(env.reportsDir)
	require.NoError(t, readErr)
	assert.Empty(t, entries, "no report in dry-run")
}

func TestRunScan_LookbackMode(t *testing.T) {
	env := setupScanRunTest(t)
	read := captureStdout(t)
	setVerbose(t)
	withScanDockerMock(t, &MockDockerClient{
		containers: []docker.Container{scanContainer()},
		logs:       map[string][]docker.LogEntry{scanContainer().ID: scanLogs},
	}, nil)
	withScanLLMMock(t, &fakeScanLLM{analysis: "Lookback analysis"})

	cmd := newScanRunCmd()
	require.NoError(t, cmd.Flags().Set("lookback", "1h"))

	err := runScan(cmd, []string{})

	require.NoError(t, err)
	out := read()
	assert.Contains(t, out, "Using lookback mode, ignoring state file")
	assert.Contains(t, out, "State: Not modified (lookback mode)")

	// Lookback mode must not create or update the state file.
	_, statErr := os.Stat(env.stateFile)
	assert.True(t, os.IsNotExist(statErr), "state file must not be created in lookback mode")
}

func TestRunScan_StateSaveFails(t *testing.T) {
	// ponytail: FILE_SHARE_DELETE lock trick is Windows-only; on Linux the save
	// would succeed, so skip there (gremlins/CI sees this branch NOT COVERED).
	if runtime.GOOS != "windows" {
		t.Skip("state save failure needs the Windows testrelock file-lock trigger")
	}

	env := setupScanRunTest(t)
	withScanDockerMock(t, &MockDockerClient{
		containers: []docker.Container{scanContainer()},
		logs:       map[string][]docker.LogEntry{scanContainer().ID: scanLogs},
	}, nil)
	withScanLLMMock(t, &fakeScanLLM{analysis: "Save-failure analysis"})
	require.NoError(t, os.WriteFile(env.stateFile, []byte("{}"), 0o600))
	lockStateFileForTest(t, env.stateFile)

	err := runScan(newScanRunCmd(), []string{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to save state")
}

func TestRunScan_UpdateGlobalSummaryFails(t *testing.T) {
	env := setupScanRunTest(t)
	read := captureStdout(t)
	withScanDockerMock(t, &MockDockerClient{
		containers: []docker.Container{scanContainer()},
		logs:       map[string][]docker.LogEntry{scanContainer().ID: scanLogs},
	}, nil)
	withScanLLMMock(t, &fakeScanLLM{analysis: "KB-broken analysis"})

	// Replace the KB directory with a regular file: validation still passes
	// (os.Stat succeeds) but MkdirAll/WriteFile inside UpdateGlobalSummary fail.
	require.NoError(t, os.RemoveAll(env.kbDir))
	require.NoError(t, os.WriteFile(env.kbDir, []byte("not a dir"), 0o600))

	err := runScan(newScanRunCmd(), []string{})

	require.NoError(t, err, "global summary failure must not abort the scan")
	assert.Contains(t, read(), "Failed to update global summary")
}

func TestRunScan_ExecutiveSummaryFails(t *testing.T) {
	setupScanRunTest(t)
	read := captureStdout(t)
	withScanDockerMock(t, &MockDockerClient{
		containers: []docker.Container{scanContainer()},
		logs:       map[string][]docker.LogEntry{scanContainer().ID: scanLogs},
	}, nil)
	// First Analyze call (container analysis) succeeds, the second (executive
	// summary) fails — the run must warn and continue, not abort.
	withScanLLMMock(t, &fakeScanLLM{analysis: "Container analysis", failAfter: 1})

	err := runScan(newScanRunCmd(), []string{})

	require.NoError(t, err, "executive summary failure must not abort the scan")
	assert.Contains(t, read(), "Failed to handle executive summary")
}

func TestRunScan_NotificationSendFails(t *testing.T) {
	env := setupScanRunTest(t)
	read := captureStdout(t)
	withScanDockerMock(t, &MockDockerClient{
		containers: []docker.Container{scanContainer()},
		logs:       map[string][]docker.LogEntry{scanContainer().ID: scanLogs},
	}, nil)
	withScanLLMMock(t, &fakeScanLLM{analysis: "Notify analysis"})
	cfg.Notification.Enabled = true
	cfg.Notification.ShoutrrURL = "invalid://test"

	err := runScan(newScanRunCmd(), []string{})

	require.NoError(t, err, "notification failure must not abort the scan")
	out := read()
	assert.Contains(t, out, "Failed to handle executive summary")
	assert.Contains(t, out, "notification failed")
	// The rest of the scan still completed and state was saved.
	_, statErr := os.Stat(env.stateFile)
	assert.NoError(t, statErr, "state should still be saved")
}

func TestRunScan_NotifierInitFails(t *testing.T) {
	setupScanRunTest(t)
	read := captureStdout(t)
	withScanDockerMock(t, &MockDockerClient{
		containers: []docker.Container{scanContainer()},
		logs:       map[string][]docker.LogEntry{scanContainer().ID: scanLogs},
	}, nil)
	withScanLLMMock(t, &fakeScanLLM{analysis: "No-URL analysis"})
	cfg.Notification.Enabled = true
	cfg.Notification.ShoutrrURL = "" // enabled but no URL → notifier init fails

	err := runScan(newScanRunCmd(), []string{})

	require.NoError(t, err)
	out := read()
	assert.Contains(t, out, "Failed to handle executive summary")
	assert.Contains(t, out, "failed to initialize notifier")
}

func TestRunScan_UnparseableLogTimestamps(t *testing.T) {
	env := setupScanRunTest(t)
	read := captureStdout(t)
	setVerbose(t)
	badLogs := []docker.LogEntry{
		{Timestamp: "not-a-timestamp", Stream: "stdout", Message: "garbage time"},
		{Timestamp: "also-bad", Stream: "stdout", Message: "more garbage"},
	}
	withScanDockerMock(t, &MockDockerClient{
		containers: []docker.Container{scanContainer()},
		logs:       map[string][]docker.LogEntry{scanContainer().ID: badLogs},
	}, nil)
	withScanLLMMock(t, &fakeScanLLM{analysis: "Timestamp-error analysis"})

	err := runScan(newScanRunCmd(), []string{})

	// Unparseable timestamps must not abort the scan and must not touch state.
	require.NoError(t, err)
	assert.Contains(t, read(), "Could not parse latest timestamp")
	_, statErr := os.Stat(env.stateFile)
	assert.True(t, os.IsNotExist(statErr), "state must not be created when no timestamp parsed")
}

func TestRunScan_ReportSaveFails(t *testing.T) {
	env := setupScanRunTest(t)
	read := captureStdout(t)
	withScanDockerMock(t, &MockDockerClient{
		containers: []docker.Container{scanContainer()},
		logs:       map[string][]docker.LogEntry{scanContainer().ID: scanLogs},
	}, nil)
	withScanLLMMock(t, &fakeScanLLM{analysis: "Report-broken analysis"})

	// Replace the reports directory with a regular file: validation passes
	// (os.Stat succeeds) but writing the report fails.
	require.NoError(t, os.RemoveAll(env.reportsDir))
	require.NoError(t, os.WriteFile(env.reportsDir, []byte("not a dir"), 0o600))

	err := runScan(newScanRunCmd(), []string{})

	require.NoError(t, err, "report failure must not abort the scan")
	assert.Contains(t, read(), "Failed to save report")
	assert.NotEmpty(t, env.tmpDir)
}

func TestRunScan_NotificationDisabledEndsWithoutSending(t *testing.T) {
	env := setupScanRunTest(t)
	read := captureStdout(t)
	withScanDockerMock(t, &MockDockerClient{
		containers: []docker.Container{scanContainer()},
		logs:       map[string][]docker.LogEntry{scanContainer().ID: scanLogs},
	}, nil)
	withScanLLMMock(t, &fakeScanLLM{analysis: "Everything is running smoothly"})

	err := runScan(newScanRunCmd(), []string{})

	// Notifications disabled: no failure, and the disabled guard is exercised.
	require.NoError(t, err)
	out := read()
	assert.Contains(t, out, "Scan complete")
	assert.NotContains(t, out, "Notification sent successfully")
	assert.NotEmpty(t, env.stateFile)
}
