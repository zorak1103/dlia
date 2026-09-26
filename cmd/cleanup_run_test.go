package cmd

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zorak1103/dlia/internal/config"
	"github.com/zorak1103/dlia/internal/docker"
	"github.com/zorak1103/dlia/internal/state"
)

// withDockerMock swaps in a mock Docker client for the duration of the test.
func withDockerMock(t *testing.T, mock *testMockDockerClient, factoryErr error) {
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

// setupCleanupRunTest loads a valid config (with real temp directories) and
// wires output capture for the cleanup subcommands.
func setupCleanupRunTest(t *testing.T) *bytes.Buffer {
	t.Helper()
	tmpDir := t.TempDir()

	reportsDir := filepath.Join(tmpDir, "reports")
	kbDir := filepath.Join(tmpDir, "knowledge_base")
	servicesDir := filepath.Join(kbDir, "services")
	llmDir := filepath.Join(tmpDir, "logs", "llm")
	for _, dir := range []string{reportsDir, servicesDir} {
		require.NoError(t, os.MkdirAll(dir, 0750))
	}

	configFile := filepath.Join(tmpDir, "config.yaml")
	require.NoError(t, os.WriteFile(configFile, []byte("test: value"), 0600))

	viper.Reset()
	viper.SetConfigFile(configFile)
	require.NoError(t, viper.ReadInConfig())

	originalCfg := cfg
	cfg = &config.Config{
		ConfigFilePath: configFile,
		Output: config.OutputConfig{
			ReportsDir:       reportsDir,
			KnowledgeBaseDir: kbDir,
			StateFile:        filepath.Join(tmpDir, "state.json"),
			LLMLogDir:        llmDir,
			LLMLogEnabled:    false,
		},
		Docker: config.DockerConfig{
			SocketPath: "unix:///var/run/docker.sock",
		},
	}
	t.Cleanup(func() {
		cfg = originalCfg
		viper.Reset()
		cleanupDryRun = false
		cleanupForce = false
	})

	out := &bytes.Buffer{}
	cleanupListCmd.SetOut(out)
	cleanupListCmd.SetErr(out)
	cleanupExecuteCmd.SetOut(out)
	cleanupExecuteCmd.SetErr(out)
	t.Cleanup(func() {
		cleanupListCmd.SetOut(nil)
		cleanupListCmd.SetErr(nil)
		cleanupExecuteCmd.SetOut(nil)
		cleanupExecuteCmd.SetErr(nil)
	})
	return out
}

func TestCleanupListCmd_ClientFactoryFails(t *testing.T) {
	out := setupCleanupRunTest(t)
	withDockerMock(t, nil, errors.New("no socket"))

	err := cleanupListCmd.RunE(cleanupListCmd, []string{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create Docker client")
	assert.Contains(t, err.Error(), "no socket")
	assert.Empty(t, out.String())
}

func TestCleanupListCmd_PingFails(t *testing.T) {
	out := setupCleanupRunTest(t)
	withDockerMock(t, &testMockDockerClient{pingErr: errors.New("daemon down")}, nil)

	err := cleanupListCmd.RunE(cleanupListCmd, []string{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to connect to Docker")
	assert.Contains(t, err.Error(), "daemon down")
	assert.Empty(t, out.String())
}

func TestCleanupListCmd_ListContainersFails(t *testing.T) {
	out := setupCleanupRunTest(t)
	withDockerMock(t, &testMockDockerClient{listErr: errors.New("api broken")}, nil)

	err := cleanupListCmd.RunE(cleanupListCmd, []string{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to find obsolete containers")
	assert.Contains(t, err.Error(), "api broken")
	assert.Empty(t, out.String())
}

func TestCleanupListCmd_NoObsolete(t *testing.T) {
	out := setupCleanupRunTest(t)
	withDockerMock(t, &testMockDockerClient{containers: []docker.Container{}}, nil)

	err := cleanupListCmd.RunE(cleanupListCmd, []string{})

	require.NoError(t, err)
	assert.Contains(t, out.String(), "No obsolete container data found")
	assert.Contains(t, out.String(), "All storage is clean!")
}

func TestCleanupListCmd_ShowsObsoleteTable(t *testing.T) {
	out := setupCleanupRunTest(t)

	// State: two dead containers.
	// - removed13chars (13-char ID): truncated to 12
	// - 12charID123 (exactly 12 chars, unnamed): NOT truncated, "-" for name
	stateJSON := `{
		"containers": {
			"removed13chars": {
				"name": "dead-app",
				"last_scan": "2024-01-01T00:00:00Z",
				"log_cursor": "2024-01-01T00:00:00Z"
			},
			"12charID123": {
				"name": "",
				"last_scan": "2024-01-01T00:00:00Z",
				"log_cursor": "2024-01-01T00:00:00Z"
			}
		},
		"last_updated": "2024-01-01T00:00:00Z"
	}`
	stateFile := cfg.Output.StateFile
	require.NoError(t, os.WriteFile(stateFile, []byte(stateJSON), 0600))

	// Storage entries only for "lonesome" (no state entry, no Docker container):
	// surfaces as an orphaned row with all storage checkmarks.
	require.NoError(t, os.WriteFile(filepath.Join(cfg.Output.KnowledgeBaseDir, "services", "lonesome.md"), []byte("# x"), 0600))
	require.NoError(t, os.MkdirAll(filepath.Join(cfg.Output.ReportsDir, "lonesome"), 0750))
	cfg.Output.LLMLogEnabled = true
	require.NoError(t, os.MkdirAll(filepath.Join(cfg.Output.LLMLogDir, "lonesome"), 0750))

	withDockerMock(t, &testMockDockerClient{containers: []docker.Container{}}, nil)

	err := cleanupListCmd.RunE(cleanupListCmd, []string{})

	require.NoError(t, err)
	assert.Contains(t, out.String(), "Found 3 obsolete container(s)")
	// 13-char ID truncated to 12; 12-char ID untouched
	assert.Contains(t, out.String(), "removed13cha")
	assert.NotContains(t, out.String(), "removed13chars")
	assert.Contains(t, out.String(), "12charID123")
	// Orphaned storage entry shows name and is not truncated
	assert.Contains(t, out.String(), "orphaned-lonesome")
	assert.Contains(t, out.String(), "lonesome")
	// Unnamed container rendered as "-"
	assert.Regexp(t, `12charID123\s+-\s`, out.String())
}

func TestCleanupExecuteCmd_ClientFactoryFails(t *testing.T) {
	setupCleanupRunTest(t)
	withDockerMock(t, nil, errors.New("no socket"))

	err := cleanupExecuteCmd.RunE(cleanupExecuteCmd, []string{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create Docker client")
	assert.Contains(t, err.Error(), "no socket")
}

func TestCleanupExecuteCmd_PingFails(t *testing.T) {
	setupCleanupRunTest(t)
	withDockerMock(t, &testMockDockerClient{pingErr: errors.New("daemon down")}, nil)

	err := cleanupExecuteCmd.RunE(cleanupExecuteCmd, []string{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to connect to Docker")
}

func TestCleanupExecuteCmd_ListContainersFails(t *testing.T) {
	setupCleanupRunTest(t)
	withDockerMock(t, &testMockDockerClient{listErr: errors.New("api broken")}, nil)

	err := cleanupExecuteCmd.RunE(cleanupExecuteCmd, []string{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to find obsolete containers")
}

func TestCleanupExecuteCmd_NoObsolete(t *testing.T) {
	out := setupCleanupRunTest(t)
	withDockerMock(t, &testMockDockerClient{containers: []docker.Container{}}, nil)

	err := cleanupExecuteCmd.RunE(cleanupExecuteCmd, []string{})

	require.NoError(t, err)
	assert.Contains(t, out.String(), "No obsolete container data found")
	assert.NotContains(t, out.String(), "Proceed with cleanup")
}

func TestCleanupExecuteCmd_DryRun(t *testing.T) {
	out := setupCleanupRunTest(t)

	stateFile := cfg.Output.StateFile
	require.NoError(t, os.WriteFile(stateFile, []byte(`{
		"containers": {"dead1": {"name": "old-app"}},
		"last_updated": "2024-01-01T00:00:00Z"
	}`), 0600))
	kbFile := filepath.Join(cfg.Output.KnowledgeBaseDir, "services", "old-app.md")
	require.NoError(t, os.WriteFile(kbFile, []byte("# x"), 0600))

	cleanupDryRun = true
	withDockerMock(t, &testMockDockerClient{containers: []docker.Container{}}, nil)

	err := cleanupExecuteCmd.RunE(cleanupExecuteCmd, []string{})

	require.NoError(t, err)
	assert.Contains(t, out.String(), "DRY RUN - No changes made")
	// Nothing deleted
	_, statErr := os.Stat(kbFile)
	assert.NoError(t, statErr, "dry run must not delete files")
}

func TestCleanupExecuteCmd_PromptVariants(t *testing.T) {
	tests := []struct {
		name     string
		in       string
		setIn    bool
		expected string
	}{
		{name: "declined", in: "n\n", setIn: true, expected: "Cleanup canceled"},
		{name: "stdin closed (scan error counts as no)", setIn: false, expected: "Cleanup canceled"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := setupCleanupRunTest(t)

			require.NoError(t, os.WriteFile(cfg.Output.StateFile, []byte(`{
				"containers": {"dead1": {"name": "old-app"}},
				"last_updated": "2024-01-01T00:00:00Z"
			}`), 0600))
			kbFile := filepath.Join(cfg.Output.KnowledgeBaseDir, "services", "old-app.md")
			require.NoError(t, os.WriteFile(kbFile, []byte("# x"), 0600))

			if tt.setIn {
				cleanupExecuteCmd.SetIn(strings.NewReader(tt.in))
			} else {
				cleanupExecuteCmd.SetIn(strings.NewReader(""))
			}
			t.Cleanup(func() { cleanupExecuteCmd.SetIn(nil) })
			withDockerMock(t, &testMockDockerClient{containers: []docker.Container{}}, nil)

			err := cleanupExecuteCmd.RunE(cleanupExecuteCmd, []string{})

			require.NoError(t, err)
			assert.Contains(t, out.String(), tt.expected)
			_, statErr := os.Stat(kbFile)
			assert.NoError(t, statErr, "declined cleanup must not delete files")
		})
	}
}

func TestCleanupExecuteCmd_ForceSuccess(t *testing.T) {
	out := setupCleanupRunTest(t)

	// Dead containers present in state: dead1 (all storage), dead2 (13-char ID, no storage).
	stateFile := cfg.Output.StateFile
	require.NoError(t, os.WriteFile(stateFile, []byte(`{
		"containers": {
			"dead1": {"name": "old-app"},
			"thirteen-charx": {"name": "long-app"}
		},
		"last_updated": "2024-01-01T00:00:00Z"
	}`), 0600))
	kbFile := filepath.Join(cfg.Output.KnowledgeBaseDir, "services", "old-app.md")
	require.NoError(t, os.WriteFile(kbFile, []byte("# x"), 0600))
	reportsSub := filepath.Join(cfg.Output.ReportsDir, "old-app")
	require.NoError(t, os.MkdirAll(reportsSub, 0750))
	require.NoError(t, os.WriteFile(filepath.Join(reportsSub, "r.md"), []byte("r"), 0600))
	// LLM logging enabled with existing dir for the container
	cfg.Output.LLMLogEnabled = true
	llmSub := filepath.Join(cfg.Output.LLMLogDir, "old-app")
	require.NoError(t, os.MkdirAll(llmSub, 0750))
	require.NoError(t, os.WriteFile(filepath.Join(llmSub, "c.json"), []byte("{}"), 0600))

	cleanupForce = true
	withDockerMock(t, &testMockDockerClient{containers: []docker.Container{}}, nil)

	err := cleanupExecuteCmd.RunE(cleanupExecuteCmd, []string{})

	require.NoError(t, err)
	assert.Contains(t, out.String(), "Cleanup complete")
	assert.Contains(t, out.String(), "Removed: 3 container(s)") // dead1 + orphaned-old-app + thirteen-charx
	assert.Contains(t, out.String(), "thirteen-cha")            // execute loops truncate 13-char IDs
	assert.NotContains(t, out.String(), "thirteen-charx")
	assert.NotContains(t, out.String(), "Failed:")
	// Everything gone
	_, statErr := os.Stat(kbFile)
	assert.True(t, os.IsNotExist(statErr), "KB file should be deleted")
	_, statErr = os.Stat(reportsSub)
	assert.True(t, os.IsNotExist(statErr), "reports dir should be deleted")
	_, statErr = os.Stat(llmSub)
	assert.True(t, os.IsNotExist(statErr), "LLM logs dir should be deleted")
	st, loadErr := state.Load(stateFile)
	require.NoError(t, loadErr)
	_, existed := st.GetAllContainers()["dead1"]
	assert.False(t, existed, "state entry should be removed")
}

func TestCleanupExecuteCmd_ForceWithFailure(t *testing.T) {
	// ponytail: no portable way to make state.Save()'s rename fail — file locks
	// are Windows-only here; read-only files are silently removed by the toolchain.
	if runtime.GOOS != "windows" {
		t.Skip("file-delete lock is Windows-specific")
	}
	out := setupCleanupRunTest(t)

	// Containers in state + storage entries everywhere; locks make
	// state.Save() and each Remove/RemoveAll fail.
	require.NoError(t, os.WriteFile(cfg.Output.StateFile, []byte(`{
		"containers": {"dead1": {"name": "old-app"}},
		"last_updated": "2024-01-01T00:00:00Z"
	}`), 0600))
	kbFile := filepath.Join(cfg.Output.KnowledgeBaseDir, "services", "old-app.md")
	require.NoError(t, os.WriteFile(kbFile, []byte("# x"), 0600))
	reportsFile := filepath.Join(cfg.Output.ReportsDir, "old-app", "r.md")
	require.NoError(t, os.MkdirAll(filepath.Join(cfg.Output.ReportsDir, "old-app"), 0750))
	require.NoError(t, os.WriteFile(reportsFile, []byte("r"), 0600))
	cfg.Output.LLMLogEnabled = true
	llmFile := filepath.Join(cfg.Output.LLMLogDir, "old-app", "c.json")
	require.NoError(t, os.MkdirAll(filepath.Join(cfg.Output.LLMLogDir, "old-app"), 0750))
	require.NoError(t, os.WriteFile(llmFile, []byte("{}"), 0600))

	// Block renames on the state file -> deleteFromState's Save() fails.
	lockStateFileForTest(t, cfg.Output.StateFile)
	// Block deletes of KB/reports/LLM files -> Remove/RemoveAll fail.
	for _, locked := range []string{kbFile, reportsFile, llmFile} {
		lockStateFileForTest(t, locked)
	}

	cleanupForce = true
	withDockerMock(t, &testMockDockerClient{containers: []docker.Container{}}, nil)

	err := cleanupExecuteCmd.RunE(cleanupExecuteCmd, []string{})

	require.NoError(t, err)
	assert.Contains(t, out.String(), "Failed: 2 container(s)") // state entry + orphan both hit locked files
	assert.Contains(t, out.String(), "state deletion failed")
	assert.Contains(t, out.String(), "KB deletion failed")
	assert.Contains(t, out.String(), "reports deletion failed")
	assert.Contains(t, out.String(), "LLM logs deletion failed")
	// Documented behavior: cleanup continues through the remaining
	// per-container locations even when one deletion failed.
	_, statErr := os.Stat(kbFile)
	assert.NoError(t, statErr, "locked files stay in place")
}

// Direct tests for scanLLMLogs branches not exercised by RunE tests.

func TestScanLLMLogs(t *testing.T) {
	t.Run("disabled returns empty", func(t *testing.T) {
		cfg := &config.Config{Output: config.OutputConfig{LLMLogEnabled: false}}
		names, err := scanLLMLogs(cfg)
		require.NoError(t, err)
		assert.Empty(t, names)
	})

	t.Run("missing dir returns empty", func(t *testing.T) {
		cfg := &config.Config{Output: config.OutputConfig{LLMLogEnabled: true, LLMLogDir: filepath.Join(t.TempDir(), "nope")}}
		names, err := scanLLMLogs(cfg)
		require.NoError(t, err)
		assert.Empty(t, names)
	})

	t.Run("dir with containers", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "llm")
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "web"), 0750))
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "db"), 0750))
		cfg := &config.Config{Output: config.OutputConfig{LLMLogEnabled: true, LLMLogDir: dir}}
		names, err := scanLLMLogs(cfg)
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"web", "db"}, names)
	})

	t.Run("path is a file not a dir", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "llm")
		require.NoError(t, os.WriteFile(dir, []byte("x"), 0600))
		cfg := &config.Config{Output: config.OutputConfig{LLMLogEnabled: true, LLMLogDir: dir}}
		_, err := scanLLMLogs(cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read LLM logs directory")
	})
}

func TestIsOrphanedEntry(t *testing.T) {
	containers := map[string]*state.Container{
		"abc123": {Name: "web"},
	}
	dockerIDs := map[string]bool{"abc123": true}

	assert.False(t, isOrphanedEntry("web", containers, dockerIDs), "name owned by a live container is not orphaned")
	assert.True(t, isOrphanedEntry("ghost", containers, dockerIDs), "name owned by no one is orphaned")
	assert.True(t, isOrphanedEntry("web", containers, map[string]bool{}), "container itself dead -> orphaned")
}

func TestAddOrphanedEntry(t *testing.T) {
	storageMaps := &storageMaps{
		kbMap:      map[string]bool{"project_db": true},
		reportsMap: map[string]bool{"project_db": true},
		llmLogsMap: map[string]bool{},
	}
	obsoleteMap := map[string]*ObsoleteContainer{}

	addOrphanedEntry("project_db", storageMaps, obsoleteMap)
	require.Len(t, obsoleteMap, 1)
	entry := obsoleteMap["orphaned-project_db"]
	require.NotNil(t, entry)
	assert.Equal(t, "project/db", entry.Name) // underscores un-sanitized for display
	assert.True(t, entry.InKB)
	assert.True(t, entry.InReports)
	assert.False(t, entry.InLLMLogs)
	assert.False(t, entry.InState)

	// Adding the same name again must not overwrite the existing entry.
	obsoleteMap["orphaned-project_db"].ID = "custom-marker"
	addOrphanedEntry("project_db", storageMaps, obsoleteMap)
	assert.Equal(t, "custom-marker", obsoleteMap["orphaned-project_db"].ID)
}

func TestConvertToSortedList(t *testing.T) {
	require.Empty(t, convertToSortedList(map[string]*ObsoleteContainer{}))

	got := convertToSortedList(map[string]*ObsoleteContainer{
		"b": {ID: "b"},
		"a": {ID: "a"},
		"c": {ID: "c"},
	})
	require.Len(t, got, 3)
	assert.Equal(t, []string{"a", "b", "c"}, []string{got[0].ID, got[1].ID, got[2].ID})
}

func TestDeleteFromState(t *testing.T) {
	tmpDir := t.TempDir()
	stateFile := filepath.Join(tmpDir, "state.json")
	require.NoError(t, os.WriteFile(stateFile, []byte(`{
		"containers": {"dead1": {"name": "old-app"}, "live1": {"name": "web"}},
		"last_updated": "2024-01-01T00:00:00Z"
	}`), 0600))

	st, err := state.Load(stateFile)
	require.NoError(t, err)

	require.NoError(t, deleteFromState("dead1", st))

	reloaded, err := state.Load(stateFile)
	require.NoError(t, err)
	containers := reloaded.GetAllContainers()
	_, deadStillThere := containers["dead1"]
	assert.False(t, deadStillThere, "dead container should be gone from persisted state")
	_, liveStillThere := containers["live1"]
	assert.True(t, liveStillThere, "other entries must survive")
}

func TestDeleteKnowledgeBase(t *testing.T) {
	kbDir := filepath.Join(t.TempDir(), "knowledge_base")
	cfg := &config.Config{Output: config.OutputConfig{KnowledgeBaseDir: kbDir}}

	// Empty name: nothing to do
	require.NoError(t, deleteKnowledgeBase("", cfg))

	// Nonexistent entry: nothing to do
	require.NoError(t, deleteKnowledgeBase("ghost", cfg))

	// Existing file: deleted
	servicesDir := filepath.Join(kbDir, "services")
	require.NoError(t, os.MkdirAll(servicesDir, 0750))
	kbFile := filepath.Join(servicesDir, "web.md")
	require.NoError(t, os.WriteFile(kbFile, []byte("# x"), 0600))
	require.NoError(t, deleteKnowledgeBase("web", cfg))
	_, statErr := os.Stat(kbFile)
	assert.True(t, os.IsNotExist(statErr))

	// Entry is a non-empty directory: removal fails
	broken := filepath.Join(servicesDir, "stuck.md")
	require.NoError(t, os.MkdirAll(filepath.Join(broken, "stuck"), 0750))
	err := deleteKnowledgeBase("stuck", cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "knowledge base file")
}

func TestDeleteReportsDir(t *testing.T) {
	reportsDir := filepath.Join(t.TempDir(), "reports")
	cfg := &config.Config{Output: config.OutputConfig{ReportsDir: reportsDir}}

	// Empty name / nonexistent: nothing to do
	require.NoError(t, deleteReportsDir("", cfg))
	require.NoError(t, deleteReportsDir("ghost", cfg))

	// Existing directory removed with contents
	sub := filepath.Join(reportsDir, "web")
	require.NoError(t, os.MkdirAll(sub, 0750))
	require.NoError(t, os.WriteFile(filepath.Join(sub, "r.md"), []byte("r"), 0600))
	require.NoError(t, deleteReportsDir("web", cfg))
	_, statErr := os.Stat(sub)
	assert.True(t, os.IsNotExist(statErr))
}

func TestDeleteLLMLogsDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "llm")

	// Empty name / disabled logging: nothing to do
	cfg := &config.Config{Output: config.OutputConfig{LLMLogEnabled: false, LLMLogDir: dir}}
	require.NoError(t, deleteLLMLogsDir("", cfg))
	require.NoError(t, deleteLLMLogsDir("web", cfg))

	// Enabled but nonexistent: nothing to do
	cfg.Output.LLMLogEnabled = true
	require.NoError(t, deleteLLMLogsDir("ghost", cfg))

	// Existing directory removed with contents
	sub := filepath.Join(dir, "web")
	require.NoError(t, os.MkdirAll(sub, 0750))
	require.NoError(t, os.WriteFile(filepath.Join(sub, "c.json"), []byte("{}"), 0600))
	require.NoError(t, deleteLLMLogsDir("web", cfg))
	_, statErr := os.Stat(sub)
	assert.True(t, os.IsNotExist(statErr))
}
