package agent

import (
	"errors"
	"fmt"
	"testing"
)

type fakeDisplayErr struct{}

func (fakeDisplayErr) Error() string       { return "boom" }
func (fakeDisplayErr) UserMessage() string { return "short" }
func (fakeDisplayErr) Detail() string      { return "long detail" }

func TestErrorDisplay_UsesStructuredDisplay(t *testing.T) {
	s, d := ErrorDisplay(fakeDisplayErr{}, ReasonStreamError)
	if s != "short" || d != "long detail" {
		t.Fatalf("unexpected summary/detail: %q / %q", s, d)
	}
}

func TestErrorDisplay_UsesWrappedStructuredDisplay(t *testing.T) {
	s, d := ErrorDisplay(fmt.Errorf("failed to process events: %w", fakeDisplayErr{}), ReasonStreamError)
	if s != "short" || d != "long detail" {
		t.Fatalf("unexpected wrapped summary/detail: %q / %q", s, d)
	}
}

func TestErrorDisplay_Fallback(t *testing.T) {
	s, d := ErrorDisplay(errors.New("raw failure"), ReasonStreamError)
	if s == "" || d != "raw failure" {
		t.Fatalf("unexpected fallback summary/detail: %q / %q", s, d)
	}
}
