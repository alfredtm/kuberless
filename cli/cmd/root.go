package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/alfredtm/kuberless/cli/client"
)

var apiClient *client.Client

var rootCmd = &cobra.Command{
	Use:   "kuberless",
	Short: "Serverless platform CLI",
	Long:  "Deploy and manage containerized applications on the kuberless platform.",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Skip client init for login command
		if cmd.Name() == "login" {
			return nil
		}
		var err error
		apiClient, err = client.New()
		if err != nil {
			return fmt.Errorf("initializing client: %w", err)
		}
		return nil
	},
}

// Execute runs the root command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(loginCmd)
	rootCmd.AddCommand(tenantCmd)
	rootCmd.AddCommand(deployCmd)
	rootCmd.AddCommand(appsCmd)
	rootCmd.AddCommand(logsCmd)
	rootCmd.AddCommand(envCmd)
	rootCmd.AddCommand(domainsCmd)
}
