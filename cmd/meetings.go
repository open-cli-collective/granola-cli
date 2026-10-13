package cmd

import (
	"fmt"
	"slices"
	"strings"

	"github.com/spf13/cobra"
)

const (
	maxMeetingsPerGet = 10
	defaultTimeRange  = "last_30_days"
	flagCapturedByMe  = "captured-by-me"
	flagParticipant   = "participant"
	argTimeRange      = "time_range"
	argQuery          = "query"
	groupMeetings     = "meetings"
)

var timeRanges = []string{"this_week", "last_week", "last_30_days"}

var (
	listTimeRange     string
	listCapturedByMe  bool
	listParticipant   bool
	listMine          bool
	listWorkspaceOnly bool
	listFolder        string
	listJSON          bool

	getJSON        bool
	transcriptJSON bool

	queryMeetingIDs []string
	queryJSON       bool
)

var meetingsCmd = &cobra.Command{
	Use:     groupMeetings,
	Aliases: []string{"meeting", "m"},
	Short:   "List, read, and query Granola meeting notes",
}

var meetingsListCmd = &cobra.Command{
	Use:     verbList,
	Aliases: []string{"ls"},
	Short:   "List meetings in a time range (list_meetings)",
	Long: `List meeting notes you can access, with IDs, titles, dates, participants,
and note URLs. Wraps the list_meetings MCP tool and prints its text output.

Granola's server takes a named time range rather than arbitrary dates:
this_week, last_week, or last_30_days (the default). On the free Basic plan
the server only returns notes from the last 30 days.

Involvement filters follow the tool's rules: conditions set to true are
combined with OR, conditions set to false exclude matches, and an omitted
flag is ignored.

Examples:
  granola meetings list
  granola meetings list --time-range this_week
  granola meetings list --mine
  granola meetings list --captured-by-me=false
  granola meetings list --workspace-only`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		toolArgs, err := listMeetingsArgs(cmd)
		if err != nil {
			return err
		}
		return runTool(cmd, toolListMeetings, toolArgs, listJSON)
	},
}

func listMeetingsArgs(cmd *cobra.Command) (map[string]any, error) {
	if !slices.Contains(timeRanges, listTimeRange) {
		return nil, usageErrorf("--time-range must be one of %s", strings.Join(timeRanges, ", "))
	}
	args := map[string]any{argTimeRange: listTimeRange}

	involvement := map[string]any{}
	if listMine {
		if cmd.Flags().Changed(flagCapturedByMe) || cmd.Flags().Changed(flagParticipant) {
			return nil, usageErrorf("--mine sets both involvement conditions; do not combine it with --%s or --%s", flagCapturedByMe, flagParticipant)
		}
		involvement["captured_by_me"] = true
		involvement["listed_as_participant"] = true
	}
	if cmd.Flags().Changed(flagCapturedByMe) {
		involvement["captured_by_me"] = listCapturedByMe
	}
	if cmd.Flags().Changed(flagParticipant) {
		involvement["listed_as_participant"] = listParticipant
	}
	if len(involvement) > 0 {
		args["involvement"] = involvement
	}
	if listWorkspaceOnly {
		args["workspace_only"] = true
	}
	if listFolder != "" {
		args["folder_id"] = listFolder
	}
	return args, nil
}

var meetingsGetCmd = &cobra.Command{
	Use:   "get <meeting-id>...",
	Short: "Read one or more meetings: summary, private notes, attendees (get_meetings)",
	Long: fmt.Sprintf(`Read meetings by ID: the AI summary, your private notes, attendees, and
metadata. Wraps the get_meetings MCP tool and prints its text output.

Meeting IDs are the UUIDs "granola meetings list" prints. The server accepts
up to %d IDs per call.

Examples:
  granola meetings get <meeting-id>
  granola meetings get <meeting-id> <meeting-id> --json`, maxMeetingsPerGet),
	Args: cobra.RangeArgs(1, maxMeetingsPerGet),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runTool(cmd, toolGetMeetings, map[string]any{"meeting_ids": args}, getJSON)
	},
}

var meetingsTranscriptCmd = &cobra.Command{
	Use:   "transcript <meeting-id>",
	Short: "Print a meeting's verbatim transcript (get_meeting_transcript)",
	Long: `Print the verbatim transcript of one meeting. Wraps the
get_meeting_transcript MCP tool and prints its text output.

Granola offers transcripts through MCP on paid plans only; on the free plan
the server's refusal is printed as it sent it and the command exits 5.

Example:
  granola meetings transcript <meeting-id>`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runTool(cmd, toolTranscript, map[string]any{"meeting_id": args[0]}, transcriptJSON)
	},
}

var meetingsQueryCmd = &cobra.Command{
	Use:   "query <question>",
	Short: "Ask a natural-language question about your meetings (query_granola_meetings)",
	Long: `Ask Granola a natural-language question about your meeting notes. Wraps the
query_granola_meetings MCP tool: the search and the answer are done by
Granola's server, which returns a written answer with numbered citation links
to the source notes. This is the only search the server offers; there is no
keyword or date-range search tool.

Examples:
  granola meetings query "what did we decide about the rollout?"
  granola meetings query "open action items for me" --meeting <meeting-id>`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if strings.TrimSpace(args[0]) == "" {
			return usageErrorf("the question must not be empty")
		}
		toolArgs := map[string]any{argQuery: args[0]}
		if len(queryMeetingIDs) > 0 {
			toolArgs["document_ids"] = queryMeetingIDs
		}
		return runTool(cmd, toolQueryMeetings, toolArgs, queryJSON)
	},
}

func init() {
	f := meetingsListCmd.Flags()
	f.StringVar(&listTimeRange, "time-range", defaultTimeRange, "Time range: "+strings.Join(timeRanges, ", "))
	f.BoolVar(&listCapturedByMe, flagCapturedByMe, false, "true: include meetings whose note you captured; false: exclude them")
	f.BoolVar(&listParticipant, flagParticipant, false, "true: include meetings listing you as a participant; false: exclude them")
	f.BoolVar(&listMine, "mine", false, "Meetings you captured or are listed in (sets both involvement conditions to true)")
	f.BoolVar(&listWorkspaceOnly, "workspace-only", false, "Only Team Space and workspace-visible meetings")
	f.StringVar(&listFolder, "folder", "", "Folder ID from granola folders list, passed as folder_id (paid plans; the free plan ignores it)")
	f.BoolVar(&listJSON, "json", false, "Output the raw MCP tool result as JSON")

	meetingsGetCmd.Flags().BoolVar(&getJSON, "json", false, "Output the raw MCP tool result as JSON")
	meetingsTranscriptCmd.Flags().BoolVar(&transcriptJSON, "json", false, "Output the raw MCP tool result as JSON")
	meetingsQueryCmd.Flags().StringSliceVar(&queryMeetingIDs, "meeting", nil, "Limit the question to these meeting IDs (repeatable or comma-separated)")
	meetingsQueryCmd.Flags().BoolVar(&queryJSON, "json", false, "Output the raw MCP tool result as JSON")

	meetingsCmd.AddCommand(meetingsListCmd, meetingsGetCmd, meetingsTranscriptCmd, meetingsQueryCmd)
	rootCmd.AddCommand(meetingsCmd)
}
