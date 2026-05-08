package permission

import (
	"testing"
)

func TestDefaultAskRules(t *testing.T) {
	rules := DefaultAskRules()
	if len(rules) == 0 {
		t.Fatal("DefaultAskRules returned empty slice")
	}

	// All rules should be Ask decisions for Bash tool
	for _, r := range rules {
		if r.Tool != "Bash" {
			t.Errorf("expected tool Bash, got %s", r.Tool)
		}
		if r.Decision != Ask {
			t.Errorf("expected decision Ask, got %s for pattern %s", r.Decision, r.Pattern)
		}
	}

	// Should have rules for all 4 categories
	expectedCount := len(DangerousInterpreters) + len(DangerousNetworkCommands) +
		len(DangerousCloudCommands) + len(DangerousPrivilegedCommands)
	if len(rules) != expectedCount {
		t.Errorf("expected %d rules, got %d", expectedCount, len(rules))
	}
}

func TestDangerousPatternMatching(t *testing.T) {
	rules := DefaultAskRules()

	tests := []struct {
		cmd      string
		expected bool // true = should match an ask rule
	}{
		{"python -c 'import os'", true},
		{"python3 script.py", true},
		{"curl https://example.com", true},
		{"wget http://example.com", true},
		{"sudo rm -rf /tmp", true},
		{"kubectl get pods", true},
		{"aws s3 ls", true},
		{"ssh user@host", true},
	}

	for _, tt := range tests {
		matched := false
		for _, rule := range rules {
			if MatchBashRule(rule, tt.cmd) {
				matched = true
				break
			}
		}
		if matched != tt.expected {
			t.Errorf("command %q: matched=%v, expected=%v", tt.cmd, matched, tt.expected)
		}
	}
}
