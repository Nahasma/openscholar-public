package models

import "testing"

func TestResolveModel_Aliases(t *testing.T) {
	tests := []struct {
		input  string
		wantOK bool
	}{
		{"sonnet", true},
		{"opus", true},
		{"haiku", true},
		{"Sonnet", true}, // case-insensitive
		{"OPUS", true},
		{"r1", true},
		{"deepseek", true},
		{"gpt4", true},
		{"nonexistent_alias", false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			_, ok := ResolveModel(tt.input)
			if ok != tt.wantOK {
				t.Errorf("ResolveModel(%q) ok = %v, want %v", tt.input, ok, tt.wantOK)
			}
		})
	}
}

func TestResolveModel_CurrentAliases(t *testing.T) {
	tests := []struct {
		input string
		want  ModelID
	}{
		{"sonnet", Claude46Sonnet},
		{"opus", Claude47Opus},
		{"haiku", Claude45Haiku},
		{"claude", Claude46Sonnet},
		{"gpt5", GPT55},
		{"gpt5mini", GPT54Mini},
		{"deepseek", DeepSeekChat},
		{"r1", DeepSeekR1},
		{"deepseek-v4", DeepSeekV4Flash},
		{"v4flash", DeepSeekV4Flash},
		{"v4pro", DeepSeekV4Pro},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, ok := ResolveModel(tt.input)
			if !ok {
				t.Fatalf("ResolveModel(%q) not found", tt.input)
			}
			if got.ID != tt.want {
				t.Fatalf("ResolveModel(%q) = %s, want %s", tt.input, got.ID, tt.want)
			}
		})
	}
}

func TestModelAliases_AllPointToValidModels(t *testing.T) {
	for alias, id := range ModelAliases {
		if _, ok := SupportedModels[id]; !ok {
			t.Errorf("alias %q points to unknown ModelID %q", alias, id)
		}
	}
}
