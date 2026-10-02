package llm

import (
	"errors"
	"testing"
)

func TestIncompleteAnswerError_NoContainer(t *testing.T) {
	err := &IncompleteAnswerError{Reason: "empty answer", Limit: 4000, LimitKey: "llm.max_answer_tokens"}

	want := "LLM answer incomplete (empty answer, limit 4000 tokens): " +
		"raise llm.max_answer_tokens or lower the reasoning effort via llm.extra_body"
	if err.Error() != want {
		t.Errorf("message = %q, want %q", err.Error(), want)
	}
}

func TestIncompleteAnswerError_WithContainer(t *testing.T) {
	err := &IncompleteAnswerError{Container: "nginx", Reason: "finish_reason=length", Limit: 4000, LimitKey: "llm.max_answer_tokens"}

	want := "LLM answer for container nginx incomplete (finish_reason=length, limit 4000 tokens): " +
		"raise llm.max_answer_tokens or lower the reasoning effort via llm.extra_body"
	if err.Error() != want {
		t.Errorf("message = %q, want %q", err.Error(), want)
	}
}

func TestIncompleteAnswerError_Is(t *testing.T) {
	err := &IncompleteAnswerError{}

	if !err.Is(ErrIncompleteAnswer) {
		t.Error("must match ErrIncompleteAnswer")
	}
	if err.Is(errors.New("other")) {
		t.Error("must not match unrelated errors")
	}
}
