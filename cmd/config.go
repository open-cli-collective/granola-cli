package cmd

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/open-cli-collective/granola-cli/internal/credentials"
)

var configShowJSON, configClearJSON bool

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Inspect or clear the stored Granola session",
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show where the session is stored and which keys are present (never values)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		st, err := creds.Inspect()
		if err != nil {
			return err
		}
		out := cmd.OutOrStdout()
		if configShowJSON {
			enc := json.NewEncoder(out)
			enc.SetIndent("", "  ")
			return enc.Encode(st)
		}
		fmt.Fprintf(out, "Credential ref: %s\n", st.Ref)
		fmt.Fprintf(out, "Backend: %s\n", st.Backend)
		for _, k := range credentials.Keys {
			fmt.Fprintf(out, "%s: %s\n", k, presence(st.Keys[k]))
		}
		expiry := "-"
		if st.TokenExpiry != nil {
			expiry = st.TokenExpiry.UTC().Format(time.RFC3339)
		}
		fmt.Fprintf(out, "Access token expiry: %s\n", expiry)
		fmt.Fprintf(out, "Refresh token: %s\n", presence(st.HasRefreshToken))
		fmt.Fprintf(out, "MCP server: %s\n", serverURL())
		return nil
	},
}

var configClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Remove the stored Granola session from the OS credential store",
	Long: `Remove every key stored under granola-cli/default. The tool keeps no config
file or cache, so this returns it to its pre-login state. Run "granola login"
to sign in again.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		removed, err := creds.Clear()
		if err != nil {
			return err
		}
		if configClearJSON {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{
				"ref":     credentials.Ref,
				"removed": removed,
			})
		}
		if len(removed) == 0 {
			fmt.Fprintf(cmd.ErrOrStderr(), "nothing stored under %s\n", credentials.Ref)
			return nil
		}
		for _, k := range removed {
			fmt.Fprintf(cmd.ErrOrStderr(), "removed %s/%s\n", credentials.Ref, k)
		}
		return nil
	},
}

func presence(ok bool) string {
	if ok {
		return "present"
	}
	return "missing"
}

func init() {
	configShowCmd.Flags().BoolVar(&configShowJSON, "json", false, "Output as JSON")
	configClearCmd.Flags().BoolVar(&configClearJSON, "json", false, "Output as JSON")
	configCmd.AddCommand(configShowCmd, configClearCmd)
	rootCmd.AddCommand(configCmd)
}
