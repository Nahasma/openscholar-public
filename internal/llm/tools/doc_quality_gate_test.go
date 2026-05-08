package tools

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// helper to count issues by category
func countByCategory(issues []QualityIssue, category string) int {
	n := 0
	for _, iss := range issues {
		if iss.Category == category {
			n++
		}
	}
	return n
}

// helper to count issues by severity
func countBySeverity(issues []QualityIssue, severity string) int {
	n := 0
	for _, iss := range issues {
		if iss.Severity == severity {
			n++
		}
	}
	return n
}

// hasIssue returns true if any issue matches the given severity and category.
func hasIssue(issues []QualityIssue, severity, category string) bool {
	for _, iss := range issues {
		if iss.Severity == severity && iss.Category == category {
			return true
		}
	}
	return false
}

func TestCheckDocument_EmptySection(t *testing.T) {
	input := `# Title

## Section 1

Some content here.

## Empty Section

## Section 3

More content.
`
	lines := strings.Split(input, "\n")
	issues := checkDocument(lines, input, "/tmp/test.md")

	emptySectionIssues := 0
	for _, iss := range issues {
		if iss.Category == "empty_section" && iss.Severity == "ERROR" {
			if strings.Contains(iss.Message, "Empty Section") {
				emptySectionIssues++
			}
		}
	}

	if emptySectionIssues == 0 {
		t.Errorf("expected at least one empty_section ERROR for 'Empty Section', got issues: %+v", issues)
	}
}

