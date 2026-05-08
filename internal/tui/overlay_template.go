package tui

import (
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/openscholar/openscholar/internal/picker"
	"github.com/openscholar/openscholar/internal/tui/components"
)

// TemplateChoice represents the completed template selection.
type TemplateChoice struct {
	Template    string // "empirical" | "aris_empirical" | "survey" | "theoretical"
	FolderName  string
	PendingText string // stashed user input to resume
}

// TemplateOverlay implements Overlay for the research template selection dialog.
type TemplateOverlay struct {
	options     []string
	idx         int
	folderName  string
	pendingText string
	picker      *picker.State
	fileIndex   []picker.FileEntry
}

// NewTemplateOverlay creates a new template selection overlay.
func NewTemplateOverlay(options []string, pendingText string) *TemplateOverlay {
	return &TemplateOverlay{
		options:     options,
		idx:         0,
		pendingText: pendingText,
		picker:      picker.New(8),
	}
}

func (o *TemplateOverlay) ID() string        { return "template" }
func (o *TemplateOverlay) Kind() OverlayKind { return OverlayBlocking }
func (o *TemplateOverlay) BlocksInput() bool { return true }

func (o *TemplateOverlay) Update(msg tea.Msg) (Overlay, *OverlayResult, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return o, nil, nil
	}

	pk := o.picker

	// @ picker active: intercept navigation keys
	if pk != nil && pk.Active && len(pk.Items) > 0 {
		switch keyMsg.Type {
		case tea.KeyUp:
			pk.MoveUp()
			return o, nil, nil
		case tea.KeyDown:
			pk.MoveDown()
			return o, nil, nil
		case tea.KeyEnter, tea.KeyTab:
			entry, isDir := pk.Select()
			atIdx := strings.LastIndex(o.folderName, "@")
			if atIdx >= 0 {
				suffix := entry.RelPath
				if isDir {
					suffix += "/"
				}
				o.folderName = o.folderName[:atIdx] + suffix
			}
			if isDir {
				o.updatePickerQuery()
			} else {
				pk.Reset()
			}
			return o, nil, nil
		case tea.KeyEsc:
			pk.Reset()
			return o, nil, nil
		}
	}

	switch keyMsg.Type {
	case tea.KeyEnter:
		template := o.options[o.idx]
		return o, &OverlayResult{
			Action: "accept",
			Data: TemplateChoice{
				Template:    template,
				FolderName:  o.folderName,
				PendingText: o.pendingText,
			},
		}, nil

	case tea.KeyEsc:
		return o, &OverlayResult{Action: "dismiss"}, nil

	case tea.KeyUp:
		if o.idx > 0 {
			o.idx--
		}
		return o, nil, nil

	case tea.KeyDown:
		if o.idx < len(o.options)-1 {
			o.idx++
		}
		return o, nil, nil

	case tea.KeyBackspace:
		runes := []rune(o.folderName)
		if len(runes) > 0 {
			o.folderName = string(runes[:len(runes)-1])
		}
		o.updatePickerQuery()
		return o, nil, nil

	default:
		ch := keyMsg.String()
		if keyMsg.Type == tea.KeySpace {
			ch = " "
		}
		if ch != "" {
			o.folderName += ch
			if strings.Contains(ch, "@") && o.fileIndex == nil {
				cwd, _ := os.Getwd()
				o.fileIndex = picker.IndexFiles(cwd, picker.DefaultIndexConfig())
			}
			o.updatePickerQuery()
		}
		return o, nil, nil
	}
}

// updatePickerQuery extracts the @query from folderName and updates the picker.
func (o *TemplateOverlay) updatePickerQuery() {
	if o.picker == nil {
		o.picker = picker.New(8)
	}
	atIdx := strings.LastIndex(o.folderName, "@")
	if atIdx < 0 {
		o.picker.Reset()
		return
	}
	query := o.folderName[atIdx+1:]
	if o.fileIndex == nil {
		cwd, _ := os.Getwd()
		o.fileIndex = picker.IndexFiles(cwd, picker.DefaultIndexConfig())
	}
	o.picker.UpdateQuery(query, o.fileIndex)
}

func (o *TemplateOverlay) View(width, height int) string {
	overlay := components.RenderTemplateDialog(
		o.options, o.idx, o.folderName, width-4,
	)
	if o.picker != nil && o.picker.Active {
		visible := o.picker.VisibleItems()
		visIdx := o.picker.VisibleSelectedIndex()
		overlay += components.RenderFilePicker(visible, visIdx, width-4)
	}
	return overlay
}
