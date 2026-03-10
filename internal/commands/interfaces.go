// Package commands provides the CLI command implementations.
//
// # Dependency Injection
//
// The interfaces below define the dependencies of App, enabling testability
// through mock injection. Types are shared via internal/types to avoid
// circular dependencies.
//
// Usage:
//   - Production: Use NewApp() which creates concrete implementations
//   - Testing: Use NewAppWithDeps() to inject mocks
package commands

import (
	"context"
	"time"

	"github.com/vicentereig/whatsapp-cli/internal/client"
	"github.com/vicentereig/whatsapp-cli/internal/store"
	"github.com/vicentereig/whatsapp-cli/internal/types"
	"go.mau.fi/whatsmeow/types/events"
)

// Ensure client.WAClient satisfies the WAClient interface at compile time.
var _ WAClient = (*client.WAClient)(nil)

// MessageStore defines the interface for message persistence.
// The concrete implementation is store.MessageStore.
// Defined here (at consumer) per Go best practice: "Accept interfaces, return concrete types"
type MessageStore interface {
	ListMessages(params store.ListMessagesParams) ([]store.Message, error)
	SearchContacts(query string) ([]store.Contact, error)
	ListChats(params store.ListChatsParams) ([]store.Chat, error)
	StoreChat(jid, name string, lastMessageTime time.Time) error
	StoreMessage(p store.StoreMessageParams) error
	GetMessageForDownload(id string, chatJID *string) (store.MessageDownloadInfo, error)
	GetMessageMetadata(id string, chatJID *string) (store.Message, error)
	MarkMediaDownloaded(id, chatJID, localPath string, downloadedAt time.Time) error
	GetLIDSenders() ([]store.LIDSenderRow, error)
	UpdateSender(id, chatJID, newSender string) error
	GetLIDChats() ([]store.LIDChatRow, error)
	UpdateChatJID(oldJID, newJID string) error
	Close() error
}

// WAClient defines the interface for WhatsApp client operations.
// The concrete implementation is client.WAClient.
type WAClient interface {
	IsAuthenticated() bool
	Authenticate(ctx context.Context) error
	Connect(ctx context.Context) error
	Disconnect()
	GetOwnJID() string
	SendMessage(ctx context.Context, recipient, message string) (string, error)
	SendImageMessage(ctx context.Context, recipient, imagePath, caption string) (string, error)
	SendVideoMessage(ctx context.Context, recipient, videoPath, caption string) (string, error)
	SendAudioMessage(ctx context.Context, recipient, audioPath string, ptt bool) (string, error)
	SendDocumentMessage(ctx context.Context, recipient, docPath, filename string) (string, error)
	SendTextReply(ctx context.Context, recipient, message, replyToID, replyToSender string) (string, error)
	ResolveChatName(ctx context.Context, jid string, evt interface{}) string
	DownloadMediaToFile(ctx context.Context, req types.MediaDownloadRequest, targetPath string) (int64, error)
	ReactToMessage(ctx context.Context, chatJID, senderJID, messageID, emoji string) error
	RevokeMessage(ctx context.Context, chatJID, senderJID, messageID string) error
	EditMessage(ctx context.Context, chatJID, messageID, newText string) error
	MarkRead(ctx context.Context, messageIDs []string, timestamp time.Time, chatJID, senderJID string) error
	StartSync(ctx context.Context, eventHandler func(interface{})) error
	HandleMessage(ctx context.Context, msg *events.Message) client.MessageDetails
	ResolveJID(ctx context.Context, jid string) string

	// Contact operations
	UpdateBlocklist(ctx context.Context, jid string, action string) error
	GetBlocklist(ctx context.Context) ([]string, error)
	IsOnWhatsApp(ctx context.Context, phones []string) ([]types.IsOnWhatsAppResponse, error)

	// Group operations
	GetJoinedGroups(ctx context.Context) ([]types.GroupInfo, error)
	GetGroupInfo(ctx context.Context, jid string) (*types.GroupInfo, error)
	CreateGroup(ctx context.Context, name string, members []string) (*types.GroupInfo, error)
	GetGroupInviteLink(ctx context.Context, jid string, reset bool) (string, error)
	JoinGroupWithLink(ctx context.Context, link string) (string, error)
	LeaveGroup(ctx context.Context, jid string) error
	UpdateGroupParticipants(ctx context.Context, jid string, members []string, action string) error
	SetGroupName(ctx context.Context, jid, name string) error
	SetGroupDescription(ctx context.Context, jid, description string) error
	SetGroupPhoto(ctx context.Context, jid, imagePath string) error
}
