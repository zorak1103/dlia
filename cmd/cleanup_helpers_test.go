package cmd

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zorak1103/dlia/internal/config"
	"github.com/zorak1103/dlia/internal/docker"
	"github.com/zorak1103/dlia/internal/sanitize"
)

func TestSanitizeName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple name without slash",
			input:    "nginx",
			expected: "nginx",
		},
		{
			name:     "name with single slash",
			input:    "project/nginx",
			expected: "project_nginx",
		},
		{
			name:     "name with multiple slashes",
			input:    "company/project/nginx",
			expected: "company_project_nginx",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitize.Name(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestScanStateFile(t *testing.T) {
	t.Run("empty state file", func(t *testing.T) {
		tempDir := t.TempDir()
		stateFile := filepath.Join(tempDir, "state.json")

		// Create empty state file
		err := os.WriteFile(stateFile, []byte(`{"containers":{},"last_updated":"2024-01-01T00:00:00Z"}`), 0600)
		require.NoError(t, err)

		cfg := &config.Config{
			Output: config.OutputConfig{
				StateFile: stateFile,
			},
		}

		ids, err := scanStateFile(cfg)
		require.NoError(t, err)
		assert.Empty(t, ids)
	})

	t.Run("state file with containers", func(t *testing.T) {
		tempDir := t.TempDir()
		stateFile := filepath.Join(tempDir, "state.json")

		// Create state file with two containers
		stateJSON := `{
			"containers": {
				"abc123": {
					"name": "nginx",
					"last_scan": "2024-01-01T00:00:00Z",
					"log_cursor": "2024-01-01T00:00:00Z"
				},
				"def456": {
					"name": "postgres",
					"last_scan": "2024-01-01T00:00:00Z",
					"log_cursor": "2024-01-01T00:00:00Z"
				}
			},
			"last_updated": "2024-01-01T00:00:00Z"
		}`
		err := os.WriteFile(stateFile, []byte(stateJSON), 0600)
		require.NoError(t, err)

		cfg := &config.Config{
			Output: config.OutputConfig{
				StateFile: stateFile,
			},
		}

		ids, err := scanStateFile(cfg)
		require.NoError(t, err)
		assert.Len(t, ids, 2)
		assert.Contains(t, ids, "abc123")
		assert.Contains(t, ids, "def456")
	})

	t.Run("state file does not exist", func(t *testing.T) {
		tempDir := t.TempDir()
		stateFile := filepath.Join(tempDir, "nonexistent.json")

		cfg := &config.Config{
			Output: config.OutputConfig{
				StateFile: stateFile,
			},
		}

		ids, err := scanStateFile(cfg)
		require.NoError(t, err)
		assert.Empty(t, ids)
	})
}

func TestScanKnowledgeBase(t *testing.T) {
	t.Run("empty knowledge base", func(t *testing.T) {
		tempDir := t.TempDir()
		kbDir := filepath.Join(tempDir, "knowledge_base")
		servicesDir := filepath.Join(kbDir, "services")
		err := os.MkdirAll(servicesDir, 0750)
		require.NoError(t, err)

		cfg := &config.Config{
			Output: config.OutputConfig{
				KnowledgeBaseDir: kbDir,
			},
		}

		names, err := scanKnowledgeBase(cfg)
		require.NoError(t, err)
		assert.Empty(t, names)
	})

	t.Run("knowledge base with containers", func(t *testing.T) {
		tempDir := t.TempDir()
		kbDir := filepath.Join(tempDir, "knowledge_base")
		servicesDir := filepath.Join(kbDir, "services")
		err := os.MkdirAll(servicesDir, 0750)
		require.NoError(t, err)

		// Create .md files
		err = os.WriteFile(filepath.Join(servicesDir, "nginx.md"), []byte("# nginx"), 0600)
		require.NoError(t, err)
		err = os.WriteFile(filepath.Join(servicesDir, "project_postgres.md"), []byte("# postgres"), 0600)
		require.NoError(t, err)

		cfg := &config.Config{
			Output: config.OutputConfig{
				KnowledgeBaseDir: kbDir,
			},
		}

		names, err := scanKnowledgeBase(cfg)
		require.NoError(t, err)
		assert.Len(t, names, 2)
		assert.Contains(t, names, "nginx")
		assert.Contains(t, names, "project_postgres")
	})

	t.Run("knowledge base directory does not exist", func(t *testing.T) {
		tempDir := t.TempDir()
		kbDir := filepath.Join(tempDir, "nonexistent")

		cfg := &config.Config{
			Output: config.OutputConfig{
				KnowledgeBaseDir: kbDir,
			},
		}

		names, err := scanKnowledgeBase(cfg)
		require.NoError(t, err)
		assert.Empty(t, names)
	})
}

func TestScanReports(t *testing.T) {
	t.Run("empty reports directory", func(t *testing.T) {
		tempDir := t.TempDir()
		reportsDir := filepath.Join(tempDir, "reports")
		err := os.MkdirAll(reportsDir, 0750)
		require.NoError(t, err)

		cfg := &config.Config{
			Output: config.OutputConfig{
				ReportsDir: reportsDir,
			},
		}

		names, err := scanReports(cfg)
		require.NoError(t, err)
		assert.Empty(t, names)
	})

	t.Run("reports with container directories", func(t *testing.T) {
		tempDir := t.TempDir()
		reportsDir := filepath.Join(tempDir, "reports")

		// Create container directories
		err := os.MkdirAll(filepath.Join(reportsDir, "nginx"), 0750)
		require.NoError(t, err)
		err = os.MkdirAll(filepath.Join(reportsDir, "project_postgres"), 0750)
		require.NoError(t, err)

		cfg := &config.Config{
			Output: config.OutputConfig{
				ReportsDir: reportsDir,
			},
		}

		names, err := scanReports(cfg)
		require.NoError(t, err)
		assert.Len(t, names, 2)
		assert.Contains(t, names, "nginx")
		assert.Contains(t, names, "project_postgres")
	})

	t.Run("reports directory does not exist", func(t *testing.T) {
		tempDir := t.TempDir()
		reportsDir := filepath.Join(tempDir, "nonexistent")

		cfg := &config.Config{
			Output: config.OutputConfig{
				ReportsDir: reportsDir,
			},
		}

		names, err := scanReports(cfg)
		require.NoError(t, err)
		assert.Empty(t, names)
	})
}

// testMockDockerClient is a simple mock for cleanup tests
type testMockDockerClient struct {
	containers []docker.Container
}

func (m *testMockDockerClient) Ping(_ context.Context) error {
	return nil
}

func (m *testMockDockerClient) ListContainers(_ context.Context, _ docker.FilterOptions) ([]docker.Container, error) {
	return m.containers, nil
}

func (m *testMockDockerClient) GetContainer(_ context.Context, containerID string) (*docker.Container, error) {
	for _, c := range m.containers {
		if c.ID == containerID {
			return &c, nil
		}
	}
	// Return a proper error instead of nil
	return nil, os.ErrNotExist
}

func (m *testMockDockerClient) ContainerExists(_ context.Context, containerID string) bool {
	for _, c := range m.containers {
		if c.ID == containerID {
			return true
		}
	}
	return false
}

func (m *testMockDockerClient) Close() error {
	return nil
}

func (m *testMockDockerClient) ReadLogsSince(_ context.Context, _ string, _ time.Time) ([]docker.LogEntry, error) {
	return []docker.LogEntry{}, nil
}

func (m *testMockDockerClient) ReadLogs(_ context.Context, _ string, _ docker.LogsOptions) ([]docker.LogEntry, error) {
	return []docker.LogEntry{}, nil
}

func (m *testMockDockerClient) ReadLogsLookback(_ context.Context, _ string, _ time.Duration) ([]docker.LogEntry, error) {
	return []docker.LogEntry{}, nil
}

func TestFindObsoleteContainers(t *testing.T) {
	t.Run("no obsolete containers", func(t *testing.T) {
		tempDir := t.TempDir()
		stateFile := filepath.Join(tempDir, "state.json")

		// Create state file with container that exists in Docker
		stateJSON := `{
			"containers": {
				"running123": {
					"name": "nginx",
					"last_scan": "2024-01-01T00:00:00Z",
					"log_cursor": "2024-01-01T00:00:00Z"
				}
			},
			"last_updated": "2024-01-01T00:00:00Z"
		}`
		err := os.WriteFile(stateFile, []byte(stateJSON), 0600)
		require.NoError(t, err)

		cfg := &config.Config{
			Output: config.OutputConfig{
				StateFile:        stateFile,
				KnowledgeBaseDir: filepath.Join(tempDir, "kb"),
				ReportsDir:       filepath.Join(tempDir, "reports"),
				LLMLogDir:        filepath.Join(tempDir, "llm"),
				LLMLogEnabled:    false,
			},
		}

		// Mock Docker client that returns one running container
		mockClient := &testMockDockerClient{
			containers: []docker.Container{
				{ID: "running123"},
			},
		}

		ctx := context.Background()
		obsolete, err := findObsoleteContainers(ctx, mockClient, cfg)
		require.NoError(t, err)
		assert.Empty(t, obsolete, "Should find no obsolete containers when all containers in state exist in Docker")
	})

	t.Run("finds obsolete container in state", func(t *testing.T) {
		tempDir := t.TempDir()
		stateFile := filepath.Join(tempDir, "state.json")

		// Create state file with container that doesn't exist in Docker
		stateJSON := `{
			"containers": {
				"removed123": {
					"name": "old-nginx",
					"last_scan": "2024-01-01T00:00:00Z",
					"log_cursor": "2024-01-01T00:00:00Z"
				}
			},
			"last_updated": "2024-01-01T00:00:00Z"
		}`
		err := os.WriteFile(stateFile, []byte(stateJSON), 0600)
		require.NoError(t, err)

		cfg := &config.Config{
			Output: config.OutputConfig{
				StateFile:        stateFile,
				KnowledgeBaseDir: filepath.Join(tempDir, "kb"),
				ReportsDir:       filepath.Join(tempDir, "reports"),
				LLMLogDir:        filepath.Join(tempDir, "llm"),
				LLMLogEnabled:    false,
			},
		}

		// Mock Docker client that returns no containers
		mockClient := &testMockDockerClient{
			containers: []docker.Container{},
		}

		ctx := context.Background()
		obsolete, err := findObsoleteContainers(ctx, mockClient, cfg)
		require.NoError(t, err)
		require.Len(t, obsolete, 1)
		assert.Equal(t, "removed123", obsolete[0].ID)
		assert.Equal(t, "old-nginx", obsolete[0].Name)
		assert.True(t, obsolete[0].InState)
	})
}
