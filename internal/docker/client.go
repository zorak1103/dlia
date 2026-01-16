// Package docker provides a client for interacting with the Docker API.
package docker

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

// Common errors
var (
	ErrConnectionFailed = errors.New("docker connection failed")
	ErrNotFound         = errors.New("container not found")
)

// Client defines the interface for Docker client operations.
// Implementations must provide container listing, log reading, and connection management.
// All methods accept context.Context for cancellation and timeout support.
type Client interface {
	// Ping verifies the Docker daemon is accessible. Returns error if connection fails.
	Ping(ctx context.Context) error
	// Close closes the Docker client connection and releases resources.
	Close() error

	// ListContainers lists containers matching the provided filter options.
	//
	// Example usage with filters:
	//   opts := FilterOptions{
	//       IncludeAll:  true,              // Include stopped containers
	//       NamePattern: "^app-.*-prod$",   // Match production app containers
	//   }
	//   containers, err := client.ListContainers(ctx, opts)
	//   if err != nil {
	//       return fmt.Errorf("failed to list containers: %w", err)
	//   }
	//   fmt.Printf("Found %d containers matching filter\n", len(containers))
	//   for _, container := range containers {
	//       fmt.Printf("  %s: %s (%s)\n", container.Name, container.ID[:12], container.Status)
	//   }
	ListContainers(ctx context.Context, opts FilterOptions) ([]Container, error)

	// ReadLogsSince reads container logs from the specified time forward.
	//
	// Example reading logs from last 5 minutes:
	//   since := time.Now().Add(-5 * time.Minute)
	//   logs, err := client.ReadLogsSince(ctx, "container-id-abc123", since)
	//   if err != nil {
	//       return fmt.Errorf("failed to read logs: %w", err)
	//   }
	//   fmt.Printf("Retrieved %d log entries since %s\n", len(logs), since.Format(time.RFC3339))
	//   for _, entry := range logs {
	//       fmt.Printf("[%s] %s: %s\n", entry.Timestamp.Format("15:04:05"), entry.Stream, entry.Message)
	//   }
	ReadLogsSince(ctx context.Context, containerID string, since time.Time) ([]LogEntry, error)
	// ReadLogsLookback reads logs from a container looking back a specific duration.
	ReadLogsLookback(ctx context.Context, containerID string, lookback time.Duration) ([]LogEntry, error)
}

// dockerClientWrapper wraps the Docker client to implement our interface
type dockerClientWrapper struct {
	cli        *client.Client
	socketPath string
}

// Compile-time verification that dockerClientWrapper implements Client
var _ Client = (*dockerClientWrapper)(nil)

// NewClient connects to the Docker daemon at socketPath (or default if empty).
func NewClient(socketPath string) (Client, error) {
	opts := []client.Opt{
		client.WithAPIVersionNegotiation(),
	}

	// Add host option if socket path is specified
	if socketPath != "" {
		opts = append(opts, client.WithHost(socketPath))
	}

	cli, err := client.NewClientWithOpts(opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create Docker client for socket %s: %w", socketPath, err)
	}

	wrapper := &dockerClientWrapper{
		cli:        cli,
		socketPath: socketPath,
	}
	return &dockerClient{cli: wrapper}, nil
}

// NewClientWithInterface is used for testing with mock implementations.
func NewClientWithInterface(dockerCli Client) Client {
	return &dockerClient{cli: dockerCli}
}

func (w *dockerClientWrapper) Ping(ctx context.Context) error {
	_, err := w.cli.Ping(ctx)
	if err != nil {
		return fmt.Errorf("failed to ping Docker daemon at %s: %w", w.socketPath, err)
	}
	return nil
}

func (w *dockerClientWrapper) Close() error {
	return w.cli.Close()
}

func (w *dockerClientWrapper) ListContainers(ctx context.Context, opts FilterOptions) ([]Container, error) {
	listOptions := container.ListOptions{
		All: opts.IncludeAll,
	}

	containers, err := w.cli.ContainerList(ctx, listOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to list containers from socket %s: %w", w.socketPath, err)
	}

	var result []Container
	var nameFilter *regexp.Regexp

	// Compile regex if pattern is provided
	if opts.NamePattern != "" {
		nameFilter, err = regexp.Compile(opts.NamePattern)
		if err != nil {
			return nil, fmt.Errorf("invalid name pattern '%s': %w", opts.NamePattern, err)
		}
	}

	for _, ctr := range containers {
		// Extract container name (remove leading slash)
		name := ""
		if len(ctr.Names) > 0 {
			name = ctr.Names[0]
			if name != "" && name[0] == '/' {
				name = name[1:]
			}
		}

		// Apply name filter if specified
		if nameFilter != nil && !nameFilter.MatchString(name) {
			continue
		}

		result = append(result, Container{
			ID:     ctr.ID,
			Name:   name,
			State:  ctr.State,
			Image:  ctr.Image,
			Labels: ctr.Labels,
		})
	}

	return result, nil
}

func (w *dockerClientWrapper) ReadLogsSince(ctx context.Context, containerID string, since time.Time) ([]LogEntry, error) {
	logOpts := container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Timestamps: true,
		Since:      since.Format(time.RFC3339Nano),
	}

	reader, err := w.cli.ContainerLogs(ctx, containerID, logOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to read logs for container %s: %w", containerID, err)
	}
	// Close reader after parsing; error not actionable in defer context as stream is already consumed
	defer func() { _ = reader.Close() }()

	return parseLogStream(reader)
}

func (w *dockerClientWrapper) ReadLogsLookback(ctx context.Context, containerID string, lookback time.Duration) ([]LogEntry, error) {
	since := time.Now().Add(-lookback)
	return w.ReadLogsSince(ctx, containerID, since)
}

// dockerClient wraps the Docker client with application-specific logic
type dockerClient struct {
	cli Client
}

func (c *dockerClient) Close() error {
	return c.cli.Close()
}

func (c *dockerClient) Ping(ctx context.Context) error {
	return c.cli.Ping(ctx)
}

func (c *dockerClient) ListContainers(ctx context.Context, opts FilterOptions) ([]Container, error) {
	return c.cli.ListContainers(ctx, opts)
}

func (c *dockerClient) ReadLogsSince(ctx context.Context, containerID string, since time.Time) ([]LogEntry, error) {
	return c.cli.ReadLogsSince(ctx, containerID, since)
}

func (c *dockerClient) ReadLogsLookback(ctx context.Context, containerID string, lookback time.Duration) ([]LogEntry, error) {
	return c.cli.ReadLogsLookback(ctx, containerID, lookback)
}
