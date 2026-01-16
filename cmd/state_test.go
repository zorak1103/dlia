package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/zorak1103/dlia/internal/config"
	"github.com/zorak1103/dlia/internal/state"
)

const (
	testConfigNotLoaded = "configuration not loaded\n\nDLIA has not been initialized in this directory.\nRun 'dlia init' to set up DLIA and create the necessary configuration"
)

func TestStateCmd_Structure(t *testing.T) {
	t.Parallel()

	cmd := stateCmd

	if cmd.Use != "state" {
		t.Errorf("Expected command use 'state', got '%s'", cmd.Use)
	}

	if cmd.Short == "" {
		t.Error("Expected command short description to be set")
	}

	if cmd.Long == "" {
		t.Error("Expected command long description to be set")
	}
}

func TestStateListCmd_Structure(t *testing.T) {
	t.Parallel()

	cmd := stateListCmd

	if cmd.Use != "list" {
		t.Errorf("Expected command use 'list', got '%s'", cmd.Use)
	}

	if cmd.Short == "" {
		t.Error("Expected command short description to be set")
	}

	if cmd.Example == "" {
		t.Error("Expected command example to be set")
	}
}

func TestStateResetCmd_Structure(t *testing.T) {
	t.Parallel()

	cmd := stateResetCmd

	if cmd.Use != "reset [container-filter]" {
		t.Errorf("Expected command use 'reset [container-filter]', got '%s'", cmd.Use)
	}

	if cmd.Short == "" {
		t.Error("Expected command short description to be set")
	}

	if cmd.Example == "" {
		t.Error("Expected command example to be set")
	}
}

func TestStateResetCmd_Flags(t *testing.T) {
	t.Parallel()

	cmd := stateResetCmd
	flags := cmd.Flags()

	forceFlag := flags.Lookup("force")
	if forceFlag == nil {
		t.Error("Expected 'force' flag to be defined")
		return
	}

	if forceFlag.DefValue != "false" {
		t.Errorf("Expected 'force' flag default to be 'false', got '%s'", forceFlag.DefValue)
	}
}

func TestStateListCmd_EmptyState(t *testing.T) {
	// Setup temp directory with config
	tmpDir := t.TempDir()
	stateFile := filepath.Join(tmpDir, "state.json")
	configFile := filepath.Join(tmpDir, "config.yaml")

	// Create a minimal config file
	if err := os.WriteFile(configFile, []byte("llm:\n  api_key: test\n"), 0600); err != nil {
		t.Fatalf("Failed to create config file: %v", err)
	}

	// Reset viper and set config
	viper.Reset()
	viper.SetConfigFile(configFile)
	viper.Set("llm.api_key", "test-key")
	viper.Set("llm.model", "test-model")
	viper.Set("llm.base_url", "http://test")
	viper.Set("docker.socket_path", "unix:///test")
	viper.Set("output.reports_dir", tmpDir)
	viper.Set("output.knowledge_base_dir", tmpDir)
	viper.Set("output.state_file", stateFile)

	// Load config from viper
	testCfg, err := config.LoadFromViper()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Set global config
	originalCfg := cfg
	cfg = testCfg
	defer func() { cfg = originalCfg }()

	// Create empty state file
	st, err := state.Load(stateFile)
	if err != nil {
		t.Fatalf("Failed to load state: %v", err)
	}
	if err := st.Save(); err != nil {
		t.Fatalf("Failed to save empty state: %v", err)
	}

	var buf bytes.Buffer
	stateListCmd.SetOut(&buf)
	stateListCmd.SetErr(&buf)

	err = stateListCmd.RunE(stateListCmd, []string{})
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "No containers in state file") {
		t.Errorf("Expected message about no containers, got: %s", output)
	}
}

