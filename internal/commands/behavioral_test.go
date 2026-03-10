package commands

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/vicentereig/whatsapp-cli/internal/store"
)

// Helper to create string pointer
func ptr(s string) *string {
	return &s
}

// Response is the standard JSON response format
type Response struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Error   *string         `json:"error"`
}

func parseResponse(t *testing.T, result string) Response {
	t.Helper()
	var resp Response
	err := json.Unmarshal([]byte(result), &resp)
	require.NoError(t, err, "response should be valid JSON: %s", result)
	return resp
}

// TestAuth_AlreadyAuthenticated verifies that Auth returns immediately when already authenticated.
func TestAuth_AlreadyAuthenticated(t *testing.T) {
	mockClient := &MockWAClient{
		IsAuthenticatedFunc: func() bool { return true },
	}

	app := NewAppWithDeps(mockClient, &MockMessageStore{}, "/tmp", "test")

	result, err := app.Auth(context.Background())
	require.NoError(t, err)
	require.True(t, result.Authenticated)
	require.Equal(t, "Already authenticated", result.Message)
}

// TestAuth_SuccessfulAuthentication verifies that Auth authenticates and returns success.
func TestAuth_SuccessfulAuthentication(t *testing.T) {
	mockClient := &MockWAClient{
		IsAuthenticatedFunc: func() bool { return false },
		AuthenticateFunc:    func(ctx context.Context) error { return nil },
	}

	app := NewAppWithDeps(mockClient, &MockMessageStore{}, "/tmp", "test")

	result, err := app.Auth(context.Background())
	require.NoError(t, err)
	require.True(t, result.Authenticated)
	require.Equal(t, "Successfully authenticated", result.Message)
}

// TestAuth_AuthenticationFails verifies that Auth returns an error when authentication fails.
func TestAuth_AuthenticationFails(t *testing.T) {
	mockClient := &MockWAClient{
		IsAuthenticatedFunc: func() bool { return false },
		AuthenticateFunc:    func(ctx context.Context) error { return fmt.Errorf("QR code expired") },
	}

	app := NewAppWithDeps(mockClient, &MockMessageStore{}, "/tmp", "test")

	_, err := app.Auth(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "QR code expired")
}

// TestSync_StartSyncFails verifies that Sync returns an error when StartSync fails.
func TestSync_StartSyncFails(t *testing.T) {
	mockClient := &MockWAClient{
		StartSyncFunc: func(ctx context.Context, handler func(interface{})) error {
			return fmt.Errorf("connection refused")
		},
	}

	app := NewAppWithDeps(mockClient, &MockMessageStore{}, "/tmp", "test")

	_, err := app.Sync(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "connection refused")
}

// TestSync_ReturnsTypedResult verifies that Sync returns a SyncResult after context cancellation.
func TestSync_ReturnsTypedResult(t *testing.T) {
	mockClient := &MockWAClient{
		StartSyncFunc: func(ctx context.Context, handler func(interface{})) error {
			return nil
		},
	}

	app := NewAppWithDeps(mockClient, &MockMessageStore{}, "/tmp", "test")

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel immediately so Sync doesn't block
	cancel()

	result, err := app.Sync(ctx)
	require.NoError(t, err)
	require.True(t, result.Synced)
	require.Equal(t, int64(0), result.MessagesCount)
}

// TestListMessages_FiltersByChat verifies that --chat flag filters messages correctly.
// This is the bug we fixed in PR #8.
func TestListMessages_FiltersByChat(t *testing.T) {
	targetJID := "target@s.whatsapp.net"
	otherJID := "other@s.whatsapp.net"

	allMessages := []store.Message{
		{ID: "1", ChatJID: targetJID, Content: "hello from target", Timestamp: time.Now()},
		{ID: "2", ChatJID: otherJID, Content: "hello from other", Timestamp: time.Now()},
		{ID: "3", ChatJID: targetJID, Content: "another from target", Timestamp: time.Now()},
	}

	mockStore := &MockMessageStore{
		ListMessagesFunc: func(params store.ListMessagesParams) ([]store.Message, error) {
			// Simulate filtering behavior
			if params.ChatJID != nil {
				var filtered []store.Message
				for _, m := range allMessages {
					if m.ChatJID == *params.ChatJID {
						filtered = append(filtered, m)
					}
				}
				return filtered, nil
			}
			return allMessages, nil
		},
	}

	app := NewAppWithDeps(&MockWAClient{}, mockStore, "/tmp", "test")

	// When: ListMessages called with chat filter
	messages, err := app.ListMessages(ptr(targetJID), nil, 10, 0)
	require.NoError(t, err)

	// Then: Only messages from target chat are returned
	require.Len(t, messages, 2, "should return only 2 messages from target chat")

	for _, m := range messages {
		require.Equal(t, targetJID, m.ChatJID, "all messages should be from target chat")
	}
}

