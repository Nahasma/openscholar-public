package cmd

import (
	"fmt"
	"os"

	"github.com/openscholar/openscholar/internal/config"
	"github.com/spf13/cobra"
)

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Manage API authentication",
}

var authStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show authentication status",
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		if _, err := config.Load(cwd); err != nil {
			fmt.Println(err)
			return nil
		}

		for _, providerName := range config.KnownProviders() {
			key := config.LoadAPIKey(providerName)
			if key == "" {
				fmt.Printf("%-18s not configured\n", providerName)
				continue
			}

			providerCfg := config.Get().Providers[providerName]
			line := fmt.Sprintf("%-18s %s", providerName, maskKey(key))
			if providerCfg.BaseURL != "" {
				line += " (" + providerCfg.BaseURL + ")"
			}
			fmt.Println(line)
		}
		return nil
	},
}

func maskKey(key string) string {
	if len(key) <= 12 {
		return "****"
	}
	return key[:8] + "..." + key[len(key)-4:]
}

func init() {
	authCmd.AddCommand(authStatusCmd)
	rootCmd.AddCommand(authCmd)
}
