package components

import (
	"time"

	"github.com/charmbracelet/lipgloss"
)

const memoryToastTTL = 3 * time.Second

// MemoryToast represents a single memory update notification.
type MemoryToast struct {
	FilePath  string    // e.g. "user_preferences.md"
	Action    string    // e.g. "updated", "created", "deleted"
	CreatedAt time.Time
}

// MemoryToastManager manages a single toast with TTL auto-expiry.
type MemoryToastManager struct {
	current *MemoryToast
}

// NewMemoryToastManager creates a new MemoryToastManager.
func NewMemoryToastManager() *MemoryToastManager {
	return &MemoryToastManager{}
}

// Push replaces the current toast (new toast always replaces old one).
func (m *MemoryToastManager) Push(toast MemoryToast) {
	if toast.CreatedAt.IsZero() {
		toast.CreatedAt = time.Now()
	}
	m.current = &toast
}

// Current returns the current active toast, or nil if expired or none.
func (m *MemoryToastManager) Current() *MemoryToast {
	if m.current == nil {
		return nil
	}
	if time.Since(m.current.CreatedAt) >= memoryToastTTL {
		m.current = nil
		return nil
	}
	return m.current
}

// Tick checks TTL and clears expired toast. Call this on each tick (100ms).
func (m *MemoryToastManager) Tick() {
	if m.current != nil && time.Since(m.current.CreatedAt) >= memoryToastTTL {
		m.current = nil
	}
}

// RenderMemoryToast renders a single toast notification line.
// Format: "  ∙ Memory updated → user_preferences.md"
// Uses dim styling (Theme.TextMuted) so it doesn't steal focus.
func RenderMemoryToast(toast MemoryToast, width int) string {
	mutedStyle := lipgloss.NewStyle().Foreground(Theme.TextMuted)
	infoStyle := lipgloss.NewStyle().Foreground(Theme.Info)

	bullet := mutedStyle.Render(FigBullet)
	memWord := infoStyle.Render("Memory")
	action := toast.Action
	if action == "" {
		action = "updated"
	}

	// Build the prefix: "  ∙ Memory <action> → "
	prefix := "  " + bullet + " " + memWord + " " + mutedStyle.Render(action+" → ")

	// Calculate remaining width for the file path
	prefixWidth := lipgloss.Width(prefix)
	fileWidth := width - prefixWidth
	if fileWidth < 4 {
		fileWidth = 4
	}

	filePath := toast.FilePath
	fileRunes := []rune(filePath)
	if len(fileRunes) > fileWidth {
		// Truncate from left with ellipsis to keep the filename visible
		if fileWidth > 3 {
			filePath = "…" + string(fileRunes[len(fileRunes)-(fileWidth-1):])
		} else {
			filePath = string(fileRunes[:fileWidth])
		}
	}

	fileStr := mutedStyle.Render(filePath)
	return prefix + fileStr + "\n"
}