// TestListMessages_RespectsLimit verifies that --limit flag is honored.
// Tests behavior (output count) not implementation (param passing).
func TestListMessages_RespectsLimit(t *testing.T) {
	allMessages := []store.Message{
		{ID: "1", Content: "msg1", Timestamp: time.Now()},
		{ID: "2", Content: "msg2", Timestamp: time.Now()},
		{ID: "3", Content: "msg3", Timestamp: time.Now()},
		{ID: "4", Content: "msg4", Timestamp: time.Now()},
		{ID: "5", Content: "msg5", Timestamp: time.Now()},
	}

	mockStore := &MockMessageStore{
		ListMessagesFunc: func(params store.ListMessagesParams) ([]store.Message, error) {
			// Simulate limit behavior - this is what the real store does
			if params.Limit > 0 && params.Limit < len(allMessages) {
				return allMessages[:params.Limit], nil
			}
			return allMessages, nil
		},
	}

	app := NewAppWithDeps(&MockWAClient{}, mockStore, "/tmp", "test")

	// When: ListMessages called with limit=2
	messages, err := app.ListMessages(nil, nil, 2, 0)
	require.NoError(t, err)

	// Then: Only 2 messages returned (behavioral - tests output, not internals)
	require.Len(t, messages, 2, "should return only 2 messages")
}

// TestDownloadMedia_Errors uses table-driven tests for error cases.
// Per Go best practice: group related test cases in tables.
func TestDownloadMedia_Errors(t *testing.T) {
	tests := []struct {
		name        string
		messageID   string
		mockStore   *MockMessageStore
		wantContain string
	}{
		{
			name:      "missing message returns not found error",
			messageID: "nonexistent123",
			mockStore: &MockMessageStore{
				GetMessageForDownloadFunc: func(id string, chatJID *string) (store.MessageDownloadInfo, error) {
					return store.MessageDownloadInfo{}, sql.ErrNoRows
				},
			},
			wantContain: "not found",
		},
		{
			name:        "empty message ID returns required error",
			messageID:   "",
			mockStore:   &MockMessageStore{},
			wantContain: "required",
		},
		{
			name:      "message without media returns no media error",
			messageID: "textonly123",
			mockStore: &MockMessageStore{
				GetMessageForDownloadFunc: func(id string, chatJID *string) (store.MessageDownloadInfo, error) {
					return store.MessageDownloadInfo{
						ID:      id,
						ChatJID: "chat@jid",
						// No MediaType, DirectPath, or MediaKey
					}, nil
				},
			},
			wantContain: "no downloadable media",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := NewAppWithDeps(&MockWAClient{}, tt.mockStore, "/tmp", "test")

			_, err := app.DownloadMedia(context.Background(), tt.messageID, nil, "")
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.wantContain)
		})
	}
}

// TestSearchContacts_ReturnsResults verifies contact search works.
func TestSearchContacts_ReturnsResults(t *testing.T) {
	mockStore := &MockMessageStore{
		SearchContactsFunc: func(query string) ([]store.Contact, error) {
			if query == "john" {
				return []store.Contact{
					{Name: "John Doe", PhoneNumber: "1234567890", JID: "1234567890@s.whatsapp.net"},
				}, nil
			}
			return nil, nil
		},
	}

	app := NewAppWithDeps(&MockWAClient{}, mockStore, "/tmp", "test")

	// When: SearchContacts called with query
	contacts, err := app.SearchContacts("john")
	require.NoError(t, err)

	// Then: Returns matching contacts
	require.Len(t, contacts, 1)
	require.Equal(t, "John Doe", contacts[0].Name)
}

