package commands

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"mime"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/vicentereig/whatsapp-cli/internal/client"
	"github.com/vicentereig/whatsapp-cli/internal/store"
	"github.com/vicentereig/whatsapp-cli/internal/types"
	"go.mau.fi/whatsmeow/types/events"
)

type App struct {
	client          WAClient
	store           MessageStore
	version         string
	storeDir        string
	mediaDownloader func(ctx context.Context, info store.MessageDownloadInfo, targetPath string) (int64, error)
	mediaWorker     *mediaDownloadWorker
}

// NewApp creates a new App with production dependencies.
func NewApp(storeDir, version string) (*App, error) {
	cli, err := client.NewWAClient(storeDir)
	if err != nil {
		return nil, err
	}

	dbPath := filepath.Join(storeDir, "messages.db")
	st, err := store.NewMessageStore(dbPath)
	if err != nil {
		return nil, err
	}

	app := &App{
		client:   cli,
		store:    st,
		version:  resolveVersion(version, gitDescribe),
		storeDir: storeDir,
	}
	app.mediaDownloader = app.downloadMediaWithClient
	return app, nil
}

// NewAppWithDeps creates a new App with injected dependencies for testing.
func NewAppWithDeps(client WAClient, store MessageStore, storeDir, version string) *App {
	app := &App{
		client:   client,
		store:    store,
		version:  version,
		storeDir: storeDir,
	}
	return app
}

func (a *App) Close() {
	if a.mediaWorker != nil {
		a.mediaWorker.Stop()
	}
	if a.client != nil {
		a.client.Disconnect()
	}
	if a.store != nil {
		a.store.Close()
	}
}

func (a *App) Auth(ctx context.Context) (AuthResult, error) {
	if a.client.IsAuthenticated() {
		return AuthResult{Authenticated: true, Message: "Already authenticated"}, nil
	}

	if err := a.client.Authenticate(ctx); err != nil {
		return AuthResult{}, err
	}

	return AuthResult{Authenticated: true, Message: "Successfully authenticated"}, nil
}

func (a *App) ListMessages(chatJID *string, query *string, limit, page int) ([]store.Message, error) {
	messages, err := a.store.ListMessages(store.ListMessagesParams{
		ChatJID: chatJID,
		Query:   query,
		Limit:   limit,
		Page:    page,
	})
	if err != nil {
		return nil, err
	}
	if messages == nil {
		messages = []store.Message{}
	}

	return messages, nil
}

func (a *App) SearchContacts(query string) ([]store.Contact, error) {
	contacts, err := a.store.SearchContacts(query)
	if err != nil {
		return nil, err
	}
	if contacts == nil {
		contacts = []store.Contact{}
	}

	return contacts, nil
}

func (a *App) ListChats(query *string, limit, page int) ([]store.Chat, error) {
	chats, err := a.store.ListChats(store.ListChatsParams{
		Query: query,
		Limit: limit,
		Page:  page,
	})
	if err != nil {
		return nil, err
	}
	if chats == nil {
		chats = []store.Chat{}
	}

	return chats, nil
}

// recipientToJID normalizes a recipient string to a full JID.
func recipientToJID(recipient string) string {
	if strings.Contains(recipient, "@") {
		return recipient
	}
	return recipient + "@s.whatsapp.net"
}

// ownSender returns the device's JID for use as the sender field in stored
// messages, falling back to "me" if the JID is unavailable.
func (a *App) ownSender() string {
	if jid := a.client.GetOwnJID(); jid != "" {
		return jid
	}
	return "me"
}

func (a *App) SendMessage(ctx context.Context, recipient, message string) (SendResult, error) {
	if err := a.client.Connect(ctx); err != nil {
		return SendResult{}, err
	}

	msgID, err := a.client.SendMessage(ctx, recipient, message)
	if err != nil {
		return SendResult{}, err
	}

	timestamp := time.Now()
	chatJID := recipientToJID(recipient)

	chatName := a.client.ResolveChatName(ctx, chatJID, nil)
	if chatName == "" {
		chatName = recipient
	}

	if err := a.store.StoreChat(chatJID, chatName, timestamp); err != nil {
		return SendResult{}, fmt.Errorf("storing chat: %w", err)
	}
	if err := a.store.StoreMessage(store.StoreMessageParams{
		ID:        msgID,
		ChatJID:   chatJID,
		Sender:    a.ownSender(),
		Content:   message,
		Timestamp: timestamp,
		IsFromMe:  true,
	}); err != nil {
		return SendResult{}, fmt.Errorf("storing message: %w", err)
	}

	return SendResult{Sent: true, ID: msgID, Recipient: recipient, Message: message}, nil
}

