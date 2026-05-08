package cmd

import (
	"fmt"

	"github.com/openscholar/openscholar/internal/version"
	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("openscholar %s\n", version.Version)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
