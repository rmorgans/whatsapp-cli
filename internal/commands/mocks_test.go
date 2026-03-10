package commands

import (
	"context"
	"time"

	"github.com/vicentereig/whatsapp-cli/internal/client"
	"github.com/vicentereig/whatsapp-cli/internal/store"
	"github.com/vicentereig/whatsapp-cli/internal/types"
	"go.mau.fi/whatsmeow/types/events"
)

// MockMessageStore implements MessageStore for testing.
type MockMessageStore struct {
	ListMessagesFunc        func(params store.ListMessagesParams) ([]store.Message, error)
	SearchContactsFunc      func(query string) ([]store.Contact, error)
	ListChatsFunc           func(params store.ListChatsParams) ([]store.Chat, error)
	StoreChatFunc           func(jid, name string, lastMessageTime time.Time) error
	StoreMessageFunc        func(p store.StoreMessageParams) error
	GetMessageForDownloadFunc func(id string, chatJID *string) (store.MessageDownloadInfo, error)
	GetMessageMetadataFunc    func(id string, chatJID *string) (store.Message, error)
	MarkMediaDownloadedFunc func(id, chatJID, localPath string, downloadedAt time.Time) error
	GetLIDSendersFunc       func() ([]store.LIDSenderRow, error)
	GetBareSendersFunc      func() ([]string, error)
	UpdateSenderFunc        func(id, chatJID, newSender string) error
	UpdateSenderBatchFunc   func(oldSender, newSender string) (int64, error)
	GetLIDChatsFunc         func() ([]store.LIDChatRow, error)
	UpdateChatJIDFunc       func(oldJID, newJID string) error
	CloseFunc               func() error
}

func (m *MockMessageStore) ListMessages(params store.ListMessagesParams) ([]store.Message, error) {
	if m.ListMessagesFunc != nil {
		return m.ListMessagesFunc(params)
	}
	return nil, nil
}

func (m *MockMessageStore) SearchContacts(query string) ([]store.Contact, error) {
	if m.SearchContactsFunc != nil {
		return m.SearchContactsFunc(query)
	}
	return nil, nil
}

func (m *MockMessageStore) ListChats(params store.ListChatsParams) ([]store.Chat, error) {
	if m.ListChatsFunc != nil {
		return m.ListChatsFunc(params)
	}
	return nil, nil
}

func (m *MockMessageStore) StoreChat(jid, name string, lastMessageTime time.Time) error {
	if m.StoreChatFunc != nil {
		return m.StoreChatFunc(jid, name, lastMessageTime)
	}
	return nil
}

func (m *MockMessageStore) StoreMessage(p store.StoreMessageParams) error {
	if m.StoreMessageFunc != nil {
		return m.StoreMessageFunc(p)
	}
	return nil
}

func (m *MockMessageStore) GetMessageForDownload(id string, chatJID *string) (store.MessageDownloadInfo, error) {
	if m.GetMessageForDownloadFunc != nil {
		return m.GetMessageForDownloadFunc(id, chatJID)
	}
	return store.MessageDownloadInfo{}, nil
}

func (m *MockMessageStore) GetMessageMetadata(id string, chatJID *string) (store.Message, error) {
	if m.GetMessageMetadataFunc != nil {
		return m.GetMessageMetadataFunc(id, chatJID)
	}
	return store.Message{}, nil
}

func (m *MockMessageStore) MarkMediaDownloaded(id, chatJID, localPath string, downloadedAt time.Time) error {
	if m.MarkMediaDownloadedFunc != nil {
		return m.MarkMediaDownloadedFunc(id, chatJID, localPath, downloadedAt)
	}
	return nil
}

func (m *MockMessageStore) GetLIDSenders() ([]store.LIDSenderRow, error) {
	if m.GetLIDSendersFunc != nil {
		return m.GetLIDSendersFunc()
	}
	return nil, nil
}

func (m *MockMessageStore) GetBareSenders() ([]string, error) {
	if m.GetBareSendersFunc != nil {
		return m.GetBareSendersFunc()
	}
	return nil, nil
}

