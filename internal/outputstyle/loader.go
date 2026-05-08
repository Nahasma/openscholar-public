package outputstyle

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Manager manages output style loading and activation.
type Manager struct {
	styles      map[string]*Style // name → style (project overrides user)
	activeStyle *Style
}

// NewManager creates a new Manager by scanning style directories.
// projectDir: <cwd>/.openscholar/output-styles/
// userDir: ~/.openscholar/output-styles/
// If a directory does not exist it is silently skipped.
func NewManager(projectDir, userDir string) *Manager {
	m := &Manager{
		styles: make(map[string]*Style),
	}
	// Load user styles first, then project styles (project overrides user).
	m.loadDir(userDir, "user")
	m.loadDir(projectDir, "project")
	return m
}

// loadDir scans a directory for *.md files and adds them to the style map.
func (m *Manager) loadDir(dir, source string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		// Directory not found or unreadable — silently skip.
		return
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".md") {
			continue
		}
		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		style := parseStyleFile(string(data), strings.TrimSuffix(name, ".md"), source)
		m.styles[style.Name] = style
	}
}

// parseStyleFile parses a Markdown file with optional YAML-like frontmatter.
// The frontmatter is delimited by "---" on its own line at the start of the file.
func parseStyleFile(raw, fallbackName, source string) *Style {
	style := &Style{
		Name:   fallbackName,
		Source: source,
	}

	content := raw
	if strings.HasPrefix(strings.TrimSpace(raw), "---") {
		// Find the closing "---"
		trimmed := strings.TrimLeft(raw, " \t\r\n")
		// Remove the opening "---\n"
		rest := trimmed[3:]
		// Find the next "---"
		if fm, body, found := strings.Cut(rest, "---"); found {
			// Strip leading newline from body
			body = strings.TrimLeft(body, "\r\n")
			parseFrontmatter(fm, style)
			content = body
		}
	}

	style.Content = content
	return style
}

// parseFrontmatter parses simple key: value pairs from frontmatter text.
func parseFrontmatter(fm string, style *Style) {
	for _, line := range strings.Split(fm, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		key, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		// Strip surrounding quotes
		if len(value) >= 2 && ((value[0] == '"' && value[len(value)-1] == '"') ||
			(value[0] == '\'' && value[len(value)-1] == '\'')) {
			value = value[1 : len(value)-1]
		}
		switch key {
		case "name":
			if value != "" {
				style.Name = value
			}
		case "description":
			style.Description = value
		case "keep-coding-instructions":
			style.KeepCodingInstructions = value == "true"
		}
	}
}

// SetActive activates a style by name. Returns error if not found.
func (m *Manager) SetActive(name string) error {
	s, ok := m.styles[name]
	if !ok {
		return fmt.Errorf("output style %q not found", name)
	}
	m.activeStyle = s
	return nil
}

// GetActive returns the currently active style, or nil.
func (m *Manager) GetActive() *Style {
	return m.activeStyle
}

// Clear deactivates the current style.
func (m *Manager) Clear() {
	m.activeStyle = nil
}

// List returns all available styles sorted by name.
func (m *Manager) List() []*Style {
	result := make([]*Style, 0, len(m.styles))
	for _, s := range m.styles {
		result = append(result, s)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result
}
