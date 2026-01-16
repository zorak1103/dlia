package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/zorak1103/dlia/internal/config"
	"github.com/zorak1103/dlia/internal/docker"
	"github.com/zorak1103/dlia/internal/sanitize"
	"github.com/zorak1103/dlia/internal/state"
)

// getDockerContainerIDs queries the Docker daemon for all containers (running and stopped)
// and returns their IDs as a set for efficient membership testing during cleanup operations.
func getDockerContainerIDs(ctx context.Context, dockerClient docker.Client) (map[string]bool, error) {
	containers, err := dockerClient.ListContainers(ctx, docker.FilterOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list Docker containers: %w", err)
	}

	containerIDs := make(map[string]bool, len(containers))
	for _, container := range containers {
		containerIDs[container.ID] = true
	}

	return containerIDs, nil
}

// scanStateFile reads the state file and extracts all tracked container IDs.
// Returns an empty list if the state file doesn't exist (not an error condition).
func scanStateFile(cfg *config.Config) ([]string, error) {
	st, err := state.Load(cfg.Output.StateFile)
	if err != nil {
		// If state file doesn't exist, return empty list (not an error)
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("failed to load state file: %w", err)
	}

	containers := st.GetAllContainers()
	containerIDs := make([]string, 0, len(containers))
	for id := range containers {
		containerIDs = append(containerIDs, id)
	}

	return containerIDs, nil
}

// scanKnowledgeBase returns container names found in knowledge_base/services/*.md files
func scanKnowledgeBase(cfg *config.Config) ([]string, error) {
	kbServicesDir := filepath.Join(cfg.Output.KnowledgeBaseDir, "services")

	// If directory doesn't exist, return empty list (not an error)
	if _, err := os.Stat(kbServicesDir); os.IsNotExist(err) {
		return []string{}, nil
	}

	entries, err := os.ReadDir(kbServicesDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read knowledge base directory: %w", err)
	}

	containerNames := make([]string, 0)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		// Extract container name from .md files
		if strings.HasSuffix(entry.Name(), ".md") {
			// Remove .md extension and un-sanitize (replace _ with /)
			name := strings.TrimSuffix(entry.Name(), ".md")
			containerNames = append(containerNames, name)
		}
	}

	return containerNames, nil
}

// scanReports returns container names by scanning subdirectory names in reports/.
// Each subdirectory name corresponds to a sanitized container name.
// File contents are not examined, only directory structure.
func scanReports(cfg *config.Config) ([]string, error) {
	reportsDir := cfg.Output.ReportsDir

	// If directory doesn't exist, return empty list (not an error)
	if _, err := os.Stat(reportsDir); os.IsNotExist(err) {
		return []string{}, nil
	}

	entries, err := os.ReadDir(reportsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read reports directory: %w", err)
	}

	containerNames := make([]string, 0)
	for _, entry := range entries {
		if entry.IsDir() {
			// Directory name is the (sanitized) container name
			containerNames = append(containerNames, entry.Name())
		}
	}

	return containerNames, nil
}

// scanLLMLogs returns container names found in logs/llm/ directories (if LLM logging is enabled)
func scanLLMLogs(cfg *config.Config) ([]string, error) {
	// Skip if LLM logging is not enabled
	if !cfg.Output.LLMLogEnabled {
		return []string{}, nil
	}

	llmLogsDir := cfg.Output.LLMLogDir

	// If directory doesn't exist, return empty list (not an error)
	if _, err := os.Stat(llmLogsDir); os.IsNotExist(err) {
		return []string{}, nil
	}

	entries, err := os.ReadDir(llmLogsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read LLM logs directory: %w", err)
	}

	containerNames := make([]string, 0)
	for _, entry := range entries {
		if entry.IsDir() {
			// Directory name is the (sanitized) container name
			containerNames = append(containerNames, entry.Name())
		}
	}

	return containerNames, nil
}

// ObsoleteContainer represents a container that exists in storage but not in Docker
type ObsoleteContainer struct {
	ID        string // Container ID (from state file)
	Name      string // Container name
	InState   bool   // Present in state.json
	InKB      bool   // Present in knowledge_base/services/
	InReports bool   // Present in reports/
	InLLMLogs bool   // Present in logs/llm/
}