func (m *MockMessageStore) UpdateSender(id, chatJID, newSender string) error {
	if m.UpdateSenderFunc != nil {
		return m.UpdateSenderFunc(id, chatJID, newSender)
	}
	return nil
}

func (m *MockMessageStore) UpdateSenderBatch(oldSender, newSender string) (int64, error) {
	if m.UpdateSenderBatchFunc != nil {
		return m.UpdateSenderBatchFunc(oldSender, newSender)
	}
	return 0, nil
}

func (m *MockMessageStore) GetLIDChats() ([]store.LIDChatRow, error) {
	if m.GetLIDChatsFunc != nil {
		return m.GetLIDChatsFunc()
	}
	return nil, nil
}

func (m *MockMessageStore) UpdateChatJID(oldJID, newJID string) error {
	if m.UpdateChatJIDFunc != nil {
		return m.UpdateChatJIDFunc(oldJID, newJID)
	}
	return nil
}

func (m *MockMessageStore) Close() error {
	if m.CloseFunc != nil {
		return m.CloseFunc()
	}
	return nil
}

// MockWAClient implements WAClient for testing.
type MockWAClient struct {
	IsAuthenticatedFunc        func() bool
	AuthenticateFunc           func(ctx context.Context) error
	ConnectFunc                func(ctx context.Context) error
	DisconnectFunc             func()
	GetOwnJIDFunc              func() string
	SendMessageFunc            func(ctx context.Context, recipient, message string) (string, error)
	SendImageMessageFunc       func(ctx context.Context, recipient, imagePath, caption string) (string, error)
	SendVideoMessageFunc       func(ctx context.Context, recipient, videoPath, caption string) (string, error)
	SendAudioMessageFunc       func(ctx context.Context, recipient, audioPath string, ptt bool) (string, error)
	SendDocumentMessageFunc    func(ctx context.Context, recipient, docPath, filename string) (string, error)
	SendTextReplyFunc          func(ctx context.Context, recipient, message, replyToID, replyToSender string) (string, error)
	ReactToMessageFunc         func(ctx context.Context, chatJID, senderJID, messageID, emoji string) error
	RevokeMessageFunc          func(ctx context.Context, chatJID, senderJID, messageID string) error
	EditMessageFunc            func(ctx context.Context, chatJID, messageID, newText string) error
	MarkReadFunc               func(ctx context.Context, messageIDs []string, timestamp time.Time, chatJID, senderJID string) error
	ResolveChatNameFunc        func(ctx context.Context, jid string, evt interface{}) string
	DownloadMediaToFileFunc    func(ctx context.Context, req types.MediaDownloadRequest, targetPath string) (int64, error)
	StartSyncFunc              func(ctx context.Context, eventHandler func(interface{})) error
	HandleMessageFunc          func(ctx context.Context, msg *events.Message) client.MessageDetails
	ResolveJIDFunc             func(ctx context.Context, jid string) string

	// Contact operations
	UpdateBlocklistFunc        func(ctx context.Context, jid string, action string) error
	GetBlocklistFunc           func(ctx context.Context) ([]string, error)
	IsOnWhatsAppFunc           func(ctx context.Context, phones []string) ([]types.IsOnWhatsAppResponse, error)

	// Group operations
	GetJoinedGroupsFunc        func(ctx context.Context) ([]types.GroupInfo, error)
	GetGroupInfoFunc           func(ctx context.Context, jid string) (*types.GroupInfo, error)
	CreateGroupFunc            func(ctx context.Context, name string, members []string) (*types.GroupInfo, error)
	GetGroupInviteLinkFunc     func(ctx context.Context, jid string, reset bool) (string, error)
	JoinGroupWithLinkFunc      func(ctx context.Context, link string) (string, error)
	LeaveGroupFunc             func(ctx context.Context, jid string) error
	UpdateGroupParticipantsFunc func(ctx context.Context, jid string, members []string, action string) error
	SetGroupNameFunc           func(ctx context.Context, jid, name string) error
	SetGroupDescriptionFunc    func(ctx context.Context, jid, description string) error
	SetGroupPhotoFunc          func(ctx context.Context, jid, imagePath string) error
}

