package prompts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zorak1103/dlia/internal/config"
)

const unknownSource = "UNKNOWN"

func TestNewPromptLoader(t *testing.T) {
	cfg := &config.Config{}
	loader := NewPromptLoader(cfg)

	if loader == nil {
		t.Fatal("NewPromptLoader returned nil")
	}
	if loader.cfg != cfg {
		t.Error("Config not set correctly")
	}
	if loader.promptSources == nil {
		t.Error("promptSources map not initialized")
	}
}

func TestPromptLoader_SystemPrompt(t *testing.T) {
	tests := []struct {
		name               string
		ignoreInstructions string
		externalPromptPath string
		externalContent    string
		wantContains       []string
		wantNotContains    []string
	}{
		{
			name:               "default system prompt without instructions",
			ignoreInstructions: "",
			wantContains: []string{
				"log analysis",
			},
		},
		{
			name:               "default system prompt with instructions",
			ignoreInstructions: "Ignore routine DEBUG messages",
			wantContains: []string{
				"log analysis",
				"User Instructions for this container:",
				"Ignore routine DEBUG messages",
			},
		},
		{
			name:               "external prompt file",
			externalPromptPath: "system_custom.md",
			externalContent:    "Custom system prompt for testing",
			wantContains: []string{
				"Custom system prompt for testing",
			},
			wantNotContains: []string{
				"log analysis",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			cfg := &config.Config{
				Prompts: config.PromptsConfig{},
			}

			// Setup external file if specified
			if tt.externalPromptPath != "" {
				filePath := filepath.Join(tmpDir, tt.externalPromptPath)
				err := os.WriteFile(filePath, []byte(tt.externalContent), 0600)
				if err != nil {
					t.Fatalf("Failed to write external prompt: %v", err)
				}
				cfg.Prompts.SystemPrompt = filePath
			}

			loader := NewPromptLoader(cfg)
			prompt, err := loader.SystemPrompt(tt.ignoreInstructions)
			if err != nil {
				t.Fatalf("SystemPrompt() error = %v", err)
			}

			for _, want := range tt.wantContains {
				if !strings.Contains(prompt, want) {
					t.Errorf("SystemPrompt() missing expected content: %q", want)
				}
			}

			for _, notWant := range tt.wantNotContains {
				if strings.Contains(prompt, notWant) {
					t.Errorf("SystemPrompt() contains unexpected content: %q", notWant)
				}
			}
		})
	}
}

func TestPromptLoader_AnalysisPrompt(t *testing.T) {
	tests := []struct {
		name          string
		containerName string
		logs          string
		logCount      int
		wantContains  []string
	}{
		{
			name:          "basic analysis prompt",
			containerName: "test-container",
			logs:          "Sample log line 1\nSample log line 2",
			logCount:      2,
			wantContains: []string{
				"test-container",
				"Sample log line 1",
				"Sample log line 2",
			},
		},
		{
			name:          "empty logs",
			containerName: "empty-container",
			logs:          "",
			logCount:      0,
			wantContains: []string{
				"empty-container",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{}
			loader := NewPromptLoader(cfg)

			prompt, err := loader.AnalysisPrompt(tt.containerName, tt.logs, tt.logCount)
			if err != nil {
				t.Fatalf("AnalysisPrompt() error = %v", err)
			}

			for _, want := range tt.wantContains {
				if !strings.Contains(prompt, want) {
					t.Errorf("AnalysisPrompt() missing expected content: %q", want)
				}
			}
		})
	}
}

func TestPromptLoader_ChunkSummaryPrompt(t *testing.T) {
	tests := []struct {
		name          string
		containerName string
		chunkNum      int
		totalChunks   int
		logs          string
		wantContains  []string
	}{
		{
			name:          "chunk summary",
			containerName: "test-container",
			chunkNum:      1,
			totalChunks:   3,
			logs:          "Chunk logs here",
			wantContains: []string{
				"test-container",
				"Chunk logs here",
			},
		},
		{
			name:          "last chunk",
			containerName: "test-container",
			chunkNum:      5,
			totalChunks:   5,
			logs:          "Final chunk",
			wantContains: []string{
				"test-container",
				"Final chunk",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{}
			loader := NewPromptLoader(cfg)

			prompt, err := loader.ChunkSummaryPrompt(tt.containerName, tt.chunkNum, tt.totalChunks, tt.logs)
			if err != nil {
				t.Fatalf("ChunkSummaryPrompt() error = %v", err)
			}

			for _, want := range tt.wantContains {
				if !strings.Contains(prompt, want) {
					t.Errorf("ChunkSummaryPrompt() missing expected content: %q", want)
				}
			}
		})
	}
}

