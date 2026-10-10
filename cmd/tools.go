package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"

	"github.com/open-cli-collective/granola-cli/internal/mcpclient"
)

const groupTools = "tools"

var (
	toolsListJSON bool
	toolsGetJSON  bool
	toolsCallJSON bool
	toolsCallArgs string
)

var toolsCmd = &cobra.Command{
	Use:   groupTools,
	Short: "Inspect and call the MCP tools Granola's server exposes",
	Long: `Inspect and call the MCP tools Granola's server exposes to this account.

The tool set depends on your Granola plan; "tools list" shows what the server
offers right now. The first-class commands (meetings, folders, account) each
wrap one of these tools; "tools call" reaches any tool directly.`,
}

var toolsListCmd = &cobra.Command{
	Use:     verbList,
	Aliases: []string{"ls"},
	Short:   "List the server's tools",
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return withSession(cmd, func(ctx context.Context, cs *mcp.ClientSession) error {
			tools, err := listTools(ctx, cs)
			if err != nil {
				return err
			}
			if toolsListJSON {
				return writeJSON(cmd.OutOrStdout(), tools)
			}
			out := cmd.OutOrStdout()
			fmt.Fprintln(out, "NAME | DESCRIPTION")
			for _, t := range tools {
				fmt.Fprintf(out, "%s | %s\n", t.Name, cell(firstLine(t.Description)))
			}
			return nil
		})
	},
}

var toolsGetCmd = &cobra.Command{
	Use:   "get <tool-name>",
	Short: "Show one tool's full description and input schema",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return withSession(cmd, func(ctx context.Context, cs *mcp.ClientSession) error {
			tools, err := listTools(ctx, cs)
			if err != nil {
				return err
			}
			for _, t := range tools {
				if t.Name != args[0] {
					continue
				}
				if toolsGetJSON {
					return writeJSON(cmd.OutOrStdout(), t)
				}
				schema, err := json.MarshalIndent(t.InputSchema, "", "  ")
				if err != nil {
					return err
				}
				out := cmd.OutOrStdout()
				fmt.Fprintf(out, "%s\n\nDescription:\n%s\n\nInput schema:\n%s\n", t.Name, t.Description, schema)
				return nil
			}
			return errNoSuchTool(args[0])
		})
	},
}

var toolsCallCmd = &cobra.Command{
	Use:   "call <tool-name>",
	Short: "Call any tool with JSON arguments and print its result",
	Long: `Call a tool by name with a JSON object of arguments and print its result.

Use "granola tools get <tool-name>" to see the arguments a tool accepts.
Only tools the server annotates as read-only (readOnlyHint) can be called;
anything else is refused before it is sent, so this CLI stays read-only if
Granola adds tools that change data.

Examples:
  granola tools call get_account_info
  granola tools call list_meetings --args '{"time_range":"last_30_days"}' --json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		toolArgs := map[string]any{}
		if strings.TrimSpace(toolsCallArgs) != "" {
			if err := json.Unmarshal([]byte(toolsCallArgs), &toolArgs); err != nil {
				return usageErrorf("--args must be a JSON object: %v", err)
			}
		}
		return withSession(cmd, func(ctx context.Context, cs *mcp.ClientSession) error {
			tools, err := listTools(ctx, cs)
			if err != nil {
				return err
			}
			if err := requireReadOnly(tools, args[0]); err != nil {
				return err
			}
			return callAndPrint(ctx, cmd, cs, args[0], toolArgs, toolsCallJSON)
		})
	},
}

// requireReadOnly refuses a tool the server does not list or does not
// annotate as read-only.
func requireReadOnly(tools []*mcp.Tool, name string) error {
	for _, t := range tools {
		if t.Name != name {
			continue
		}
		if t.Annotations == nil || !t.Annotations.ReadOnlyHint {
			return usageErrorf("refusing to call %s: the server does not mark it read-only, and granola only reads", name)
		}
		return nil
	}
	return errNoSuchTool(name)
}

func errNoSuchTool(name string) error {
	return usageErrorf("the server has no tool named %q; run `granola tools list`", name)
}

func listTools(ctx context.Context, cs *mcp.ClientSession) ([]*mcp.Tool, error) {
	var tools []*mcp.Tool
	for t, err := range cs.Tools(ctx, nil) {
		if err != nil {
			return nil, fmt.Errorf("list tools: %w", err)
		}
		tools = append(tools, t)
	}
	return tools, nil
}

// runTool calls one tool and prints the result: the tool's text verbatim by
// default, or the raw MCP CallToolResult with asJSON. A result the server
// marks isError is printed in full (JSON mode) and returned as an error.
func runTool(cmd *cobra.Command, name string, args map[string]any, asJSON bool) error {
	return withSession(cmd, func(ctx context.Context, cs *mcp.ClientSession) error {
		return callAndPrint(ctx, cmd, cs, name, args, asJSON)
	})
}

func callAndPrint(ctx context.Context, cmd *cobra.Command, cs *mcp.ClientSession, name string, args map[string]any, asJSON bool) error {
	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		return fmt.Errorf("call %s: %w", name, err)
	}
	if asJSON {
		if err := writeJSON(cmd.OutOrStdout(), res); err != nil {
			return err
		}
		return mcpclient.CheckError(name, res)
	}
	if err := mcpclient.CheckError(name, res); err != nil {
		return err
	}
	text := mcpclient.RenderText(res)
	if text == "" {
		fmt.Fprintf(cmd.ErrOrStderr(), "%s returned no content\n", name)
		return nil
	}
	fmt.Fprintln(cmd.OutOrStdout(), strings.TrimRight(text, "\n"))
	return nil
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		return s[:i]
	}
	return s
}

// cell makes a value safe for one pipe-delimited row.
func cell(s string) string {
	if s == "" {
		return "-"
	}
	s = strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ", " | ", " ").Replace(s)
	return s
}

func serverURL() string {
	if endpointOverride != "" {
		return endpointOverride
	}
	return mcpclient.Endpoint
}

func init() {
	toolsListCmd.Flags().BoolVar(&toolsListJSON, "json", false, "Output the raw MCP tool definitions as JSON")
	toolsGetCmd.Flags().BoolVar(&toolsGetJSON, "json", false, "Output the raw MCP tool definition as JSON")
	toolsCallCmd.Flags().StringVar(&toolsCallArgs, "args", "", "Tool arguments as a JSON object")
	toolsCallCmd.Flags().BoolVar(&toolsCallJSON, "json", false, "Output the raw MCP tool result as JSON")
	toolsCmd.AddCommand(toolsListCmd, toolsGetCmd, toolsCallCmd)
	rootCmd.AddCommand(toolsCmd)
}