func TestCheckDocument_EmptyList(t *testing.T) {
	input := `# Guide

这种模式有几个局限：

## Next Section
`
	lines := strings.Split(input, "\n")
	issues := checkDocument(lines, input, "/tmp/test.md")

	found := false
	for _, iss := range issues {
		if (iss.Category == "empty_list" || iss.Category == "dangling_colon") && iss.Severity == "ERROR" {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("expected ERROR for dangling list introduction '局限：', got issues: %+v", issues)
	}
}

func TestCheckDocument_BrokenImageRef(t *testing.T) {
	input := `# Doc

Here is an image:

![My Figure]()

And another:

![Missing](/nonexistent/path/image.png)
`
	lines := strings.Split(input, "\n")
	issues := checkDocument(lines, input, "/tmp/test.md")

	// Empty URL should be ERROR broken_image
	hasEmptyURLError := false
	for _, iss := range issues {
		if iss.Category == "broken_image" && iss.Severity == "ERROR" {
			hasEmptyURLError = true
			break
		}
	}
	if !hasEmptyURLError {
		t.Errorf("expected ERROR broken_image for empty image URL, got: %+v", issues)
	}

	// Missing file should be WARNING broken_image
	hasMissingFileWarning := false
	for _, iss := range issues {
		if iss.Category == "broken_image" && iss.Severity == "WARNING" {
			hasMissingFileWarning = true
			break
		}
	}
	if !hasMissingFileWarning {
		t.Errorf("expected WARNING broken_image for missing image file, got: %+v", issues)
	}
}

func TestCheckDocument_Placeholders(t *testing.T) {
	input := `# Doc

This section is TODO.

Some content with [待补充] marker.
`
	lines := strings.Split(input, "\n")
	issues := checkDocument(lines, input, "/tmp/test.md")

	todoFound := false
	placeholderFound := false
	for _, iss := range issues {
		if iss.Category == "placeholder" && iss.Severity == "WARNING" {
			if strings.Contains(iss.Message, "TODO") {
				todoFound = true
			}
			if strings.Contains(iss.Message, "[待补充]") {
				placeholderFound = true
			}
		}
	}

	if !todoFound {
		t.Errorf("expected WARNING placeholder for 'TODO', got: %+v", issues)
	}
	if !placeholderFound {
		t.Errorf("expected WARNING placeholder for '[待补充]', got: %+v", issues)
	}
}

func TestCheckDocument_PromisedAsset(t *testing.T) {
	input := `# Guide

## 一张图理解

Agent 是大框架，Skill 是零部件。
两者配合才能完成任务。
`
	lines := strings.Split(input, "\n")
	issues := checkDocument(lines, input, "/tmp/test.md")

	found := false
	for _, iss := range issues {
		if iss.Category == "promised_asset" && iss.Severity == "ERROR" {
			if strings.Contains(iss.Message, "一张图理解") {
				found = true
				break
			}
		}
	}

	if !found {
		t.Errorf("expected ERROR promised_asset for section '一张图理解' with no image, got: %+v", issues)
	}
}

func TestCheckDocument_CleanDoc(t *testing.T) {
	// H1 is now skipped for empty/short checks, so a standard title-only H1 is fine.
	input := `# Good Document

## Introduction

This is a well-written introduction with enough content.
It has multiple paragraphs and good structure.
Each section covers its topic thoroughly.

## Details

More detailed content here with specific examples.
The section has good depth and coverage.
It references external sources properly.
`
	lines := strings.Split(input, "\n")
	issues := checkDocument(lines, input, "/tmp/test.md")

	if len(issues) != 0 {
		t.Errorf("expected 0 issues for clean document, got %d: %+v", len(issues), issues)
	}
}

func TestCheckDocument_ShortSection(t *testing.T) {
	input := `# Doc

## Very Short

One line.

## Normal Section

This section has enough content.
Multiple lines of text here.
And more content to make it substantial.
`
	lines := strings.Split(input, "\n")
	issues := checkDocument(lines, input, "/tmp/test.md")

	found := false
	for _, iss := range issues {
		if iss.Category == "short_section" && iss.Severity == "WARNING" {
			if strings.Contains(iss.Message, "Very Short") {
				found = true
				break
			}
		}
	}

	if !found {
		t.Errorf("expected WARNING short_section for 'Very Short', got: %+v", issues)
	}

	// Normal Section should not be flagged as short
	for _, iss := range issues {
		if iss.Category == "short_section" && strings.Contains(iss.Message, "Normal Section") {
			t.Errorf("Normal Section should not be flagged as short_section, got: %+v", iss)
		}
	}
}

func TestCheckDocument_DanglingColon(t *testing.T) {
	// checkDanglingColons fires when a line ending with "：" or ":" is followed
	// by a heading (or EOF) rather than actual list content.
	input := `# Doc

Some context about the problem.
Many cases fall into this category.
This is widely documented in literature.

在AI领域，Skill通常指：

## Next Section

More content.
More content here too.
And even more to be safe.
`
	lines := strings.Split(input, "\n")
	issues := checkDocument(lines, input, "/tmp/test.md")

	found := false
	for _, iss := range issues {
		if iss.Severity == "ERROR" && (iss.Category == "dangling_colon" || iss.Category == "empty_list") {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("expected ERROR for dangling colon pattern (colon followed by heading), got: %+v", issues)
	}
}

func TestDocQualityGateTool_Integration(t *testing.T) {
	// Create a temp file with known issues
	content := `# Test Document

## Good Section

This section has enough content to not trigger short_section.
Multiple lines with substantial text here.
More content to ensure length.

## Empty Section

## Section With TODO

This section contains TODO placeholder text.

## Short

Only one line.
`
	tmpFile, err := os.CreateTemp(t.TempDir(), "quality-gate-*.md")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()

	tool := NewDocQualityGateTool()

	params := map[string]any{
		"file_path": tmpFile.Name(),
	}
	paramJSON, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}

	call := ToolCall{
		ID:    "tc-quality-gate-1",
		Name:  "DocQualityGate",
		Input: string(paramJSON),
	}

	resp, err := tool.Run(context.Background(), call)
	if err != nil {
		t.Fatalf("tool.Run returned unexpected error: %v", err)
	}
	if resp.IsError {
		t.Fatalf("tool.Run returned error response: %s", resp.Content)
	}

	// The report should mention the empty section
	if !strings.Contains(resp.Content, "Empty Section") {
		t.Errorf("response should mention 'Empty Section', got:\n%s", resp.Content)
	}

	// The report should mention TODO placeholder
	if !strings.Contains(resp.Content, "TODO") && !strings.Contains(strings.ToUpper(resp.Content), "TODO") {
		t.Errorf("response should mention 'TODO' placeholder, got:\n%s", resp.Content)
	}

	// Should have ERROR count > 0
	if !strings.Contains(resp.Content, "ERROR") {
		t.Errorf("response should contain ERROR severity, got:\n%s", resp.Content)
	}
}

func TestDocQualityGateTool_MissingFile(t *testing.T) {
	tool := NewDocQualityGateTool()

	params := map[string]any{
		"file_path": "/nonexistent/path/to/file.md",
	}
	paramJSON, _ := json.Marshal(params)

	call := ToolCall{
		ID:    "tc-quality-gate-missing",
		Name:  "DocQualityGate",
		Input: string(paramJSON),
	}

	resp, err := tool.Run(context.Background(), call)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.IsError {
		t.Errorf("expected error response for missing file, got: %s", resp.Content)
	}
}

func TestDocQualityGateTool_EmptyFilePath(t *testing.T) {
	tool := NewDocQualityGateTool()

	params := map[string]any{}
	paramJSON, _ := json.Marshal(params)

	call := ToolCall{
		ID:    "tc-quality-gate-empty",
		Name:  "DocQualityGate",
		Input: string(paramJSON),
	}

	resp, err := tool.Run(context.Background(), call)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.IsError {
		t.Errorf("expected error response for empty file_path, got: %s", resp.Content)
	}
}

func TestDocQualityGateTool_InvalidJSON(t *testing.T) {
	tool := NewDocQualityGateTool()

	call := ToolCall{
		ID:    "tc-quality-gate-badjson",
		Name:  "DocQualityGate",
		Input: "not valid json",
	}

	resp, err := tool.Run(context.Background(), call)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.IsError {
		t.Errorf("expected error response for invalid JSON, got: %s", resp.Content)
	}
}

func TestDocQualityGateTool_CleanFile(t *testing.T) {
	// H1 needs at least 3 non-blank lines before the first H2 to avoid
	// empty_section and short_section checks.
	content := `# Clean Document

This document has no quality issues at all.
It is thoroughly written with good structure.
Each section provides sufficient depth and coverage.

## Introduction

This document has no quality issues at all.
It has multiple paragraphs and good structure.
Each section covers its topic thoroughly.

## Details

More detailed content here with specific examples.
The section has good depth and coverage.
It references external sources properly.
`
	tmpFile, err := os.CreateTemp(t.TempDir(), "clean-doc-*.md")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()

	tool := NewDocQualityGateTool()

	params := map[string]any{"file_path": tmpFile.Name()}
	paramJSON, _ := json.Marshal(params)

	call := ToolCall{
		ID:    "tc-quality-gate-clean",
		Name:  "DocQualityGate",
		Input: string(paramJSON),
	}

	resp, err := tool.Run(context.Background(), call)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.IsError {
		t.Fatalf("unexpected error response: %s", resp.Content)
	}
	if !strings.Contains(resp.Content, "No issues found") {
		t.Errorf("expected 'No issues found' for clean document, got:\n%s", resp.Content)
	}
}

func TestDocQualityGateTool_Info(t *testing.T) {
	tool := NewDocQualityGateTool()
	info := tool.Info()

	if info.Name != "DocQualityGate" {
		t.Errorf("expected tool name 'DocQualityGate', got %q", info.Name)
	}
	if info.Description == "" {
		t.Error("tool description should not be empty")
	}
	if len(info.Required) == 0 {
		t.Error("tool should have required parameters")
	}
}