func (m *MockWAClient) IsAuthenticated() bool {
	if m.IsAuthenticatedFunc != nil {
		return m.IsAuthenticatedFunc()
	}
	return true
}

func (m *MockWAClient) Authenticate(ctx context.Context) error {
	if m.AuthenticateFunc != nil {
		return m.AuthenticateFunc(ctx)
	}
	return nil
}

func (m *MockWAClient) Connect(ctx context.Context) error {
	if m.ConnectFunc != nil {
		return m.ConnectFunc(ctx)
	}
	return nil
}

func (m *MockWAClient) Disconnect() {
	if m.DisconnectFunc != nil {
		m.DisconnectFunc()
	}
}

func (m *MockWAClient) GetOwnJID() string {
	if m.GetOwnJIDFunc != nil {
		return m.GetOwnJIDFunc()
	}
	return "1234567890@s.whatsapp.net"
}

func (m *MockWAClient) SendMessage(ctx context.Context, recipient, message string) (string, error) {
	if m.SendMessageFunc != nil {
		return m.SendMessageFunc(ctx, recipient, message)
	}
	return "mock-id", nil
}

func (m *MockWAClient) SendImageMessage(ctx context.Context, recipient, imagePath, caption string) (string, error) {
	if m.SendImageMessageFunc != nil {
		return m.SendImageMessageFunc(ctx, recipient, imagePath, caption)
	}
	return "mock-id", nil
}

func (m *MockWAClient) SendVideoMessage(ctx context.Context, recipient, videoPath, caption string) (string, error) {
	if m.SendVideoMessageFunc != nil {
		return m.SendVideoMessageFunc(ctx, recipient, videoPath, caption)
	}
	return "mock-id", nil
}

func (m *MockWAClient) SendAudioMessage(ctx context.Context, recipient, audioPath string, ptt bool) (string, error) {
	if m.SendAudioMessageFunc != nil {
		return m.SendAudioMessageFunc(ctx, recipient, audioPath, ptt)
	}
	return "mock-id", nil
}

func (m *MockWAClient) SendDocumentMessage(ctx context.Context, recipient, docPath, filename string) (string, error) {
	if m.SendDocumentMessageFunc != nil {
		return m.SendDocumentMessageFunc(ctx, recipient, docPath, filename)
	}
	return "mock-id", nil
}

func (m *MockWAClient) SendTextReply(ctx context.Context, recipient, message, replyToID, replyToSender string) (string, error) {
	if m.SendTextReplyFunc != nil {
		return m.SendTextReplyFunc(ctx, recipient, message, replyToID, replyToSender)
	}
	return "mock-reply-id", nil
}

func (m *MockWAClient) ReactToMessage(ctx context.Context, chatJID, senderJID, messageID, emoji string) error {
	if m.ReactToMessageFunc != nil {
		return m.ReactToMessageFunc(ctx, chatJID, senderJID, messageID, emoji)
	}
	return nil
}

func (m *MockWAClient) RevokeMessage(ctx context.Context, chatJID, senderJID, messageID string) error {
	if m.RevokeMessageFunc != nil {
		return m.RevokeMessageFunc(ctx, chatJID, senderJID, messageID)
	}
	return nil
}

func (m *MockWAClient) EditMessage(ctx context.Context, chatJID, messageID, newText string) error {
	if m.EditMessageFunc != nil {
		return m.EditMessageFunc(ctx, chatJID, messageID, newText)
	}
	return nil
}

func (m *MockWAClient) MarkRead(ctx context.Context, messageIDs []string, timestamp time.Time, chatJID, senderJID string) error {
	if m.MarkReadFunc != nil {
		return m.MarkReadFunc(ctx, messageIDs, timestamp, chatJID, senderJID)
	}
	return nil
}

func (m *MockWAClient) ResolveChatName(ctx context.Context, jid string, evt interface{}) string {
	if m.ResolveChatNameFunc != nil {
		return m.ResolveChatNameFunc(ctx, jid, evt)
	}
	return jid
}

func (m *MockWAClient) DownloadMediaToFile(ctx context.Context, req types.MediaDownloadRequest, targetPath string) (int64, error) {
	if m.DownloadMediaToFileFunc != nil {
		return m.DownloadMediaToFileFunc(ctx, req, targetPath)
	}
	return 0, nil
}