func TestStateListCmd_WithContainers(t *testing.T) {
	// Setup temp directory with config
	tmpDir := t.TempDir()
	stateFile := filepath.Join(tmpDir, "state.json")
	configFile := filepath.Join(tmpDir, "config.yaml")

	// Create a minimal config file
	if err := os.WriteFile(configFile, []byte("llm:\n  api_key: test\n"), 0600); err != nil {
		t.Fatalf("Failed to create config file: %v", err)
	}

	// Reset viper and set config
	viper.Reset()
	viper.SetConfigFile(configFile)
	viper.Set("llm.api_key", "test-key")
	viper.Set("llm.model", "test-model")
	viper.Set("llm.base_url", "http://test")
	viper.Set("docker.socket_path", "unix:///test")
	viper.Set("output.reports_dir", tmpDir)
	viper.Set("output.knowledge_base_dir", tmpDir)
	viper.Set("output.state_file", stateFile)

	// Load config from viper
	testCfg, err := config.LoadFromViper()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Set global config
	originalCfg := cfg
	cfg = testCfg
	defer func() { cfg = originalCfg }()

	// Create state with containers
	st, err := state.Load(stateFile)
	if err != nil {
		t.Fatalf("Failed to load state: %v", err)
	}
	st.UpdateContainer("container1", "test-container-1", time.Now(), "cursor1")
	st.UpdateContainer("container2", "test-container-2", time.Now(), "cursor2")
	if err := st.Save(); err != nil {
		t.Fatalf("Failed to save state: %v", err)
	}

	var buf bytes.Buffer
	stateListCmd.SetOut(&buf)
	stateListCmd.SetErr(&buf)

	err = stateListCmd.RunE(stateListCmd, []string{})
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "test-container-1") {
		t.Errorf("Expected container name in output, got: %s", output)
	}
	if !strings.Contains(output, "test-container-2") {
		t.Errorf("Expected container name in output, got: %s", output)
	}
	if !strings.Contains(output, "Total: 2 container(s)") {
		t.Errorf("Expected total count in output, got: %s", output)
	}
}

func TestStateResetCmd_RequiresForce(t *testing.T) {
	// Setup temp directory with config
	tmpDir := t.TempDir()
	stateFile := filepath.Join(tmpDir, "state.json")
	configFile := filepath.Join(tmpDir, "config.yaml")

	// Create a minimal config file
	if err := os.WriteFile(configFile, []byte("llm:\n  api_key: test\n"), 0600); err != nil {
		t.Fatalf("Failed to create config file: %v", err)
	}

	// Reset viper and set config
	viper.Reset()
	viper.SetConfigFile(configFile)
	viper.Set("llm.api_key", "test-key")
	viper.Set("llm.model", "test-model")
	viper.Set("llm.base_url", "http://test")
	viper.Set("docker.socket_path", "unix:///test")
	viper.Set("output.reports_dir", tmpDir)
	viper.Set("output.knowledge_base_dir", tmpDir)
	viper.Set("output.state_file", stateFile)

	// Load config from viper
	testCfg, err := config.LoadFromViper()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Set global config
	originalCfg := cfg
	cfg = testCfg
	defer func() { cfg = originalCfg }()

	// Create state
	st, err := state.Load(stateFile)
	if err != nil {
		t.Fatalf("Failed to load state: %v", err)
	}
	st.UpdateContainer("container1", "test-container", time.Now(), "")
	if err := st.Save(); err != nil {
		t.Fatalf("Failed to save state: %v", err)
	}

	// Reset force flag
	originalForce := force
	force = false
	defer func() { force = originalForce }()

	var buf bytes.Buffer
	stateResetCmd.SetOut(&buf)
	stateResetCmd.SetErr(&buf)

	err = stateResetCmd.RunE(stateResetCmd, []string{})
	if err != nil {
		t.Errorf("Expected no error (just warning), got: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Aborted") {
		t.Errorf("Expected abort message without --force, got: %s", output)
	}

	// Verify state file still exists
	if _, err := os.Stat(stateFile); os.IsNotExist(err) {
		t.Error("State file should not be deleted without --force")
	}
}

func TestStateResetCmd_WithForce(t *testing.T) {
	// Setup temp directory with config
	tmpDir := t.TempDir()
	stateFile := filepath.Join(tmpDir, "state.json")
	configFile := filepath.Join(tmpDir, "config.yaml")

	// Create a minimal config file
	if err := os.WriteFile(configFile, []byte("llm:\n  api_key: test\n"), 0600); err != nil {
		t.Fatalf("Failed to create config file: %v", err)
	}

	// Reset viper and set config
	viper.Reset()
	viper.SetConfigFile(configFile)
	viper.Set("llm.api_key", "test-key")
	viper.Set("llm.model", "test-model")
	viper.Set("llm.base_url", "http://test")
	viper.Set("docker.socket_path", "unix:///test")
	viper.Set("output.reports_dir", tmpDir)
	viper.Set("output.knowledge_base_dir", tmpDir)
	viper.Set("output.state_file", stateFile)

	// Load config from viper
	testCfg, err := config.LoadFromViper()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Set global config
	originalCfg := cfg
	cfg = testCfg
	defer func() { cfg = originalCfg }()

	// Create state with containers
	st, err := state.Load(stateFile)
	if err != nil {
		t.Fatalf("Failed to load state: %v", err)
	}
	st.UpdateContainer("container1", "test-container-1", time.Now(), "")
	st.UpdateContainer("container2", "test-container-2", time.Now(), "")
	if err := st.Save(); err != nil {
		t.Fatalf("Failed to save state: %v", err)
	}

	// Set force flag
	originalForce := force
	force = true
	defer func() { force = originalForce }()

	var buf bytes.Buffer
	stateResetCmd.SetOut(&buf)
	stateResetCmd.SetErr(&buf)

	err = stateResetCmd.RunE(stateResetCmd, []string{})
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "State reset complete") {
		t.Errorf("Expected success message, got: %s", output)
	}
	if !strings.Contains(output, "Removed 2 container(s)") {
		t.Errorf("Expected container count in output, got: %s", output)
	}

	// Verify state file was deleted
	if _, err := os.Stat(stateFile); !os.IsNotExist(err) {
		t.Error("State file should be deleted with --force")
	}
}

