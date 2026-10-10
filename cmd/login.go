package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/pkg/browser"
	"github.com/spf13/cobra"

	"github.com/open-cli-collective/granola-cli/internal/credentials"
	"github.com/open-cli-collective/granola-cli/internal/oauthlogin"
)

var (
	loginNoBrowser bool
	loginWait      time.Duration

	// openBrowser and runLogin are replaced in tests.
	openBrowser = browser.OpenURL
	runLogin    = oauthlogin.Login
)

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Sign in to Granola in the browser and store the session",
	Long: `Sign in to Granola's MCP server with OAuth.

login discovers Granola's authorization server from the MCP server's
metadata, registers this CLI as an OAuth client (Dynamic Client
Registration), and opens your browser to Granola's sign-in page. The
redirect returns to a temporary listener on 127.0.0.1. The resulting client
registration and access/refresh tokens are stored in the OS credential store
under granola-cli/default, replacing any earlier session.

Run it again whenever a command reports that the session was rejected.

Examples:
  granola login
  granola login --no-browser   # print the URL instead of opening it`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		ctx, cancel := context.WithTimeout(cmd.Context(), loginWait+time.Minute)
		defer cancel()
		opener := openBrowser
		if loginNoBrowser {
			opener = nil
		}
		sess, err := runLogin(ctx, oauthlogin.Options{
			Endpoint:    endpointOverride,
			HTTPClient:  httpClientFactory(verboseLog(cmd)),
			OpenBrowser: opener,
			Out:         cmd.ErrOrStderr(),
			Wait:        loginWait,
		})
		if err != nil {
			return err
		}
		if err := creds.SaveSession(*sess); err != nil {
			return err
		}
		fmt.Fprintf(cmd.ErrOrStderr(), "Signed in. Session stored at %s.\n", credentials.Ref)
		return nil
	},
}

func init() {
	loginCmd.Flags().BoolVar(&loginNoBrowser, "no-browser", false, "Print the sign-in URL without opening a browser")
	loginCmd.Flags().DurationVar(&loginWait, "wait", 5*time.Minute, "How long to wait for the browser sign-in to complete")
	rootCmd.AddCommand(loginCmd)
}