func (a *App) SendReply(ctx context.Context, recipient, message, replyToID string) (SendResult, error) {
	if err := a.client.Connect(ctx); err != nil {
		return SendResult{}, err
	}

	// Look up the original message scoped to the destination chat so we don't
	// accidentally quote a message from a different conversation.
	chatJID := recipientToJID(recipient)
	meta, err := a.store.GetMessageMetadata(replyToID, &chatJID)
	if err != nil {
		return SendResult{}, fmt.Errorf("looking up reply-to message %s: %w", replyToID, err)
	}

	// The Participant field in ContextInfo needs a full JID.
	senderJID := recipientToJID(meta.Sender)

	msgID, err := a.client.SendTextReply(ctx, recipient, message, replyToID, senderJID)
	if err != nil {
		return SendResult{}, err
	}

	timestamp := time.Now()

	chatName := a.client.ResolveChatName(ctx, chatJID, nil)
	if chatName == "" {
		chatName = recipient
	}

	if err := a.store.StoreChat(chatJID, chatName, timestamp); err != nil {
		return SendResult{}, fmt.Errorf("storing chat: %w", err)
	}
	if err := a.store.StoreMessage(store.StoreMessageParams{
		ID:        msgID,
		ChatJID:   chatJID,
		Sender:    a.ownSender(),
		Content:   message,
		Timestamp: timestamp,
		IsFromMe:  true,
	}); err != nil {
		return SendResult{}, fmt.Errorf("storing message: %w", err)
	}

	return SendResult{Sent: true, ID: msgID, Recipient: recipient, Message: message, ReplyTo: replyToID}, nil
}

func (a *App) ReactToMessage(ctx context.Context, messageID, emoji string, chatJID *string) (ReactResult, error) {
	meta, err := a.store.GetMessageMetadata(messageID, chatJID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ReactResult{}, fmt.Errorf("message %s not found", messageID)
		}
		return ReactResult{}, err
	}

	if err := a.client.Connect(ctx); err != nil {
		return ReactResult{}, err
	}

	if err := a.client.ReactToMessage(ctx, meta.ChatJID, recipientToJID(meta.Sender), messageID, emoji); err != nil {
		return ReactResult{}, err
	}

	result := ReactResult{Reacted: true, MessageID: messageID, ChatJID: meta.ChatJID, Emoji: emoji}
	if emoji == "" {
		result.Reacted = false
		result.Action = "removed"
	}
	return result, nil
}

func (a *App) DeleteMessage(ctx context.Context, messageID string, chatJID *string) (MessageActionResult, error) {
	meta, err := a.store.GetMessageMetadata(messageID, chatJID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return MessageActionResult{}, fmt.Errorf("message %s not found", messageID)
		}
		return MessageActionResult{}, err
	}

	if err := a.client.Connect(ctx); err != nil {
		return MessageActionResult{}, err
	}

	if err := a.client.RevokeMessage(ctx, meta.ChatJID, recipientToJID(meta.Sender), messageID); err != nil {
		return MessageActionResult{}, err
	}

	return MessageActionResult{MessageID: messageID, ChatJID: meta.ChatJID, Deleted: true}, nil
}

func (a *App) EditMessage(ctx context.Context, messageID, newText string, chatJID *string) (MessageActionResult, error) {
	meta, err := a.store.GetMessageMetadata(messageID, chatJID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return MessageActionResult{}, fmt.Errorf("message %s not found", messageID)
		}
		return MessageActionResult{}, err
	}

	if err := a.client.Connect(ctx); err != nil {
		return MessageActionResult{}, err
	}

	if err := a.client.EditMessage(ctx, meta.ChatJID, messageID, newText); err != nil {
		return MessageActionResult{}, err
	}

	return MessageActionResult{MessageID: messageID, ChatJID: meta.ChatJID, Edited: true, NewText: newText}, nil
}

func (a *App) MarkMessageRead(ctx context.Context, messageID string, chatJID *string) (MessageActionResult, error) {
	meta, err := a.store.GetMessageMetadata(messageID, chatJID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return MessageActionResult{}, fmt.Errorf("message %s not found", messageID)
		}
		return MessageActionResult{}, err
	}

	if err := a.client.Connect(ctx); err != nil {
		return MessageActionResult{}, err
	}

	if err := a.client.MarkRead(ctx, []string{messageID}, meta.Timestamp, meta.ChatJID, recipientToJID(meta.Sender)); err != nil {
		return MessageActionResult{}, err
	}

	return MessageActionResult{MessageID: messageID, ChatJID: meta.ChatJID, MarkedRead: true}, nil
}