// findObsoleteContainers implements a two-phase cleanup detection algorithm:
// Phase 1: Query Docker for all existing containers (ground truth)
// Phase 2: Scan all storage locations (state, KB, reports, logs)
// Phase 3: Identify mismatches where storage references non-existent containers
// This approach ensures we never delete data for active containers, even if
// the state file is outdated.
func findObsoleteContainers(ctx context.Context, dockerClient docker.Client, cfg *config.Config) ([]ObsoleteContainer, error) {
	// Get set of existing Docker container IDs
	dockerIDs, err := getDockerContainerIDs(ctx, dockerClient)
	if err != nil {
		return nil, err
	}

	// Scan all storage locations
	storageMaps, err := scanAllStorageLocations(cfg)
	if err != nil {
		return nil, err
	}

	// Find obsolete containers from state
	obsoleteMap := findObsoleteFromState(cfg, dockerIDs)

	// Enrich with storage location flags
	enrichObsoleteWithStorageFlags(obsoleteMap, storageMaps)

	// Find orphaned entries
	findOrphanedEntries(cfg, dockerIDs, storageMaps, obsoleteMap)

	// Convert to sorted list
	return convertToSortedList(obsoleteMap), nil
}

// storageMaps holds maps of container names in different storage locations
type storageMaps struct {
	stateIDs   []string
	kbMap      map[string]bool
	reportsMap map[string]bool
	llmLogsMap map[string]bool
}

// scanAllStorageLocations scans all storage locations and returns maps
func scanAllStorageLocations(cfg *config.Config) (*storageMaps, error) {
	stateIDs, err := scanStateFile(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to scan state file: %w", err)
	}

	kbNames, err := scanKnowledgeBase(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to scan knowledge base: %w", err)
	}

	reportNames, err := scanReports(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to scan reports: %w", err)
	}

	llmLogNames, err := scanLLMLogs(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to scan LLM logs: %w", err)
	}

	// Build maps for quick lookup
	kbMap := make(map[string]bool, len(kbNames))
	for _, name := range kbNames {
		kbMap[name] = true
	}

	reportsMap := make(map[string]bool, len(reportNames))
	for _, name := range reportNames {
		reportsMap[name] = true
	}

	llmLogsMap := make(map[string]bool, len(llmLogNames))
	for _, name := range llmLogNames {
		llmLogsMap[name] = true
	}

	return &storageMaps{
		stateIDs:   stateIDs,
		kbMap:      kbMap,
		reportsMap: reportsMap,
		llmLogsMap: llmLogsMap,
	}, nil
}

// findObsoleteFromState finds containers in state that don't exist in Docker
func findObsoleteFromState(cfg *config.Config, dockerIDs map[string]bool) map[string]*ObsoleteContainer {
	obsoleteMap := make(map[string]*ObsoleteContainer)

	// Load state to get container info
	st, err := state.Load(cfg.Output.StateFile)
	if err != nil {
		return obsoleteMap
	}

	containers := st.GetAllContainers()
	for id, ctr := range containers {
		if !dockerIDs[id] {
			// Container in state but not in Docker - it's obsolete
			obsoleteMap[id] = &ObsoleteContainer{
				ID:      id,
				Name:    ctr.Name,
				InState: true,
			}
		}
	}

	return obsoleteMap
}

// enrichObsoleteWithStorageFlags adds storage location flags to obsolete containers
func enrichObsoleteWithStorageFlags(obsoleteMap map[string]*ObsoleteContainer, storageMaps *storageMaps) {
	for _, obsolete := range obsoleteMap {
		if obsolete.Name != "" {
			sanitized := sanitize.Name(obsolete.Name)
			if storageMaps.kbMap[sanitized] {
				obsolete.InKB = true
			}
			if storageMaps.reportsMap[sanitized] {
				obsolete.InReports = true
			}
			if storageMaps.llmLogsMap[sanitized] {
				obsolete.InLLMLogs = true
			}
		}
	}
}

// findOrphanedEntries finds storage entries that don't have corresponding Docker containers
func findOrphanedEntries(cfg *config.Config, dockerIDs map[string]bool, storageMaps *storageMaps, obsoleteMap map[string]*ObsoleteContainer) {
	// Collect all unique names from storage
	allNames := make(map[string]bool)
	for name := range storageMaps.kbMap {
		allNames[name] = true
	}
	for name := range storageMaps.reportsMap {
		allNames[name] = true
	}
	for name := range storageMaps.llmLogsMap {
		allNames[name] = true
	}

	// Load state to check against
	st, err := state.Load(cfg.Output.StateFile)
	if err != nil {
		return
	}

	containers := st.GetAllContainers()
	for name := range allNames {
		if isOrphanedEntry(name, containers, dockerIDs) {
			addOrphanedEntry(name, storageMaps, obsoleteMap)
		}
	}
}