func TestPromptLoader_SynthesisPrompt(t *testing.T) {
	tests := []struct {
		name          string
		containerName string
		summaries     []string
		wantContains  []string
	}{
		{
			name:          "single summary",
			containerName: "test-container",
			summaries:     []string{"Summary 1"},
			wantContains: []string{
				"test-container",
				"Summary 1",
			},
		},
		{
			name:          "multiple summaries",
			containerName: "test-container",
			summaries:     []string{"Summary 1", "Summary 2", "Summary 3"},
			wantContains: []string{
				"test-container",
				"Summary 1",
				"Summary 2",
				"Summary 3",
			},
		},
		{
			name:          "empty summaries",
			containerName: "test-container",
			summaries:     []string{},
			wantContains: []string{
				"test-container",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{}
			loader := NewPromptLoader(cfg)

			prompt, err := loader.SynthesisPrompt(tt.containerName, tt.summaries)
			if err != nil {
				t.Fatalf("SynthesisPrompt() error = %v", err)
			}

			for _, want := range tt.wantContains {
				if !strings.Contains(prompt, want) {
					t.Errorf("SynthesisPrompt() missing expected content: %q", want)
				}
			}
		})
	}
}

func TestPromptLoader_ExecutiveSummaryPrompt(t *testing.T) {
	tests := []struct {
		name             string
		containerResults map[string]string
		wantContains     []string
	}{
		{
			name: "single container",
			containerResults: map[string]string{
				"container1": "Analysis for container 1",
			},
			wantContains: []string{
				"container1",
				"Analysis for container 1",
			},
		},
		{
			name: "multiple containers",
			containerResults: map[string]string{
				"container1": "Analysis 1",
				"container2": "Analysis 2",
			},
			wantContains: []string{
				"container1",
				"Analysis 1",
				"container2",
				"Analysis 2",
			},
		},
		{
			name:             "no containers",
			containerResults: map[string]string{},
			wantContains:     []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{}
			loader := NewPromptLoader(cfg)

			prompt, err := loader.ExecutiveSummaryPrompt(tt.containerResults)
			if err != nil {
				t.Fatalf("ExecutiveSummaryPrompt() error = %v", err)
			}

			for _, want := range tt.wantContains {
				if !strings.Contains(prompt, want) {
					t.Errorf("ExecutiveSummaryPrompt() missing expected content: %q", want)
				}
			}
		})
	}
}

func TestPromptLoader_GetPromptSource(t *testing.T) {
	cfg := &config.Config{}
	loader := NewPromptLoader(cfg)

	// Load a prompt to populate sources
	_, err := loader.SystemPrompt("")
	if err != nil {
		t.Fatalf("Failed to load system prompt: %v", err)
	}

	source := loader.GetPromptSource("system_prompt")
	if source == "" {
		t.Error("Expected non-empty source")
	}
	if source == "UNKNOWN" {
		t.Error("Expected known source, got UNKNOWN")
	}

	// Test unknown prompt
	unknownSrc := loader.GetPromptSource("nonexistent")
	if unknownSrc != unknownSource {
		t.Errorf("Expected UNKNOWN for nonexistent prompt, got %q", unknownSrc)
	}
}

func TestPromptLoader_GetAllPromptSources(t *testing.T) {
	cfg := &config.Config{}
	loader := NewPromptLoader(cfg)

	// Initially empty
	sources := loader.GetAllPromptSources()
	if len(sources) != 0 {
		t.Error("Expected empty sources initially")
	}

	// Load some prompts
	_, _ = loader.SystemPrompt("")
	_, _ = loader.AnalysisPrompt("test", "logs", 10)

	sources = loader.GetAllPromptSources()
	if len(sources) == 0 {
		t.Error("Expected non-empty sources after loading prompts")
	}
}

func TestPromptLoader_ExternalPromptFallback(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a non-existent path
	nonExistentPath := filepath.Join(tmpDir, "nonexistent.md")

	cfg := &config.Config{
		Prompts: config.PromptsConfig{
			SystemPrompt: nonExistentPath,
		},
	}

	loader := NewPromptLoader(cfg)

	// Should fall back to embedded default
	prompt, err := loader.SystemPrompt("")
	if err != nil {
		t.Fatalf("SystemPrompt() should fallback, got error: %v", err)
	}

	if prompt == "" {
		t.Error("Expected non-empty prompt from fallback")
	}

	// Check it's using internal default
	source := loader.GetPromptSource("system_prompt")
	if source != "INTERNAL DEFAULT" {
		t.Errorf("Expected INTERNAL DEFAULT source, got %q", source)
	}
}

