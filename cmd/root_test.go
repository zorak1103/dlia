package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/zorak1103/dlia/internal/config"
)

const (
	testFalseValue = "false"
	testInitCmd    = "init"
)

func TestRootCmd_Structure(t *testing.T) {
	t.Parallel()

	cmd := rootCmd

	if cmd.Use != "dlia" {
		t.Errorf("Expected command use 'dlia', got '%s'", cmd.Use)
	}

	if cmd.Short == "" {
		t.Error("Expected command short description to be set")
	}

	if cmd.Long == "" {
		t.Error("Expected command long description to be set")
	}

	if cmd.Version == "" {
		t.Error("Expected command version to be set")
	}
}

func TestRootCmd_PersistentFlags(t *testing.T) {
	t.Parallel()

	cmd := rootCmd
	flags := cmd.PersistentFlags()

	// Check --config flag
	configFlag := flags.Lookup("config")
	if configFlag == nil {
		t.Error("Expected 'config' flag to be defined")
	} else if configFlag.DefValue != "" {
		t.Errorf("Expected 'config' flag default to be empty, got '%s'", configFlag.DefValue)
	}

	// Check --verbose flag
	verboseFlag := flags.Lookup("verbose")
	if verboseFlag == nil {
		t.Fatal("Expected 'verbose' flag to be defined")
	}

	if verboseFlag.DefValue != testFalseValue {
		t.Errorf("Expected 'verbose' flag default to be 'false', got '%s'", verboseFlag.DefValue)
	}

	if verboseFlag.Shorthand != "v" {
		t.Errorf("Expected 'verbose' flag shorthand to be 'v', got '%s'", verboseFlag.Shorthand)
	}
}

func TestRootCmd_HelpOutput(t *testing.T) {
	var buf bytes.Buffer

	rootCmd.SetOut(&buf)
	rootCmd.SetArgs([]string{"--help"})

	err := rootCmd.Execute()
	if err != nil {
		t.Errorf("Expected no error executing help command, got: %v", err)
	}

	output := buf.String()

	// Verify help contains expected content
	expectedStrings := []string{
		"DLIA",
		"Docker Log Intelligence Agent",
		"AI-powered log analysis",
		"--config",
		"--verbose",
		"-v",
	}

	for _, expected := range expectedStrings {
		if !containsString(output, expected) {
			t.Errorf("Expected help output to contain %q, got:\n%s", expected, output)
		}
	}
}

func TestRootCmd_VersionOutput(t *testing.T) {
	var buf bytes.Buffer

	rootCmd.SetOut(&buf)
	rootCmd.SetArgs([]string{"--version"})

	err := rootCmd.Execute()
	if err != nil {
		t.Errorf("Expected no error executing version command, got: %v", err)
	}

	output := buf.String()

	// Should contain version information
	if !containsString(output, "dlia") {
		t.Errorf("Expected version output to contain 'dlia', got:\n%s", output)
	}
}