// isOrphanedEntry checks if a storage name is orphaned (no corresponding Docker container)
func isOrphanedEntry(name string, containers map[string]*state.Container, dockerIDs map[string]bool) bool {
	for id, ctr := range containers {
		if sanitize.Name(ctr.Name) == name && dockerIDs[id] {
			return false
		}
	}
	return true
}

// addOrphanedEntry adds an orphaned storage entry to the obsolete map
func addOrphanedEntry(name string, storageMaps *storageMaps, obsoleteMap map[string]*ObsoleteContainer) {
	pseudoID := "orphaned-" + name
	if _, exists := obsoleteMap[pseudoID]; !exists {
		obsoleteMap[pseudoID] = &ObsoleteContainer{
			ID:        pseudoID,
			Name:      strings.ReplaceAll(name, "_", "/"), // Un-sanitize for display
			InState:   false,
			InKB:      storageMaps.kbMap[name],
			InReports: storageMaps.reportsMap[name],
			InLLMLogs: storageMaps.llmLogsMap[name],
		}
	}
}

// convertToSortedList converts obsolete map to sorted list.
// Sort by container ID for consistent output using Go's standard library.
// sort.Slice uses an optimized algorithm (typically introsort) that is O(n log n)
// and handles edge cases better than manual implementations.
func convertToSortedList(obsoleteMap map[string]*ObsoleteContainer) []ObsoleteContainer {
	obsoleteList := make([]ObsoleteContainer, 0, len(obsoleteMap))
	for _, obsolete := range obsoleteMap {
		obsoleteList = append(obsoleteList, *obsolete)
	}

	sort.Slice(obsoleteList, func(i, j int) bool {
		return obsoleteList[i].ID < obsoleteList[j].ID
	})

	return obsoleteList
}

// deleteFromState removes a container entry from state.json
func deleteFromState(containerID string, st *state.State) error {
	st.RemoveContainer(containerID)
	if err := st.Save(); err != nil {
		return fmt.Errorf("failed to save state after removing container: %w", err)
	}
	return nil
}

// deleteKnowledgeBase removes a container's knowledge base file
func deleteKnowledgeBase(containerName string, cfg *config.Config) error {
	if containerName == "" {
		return nil // No name, nothing to delete
	}

	sanitized := sanitize.Name(containerName)
	kbFile := filepath.Join(cfg.Output.KnowledgeBaseDir, "services", sanitized+".md")

	// Check if file exists
	if _, err := os.Stat(kbFile); os.IsNotExist(err) {
		return nil // File doesn't exist, nothing to delete
	}

	// Delete the file
	if err := os.Remove(kbFile); err != nil {
		// Check for permission errors
		if os.IsPermission(err) {
			return fmt.Errorf("permission denied deleting %s. Check file permissions", kbFile)
		}
		return fmt.Errorf("failed to delete knowledge base file %s: %w", kbFile, err)
	}

	return nil
}

// deleteReportsDir recursively removes a container's reports directory
func deleteReportsDir(containerName string, cfg *config.Config) error {
	if containerName == "" {
		return nil // No name, nothing to delete
	}

	sanitized := sanitize.Name(containerName)
	reportsDir := filepath.Join(cfg.Output.ReportsDir, sanitized)

	// Check if directory exists
	if _, err := os.Stat(reportsDir); os.IsNotExist(err) {
		return nil // Directory doesn't exist, nothing to delete
	}

	// Recursively delete the directory
	if err := os.RemoveAll(reportsDir); err != nil {
		// Check for permission errors
		if os.IsPermission(err) {
			return fmt.Errorf("permission denied deleting %s. Check file permissions", reportsDir)
		}
		return fmt.Errorf("failed to delete reports directory %s: %w", reportsDir, err)
	}

	return nil
}

// deleteLLMLogsDir recursively removes a container's LLM logs directory
func deleteLLMLogsDir(containerName string, cfg *config.Config) error {
	if containerName == "" {
		return nil // No name, nothing to delete
	}

	// Skip if LLM logging is not enabled
	if !cfg.Output.LLMLogEnabled {
		return nil
	}

	sanitized := sanitize.Name(containerName)
	llmLogsDir := filepath.Join(cfg.Output.LLMLogDir, sanitized)

	// Check if directory exists
	if _, err := os.Stat(llmLogsDir); os.IsNotExist(err) {
		return nil // Directory doesn't exist, nothing to delete
	}

	// Recursively delete the directory
	if err := os.RemoveAll(llmLogsDir); err != nil {
		// Check for permission errors
		if os.IsPermission(err) {
			return fmt.Errorf("permission denied deleting %s. Check file permissions", llmLogsDir)
		}
		return fmt.Errorf("failed to delete LLM logs directory %s: %w", llmLogsDir, err)
	}

	return nil
}
