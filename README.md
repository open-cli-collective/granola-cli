# granola-cli

Read-only command-line interface for Granola meeting notes, backed by
Granola's official hosted MCP server (`https://mcp.granola.ai/mcp`).

Ships as the **`granola`** binary. The repository, Homebrew cask, and module
path use the `granola-cli` name.

Each command calls one of the server's MCP tools and prints the tool's text
output as Granola wrote it. That output is written for LLM callers (tagged
blocks of meetings, participants, and summaries), so the CLI passes it through
rather than reformatting it. `--json` prints the raw MCP `CallToolResult`.

The CLI talks only to Granola's documented MCP server and its OAuth
authorization server. It never reads the Granola desktop app's local files,
database, or stored credentials.

## Install

macOS (Homebrew):

```bash
brew install --cask open-cli-collective/tap/granola-cli
```

The cask is `granola-cli` because Homebrew's own `granola` cask is the Granola
desktop app.

Linux: add the Open CLI Collective package repository
([instructions](https://github.com/open-cli-collective/linux-packages)), then
install the `granola` package with apt, dnf, or pacman.

Other platforms: download an archive from the
[releases page](https://github.com/open-cli-collective/granola-cli/releases).

From source (Go 1.26+): `make install` builds and copies `granola` to
`~/.local/bin`.

## Sign in

```bash
granola login
```

`login` opens your browser to Granola's sign-in page once. Behind it: the CLI
reads the MCP server's protected-resource metadata, discovers Granola's
authorization server, registers itself as an OAuth client (Dynamic Client
Registration), and runs the authorization code flow with PKCE against a
temporary listener on `127.0.0.1`. The registered client and the
access/refresh tokens are stored in the OS credential store under
`granola-cli/default`. Later commands refresh the access token without a
browser; when a command reports that the session was rejected, run `login`
again. Use `--no-browser` to print the sign-in URL instead of opening it.

```bash
granola config show   # where the session lives, which keys are present, token expiry
granola config clear  # remove the stored session
```

## Commands

| Command | MCP tool | Notes |
|---|---|---|
| `granola meetings list` | `list_meetings` | `--time-range this_week\|last_week\|last_30_days` (default `last_30_days`), `--mine`, `--captured-by-me[=false]`, `--participant[=false]`, `--workspace-only`, `--folder <id>` |
| `granola meetings get <id>...` | `get_meetings` | Summary, private notes, attendees; up to 10 IDs |
| `granola meetings transcript <id>` | `get_meeting_transcript` | Verbatim transcript |
| `granola meetings query "<question>"` | `query_granola_meetings` | Natural-language question answered by Granola, with citation links; `--meeting <id>` narrows it |
| `granola folders list` | `list_meeting_folders` | Folder IDs for `meetings list --folder` |
| `granola account get` | `get_account_info` | Signed-in email, active workspace, note-access scopes |
| `granola tools list` / `tools get <name>` | `tools/list` | What the server offers this account, with input schemas |
| `granola tools call <name> --args '<json>'` | any read-only tool | Direct call for tools or arguments without a first-class command; tools the server does not annotate `readOnlyHint` are refused before sending |

Every tool command accepts `--json`. Meeting IDs are the UUIDs that
`meetings list` prints.

Search: the server offers no keyword or date-range search tool. `meetings
query` is Granola's own natural-language search and answer; `meetings list`
filters only by the named time ranges and involvement flags above.

## Plan limits

Granola decides what each plan can reach through MCP:

- Basic (free): personal notes from the last 30 days. The server refuses
  `folders list` and `meetings transcript`, and ignores `meetings list
  --folder` (the full list comes back).
- Business and Enterprise: personal and workspace notes, folders, and
  transcripts, subject to workspace admin settings.

A refusal is a tool result the server marks as an error. The CLI prints the
server's text unchanged on stderr and exits 5. The server follows the
workspace selected in the Granola app; `granola account get` shows which one.

## Exit codes

| Code | Meaning |
|---|---|
| 0 | Success |
| 2 | Usage error (bad flag, bad argument, unknown tool) |
| 3 | Not signed in, or the server rejected the session or its refresh token; run `granola login` |
| 5 | The server returned an error, including plan-limit refusals, or could not be reached (also during a token refresh) |

## Rate limits

Granola documents an MCP rate limit averaging about 100 requests per minute.
One command makes three or four HTTP requests (initialize, the initialized
notification, the tool call); the client paces requests at one per second
after a burst of four, caps a single invocation at 30 requests, and never
retries a failed call.
