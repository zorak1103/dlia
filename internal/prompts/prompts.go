// Package prompts manages AI prompts and templates.
package prompts

import (
	"bytes"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"text/template"

	"github.com/zorak1103/dlia/internal/config"
)

//go:embed defaults/*.md
var embeddedPrompts embed.FS

// PromptLoader manages loading prompts from files or embedded defaults
type PromptLoader struct {
	cfg           *config.Config
	mu            sync.RWMutex      // protects promptSources map
	promptSources map[string]string // tracks source of each prompt (for introspection)
}

// NewPromptLoader initializes prompt loading with external file overrides from config.
func NewPromptLoader(cfg *config.Config) *PromptLoader {
	return &PromptLoader{
		cfg: cfg,
		// Typical: 5 prompt types (system, analysis, chunk_summary, synthesis, executive_summary)
		promptSources: make(map[string]string, 5),
	}
}

// loadPrompt loads a prompt from external file or embedded default
func (pl *PromptLoader) loadPrompt(name, embeddedPath, externalPath string) (string, error) {
	// Try external file first if specified
	if externalPath != "" {
		// Clean path to prevent directory traversal
		cleanPath := filepath.Clean(externalPath)
		content, err := os.ReadFile(cleanPath)
		if err == nil {
			pl.mu.Lock()
			pl.promptSources[name] = fmt.Sprintf("EXTERNAL: %s", cleanPath)
			pl.mu.Unlock()
			return string(content), nil
		}
		// Log warning but fall back to embedded
		fmt.Printf("⚠️  Warning: Could not read %s from %s: %v\n", name, cleanPath, err)
		fmt.Printf("   Falling back to built-in default\n")
	}

	// Use embedded default
	content, err := embeddedPrompts.ReadFile(embeddedPath)
	if err != nil {
		return "", fmt.Errorf("failed to load embedded prompt %s: %w", embeddedPath, err)
	}

	pl.mu.Lock()
	pl.promptSources[name] = "INTERNAL DEFAULT"
	pl.mu.Unlock()
	return string(content), nil
}

// GetPromptSource returns the source of a prompt (for introspection)
func (pl *PromptLoader) GetPromptSource(name string) string {
	pl.mu.RLock()
	defer pl.mu.RUnlock()
	if source, exists := pl.promptSources[name]; exists {
		return source
	}
	return "UNKNOWN"
}

// GetAllPromptSources returns all prompt sources for introspection
func (pl *PromptLoader) GetAllPromptSources() map[string]string {
	pl.mu.RLock()
	defer pl.mu.RUnlock()
	// Return a copy to prevent external modification
	sources := make(map[string]string, len(pl.promptSources))
	for k, v := range pl.promptSources {
		sources[k] = v
	}
	return sources
}

// SystemPrompt returns the base system prompt, optionally extended with ignore instructions.
func (pl *PromptLoader) SystemPrompt(ignoreInstructions string) (string, error) {
	basePrompt, err := pl.loadPrompt(
		"system_prompt",
		"defaults/system_prompt.md",
		pl.cfg.Prompts.SystemPrompt,
	)
	if err != nil {
		return "", err
	}

	if ignoreInstructions != "" {
		basePrompt += fmt.Sprintf("\n\nUser Instructions for this container:\n%s", ignoreInstructions)
	}

	return basePrompt, nil
}

// AnalysisPrompt renders the log analysis template with container context.
func (pl *PromptLoader) AnalysisPrompt(containerName, logs string, logCount int) (string, error) {
	templateContent, err := pl.loadPrompt(
		"analysis_prompt",
		"defaults/analysis_prompt.md",
		pl.cfg.Prompts.AnalysisPrompt,
	)
	if err != nil {
		return "", err
	}

	tmpl, err := template.New("analysis").Option("missingkey=error").Parse(templateContent)
	if err != nil {
		return "", fmt.Errorf("failed to parse analysis template: %w", err)
	}

	data := map[string]interface{}{
		"ContainerName": containerName,
		"Logs":          logs,
		"LogCount":      logCount,
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute analysis template: %w", err)
	}

	return buf.String(), nil
}

