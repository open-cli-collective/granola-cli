# granola-cli development notes

Repo-local facts for `granola-cli`. Shared standards:
https://github.com/open-cli-collective/cli-common/tree/main/docs

## Quick commands

```bash
make build    # ./granola
make test     # go test -race ./...
make lint     # golangci-lint
make smoke    # build, then run --help on every command group
make check    # tidy, fmt, lint, test, build
git config core.hooksPath scripts/hooks   # reject AI-tool mentions in commit messages
```

The Makefile exports the standard keyring build tags
(`keyring_no1password,keyring_nopassage`), so run builds and tests through it
or set the same `GOFLAGS`.

## Layout

- `main.go` and `cmd/`: the Cobra command tree. Each tool command is a thin
  wrapper that builds the tool's arguments and calls `runTool`. A command
  whose result carries binary content (`meetings attachments`) calls
  `withSession` itself and writes the files in `cmd/`; it still renders text
  blocks with `mcpclient.RenderText` and reports error results with
  `mcpclient.CheckError` or `mcpclient.ToolError`.
- `internal/mcpclient/`: the MCP connection, the stored-session OAuth handler
  (silent refresh, no browser), request pacing, and result rendering.
- `internal/oauthlogin/`: the one-time browser sign-in (discovery, Dynamic
  Client Registration, PKCE, loopback redirect).
- `internal/credentials/`: the credential bundle `granola-cli/default`.
- `internal/clierr/`: typed errors that carry the exit code.
- `testdata/`: synthetic MCP wire fixtures.

The command tree lives in a flat `cmd/` package with `main.go` at the root,
rather than the `cmd/<binary>/` and `internal/cmd/<group>/` layout in
cli-common `repo-layout.md` §1. The tool has a handful of thin commands that
share one connection helper, so the split would add packages without
separating anything.

## Rules for changes

- The data source is Granola's hosted MCP server only. Do not add readers for
  the Granola desktop app's local files, its database, or its stored
  credentials, and do not call Granola's undocumented app API.
- Tool output is LLM-oriented text; print it verbatim. Parsing it into
  tables would break whenever Granola changes the layout.
- Tool names, argument names, and plan gating come from the server's
  `tools/list`. Check `granola tools get <name>` before changing a command's
  arguments.
- Meeting notes are sensitive. Fixtures, tests, PRs, and issues use invented
  content only.

## Data source

Granola exposes meeting data to external clients two ways: a REST API
(`public-api.granola.ai`) keyed by personal API keys that only Business and
Enterprise plans can create, and a hosted MCP server
(`https://mcp.granola.ai/mcp`) that any plan can reach through browser OAuth.
This CLI uses the MCP server so it works on every plan; what a given account
can read is decided by the server per plan.

## Auth

`granola login` (`internal/oauthlogin`) drives the MCP authorization flow
through the official Go SDK's `auth.AuthorizationCodeHandler`:

1. An unauthenticated `initialize` draws a 401 whose `WWW-Authenticate`
   header names the protected-resource metadata URL.
2. The protected-resource metadata names the authorization server; its
   metadata supplies the authorize, token, and registration endpoints. No
   endpoint is hardcoded.
3. The CLI registers a public client (`token_endpoint_auth_method: none`)
   with a single redirect URI on a random `127.0.0.1` port, requesting the
   `mcp` scope plus `offline_access` so a refresh token is issued.
4. The browser completes the authorization code flow with PKCE (S256); the
   loopback listener receives the code, and the SDK exchanges it.

The session is written only after the retried `initialize` succeeds with the
new token. Each login registers a fresh client because the redirect port
differs per run.

Normal commands use `mcpclient.StoredHandler`: an `oauth2` token source that
refreshes an expired access token with the stored refresh token and writes
the new token back to the store. Its `Authorize` never opens a browser; a
401 or 403 becomes an auth error (exit 3) carrying the server's response and
a pointer to `granola login`.

## Credential key set

Service `granola-cli`, profile `default` (ref `granola-cli/default`), stored
through cli-common `credstore`:

| Key | Required | Contents |
|---|---|---|
| `oauth_client` | yes | JSON: registered `client_id`, `client_secret` (empty for the public client), authorize and token URLs, token auth style, redirect URL |
| `oauth_token` | yes | JSON: access token, refresh token, token type, expiry |

`login` writes both keys together; token refresh rewrites only
`oauth_token`; `config clear` removes both. There is no `set-credential`
command: the session comes from the OAuth flow, and there is no
out-of-band secret to pre-stage. The tool has no config file or cache, so
`config clear` returns it to its pre-login state.

Darwin builds need cgo because the keychain backend links
Security.framework. `granola config show --json` reports `backend` and
`backend_source`; the release workflow's darwin gate runs it through the
`keychain_probe` in `packaging/identity.yml` and fails the release unless the
arm64 binary auto-selects the keychain.

## Live verification

```bash
granola login                        # one browser sign-in
granola config show                  # both keys present, refresh token present
granola account get                  # the connected account and workspace
granola tools list                   # the tools this plan exposes
granola meetings list --time-range this_week
granola -v account get               # request log, credentials redacted
```

On the free plan, `folders list` and `meetings transcript` exit 5 with the
server's own refusal text; that is the expected result, not a defect.

## Request pacing

`internal/mcpclient/transport.go` wraps every request, including OAuth
discovery and token calls, in a limiter (one request per second after a
burst of four) and a per-invocation cap of 30 requests. The SDK transport is
configured with reconnect retries disabled and no standalone SSE stream, so
a command makes exactly the requests it needs: `initialize`, the
`initialized` notification, and the tool call. The client requests MCP
protocol version `2025-11-25` directly; the SDK's default first probes a
newer `server/discover` RPC, which Granola's server answers with HTTP 400.

`-v` logs each request line to stderr with the query string and the
`Authorization` value redacted to its scheme and length.

## Testing

- `internal/credentials`: memory-backend tests for the exact key set,
  round trips, token-only refresh writes, clear, and malformed values.
- `internal/mcpclient`: rendering of mixed content blocks, `isError`
  handling, pacing, the request cap, and log redaction.
- `cmd`: end-to-end tests against an in-process fake of the MCP server and
  its OAuth server (built from the SDK's server and the `testdata/`
  fixtures): argument shaping per command, verbatim output, `--json`
  passthrough, plan-limit errors, exit codes, silent refresh and
  persistence, a full discovery + registration + PKCE login, and a no-leak
  test that greps every output channel for the stored tokens.

Tests never touch the real OS keychain or the real server. `make test` runs
everything with `-race`.

## Release

Releases follow cli-common `release.md` and `distribution.md`: a merged
`feat:` or `fix:` PR that touches Go code, `version.txt`, `.goreleaser.yml`,
or `packaging/` makes `auto-release.yml` tag `v<version.txt>.<run number>`,
and `release.yml` builds with GoReleaser, signs the darwin binaries, runs the
keychain gate, and publishes the Homebrew cask and Linux packages declared in
`packaging/identity.yml`.

## Update trigger

Revise this doc and the README when Granola adds, removes, or renames an MCP
tool or argument (compare `granola tools list --json` with
`testdata/tools_list.json`), or changes its authorization metadata.