func (a *App) SendImage(ctx context.Context, recipient, imagePath, caption string) (SendResult, error) {
	if err := a.client.Connect(ctx); err != nil {
		return SendResult{}, err
	}

	msgID, err := a.client.SendImageMessage(ctx, recipient, imagePath, caption)
	if err != nil {
		return SendResult{}, err
	}

	timestamp := time.Now()
	chatJID := recipientToJID(recipient)

	chatName := a.client.ResolveChatName(ctx, chatJID, nil)
	if chatName == "" {
		chatName = recipient
	}

	content := caption
	if content == "" {
		content = "[Image]"
	}

	if err := a.store.StoreChat(chatJID, chatName, timestamp); err != nil {
		return SendResult{}, fmt.Errorf("storing chat: %w", err)
	}
	if err := a.store.StoreMessage(store.StoreMessageParams{
		ID:        msgID,
		ChatJID:   chatJID,
		Sender:    a.ownSender(),
		Content:   content,
		Timestamp: timestamp,
		IsFromMe:  true,
		MediaType: "image",
		Filename:  filepath.Base(imagePath),
	}); err != nil {
		return SendResult{}, fmt.Errorf("storing message: %w", err)
	}

	return SendResult{Sent: true, ID: msgID, Recipient: recipient, Image: imagePath, Caption: caption}, nil
}

func (a *App) SendVideo(ctx context.Context, recipient, videoPath, caption string) (SendResult, error) {
	if err := a.client.Connect(ctx); err != nil {
		return SendResult{}, err
	}

	msgID, err := a.client.SendVideoMessage(ctx, recipient, videoPath, caption)
	if err != nil {
		return SendResult{}, err
	}

	timestamp := time.Now()
	chatJID := recipientToJID(recipient)

	chatName := a.client.ResolveChatName(ctx, chatJID, nil)
	if chatName == "" {
		chatName = recipient
	}

	content := caption
	if content == "" {
		content = "[Video]"
	}

	if err := a.store.StoreChat(chatJID, chatName, timestamp); err != nil {
		return SendResult{}, fmt.Errorf("storing chat: %w", err)
	}
	if err := a.store.StoreMessage(store.StoreMessageParams{
		ID:        msgID,
		ChatJID:   chatJID,
		Sender:    a.ownSender(),
		Content:   content,
		Timestamp: timestamp,
		IsFromMe:  true,
		MediaType: "video",
		Filename:  filepath.Base(videoPath),
	}); err != nil {
		return SendResult{}, fmt.Errorf("storing message: %w", err)
	}

	return SendResult{Sent: true, ID: msgID, Recipient: recipient, Video: videoPath, Caption: caption}, nil
}

func (a *App) SendAudio(ctx context.Context, recipient, audioPath string, ptt bool) (SendResult, error) {
	if err := a.client.Connect(ctx); err != nil {
		return SendResult{}, err
	}

	msgID, err := a.client.SendAudioMessage(ctx, recipient, audioPath, ptt)
	if err != nil {
		return SendResult{}, err
	}

	timestamp := time.Now()
	chatJID := recipientToJID(recipient)

	chatName := a.client.ResolveChatName(ctx, chatJID, nil)
	if chatName == "" {
		chatName = recipient
	}

	if err := a.store.StoreChat(chatJID, chatName, timestamp); err != nil {
		return SendResult{}, fmt.Errorf("storing chat: %w", err)
	}
	if err := a.store.StoreMessage(store.StoreMessageParams{
		ID:        msgID,
		ChatJID:   chatJID,
		Sender:    a.ownSender(),
		Content:   "[Audio]",
		Timestamp: timestamp,
		IsFromMe:  true,
		MediaType: "audio",
		Filename:  filepath.Base(audioPath),
	}); err != nil {
		return SendResult{}, fmt.Errorf("storing message: %w", err)
	}

	return SendResult{Sent: true, ID: msgID, Recipient: recipient, Audio: audioPath}, nil
}

func (a *App) SendDocument(ctx context.Context, recipient, docPath, filename string) (SendResult, error) {
	if err := a.client.Connect(ctx); err != nil {
		return SendResult{}, err
	}

	msgID, err := a.client.SendDocumentMessage(ctx, recipient, docPath, filename)
	if err != nil {
		return SendResult{}, err
	}

	timestamp := time.Now()
	chatJID := recipientToJID(recipient)

	chatName := a.client.ResolveChatName(ctx, chatJID, nil)
	if chatName == "" {
		chatName = recipient
	}

	// Use the effective filename (client falls back to filepath.Base if empty)
	displayName := filename
	if displayName == "" {
		displayName = filepath.Base(docPath)
	}

	if err := a.store.StoreChat(chatJID, chatName, timestamp); err != nil {
		return SendResult{}, fmt.Errorf("storing chat: %w", err)
	}
	if err := a.store.StoreMessage(store.StoreMessageParams{
		ID:        msgID,
		ChatJID:   chatJID,
		Sender:    a.ownSender(),
		Content:   displayName,
		Timestamp: timestamp,
		IsFromMe:  true,
		MediaType: "document",
		Filename:  displayName,
	}); err != nil {
		return SendResult{}, fmt.Errorf("storing message: %w", err)
	}

	return SendResult{Sent: true, ID: msgID, Recipient: recipient, Document: docPath, Filename: displayName}, nil
}