func TestLegacyWrappers(t *testing.T) {
	t.Run("InitPrompts and SystemPrompt wrapper", func(t *testing.T) {
		cfg := &config.Config{}
		InitPrompts(cfg)

		prompt := SystemPrompt("")
		if prompt == "" {
			t.Error("SystemPrompt wrapper returned empty string")
		}

		promptWithInstructions := SystemPrompt("Custom instructions")
		if !strings.Contains(promptWithInstructions, "Custom instructions") {
			t.Error("SystemPrompt wrapper didn't include instructions")
		}
	})

	t.Run("AnalysisPrompt wrapper", func(t *testing.T) {
		cfg := &config.Config{}
		InitPrompts(cfg)

		prompt := AnalysisPrompt("test-container", "sample logs", 5)
		if prompt == "" {
			t.Error("AnalysisPrompt wrapper returned empty string")
		}
		if !strings.Contains(prompt, "test-container") {
			t.Error("AnalysisPrompt wrapper didn't include container name")
		}
	})

	t.Run("ChunkSummaryPrompt wrapper", func(t *testing.T) {
		cfg := &config.Config{}
		InitPrompts(cfg)

		prompt := ChunkSummaryPrompt("test-container", 1, 3, "chunk logs")
		if prompt == "" {
			t.Error("ChunkSummaryPrompt wrapper returned empty string")
		}
	})

	t.Run("SynthesisPrompt wrapper", func(t *testing.T) {
		cfg := &config.Config{}
		InitPrompts(cfg)

		summaries := []string{"Summary 1", "Summary 2"}
		prompt := SynthesisPrompt("test-container", summaries)
		if prompt == "" {
			t.Error("SynthesisPrompt wrapper returned empty string")
		}
	})

	t.Run("ExecutiveSummaryPrompt wrapper", func(t *testing.T) {
		cfg := &config.Config{}
		InitPrompts(cfg)

		results := map[string]string{
			"container1": "Analysis 1",
		}
		prompt := ExecutiveSummaryPrompt(results)
		if prompt == "" {
			t.Error("ExecutiveSummaryPrompt wrapper returned empty string")
		}
	})
}

func TestLegacyWrappers_Uninitialized(t *testing.T) {
	// Reset default loader
	defaultLoader = nil

	t.Run("SystemPrompt without init", func(t *testing.T) {
		prompt := SystemPrompt("")
		if prompt == "" {
			t.Error("SystemPrompt should return fallback when uninitialized")
		}

		promptWithInstructions := SystemPrompt("Test instructions")
		if !strings.Contains(promptWithInstructions, "Test instructions") {
			t.Error("SystemPrompt fallback should include instructions")
		}
	})

	t.Run("AnalysisPrompt without init", func(t *testing.T) {
		prompt := AnalysisPrompt("test", "logs", 5)
		if prompt == "" {
			t.Error("AnalysisPrompt should return fallback when uninitialized")
		}
	})

	t.Run("ChunkSummaryPrompt without init", func(t *testing.T) {
		prompt := ChunkSummaryPrompt("test", 1, 3, "logs")
		if prompt == "" {
			t.Error("ChunkSummaryPrompt should return fallback when uninitialized")
		}
	})

	t.Run("SynthesisPrompt without init", func(t *testing.T) {
		prompt := SynthesisPrompt("test", []string{"summary"})
		if prompt == "" {
			t.Error("SynthesisPrompt should return fallback when uninitialized")
		}
	})

	t.Run("ExecutiveSummaryPrompt without init", func(t *testing.T) {
		prompt := ExecutiveSummaryPrompt(map[string]string{"test": "analysis"})
		if prompt == "" {
			t.Error("ExecutiveSummaryPrompt should return fallback when uninitialized")
		}
	})
}

func TestGetDefaultLoader(t *testing.T) {
	// Initialize
	cfg := &config.Config{}
	InitPrompts(cfg)

	loader := GetDefaultLoader()
	if loader == nil {
		t.Error("GetDefaultLoader returned nil after initialization")
	}

	// Reset
	defaultLoader = nil
	loader = GetDefaultLoader()
	if loader != nil {
		t.Error("GetDefaultLoader should return nil when not initialized")
	}
}

func TestPromptLoader_InvalidTemplate(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a file with invalid template syntax
	invalidTemplatePath := filepath.Join(tmpDir, "invalid_template.md")
	err := os.WriteFile(invalidTemplatePath, []byte("{{.InvalidSyntax"), 0600)
	if err != nil {
		t.Fatalf("Failed to write invalid template: %v", err)
	}

	cfg := &config.Config{
		Prompts: config.PromptsConfig{
			AnalysisPrompt: invalidTemplatePath,
		},
	}

	loader := NewPromptLoader(cfg)

	_, err = loader.AnalysisPrompt("test", "logs", 5)
	if err == nil {
		t.Error("Expected error for invalid template")
	}
}

