package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	tests := []struct {
		name        string
		setupFile   func(string) error
		wantErr     bool
		wantVersion string
		wantCount   int
	}{
		{
			name: "non-existent file returns empty state",
			setupFile: func(_ string) error {
				return nil // Don't create file
			},
			wantErr:     false,
			wantVersion: "1",
			wantCount:   0,
		},
		{
			name: "valid file loads correctly",
			setupFile: func(path string) error {
				s := &State{
					Version:     "1",
					LastUpdated: time.Now(),
					Containers: map[string]*Container{
						"abc123": {
							Name:      "test-container",
							LastScan:  time.Now(),
							LogCursor: "cursor1",
						},
					},
				}
				data, _ := json.MarshalIndent(s, "", "  ")
				return os.WriteFile(path, data, 0600)
			},
			wantErr:     false,
			wantVersion: "1",
			wantCount:   1,
		},
		{
			name: "invalid JSON returns error",
			setupFile: func(path string) error {
				return os.WriteFile(path, []byte("invalid json"), 0600)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			filePath := filepath.Join(tmpDir, "state.json")

			if err := tt.setupFile(filePath); err != nil {
				t.Fatalf("setup failed: %v", err)
			}

			s, err := Load(filePath)
			if (err != nil) != tt.wantErr {
				t.Errorf("Load() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if s.Version != tt.wantVersion {
					t.Errorf("Version = %v, want %v", s.Version, tt.wantVersion)
				}
				if len(s.Containers) != tt.wantCount {
					t.Errorf("Container count = %v, want %v", len(s.Containers), tt.wantCount)
				}
				if s.filePath != filePath {
					t.Errorf("filePath = %v, want %v", s.filePath, filePath)
				}
			}
		})
	}
}

func TestState_Save(t *testing.T) {
	tests := []struct {
		name     string
		modified bool
		wantSave bool
	}{
		{
			name:     "modified state saves",
			modified: true,
			wantSave: true,
		},
		{
			name:     "unmodified state does not save",
			modified: false,
			wantSave: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			filePath := filepath.Join(tmpDir, "state.json")

			s := &State{
				Version:    "1",
				Containers: make(map[string]*Container),
				filePath:   filePath,
				modified:   tt.modified,
			}

			oldTime := time.Now().Add(-1 * time.Hour)
			s.LastUpdated = oldTime

			err := s.Save()
			if err != nil {
				t.Fatalf("Save() error = %v", err)
			}

			if tt.wantSave {
				// Check file was created
				if _, err := os.Stat(filePath); os.IsNotExist(err) {
					t.Error("Expected file to be created but it doesn't exist")
				}

				// Check LastUpdated was updated
				if !s.LastUpdated.After(oldTime) {
					t.Error("LastUpdated should be updated after save")
				}

				// Verify file content
				// #nosec G304 - reading from controlled test temp directory
				data, err := os.ReadFile(filePath)
				if err != nil {
					t.Fatalf("Failed to read saved file: %v", err)
				}

				var loaded State
				if err := json.Unmarshal(data, &loaded); err != nil {
					t.Fatalf("Failed to unmarshal saved file: %v", err)
				}

				if loaded.Version != "1" {
					t.Errorf("Saved version = %v, want 1", loaded.Version)
				}
			} else {
				// Check file was not created
				if _, err := os.Stat(filePath); err == nil {
					t.Error("File should not be created for unmodified state")
				}
			}
		})
	}
}

func TestState_UpdateContainer(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "state.json")

	s := &State{
		Version:    "1",
		Containers: make(map[string]*Container),
		filePath:   filePath,
		modified:   false,
	}

	now := time.Now()
	s.UpdateContainer("abc123", "test-container", now, "cursor1")

	if !s.modified {
		t.Error("State should be marked as modified")
	}

	if len(s.Containers) != 1 {
		t.Errorf("Container count = %v, want 1", len(s.Containers))
	}

	ctr, exists := s.Containers["abc123"]
	if !exists {
		t.Fatal("Container should exist")
	}

	if ctr.Name != "test-container" {
		t.Errorf("Name = %v, want test-container", ctr.Name)
	}
	if !ctr.LastScan.Equal(now) {
		t.Errorf("LastScan = %v, want %v", ctr.LastScan, now)
	}
	if ctr.LogCursor != "cursor1" {
		t.Errorf("LogCursor = %v, want cursor1", ctr.LogCursor)
	}
}