func (a *App) DownloadMedia(ctx context.Context, messageID string, chatJID *string, outputPath string) (MediaDownloadResult, error) {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return MediaDownloadResult{}, fmt.Errorf("message ID is required")
	}

	info, err := a.store.GetMessageForDownload(messageID, chatJID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return MediaDownloadResult{}, fmt.Errorf("message %s not found", messageID)
		}
		return MediaDownloadResult{}, err
	}

	if strings.TrimSpace(info.MediaType) == "" || strings.TrimSpace(info.DirectPath) == "" || len(info.MediaKey) == 0 {
		return MediaDownloadResult{}, fmt.Errorf("message %s has no downloadable media", messageID)
	}

	targetPath, bytesWritten, downloadedAt, err := a.downloadMediaAndPersist(ctx, info, outputPath)
	if err != nil {
		return MediaDownloadResult{}, err
	}

	result := MediaDownloadResult{
		MessageID:    messageID,
		ChatJID:      info.ChatJID,
		Path:         targetPath,
		Bytes:        bytesWritten,
		MediaType:    info.MediaType,
		MimeType:     info.MimeType,
		DownloadedAt: downloadedAt.Format(time.RFC3339Nano),
	}
	if info.ChatName != nil && *info.ChatName != "" {
		result.ChatName = *info.ChatName
	}
	return result, nil
}

func (a *App) resolveOutputPath(info store.MessageDownloadInfo, requested string) (string, error) {
	filename := sanitizeFilename(filenameFor(info))
	if filename == "" {
		filename = "file"
	}

	if strings.TrimSpace(requested) != "" {
		cleaned := requested
		if !filepath.IsAbs(cleaned) {
			if abs, err := filepath.Abs(cleaned); err == nil {
				cleaned = abs
			}
		}
		if info, err := os.Stat(cleaned); err == nil && info.IsDir() {
			return filepath.Join(cleaned, filename), nil
		}
		if strings.HasSuffix(cleaned, string(os.PathSeparator)) {
			return filepath.Join(cleaned, filename), nil
		}
		return cleaned, nil
	}

	baseDir := filepath.Join(a.storeDir, "media", sanitizeSegment(info.ChatJID), sanitizeSegment(info.ID))
	if info.MediaType != "" {
		baseDir = filepath.Join(baseDir, sanitizeSegment(info.MediaType))
	}
	if abs, err := filepath.Abs(baseDir); err == nil {
		baseDir = abs
	}
	return filepath.Join(baseDir, filename), nil
}

var pathReplacer = strings.NewReplacer(
	"/", "_",
	"\\", "_",
	":", "_",
	"@", "_",
	"?", "_",
	"*", "_",
	"<", "_",
	">", "_",
	"|", "_",
)

func sanitizeSegment(seg string) string {
	seg = strings.TrimSpace(seg)
	if seg == "" {
		return "unknown"
	}
	seg = pathReplacer.Replace(seg)
	seg = strings.ReplaceAll(seg, "..", "_")
	return seg
}

const maxFilenameLen = 200 // Leave room for directory path; most filesystems allow 255

func sanitizeFilename(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "file"
	}
	name = pathReplacer.Replace(name)
	name = strings.ReplaceAll(name, string(os.PathSeparator), "_")
	name = strings.ReplaceAll(name, "..", "_")
	// Truncate if too long (preserve extension if possible)
	if len(name) > maxFilenameLen {
		ext := filepath.Ext(name)
		if len(ext) < 20 && len(ext) > 0 {
			base := name[:maxFilenameLen-len(ext)]
			name = base + ext
		} else {
			name = name[:maxFilenameLen]
		}
	}
	return name
}

func filenameFor(info store.MessageDownloadInfo) string {
	if trimmed := strings.TrimSpace(info.Filename); trimmed != "" {
		return trimmed
	}
	if ext := extensionForMime(info.MimeType); ext != "" {
		return info.ID + ext
	}
	switch strings.ToLower(strings.TrimSpace(info.MediaType)) {
	case "image":
		return info.ID + ".jpg"
	case "video":
		return info.ID + ".mp4"
	case "audio":
		return info.ID + ".ogg"
	case "document":
		return info.ID
	default:
		return info.ID
	}
}

func extensionForMime(mimeType string) string {
	mimeType = strings.ToLower(strings.TrimSpace(mimeType))
	if mimeType == "" {
		return ""
	}
	if exts, err := mime.ExtensionsByType(mimeType); err == nil {
		for _, ext := range exts {
			switch ext {
			case ".jpe":
				return ".jpg"
			default:
				if ext != "" {
					return ext
				}
			}
		}
	}
	switch mimeType {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	case "video/mp4":
		return ".mp4"
	case "audio/ogg":
		return ".ogg"
	case "audio/mpeg":
		return ".mp3"
	case "application/pdf":
		return ".pdf"
	default:
		return ""
	}
}

