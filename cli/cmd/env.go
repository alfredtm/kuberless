package cmd

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

var envCmd = &cobra.Command{
	Use:   "env",
	Short: "Manage app environment variables",
}

var envSetCmd = &cobra.Command{
	Use:   "set <app> KEY=VALUE [KEY=VALUE...]",
	Short: "Set environment variables",
	Args:  cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		app, err := apiClient.GetAppByName(context.Background(), args[0])
		if err != nil {
			return err
		}
		env := make(map[string]string)
		for _, kv := range args[1:] {
			parts := strings.SplitN(kv, "=", 2)
			if len(parts) != 2 {
				return fmt.Errorf("invalid format: %q (expected KEY=VALUE)", kv)
			}
			env[parts[0]] = parts[1]
		}

		if err := apiClient.PatchEnv(context.Background(), app.ID, env); err != nil {
			return err
		}

		for k, v := range env {
			fmt.Printf("Set %s=%s\n", k, v)
		}
		return nil
	},
}

var envGetCmd = &cobra.Command{
	Use:   "get <app>",
	Short: "Get environment variables",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		app, err := apiClient.GetAppByName(context.Background(), args[0])
		if err != nil {
			return err
		}
		env, err := apiClient.GetEnv(context.Background(), app.ID)
		if err != nil {
			return err
		}

		if len(env) == 0 {
			fmt.Println("No environment variables set.")
			return nil
		}

		keys := make([]string, 0, len(env))
		for k := range env {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		for _, k := range keys {
			fmt.Printf("%s=%s\n", k, env[k])
		}
		return nil
	},
}

var envUnsetCmd = &cobra.Command{
	Use:   "unset <app> KEY [KEY...]",
	Short: "Unset environment variables",
	Args:  cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		app, err := apiClient.GetAppByName(context.Background(), args[0])
		if err != nil {
			return err
		}
		env := make(map[string]string)
		for _, key := range args[1:] {
			env[key] = ""
		}

		if err := apiClient.PatchEnv(context.Background(), app.ID, env); err != nil {
			return err
		}

		for _, key := range args[1:] {
			fmt.Printf("Unset %s\n", key)
		}
		return nil
	},
}

func init() {
	envCmd.AddCommand(envSetCmd)
	envCmd.AddCommand(envGetCmd)
	envCmd.AddCommand(envUnsetCmd)
}
