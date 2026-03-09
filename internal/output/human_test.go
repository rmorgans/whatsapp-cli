package output

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- FormatHuman dispatch tests ---

func TestFormatHuman_UnknownCommand(t *testing.T) {
	t.Parallel()

	env := envelope(true, map[string]interface{}{"foo": "bar"}, nil)
	_, ok := FormatHuman("no.such.command", env)
	assert.False(t, ok)
}

func TestFormatHuman_BadData_ReturnsFalse(t *testing.T) {
	t.Parallel()

	// send formatter expects an object, give it a string
	env := Envelope{
		Success: true,
		Data:    json.RawMessage(`"not an object"`),
	}
	_, ok := FormatHuman("send", env)
	assert.False(t, ok)
}

func TestFormatHuman_KnownCommand_ReturnsTrue(t *testing.T) {
	t.Parallel()

	env := envelope(true, map[string]interface{}{
		"sent":      true,
		"id":        "ABC123",
		"recipient": "61412345678@s.whatsapp.net",
	}, nil)
	result, ok := FormatHuman("send", env)
	assert.True(t, ok)
	assert.Equal(t, "Sent to 61412345678@s.whatsapp.net (ID: ABC123)", result)
}

// --- send formatter ---

func TestFormatSend(t *testing.T) {
	t.Parallel()

	env := envelope(true, map[string]interface{}{
		"sent":      true,
		"id":        "MSG001",
		"recipient": "61400000000@s.whatsapp.net",
	}, nil)
	got, err := formatSend(env)
	require.NoError(t, err)
	assert.Equal(t, "Sent to 61400000000@s.whatsapp.net (ID: MSG001)", got)
}

// --- messages.list / messages.search formatter ---

func TestFormatMessages(t *testing.T) {
	freezeTime(t)

	ts := frozenNow.Add(-5 * time.Minute).Format(time.RFC3339)
	env := envelope(true, []map[string]interface{}{
		{
			"id":         "m1",
			"chat_jid":   "123@s.whatsapp.net",
			"chat_name":  "Alice",
			"sender":     "61400000000@s.whatsapp.net",
			"content":    "Hello there",
			"timestamp":  ts,
			"is_from_me": false,
			"media_type": "",
		},
		{
			"id":         "m2",
			"chat_jid":   "456@s.whatsapp.net",
			"chat_name":  "Bob",
			"sender":     "me@s.whatsapp.net",
			"content":    "Check this out",
			"timestamp":  ts,
			"is_from_me": true,
			"media_type": "image",
		},
	}, nil)

	got, err := formatMessages(env)
	require.NoError(t, err)

	lines := strings.Split(got, "\n")
	require.True(t, len(lines) >= 3, "expected header + 2 rows, got %d lines", len(lines))

	// Header has correct columns in order.
	assert.Contains(t, lines[0], "TIME")
	assert.Contains(t, lines[0], "CHAT")
	assert.Contains(t, lines[0], "SENDER")
	assert.Contains(t, lines[0], "MESSAGE")
	assert.True(t, strings.Index(lines[0], "TIME") < strings.Index(lines[0], "CHAT"))
	assert.True(t, strings.Index(lines[0], "CHAT") < strings.Index(lines[0], "SENDER"))
	assert.True(t, strings.Index(lines[0], "SENDER") < strings.Index(lines[0], "MESSAGE"))

	// Row 1: not from me, no media.
	assert.Contains(t, lines[1], "5m ago")
	assert.Contains(t, lines[1], "Alice")
	assert.Contains(t, lines[1], "61400000000@s.whatsapp.net")
	assert.Contains(t, lines[1], "Hello there")

	// Row 2: from me, image media type.
	assert.Contains(t, lines[2], "me")
	assert.Contains(t, lines[2], "[Image]")
}

func TestFormatMessages_Empty(t *testing.T) {
	t.Parallel()

	env := envelope(true, []map[string]interface{}{}, nil)
	got, err := formatMessages(env)
	require.NoError(t, err)
	assert.Equal(t, "No results.", got)
}

func TestFormatMessages_SharedCommand(t *testing.T) {
	freezeTime(t)

	ts := frozenNow.Add(-1 * time.Minute).Format(time.RFC3339)
	env := envelope(true, []map[string]interface{}{
		{
			"id": "m1", "chat_jid": "c@s", "chat_name": "C",
			"sender": "s@s", "content": "hi", "timestamp": ts,
			"is_from_me": false, "media_type": "",
		},
	}, nil)

	// Both messages.list and messages.search should use the same formatter.
	r1, ok1 := FormatHuman("messages.list", env)
	r2, ok2 := FormatHuman("messages.search", env)
	assert.True(t, ok1)
	assert.True(t, ok2)
	assert.Equal(t, r1, r2)
}

// --- chats.list formatter ---