func (a *App) downloadMediaWithClient(ctx context.Context, info store.MessageDownloadInfo, targetPath string) (int64, error) {
	if a.client == nil {
		return 0, fmt.Errorf("whatsapp client not initialized")
	}
	if err := a.client.Connect(ctx); err != nil {
		return 0, err
	}
	req := types.MediaDownloadRequest{
		DirectPath:    info.DirectPath,
		MediaKey:      info.MediaKey,
		FileSHA256:    info.FileSHA256,
		FileEncSHA256: info.FileEncSHA256,
		FileLength:    info.FileLength,
		MediaType:     info.MediaType,
		MimeType:      info.MimeType,
	}
	return a.client.DownloadMediaToFile(ctx, req, targetPath)
}

func (a *App) downloadMediaAndPersist(ctx context.Context, info store.MessageDownloadInfo, requestedPath string) (string, int64, time.Time, error) {
	finalPath, err := a.resolveOutputPath(info, requestedPath)
	if err != nil {
		return "", 0, time.Time{}, err
	}
	if err := os.MkdirAll(filepath.Dir(finalPath), 0755); err != nil {
		return "", 0, time.Time{}, fmt.Errorf("failed to create destination directory: %w", err)
	}

	downloader := a.mediaDownloader
	if downloader == nil {
		downloader = a.downloadMediaWithClient
	}

	bytesWritten, err := downloader(ctx, info, finalPath)
	if err != nil {
		return "", 0, time.Time{}, err
	}

	now := time.Now().UTC()
	if err := a.store.MarkMediaDownloaded(info.ID, info.ChatJID, finalPath, now); err != nil {
		return "", 0, time.Time{}, fmt.Errorf("failed to mark media downloaded: %w", err)
	}

	return finalPath, bytesWritten, now, nil
}

func (a *App) processMediaJob(ctx context.Context, job mediaJob) error {
	if a.store == nil {
		return fmt.Errorf("message store not initialized")
	}
	info, err := a.store.GetMessageForDownload(job.messageID, &job.chatJID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	}
	if strings.TrimSpace(info.DirectPath) == "" || len(info.MediaKey) == 0 {
		return nil
	}
	if info.LocalPath != nil {
		if _, err := os.Stat(*info.LocalPath); err == nil {
			return nil
		}
	}
	_, _, _, err = a.downloadMediaAndPersist(ctx, info, "")
	return err
}

type mediaJob struct {
	messageID string
	chatJID   string
}

type mediaDownloadWorker struct {
	app       *App
	workers   int
	jobs      chan mediaJob
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	startOnce sync.Once

	// Error tracking
	mu             sync.Mutex
	expiredCount   int // 403/404/410 errors (media expired/deleted)
	droppedCount   int // jobs dropped due to full queue
	otherErrors    int
	otherErrorMsgs []string // Keep first few for debugging
}

func newMediaDownloadWorker(app *App, workers int) *mediaDownloadWorker {
	if workers <= 0 {
		workers = 2
	}
	return &mediaDownloadWorker{
		app:     app,
		workers: workers,
		jobs:    make(chan mediaJob, workers*4),
	}
}

func (w *mediaDownloadWorker) Start(ctx context.Context) {
	if w == nil {
		return
	}
	called := false
	w.startOnce.Do(func() {
		called = true
		w.ctx, w.cancel = context.WithCancel(ctx)
		for i := 0; i < w.workers; i++ {
			w.wg.Add(1)
			go w.run()
		}
	})
	if !called {
		panic("mediaDownloadWorker: Start called twice")
	}
}

func (w *mediaDownloadWorker) run() {
	defer w.wg.Done()
	for {
		select {
		case <-w.ctx.Done():
			return
		case job := <-w.jobs:
			if err := w.app.processMediaJob(w.ctx, job); err != nil {
				w.trackError(err)
			}
		}
	}
}

func (w *mediaDownloadWorker) trackError(err error) {
	errStr := err.Error()
	// Check for expected expired/deleted media errors
	isExpired := strings.Contains(errStr, "status code 403") ||
		strings.Contains(errStr, "status code 404") ||
		strings.Contains(errStr, "status code 410")

	w.mu.Lock()
	defer w.mu.Unlock()

	if isExpired {
		w.expiredCount++
	} else {
		w.otherErrors++
		// Keep first 5 other errors for debugging
		if len(w.otherErrorMsgs) < 5 {
			w.otherErrorMsgs = append(w.otherErrorMsgs, errStr)
		}
	}
}

