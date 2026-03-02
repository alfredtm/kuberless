package cmd

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/alfredtm/kuberless/cli/client"
)

var appsCmd = &cobra.Command{
	Use:   "apps",
	Short: "Manage apps",
}

var appsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all apps in the active tenant",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := apiClient.GetConfig()
		if cfg.TenantID == "" {
			return fmt.Errorf("no active tenant")
		}

		apps, err := apiClient.ListApps(context.Background())
		if err != nil {
			return err
		}

		if len(apps) == 0 {
			fmt.Println("No apps found. Deploy one with 'kuberless deploy <image>'")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		_, _ = fmt.Fprintln(w, "NAME\tIMAGE\tPHASE\tURL\tREADY")
		for _, a := range apps {
			_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\n", a.Name, a.Image, a.Phase, a.URL, a.ReadyInstances)
		}
		_ = w.Flush()
		return nil
	},
}

var appsGetCmd = &cobra.Command{
	Use:   "get <name>",
	Short: "Get app details",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		app, err := apiClient.GetAppByName(context.Background(), args[0])
		if err != nil {
			return err
		}

		fmt.Printf("Name:           %s\n", app.Name)
		fmt.Printf("Image:          %s\n", app.Image)
		fmt.Printf("Port:           %d\n", app.Port)
		fmt.Printf("Phase:          %s\n", app.Phase)
		fmt.Printf("URL:            %s\n", app.URL)
		fmt.Printf("Latest Rev:     %s\n", app.LatestRevision)
		fmt.Printf("Ready Replicas: %d\n", app.ReadyInstances)
		fmt.Printf("Paused:         %v\n", app.Paused)
		fmt.Printf("Created:        %s\n", app.CreatedAt)
		return nil
	},
}

var appsDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete an app",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		app, err := apiClient.GetAppByName(context.Background(), args[0])
		if err != nil {
			return err
		}
		if err := apiClient.DeleteApp(context.Background(), app.ID); err != nil {
			return err
		}
		fmt.Printf("App %s deleted.\n", args[0])
		return nil
	},
}

var appsPauseCmd = &cobra.Command{
	Use:   "pause <name>",
	Short: "Pause an app (scale to zero)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		app, err := apiClient.GetAppByName(context.Background(), args[0])
		if err != nil {
			return err
		}
		paused := true
		_, err = apiClient.UpdateApp(context.Background(), app.ID, &client.UpdateAppRequest{
			Paused: &paused,
		})
		if err != nil {
			return err
		}
		fmt.Printf("App %s paused.\n", args[0])
		return nil
	},
}

var appsResumeCmd = &cobra.Command{
	Use:   "resume <name>",
	Short: "Resume a paused app",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		app, err := apiClient.GetAppByName(context.Background(), args[0])
		if err != nil {
			return err
		}
		paused := false
		_, err = apiClient.UpdateApp(context.Background(), app.ID, &client.UpdateAppRequest{
			Paused: &paused,
		})
		if err != nil {
			return err
		}
		fmt.Printf("App %s resumed.\n", args[0])
		return nil
	},
}

func init() {
	appsCmd.AddCommand(appsListCmd)
	appsCmd.AddCommand(appsGetCmd)
	appsCmd.AddCommand(appsDeleteCmd)
	appsCmd.AddCommand(appsPauseCmd)
	appsCmd.AddCommand(appsResumeCmd)
}
