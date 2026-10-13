package cmd

import "github.com/spf13/cobra"

var foldersListJSON bool

var foldersCmd = &cobra.Command{
	Use:     "folders",
	Aliases: []string{"folder"},
	Short:   "List Granola meeting folders",
}

var foldersListCmd = &cobra.Command{
	Use:     verbList,
	Aliases: []string{"ls"},
	Short:   "List folders with IDs and note counts (list_meeting_folders)",
	Long: `List the meeting folders you can access, with folder ID, title,
description, and note count. Wraps the list_meeting_folders MCP tool and
prints its text output. Pass a folder ID to "granola meetings list --folder".

Granola offers folders through MCP on paid plans only; on the free plan the
server's refusal is printed as it sent it and the command exits 5.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return runTool(cmd, toolListFolders, map[string]any{}, foldersListJSON)
	},
}

func init() {
	foldersListCmd.Flags().BoolVar(&foldersListJSON, "json", false, "Output the raw MCP tool result as JSON")
	foldersCmd.AddCommand(foldersListCmd)
	rootCmd.AddCommand(foldersCmd)
}
