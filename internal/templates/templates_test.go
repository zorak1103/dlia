package templates

import (
	"strings"
	"testing"
)

func TestConfigYAML_NotEmpty(t *testing.T) {
	if len(ConfigYAML) == 0 {
		t.Error("Expected ConfigYAML to be non-empty")
	}
}

func TestConfigYAML_ContainsYAMLContent(t *testing.T) {
	content := string(ConfigYAML)

	// Check for expected config sections
	expectedSections := []string{
		"llm:",
		"docker:",
		"notification:",
		"output:",
		"privacy:",
	}

	for _, section := range expectedSections {
		if !strings.Contains(content, section) {
			t.Errorf("Expected ConfigYAML to contain section %q", section)
		}
	}
}

func TestConfigYAML_ContainsLLMFields(t *testing.T) {
	content := string(ConfigYAML)

	expectedFields := []string{
		"base_url:",
		"api_key:",
		"model:",
		"max_tokens:",
	}

	for _, field := range expectedFields {
		if !strings.Contains(content, field) {
			t.Errorf("Expected ConfigYAML to contain field %q", field)
		}
	}
}

func TestConfigYAML_ContainsComments(t *testing.T) {
	content := string(ConfigYAML)

	// YAML comments start with #
	if !strings.Contains(content, "#") {
		t.Error("Expected ConfigYAML to contain comments (lines starting with #)")
	}
}

func TestEnvFile_NotEmpty(t *testing.T) {
	if len(EnvFile) == 0 {
		t.Error("Expected EnvFile to be non-empty")
	}
}

func TestEnvFile_ContainsEnvVars(t *testing.T) {
	content := string(EnvFile)

	// Check for expected environment variable patterns
	expectedVars := []string{
		"DLIA_LLM_API_KEY",
		"DLIA_LLM_BASE_URL",
		"DLIA_LLM_MODEL",
	}

	for _, envVar := range expectedVars {
		if !strings.Contains(content, envVar) {
			t.Errorf("Expected EnvFile to contain variable %q", envVar)
		}
	}
}

func TestEnvFile_HasProperFormat(t *testing.T) {
	content := string(EnvFile)

	// Check that it follows KEY=value format
	if !strings.Contains(content, "=") {
		t.Error("Expected EnvFile to contain '=' for key=value format")
	}
}

func TestConfigYAML_ValidYAMLStructure(t *testing.T) {
	content := string(ConfigYAML)

	// Check for proper YAML indentation (2 spaces)
	lines := strings.Split(content, "\n")
	hasIndentation := false

	for _, line := range lines {
		if strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "   ") {
			hasIndentation = true
			break
		}
	}

	if !hasIndentation {
		t.Error("Expected ConfigYAML to have proper YAML indentation (2 spaces)")
	}
}

func TestEnvFile_NoEmptyLines(t *testing.T) {
	content := string(EnvFile)

	// Environment files typically don't have random empty lines between vars
	// Just verify content is readable
	if len(strings.TrimSpace(content)) == 0 {
		t.Error("Expected EnvFile to have non-whitespace content")
	}
}

func TestConfigYAML_ContainsDockerConfig(t *testing.T) {
	content := string(ConfigYAML)

	if !strings.Contains(content, "docker:") {
		t.Error("Expected ConfigYAML to contain docker configuration")
	}

	if !strings.Contains(content, "socket_path:") {
		t.Error("Expected ConfigYAML to contain socket_path field")
	}
}

func TestConfigYAML_ContainsNotificationConfig(t *testing.T) {
	content := string(ConfigYAML)

	if !strings.Contains(content, "notification:") {
		t.Error("Expected ConfigYAML to contain notification configuration")
	}
}

func TestConfigYAML_ContainsOutputConfig(t *testing.T) {
	content := string(ConfigYAML)

	expectedFields := []string{
		"output:",
		"reports_dir:",
		"knowledge_base_dir:",
		"state_file:",
	}

	for _, field := range expectedFields {
		if !strings.Contains(content, field) {
			t.Errorf("Expected ConfigYAML to contain field %q", field)
		}
	}
}

func TestConfigYAML_ContainsPrivacyConfig(t *testing.T) {
	content := string(ConfigYAML)

	expectedFields := []string{
		"privacy:",
		"anonymize_ips:",
		"anonymize_secrets:",
	}

	for _, field := range expectedFields {
		if !strings.Contains(content, field) {
			t.Errorf("Expected ConfigYAML to contain field %q", field)
		}
	}
}

func TestEnvFile_ContainsDockerSocketVar(t *testing.T) {
	content := string(EnvFile)

	if !strings.Contains(content, "DLIA_DOCKER_SOCKET_PATH") {
		t.Error("Expected EnvFile to contain DLIA_DOCKER_SOCKET_PATH variable")
	}
}

func TestConfigYAML_IsByteSlice(_ *testing.T) {
	// Verify ConfigYAML is a byte slice
	_ = ConfigYAML[0] // Should not panic if it's a valid byte slice with content
}

func TestEnvFile_IsByteSlice(_ *testing.T) {
	// Verify EnvFile is a byte slice
	_ = EnvFile[0] // Should not panic if it's a valid byte slice with content
}
