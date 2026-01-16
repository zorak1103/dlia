// Package apperrors provides domain-specific error types for the DLIA application.
// These error types include contextual information to aid debugging and error reporting.
package apperrors

import "fmt"

// ConfigurationError represents configuration-related errors.
// It includes the configuration file path and specific key that caused the error.
type ConfigurationError struct {
	ConfigPath string // Path to the configuration file
	Key        string // Configuration key that caused the error
	Err        error  // Underlying error
}

// Error implements the error interface for ConfigurationError.
func (e *ConfigurationError) Error() string {
	if e.Key != "" {
		return fmt.Sprintf("configuration error in %s (key: %s): %v", e.ConfigPath, e.Key, e.Err)
	}
	return fmt.Sprintf("configuration error in %s: %v", e.ConfigPath, e.Err)
}

// Unwrap returns the underlying error for error wrapping chains.
func (e *ConfigurationError) Unwrap() error {
	return e.Err
}

// DockerConnectionError represents Docker connection and operation errors.
// It includes the socket path and the operation that failed.
type DockerConnectionError struct {
	SocketPath string // Docker socket path (e.g., /var/run/docker.sock)
	Operation  string // Operation that failed (e.g., "Ping", "ListContainers")
	Err        error  // Underlying error
}

// Error implements the error interface for DockerConnectionError.
func (e *DockerConnectionError) Error() string {
	if e.SocketPath != "" {
		return fmt.Sprintf("docker %s failed (socket: %s): %v", e.Operation, e.SocketPath, e.Err)
	}
	return fmt.Sprintf("docker %s failed: %v", e.Operation, e.Err)
}

// Unwrap returns the underlying error for error wrapping chains.
func (e *DockerConnectionError) Unwrap() error {
	return e.Err
}

// LLMAPIError represents LLM API request and response errors.
// It includes the API endpoint, HTTP status code, and response details.
type LLMAPIError struct {
	Endpoint     string // API endpoint URL
	StatusCode   int    // HTTP status code (0 if not applicable)
	ResponseBody string // Response body for debugging
	Err          error  // Underlying error
}

// Error implements the error interface for LLMAPIError.
func (e *LLMAPIError) Error() string {
	if e.StatusCode > 0 {
		return fmt.Sprintf("LLM API error at %s (status: %d): %v", e.Endpoint, e.StatusCode, e.Err)
	}
	return fmt.Sprintf("LLM API error at %s: %v", e.Endpoint, e.Err)
}

// Unwrap returns the underlying error for error wrapping chains.
func (e *LLMAPIError) Unwrap() error {
	return e.Err
}
