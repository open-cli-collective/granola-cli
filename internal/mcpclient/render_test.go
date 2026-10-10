package mcpclient

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func loadResult(t *testing.T, name string) *mcp.CallToolResult {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	var res mcp.CallToolResult
	if err := json.Unmarshal(b, &res); err != nil {
		t.Fatal(err)
	}
	return &res
}

func TestRenderTextKeepsTextVerbatimAndNamesOtherBlocks(t *testing.T) {
	got := RenderText(loadResult(t, "call_mixed_content.json"))
	want := "First block\n\n[image content, image/png; use --json for the data]\n\n" +
		"[resource link: https://notes.granola.ai/d/0b1f6c2e-4d3a-4f5b-9c8d-7e6f5a4b3c2d]\n\nLast block"
	if got != want {
		t.Fatalf("got %q\nwant %q", got, want)
	}
}

func TestRenderTextFallsBackToStructuredContent(t *testing.T) {
	got := RenderText(loadResult(t, "call_structured_only.json"))
	if got != "{\n  \"count\": 2\n}" {
		t.Fatalf("got %q", got)
	}
}

func TestCheckError(t *testing.T) {
	if err := CheckError("list_meetings", loadResult(t, "call_list_meetings.json")); err != nil {
		t.Fatalf("a normal result is not an error: %v", err)
	}
	err := CheckError("list_meeting_folders", loadResult(t, "call_list_meeting_folders_paid_only.json"))
	var toolErr *ToolError
	if !errors.As(err, &toolErr) {
		t.Fatalf("want ToolError, got %v", err)
	}
	if toolErr.Text != "Meeting folders are only available to paid Granola tiers" {
		t.Fatalf("server text altered: %q", toolErr.Text)
	}
}
