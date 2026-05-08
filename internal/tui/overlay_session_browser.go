package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/Nahasma/openscholar-public/internal/session"
	"github.com/Nahasma/openscholar-public/internal/tui/components"
)

// SessionBrowserChoice represents the user's session browser action.
type SessionBrowserChoice struct {
	Action    string // "select" | "new" | "delete"
	SessionID string
}

// SessionBrowserOverlay implements Overlay for the session browser dialog.
type SessionBrowserOverlay struct {
	list       []session.Session
	listIdx    int
	listScroll int
}

// NewSessionBrowserOverlay creates a new session browser overlay.
func NewSessionBrowserOverlay(sessions []session.Session) *SessionBrowserOverlay {
	return &SessionBrowserOverlay{
		list: sessions,
	}
}

func (o *SessionBrowserOverlay) ID() string        { return "session-browser" }
func (o *SessionBrowserOverlay) Kind() OverlayKind { return OverlayBlocking }
func (o *SessionBrowserOverlay) BlocksInput() bool { return true }

func (o *SessionBrowserOverlay) Update(msg tea.Msg) (Overlay, *OverlayResult, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return o, nil, nil
	}

	switch keyMsg.Type {
	case tea.KeyEsc:
		return o, &OverlayResult{Action: "dismiss"}, nil

	case tea.KeyUp:
		if o.listIdx > 0 {
			o.listIdx--
		}
		return o, nil, nil

	case tea.KeyDown:
		if o.listIdx < len(o.list)-1 {
			o.listIdx++
		}
		return o, nil, nil

	case tea.KeyEnter:
		if len(o.list) > 0 && o.listIdx < len(o.list) {
			sess := o.list[o.listIdx]
			return o, &OverlayResult{
				Action: "accept",
				Data:   SessionBrowserChoice{Action: "select", SessionID: sess.ID},
			}, nil
		}
		return o, nil, nil
	}

	switch keyMsg.String() {
	case "n":
		return o, &OverlayResult{
			Action: "accept",
			Data:   SessionBrowserChoice{Action: "new"},
		}, nil

	case "d":
		if len(o.list) > 0 && o.listIdx < len(o.list) {
			sess := o.list[o.listIdx]
			return o, &OverlayResult{
				Action: "accept",
				Data:   SessionBrowserChoice{Action: "delete", SessionID: sess.ID},
			}, nil
		}
	}

	return o, nil, nil
}

func (o *SessionBrowserOverlay) View(width, height int) string {
	if height <= 0 {
		return ""
	}
	browserHeight := height / 2
	if browserHeight < 8 {
		browserHeight = 8
	}
	if browserHeight > height {
		browserHeight = height
	}
	return clampRenderedHeight(
		components.RenderSessionBrowser(o.list, o.listIdx, o.listScroll, width, browserHeight),
		height,
	)
}
