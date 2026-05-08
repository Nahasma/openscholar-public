package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/openscholar/openscholar/internal/app"
	"github.com/openscholar/openscholar/internal/config"
	"github.com/openscholar/openscholar/internal/db"
	"github.com/openscholar/openscholar/internal/server"
	"github.com/spf13/cobra"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the Web GUI server",
	Example: `
  openscholar serve
  openscholar serve --port 3000
  `,
	RunE: func(cmd *cobra.Command, args []string) error {
		port, _ := cmd.Flags().GetInt("port")
		cwd, _ := cmd.Flags().GetString("cwd")

		if cwd == "" {
			var err error
			cwd, err = os.Getwd()
			if err != nil {
				return fmt.Errorf("failed to get working directory: %w", err)
			}
		}

		if _, err := config.Load(cwd); err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		conn, err := db.Connect()
		if err != nil {
			return fmt.Errorf("failed to connect to database: %w", err)
		}

		ctx := context.Background()
		application, err := app.New(ctx, conn)
		if err != nil {
			return fmt.Errorf("failed to create application: %w", err)
		}
		defer application.Shutdown()

		srv := server.New(application)
		addr := fmt.Sprintf(":%d", port)
		return srv.Start(addr)
	},
}

func init() {
	serveCmd.Flags().Int("port", 8080, "Port to listen on")
	serveCmd.Flags().String("cwd", "", "Working directory")
	rootCmd.AddCommand(serveCmd)
}
