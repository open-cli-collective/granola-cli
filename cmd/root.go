// Package cmd defines the granola command tree.
package cmd

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"

	"github.com/open-cli-collective/granola-cli/internal/clierr"
	"github.com/open-cli-collective/granola-cli/internal/credentials"
	"github.com/open-cli-collective/granola-cli/internal/mcpclient"
	"github.com/open-cli-collective/granola-cli/internal/version"
)

var (
	verbose        bool
	commandTimeout time.Duration

	// endpointOverride points the client at a test server. It is not a flag.
	endpointOverride string
	// httpClientFactory builds the paced HTTP client; tests replace it.
	httpClientFactory = func(log io.Writer) *http.Client { return mcpclient.NewHTTPClient(log) }
	// creds is the session store; tests replace it with a memory store.
	creds = credentials.Default()
)

var rootCmd = &cobra.Command{
	Use:   "granola",
	Short: "Read-only CLI for Granola meeting notes through Granola's hosted MCP server",
	Long: `granola reads your Granola meeting notes, transcripts, and folders through
Granola's official hosted MCP server (https://mcp.granola.ai/mcp).

Each command calls one of the server's MCP tools and prints the tool's text
output as Granola wrote it. The server's output is written for LLM callers,
so it is passed through rather than reformatted. --json prints the raw MCP
tool result instead.

Auth: run "granola login" once. It opens your browser to sign in to Granola
(OAuth with PKCE and a loopback redirect on 127.0.0.1) and stores the session
in the OS credential store under granola-cli/default. Later commands refresh
the access token silently.

Plan limits come from Granola: on the free Basic plan the server returns only
the last 30 days of notes, and folder and transcript tools are paid-plan
only. Limit and access errors are printed as the server sent them.

Get started:
  granola login
  granola account get
  granola tools list
  granola meetings list`,
}

// Execute runs the root command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(clierr.CodeOf(classifyError(err)))
	}
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, _ []string) {
		fmt.Fprintf(cmd.OutOrStdout(), "granola-cli %s (commit: %s, built: %s)\n", version.Version, version.Commit, version.Date)
	},
}

func verboseLog(cmd *cobra.Command) io.Writer {
	if verbose {
		return cmd.ErrOrStderr()
	}
	return nil
}

// withSession loads the stored session, connects, and runs fn.
func withSession(cmd *cobra.Command, fn func(ctx context.Context, cs *mcp.ClientSession) error) error {
	sess, err := creds.Load()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(cmd.Context(), commandTimeout)
	defer cancel()
	httpClient := httpClientFactory(verboseLog(cmd))
	cs, err := mcpclient.Connect(ctx, mcpclient.Options{
		Endpoint:     endpointOverride,
		HTTPClient:   httpClient,
		OAuthHandler: mcpclient.NewStoredHandler(ctx, sess, httpClient, creds.SaveToken),
	})
	if err != nil {
		return fmt.Errorf("connect to Granola MCP server: %w", err)
	}
	defer cs.Close()
	return fn(ctx, cs)
}

func init() {
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Log each HTTP request to stderr (credentials and query strings redacted)")
	rootCmd.PersistentFlags().DurationVar(&commandTimeout, "timeout", 3*time.Minute, "Maximum time for the command, including the server's response")
	rootCmd.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return clierr.Wrap(clierr.CodeUsage, err, "")
	})
	rootCmd.PersistentPreRun = func(cmd *cobra.Command, _ []string) {
		cmd.SilenceUsage = true
	}
	rootCmd.AddCommand(versionCmd)
}
