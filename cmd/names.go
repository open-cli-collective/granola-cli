package cmd

// MCP tool names, as Granola's server lists them in tools/list.
const (
	toolListMeetings   = "list_meetings"
	toolGetMeetings    = "get_meetings"
	toolTranscript     = "get_meeting_transcript"
	toolQueryMeetings  = "query_granola_meetings"
	toolListFolders    = "list_meeting_folders"
	toolGetAccountInfo = "get_account_info"
	toolGetAttachment  = "get_meeting_attachment"
)

// Subcommand verbs shared across command groups.
const (
	verbList = "list"
	verbGet  = "get"
)