// TestSendReply_Success verifies that SendReply looks up metadata and calls SendTextReply.
func TestSendReply_Success(t *testing.T) {
	var capturedReplyToID, capturedReplyToSender, capturedMessage string

	mockClient := &MockWAClient{
		SendTextReplyFunc: func(ctx context.Context, recipient, message, replyToID, replyToSender string) (string, error) {
			capturedMessage = message
			capturedReplyToID = replyToID
			capturedReplyToSender = replyToSender
			return "reply-msg-id", nil
		},
	}
	mockStore := &MockMessageStore{
		GetMessageMetadataFunc: func(id string, chatJID *string) (store.Message, error) {
			require.Equal(t, "orig-msg-123", id)
			return store.Message{
				ID:      "orig-msg-123",
				ChatJID: "chat@s.whatsapp.net",
				Sender:  "5511999999999",
			}, nil
		},
	}

	app := NewAppWithDeps(mockClient, mockStore, "/tmp", "test")

	result, err := app.SendReply(context.Background(), "5511999999999", "my reply text", "orig-msg-123")
	require.NoError(t, err)
	require.True(t, result.Sent)
	require.Equal(t, "reply-msg-id", result.ID)
	require.Equal(t, "orig-msg-123", result.ReplyTo)
	require.Equal(t, "my reply text", result.Message)

	// Verify the client received correct reply context
	require.Equal(t, "my reply text", capturedMessage)
	require.Equal(t, "orig-msg-123", capturedReplyToID)
	require.Equal(t, "5511999999999@s.whatsapp.net", capturedReplyToSender)
}

// TestSendReply_MetadataLookupFails verifies error when reply-to message is not found.
func TestSendReply_MetadataLookupFails(t *testing.T) {
	mockClient := &MockWAClient{}
	mockStore := &MockMessageStore{
		GetMessageMetadataFunc: func(id string, chatJID *string) (store.Message, error) {
			return store.Message{}, sql.ErrNoRows
		},
	}

	app := NewAppWithDeps(mockClient, mockStore, "/tmp", "test")

	_, err := app.SendReply(context.Background(), "5511999999999", "reply text", "nonexistent-id")
	require.Error(t, err)
	require.Contains(t, err.Error(), "looking up reply-to message")
}

// TestListChats_ReturnsChats verifies chat listing works.
func TestListChats_ReturnsChats(t *testing.T) {
	mockStore := &MockMessageStore{
		ListChatsFunc: func(params store.ListChatsParams) ([]store.Chat, error) {
			return []store.Chat{
				{JID: "chat1@jid", Name: "Chat One", LastMessageTime: time.Now()},
				{JID: "chat2@jid", Name: "Chat Two", LastMessageTime: time.Now()},
			}, nil
		},
	}

	app := NewAppWithDeps(&MockWAClient{}, mockStore, "/tmp", "test")

	// When: ListChats called
	chats, err := app.ListChats(nil, 10, 0)
	require.NoError(t, err)

	// Then: Returns chats
	require.Len(t, chats, 2)
}

// TestReactToMessage_Success verifies that reacting to a message sends the emoji.
func TestReactToMessage_Success(t *testing.T) {
	var capturedChatJID, capturedSenderJID, capturedMessageID, capturedEmoji string

	mockClient := &MockWAClient{
		ReactToMessageFunc: func(ctx context.Context, chatJID, senderJID, messageID, emoji string) error {
			capturedChatJID = chatJID
			capturedSenderJID = senderJID
			capturedMessageID = messageID
			capturedEmoji = emoji
			return nil
		},
	}
	mockStore := &MockMessageStore{
		GetMessageMetadataFunc: func(id string, chatJID *string) (store.Message, error) {
			require.Equal(t, "msg-123", id)
			return store.Message{
				ID:      "msg-123",
				ChatJID: "chat@s.whatsapp.net",
				Sender:  "5511999999999",
			}, nil
		},
	}

	app := NewAppWithDeps(mockClient, mockStore, "/tmp", "test")

	result, err := app.ReactToMessage(context.Background(), "msg-123", "\xf0\x9f\x91\x8d", nil)
	require.NoError(t, err)
	require.True(t, result.Reacted)
	require.Equal(t, "msg-123", result.MessageID)
	require.Equal(t, "chat@s.whatsapp.net", result.ChatJID)

	// Verify the client received correct parameters
	require.Equal(t, "chat@s.whatsapp.net", capturedChatJID)
	require.Equal(t, "5511999999999@s.whatsapp.net", capturedSenderJID)
	require.Equal(t, "msg-123", capturedMessageID)
	require.Equal(t, "\xf0\x9f\x91\x8d", capturedEmoji)
}

