package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/Nahasma/openscholar-public/internal/tui/components"
)

// ResearchSuggestionOverlay prompts the user to choose between research mode
// and normal chat for a detected research-intent message.
type ResearchSuggestionOverlay struct {
	idx  int    // 0 = accept research, 1 = continue normal
	text string // stashed user input to pass back in result
}

// NewResearchSuggestionOverlay creates a ResearchSuggestionOverlay with the
// original user text that triggered the suggestion.
func NewResearchSuggestionOverlay(text string) *ResearchSuggestionOverlay {
	return &ResearchSuggestionOverlay{text: text}
}

func (o *ResearchSuggestionOverlay) ID() string        { return "research-suggestion" }
func (o *ResearchSuggestionOverlay) Kind() OverlayKind { return OverlayBlocking }
func (o *ResearchSuggestionOverlay) BlocksInput() bool { return true }

func (o *ResearchSuggestionOverlay) Update(msg tea.Msg) (Overlay, *OverlayResult, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return o, nil, nil
	}

	switch keyMsg.Type {
	case tea.KeyUp, tea.KeyLeft:
		if o.idx > 0 {
			o.idx--
		}
	case tea.KeyDown, tea.KeyRight:
		if o.idx < 1 {
			o.idx++
		}
	case tea.KeyEnter:
		if o.idx == 0 {
			return o, &OverlayResult{Action: "accept", Data: o.text}, nil
		}
		return o, &OverlayResult{Action: "reject", Data: o.text}, nil
	case tea.KeyEsc:
		return o, &OverlayResult{Action: "reject", Data: o.text}, nil
	default:
		switch keyMsg.String() {
		case "1", "y":
			o.idx = 0
			return o, &OverlayResult{Action: "accept", Data: o.text}, nil
		case "2", "n":
			o.idx = 1
			return o, &OverlayResult{Action: "reject", Data: o.text}, nil
		}
	}

	return o, nil, nil
}

func (o *ResearchSuggestionOverlay) View(width, height int) string {
	return components.RenderResearchSuggestionDialog(o.idx, width-4)
}
