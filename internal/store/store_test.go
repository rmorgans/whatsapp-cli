package store

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestDB(t *testing.T) *MessageStore {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := NewMessageStore(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { store.Close() })

	return store
}

func TestNewMessageStore(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := NewMessageStore(dbPath)
	require.NoError(t, err)
	defer store.Close()

	// Verify database file was created
	_, err = os.Stat(dbPath)
	assert.NoError(t, err)
}

func TestStoreChat(t *testing.T) {
	store := setupTestDB(t)

	err := store.StoreChat("1234@s.whatsapp.net", "John Doe", time.Now())
	assert.NoError(t, err)
}

func TestStoreChatDoesNotOverwriteFriendlyWithJID(t *testing.T) {
	store := setupTestDB(t)
	jid := "1234@s.whatsapp.net"

	require.NoError(t, store.StoreChat(jid, "John Doe", time.Now()))
	require.NoError(t, store.StoreChat(jid, jid, time.Now().Add(time.Minute)))

	chats, err := store.ListChats(ListChatsParams{Limit: 1})
	require.NoError(t, err)
	require.NotEmpty(t, chats)
	assert.Equal(t, "John Doe", chats[0].Name)
}

func TestStoreChatUpgradesNameFromJID(t *testing.T) {
	store := setupTestDB(t)
	jid := "5678@s.whatsapp.net"

	require.NoError(t, store.StoreChat(jid, jid, time.Now()))
	require.NoError(t, store.StoreChat(jid, "Jane Smith", time.Now().Add(time.Minute)))

	chats, err := store.ListChats(ListChatsParams{Limit: 1})
	require.NoError(t, err)
	require.NotEmpty(t, chats)
	assert.Equal(t, "Jane Smith", chats[0].Name)
}

func TestStoreMessage(t *testing.T) {
	store := setupTestDB(t)

	// First store a chat
	chatJID := "1234@s.whatsapp.net"
	err := store.StoreChat(chatJID, "John Doe", time.Now())
	require.NoError(t, err)

	// Then store a message
	err = store.StoreMessage(StoreMessageParams{ID: "msg1", ChatJID: chatJID, Sender: "1234", Content: "Hello", Timestamp: time.Now()})
	assert.NoError(t, err)
}

func TestListMessages(t *testing.T) {
	store := setupTestDB(t)
	chatJID := "1234@s.whatsapp.net"

	// Setup test data
	store.StoreChat(chatJID, "John Doe", time.Now())
	now := time.Now()
	store.StoreMessage(StoreMessageParams{ID: "msg1", ChatJID: chatJID, Sender: "1234", Content: "Hello", Timestamp: now})
	store.StoreMessage(StoreMessageParams{ID: "msg2", ChatJID: chatJID, Sender: "1234", Content: "World", Timestamp: now.Add(time.Second)})

	messages, err := store.ListMessages(ListMessagesParams{ChatJID: &chatJID, Limit: 10})
	require.NoError(t, err)
	assert.Len(t, messages, 2)
	assert.Equal(t, "World", messages[0].Content) // Most recent first
	assert.Equal(t, "Hello", messages[1].Content)
}

func TestGetMessageForDownload(t *testing.T) {
	store := setupTestDB(t)
	chatJID := "1234@s.whatsapp.net"

	require.NoError(t, store.StoreChat(chatJID, "John Doe", time.Now()))

	now := time.Now().UTC().Truncate(time.Second)
	mediaKey := []byte{1, 2, 3}
	fileSHA := []byte{4, 5, 6}
	fileEncSHA := []byte{7, 8, 9}

	err := store.StoreMessage(StoreMessageParams{
		ID:            "msg1",
		ChatJID:       chatJID,
		Sender:        "1234",
		Content:       "Sample caption",
		Timestamp:     now,
		MediaType:     "image",
		Filename:      "photo.jpg",
		URL:           "https://example.com/image",
		DirectPath:    "/media/direct/path",
		MimeType:      "image/jpeg",
		MediaKey:      mediaKey,
		FileSHA256:    fileSHA,
		FileEncSHA256: fileEncSHA,
		FileLength:    1024,
	})
	require.NoError(t, err)

	info, err := store.GetMessageForDownload("msg1", nil)
	require.NoError(t, err)

	assert.Equal(t, "msg1", info.ID)
	assert.Equal(t, chatJID, info.ChatJID)
	assert.Equal(t, "image", info.MediaType)
	assert.Equal(t, "photo.jpg", info.Filename)
	assert.Equal(t, "/media/direct/path", info.DirectPath)
	assert.Equal(t, "image/jpeg", info.MimeType)
	assert.Equal(t, uint64(1024), info.FileLength)
	assert.Equal(t, mediaKey, info.MediaKey)
	assert.Equal(t, fileSHA, info.FileSHA256)
	assert.Equal(t, fileEncSHA, info.FileEncSHA256)
	assert.Nil(t, info.LocalPath)

	err = store.MarkMediaDownloaded("msg1", chatJID, "/tmp/photo.jpg", now.Add(time.Minute))
	require.NoError(t, err)

	infoAfter, err := store.GetMessageForDownload("msg1", nil)
	require.NoError(t, err)

	require.NotNil(t, infoAfter.LocalPath)
	assert.Equal(t, "/tmp/photo.jpg", *infoAfter.LocalPath)
	require.NotNil(t, infoAfter.DownloadedAt)
	assert.True(t, infoAfter.DownloadedAt.Equal(now.Add(time.Minute)))
}

