package tui

import (
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/Nahasma/openscholar-public/internal/tui/components"
)

// WorkspaceChoice represents the workspace selection result.
type WorkspaceChoice struct {
	Action string // "confirm" | "change"
	Path   string // resolved absolute path when Action="change"
}

// WorkspaceOverlay implements Overlay for the workspace selection dialog.
type WorkspaceOverlay struct {
	cwd       string // current working directory for display
	idx       int
	inputMode bool
	input     string
}

// NewWorkspaceOverlay creates a new workspace selection overlay.
func NewWorkspaceOverlay(cwd string) *WorkspaceOverlay {
	return &WorkspaceOverlay{cwd: cwd}
}

func (o *WorkspaceOverlay) ID() string        { return "workspace" }
func (o *WorkspaceOverlay) Kind() OverlayKind { return OverlayBlocking }
func (o *WorkspaceOverlay) BlocksInput() bool { return true }

func (o *WorkspaceOverlay) Update(msg tea.Msg) (Overlay, *OverlayResult, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return o, nil, nil
	}

	if o.inputMode {
		switch keyMsg.Type {
		case tea.KeyEsc:
			o.inputMode = false
			o.input = ""
			return o, nil, nil
		case tea.KeyEnter:
			dir := strings.TrimSpace(o.input)
			if dir != "" {
				absDir, err := filepath.Abs(dir)
				if err == nil {
					if info, statErr := os.Stat(absDir); statErr == nil && info.IsDir() {
						return o, &OverlayResult{
							Action: "accept",
							Data:   WorkspaceChoice{Action: "change", Path: absDir},
						}, nil
					}
				}
				// Invalid path, stay in input mode
				o.input = ""
			}
			return o, nil, nil
		case tea.KeyBackspace:
			if len(o.input) > 0 {
				runes := []rune(o.input)
				o.input = string(runes[:len(runes)-1])
			}
			return o, nil, nil
		default:
			if keyMsg.Type == tea.KeyRunes || keyMsg.Type == tea.KeySpace {
				o.input += keyMsg.String()
			}
			return o, nil, nil
		}
	}

	// Selection mode
	switch keyMsg.Type {
	case tea.KeyUp, tea.KeyLeft:
		if o.idx > 0 {
			o.idx--
		}
		return o, nil, nil
	case tea.KeyDown, tea.KeyRight:
		if o.idx < 1 {
			o.idx++
		}
		return o, nil, nil
	case tea.KeyEnter:
		return o.executeChoice()
	}

	switch keyMsg.String() {
	case "1":
		o.idx = 0
		return o.executeChoice()
	case "2":
		o.idx = 1
		return o.executeChoice()
	}

	return o, nil, nil
}

func (o *WorkspaceOverlay) executeChoice() (Overlay, *OverlayResult, tea.Cmd) {
	switch o.idx {
	case 0: // 使用当前目录
		return o, &OverlayResult{
			Action: "accept",
			Data:   WorkspaceChoice{Action: "confirm", Path: o.cwd},
		}, nil
	case 1: // 更换目录
		o.inputMode = true
		o.input = ""
		return o, nil, nil
	}
	return o, nil, nil
}

func (o *WorkspaceOverlay) View(width, height int) string {
	return components.RenderWorkspaceDialog(o.cwd, o.idx, o.inputMode, o.input, width-4)
}
