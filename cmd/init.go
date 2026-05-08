package cmd

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/Nahasma/openscholar-public/internal/config"
	initwizard "github.com/Nahasma/openscholar-public/internal/init"
	"github.com/Nahasma/openscholar-public/internal/template"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a new paper project from a template",
	Example: `
  openscholar init --template neurips2026
  openscholar init --template icml2026
  openscholar init --template arxiv
  `,
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, _ := os.Getwd()

		// --soft flag: run the Soft Init questionnaire only
		softFlag, _ := cmd.Flags().GetBool("soft")
		if softFlag {
			return initwizard.RunSoftInit(cwd)
		}

		// --config flag: run the configuration wizard
		configFlag, _ := cmd.Flags().GetBool("config")
		if configFlag {
			return initwizard.RunHardInit(cwd)
		}

		templateName, _ := cmd.Flags().GetString("template")
		if templateName == "" {
			// List available templates
			templates := template.List()
			fmt.Println("Available templates:")
			for _, t := range templates {
				fmt.Printf("  - %s\n", t)
			}
			fmt.Println("\nUsage:")
			fmt.Println("  openscholar init --template <name>   Initialize paper project")
			fmt.Println("  openscholar init --config             Run configuration wizard")
			fmt.Println("  openscholar init --soft               Run personalization questionnaire")
			return nil
		}

		if err := template.Init(templateName, "."); err != nil {
			return err
		}

		// Copy shared commands to .openscholar/commands/
		if err := copySharedCommands("."); err != nil {
			// Non-fatal: just warn
			fmt.Fprintf(os.Stderr, "Warning: could not copy shared commands: %v\n", err)
		}

		return nil
	},
}

// copySharedCommands copies command templates from embedded shared/commands/ to .openscholar/commands/.
func copySharedCommands(targetDir string) error {
	if template.TemplatesFS == nil {
		return nil
	}

	srcDir := "shared/commands"
	destDir := filepath.Join(targetDir, config.DefaultDataDir(), "commands")

	// Check if shared commands exist in templates
	entries, err := fs.ReadDir(template.TemplatesFS, srcDir)
	if err != nil {
		return nil // no shared commands, that's fine
	}

	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("failed to create commands directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		data, err := fs.ReadFile(template.TemplatesFS, filepath.Join(srcDir, entry.Name()))
		if err != nil {
			continue
		}
		destPath := filepath.Join(destDir, entry.Name())
		// Don't overwrite existing commands
		if _, err := os.Stat(destPath); err == nil {
			continue
		}
		if err := os.WriteFile(destPath, data, 0o644); err != nil {
			return fmt.Errorf("failed to write command %s: %w", entry.Name(), err)
		}
	}

	fmt.Printf("Copied shared commands to %s\n", destDir)
	return nil
}

func init() {
	initCmd.Flags().StringP("template", "t", "", "Template name (neurips2026, icml2026, arxiv)")
	initCmd.Flags().Bool("config", false, "Run the configuration wizard")
	initCmd.Flags().Bool("soft", false, "Run the personalization questionnaire (Soft Init)")
	rootCmd.AddCommand(initCmd)
}
