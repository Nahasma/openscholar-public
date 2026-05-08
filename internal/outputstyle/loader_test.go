package outputstyle

import (
	"os"
	"path/filepath"
	"testing"
)

// writeStyleFile is a test helper that writes content to a .md file in dir.
func writeStyleFile(t *testing.T, dir, filename, content string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	path := filepath.Join(dir, filename)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile %s: %v", path, err)
	}
}

func TestNewManager_EmptyDirs(t *testing.T) {
	// Non-existent directories must not cause an error.
	m := NewManager("/tmp/nonexistent-project-styles", "/tmp/nonexistent-user-styles")
	if m == nil {
		t.Fatal("expected non-nil Manager")
	}
	if len(m.List()) != 0 {
		t.Fatalf("expected 0 styles, got %d", len(m.List()))
	}
}

func TestNewManager_LoadStyles(t *testing.T) {
	dir := t.TempDir()
	userDir := filepath.Join(dir, "user")
	writeStyleFile(t, userDir, "academic.md", `---
name: "Academic"
description: "Formal academic writing"
keep-coding-instructions: false
---
Write in a formal academic tone.
`)

	m := NewManager("", userDir)
	styles := m.List()
	if len(styles) != 1 {
		t.Fatalf("expected 1 style, got %d", len(styles))
	}
	s := styles[0]
	if s.Name != "Academic" {
		t.Errorf("expected name %q, got %q", "Academic", s.Name)
	}
	if s.Description != "Formal academic writing" {
		t.Errorf("unexpected description: %q", s.Description)
	}
	if s.KeepCodingInstructions {
		t.Errorf("expected KeepCodingInstructions=false")
	}
	if s.Source != "user" {
		t.Errorf("expected source %q, got %q", "user", s.Source)
	}
	if s.Content == "" {
		t.Errorf("expected non-empty content")
	}
}

func TestManager_ProjectOverridesUser(t *testing.T) {
	dir := t.TempDir()
	userDir := filepath.Join(dir, "user")
	projDir := filepath.Join(dir, "project")

	// Same filename in both directories — project should win.
	writeStyleFile(t, userDir, "style.md", `---
name: "Style"
description: "User version"
---
User content.
`)
	writeStyleFile(t, projDir, "style.md", `---
name: "Style"
description: "Project version"
---
Project content.
`)

	m := NewManager(projDir, userDir)
	styles := m.List()
	if len(styles) != 1 {
		t.Fatalf("expected 1 style (project overrides user), got %d", len(styles))
	}
	if styles[0].Source != "project" {
		t.Errorf("expected source %q, got %q", "project", styles[0].Source)
	}
	if styles[0].Description != "Project version" {
		t.Errorf("expected project description, got %q", styles[0].Description)
	}
}

func TestManager_SetActive(t *testing.T) {
	dir := t.TempDir()
	userDir := filepath.Join(dir, "user")
	writeStyleFile(t, userDir, "casual.md", "Be casual.\n")

	m := NewManager("", userDir)
	if err := m.SetActive("casual"); err != nil {
		t.Fatalf("SetActive: %v", err)
	}
	active := m.GetActive()
	if active == nil {
		t.Fatal("expected active style, got nil")
	}
	if active.Name != "casual" {
		t.Errorf("expected name %q, got %q", "casual", active.Name)
	}
}

func TestManager_SetActive_NotFound(t *testing.T) {
	m := NewManager("", "")
	err := m.SetActive("nonexistent")
	if err == nil {
		t.Fatal("expected error for missing style, got nil")
	}
}

func TestManager_Clear(t *testing.T) {
	dir := t.TempDir()
	userDir := filepath.Join(dir, "user")
	writeStyleFile(t, userDir, "formal.md", "Be formal.\n")

	m := NewManager("", userDir)
	_ = m.SetActive("formal")
	m.Clear()
	if m.GetActive() != nil {
		t.Fatal("expected nil after Clear")
	}
}

func TestParseFrontmatter(t *testing.T) {
	raw := `---
name: "My Style"
description: "A test style"
keep-coding-instructions: true
---
Body content here.
`
	style := parseStyleFile(raw, "fallback", "user")
	if style.Name != "My Style" {
		t.Errorf("expected name %q, got %q", "My Style", style.Name)
	}
	if style.Description != "A test style" {
		t.Errorf("expected description %q, got %q", "A test style", style.Description)
	}
	if !style.KeepCodingInstructions {
		t.Errorf("expected KeepCodingInstructions=true")
	}
	if !containsStr(style.Content, "Body content here.") {
		t.Errorf("expected body in content, got %q", style.Content)
	}
}

func TestManager_List(t *testing.T) {
	dir := t.TempDir()
	userDir := filepath.Join(dir, "user")
	projDir := filepath.Join(dir, "project")
	writeStyleFile(t, userDir, "alpha.md", "Alpha.\n")
	writeStyleFile(t, userDir, "beta.md", "Beta.\n")
	writeStyleFile(t, projDir, "gamma.md", "Gamma.\n")

	m := NewManager(projDir, userDir)
	styles := m.List()
	if len(styles) != 3 {
		t.Fatalf("expected 3 styles, got %d", len(styles))
	}
	// Verify sorted order.
	names := make([]string, len(styles))
	for i, s := range styles {
		names[i] = s.Name
	}
	expected := []string{"alpha", "beta", "gamma"}
	for i, want := range expected {
		if names[i] != want {
			t.Errorf("position %d: expected %q, got %q", i, want, names[i])
		}
	}
}

// containsStr is a helper to check substring presence.
func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsSubstring(s, sub))
}

func containsSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
