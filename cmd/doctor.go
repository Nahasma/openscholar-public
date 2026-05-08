package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/openscholar/openscholar/internal/config"
	"github.com/openscholar/openscholar/internal/doctor"
	"github.com/spf13/cobra"
)

var doctorProviders bool

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check environment and auto-install missing dependencies",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Try to load config so API key check works
		cwd, _ := os.Getwd()
		cfg, _ := config.Load(cwd) // ignore error — doctor should work even without config

		if doctorProviders {
			if cfg == nil {
				fmt.Println("provider connectivity skipped: config is not loaded")
				return nil
			}
			report := doctor.CollectProviderConnectivity(context.Background(), cfg)
			fmt.Print(doctor.FormatProviderConnectivityReport(report))
			return nil
		}

		// Phase 1: Diagnose
		checks := doctor.RunChecks()
		fmt.Print(doctor.FormatReport(checks))

		// Phase 2: Auto-fix missing Python dependencies (both Fail and Warn)
		hasMissing := false
		for _, c := range checks {
			if (c.Status == doctor.StatusFail || c.Status == doctor.StatusWarn) && c.Fix != "" {
				hasMissing = true
				break
			}
		}

		if !hasMissing {
			return nil
		}

		fmt.Println("Installing missing dependencies...")
		fmt.Println()
		results := doctor.AutoFix(checks)
		fmt.Print(doctor.FormatFixResults(results))

		// Phase 3: Re-check to verify
		fmt.Println("Verifying...")
		fmt.Println()
		rechecks := doctor.RunChecks()
		fmt.Print(doctor.FormatReport(rechecks))

		return nil
	},
}

func init() {
	doctorCmd.Flags().BoolVar(&doctorProviders, "providers", false, "explicitly test configured provider connectivity")
	doctorCmd.Flags().BoolVar(&doctorProviders, "connectivity", false, "alias for --providers")
	rootCmd.AddCommand(doctorCmd)
}