// ChunkSummaryPrompt renders the template for summarizing a single log chunk.
func (pl *PromptLoader) ChunkSummaryPrompt(containerName string, chunkNum, totalChunks int, logs string) (string, error) {
	templateContent, err := pl.loadPrompt(
		"chunk_summary_prompt",
		"defaults/chunk_summary_prompt.md",
		pl.cfg.Prompts.ChunkSummaryPrompt,
	)
	if err != nil {
		return "", err
	}

	tmpl, err := template.New("chunk_summary").Option("missingkey=error").Parse(templateContent)
	if err != nil {
		return "", fmt.Errorf("failed to parse chunk summary template: %w", err)
	}

	data := map[string]interface{}{
		"ContainerName": containerName,
		"ChunkNum":      chunkNum,
		"TotalChunks":   totalChunks,
		"Logs":          logs,
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute chunk summary template: %w", err)
	}

	return buf.String(), nil
}

// SynthesisPrompt renders the template for combining multiple chunk summaries.
func (pl *PromptLoader) SynthesisPrompt(containerName string, summaries []string) (string, error) {
	templateContent, err := pl.loadPrompt(
		"synthesis_prompt",
		"defaults/synthesis_prompt.md",
		pl.cfg.Prompts.SynthesisPrompt,
	)
	if err != nil {
		return "", err
	}

	// Build summaries section
	combined := ""
	for i, summary := range summaries {
		combined += fmt.Sprintf("\n--- Chunk %d Summary ---\n%s\n", i+1, summary)
	}

	tmpl, err := template.New("synthesis").Option("missingkey=error").Parse(templateContent)
	if err != nil {
		return "", fmt.Errorf("failed to parse synthesis template: %w", err)
	}

	data := map[string]interface{}{
		"ContainerName": containerName,
		"Summaries":     combined,
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute synthesis template: %w", err)
	}

	return buf.String(), nil
}

// ExecutiveSummaryPrompt renders the template for cross-container summary generation.
func (pl *PromptLoader) ExecutiveSummaryPrompt(containerResults map[string]string) (string, error) {
	templateContent, err := pl.loadPrompt(
		"executive_summary_prompt",
		"defaults/executive_summary_prompt.md",
		pl.cfg.Prompts.ExecutiveSummaryPrompt,
	)
	if err != nil {
		return "", err
	}

	// Build container analyzes section
	var sb strings.Builder
	for containerName, analysis := range containerResults {
		sb.WriteString(fmt.Sprintf("### %s\n%s\n\n", containerName, analysis))
	}

	tmpl, err := template.New("executive_summary").Option("missingkey=error").Parse(templateContent)
	if err != nil {
		return "", fmt.Errorf("failed to parse executive summary template: %w", err)
	}

	data := map[string]interface{}{
		"ContainerCount":    len(containerResults),
		"ContainerAnalyses": sb.String(),
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute executive summary template: %w", err)
	}

	return buf.String(), nil
}

// Legacy wrapper functions for backward compatibility
// These maintain the original API but use the loader internally

var (
	defaultLoader   *PromptLoader
	defaultLoaderMu sync.RWMutex
)

// InitPrompts sets up the global prompt loader used by legacy wrapper functions.
func InitPrompts(cfg *config.Config) {
	defaultLoaderMu.Lock()
	defer defaultLoaderMu.Unlock()
	defaultLoader = NewPromptLoader(cfg)
}

// getDefaultLoader safely retrieves the default loader
func getDefaultLoader() *PromptLoader {
	defaultLoaderMu.RLock()
	defer defaultLoaderMu.RUnlock()
	return defaultLoader
}

// SystemPrompt returns the system prompt (legacy wrapper)
func SystemPrompt(ignoreInstructions string) string {
	loader := getDefaultLoader()
	if loader == nil {
		// Fallback to simple default if not initialized
		basePrompt := `You are an expert log analysis assistant for Docker containers. Your role is to:

1. Analyze container logs for issues, errors, and anomalies
2. Identify security concerns or suspicious patterns
3. Detect performance problems or degradation
4. Distinguish between routine logs and actual problems

Focus on providing actionable insights. Ignore routine informational logs unless they reveal patterns.
Be concise and specific in your findings.`
		if ignoreInstructions != "" {
			basePrompt += fmt.Sprintf("\n\nUser Instructions for this container:\n%s", ignoreInstructions)
		}
		return basePrompt
	}

	prompt, err := loader.SystemPrompt(ignoreInstructions)
	if err != nil {
		fmt.Printf("⚠️  Error loading system prompt: %v\n", err)
		return "You are a log analysis assistant."
	}
	return prompt
}

