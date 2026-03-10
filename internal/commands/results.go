package commands

// SendResult is returned by SendMessage, SendReply, SendImage, SendVideo, SendAudio, SendDocument.
type SendResult struct {
	Sent      bool   `json:"sent"`
	ID        string `json:"id"`
	Recipient string `json:"recipient"`
	Message   string `json:"message,omitempty"`
	Image     string `json:"image,omitempty"`
	Video     string `json:"video,omitempty"`
	Audio     string `json:"audio,omitempty"`
	Document  string `json:"document,omitempty"`
	Filename  string `json:"filename,omitempty"`
	Caption   string `json:"caption,omitempty"`
	ReplyTo   string `json:"reply_to,omitempty"`
}

// ReactResult is returned by ReactToMessage.
type ReactResult struct {
	Reacted   bool   `json:"reacted"`
	MessageID string `json:"message_id"`
	ChatJID   string `json:"chat_jid"`
	Emoji     string `json:"emoji"`
	Action    string `json:"action,omitempty"`
}

// MessageActionResult is returned by DeleteMessage, EditMessage, MarkMessageRead.
type MessageActionResult struct {
	MessageID  string `json:"message_id"`
	ChatJID    string `json:"chat_jid"`
	Deleted    bool   `json:"deleted,omitempty"`
	Edited     bool   `json:"edited,omitempty"`
	MarkedRead bool   `json:"marked_read,omitempty"`
	NewText    string `json:"new_text,omitempty"`
}

// MediaDownloadResult is returned by DownloadMedia.
type MediaDownloadResult struct {
	MessageID    string `json:"message_id"`
	ChatJID      string `json:"chat_jid"`
	Path         string `json:"path"`
	Bytes        int64  `json:"bytes"`
	MediaType    string `json:"media_type"`
	MimeType     string `json:"mime_type"`
	DownloadedAt string `json:"downloaded_at"`
	ChatName     string `json:"chat_name,omitempty"`
}

// JIDResult is used by group/contact mutations that return a JID.
type JIDResult struct {
	JID         string   `json:"jid"`
	Joined      bool     `json:"joined,omitempty"`
	Left        bool     `json:"left,omitempty"`
	Blocked     bool     `json:"blocked,omitempty"`
	Unblocked   bool     `json:"unblocked,omitempty"`
	Added       bool     `json:"added,omitempty"`
	Removed     bool     `json:"removed,omitempty"`
	Updated     bool     `json:"updated,omitempty"`
	Members     []string `json:"members,omitempty"`
	Name        string   `json:"name,omitempty"`
	Description string   `json:"description,omitempty"`
}

// InviteLinkResult is returned by GroupsInviteLink.
type InviteLinkResult struct {
	JID  string `json:"jid"`
	Link string `json:"link"`
}

// AuthResult is returned by Auth.
type AuthResult struct {
	Authenticated bool   `json:"authenticated"`
	Message       string `json:"message"`
}

// SyncResult is returned by Sync.
type SyncResult struct {
	Synced        bool  `json:"synced"`
	MessagesCount int64 `json:"messages_count"`
}
