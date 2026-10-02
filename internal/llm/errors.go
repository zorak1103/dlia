package llm

import (
	"errors"
	"fmt"
)

// ErrIncompleteAnswer is matched (via errors.Is) by every *IncompleteAnswerError.
var ErrIncompleteAnswer = errors.New("incomplete LLM answer")

// IncompleteAnswerError reports an LLM answer that was cut off or empty.
type IncompleteAnswerError struct {
	Container string
	Reason    string
	Limit     int
	LimitKey  string
}

// Error implements the error interface.
func (e *IncompleteAnswerError) Error() string {
	subject := ""
	if e.Container != "" {
		subject = " for container " + e.Container
	}
	return fmt.Sprintf(
		"LLM answer incomplete (%s)%s: raise %s (currently %d) or limit reasoning via llm.extra_body",
		e.Reason, subject, e.LimitKey, e.Limit,
	)
}

// Is reports whether target is ErrIncompleteAnswer.
func (e *IncompleteAnswerError) Is(target error) bool {
	return target == ErrIncompleteAnswer
}