func (w *mediaDownloadWorker) PrintSummary() {
	if w == nil {
		return
	}
	w.mu.Lock()
	expiredCount := w.expiredCount
	droppedCount := w.droppedCount
	otherErrors := w.otherErrors
	otherErrorMsgs := w.otherErrorMsgs
	w.mu.Unlock()

	if expiredCount > 0 {
		fmt.Fprintf(os.Stderr, "⚠️  Skipped %d expired/deleted media files (normal for old messages)\n", expiredCount)
	}
	if droppedCount > 0 {
		fmt.Fprintf(os.Stderr, "⚠️  Dropped %d media downloads (queue full)\n", droppedCount)
	}
	if otherErrors > 0 {
		fmt.Fprintf(os.Stderr, "⚠️  %d media downloads failed:\n", otherErrors)
		for _, msg := range otherErrorMsgs {
			fmt.Fprintf(os.Stderr, "   - %s\n", msg)
		}
		if otherErrors > len(otherErrorMsgs) {
			fmt.Fprintf(os.Stderr, "   ... and %d more\n", otherErrors-len(otherErrorMsgs))
		}
	}
}

func (w *mediaDownloadWorker) Enqueue(job mediaJob) {
	if w == nil || w.ctx == nil {
		return
	}
	select {
	case w.jobs <- job:
	case <-w.ctx.Done():
	default:
		w.mu.Lock()
		w.droppedCount++
		w.mu.Unlock()
	}
}

func (w *mediaDownloadWorker) Stop() {
	if w == nil {
		return
	}
	if w.cancel != nil {
		w.cancel()
	}
	w.wg.Wait()
}