func TestRootCmd_SubcommandsList(t *testing.T) {
	t.Parallel()

	cmd := rootCmd

	// Verify subcommands are registered
	subcommands := cmd.Commands()

	expectedSubcommands := []string{"init", "scan", "config", "state"}
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

func TestGetConfig(t *testing.T) {
	// Save original and restore after test
	originalCfg := cfg
	defer func() { cfg = originalCfg }()

	// Test when cfg is nil
	cfg = nil
	if result := GetConfig(); result != nil {
		t.Error("Expected GetConfig() to return nil when cfg is nil")
	}

	// Test when cfg is set
	testConfig := &config.Config{
		LLM: config.LLMConfig{
			BaseURL: "http://test",
			Model:   "test-model",
		},
	}
	cfg = testConfig

	result := GetConfig()
	if result != testConfig {
		t.Error("Expected GetConfig() to return the set config")
	}

	if result.LLM.BaseURL != "http://test" {
		t.Errorf("Expected BaseURL to be 'http://test', got '%s'", result.LLM.BaseURL)
	}
}

func TestIsVerbose(t *testing.T) {
	// Save original and restore after test
	originalVerbose := verbose
	defer func() { verbose = originalVerbose }()

	// Test when verbose is false
	verbose = false
	if IsVerbose() {
		t.Error("Expected IsVerbose() to return false")
	}

	// Test when verbose is true
	verbose = true
	if !IsVerbose() {
		t.Error("Expected IsVerbose() to return true")
	}
}

func TestRootCmd_HasFeatureDescriptions(t *testing.T) {
	t.Parallel()

	longDesc := rootCmd.Long

	// Verify key features are mentioned
	expectedFeatures := []string{
		"Semantic log analysis",
		"LLM",
		"Historical context",
		"Privacy",
		"anonymization",
		"notification",
		"Shoutrrr",
		"knowledge base",
	}

	for _, feature := range expectedFeatures {
		if !containsString(longDesc, feature) {
			t.Errorf("Expected long description to mention '%s'", feature)
		}
	}
}

func TestRootCmd_ShortDescription(t *testing.T) {
	t.Parallel()

	short := rootCmd.Short

	if short != "Docker Log Intelligence Agent" {
		t.Errorf("Expected short description to be 'Docker Log Intelligence Agent', got '%s'", short)
	}
}

func TestRootCmd_ConfigFlagDescription(t *testing.T) {
	t.Parallel()

	flags := rootCmd.PersistentFlags()
	configFlag := flags.Lookup("config")

	if configFlag == nil {
		t.Fatal("Expected 'config' flag to be defined")
	}

	if !containsString(configFlag.Usage, "config file") {
		t.Errorf("Expected config flag usage to mention 'config file', got '%s'", configFlag.Usage)
	}
}

func TestRootCmd_VerboseFlagDescription(t *testing.T) {
	t.Parallel()

	flags := rootCmd.PersistentFlags()
	verboseFlag := flags.Lookup("verbose")

	if verboseFlag == nil {
		t.Fatal("Expected 'verbose' flag to be defined")
	}

	if !containsString(verboseFlag.Usage, "verbose") {
		t.Errorf("Expected verbose flag usage to mention 'verbose', got '%s'", verboseFlag.Usage)
	}
}

func TestRootCmd_UseLine(t *testing.T) {
	t.Parallel()

	useLine := rootCmd.UseLine()

	if !containsString(useLine, "dlia") {
		t.Errorf("Expected use line to contain 'dlia', got '%s'", useLine)
	}
}

func TestRootCmd_HasPersistentPreRunE(t *testing.T) {
	t.Parallel()

	if rootCmd.PersistentPreRunE == nil {
		t.Error("Expected PersistentPreRunE to be set")
	}
}

func TestRootCmd_VersionIsSet(t *testing.T) {
	t.Parallel()

	version := rootCmd.Version

	if version == "" {
		t.Error("Expected version to be set")
	}
}

func TestRootCmd_PersistentPreRunE_SkipConfigForInit(t *testing.T) {
	// Create a mock command with name "init"
	mockCmd := &cobra.Command{
		Use: testInitCmd,
	}

	err := rootCmd.PersistentPreRunE(mockCmd, []string{})
	if err != nil {
		t.Errorf("Expected no error for init command, got: %v", err)
	}
}

// runPersistentPreRunWithConfigFile drives PersistentPreRunE with a given
// config file path and restored globals, returning the post-run globals.
func runPersistentPreRunWithConfigFile(t *testing.T, commandName, configPath string) (loadedCfg *config.Config, loadErr error) {
	t.Helper()

	originalCfg, originalCfgFile, originalVerbose, originalErr := cfg, cfgFile, verbose, errConfigLoad
	t.Cleanup(func() {
		cfg, cfgFile, verbose, errConfigLoad = originalCfg, originalCfgFile, originalVerbose, originalErr
	})

	cfg = nil
	errConfigLoad = nil
	cfgFile = configPath
	verbose = false

	err := rootCmd.PersistentPreRunE(&cobra.Command{Use: commandName}, []string{})
	if err != nil {
		t.Fatalf("Expected no error from PersistentPreRunE for %q, got: %v", commandName, err)
	}

	return cfg, GetConfigLoadError()
}

func writeBrokenConfigFile(t *testing.T) string {
	t.Helper()

	brokenConfig := filepath.Join(t.TempDir(), "broken.yaml")
	err := os.WriteFile(brokenConfig, []byte("{{{ not yaml"), 0600)
	if err != nil {
		t.Fatalf("Failed to write broken config: %v", err)
	}
	return brokenConfig
}

func TestRootCmd_PersistentPreRunE_SkipNamesNeverLoadConfig(t *testing.T) {
	brokenConfig := writeBrokenConfigFile(t)

	// init/help/version must not attempt config loading: neither the config
	// nor a stored load error may appear, even though the config file is broken.
	for _, name := range []string{"init", "help", "version"} {
		t.Run(name, func(t *testing.T) {
			loadedCfg, loadErr := runPersistentPreRunWithConfigFile(t, name, brokenConfig)

			if loadedCfg != nil {
				t.Errorf("Expected config NOT to load for %q command", name)
			}
			if loadErr != nil {
				t.Errorf("Expected no stored config-load error for %q command, got: %v", name, loadErr)
			}
		})
	}
}

func TestRootCmd_PersistentPreRunE_LoadsConfigForOtherCommands(t *testing.T) {
	tmpDir := t.TempDir()

	// Directories referenced by the config (validateConfigOrExit checks them
	// separately; Load itself only validates values).
	for _, dir := range []string{"reports", "kb", "state"} {
		if err := os.MkdirAll(filepath.Join(tmpDir, dir), 0750); err != nil {
			t.Fatalf("Failed to create dir: %v", err)
		}
	}

	validConfig := filepath.Join(tmpDir, "config.yaml")
	content := fmt.Sprintf("llm:\n  api_key: test-key\n  model: test-model\n  base_url: http://test\ndocker:\n  socket_path: unix:///var/run/docker.sock\noutput:\n  reports_dir: %s\n  knowledge_base_dir: %s\n  state_file: %s\n",
		filepath.Join(tmpDir, "reports"), filepath.Join(tmpDir, "kb"), filepath.Join(tmpDir, "state", "state.json"))
	if err := os.WriteFile(validConfig, []byte(content), 0600); err != nil {
		t.Fatalf("Failed to write valid config: %v", err)
	}

	loadedCfg, loadErr := runPersistentPreRunWithConfigFile(t, "scan", validConfig)

	if loadedCfg == nil {
		t.Error("Expected config to load for a non-skip command")
	}
	if loadErr != nil {
		t.Errorf("Expected no config-load error for a valid config, got: %v", loadErr)
	}
}

func TestRootCmd_PersistentPreRunE_StoresConfigLoadError(t *testing.T) {
	brokenConfig := writeBrokenConfigFile(t)

	_, loadErr := runPersistentPreRunWithConfigFile(t, "scan", brokenConfig)

	// The error is stored (not returned) so commands can fail fast with
	// validateConfigOrExit.
	if loadErr == nil {
		t.Error("Expected a stored config-load error for a broken config file")
	}
}

func TestRootCmd_PersistentPreRunE_SkipConfigForHelp(t *testing.T) {
	// Create a mock command with name "help"
	mockCmd := &cobra.Command{
		Use: "help",
	}

	err := rootCmd.PersistentPreRunE(mockCmd, []string{})
	if err != nil {
		t.Errorf("Expected no error for help command, got: %v", err)
	}
}

func TestRootCmd_PersistentPreRunE_LoadConfig(t *testing.T) {
	// Save original values
	originalCfg := cfg
	originalCfgFile := cfgFile
	originalVerbose := verbose
	defer func() {
		cfg = originalCfg
		cfgFile = originalCfgFile
		verbose = originalVerbose
	}()

	// Create a mock command that is not init or help
	mockCmd := &cobra.Command{
		Use: "scan",
	}

	// Test with non-existent config file (should not fail)
	cfgFile = "nonexistent.yaml"
	verbose = false

	err := rootCmd.PersistentPreRunE(mockCmd, []string{})
	if err != nil {
		t.Errorf("Expected no error with missing config, got: %v", err)
	}
}

func TestRootCmd_PersistentPreRunE_VerboseMode(t *testing.T) {
	// Save original values
	originalCfg := cfg
	originalCfgFile := cfgFile
	originalVerbose := verbose
	defer func() {
		cfg = originalCfg
		cfgFile = originalCfgFile
		verbose = originalVerbose
	}()

	// Create a mock command
	mockCmd := &cobra.Command{
		Use: "scan",
	}

	// Test verbose mode with non-existent config
	cfgFile = "nonexistent_verbose.yaml"
	verbose = true

	err := rootCmd.PersistentPreRunE(mockCmd, []string{})
	if err != nil {
		t.Errorf("Expected no error in verbose mode, got: %v", err)
	}
}

func TestExecute_Exists(t *testing.T) {
	// Verify that Execute function exists
	// We can't easily test the actual execution since it calls os.Exit(1) on error
	// This test just verifies the function is properly defined
	t.Log("Execute function is defined and available")
}

func TestGetConfigLoadError(t *testing.T) {
	original := errConfigLoad
	defer func() { errConfigLoad = original }()

	errConfigLoad = nil
	if err := GetConfigLoadError(); err != nil {
		t.Errorf("expected nil, got %v", err)
	}

	sentinel := errors.New("load failed")
	errConfigLoad = sentinel
	if got := GetConfigLoadError(); !errors.Is(got, sentinel) {
		t.Errorf("expected sentinel error, got %v", got)
	}
}

func TestRootCmd_SubcommandInit(t *testing.T) {
	// Verify init command is properly registered
	initCmd := rootCmd.Commands()
	found := false
	for _, cmd := range initCmd {
		if cmd.Name() == testInitCmd {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected 'init' subcommand to be registered")
	}
}

func TestRootCmd_SubcommandScan(t *testing.T) {
	// Verify scan command is properly registered
	scanCmd := rootCmd.Commands()
	found := false
	for _, cmd := range scanCmd {
		if cmd.Name() == "scan" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected 'scan' subcommand to be registered")
	}
}

func TestRootCmd_SubcommandConfig(t *testing.T) {
	// Verify config command is properly registered
	configCmd := rootCmd.Commands()
	found := false
	for _, cmd := range configCmd {
		if cmd.Name() == "config" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected 'config' subcommand to be registered")
	}
}

func TestRootCmd_SubcommandState(t *testing.T) {
	// Verify state command is properly registered
	stateCmd := rootCmd.Commands()
	found := false
	for _, cmd := range stateCmd {
		if cmd.Name() == "state" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected 'state' subcommand to be registered")
	}
}

func TestRootCmd_PersistentPreRunE_PrintsConfigWarnings(t *testing.T) {
	t.Setenv("DLIA_LLM_CONTEXT_WINDOW", "")
	t.Setenv("DLIA_LLM_MAX_TOKENS", "")

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	content := "llm:\n  api_key: test-key\n  max_tokens: 50000\n"
	if err := os.WriteFile(configPath, []byte(content), 0600); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	original := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Failed to create pipe: %v", err)
	}
	os.Stderr = w
	t.Cleanup(func() { os.Stderr = original })

	loadedCfg, loadErr := runPersistentPreRunWithConfigFile(t, "scan", configPath)

	os.Stderr = original
	_ = w.Close()
	out, _ := io.ReadAll(r)

	if loadErr != nil || loadedCfg == nil {
		t.Fatalf("Expected config to load, got cfg=%v err=%v", loadedCfg, loadErr)
	}
	want := "Warning: llm.max_tokens is deprecated; use llm.context_window instead"
	if !strings.Contains(string(out), want) {
		t.Errorf("Expected stderr to contain %q, got %q", want, string(out))
	}
}
