package mcpclient

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ToolError is a tool result the server marked isError. Its message is the
// server's own text, unmodified, so plan limits and access errors reach the
// user as Granola wrote them.
type ToolError struct {
	Tool string
	Text string
}

func (e *ToolError) Error() string {
	if e.Text == "" {
		return fmt.Sprintf("granola tool %s returned an error with no text", e.Tool)
	}
	return fmt.Sprintf("granola tool %s: %s", e.Tool, e.Text)
}

// RenderText returns the result's content as the server sent it: text blocks
// verbatim, joined by blank lines. Non-text blocks are named by type so they
// are not silently dropped; --json carries them in full. When a result has
// no content but does have structured content, that is printed as indented
// JSON.
func RenderText(res *mcp.CallToolResult) string {
	var parts []string
	for _, c := range res.Content {
		switch v := c.(type) {
		case *mcp.TextContent:
			parts = append(parts, v.Text)
		case *mcp.ImageContent:
			parts = append(parts, fmt.Sprintf("[image content, %s; use --json for the data]", v.MIMEType))
		case *mcp.AudioContent:
			parts = append(parts, fmt.Sprintf("[audio content, %s; use --json for the data]", v.MIMEType))
		case *mcp.ResourceLink:
			parts = append(parts, fmt.Sprintf("[resource link: %s]", v.URI))
		case *mcp.EmbeddedResource:
			if v.Resource != nil && v.Resource.Text != "" {
				parts = append(parts, v.Resource.Text)
			} else if v.Resource != nil {
				parts = append(parts, fmt.Sprintf("[embedded resource: %s; use --json for the data]", v.Resource.URI))
			}
		default:
			parts = append(parts, "[unrecognized content block; use --json]")
		}
	}
	if len(parts) == 0 && res.StructuredContent != nil {
		if b, err := json.MarshalIndent(res.StructuredContent, "", "  "); err == nil {
			parts = append(parts, string(b))
		}
	}
	return strings.Join(parts, "\n\n")
}

// CheckError converts an isError result into a ToolError.
func CheckError(tool string, res *mcp.CallToolResult) error {
	if res == nil || !res.IsError {
		return nil
	}
	return &ToolError{Tool: tool, Text: RenderText(res)}
}
