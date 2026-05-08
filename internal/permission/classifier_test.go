package permission

import (
	"context"
	"testing"
)

type mockClassifier struct {
	result ClassifyResult
	err    error
}

func (m *mockClassifier) Classify(ctx context.Context, req CreatePermissionRequest) (ClassifyResult, error) {
	return m.result, m.err
}

func (m *mockClassifier) Name() string { return "mock" }

func TestClassifyResult_Values(t *testing.T) {
	tests := []struct {
		result   ClassifyResult
		expected string
	}{
		{ClassifyAllow, "allow"},
		{ClassifyDeny, "deny"},
		{ClassifyUnsure, "unsure"},
	}

	for _, tt := range tests {
		if string(tt.result) != tt.expected {
			t.Errorf("ClassifyResult %q != %q", tt.result, tt.expected)
		}
	}
}

func TestSecurityClassifier_NilSafe(t *testing.T) {
	var c SecurityClassifier = &mockClassifier{result: ClassifyAllow}

	result, err := c.Classify(context.Background(), CreatePermissionRequest{
		SessionID: "test-session",
		ToolName:  "bash",
		Action:    "execute",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != ClassifyAllow {
		t.Errorf("expected ClassifyAllow, got %v", result)
	}
	if c.Name() != "mock" {
		t.Errorf("expected name %q, got %q", "mock", c.Name())
	}
}