func TestSearchContacts(t *testing.T) {
	store := setupTestDB(t)

	// Setup test data
	store.StoreChat("1234@s.whatsapp.net", "John Doe", time.Now())
	store.StoreChat("5678@s.whatsapp.net", "Jane Smith", time.Now())
	store.StoreChat("9999@g.us", "Group Chat", time.Now()) // Should be excluded

	contacts, err := store.SearchContacts("John")
	require.NoError(t, err)
	assert.Len(t, contacts, 1)
	assert.Equal(t, "John Doe", contacts[0].Name)
}

func TestGetMessageMetadata(t *testing.T) {
	s := setupTestDB(t)
	chatJID := "1234@s.whatsapp.net"
	now := time.Now().UTC().Truncate(time.Second)

	require.NoError(t, s.StoreChat(chatJID, "John Doe", now))
	require.NoError(t, s.StoreMessage(StoreMessageParams{ID: "msg1", ChatJID: chatJID, Sender: "1234", Content: "Hello", Timestamp: now}))

	msg, err := s.GetMessageMetadata("msg1", nil)
	require.NoError(t, err)
	assert.Equal(t, "msg1", msg.ID)
	assert.Equal(t, chatJID, msg.ChatJID)
	assert.Equal(t, "John Doe", msg.ChatName)
	assert.Equal(t, "1234", msg.Sender)
	assert.Equal(t, "Hello", msg.Content)
	assert.True(t, msg.Timestamp.Equal(now))
	assert.False(t, msg.IsFromMe)
}

func TestGetMessageMetadataWithChatJID(t *testing.T) {
	s := setupTestDB(t)
	chatJID := "1234@s.whatsapp.net"
	now := time.Now().UTC().Truncate(time.Second)

	require.NoError(t, s.StoreChat(chatJID, "John Doe", now))
	require.NoError(t, s.StoreMessage(StoreMessageParams{ID: "msg1", ChatJID: chatJID, Sender: "1234", Content: "Hello", Timestamp: now}))

	msg, err := s.GetMessageMetadata("msg1", &chatJID)
	require.NoError(t, err)
	assert.Equal(t, "msg1", msg.ID)
	assert.Equal(t, chatJID, msg.ChatJID)
}

func TestGetMessageMetadataNotFound(t *testing.T) {
	s := setupTestDB(t)

	_, err := s.GetMessageMetadata("nonexistent", nil)
	assert.ErrorIs(t, err, sql.ErrNoRows)
}

func TestGetMessageMetadataMultipleChatsNoFilter(t *testing.T) {
	s := setupTestDB(t)
	chat1 := "1234@s.whatsapp.net"
	chat2 := "5678@s.whatsapp.net"
	now := time.Now().UTC().Truncate(time.Second)

	require.NoError(t, s.StoreChat(chat1, "John", now))
	require.NoError(t, s.StoreChat(chat2, "Jane", now))
	require.NoError(t, s.StoreMessage(StoreMessageParams{ID: "msg1", ChatJID: chat1, Sender: "1234", Content: "Hello", Timestamp: now}))
	require.NoError(t, s.StoreMessage(StoreMessageParams{ID: "msg1", ChatJID: chat2, Sender: "5678", Content: "Hi", Timestamp: now}))

	_, err := s.GetMessageMetadata("msg1", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "multiple messages found with ID msg1")
}

func TestGetMessageMetadataMultipleChatsWithFilter(t *testing.T) {
	s := setupTestDB(t)
	chat1 := "1234@s.whatsapp.net"
	chat2 := "5678@s.whatsapp.net"
	now := time.Now().UTC().Truncate(time.Second)

	require.NoError(t, s.StoreChat(chat1, "John", now))
	require.NoError(t, s.StoreChat(chat2, "Jane", now))
	require.NoError(t, s.StoreMessage(StoreMessageParams{ID: "msg1", ChatJID: chat1, Sender: "1234", Content: "Hello", Timestamp: now}))
	require.NoError(t, s.StoreMessage(StoreMessageParams{ID: "msg1", ChatJID: chat2, Sender: "5678", Content: "Hi", Timestamp: now}))

	msg, err := s.GetMessageMetadata("msg1", &chat2)
	require.NoError(t, err)
	assert.Equal(t, chat2, msg.ChatJID)
	assert.Equal(t, "5678", msg.Sender)
	assert.Equal(t, "Hi", msg.Content)
}

func TestListChats(t *testing.T) {
	store := setupTestDB(t)

	// Setup test data
	store.StoreChat("1234@s.whatsapp.net", "John Doe", time.Now())
	store.StoreChat("5678@s.whatsapp.net", "Jane Smith", time.Now().Add(-time.Hour))

	chats, err := store.ListChats(ListChatsParams{Limit: 10})
	require.NoError(t, err)
	assert.Len(t, chats, 2)
	assert.Equal(t, "John Doe", chats[0].Name) // Most recent first
}
