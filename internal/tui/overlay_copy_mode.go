package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/Nahasma/openscholar-public/internal/tui/components"
)

// CopyModeOverlay presents numbered code blocks for clipboard selection.
type CopyModeOverlay struct {
	blocks []CodeBlock
}

// NewCopyModeOverlay creates a CopyModeOverlay with the given code blocks.
func NewCopyModeOverlay(blocks []CodeBlock) *CopyModeOverlay {
	return &CopyModeOverlay{blocks: blocks}
}

func (o *CopyModeOverlay) ID() string        { return "copy-mode" }
func (o *CopyModeOverlay) Kind() OverlayKind { return OverlayNonBlocking }
func (o *CopyModeOverlay) BlocksInput() bool { return true }

func (o *CopyModeOverlay) Update(msg tea.Msg) (Overlay, *OverlayResult, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return o, nil, nil
	}

	switch {
	case keyMsg.Type == tea.KeyEsc:
		return o, &OverlayResult{Action: "dismiss"}, nil

	case keyMsg.Type == tea.KeyRunes && keyMsg.String() == "a":
		var all strings.Builder
		for i, b := range o.blocks {
			if i > 0 {
				all.WriteString("\n\n")
			}
			all.WriteString(b.Content)
		}
		return o, &OverlayResult{Action: "accept", Data: all.String()}, nil

	case keyMsg.Type == tea.KeyRunes:
		r := keyMsg.String()
		if len(r) == 1 && r[0] >= '1' && r[0] <= '9' {
			idx := int(r[0] - '1')
			if idx < len(o.blocks) {
				return o, &OverlayResult{Action: "accept", Data: o.blocks[idx].Content}, nil
			}
		}
		return o, &OverlayResult{Action: "dismiss"}, nil

	default:
		return o, &OverlayResult{Action: "dismiss"}, nil
	}
}

func (o *CopyModeOverlay) View(width, height int) string {
	copyBlocks := make([]components.CopyModeBlock, len(o.blocks))
	for i, b := range o.blocks {
		copyBlocks[i] = components.CopyModeBlock{Language: b.Language, Content: b.Content}
	}
	return components.RenderCopyModeOverlay(copyBlocks, width-4)
}
