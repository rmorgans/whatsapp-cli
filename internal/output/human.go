package output

import (
	"encoding/json"
	"fmt"
	"strings"
)

// humanFormatter is a function that formats a success envelope for a specific command.
type humanFormatter func(env Envelope) (string, error)

// registry maps command IDs to their custom human formatters.
var registry = map[string]humanFormatter{
	"send":                  formatSend,
	"messages.list":         formatMessages,
	"messages.search":       formatMessages,
	"chats.list":            formatChats,
	"groups.list":           formatGroups,
	"contacts.search":       formatContacts,
	"media.download":        formatMediaDownload,
	"messages.react":        formatMessagesReact,
	"messages.delete":       messageIDFormatter("Deleted message %s"),
	"messages.edit":         messageIDFormatter("Edited message %s"),
	"messages.mark-read":    messageIDFormatter("Marked %s as read"),
	"contacts.block":        jidFormatter("Blocked %s"),
	"contacts.unblock":      jidFormatter("Unblocked %s"),
	"groups.create":         formatGroupsCreate,
	"groups.join":           jidFormatter("Joined group (%s)"),
	"groups.leave":          jidFormatter("Left group (%s)"),
	"groups.add-members":    jidFormatter("Added members to %s"),
	"groups.remove-members": jidFormatter("Removed members from %s"),
	"groups.set-name":       jidFormatter("Updated group %s"),
	"groups.set-description": jidFormatter("Updated group %s"),
	"groups.set-photo":      jidFormatter("Updated group %s"),
	"groups.invite-link":    formatGroupsInviteLink,
}

// FormatHuman looks up a per-command formatter by command ID.
// Returns (formatted, true, nil) on success.
// Returns ("", false, nil) if no custom formatter is registered.
// Returns ("", false, err) if the formatter fails — the caller should
// decide whether to fall back to generic output or surface the error.
func FormatHuman(commandID string, env Envelope) (string, bool, error) {
	fn, ok := registry[commandID]
	if !ok {
		return "", false, nil
	}
	result, err := fn(env)
	if err != nil {
		return "", false, fmt.Errorf("formatting %s: %w", commandID, err)
	}
	return result, true, nil
}

// --- Data types for JSON unmarshalling ---

type sendData struct {
	Sent      bool   `json:"sent"`
	ID        string `json:"id"`
	Recipient string `json:"recipient"`
}

type messageData struct {
	ID        string `json:"id"`
	ChatJID   string `json:"chat_jid"`
	ChatName  string `json:"chat_name"`
	Sender    string `json:"sender"`
	Content   string `json:"content"`
	Timestamp string `json:"timestamp"`
	IsFromMe  bool   `json:"is_from_me"`
	MediaType string `json:"media_type"`
}

type chatData struct {
	JID             string `json:"jid"`
	Name            string `json:"name"`
	LastMessageTime string `json:"last_message_time"`
}

type groupData struct {
	JID         string `json:"jid"`
	Name        string `json:"name"`
	MemberCount int    `json:"member_count"`
}

type contactData struct {
	JID         string `json:"jid"`
	Name        string `json:"name"`
	PhoneNumber string `json:"phone_number"`
}

type mediaDownloadData struct {
	MessageID string `json:"message_id"`
	Path      string `json:"path"`
	Bytes     int64  `json:"bytes"`
	MediaType string `json:"media_type"`
}

type reactData struct {
	MessageID string `json:"message_id"`
	Emoji     string `json:"emoji"`
}

type messageIDData struct {
	MessageID string `json:"message_id"`
}

type jidData struct {
	JID string `json:"jid"`
}

type groupInfoData struct {
	JID  string `json:"jid"`
	Name string `json:"name"`
}

type inviteLinkData struct {
	JID  string `json:"jid"`
	Link string `json:"link"`
}

// --- Formatters ---

func formatSend(env Envelope) (string, error) {
	var d sendData
	if err := json.Unmarshal(env.Data, &d); err != nil {
		return "", err
	}
	return fmt.Sprintf("Sent to %s (ID: %s)", d.Recipient, d.ID), nil
}

func formatMessages(env Envelope) (string, error) {
	var msgs []messageData
	if err := json.Unmarshal(env.Data, &msgs); err != nil {
		return "", err
	}
	if len(msgs) == 0 {
		return "No results.", nil
	}

	headers := []string{"TIME", "CHAT", "SENDER", "MESSAGE"}
	rows := make([][]string, len(msgs))
	for i, m := range msgs {
		ts := m.Timestamp
		if t, ok := tryParseTimestamp(ts); ok {
			ts = formatTimestamp(t)
		}
		sender := m.Sender
		if m.IsFromMe {
			sender = "me"
		}
		message := m.Content
		if m.MediaType != "" {
			message += " [" + mediaLabel(m.MediaType) + "]"
		}
		rows[i] = []string{ts, m.ChatName, sender, message}
	}

	return renderTable(headers, rows), nil
}

func formatChats(env Envelope) (string, error) {
	var chats []chatData
	if err := json.Unmarshal(env.Data, &chats); err != nil {
		return "", err
	}
	if len(chats) == 0 {
		return "No results.", nil
	}

	headers := []string{"LAST ACTIVE", "NAME", "JID"}
	rows := make([][]string, len(chats))
	for i, c := range chats {
		ts := c.LastMessageTime
		if t, ok := tryParseTimestamp(ts); ok {
			ts = formatTimestamp(t)
		}
		rows[i] = []string{ts, c.Name, c.JID}
	}

	return renderTable(headers, rows), nil
}

