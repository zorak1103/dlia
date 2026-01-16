package docker

// Container represents a Docker container with relevant metadata
type Container struct {
	ID     string
	Name   string
	State  string // running, exited, etc.
	Image  string
	Labels map[string]string
}

// LogEntry represents a single log line from a container
type LogEntry struct {
	Timestamp string
	Stream    string // stdout or stderr
	Message   string
}

// FilterOptions contains options for filtering containers
type FilterOptions struct {
	NamePattern string // Regex pattern for container names
	IncludeAll  bool   // Include stopped containers
}