func TestFormatChats(t *testing.T) {
	freezeTime(t)

	ts := frozenNow.Add(-2 * time.Hour).Format(time.RFC3339)
	env := envelope(true, []map[string]interface{}{
		{
			"jid":               "123@s.whatsapp.net",
			"name":              "Family Group",
			"last_message_time": ts,
		},
	}, nil)

	got, err := formatChats(env)
	require.NoError(t, err)

	lines := strings.Split(got, "\n")
	require.True(t, len(lines) >= 2)

	assert.Contains(t, lines[0], "LAST ACTIVE")
	assert.Contains(t, lines[0], "NAME")
	assert.Contains(t, lines[0], "JID")
	assert.True(t, strings.Index(lines[0], "LAST ACTIVE") < strings.Index(lines[0], "NAME"))

	assert.Contains(t, lines[1], "2h ago")
	assert.Contains(t, lines[1], "Family Group")
	assert.Contains(t, lines[1], "123@s.whatsapp.net")
}

func TestFormatChats_Empty(t *testing.T) {
	t.Parallel()

	env := envelope(true, []map[string]interface{}{}, nil)
	got, err := formatChats(env)
	require.NoError(t, err)
	assert.Equal(t, "No results.", got)
}

// --- groups.list formatter ---

func TestFormatGroups(t *testing.T) {
	t.Parallel()

	env := envelope(true, []map[string]interface{}{
		{
			"jid":          "g1@g.us",
			"name":         "Dev Team",
			"member_count": 15,
			"members":      []interface{}{"a", "b"},
			"created_at":   1700000000,
		},
	}, nil)

	got, err := formatGroups(env)
	require.NoError(t, err)

	lines := strings.Split(got, "\n")
	require.True(t, len(lines) >= 2)

	assert.Contains(t, lines[0], "NAME")
	assert.Contains(t, lines[0], "MEMBERS")
	assert.Contains(t, lines[0], "JID")
	assert.True(t, strings.Index(lines[0], "NAME") < strings.Index(lines[0], "MEMBERS"))
	assert.True(t, strings.Index(lines[0], "MEMBERS") < strings.Index(lines[0], "JID"))

	assert.Contains(t, lines[1], "Dev Team")
	assert.Contains(t, lines[1], "15")
	assert.Contains(t, lines[1], "g1@g.us")

	// members array and created_at should NOT appear in output.
	assert.NotContains(t, got, "[...")
	assert.NotContains(t, got, "1700000000")
	assert.NotContains(t, got, "CREATED")
}

func TestFormatGroups_Empty(t *testing.T) {
	t.Parallel()

	env := envelope(true, []map[string]interface{}{}, nil)
	got, err := formatGroups(env)
	require.NoError(t, err)
	assert.Equal(t, "No results.", got)
}

// --- contacts.search formatter ---

func TestFormatContacts(t *testing.T) {
	t.Parallel()

	env := envelope(true, []map[string]interface{}{
		{
			"jid":          "c1@s.whatsapp.net",
			"name":         "Alice Smith",
			"phone_number": "+61400000000",
		},
		{
			"jid":          "c2@s.whatsapp.net",
			"name":         "Bob Jones",
			"phone_number": "+61411111111",
		},
	}, nil)

	got, err := formatContacts(env)
	require.NoError(t, err)

	lines := strings.Split(got, "\n")
	require.True(t, len(lines) >= 3)

	assert.Contains(t, lines[0], "NAME")
	assert.Contains(t, lines[0], "PHONE")
	assert.Contains(t, lines[0], "JID")
	assert.True(t, strings.Index(lines[0], "NAME") < strings.Index(lines[0], "PHONE"))
	assert.True(t, strings.Index(lines[0], "PHONE") < strings.Index(lines[0], "JID"))

	assert.Contains(t, lines[1], "Alice Smith")
	assert.Contains(t, lines[1], "+61400000000")
	assert.Contains(t, lines[2], "Bob Jones")
}

func TestFormatContacts_Empty(t *testing.T) {
	t.Parallel()

	env := envelope(true, []map[string]interface{}{}, nil)
	got, err := formatContacts(env)
	require.NoError(t, err)
	assert.Equal(t, "No results.", got)
}

// --- media.download formatter ---