func TestState_GetLastScan(t *testing.T) {
	s := &State{
		Version:    "1",
		Containers: make(map[string]*Container),
		filePath:   "/tmp/test.json",
	}

	now := time.Now()
	s.Containers["abc123"] = &Container{
		Name:     "test-container",
		LastScan: now,
	}

	t.Run("existing container", func(t *testing.T) {
		lastScan, exists := s.GetLastScan("abc123")
		if !exists {
			t.Error("Container should exist")
		}
		if !lastScan.Equal(now) {
			t.Errorf("LastScan = %v, want %v", lastScan, now)
		}
	})

	t.Run("non-existent container", func(t *testing.T) {
		_, exists := s.GetLastScan("nonexistent")
		if exists {
			t.Error("Container should not exist")
		}
	})
}

func TestState_RemoveContainer(t *testing.T) {
	s := &State{
		Version:    "1",
		Containers: make(map[string]*Container),
		filePath:   "/tmp/test.json",
		modified:   false,
	}

	s.Containers["abc123"] = &Container{Name: "test-container"}

	t.Run("remove existing container", func(t *testing.T) {
		s.RemoveContainer("abc123")
		if !s.modified {
			t.Error("State should be marked as modified")
		}
		if len(s.Containers) != 0 {
			t.Error("Container should be removed")
		}
	})

	t.Run("remove non-existent container", func(t *testing.T) {
		s.modified = false
		s.RemoveContainer("nonexistent")
		if s.modified {
			t.Error("State should not be marked as modified")
		}
	})
}

func TestState_ResetAll(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "state.json")

	s := &State{
		Version: "1",
		Containers: map[string]*Container{
			"abc123": {Name: "test1"},
			"def456": {Name: "test2"},
		},
		filePath: filePath,
		modified: true,
	}

	err := s.ResetAll()
	if err != nil {
		t.Fatalf("ResetAll() error = %v", err)
	}

	if len(s.Containers) != 0 {
		t.Errorf("Containers should be empty, got %v", len(s.Containers))
	}

	if s.modified {
		t.Error("State should not be modified after save")
	}
}

func TestState_ResetFiltered(t *testing.T) {
	tests := []struct {
		name       string
		pattern    string
		containers map[string]*Container
		wantCount  int
		wantRemain int
		wantErr    bool
	}{
		{
			name:    "empty pattern returns error",
			pattern: "",
			wantErr: true,
		},
		{
			name:    "invalid regex returns error",
			pattern: "[invalid",
			wantErr: true,
		},
		{
			name:    "match by name",
			pattern: "test-.*",
			containers: map[string]*Container{
				"abc123": {Name: "test-container"},
				"def456": {Name: "prod-container"},
			},
			wantCount:  1,
			wantRemain: 1,
			wantErr:    false,
		},
		{
			name:    "match by ID",
			pattern: "abc.*",
			containers: map[string]*Container{
				"abc123": {Name: "container1"},
				"def456": {Name: "container2"},
			},
			wantCount:  1,
			wantRemain: 1,
			wantErr:    false,
		},
		{
			name:    "no matches",
			pattern: "nonexistent",
			containers: map[string]*Container{
				"abc123": {Name: "container1"},
			},
			wantCount:  0,
			wantRemain: 1,
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			filePath := filepath.Join(tmpDir, "state.json")

			s := &State{
				Version:    "1",
				Containers: tt.containers,
				filePath:   filePath,
			}

			count, err := s.ResetFiltered(tt.pattern)
			if (err != nil) != tt.wantErr {
				t.Errorf("ResetFiltered() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if count != tt.wantCount {
					t.Errorf("ResetFiltered() count = %v, want %v", count, tt.wantCount)
				}
				if len(s.Containers) != tt.wantRemain {
					t.Errorf("Remaining containers = %v, want %v", len(s.Containers), tt.wantRemain)
				}
			}
		})
	}
}

func TestState_GetAllContainers(t *testing.T) {
	s := &State{
		Version: "1",
		Containers: map[string]*Container{
			"abc123": {
				Name:      "test-container",
				LastScan:  time.Now(),
				LogCursor: "cursor1",
			},
		},
		filePath: "/tmp/test.json",
	}

	containers := s.GetAllContainers()

	if len(containers) != 1 {
		t.Errorf("Container count = %v, want 1", len(containers))
	}

	// Verify it's a deep copy
	containers["abc123"].Name = "modified"
	if s.Containers["abc123"].Name == "modified" {
		t.Error("GetAllContainers should return a deep copy")
	}
}

func TestState_Count(t *testing.T) {
	s := &State{
		Version: "1",
		Containers: map[string]*Container{
			"abc123": {Name: "test1"},
			"def456": {Name: "test2"},
		},
		filePath: "/tmp/test.json",
	}

	count := s.Count()
	if count != 2 {
		t.Errorf("Count() = %v, want 2", count)
	}
}

