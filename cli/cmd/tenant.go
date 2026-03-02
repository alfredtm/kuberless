package cmd

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/alfredtm/kuberless/cli/client"
)

var tenantCmd = &cobra.Command{
	Use:   "tenant",
	Short: "Manage tenants",
}

var tenantCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a new tenant",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		plan, _ := cmd.Flags().GetString("plan")
		displayName, _ := cmd.Flags().GetString("display-name")
		if displayName == "" {
			displayName = name
		}

		resp, err := apiClient.CreateTenant(context.Background(), &client.CreateTenantRequest{
			Name:        name,
			DisplayName: displayName,
			Plan:        plan,
		})
		if err != nil {
			return err
		}

		fmt.Printf("Tenant created: %s (ID: %s)\n", resp.Name, resp.ID)

		// Auto-switch to the new tenant.
		if err := apiClient.SetTenant(resp.ID, resp.Name); err == nil {
			fmt.Printf("Active tenant set to: %s\n", resp.Name)
		}

		return nil
	},
}

var tenantListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all tenants",
	RunE: func(cmd *cobra.Command, args []string) error {
		tenants, err := apiClient.ListTenants(context.Background())
		if err != nil {
			return err
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		_, _ = fmt.Fprintln(w, "NAME\tDISPLAY NAME\tPLAN\tID")
		for _, t := range tenants {
			marker := ""
			if t.ID == apiClient.GetConfig().TenantID {
				marker = " *"
			}
			_, _ = fmt.Fprintf(w, "%s%s\t%s\t%s\t%s\n", t.Name, marker, t.DisplayName, t.Plan, t.ID)
		}
		_ = w.Flush()
		return nil
	},
}

var tenantSwitchCmd = &cobra.Command{
	Use:   "switch <name>",
	Short: "Switch active tenant",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		tenants, err := apiClient.ListTenants(context.Background())
		if err != nil {
			return err
		}

		for _, t := range tenants {
			if t.Name == name || t.ID == name {
				if err := apiClient.SetTenant(t.ID, t.Name); err != nil {
					return err
				}
				fmt.Printf("Switched to tenant: %s\n", t.Name)
				return nil
			}
		}

		return fmt.Errorf("tenant %q not found", name)
	},
}

var tenantInfoCmd = &cobra.Command{
	Use:   "info",
	Short: "Show active tenant info",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := apiClient.GetConfig()
		if cfg.TenantID == "" {
			return fmt.Errorf("no active tenant. Use 'kuberless tenant switch <name>'")
		}

		resp, err := apiClient.GetTenant(context.Background(), cfg.TenantID)
		if err != nil {
			return err
		}

		fmt.Printf("Name:         %s\n", resp.Name)
		fmt.Printf("Display Name: %s\n", resp.DisplayName)
		fmt.Printf("Plan:         %s\n", resp.Plan)
		fmt.Printf("ID:           %s\n", resp.ID)
		fmt.Printf("Created:      %s\n", resp.CreatedAt)
		return nil
	},
}

var tenantDeleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "Delete a tenant",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := apiClient.DeleteTenant(context.Background(), args[0]); err != nil {
			return err
		}
		fmt.Println("Tenant deleted.")
		return nil
	},
}

func init() {
	tenantCreateCmd.Flags().String("plan", "free", "Billing plan (free, starter, pro, enterprise)")
	tenantCreateCmd.Flags().String("display-name", "", "Display name for the tenant")

	tenantCmd.AddCommand(tenantCreateCmd)
	tenantCmd.AddCommand(tenantListCmd)
	tenantCmd.AddCommand(tenantSwitchCmd)
	tenantCmd.AddCommand(tenantInfoCmd)
	tenantCmd.AddCommand(tenantDeleteCmd)
}