// TestReactToMessage_RemoveReaction verifies that empty emoji removes a reaction.
func TestReactToMessage_RemoveReaction(t *testing.T) {
	var capturedEmoji string

	mockClient := &MockWAClient{
		ReactToMessageFunc: func(ctx context.Context, chatJID, senderJID, messageID, emoji string) error {
			capturedEmoji = emoji
			return nil
		},
	}
	mockStore := &MockMessageStore{
		GetMessageMetadataFunc: func(id string, chatJID *string) (store.Message, error) {
			return store.Message{
				ID:      "msg-123",
				ChatJID: "chat@s.whatsapp.net",
				Sender:  "5511999999999@s.whatsapp.net",
			}, nil
		},
	}

	app := NewAppWithDeps(mockClient, mockStore, "/tmp", "test")

	result, err := app.ReactToMessage(context.Background(), "msg-123", "", nil)
	require.NoError(t, err)
	require.False(t, result.Reacted)
	require.Equal(t, "removed", result.Action)
	require.Equal(t, "", capturedEmoji)
}

// TestReactToMessage_MessageNotFound verifies error when message is not found.
func TestReactToMessage_MessageNotFound(t *testing.T) {
	mockClient := &MockWAClient{}
	mockStore := &MockMessageStore{
		GetMessageMetadataFunc: func(id string, chatJID *string) (store.Message, error) {
			return store.Message{}, sql.ErrNoRows
		},
	}

	app := NewAppWithDeps(mockClient, mockStore, "/tmp", "test")

	_, err := app.ReactToMessage(context.Background(), "nonexistent-id", "\xf0\x9f\x91\x8d", nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not found")
}

// TestDeleteMessage_Success verifies that deleting a message looks up metadata and calls RevokeMessage.
func TestDeleteMessage_Success(t *testing.T) {
	var capturedChatJID, capturedSenderJID, capturedMessageID string

	mockClient := &MockWAClient{
		RevokeMessageFunc: func(ctx context.Context, chatJID, senderJID, messageID string) error {
			capturedChatJID = chatJID
			capturedSenderJID = senderJID
			capturedMessageID = messageID
			return nil
		},
	}
	mockStore := &MockMessageStore{
		GetMessageMetadataFunc: func(id string, chatJID *string) (store.Message, error) {
			require.Equal(t, "msg-456", id)
			return store.Message{
				ID:      "msg-456",
				ChatJID: "chat@s.whatsapp.net",
				Sender:  "5511999999999",
			}, nil
		},
	}

	app := NewAppWithDeps(mockClient, mockStore, "/tmp", "test")

	result, err := app.DeleteMessage(context.Background(), "msg-456", nil)
	require.NoError(t, err)
	require.True(t, result.Deleted)
	require.Equal(t, "msg-456", result.MessageID)
	require.Equal(t, "chat@s.whatsapp.net", result.ChatJID)

	require.Equal(t, "chat@s.whatsapp.net", capturedChatJID)
	require.Equal(t, "5511999999999@s.whatsapp.net", capturedSenderJID)
	require.Equal(t, "msg-456", capturedMessageID)
}

// TestDeleteMessage_NotFound verifies error when message is not in store.
func TestDeleteMessage_NotFound(t *testing.T) {
	mockClient := &MockWAClient{}
	mockStore := &MockMessageStore{
		GetMessageMetadataFunc: func(id string, chatJID *string) (store.Message, error) {
			return store.Message{}, sql.ErrNoRows
		},
	}

	app := NewAppWithDeps(mockClient, mockStore, "/tmp", "test")

	_, err := app.DeleteMessage(context.Background(), "nonexistent-id", nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not found")
}

// TestEditMessage_Success verifies that editing a message looks up metadata and calls EditMessage.
func TestEditMessage_Success(t *testing.T) {
	var capturedChatJID, capturedMessageID, capturedNewText string

	mockClient := &MockWAClient{
		EditMessageFunc: func(ctx context.Context, chatJID, messageID, newText string) error {
			capturedChatJID = chatJID
			capturedMessageID = messageID
			capturedNewText = newText
			return nil
		},
	}
	mockStore := &MockMessageStore{
		GetMessageMetadataFunc: func(id string, chatJID *string) (store.Message, error) {
			require.Equal(t, "msg-789", id)
			return store.Message{
				ID:      "msg-789",
				ChatJID: "group@g.us",
				Sender:  "5511999999999@s.whatsapp.net",
			}, nil
		},
	}

	app := NewAppWithDeps(mockClient, mockStore, "/tmp", "test")

	result, err := app.EditMessage(context.Background(), "msg-789", "updated text", nil)
	require.NoError(t, err)
	require.True(t, result.Edited)
	require.Equal(t, "msg-789", result.MessageID)
	require.Equal(t, "group@g.us", result.ChatJID)
	require.Equal(t, "updated text", result.NewText)

	require.Equal(t, "group@g.us", capturedChatJID)
	require.Equal(t, "msg-789", capturedMessageID)
	require.Equal(t, "updated text", capturedNewText)
}

// TestEditMessage_NotFound verifies error when message is not in store.
func TestEditMessage_NotFound(t *testing.T) {
	mockClient := &MockWAClient{}
	mockStore := &MockMessageStore{
		GetMessageMetadataFunc: func(id string, chatJID *string) (store.Message, error) {
			return store.Message{}, sql.ErrNoRows
		},
	}

	app := NewAppWithDeps(mockClient, mockStore, "/tmp", "test")

	_, err := app.EditMessage(context.Background(), "nonexistent-id", "new text", nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not found")
}

// TestMarkMessageRead_Success verifies that marking a message read looks up metadata and calls MarkRead.
func TestMarkMessageRead_Success(t *testing.T) {
	var capturedIDs []string
	var capturedTimestamp time.Time
	var capturedChatJID, capturedSenderJID string

	msgTime := time.Date(2026, 3, 9, 12, 0, 0, 0, time.UTC)

	mockClient := &MockWAClient{
		MarkReadFunc: func(ctx context.Context, messageIDs []string, timestamp time.Time, chatJID, senderJID string) error {
			capturedIDs = messageIDs
			capturedTimestamp = timestamp
			capturedChatJID = chatJID
			capturedSenderJID = senderJID
			return nil
		},
	}
	mockStore := &MockMessageStore{
		GetMessageMetadataFunc: func(id string, chatJID *string) (store.Message, error) {
			require.Equal(t, "msg-read-1", id)
			return store.Message{
				ID:        "msg-read-1",
				ChatJID:   "chat@s.whatsapp.net",
				Sender:    "5511888888888",
				Timestamp: msgTime,
			}, nil
		},
	}

	app := NewAppWithDeps(mockClient, mockStore, "/tmp", "test")

	result, err := app.MarkMessageRead(context.Background(), "msg-read-1", nil)
	require.NoError(t, err)
	require.True(t, result.MarkedRead)
	require.Equal(t, "msg-read-1", result.MessageID)
	require.Equal(t, "chat@s.whatsapp.net", result.ChatJID)

	require.Equal(t, []string{"msg-read-1"}, capturedIDs)
	require.Equal(t, msgTime, capturedTimestamp)
	require.Equal(t, "chat@s.whatsapp.net", capturedChatJID)
	require.Equal(t, "5511888888888@s.whatsapp.net", capturedSenderJID)
}

// TestMarkMessageRead_NotFound verifies error when message is not in store.
func TestMarkMessageRead_NotFound(t *testing.T) {
	mockClient := &MockWAClient{}
	mockStore := &MockMessageStore{
		GetMessageMetadataFunc: func(id string, chatJID *string) (store.Message, error) {
			return store.Message{}, sql.ErrNoRows
		},
	}

	app := NewAppWithDeps(mockClient, mockStore, "/tmp", "test")

	_, err := app.MarkMessageRead(context.Background(), "nonexistent-id", nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not found")
}
