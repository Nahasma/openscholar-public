package initwizard

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/Nahasma/openscholar-public/internal/config"
)

// RunHardInit launches the interactive configuration wizard.
// Called from cmd/root.go when config.Load() fails due to missing providers.
func RunHardInit(workingDir string) error {
	m := newWizardModel(workingDir)

	p := tea.NewProgram(m)
	finalModel, err := p.Run()
	if err != nil {
		return fmt.Errorf("wizard error: %w", err)
	}

	wizard, ok := finalModel.(wizardModel)
	if !ok {
		return fmt.Errorf("unexpected model type")
	}
	if wizard.err != nil {
		return wizard.err
	}
	if wizard.result == nil {
		return fmt.Errorf("no configuration generated")
	}

	// Save the config
	if err := config.SaveFull(workingDir, *wizard.result); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	return nil
}
