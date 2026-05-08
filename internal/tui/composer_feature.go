package tui

import (
	"github.com/Nahasma/openscholar-public/internal/command"
	"github.com/Nahasma/openscholar-public/internal/picker"
	"github.com/Nahasma/openscholar-public/internal/tui/components"
)

// ComposerFeature groups input, command picker, and file picker state.
type ComposerFeature struct {
	input                  components.InputModel
	commandPickerActive    bool
	commandPickerIdx       int
	filteredCompletions    []command.CompletionItem
	commandOutput          string
	commandDialog          CommandDialogState
	pendingCommandActivity CommandActivityState
	filePicker             *picker.State
	fileIndex              []picker.FileEntry
	fileIndexRoot          string
	fileIndexRefreshing    bool
	prevInputHeight        int
}

type CommandDialogState struct {
	Visible         bool
	Invocation      string
	Title           string
	Body            string
	DismissText     string
	RecordOnDismiss bool
}

type CommandActivityState struct {
	Invocation string
}

// NewComposerFeature creates a ComposerFeature with initialized state.
func NewComposerFeature() ComposerFeature {
	return ComposerFeature{
		input:      components.NewInputModel(),
		filePicker: picker.New(8),
	}
}