// Sync connects to WhatsApp and continuously syncs messages to the database
func (a *App) Sync(ctx context.Context) (SyncResult, error) {
	var messageCount atomic.Int64

	version := a.version
	if strings.TrimSpace(version) == "" {
		version = "unknown"
	}
	fmt.Fprintf(os.Stderr, "ℹ️  whatsapp-cli version: %s\n", version)

	worker := newMediaDownloadWorker(a, 4)
	worker.Start(ctx)
	a.mediaWorker = worker
	defer func() {
		worker.Stop()
		worker.PrintSummary()
		if a.mediaWorker == worker {
			a.mediaWorker = nil
		}
	}()

	// Create event handler
	eventHandler := func(evt interface{}) {
		switch v := evt.(type) {
		case *events.Message:
			// Extract message details
			details := a.client.HandleMessage(ctx, v)
			id := details.ID
			chatJID := details.ChatJID
			sender := details.Sender
			content := details.Content
			msgTime := details.Timestamp
			isFromMe := details.IsFromMe
			mediaType := ""
			filename := ""
			url := ""
			directPath := ""
			mimeType := ""
			var mediaKey, fileSHA256, fileEncSHA256 []byte
			var fileLength uint64

			if details.Media != nil {
				mediaType = details.Media.Type
				filename = details.Media.Filename
				url = details.Media.URL
				directPath = details.Media.DirectPath
				mimeType = details.Media.MimeType
				mediaKey = details.Media.MediaKey
				fileSHA256 = details.Media.FileSHA256
				fileEncSHA256 = details.Media.FileEncSHA256
				fileLength = details.Media.FileLength
			}

			chatName := a.client.ResolveChatName(ctx, chatJID, v)
			if chatName == "" && chatJID != "" {
				chatName = chatJID
			}

			if err := a.store.StoreChat(chatJID, chatName, msgTime); err != nil {
				fmt.Fprintf(os.Stderr, "\n⚠ failed to store chat %s: %v\n", chatJID, err)
			}

			if err := a.store.StoreMessage(store.StoreMessageParams{
				ID:            id,
				ChatJID:       chatJID,
				Sender:        sender,
				Content:       content,
				Timestamp:     msgTime,
				IsFromMe:      isFromMe,
				MediaType:     mediaType,
				Filename:      filename,
				URL:           url,
				DirectPath:    directPath,
				MimeType:      mimeType,
				MediaKey:      mediaKey,
				FileSHA256:    fileSHA256,
				FileEncSHA256: fileEncSHA256,
				FileLength:    fileLength,
			}); err != nil {
				fmt.Fprintf(os.Stderr, "\n⚠ failed to store message %s: %v\n", id, err)
			}

			if directPath != "" && len(mediaKey) > 0 {
				worker.Enqueue(mediaJob{messageID: id, chatJID: chatJID})
			}

			messageCount.Add(1)
			fmt.Fprintf(os.Stderr, "\r💬 Synced %d messages...", messageCount.Load())

		case *events.HistorySync:
			fmt.Fprintf(os.Stderr, "\n📜 Processing history sync (%d conversations)...\n", len(v.Data.Conversations))
			for _, conv := range v.Data.Conversations {
				chatJID := conv.GetID()
				chatName := conv.GetName()
				if chatName == "" {
					chatName = a.client.ResolveChatName(ctx, chatJID, nil)
					if chatName == "" {
						chatName = chatJID
					}
				}

				// Process messages in this conversation
				for _, msg := range conv.Messages {
					if msg.Message == nil {
						continue
					}

					histMsg := msg.Message
					msgID := histMsg.Key.GetID()
					sender := a.client.ResolveJID(ctx, histMsg.Key.GetParticipant())
					if sender == "" {
						sender = a.client.ResolveJID(ctx, histMsg.Key.GetRemoteJID())
					}
					isFromMe := histMsg.Key.GetFromMe()
					msgTimestamp := time.Unix(int64(histMsg.GetMessageTimestamp()), 0)

					// Extract content
					content := ""
					mediaType := ""
					filename := ""
					url := ""
					directPath := ""
					mimeType := ""
					var mediaKey, fileSHA256, fileEncSHA256 []byte
					var fileLength uint64

					switch {
					case histMsg.Message.GetConversation() != "":
						content = histMsg.Message.GetConversation()
					case histMsg.Message.GetExtendedTextMessage() != nil:
						extText := histMsg.Message.GetExtendedTextMessage()
						content = extText.GetText()
					case histMsg.Message.GetImageMessage() != nil:
						img := histMsg.Message.GetImageMessage()
						mediaType = "image"
						content = img.GetCaption()
						// Don't use caption as filename - it can be very long text
						url = img.GetURL()
						directPath = img.GetDirectPath()
						mimeType = img.GetMimetype()
						mediaKey = img.GetMediaKey()
						fileSHA256 = img.GetFileSHA256()
						fileEncSHA256 = img.GetFileEncSHA256()
						fileLength = img.GetFileLength()
					case histMsg.Message.GetVideoMessage() != nil:
						video := histMsg.Message.GetVideoMessage()
						mediaType = "video"
						content = video.GetCaption()
						// Don't use caption as filename - it can be very long text
						url = video.GetURL()
						directPath = video.GetDirectPath()
						mimeType = video.GetMimetype()
						mediaKey = video.GetMediaKey()
						fileSHA256 = video.GetFileSHA256()
						fileEncSHA256 = video.GetFileEncSHA256()
						fileLength = video.GetFileLength()
					case histMsg.Message.GetAudioMessage() != nil:
						audio := histMsg.Message.GetAudioMessage()
						mediaType = "audio"
						content = "[Audio]"
						url = audio.GetURL()
						directPath = audio.GetDirectPath()
						mimeType = audio.GetMimetype()
						mediaKey = audio.GetMediaKey()
						fileSHA256 = audio.GetFileSHA256()
						fileEncSHA256 = audio.GetFileEncSHA256()
						fileLength = audio.GetFileLength()
					case histMsg.Message.GetDocumentMessage() != nil:
						doc := histMsg.Message.GetDocumentMessage()
						mediaType = "document"
						content = doc.GetCaption()
						filename = doc.GetFileName()
						url = doc.GetURL()
						directPath = doc.GetDirectPath()
						mimeType = doc.GetMimetype()
						mediaKey = doc.GetMediaKey()
						fileSHA256 = doc.GetFileSHA256()
						fileEncSHA256 = doc.GetFileEncSHA256()
						fileLength = doc.GetFileLength()
					}

					if err := a.store.StoreChat(chatJID, chatName, msgTimestamp); err != nil {
						fmt.Fprintf(os.Stderr, "\n⚠ failed to store chat %s: %v\n", chatJID, err)
					}

					if err := a.store.StoreMessage(store.StoreMessageParams{
						ID:            msgID,
						ChatJID:       chatJID,
						Sender:        sender,
						Content:       content,
						Timestamp:     msgTimestamp,
						IsFromMe:      isFromMe,
						MediaType:     mediaType,
						Filename:      filename,
						URL:           url,
						DirectPath:    directPath,
						MimeType:      mimeType,
						MediaKey:      mediaKey,
						FileSHA256:    fileSHA256,
						FileEncSHA256: fileEncSHA256,
						FileLength:    fileLength,
					}); err != nil {
						fmt.Fprintf(os.Stderr, "\n⚠ failed to store message %s: %v\n", msgID, err)
					}

					if directPath != "" && len(mediaKey) > 0 {
						worker.Enqueue(mediaJob{messageID: msgID, chatJID: chatJID})
					}

					messageCount.Add(1)
				}
			}
			fmt.Fprintf(os.Stderr, "\r💬 Synced %d messages...", messageCount.Load())

		case *events.Connected:
			fmt.Fprintln(os.Stderr, "\n✓ Connected to WhatsApp")
			fmt.Fprintln(os.Stderr, "🔄 Listening for messages... (Press Ctrl+C to stop)")

		case *events.Disconnected:
			fmt.Fprintln(os.Stderr, "\n⚠ Disconnected from WhatsApp")
		}
	}

	// Start syncing
	fmt.Fprintln(os.Stderr, "🚀 Starting WhatsApp sync...")
	if err := a.client.StartSync(ctx, eventHandler); err != nil {
		return SyncResult{}, err
	}

	// Repair any stored LID identities from before LID resolution was added.
	// Must run AFTER StartSync — the LID store on the whatsmeow client is only
	// addressable once Connect populates the device store.
	a.repairLIDIdentities(ctx)

	// Wait for context cancellation (Ctrl+C)
	<-ctx.Done()

	total := messageCount.Load()
	fmt.Fprintf(os.Stderr, "\n\n✓ Sync completed. Total messages synced: %d\n", total)

	return SyncResult{Synced: true, MessagesCount: total}, nil
}

