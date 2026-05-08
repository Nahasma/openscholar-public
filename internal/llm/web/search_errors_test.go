package web

import (
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestSearchError_ShouldRotate(t *testing.T) {
	tests := []struct {
		kind   SearchErrorKind
		rotate bool
	}{
		{SearchErrQuotaExhausted, true},
		{SearchErrRateLimited, true},
		{SearchErrTemporary, true},
		{SearchErrNoResults, true},
		{SearchErrAuth, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.kind), func(t *testing.T) {
			err := &SearchError{Provider: "test", Kind: tt.kind, Cause: fmt.Errorf("test")}
			if got := err.ShouldRotate(); got != tt.rotate {
				t.Errorf("ShouldRotate() = %v, want %v", got, tt.rotate)
			}
		})
	}
}

func TestSearchError_Error(t *testing.T) {
	err := &SearchError{Provider: "brave", Kind: SearchErrAuth, Cause: fmt.Errorf("401")}
	got := err.Error()
	want := "search brave [auth]: 401"
	if got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestSearchError_Unwrap(t *testing.T) {
	cause := fmt.Errorf("original")
	err := &SearchError{Provider: "test", Kind: SearchErrTemporary, Cause: cause}
	if err.Unwrap() != cause {
		t.Error("Unwrap() did not return cause")
	}
}

func TestProviderAttempt(t *testing.T) {
	a := ProviderAttempt{
		Name:     "brave",
		Success:  true,
		Duration: 0.5,
	}
	if a.Name != "brave" || !a.Success {
		t.Error("unexpected ProviderAttempt values")
	}

	a2 := ProviderAttempt{
		Name:     "tavily",
		Success:  false,
		Duration: 1.2,
		Error:    "timeout",
	}
	_ = a2
	_ = time.Second // use time package
}

func TestSearchFailureError_ErrorAndUnwrap(t *testing.T) {
	cause := fmt.Errorf("backend timeout")
	err := &SearchFailureError{
		Message: "all search backends failed",
		Attempts: []ProviderAttempt{
			{Name: "duckduckgo", Success: false, Error: "temporary"},
		},
		Cause: cause,
	}
	if got := err.Error(); got != "all search backends failed: backend timeout" {
		t.Fatalf("Error() = %q", got)
	}
	if !errors.Is(err, cause) {
		t.Fatal("expected errors.Is to match wrapped cause")
	}
}
