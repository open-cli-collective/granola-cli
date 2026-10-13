package cmd

import (
	"errors"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"

	"github.com/open-cli-collective/granola-cli/internal/clierr"
	"github.com/open-cli-collective/granola-cli/internal/credentials"
	"github.com/open-cli-collective/granola-cli/internal/mcpclient"
)

func usageErrorf(format string, args ...any) error {
	return clierr.New(clierr.CodeUsage, fmt.Sprintf(format, args...))
}

// classifyError maps errors to the family's exit codes: 2 usage, 3 auth or
// config, 5 upstream (the server returned an error, a tool reported one, or
// the token endpoint failed without rejecting the session).
func classifyError(err error) error {
	if err == nil || clierr.CodeOf(err) != int(clierr.CodeGeneric) {
		return err
	}
	if errors.Is(err, credentials.ErrNotLoggedIn) || errors.Is(err, mcpclient.ErrSessionRejected) {
		return clierr.Wrap(clierr.CodeAuthConfig, err, "")
	}
	// Server-authored text is checked before the usage matcher so a tool
	// message that happens to contain a cobra phrase is not misread.
	var toolErr *mcpclient.ToolError
	var rpcErr *jsonrpc.Error
	if errors.Is(err, mcpclient.ErrTokenRefresh) || errors.As(err, &toolErr) || errors.As(err, &rpcErr) {
		return clierr.Wrap(clierr.CodeUpstream, err, "")
	}
	if clierr.LooksLikeUsage(err) {
		return clierr.Wrap(clierr.CodeUsage, err, "")
	}
	return err
}
