package render

import (
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/appifyhub/agent-cli/internal/term"
	"github.com/appifyhub/agent-cli/internal/text"
)

// ChatRow holds the values displayed in one chat table row.
type ChatRow struct {
	ID, Platform, Title string
	Private             bool
}

// ChatTable renders a complete, sanitized table with a case-insensitive default marker.
func ChatTable(rows []ChatRow, defaultID string, cat *text.Catalog) string {
	var out strings.Builder
	writer := tabwriter.NewWriter(&out, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintf(writer, "  %s\t%s\t%s\t%s   %s\n",
		cat.T("chats.table_id"), cat.T("chats.table_platform"),
		cat.T("chats.table_title"), cat.T("chats.table_private"), cat.T("chats.table_legend"))
	yes, no, untitled := cat.T("chats.yes"), cat.T("chats.no"), cat.T("chats.untitled")
	for _, row := range rows {
		prefix := "  "
		if defaultID != "" && strings.EqualFold(row.ID, defaultID) {
			prefix = "* "
		}
		title := term.Clean(row.Title, 0)
		if title == "" {
			title = untitled
		}
		private := no
		if row.Private {
			private = yes
		}
		_, _ = fmt.Fprintf(writer, "%s%s\t%s\t%s\t%s\n", prefix,
			term.Clean(row.ID, 0), term.Clean(row.Platform, 0), title, private)
	}
	_ = writer.Flush()
	return out.String()
}