func TestFormatMediaDownload(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		bytes int64
		want  string
	}{
		{
			name:  "megabytes",
			bytes: 1234567,
			want:  "Downloaded media (1.2 MB) -> /tmp/photo.jpg",
		},
		{
			name:  "kilobytes",
			bytes: 456000,
			want:  "Downloaded media (445 KB) -> /tmp/photo.jpg",
		},
		{
			name:  "bytes",
			bytes: 512,
			want:  "Downloaded media (512 B) -> /tmp/photo.jpg",
		},
		{
			name:  "gigabytes",
			bytes: 2147483648,
			want:  "Downloaded media (2.0 GB) -> /tmp/photo.jpg",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			env := envelope(true, map[string]interface{}{
				"message_id": "m1",
				"path":       "/tmp/photo.jpg",
				"bytes":      tt.bytes,
			}, nil)
			got, err := formatMediaDownload(env)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// --- Mutation one-liner formatters ---

func TestMutationFormatters(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		commandID string
		data      interface{}
		want      string
	}{
		{
			name:      "messages.react",
			commandID: "messages.react",
			data:      map[string]interface{}{"message_id": "m1", "emoji": "\U0001f44d"},
			want:      "Reacted \U0001f44d to message m1",
		},
		{
			name:      "messages.delete",
			commandID: "messages.delete",
			data:      map[string]interface{}{"message_id": "m1"},
			want:      "Deleted message m1",
		},
		{
			name:      "messages.edit",
			commandID: "messages.edit",
			data:      map[string]interface{}{"message_id": "m1"},
			want:      "Edited message m1",
		},
		{
			name:      "messages.mark-read",
			commandID: "messages.mark-read",
			data:      map[string]interface{}{"message_id": "m1"},
			want:      "Marked m1 as read",
		},
		{
			name:      "contacts.block",
			commandID: "contacts.block",
			data:      map[string]interface{}{"jid": "u1@s.whatsapp.net"},
			want:      "Blocked u1@s.whatsapp.net",
		},
		{
			name:      "contacts.unblock",
			commandID: "contacts.unblock",
			data:      map[string]interface{}{"jid": "u1@s.whatsapp.net"},
			want:      "Unblocked u1@s.whatsapp.net",
		},
		{
			name:      "groups.create",
			commandID: "groups.create",
			data:      map[string]interface{}{"jid": "g1@g.us", "name": "New Group"},
			want:      `Created group "New Group" (g1@g.us)`,
		},
		{
			name:      "groups.join",
			commandID: "groups.join",
			data:      map[string]interface{}{"jid": "g1@g.us"},
			want:      "Joined group (g1@g.us)",
		},
		{
			name:      "groups.leave",
			commandID: "groups.leave",
			data:      map[string]interface{}{"jid": "g1@g.us"},
			want:      "Left group (g1@g.us)",
		},
		{
			name:      "groups.add-members",
			commandID: "groups.add-members",
			data:      map[string]interface{}{"jid": "g1@g.us"},
			want:      "Added members to g1@g.us",
		},
		{
			name:      "groups.remove-members",
			commandID: "groups.remove-members",
			data:      map[string]interface{}{"jid": "g1@g.us"},
			want:      "Removed members from g1@g.us",
		},
		{
			name:      "groups.set-name",
			commandID: "groups.set-name",
			data:      map[string]interface{}{"jid": "g1@g.us"},
			want:      "Updated group g1@g.us",
		},
		{
			name:      "groups.set-description",
			commandID: "groups.set-description",
			data:      map[string]interface{}{"jid": "g1@g.us"},
			want:      "Updated group g1@g.us",
		},
		{
			name:      "groups.set-photo",
			commandID: "groups.set-photo",
			data:      map[string]interface{}{"jid": "g1@g.us"},
			want:      "Updated group g1@g.us",
		},
		{
			name:      "groups.invite-link",
			commandID: "groups.invite-link",
			data:      map[string]interface{}{"jid": "g1@g.us", "link": "https://chat.whatsapp.com/ABC"},
			want:      "Invite link for g1@g.us: https://chat.whatsapp.com/ABC",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			env := envelope(true, tt.data, nil)
			result, ok := FormatHuman(tt.commandID, env)
			assert.True(t, ok)
			assert.Equal(t, tt.want, result)
		})
	}
}

// --- formatBytes helper ---

func TestFormatBytes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input int64
		want  string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1 KB"},
		{1536, "2 KB"},
		{1048576, "1.0 MB"},
		{1234567, "1.2 MB"},
		{1073741824, "1.0 GB"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, formatBytes(tt.input))
		})
	}
}

// --- mediaLabel helper ---

func TestMediaLabel(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "Image", mediaLabel("image"))
	assert.Equal(t, "Video", mediaLabel("video"))
	assert.Equal(t, "Document", mediaLabel("document"))
	assert.Equal(t, "", mediaLabel(""))
}

// --- Table rendering (no trailing whitespace) ---

func TestRenderTable_NoTrailingWhitespace(t *testing.T) {
	t.Parallel()

	headers := []string{"A", "B"}
	rows := [][]string{
		{"short", "x"},
		{"longer value", "y"},
	}
	got := renderTable(headers, rows)
	for i, line := range strings.Split(got, "\n") {
		assert.Equal(t, strings.TrimRight(line, " \t"), line,
			"line %d has trailing whitespace", i)
	}
}
