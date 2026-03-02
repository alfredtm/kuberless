package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/alfredtm/kuberless/cli/client"
)

var deployCmd = &cobra.Command{
	Use:   "deploy <image>",
	Short: "Deploy a container image",
	Long:  "Deploy a container image as a new app or update an existing app.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		image := args[0]
		name, _ := cmd.Flags().GetString("name")
		port, _ := cmd.Flags().GetInt32("port")
		envFlags, _ := cmd.Flags().GetStringSlice("env")

		cfg := apiClient.GetConfig()
		if cfg.TenantID == "" {
			return fmt.Errorf("no active tenant. Use 'kuberless tenant switch <name>' first")
		}

		// Parse env vars.
		envVars := make(map[string]string)
		for _, e := range envFlags {
			parts := strings.SplitN(e, "=", 2)
			if len(parts) != 2 {
				return fmt.Errorf("invalid env var format: %q (expected KEY=VALUE)", e)
			}
			envVars[parts[0]] = parts[1]
		}

		// If no name, derive from image.
		if name == "" {
			name = deriveAppName(image)
		}

		fmt.Printf("Deploying %s as %s...\n", image, name)

		resp, err := apiClient.CreateApp(context.Background(), &client.CreateAppRequest{
			Name:  name,
			Image: image,
			Port:  port,
			Env:   envVars,
		})
		if err != nil {
			// Try update if app already exists.
			existing, lookupErr := apiClient.GetAppByName(context.Background(), name)
			if lookupErr != nil {
				return fmt.Errorf("deploying app: %w", err)
			}
			resp, err = apiClient.UpdateApp(context.Background(), existing.ID, &client.UpdateAppRequest{
				Image: image,
				Port:  port,
			})
			if err != nil {
				return fmt.Errorf("deploying app: %w", err)
			}
		}

		fmt.Printf("App: %s\n", resp.Name)
		fmt.Printf("Phase: %s\n", resp.Phase)

		// Poll for ready status.
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				fmt.Println("\nTimeout waiting for app to become ready.")
				fmt.Printf("Check status with: kuberless apps get %s\n", name)
				return nil
			case <-ticker.C:
				app, err := apiClient.GetApp(ctx, resp.ID)
				if err != nil {
					continue
				}
				fmt.Printf("\rPhase: %s", app.Phase)
				if app.Phase == "Ready" {
					fmt.Printf("\n\nApp is ready!\n")
					fmt.Printf("URL: %s\n", app.URL)
					return nil
				}
				if app.Phase == "Failed" {
					fmt.Printf("\n\nApp deployment failed.\n")
					return fmt.Errorf("deployment failed")
				}
			}
		}
	},
}

func deriveAppName(image string) string {
	// Extract name from image like "ghcr.io/org/name:tag" -> "name"
	parts := strings.Split(image, "/")
	name := parts[len(parts)-1]
	// Remove tag.
	if idx := strings.Index(name, ":"); idx > 0 {
		name = name[:idx]
	}
	// Remove @digest.
	if idx := strings.Index(name, "@"); idx > 0 {
		name = name[:idx]
	}
	return name
}

func init() {
	deployCmd.Flags().String("name", "", "App name (defaults to image name)")
	deployCmd.Flags().Int32("port", 8080, "Container port to expose")
	deployCmd.Flags().StringSlice("env", nil, "Environment variables (KEY=VALUE)")
}
