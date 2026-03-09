package types

// GroupInfo represents a WhatsApp group without leaking whatsmeow types.
type GroupInfo struct {
	JID         string              `json:"jid"`
	Name        string              `json:"name"`
	Description string              `json:"description,omitempty"`
	MemberCount int                 `json:"member_count"`
	Members     []GroupParticipant  `json:"members,omitempty"`
	CreatedAt   int64               `json:"created_at,omitempty"`
}

// GroupParticipant represents a participant of a WhatsApp group.
type GroupParticipant struct {
	JID          string `json:"jid"`
	IsAdmin      bool   `json:"is_admin,omitempty"`
	IsSuperAdmin bool   `json:"is_super_admin,omitempty"`
}
