package cmd

import (
	"fmt"
	"os"

	"github.com/openscholar/openscholar/internal/config"
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage local project configuration",
}

var configInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Create a local project config template",
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}

		path, err := config.InitConfigFile(cwd)
		if err != nil {
			return err
		}

		fmt.Printf("Created config template: %s\n", path)
		return nil
	},
}

func init() {
	configCmd.AddCommand(configInitCmd)
	rootCmd.AddCommand(configCmd)
}
