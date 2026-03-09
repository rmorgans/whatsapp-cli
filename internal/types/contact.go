package types

// IsOnWhatsAppResponse represents the result of checking whether a phone number
// is registered on WhatsApp.
type IsOnWhatsAppResponse struct {
	Query        string `json:"query"`
	JID          string `json:"jid,omitempty"`
	IsOnWhatsApp bool   `json:"is_on_whatsapp"`
}