func TestPromptLoader_TemplateExecutionError(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a template that will fail during execution
	templatePath := filepath.Join(tmpDir, "bad_exec.md")
	// Using undefined variable will cause execution error
	err := os.WriteFile(templatePath, []byte("{{.UndefinedField}}"), 0600)
	if err != nil {
		t.Fatalf("Failed to write template: %v", err)
	}

	cfg := &config.Config{
		Prompts: config.PromptsConfig{
			AnalysisPrompt: templatePath,
		},
	}

	loader := NewPromptLoader(cfg)

	_, err = loader.AnalysisPrompt("test", "logs", 5)
	if err == nil {
		t.Error("Expected error for template execution failure")
	}
}

func TestPromptLoader_AllPromptsWithExternalFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Create external prompt files
	files := map[string]string{
		"system.md":        "Custom system prompt",
		"analysis.md":      "Container: {{.ContainerName}}\nLogs: {{.Logs}}",
		"chunk_summary.md": "Chunk {{.ChunkNum}}/{{.TotalChunks}}: {{.Logs}}",
		"synthesis.md":     "Container: {{.ContainerName}}\n{{.Summaries}}",
		"executive.md":     "Count: {{.ContainerCount}}\n{{.ContainerAnalyses}}",
	}

	for filename, content := range files {
		path := filepath.Join(tmpDir, filename)
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatalf("Failed to write %s: %v", filename, err)
		}
	}

	cfg := &config.Config{
		Prompts: config.PromptsConfig{
			SystemPrompt:           filepath.Join(tmpDir, "system.md"),
			AnalysisPrompt:         filepath.Join(tmpDir, "analysis.md"),
			ChunkSummaryPrompt:     filepath.Join(tmpDir, "chunk_summary.md"),
			SynthesisPrompt:        filepath.Join(tmpDir, "synthesis.md"),
			ExecutiveSummaryPrompt: filepath.Join(tmpDir, "executive.md"),
		},
	}

	loader := NewPromptLoader(cfg)

	// Test all prompts use external files
	t.Run("system prompt", func(t *testing.T) {
		prompt, err := loader.SystemPrompt("")
		if err != nil {
			t.Fatalf("Error: %v", err)
		}
		if !strings.Contains(prompt, "Custom system prompt") {
			t.Error("Expected custom system prompt content")
		}
		if !strings.Contains(loader.GetPromptSource("system_prompt"), "EXTERNAL") {
			t.Error("Expected EXTERNAL source")
		}
	})

	t.Run("analysis prompt", func(t *testing.T) {
		prompt, err := loader.AnalysisPrompt("test", "log data", 5)
		if err != nil {
			t.Fatalf("Error: %v", err)
		}
		if !strings.Contains(prompt, "Container: test") || !strings.Contains(prompt, "Logs: log data") {
			t.Error("Expected custom analysis prompt content")
		}
	})

	t.Run("chunk summary prompt", func(t *testing.T) {
		prompt, err := loader.ChunkSummaryPrompt("test", 2, 5, "chunk data")
		if err != nil {
			t.Fatalf("Error: %v", err)
		}
		if !strings.Contains(prompt, "Chunk 2/5") {
			t.Error("Expected custom chunk summary content")
		}
	})

	t.Run("synthesis prompt", func(t *testing.T) {
		prompt, err := loader.SynthesisPrompt("test", []string{"sum1", "sum2"})
		if err != nil {
			t.Fatalf("Error: %v", err)
		}
		if !strings.Contains(prompt, "Container: test") {
			t.Error("Expected custom synthesis content")
		}
	})

	t.Run("executive summary prompt", func(t *testing.T) {
		results := map[string]string{"c1": "a1"}
		prompt, err := loader.ExecutiveSummaryPrompt(results)
		if err != nil {
			t.Fatalf("Error: %v", err)
		}
		if !strings.Contains(prompt, "Count: 1") {
			t.Error("Expected custom executive summary content")
		}
	})
}

func BenchmarkPromptLoader_SystemPrompt(b *testing.B) {
	cfg := &config.Config{}
	loader := NewPromptLoader(cfg)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = loader.SystemPrompt("")
	}
}

func BenchmarkPromptLoader_AnalysisPrompt(b *testing.B) {
	cfg := &config.Config{}
	loader := NewPromptLoader(cfg)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = loader.AnalysisPrompt("test", "logs", 100)
	}
}

func BenchmarkLegacyWrappers(b *testing.B) {
	cfg := &config.Config{}
	InitPrompts(cfg)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = SystemPrompt("")
		_ = AnalysisPrompt("test", "logs", 10)
		_ = ChunkSummaryPrompt("test", 1, 3, "logs")
		_ = SynthesisPrompt("test", []string{"sum"})
		_ = ExecutiveSummaryPrompt(map[string]string{"test": "analysis"})
	}
}
