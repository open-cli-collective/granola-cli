package cmd

import (
	"context"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"

	"github.com/open-cli-collective/granola-cli/internal/mcpclient"
)

const maxAttachmentsPerGet = 5

var (
	attachmentsOutDir string
	attachmentsJSON   bool
)

var meetingsAttachmentsCmd = &cobra.Command{
	Use:   "attachments <meeting-id> <attachment-id>...",
	Short: "Save a meeting's screenshots and other image attachments (get_meeting_attachment)",
	Long: fmt.Sprintf(`Download image attachments of a meeting, such as the snapshots Granola takes
of shared screens, and save each one as a file. Wraps the
get_meeting_attachment MCP tool.

Attachment IDs are listed in the <attachments> section of
"granola meetings get <meeting-id>". The server returns up to %d attachments
per call, in request order, each followed by its metadata. Each image is
saved as <meeting-id>-<n>.<ext> in --out-dir (the current directory by
default) and reported with a "Saved" line; the server's text is printed as
it sent it. When some attachments fail, the images that succeeded are still
saved, the server's errors are printed, and the command exits 5.

Examples:
  granola meetings attachments <meeting-id> <attachment-id>
  granola meetings attachments <meeting-id> <attachment-id> <attachment-id> --out-dir ./slides`, maxAttachmentsPerGet),
	Args: cobra.RangeArgs(2, 1+maxAttachmentsPerGet),
	RunE: func(cmd *cobra.Command, args []string) error {
		toolArgs := map[string]any{"meeting_id": args[0], "attachment_ids": args[1:]}
		if attachmentsJSON {
			return runTool(cmd, toolGetAttachment, toolArgs, true)
		}
		return withSession(cmd, func(ctx context.Context, cs *mcp.ClientSession) error {
			res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: toolGetAttachment, Arguments: toolArgs})
			if err != nil {
				return fmt.Errorf("call %s: %w", toolGetAttachment, err)
			}
			return saveAttachments(cmd.OutOrStdout(), cmd.ErrOrStderr(), res, attachmentsOutDir, args[0])
		})
	},
}

// saveAttachments writes each image block of res to dir and prints the other
// blocks verbatim, in the order the server sent them, so each "Saved" line
// sits next to the metadata that follows its image.
func saveAttachments(out, errOut io.Writer, res *mcp.CallToolResult, dir, meetingID string) error {
	if len(res.Content) == 0 && !res.IsError {
		fmt.Fprintf(errOut, "%s returned no content\n", toolGetAttachment)
		return nil
	}
	images := 0
	for _, c := range res.Content {
		if _, ok := c.(*mcp.ImageContent); ok {
			images++
		}
	}
	// With no image to keep, an error result is reported like any other tool
	// error: nothing on stdout, the server's text on stderr.
	if images == 0 {
		if err := mcpclient.CheckError(toolGetAttachment, res); err != nil {
			return err
		}
	} else if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}

	n := 0
	for _, c := range res.Content {
		img, ok := c.(*mcp.ImageContent)
		if !ok {
			text := mcpclient.RenderText(&mcp.CallToolResult{Content: []mcp.Content{c}})
			fmt.Fprintln(out, strings.TrimRight(text, "\n"))
			continue
		}
		n++
		path := filepath.Join(dir, fmt.Sprintf("%s-%d%s", meetingID, n, imageExt(img.MIMEType)))
		// Meeting screenshots can hold anything that was on screen, so the
		// files are readable by the owner only.
		if err := os.WriteFile(path, img.Data, 0o600); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
		fmt.Fprintf(out, "Saved %s (%s, %d bytes)\n", path, img.MIMEType, len(img.Data))
	}
	if res.IsError {
		return &mcpclient.ToolError{Tool: toolGetAttachment, Text: "one or more attachments failed; the server's errors are printed above"}
	}
	return nil
}

func imageExt(mimeType string) string {
	switch mimeType {
	case "image/png":
		return ".png"
	case "image/jpeg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	}
	if exts, err := mime.ExtensionsByType(mimeType); err == nil && len(exts) > 0 {
		return exts[0]
	}
	return ".img"
}

func init() {
	f := meetingsAttachmentsCmd.Flags()
	f.StringVar(&attachmentsOutDir, "out-dir", ".", "Directory to save the images in (created if missing)")
	f.BoolVar(&attachmentsJSON, "json", false, "Output the raw MCP tool result as JSON (images base64-encoded) and save nothing")
	meetingsCmd.AddCommand(meetingsAttachmentsCmd)
}