func TestStateResetCmd_WithFilter(t *testing.T) {
	// Setup temp directory with config
	tmpDir := t.TempDir()
	stateFile := filepath.Join(tmpDir, "state.json")
	configFile := filepath.Join(tmpDir, "config.yaml")

	// Create a minimal config file
	if err := os.WriteFile(configFile, []byte("llm:\n  api_key: test\n"), 0600); err != nil {
		t.Fatalf("Failed to create config file: %v", err)
	}

	// Reset viper and set config
	viper.Reset()
	viper.SetConfigFile(configFile)
	viper.Set("llm.api_key", "test-key")
	viper.Set("llm.model", "test-model")
	viper.Set("llm.base_url", "http://test")
	viper.Set("docker.socket_path", "unix:///test")
	viper.Set("output.reports_dir", tmpDir)
	viper.Set("output.knowledge_base_dir", tmpDir)
	viper.Set("output.state_file", stateFile)

	// Load config from viper
	testCfg, err := config.LoadFromViper()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Set global config
	originalCfg := cfg
	cfg = testCfg
	defer func() { cfg = originalCfg }()

	// Create state with containers
	st, err := state.Load(stateFile)
	if err != nil {
		t.Fatalf("Failed to load state: %v", err)
	}
	st.UpdateContainer("container1", "nginx-1", time.Now(), "")
	st.UpdateContainer("container2", "postgres-1", time.Now(), "")
	if err := st.Save(); err != nil {
		t.Fatalf("Failed to save state: %v", err)
	}

	// Set force flag
	originalForce := force
	force = true
	defer func() { force = originalForce }()

	var buf bytes.Buffer
	stateResetCmd.SetOut(&buf)
	stateResetCmd.SetErr(&buf)

	err = stateResetCmd.RunE(stateResetCmd, []string{"nginx.*"})
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "State reset complete") {
		t.Errorf("Expected success message, got: %s", output)
	}

	// Reload state and verify only nginx was removed
	st, err = state.Load(stateFile)
	if err != nil {
		t.Fatalf("Failed to reload state: %v", err)
	}

	containers := st.GetAllContainers()
	if len(containers) != 1 {
		t.Errorf("Expected 1 container remaining, got %d", len(containers))
	}

	// postgres should still exist
	hasPostgres := false
	for _, ctr := range containers {
		if ctr.Name == "postgres-1" {
			hasPostgres = true
		}
	}
	if !hasPostgres {
		t.Error("Expected postgres-1 to remain in state")
	}
}

func TestStateResetCmd_FilterNoMatches(t *testing.T) {
	// Setup temp directory with config
	tmpDir := t.TempDir()
	stateFile := filepath.Join(tmpDir, "state.json")
	configFile := filepath.Join(tmpDir, "config.yaml")

	// Create a minimal config file
	if err := os.WriteFile(configFile, []byte("llm:\n  api_key: test\n"), 0600); err != nil {
		t.Fatalf("Failed to create config file: %v", err)
	}

	// Reset viper and set config
	viper.Reset()
	viper.SetConfigFile(configFile)
	viper.Set("llm.api_key", "test-key")
	viper.Set("llm.model", "test-model")
	viper.Set("llm.base_url", "http://test")
	viper.Set("docker.socket_path", "unix:///test")
	viper.Set("output.reports_dir", tmpDir)
	viper.Set("output.knowledge_base_dir", tmpDir)
	viper.Set("output.state_file", stateFile)

	// Load config from viper
	testCfg, err := config.LoadFromViper()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Set global config
	originalCfg := cfg
	cfg = testCfg
	defer func() { cfg = originalCfg }()

	// Create state with containers
	st, err := state.Load(stateFile)
	if err != nil {
		t.Fatalf("Failed to load state: %v", err)
	}
	st.UpdateContainer("container1", "nginx-1", time.Now(), "")
	if err := st.Save(); err != nil {
		t.Fatalf("Failed to save state: %v", err)
	}

	// Set force flag
	originalForce := force
	force = true
	defer func() { force = originalForce }()

	var buf bytes.Buffer
	stateResetCmd.SetOut(&buf)
	stateResetCmd.SetErr(&buf)

	err = stateResetCmd.RunE(stateResetCmd, []string{"nonexistent.*"})
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "No containers matched") {
		t.Errorf("Expected 'no containers matched' message, got: %s", output)
	}
}