func TestState_Delete(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "state.json")

	// Create a file first
	s := &State{
		Version:    "1",
		Containers: map[string]*Container{"abc123": {Name: "test"}},
		filePath:   filePath,
		modified:   true,
	}
	if err := s.Save(); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Delete it
	err := s.Delete()
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	// Check file is gone
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Error("File should be deleted")
	}

	// Check containers cleared
	if len(s.Containers) != 0 {
		t.Error("Containers should be cleared")
	}

	if s.modified {
		t.Error("modified should be false")
	}
}

func TestState_Delete_NonExistentFile(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "nonexistent.json")

	s := &State{
		Version:    "1",
		Containers: make(map[string]*Container),
		filePath:   filePath,
	}

	err := s.Delete()
	if err != nil {
		t.Errorf("Delete() should not error on non-existent file, got: %v", err)
	}
}

func TestState_Concurrency(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "state.json")

	s := &State{
		Version:    "1",
		Containers: make(map[string]*Container),
		filePath:   filePath,
	}

	var wg sync.WaitGroup
	numGoroutines := 10
	numOps := 100

	// Concurrent updates
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOps; j++ {
				containerID := string(rune('a' + id))
				s.UpdateContainer(containerID, "test", time.Now(), "cursor")
			}
		}(i)
	}

	// Concurrent reads
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < numOps; j++ {
				_ = s.GetAllContainers()
				_ = s.Count()
			}
		}()
	}

	wg.Wait()

	// Should complete without panics or races
	if s.Count() != numGoroutines {
		t.Errorf("Expected %d containers, got %d", numGoroutines, s.Count())
	}
}

func TestState_Save_AtomicWrite(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "state.json")

	s := &State{
		Version:    "1",
		Containers: map[string]*Container{"abc123": {Name: "test"}},
		filePath:   filePath,
		modified:   true,
	}

	err := s.Save()
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Check no temp files left behind
	files, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("Failed to read dir: %v", err)
	}

	for _, file := range files {
		if filepath.Ext(file.Name()) == ".tmp" {
			t.Errorf("Temp file left behind: %s", file.Name())
		}
	}
}

func TestState_ResetFiltered_SaveFailure(t *testing.T) {
	// Use an invalid path to force save failure
	s := &State{
		Version: "1",
		Containers: map[string]*Container{
			"abc123": {Name: "test-container"},
		},
		filePath: "/invalid/path/state.json",
	}

	count, err := s.ResetFiltered("test-.*")
	if err == nil {
		t.Error("Expected error when save fails")
	}
	if count != 1 {
		t.Errorf("Expected count = 1, got %d", count)
	}
}

func TestLoad_ReadError(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "state.json")

	// Create a directory instead of a file to cause read error
	// #nosec G301 - test code, intentionally creating directory with appropriate permissions
	if err := os.Mkdir(filePath, 0755); err != nil {
		t.Fatalf("Failed to create dir: %v", err)
	}

	_, err := Load(filePath)
	if err == nil {
		t.Error("Expected error when reading directory as file")
	}
}

func BenchmarkState_UpdateContainer(b *testing.B) {
	s := &State{
		Version:    "1",
		Containers: make(map[string]*Container),
		filePath:   "/tmp/benchmark.json",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.UpdateContainer("abc123", "test", time.Now(), "cursor")
	}
}

func BenchmarkState_GetAllContainers(b *testing.B) {
	s := &State{
		Version:    "1",
		Containers: make(map[string]*Container),
		filePath:   "/tmp/benchmark.json",
	}

	// Add some containers
	for i := 0; i < 100; i++ {
		s.Containers[string(rune(i))] = &Container{
			Name:     "test",
			LastScan: time.Now(),
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = s.GetAllContainers()
	}
}

func TestState_ResetFiltered_Patterns(t *testing.T) {
	tests := []struct {
		name       string
		pattern    string
		containers map[string]*Container
		wantCount  int
	}{
		{
			name:    "match all",
			pattern: ".*",
			containers: map[string]*Container{
				"abc123": {Name: "container1"},
				"def456": {Name: "container2"},
			},
			wantCount: 2,
		},
		{
			name:    "case sensitive",
			pattern: "Test",
			containers: map[string]*Container{
				"abc123": {Name: "test-container"},
				"def456": {Name: "Test-container"},
			},
			wantCount: 1,
		},
		{
			name:    "special regex characters",
			pattern: regexp.QuoteMeta("test.container"),
			containers: map[string]*Container{
				"abc123": {Name: "test.container"},
				"def456": {Name: "testXcontainer"},
			},
			wantCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			filePath := filepath.Join(tmpDir, "state.json")

			s := &State{
				Version:    "1",
				Containers: tt.containers,
				filePath:   filePath,
			}

			count, err := s.ResetFiltered(tt.pattern)
			if err != nil {
				t.Fatalf("ResetFiltered() error = %v", err)
			}

			if count != tt.wantCount {
				t.Errorf("ResetFiltered() count = %v, want %v", count, tt.wantCount)
			}
		})
	}
}
