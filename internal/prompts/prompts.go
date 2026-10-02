// Package prompts manages AI prompts and templates.
package prompts

import (
	"bytes"
	"crypto/rand"
	"embed"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"text/template"

	"github.com/zorak1103/dlia/internal/config"
	"github.com/zorak1103/dlia/internal/severity"
)

//go:embed defaults/*.md
var embeddedPrompts embed.FS

// PromptLoader manages loading prompts from files or embedded defaults
type PromptLoader struct {
	cfg           *config.Config
	mu            sync.RWMutex      // protects promptSources map
	promptSources map[string]string // tracks source of each prompt (for introspection)
	rand          io.Reader         // source of boundary marker randomness
}

// NewPromptLoader initializes prompt loading with external file overrides from config.
func NewPromptLoader(cfg *config.Config) *PromptLoader {
	return &PromptLoader{
		cfg: cfg,
		// Typical: 5 prompt types (system, analysis, chunk_summary, synthesis, executive_summary)
		promptSources: make(map[string]string, 5),
		rand:          rand.Reader,
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

// Messages is a ready-to-send system/user prompt pair for one LLM call.
type Messages struct {
	System string
	User   string
}

// callSpec describes how the untrusted data of one kind of LLM call is fenced.
type callSpec struct {
	kind         string // marker kind: logs, summaries or analyses
	subject      string // what the enclosed data is called in the data rule
	source       string // where the enclosed data comes from, for the data rule
	withSeverity bool   // analysis/synthesis only: severity clause and instruction
}

var (
	analysisSpec  = callSpec{"logs", "log data", "the monitored container", true}
	chunkSpec     = callSpec{"logs", "log data", "the monitored container", false}
	synthesisSpec = callSpec{"summaries", "chunk summaries", "summaries of the monitored container's logs", true}
	execSpec      = callSpec{"analyses", "per-container analyses", "analyses of the monitored containers' logs", false}
)

// systemPrompt returns the base system prompt, optionally extended with ignore instructions.
func (pl *PromptLoader) systemPrompt(ignoreInstructions string) (string, error) {
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

// renderTemplate loads the named prompt template (external override or embedded
// default) and executes it with data.
func (pl *PromptLoader) renderTemplate(name, externalPath string, data map[string]interface{}) (string, error) {
	templateContent, err := pl.loadPrompt(name, "defaults/"+name+".md", externalPath)
	if err != nil {
		return "", err
	}

	tmplName := strings.TrimSuffix(name, "_prompt")
	label := strings.ReplaceAll(tmplName, "_", " ")
	tmpl, err := template.New(tmplName).Option("missingkey=error").Parse(templateContent)
	if err != nil {
		return "", fmt.Errorf("failed to parse %s template: %w", label, err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute %s template: %w", label, err)
	}

	return buf.String(), nil
}

// combineSummaries joins chunk summaries with numbered separators.
func combineSummaries(summaries []string) string {
	var sb strings.Builder
	for i, summary := range summaries {
		fmt.Fprintf(&sb, "\n--- Chunk %d Summary ---\n%s\n", i+1, summary)
	}
	return sb.String()
}

// combineAnalyses joins per-container analyses under "### name" headings.
func combineAnalyses(analyses map[string]string) string {
	var sb strings.Builder
	for containerName, analysis := range analyses {
		fmt.Fprintf(&sb, "### %s\n%s\n\n", containerName, analysis)
	}
	return sb.String()
}

// buildMessages fences data with a fresh per-call marker, renders the user prompt
// via render (which receives the wrapped data) and builds the matching system
// prompt: base prompt, optional ignore instructions, then the data rule.
func (pl *PromptLoader) buildMessages(spec callSpec, ignoreInstructions, data string, render func(wrapped string) (string, error)) (Messages, error) {
	m, err := newMarker(spec.kind, data, pl.rand)
	if err != nil {
		return Messages{}, err
	}

	user, err := render(m.wrap(data))
	if err != nil {
		return Messages{}, err
	}
	if spec.withSeverity {
		user += "\n\n" + severity.Instruction
	}

	system, err := pl.systemPrompt(ignoreInstructions)
	if err != nil {
		return Messages{}, err
	}
	system += "\n\n" + dataRule(m, spec.subject, spec.source, spec.withSeverity)

	return Messages{System: system, User: user}, nil
}

// AnalysisMessages builds the system/user prompts for analyzing logs in one call.
func (pl *PromptLoader) AnalysisMessages(containerName, ignoreInstructions, logs string, logCount int) (Messages, error) {
	return pl.buildMessages(analysisSpec, ignoreInstructions, logs, func(wrapped string) (string, error) {
		return pl.analysisPrompt(containerName, wrapped, logCount)
	})
}

// ChunkMessages builds the system/user prompts for summarizing a single log chunk.
func (pl *PromptLoader) ChunkMessages(containerName, ignoreInstructions string, chunkNum, totalChunks int, logs string) (Messages, error) {
	return pl.buildMessages(chunkSpec, ignoreInstructions, logs, func(wrapped string) (string, error) {
		return pl.chunkSummaryPrompt(containerName, chunkNum, totalChunks, wrapped)
	})
}

// SynthesisMessages builds the system/user prompts for combining chunk summaries.
func (pl *PromptLoader) SynthesisMessages(containerName, ignoreInstructions string, summaries []string) (Messages, error) {
	return pl.buildMessages(synthesisSpec, ignoreInstructions, combineSummaries(summaries), func(wrapped string) (string, error) {
		return pl.synthesisPrompt(containerName, wrapped)
	})
}

// ExecutiveSummaryMessages builds the system/user prompts for the cross-container summary.
func (pl *PromptLoader) ExecutiveSummaryMessages(containerAnalyses map[string]string) (Messages, error) {
	return pl.buildMessages(execSpec, "", combineAnalyses(containerAnalyses), func(wrapped string) (string, error) {
		return pl.executiveSummaryPrompt(len(containerAnalyses), wrapped)
	})
}

// analysisPrompt renders the log analysis template with container context.
func (pl *PromptLoader) analysisPrompt(containerName, logs string, logCount int) (string, error) {
	return pl.renderTemplate("analysis_prompt", pl.cfg.Prompts.AnalysisPrompt, map[string]interface{}{
		"ContainerName": containerName,
		"Logs":          logs,
		"LogCount":      logCount,
	})
}

// chunkSummaryPrompt renders the template for summarizing a single log chunk.
func (pl *PromptLoader) chunkSummaryPrompt(containerName string, chunkNum, totalChunks int, logs string) (string, error) {
	return pl.renderTemplate("chunk_summary_prompt", pl.cfg.Prompts.ChunkSummaryPrompt, map[string]interface{}{
		"ContainerName": containerName,
		"ChunkNum":      chunkNum,
		"TotalChunks":   totalChunks,
		"Logs":          logs,
	})
}

// synthesisPrompt renders the template for combining multiple chunk summaries.
func (pl *PromptLoader) synthesisPrompt(containerName, combinedSummaries string) (string, error) {
	return pl.renderTemplate("synthesis_prompt", pl.cfg.Prompts.SynthesisPrompt, map[string]interface{}{
		"ContainerName": containerName,
		"Summaries":     combinedSummaries,
	})
}

// executiveSummaryPrompt renders the template for cross-container summary generation.
func (pl *PromptLoader) executiveSummaryPrompt(containerCount int, combinedAnalyses string) (string, error) {
	return pl.renderTemplate("executive_summary_prompt", pl.cfg.Prompts.ExecutiveSummaryPrompt, map[string]interface{}{
		"ContainerCount":    containerCount,
		"ContainerAnalyses": combinedAnalyses,
	})
}

// defaultLoader is a process-wide PromptLoader used by GetDefaultLoader for
// introspection (e.g. reporting which prompt sources are active). Callers
// that need to render prompts should construct their own PromptLoader via
// NewPromptLoader instead of going through this global.
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

// GetDefaultLoader returns the default prompt loader for introspection
func GetDefaultLoader() *PromptLoader {
	return getDefaultLoader()
}
