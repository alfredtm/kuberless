package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/alfredtm/kuberless/cli/client"
)

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Authenticate with the kuberless platform",
	RunE: func(cmd *cobra.Command, args []string) error {
		serverURL, _ := cmd.Flags().GetString("server")
		username, _ := cmd.Flags().GetString("username")
		password, _ := cmd.Flags().GetString("password")

		if username == "" {
			reader := bufio.NewReader(os.Stdin)
			fmt.Print("Username: ")
			u, _ := reader.ReadString('\n')
			username = strings.TrimSpace(u)
		}

		if password == "" {
			fmt.Print("Password: ")
			passwordBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
			if err != nil {
				return fmt.Errorf("reading password: %w", err)
			}
			fmt.Println()
			password = string(passwordBytes)
		}

		cfg := &client.Config{
			APIBaseURL: serverURL,
		}
		c := client.NewWithConfig(cfg)

		resp, err := c.AdminLogin(context.Background(), &client.AdminLoginRequest{
			Username: username,
			Password: password,
		})
		if err != nil {
			return fmt.Errorf("login failed: %w", err)
		}

		fmt.Printf("Logged in as %s (%s)\n", resp.User.DisplayName, resp.User.Email)

		// List tenants and auto-select if only one.
		apiClient = c
		tenants, err := c.ListTenants(context.Background())
		if err == nil && len(tenants) == 1 {
			if err := c.SetTenant(tenants[0].ID, tenants[0].Name); err == nil {
				fmt.Printf("Active tenant: %s\n", tenants[0].Name)
			}
		} else if err == nil && len(tenants) > 1 {
			fmt.Println("\nAvailable tenants:")
			for i, t := range tenants {
				fmt.Printf("  %d. %s (%s)\n", i+1, t.Name, t.DisplayName)
			}
			fmt.Println("\nUse 'kuberless tenant switch <name>' to select a tenant.")
		}

		return nil
	},
}

func init() {
	loginCmd.Flags().String("server", "http://localhost:8080", "API server URL")
	loginCmd.Flags().String("username", "", "Username (skips interactive prompt)")
	loginCmd.Flags().String("password", "", "Password (skips interactive prompt)")
}