// repairLIDIdentities scans stored messages and chats for unresolved LID
// identities and resolves them to phone-number JIDs using the whatsmeow
// LID mapping store. Runs once at sync startup. Unresolvable rows are
// left untouched and retried on next sync.
func (a *App) repairLIDIdentities(ctx context.Context) {
	a.repairLIDSenders(ctx)
	a.repairLIDChats(ctx)
}

func (a *App) repairLIDSenders(ctx context.Context) {
	// Phase 1: Fix full LID JIDs (e.g. "278378372440237@lid")
	rows, err := a.store.GetLIDSenders()
	if err != nil {
		fmt.Fprintf(os.Stderr, "⚠ failed to scan LID senders: %v\n", err)
		return
	}
	repaired, unresolved := 0, 0
	if len(rows) > 0 {
		fmt.Fprintf(os.Stderr, "🔧 Repairing %d stored LID sender identities...\n", len(rows))
		for _, r := range rows {
			resolved := a.client.ResolveJID(ctx, r.Sender)
			if resolved == r.Sender {
				unresolved++
				continue
			}
			if err := a.store.UpdateSender(r.ID, r.ChatJID, resolved); err != nil {
				fmt.Fprintf(os.Stderr, "⚠ failed to update sender for %s: %v\n", r.ID, err)
				continue
			}
			repaired++
		}
	}

	// Phase 2: Fix bare LID user parts (e.g. "278378372440237" without @lid).
	// Older HandleMessage extracted Sender.User which strips the @lid suffix.
	bareSenders, err := a.store.GetBareSenders()
	if err != nil {
		fmt.Fprintf(os.Stderr, "⚠ failed to scan bare senders: %v\n", err)
	}
	bareFixed := 0
	for _, sender := range bareSenders {
		// Try resolving as a LID by appending @lid.
		resolved := a.client.ResolveJID(ctx, sender+"@lid")
		if resolved == sender+"@lid" {
			continue // Not a known LID — probably a phone number, leave it.
		}
		// Strip the @s.whatsapp.net suffix to match the bare format used for senders.
		newSender := strings.TrimSuffix(resolved, "@s.whatsapp.net")
		if newSender == sender {
			continue
		}
		n, err := a.store.UpdateSenderBatch(sender, newSender)
		if err != nil {
			fmt.Fprintf(os.Stderr, "⚠ failed to batch update sender %s: %v\n", sender, err)
			continue
		}
		bareFixed += int(n)
	}

	total := repaired + bareFixed
	if total > 0 || unresolved > 0 {
		fmt.Fprintf(os.Stderr, "🔧 Senders: %d fixed, %d unresolved\n", total, unresolved)
	}
}

func (a *App) repairLIDChats(ctx context.Context) {
	rows, err := a.store.GetLIDChats()
	if err != nil {
		fmt.Fprintf(os.Stderr, "⚠ failed to scan LID chats: %v\n", err)
		return
	}
	if len(rows) == 0 {
		return
	}

	fmt.Fprintf(os.Stderr, "🔧 Repairing %d stored LID chat identities...\n", len(rows))
	repaired, unresolved := 0, 0
	for _, r := range rows {
		resolved := a.client.ResolveJID(ctx, r.JID)
		if resolved == r.JID {
			unresolved++
			continue
		}
		if err := a.store.UpdateChatJID(r.JID, resolved); err != nil {
			fmt.Fprintf(os.Stderr, "⚠ failed to update chat JID %s: %v\n", r.JID, err)
			continue
		}
		repaired++
	}
	fmt.Fprintf(os.Stderr, "🔧 Chats: %d fixed, %d unresolved\n", repaired, unresolved)
}

func resolveVersion(version string, describeFn func() (string, error)) string {
	if strings.TrimSpace(version) != "" && version != "dev" {
		return version
	}

	if describeFn != nil {
		if gitVersion, err := describeFn(); err == nil && strings.TrimSpace(gitVersion) != "" {
			return gitVersion
		}
	}

	if strings.TrimSpace(version) == "" {
		return "unknown"
	}
	return version
}

func gitDescribe() (string, error) {
	cmd := exec.Command("git", "describe", "--tags", "--dirty", "--always")
	cmd.Env = os.Environ()
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}
