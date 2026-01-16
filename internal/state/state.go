// Package state manages the persistent state of the application.
package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"
)

// State represents the persistent state of log scanning
type State struct {
	Version     string                `json:"version"`
	LastUpdated time.Time             `json:"last_updated"`
	Containers  map[string]*Container `json:"containers"`
	mu          sync.RWMutex          `json:"-"`
	filePath    string                `json:"-"`
	modified    bool                  `json:"-"`
}

// Container represents the state for a single container
type Container struct {
	Name      string    `json:"name"`
	LastScan  time.Time `json:"last_scan"`
	LogCursor string    `json:"log_cursor,omitempty"`
}

// Load loads the state from a JSON file at the specified path.
// Returns a new State instance populated from the file, or an empty state if the file doesn't exist.
// Returns error if the file cannot be read or parsed.
func Load(filePath string) (*State, error) {
	s := &State{
		Version:    "1",
		Containers: make(map[string]*Container),
		filePath:   filePath,
	}

	// If file doesn't exist, return empty state
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return s, nil
	}

	// Read file
	data, err := os.ReadFile(filePath) // #nosec G304 -- filePath is controlled by application, not user input
	if err != nil {
		return nil, fmt.Errorf("failed to read state file from %s: %w", filePath, err)
	}

	// Unmarshal JSON
	if err := json.Unmarshal(data, s); err != nil {
		return nil, fmt.Errorf("failed to parse state file %s: %w", filePath, err)
	}

	s.filePath = filePath
	return s, nil
}

// Save saves the state to the JSON file atomically.
// Uses a temporary file and rename operation to ensure atomic writes.
// Only saves if the state has been modified since the last save.
// Returns error if the file cannot be written or synced.
func (s *State) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.saveUnlocked()
}

// saveUnlocked performs the save operation without acquiring the lock
// Caller must hold the lock
func (s *State) saveUnlocked() error {
	if !s.modified {
		return nil // No changes to save
	}

	s.LastUpdated = time.Now()

	// Marshal to JSON with indentation
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal state for %s: %w", s.filePath, err)
	}

	// Atomic write: write to temp file, then rename
	dir := filepath.Dir(s.filePath)
	tmpFile, err := os.CreateTemp(dir, "state-*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp file in directory %s for state %s: %w", dir, s.filePath, err)
	}
	tmpPath := tmpFile.Name()

	// Write data
	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()    // Best effort cleanup
		_ = os.Remove(tmpPath) // Best effort cleanup
		return fmt.Errorf("failed to write temp file %s for state %s: %w", tmpPath, s.filePath, err)
	}

	// Sync to disk
	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()    // Best effort cleanup
		_ = os.Remove(tmpPath) // Best effort cleanup
		return fmt.Errorf("failed to sync temp file %s for state %s: %w", tmpPath, s.filePath, err)
	}

	_ = tmpFile.Close() // Explicit ignore - we've already synced

	// Atomic rename
	if err := os.Rename(tmpPath, s.filePath); err != nil {
		_ = os.Remove(tmpPath) // Best effort cleanup
		return fmt.Errorf("failed to rename temp file %s to %s: %w", tmpPath, s.filePath, err)
	}

	s.modified = false
	return nil
}

// GetLastScan returns the last scan time for a container.
// Returns the timestamp and true if the container exists in state.
// Returns zero time and false if the container is not found.
func (s *State) GetLastScan(containerID string) (time.Time, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if ctr, exists := s.Containers[containerID]; exists {
		return ctr.LastScan, true
	}
	return time.Time{}, false
}

// UpdateContainer updates the state for a container with new scan information.
// Creates a new container entry if it doesn't exist, or updates the existing one.
// Marks the state as modified requiring a save operation.
func (s *State) UpdateContainer(containerID, name string, lastScan time.Time, cursor string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.Containers[containerID] = &Container{
		Name:      name,
		LastScan:  lastScan,
		LogCursor: cursor,
	}
	s.modified = true
}

// RemoveContainer removes a container from state by its ID.
// Marks the state as modified if the container existed.
// Returns true if the container was found and removed, false otherwise.
func (s *State) RemoveContainer(containerID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.Containers[containerID]; exists {
		delete(s.Containers, containerID)
		s.modified = true
		return true
	}
	return false
}

// ResetAll clears all container state and persists the change to disk.
// Removes all containers from the state and saves immediately.
// Returns error if the save operation fails.
func (s *State) ResetAll() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.Containers = make(map[string]*Container)
	s.modified = true

	return s.saveUnlocked()
}

// ResetFiltered removes containers matching a regular expression pattern.
// The pattern is matched against both container names and IDs.
// Returns the number of containers removed and any error encountered.
// Returns error if the pattern is empty or invalid regex.
func (s *State) ResetFiltered(pattern string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if pattern == "" {
		return 0, fmt.Errorf("pattern cannot be empty for ResetFiltered operation on state %s", s.filePath)
	}

	re, err := regexp.Compile(pattern)
	if err != nil {
		return 0, fmt.Errorf("invalid pattern %q for ResetFiltered operation on state %s: %w", pattern, s.filePath, err)
	}

	count := 0
	for id, ctr := range s.Containers {
		if re.MatchString(ctr.Name) || re.MatchString(id) {
			delete(s.Containers, id)
			count++
		}
	}

	if count > 0 {
		s.modified = true
		if err := s.saveUnlocked(); err != nil {
			return count, err
		}
	}

	return count, nil
}

// GetAllContainers returns a deep copy of all container states.
// Each container is cloned to prevent external modification of internal state.
// Safe for concurrent use with state modifications.
func (s *State) GetAllContainers() map[string]*Container {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]*Container, len(s.Containers))
	for id, ctr := range s.Containers {
		// Deep copy
		result[id] = &Container{
			Name:      ctr.Name,
			LastScan:  ctr.LastScan,
			LogCursor: ctr.LogCursor,
		}
	}
	return result
}

// Count returns the number of containers currently tracked in state.
// Thread-safe for concurrent access.
func (s *State) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.Containers)
}

// Delete removes the state file from disk and clears in-memory state.
// Resets the containers map to empty and marks state as unmodified.
// Returns error if the file cannot be deleted (except if it doesn't exist).
func (s *State) Delete() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.Remove(s.filePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete state file %s: %w", s.filePath, err)
	}

	s.Containers = make(map[string]*Container)
	s.modified = false
	return nil
}
