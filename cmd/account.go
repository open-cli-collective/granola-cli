package cmd

import "github.com/spf13/cobra"

var accountGetJSON bool

var accountCmd = &cobra.Command{
	Use:   "account",
	Short: "Show the Granola account this session is signed in as",
}

var accountGetCmd = &cobra.Command{
	Use:   verbGet,
	Short: "Show the account email, active workspace, and note-access scopes (get_account_info)",
	Long: `Show the email, active workspace, and note-access scopes of the Granola
account the stored session belongs to. Wraps the get_account_info MCP tool.

Granola's MCP server follows the workspace selected in the Granola app; if
results look empty, check the workspace here first.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return runTool(cmd, toolGetAccountInfo, map[string]any{}, accountGetJSON)
	},
}

func init() {
	accountGetCmd.Flags().BoolVar(&accountGetJSON, "json", false, "Output the raw MCP tool result as JSON")
	accountCmd.AddCommand(accountGetCmd)
	rootCmd.AddCommand(accountCmd)
}