func formatGroups(env Envelope) (string, error) {
	var groups []groupData
	if err := json.Unmarshal(env.Data, &groups); err != nil {
		return "", err
	}
	if len(groups) == 0 {
		return "No results.", nil
	}

	headers := []string{"NAME", "MEMBERS", "JID"}
	rows := make([][]string, len(groups))
	for i, g := range groups {
		rows[i] = []string{g.Name, fmt.Sprintf("%d", g.MemberCount), g.JID}
	}

	return renderTable(headers, rows), nil
}

func formatContacts(env Envelope) (string, error) {
	var contacts []contactData
	if err := json.Unmarshal(env.Data, &contacts); err != nil {
		return "", err
	}
	if len(contacts) == 0 {
		return "No results.", nil
	}

	headers := []string{"NAME", "PHONE", "JID"}
	rows := make([][]string, len(contacts))
	for i, c := range contacts {
		rows[i] = []string{c.Name, c.PhoneNumber, c.JID}
	}

	return renderTable(headers, rows), nil
}

func formatMediaDownload(env Envelope) (string, error) {
	var d mediaDownloadData
	if err := json.Unmarshal(env.Data, &d); err != nil {
		return "", err
	}
	label := "media"
	if d.MediaType != "" {
		label = strings.ToLower(d.MediaType)
	}
	return fmt.Sprintf("Downloaded %s (%s) -> %s", label, formatBytes(d.Bytes), d.Path), nil
}

func formatMessagesReact(env Envelope) (string, error) {
	var d reactData
	if err := json.Unmarshal(env.Data, &d); err != nil {
		return "", err
	}
	return fmt.Sprintf("Reacted %s to message %s", d.Emoji, d.MessageID), nil
}

// jidFormatter returns a humanFormatter that unmarshals a JID
// and applies the given format string (must contain one %s verb).
func jidFormatter(tmpl string) humanFormatter {
	return func(env Envelope) (string, error) {
		var d jidData
		if err := json.Unmarshal(env.Data, &d); err != nil {
			return "", err
		}
		return fmt.Sprintf(tmpl, d.JID), nil
	}
}

// messageIDFormatter returns a humanFormatter that unmarshals a message ID
// and applies the given format string (must contain one %s verb).
func messageIDFormatter(tmpl string) humanFormatter {
	return func(env Envelope) (string, error) {
		var d messageIDData
		if err := json.Unmarshal(env.Data, &d); err != nil {
			return "", err
		}
		return fmt.Sprintf(tmpl, d.MessageID), nil
	}
}

func formatGroupsCreate(env Envelope) (string, error) {
	var d groupInfoData
	if err := json.Unmarshal(env.Data, &d); err != nil {
		return "", err
	}
	return fmt.Sprintf("Created group %q (%s)", d.Name, d.JID), nil
}

func formatGroupsInviteLink(env Envelope) (string, error) {
	var d inviteLinkData
	if err := json.Unmarshal(env.Data, &d); err != nil {
		return "", err
	}
	return fmt.Sprintf("Invite link for %s: %s", d.JID, d.Link), nil
}

// --- Helpers ---

// mediaLabel capitalises a media type for display (e.g. "image" -> "Image").
func mediaLabel(mediaType string) string {
	if mediaType == "" {
		return ""
	}
	return strings.ToUpper(mediaType[:1]) + mediaType[1:]
}

// formatBytes formats a byte count as human-readable (e.g. "1.2 MB").
func formatBytes(b int64) string {
	const (
		kb = 1024
		mb = 1024 * kb
		gb = 1024 * mb
	)
	switch {
	case b >= gb:
		return fmt.Sprintf("%.1f GB", float64(b)/float64(gb))
	case b >= mb:
		return fmt.Sprintf("%.1f MB", float64(b)/float64(mb))
	case b >= kb:
		return fmt.Sprintf("%.0f KB", float64(b)/float64(kb))
	default:
		return fmt.Sprintf("%d B", b)
	}
}

// renderTable builds a manually-controlled table with explicit column order.
// This intentionally does NOT use formatTable from generic.go, because we need
// to control column order and which fields appear.
func renderTable(headers []string, rows [][]string) string {
	numCols := len(headers)

	// Compute column widths.
	widths := make([]int, numCols)
	for c, h := range headers {
		widths[c] = runeWidth(h)
	}
	for _, row := range rows {
		for c := 0; c < numCols && c < len(row); c++ {
			w := runeWidth(row[c])
			if w > widths[c] {
				widths[c] = w
			}
		}
	}
	for c := range widths {
		if widths[c] > maxColWidth {
			widths[c] = maxColWidth
		}
	}

	// Determine visible columns within maxTotalWidth.
	visibleCols := 0
	usedWidth := 0
	for c, w := range widths {
		needed := w
		if c > 0 {
			needed += len(colSeparator)
		}
		if usedWidth+needed > maxTotalWidth {
			break
		}
		usedWidth += needed
		visibleCols++
	}
	if visibleCols == 0 && numCols > 0 {
		visibleCols = 1
	}

	var b strings.Builder

	// Header line.
	for c := 0; c < visibleCols; c++ {
		if c > 0 {
			b.WriteString(colSeparator)
		}
		b.WriteString(padOrTruncate(headers[c], widths[c]))
	}
	b.WriteString("\n")

	// Data rows.
	for _, row := range rows {
		for c := 0; c < visibleCols; c++ {
			if c > 0 {
				b.WriteString(colSeparator)
			}
			cell := ""
			if c < len(row) {
				cell = row[c]
			}
			b.WriteString(padOrTruncate(cell, widths[c]))
		}
		b.WriteString("\n")
	}

	return trimTrailingWhitespace(b.String())
}