func (m *MockWAClient) StartSync(ctx context.Context, eventHandler func(interface{})) error {
	if m.StartSyncFunc != nil {
		return m.StartSyncFunc(ctx, eventHandler)
	}
	return nil
}

func (m *MockWAClient) HandleMessage(ctx context.Context, msg *events.Message) client.MessageDetails {
	if m.HandleMessageFunc != nil {
		return m.HandleMessageFunc(ctx, msg)
	}
	return client.MessageDetails{}
}

func (m *MockWAClient) ResolveJID(ctx context.Context, jid string) string {
	if m.ResolveJIDFunc != nil {
		return m.ResolveJIDFunc(ctx, jid)
	}
	return jid
}

func (m *MockWAClient) UpdateBlocklist(ctx context.Context, jid string, action string) error {
	if m.UpdateBlocklistFunc != nil {
		return m.UpdateBlocklistFunc(ctx, jid, action)
	}
	return nil
}

func (m *MockWAClient) GetBlocklist(ctx context.Context) ([]string, error) {
	if m.GetBlocklistFunc != nil {
		return m.GetBlocklistFunc(ctx)
	}
	return nil, nil
}

func (m *MockWAClient) IsOnWhatsApp(ctx context.Context, phones []string) ([]types.IsOnWhatsAppResponse, error) {
	if m.IsOnWhatsAppFunc != nil {
		return m.IsOnWhatsAppFunc(ctx, phones)
	}
	return nil, nil
}

func (m *MockWAClient) GetJoinedGroups(ctx context.Context) ([]types.GroupInfo, error) {
	if m.GetJoinedGroupsFunc != nil {
		return m.GetJoinedGroupsFunc(ctx)
	}
	return nil, nil
}

func (m *MockWAClient) GetGroupInfo(ctx context.Context, jid string) (*types.GroupInfo, error) {
	if m.GetGroupInfoFunc != nil {
		return m.GetGroupInfoFunc(ctx, jid)
	}
	return nil, nil
}

func (m *MockWAClient) CreateGroup(ctx context.Context, name string, members []string) (*types.GroupInfo, error) {
	if m.CreateGroupFunc != nil {
		return m.CreateGroupFunc(ctx, name, members)
	}
	return nil, nil
}

func (m *MockWAClient) GetGroupInviteLink(ctx context.Context, jid string, reset bool) (string, error) {
	if m.GetGroupInviteLinkFunc != nil {
		return m.GetGroupInviteLinkFunc(ctx, jid, reset)
	}
	return "", nil
}

func (m *MockWAClient) JoinGroupWithLink(ctx context.Context, link string) (string, error) {
	if m.JoinGroupWithLinkFunc != nil {
		return m.JoinGroupWithLinkFunc(ctx, link)
	}
	return "", nil
}

func (m *MockWAClient) LeaveGroup(ctx context.Context, jid string) error {
	if m.LeaveGroupFunc != nil {
		return m.LeaveGroupFunc(ctx, jid)
	}
	return nil
}

func (m *MockWAClient) UpdateGroupParticipants(ctx context.Context, jid string, members []string, action string) error {
	if m.UpdateGroupParticipantsFunc != nil {
		return m.UpdateGroupParticipantsFunc(ctx, jid, members, action)
	}
	return nil
}

func (m *MockWAClient) SetGroupName(ctx context.Context, jid, name string) error {
	if m.SetGroupNameFunc != nil {
		return m.SetGroupNameFunc(ctx, jid, name)
	}
	return nil
}

func (m *MockWAClient) SetGroupDescription(ctx context.Context, jid, description string) error {
	if m.SetGroupDescriptionFunc != nil {
		return m.SetGroupDescriptionFunc(ctx, jid, description)
	}
	return nil
}

func (m *MockWAClient) SetGroupPhoto(ctx context.Context, jid, imagePath string) error {
	if m.SetGroupPhotoFunc != nil {
		return m.SetGroupPhotoFunc(ctx, jid, imagePath)
	}
	return nil
}
