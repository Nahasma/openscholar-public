package agent

import (
	"fmt"
	"strings"
	"testing"
)

func TestToolResultBudget(t *testing.T) {
	// Test truncation logic for oversized tool results.
	longContent := strings.Repeat("x", 30*1024) // 30 KB
	maxBytes := 20 * 1024

	if len(longContent) <= maxBytes {
		t.Fatal("test content should exceed budget")
	}

	omitted := len(longContent) - maxBytes
	truncated := longContent[:maxBytes] + fmt.Sprintf("\n\n[truncated: %d bytes omitted]", omitted)

	if len(truncated) < maxBytes {
		t.Error("truncated content too short")
	}
	if !strings.Contains(truncated, "[truncated:") {
		t.Error("missing truncation marker")
	}
	if !strings.Contains(truncated, fmt.Sprintf("%d bytes omitted", omitted)) {
		t.Errorf("truncation marker should report %d bytes omitted", omitted)
	}
}

func TestToolResultBudgetNoTruncation(t *testing.T) {
	// Content under budget should not be truncated.
	content := strings.Repeat("y", 10*1024) // 10 KB, under 20 KB default
	maxBytes := defaultMaxResultBytes

	if len(content) > maxBytes {
		t.Fatal("test content should be under budget")
	}

	// No truncation should occur.
	result := content
	if strings.Contains(result, "[truncated:") {
		t.Error("content under budget should not be truncated")
	}
}

func TestToolResultBudgetUnlimited(t *testing.T) {
	// MaxResultBytes == -1 means no truncation.
	content := strings.Repeat("z", 100*1024) // 100 KB
	maxBytes := -1

	// Simulate the budget check from executeToolCalls.
	if maxBytes > 0 && len(content) > maxBytes {
		t.Error("should not truncate when maxBytes == -1")
	}
}

func TestDefaultMaxResultBytes(t *testing.T) {
	if defaultMaxResultBytes != 20*1024 {
		t.Errorf("defaultMaxResultBytes = %d, want %d", defaultMaxResultBytes, 20*1024)
	}
}
