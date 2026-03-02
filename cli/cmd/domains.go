package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

var domainsCmd = &cobra.Command{
	Use:   "domains",
	Short: "Manage custom domains",
}

var domainsAddCmd = &cobra.Command{
	Use:   "add <app> <hostname>",
	Short: "Add a custom domain to an app",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		app, err := apiClient.GetAppByName(context.Background(), args[0])
		if err != nil {
			return err
		}
		resp, err := apiClient.AddDomain(context.Background(), app.ID, args[1])
		if err != nil {
			return err
		}
		fmt.Printf("Domain added: %s\n", resp.Hostname)
		return nil
	},
}

var domainsListCmd = &cobra.Command{
	Use:   "list <app>",
	Short: "List custom domains for an app",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		app, err := apiClient.GetAppByName(context.Background(), args[0])
		if err != nil {
			return err
		}
		domains, err := apiClient.ListDomains(context.Background(), app.ID)
		if err != nil {
			return err
		}

		if len(domains) == 0 {
			fmt.Println("No custom domains configured.")
			return nil
		}

		for _, d := range domains {
			fmt.Println(d.Hostname)
		}
		return nil
	},
}

var domainsRemoveCmd = &cobra.Command{
	Use:   "remove <app> <hostname>",
	Short: "Remove a custom domain from an app",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		app, err := apiClient.GetAppByName(context.Background(), args[0])
		if err != nil {
			return err
		}
		if err := apiClient.RemoveDomain(context.Background(), app.ID, args[1]); err != nil {
			return err
		}
		fmt.Printf("Domain removed: %s\n", args[1])
		return nil
	},
}

func init() {
	domainsCmd.AddCommand(domainsAddCmd)
	domainsCmd.AddCommand(domainsListCmd)
	domainsCmd.AddCommand(domainsRemoveCmd)
}
