package llm

import "testing"

func TestAPIError_Error(t *testing.T) {
	withCode := &APIError{Code: "rate_limit_exceeded", Message: "too many requests"}
	if got := withCode.Error(); got != "rate_limit_exceeded: too many requests" {
		t.Errorf("unexpected: %s", got)
	}

	noCode := &APIError{Message: "server error"}
	if got := noCode.Error(); got != "server error" {
		t.Errorf("unexpected: %s", got)
	}
}