func TestStateCmd_Subcommands(t *testing.T) {
	t.Parallel()

	// Verify subcommands are registered
	subcommands := stateCmd.Commands()

	expectedSubcommands := []string{"list", "reset"}
	foundSubcommands := make(map[string]bool)

	for _, subcmd := range subcommands {
		foundSubcommands[subcmd.Name()] = true
	}

	for _, expected := range expectedSubcommands {
		if !foundSubcommands[expected] {
			t.Errorf("Expected subcommand '%s' to be registered", expected)
		}
	}
}

func TestStateListCmd_HelpOutput(t *testing.T) {
	var buf bytes.Buffer

	// Set output on all commands in the hierarchy
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	stateCmd.SetOut(&buf)
	stateCmd.SetErr(&buf)
	stateListCmd.SetOut(&buf)
	stateListCmd.SetErr(&buf)

	rootCmd.SetArgs([]string{"state", "list", "--help"})

	err := rootCmd.Execute()
	if err != nil {
		t.Errorf("Expected no error executing help command, got: %v", err)
	}

	output := buf.String()

	expectedStrings := []string{
		"Display the current state file",
		"timestamp",
		"dlia state list",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(output, expected) {
			t.Errorf("Expected help output to contain %q, got:\n%s", expected, output)
		}
	}
}

func TestStateResetCmd_HelpOutput(t *testing.T) {
	var buf bytes.Buffer

	// Set output on all commands in the hierarchy
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	stateCmd.SetOut(&buf)
	stateCmd.SetErr(&buf)
	stateResetCmd.SetOut(&buf)
	stateResetCmd.SetErr(&buf)

	rootCmd.SetArgs([]string{"state", "reset", "--help"})

	err := rootCmd.Execute()
	if err != nil {
		t.Errorf("Expected no error executing help command, got: %v", err)
	}

	output := buf.String()

	expectedStrings := []string{
		"Reset the state file",
		"--force",
		"WARNING",
		"dlia state reset",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(output, expected) {
			t.Errorf("Expected help output to contain %q, got:\n%s", expected, output)
		}
	}
}

func TestStateListCmd_RequiresConfig(t *testing.T) {
	// Reset cfg to test config requirement
	originalCfg := cfg
	cfg = nil
	defer func() { cfg = originalCfg }()

	var buf bytes.Buffer
	stateListCmd.SetOut(&buf)
	stateListCmd.SetErr(&buf)

	err := stateListCmd.RunE(stateListCmd, []string{})

	if err == nil {
		t.Error("Expected error when config is nil")
	}

	if err.Error() != testConfigNotLoaded {
		t.Errorf("Expected 'configuration not loaded' error, got: %v", err)
	}
}

func TestStateResetCmd_RequiresConfig(t *testing.T) {
	// Reset cfg to test config requirement
	originalCfg := cfg
	cfg = nil
	defer func() { cfg = originalCfg }()

	var buf bytes.Buffer
	stateResetCmd.SetOut(&buf)
	stateResetCmd.SetErr(&buf)

	err := stateResetCmd.RunE(stateResetCmd, []string{})

	if err == nil {
		t.Error("Expected error when config is nil")
	}

	if err.Error() != testConfigNotLoaded {
		t.Errorf("Expected 'configuration not loaded' error, got: %v", err)
	}
}

func TestStateResetCmd_MaximumOneArg(t *testing.T) {
	t.Parallel()

	cmd := stateResetCmd

	// Verify Args validation
	if cmd.Args == nil {
		t.Error("Expected Args validation to be set")
		return
	}

	// Args should allow 0 or 1 argument
	err := cmd.Args(cmd, []string{})
	if err != nil {
		t.Errorf("Expected no error with 0 args, got: %v", err)
	}

	err = cmd.Args(cmd, []string{"filter"})
	if err != nil {
		t.Errorf("Expected no error with 1 arg, got: %v", err)
	}

	err = cmd.Args(cmd, []string{"filter1", "filter2"})
	if err == nil {
		t.Error("Expected error with 2 args")
	}
}
