package custom

import (
	"strings"
	"testing"
)

func TestParseFrontmatterExtended(t *testing.T) {
	input := `---
name: test
description: Test command
allowed_tools: [Read, Bash]
model: haiku
when_to_use: "Use when testing"
argument_hint: "[paper-id]"
arguments: [paper_id, query]
user_invocable: true
disable-model-invocation: true
---
Search for $paper_id with query: $query
Full args: $ARGUMENTS`

	fm, body, err := ParseFrontmatter(input)
	if err != nil {
		t.Fatal(err)
	}
	if fm.Name != "test" {
		t.Errorf("name = %q, want %q", fm.Name, "test")
	}
	if len(fm.AllowedTools) != 2 {
		t.Errorf("allowed_tools len = %d, want 2", len(fm.AllowedTools))
	}
	if fm.Model != "haiku" {
		t.Errorf("model = %q, want %q", fm.Model, "haiku")
	}
	if fm.WhenToUse != "Use when testing" {
		t.Errorf("when_to_use = %q", fm.WhenToUse)
	}
	if fm.ArgumentHint != "[paper-id]" {
		t.Errorf("argument_hint = %q", fm.ArgumentHint)
	}
	if len(fm.Arguments) != 2 {
		t.Errorf("arguments len = %d, want 2", len(fm.Arguments))
	}
	if fm.UserInvocable == nil || !*fm.UserInvocable {
		t.Error("user_invocable should be true")
	}
	if !fm.DisableModelInvocationAlt {
		t.Error("disable-model-invocation should be true")
	}
	_ = body
}

func TestExpandBodyFM_NamedArgs(t *testing.T) {
	fm := Frontmatter{
		Arguments: []string{"paper_id", "query"},
	}
	body := "Search for $paper_id with: $query and full: $ARGUMENTS"
	result := ExpandBodyFM(body, "abc123 machine learning", fm)

	if !strings.Contains(result, "abc123") {
		t.Errorf("expected paper_id substitution, got: %q", result)
	}
	if !strings.Contains(result, "machine") {
		t.Errorf("expected query substitution, got: %q", result)
	}
	if !strings.Contains(result, "abc123 machine learning") {
		t.Errorf("expected $ARGUMENTS substitution, got: %q", result)
	}
}

func TestExpandBodyFM_IndexedArgs(t *testing.T) {
	body := "First: $ARGUMENTS[0], Second: $ARGUMENTS[1]"
	result := ExpandBody(body, "hello world")

	if !strings.Contains(result, "hello") {
		t.Errorf("expected index 0 = hello, got: %q", result)
	}
	if !strings.Contains(result, "world") {
		t.Errorf("expected index 1 = world, got: %q", result)
	}
}

func TestParseFrontmatter_NoFrontmatter(t *testing.T) {
	input := "just a plain body without frontmatter"
	fm, body, err := ParseFrontmatter(input)
	if err != nil {
		t.Fatal(err)
	}
	if fm.Name != "" {
		t.Errorf("expected empty name, got %q", fm.Name)
	}
	if body != input {
		t.Errorf("body = %q, want %q", body, input)
	}
}