// AnalysisPrompt creates a prompt for analyzing logs (legacy wrapper)
func AnalysisPrompt(containerName, logs string, logCount int) string {
	loader := getDefaultLoader()
	if loader == nil {
		return fmt.Sprintf(`Analyze these %d log entries from container "%s":

%s

Provide a structured analysis:
1. **Summary**: Brief overview of log activity
2. **Errors**: Any errors or exceptions found (be specific)
3. **Warnings**: Potential issues or concerns
4. **Recommendations**: Suggested actions if needed

If logs are routine with no issues, state "No significant issues detected."`, logCount, containerName, logs)
	}

	prompt, err := loader.AnalysisPrompt(containerName, logs, logCount)
	if err != nil {
		fmt.Printf("⚠️  Error loading analysis prompt: %v\n", err)
		return fmt.Sprintf("Analyze these logs from %s", containerName)
	}
	return prompt
}

// ChunkSummaryPrompt creates a prompt for summarizing a chunk (legacy wrapper)
func ChunkSummaryPrompt(containerName string, chunkNum, totalChunks int, logs string) string {
	loader := getDefaultLoader()
	if loader == nil {
		return fmt.Sprintf(`This is chunk %d of %d from container "%s".
Summarize the key findings from these logs. Focus on errors, warnings, and anomalies.
Be concise:

%s`, chunkNum, totalChunks, containerName, logs)
	}

	prompt, err := loader.ChunkSummaryPrompt(containerName, chunkNum, totalChunks, logs)
	if err != nil {
		fmt.Printf("⚠️  Error loading chunk summary prompt: %v\n", err)
		return fmt.Sprintf("Summarize chunk %d of %d", chunkNum, totalChunks)
	}
	return prompt
}

// SynthesisPrompt creates a prompt for synthesizing multiple summaries (legacy wrapper)
func SynthesisPrompt(containerName string, summaries []string) string {
	loader := getDefaultLoader()
	if loader == nil {
		combined := ""
		for i, summary := range summaries {
			combined += fmt.Sprintf("\n--- Chunk %d Summary ---\n%s\n", i+1, summary)
		}
		return fmt.Sprintf(`These are summaries from analyzing logs for container "%s" in multiple chunks.
Provide a final comprehensive analysis:

%s

Final Analysis:
1. **Summary**: Overall findings across all chunks
2. **Critical Issues**: Most important errors or problems
3. **Patterns**: Any recurring issues or trends
4. **Recommendations**: Priority actions needed`, containerName, combined)
	}

	prompt, err := loader.SynthesisPrompt(containerName, summaries)
	if err != nil {
		fmt.Printf("⚠️  Error loading synthesis prompt: %v\n", err)
		return fmt.Sprintf("Synthesize summaries for %s", containerName)
	}
	return prompt
}

// ExecutiveSummaryPrompt creates a prompt for generating an executive summary (legacy wrapper)
func ExecutiveSummaryPrompt(containerResults map[string]string) string {
	loader := getDefaultLoader()
	if loader == nil {
		var sb strings.Builder
		sb.WriteString("Generate a concise executive summary of this Docker log scan.\n\n")
		sb.WriteString(fmt.Sprintf("Analyzed %d container(s):\n\n", len(containerResults)))
		for containerName, analysis := range containerResults {
			sb.WriteString(fmt.Sprintf("### %s\n%s\n\n", containerName, analysis))
		}
		sb.WriteString(`Create a brief executive summary (max 250 words) for notification delivery:

1. **Overall Status**: One-line health assessment
2. **Critical Issues**: List most urgent problems (if any)
3. **Action Required**: Yes/No and what action
4. **Affected Containers**: Which containers need attention

Keep it concise and actionable. Focus on what needs immediate attention.`)
		return sb.String()
	}

	prompt, err := loader.ExecutiveSummaryPrompt(containerResults)
	if err != nil {
		fmt.Printf("⚠️  Error loading executive summary prompt: %v\n", err)
		return "Generate executive summary"
	}
	return prompt
}

// GetDefaultLoader returns the default prompt loader for introspection
func GetDefaultLoader() *PromptLoader {
	return getDefaultLoader()
}
