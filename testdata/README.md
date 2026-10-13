# testdata

Synthetic MCP wire payloads for Granola's hosted MCP server
(`https://mcp.granola.ai/mcp`). Each `*.json` file is one JSON-RPC `result`
object exactly as the server sends it: `tools_list.json` is a `tools/list`
result, the rest are `tools/call` results (`CallToolResult`).

Shapes follow live responses (the tool definitions are Granola's, the text
layout of each tool result matches what the server returns); every meeting,
person, and ID is invented. Never replace these with real note content.

The `call_get_meeting_attachment*.json` error text matches a live response.
The layout of the success and partial-failure results (each image followed
by a metadata text block, a failed attachment as an error text block) follows
the tool's description; their metadata fields are invented. The command does
not parse the metadata, so only the block order matters to it.
