package cmd

import (
	"bufio"
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var logsCmd = &cobra.Command{
	Use:   "logs <app>",
	Short: "View app logs",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		follow, _ := cmd.Flags().GetBool("follow")
		tail, _ := cmd.Flags().GetInt("tail")

		cfg := apiClient.GetConfig()
		if cfg.TenantID == "" {
			return fmt.Errorf("no active tenant")
		}

		app, err := apiClient.GetAppByName(context.Background(), args[0])
		if err != nil {
			return err
		}

		body, err := apiClient.StreamLogs(context.Background(), app.ID, follow, tail)
		if err != nil {
			return err
		}
		defer func() { _ = body.Close() }()

		scanner := bufio.NewScanner(body)
		for scanner.Scan() {
			line := scanner.Text()
			// SSE format: "data: ..."
			if strings.HasPrefix(line, "data: ") {
				fmt.Println(line[6:])
			} else if line != "" && !strings.HasPrefix(line, ":") {
				fmt.Println(line)
			}
		}

		return scanner.Err()
	},
}

func init() {
	logsCmd.Flags().BoolP("follow", "f", false, "Follow log output")
	logsCmd.Flags().Int("tail", 100, "Number of recent lines to show")
}
