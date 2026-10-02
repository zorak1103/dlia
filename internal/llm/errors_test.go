package llm

import (
	"errors"
	"strings"
	"testing"
)

func TestIncompleteAnswerError_NoContainer(t *testing.T) {
	err := &IncompleteAnswerError{Reason: "empty answer", Limit: 4000, LimitKey: "llm.max_answer_tokens"}

	if !strings.HasPrefix(err.Error(), "LLM answer incomplete (") {
		t.Errorf("message = %q", err.Error())
	}
}

func TestIncompleteAnswerError_WithContainer(t *testing.T) {
	err := &IncompleteAnswerError{Container: "nginx", Reason: "finish_reason=length", Limit: 4000, LimitKey: "llm.max_answer_tokens"}

	if !strings.Contains(err.Error(), "nginx") {
		t.Errorf("message lacks container: %q", err.Error())
	}
	if errors.Is(errors.New("other"), err) {
		t.Error("unrelated error must not match")
	}
}
