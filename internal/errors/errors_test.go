package apperrors

import (
	"errors"
	"testing"
)

func TestConfigurationError(t *testing.T) {
	base := errors.New("bad value")

	withKey := &ConfigurationError{ConfigPath: "config.yaml", Key: "llm.api_key", Err: base}
	if got := withKey.Error(); got != "configuration error in config.yaml (key: llm.api_key): bad value" {
		t.Errorf("unexpected: %s", got)
	}
	if !errors.Is(withKey, base) {
		t.Error("Unwrap should return the underlying error")
	}

	noKey := &ConfigurationError{ConfigPath: "config.yaml", Err: base}
	if got := noKey.Error(); got != "configuration error in config.yaml: bad value" {
		t.Errorf("unexpected: %s", got)
	}
}

func TestDockerConnectionError(t *testing.T) {
	base := errors.New("refused")

	withSocket := &DockerConnectionError{SocketPath: "/var/run/docker.sock", Operation: "Ping", Err: base}
	if got := withSocket.Error(); got != "docker Ping failed (socket: /var/run/docker.sock): refused" {
		t.Errorf("unexpected: %s", got)
	}
	if !errors.Is(withSocket, base) {
		t.Error("Unwrap should return the underlying error")
	}

	noSocket := &DockerConnectionError{Operation: "Ping", Err: base}
	if got := noSocket.Error(); got != "docker Ping failed: refused" {
		t.Errorf("unexpected: %s", got)
	}
}

func TestLLMAPIError(t *testing.T) {
	base := errors.New("timeout")

	withStatus := &LLMAPIError{Endpoint: "https://api.openai.com/v1", StatusCode: 429, Err: base}
	if got := withStatus.Error(); got != "LLM API error at https://api.openai.com/v1 (status: 429): timeout" {
		t.Errorf("unexpected: %s", got)
	}
	if !errors.Is(withStatus, base) {
		t.Error("Unwrap should return the underlying error")
	}

	noStatus := &LLMAPIError{Endpoint: "https://api.openai.com/v1", Err: base}
	if got := noStatus.Error(); got != "LLM API error at https://api.openai.com/v1: timeout" {
		t.Errorf("unexpected: %s", got)
	}
}
