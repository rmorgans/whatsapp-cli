package cmd

import (
	"context"
	"encoding/json"
	"fmt"

	r "github.com/vicentereig/whatsapp-cli/cmd/registry"
	"github.com/vicentereig/whatsapp-cli/internal/commands"
)

// optStr returns nil when the flag was not explicitly set, otherwise a pointer
// to the flag's string value. This preserves the "not provided" vs "empty"
// distinction that the app layer uses for optional filter parameters.
func optStr(f r.FlagValues, name string) *string {
	if !f.IsSet(name) {
		return nil
	}
	v := f.String(name)
	return &v
}

func registerAll(reg *r.Registry) {
	// -----------------------------------------------------------------
	// version — local exec, no app init
	// -----------------------------------------------------------------
	ver := r.MustNewLeafSpec("version", r.MustNewPath("version"),
		func(_ context.Context, _ *commands.App, _ r.FlagValues) (string, error) {
			escaped, _ := json.Marshal(reg.Version())
			return fmt.Sprintf(`{"success":true,"data":{"version":%s},"error":null}`, escaped), nil
		},
	)
	ver.Doc = r.DocSpec{Short: "Print CLI version information"}
	ver.Exec = r.Local()
	reg.Register(ver)

	// -----------------------------------------------------------------
	// auth — bounded
	// -----------------------------------------------------------------
	auth := r.MustNewLeafSpec("auth", r.MustNewPath("auth"),
		func(ctx context.Context, app *commands.App, _ r.FlagValues) (string, error) {
			return app.Auth(ctx), nil
		},
	)
	auth.Doc = r.DocSpec{Short: "Authenticate with WhatsApp (scan QR code)"}
	auth.Exec = r.Bounded(0)
	reg.Register(auth)

	// -----------------------------------------------------------------
	// sync — streaming
	// -----------------------------------------------------------------
	sync := r.MustNewLeafSpec("sync", r.MustNewPath("sync"),
		func(ctx context.Context, app *commands.App, _ r.FlagValues) (string, error) {
			return app.Sync(ctx), nil
		},
	)
	sync.Doc = r.DocSpec{Short: "Sync messages continuously (run until Ctrl+C)"}
	sync.Exec = r.Streaming()
	reg.Register(sync)

	// -----------------------------------------------------------------
	// send — bounded, with mutual exclusion (message XOR image)
	// -----------------------------------------------------------------
	send := r.MustNewLeafSpec("send", r.MustNewPath("send"),
		func(ctx context.Context, app *commands.App, f r.FlagValues) (string, error) {
			to := f.String("to")
			replyTo := ""
			if f.IsSet("reply-to") {
				replyTo = f.String("reply-to")
			}
			switch {
			case f.IsSet("image"):
				if replyTo != "" {
					return "", fmt.Errorf("--reply-to is only supported with text messages")
				}
				return app.SendImage(ctx, to, f.String("image"), f.String("caption")), nil
			case f.IsSet("video"):
				if replyTo != "" {
					return "", fmt.Errorf("--reply-to is only supported with text messages")
				}
				return app.SendVideo(ctx, to, f.String("video"), f.String("caption")), nil
			case f.IsSet("audio"):
				if replyTo != "" {
					return "", fmt.Errorf("--reply-to is only supported with text messages")
				}
				return app.SendAudio(ctx, to, f.String("audio")), nil
			case f.IsSet("document"):
				if replyTo != "" {
					return "", fmt.Errorf("--reply-to is only supported with text messages")
				}
				return app.SendDocument(ctx, to, f.String("document"), f.String("filename")), nil
			default:
				if replyTo != "" {
					return app.SendReply(ctx, to, f.String("message"), replyTo), nil
				}
				return app.SendMessage(ctx, to, f.String("message")), nil
			}
		},
	)
	send.Doc = r.DocSpec{Short: "Send a message, image, video, audio, or document"}
	send.Exec = r.Bounded(0)
	send.Flags = []r.Flag{
		r.StringFlag{Name: "to", Help: "recipient JID or phone number", Required: true},
		r.StringFlag{Name: "message", Help: "message text"},
		r.StringFlag{Name: "image", Help: "image file path"},
		r.StringFlag{Name: "video", Help: "video file path"},
		r.StringFlag{Name: "audio", Help: "audio file path"},
		r.StringFlag{Name: "document", Help: "document file path"},
		r.StringFlag{Name: "caption", Help: "media caption (for image/video)"},
		r.StringFlag{Name: "filename", Help: "document filename override"},
		r.StringFlag{Name: "reply-to", Help: "message ID to reply to"},
	}
	send.Rules = r.RuleSpec{
		Requires: map[string][]string{"filename": {"document"}},
	}
	// Custom validation: content flags are mutually exclusive, exactly one required.
	// Preserves backward-compatible error messages for existing tests.
	send.Validate = func(f r.FlagValues) error {
		contentFlags := []string{"message", "image", "video", "audio", "document"}
		var set []string
		for _, name := range contentFlags {
			if f.IsSet(name) {
				set = append(set, name)
			}
		}
		if len(set) > 1 {
			return fmt.Errorf("--message and --image are mutually exclusive")
		}
		if len(set) == 0 {
			return fmt.Errorf("--message or --image required")
		}
		return nil
	}
	reg.Register(send)

	// -----------------------------------------------------------------
	// messages (parent)
	// -----------------------------------------------------------------
	reg.RegisterParent(r.ParentSpec{
		Path:  r.MustNewPath("messages"),
		Short: "List and search messages",
	})

	// messages list
	msgList := r.MustNewLeafSpec("messages.list", r.MustNewPath("messages", "list"),
		func(_ context.Context, app *commands.App, f r.FlagValues) (string, error) {
			return app.ListMessages(optStr(f, "chat"), nil, f.Int("limit"), f.Int("page")), nil
		},
	)
	msgList.Doc = r.DocSpec{Short: "List messages in a chat"}
	msgList.Exec = r.Bounded(0)
	msgList.Flags = []r.Flag{
		r.StringFlag{Name: "chat", Help: "chat JID to filter by"},
		r.IntFlag{Name: "limit", Help: "maximum messages to return", Default: 20},
		r.IntFlag{Name: "page", Help: "page number"},
	}
	reg.Register(msgList)

	// messages search
	msgSearch := r.MustNewLeafSpec("messages.search", r.MustNewPath("messages", "search"),
		func(_ context.Context, app *commands.App, f r.FlagValues) (string, error) {
			query := f.String("query")
			return app.ListMessages(nil, &query, f.Int("limit"), f.Int("page")), nil
		},
	)
	msgSearch.Doc = r.DocSpec{Short: "Search messages by text"}
	msgSearch.Exec = r.Bounded(0)
	msgSearch.Flags = []r.Flag{
		r.StringFlag{Name: "query", Help: "search text", Required: true},
		r.IntFlag{Name: "limit", Help: "maximum messages to return", Default: 20},
		r.IntFlag{Name: "page", Help: "page number"},
	}
	reg.Register(msgSearch)

	// messages react
	react := r.MustNewLeafSpec("messages.react", r.MustNewPath("messages", "react"),
		func(ctx context.Context, app *commands.App, f r.FlagValues) (string, error) {
			return app.ReactToMessage(ctx, f.String("message-id"), f.String("emoji"), optStr(f, "chat")), nil
		},
	)
	react.Doc = r.DocSpec{Short: "React to a message with an emoji"}
	react.Exec = r.Bounded(0)
	react.Flags = []r.Flag{
		r.StringFlag{Name: "message-id", Help: "message to react to", Required: true},
		r.StringFlag{Name: "emoji", Help: "reaction emoji (empty string to remove reaction)", Required: true},
		r.StringFlag{Name: "chat", Help: "chat JID (required if message ID is ambiguous)"},
	}
	reg.Register(react)

	// messages delete
	msgDelete := r.MustNewLeafSpec("messages.delete", r.MustNewPath("messages", "delete"),
		func(ctx context.Context, app *commands.App, f r.FlagValues) (string, error) {
			return app.DeleteMessage(ctx, f.String("message-id"), optStr(f, "chat")), nil
		},
	)
	msgDelete.Doc = r.DocSpec{Short: "Delete (revoke) a message"}
	msgDelete.Exec = r.Bounded(0)
	msgDelete.Flags = []r.Flag{
		r.StringFlag{Name: "message-id", Help: "message to delete", Required: true},
		r.StringFlag{Name: "chat", Help: "chat JID (required if message ID is ambiguous)"},
	}
	reg.Register(msgDelete)

	// messages edit
	msgEdit := r.MustNewLeafSpec("messages.edit", r.MustNewPath("messages", "edit"),
		func(ctx context.Context, app *commands.App, f r.FlagValues) (string, error) {
			return app.EditMessage(ctx, f.String("message-id"), f.String("text"), optStr(f, "chat")), nil
		},
	)
	msgEdit.Doc = r.DocSpec{Short: "Edit a message"}
	msgEdit.Exec = r.Bounded(0)
	msgEdit.Flags = []r.Flag{
		r.StringFlag{Name: "message-id", Help: "message to edit", Required: true},
		r.StringFlag{Name: "text", Help: "new message text", Required: true},
		r.StringFlag{Name: "chat", Help: "chat JID (required if message ID is ambiguous)"},
	}
	reg.Register(msgEdit)

	// messages mark-read
	msgMarkRead := r.MustNewLeafSpec("messages.mark-read", r.MustNewPath("messages", "mark-read"),
		func(ctx context.Context, app *commands.App, f r.FlagValues) (string, error) {
			return app.MarkMessageRead(ctx, f.String("message-id"), optStr(f, "chat")), nil
		},
	)
	msgMarkRead.Doc = r.DocSpec{Short: "Mark a message as read"}
	msgMarkRead.Exec = r.Bounded(0)
	msgMarkRead.Flags = []r.Flag{
		r.StringFlag{Name: "message-id", Help: "message to mark as read", Required: true},
		r.StringFlag{Name: "chat", Help: "chat JID (required if message ID is ambiguous)"},
	}
	reg.Register(msgMarkRead)

	// -----------------------------------------------------------------
	// contacts (parent)
	// -----------------------------------------------------------------
	reg.RegisterParent(r.ParentSpec{
		Path:  r.MustNewPath("contacts"),
		Short: "Manage contacts (search, block, check)",
	})

	// contacts search
	ctSearch := r.MustNewLeafSpec("contacts.search", r.MustNewPath("contacts", "search"),
		func(_ context.Context, app *commands.App, f r.FlagValues) (string, error) {
			return app.SearchContacts(f.String("query")), nil
		},
	)
	ctSearch.Doc = r.DocSpec{Short: "Search contacts by name"}
	ctSearch.Exec = r.Bounded(0)
	ctSearch.Flags = []r.Flag{
		r.StringFlag{Name: "query", Help: "search text", Required: true},
	}
	reg.Register(ctSearch)

	// contacts block
	ctBlock := r.MustNewLeafSpec("contacts.block", r.MustNewPath("contacts", "block"),
		func(ctx context.Context, app *commands.App, f r.FlagValues) (string, error) {
			return app.BlockContact(ctx, f.String("jid")), nil
		},
	)
	ctBlock.Doc = r.DocSpec{Short: "Block a contact"}
	ctBlock.Exec = r.Bounded(0)
	ctBlock.Flags = []r.Flag{
		r.StringFlag{Name: "jid", Help: "JID of the contact to block", Required: true},
	}
	reg.Register(ctBlock)

	// contacts unblock
	ctUnblock := r.MustNewLeafSpec("contacts.unblock", r.MustNewPath("contacts", "unblock"),
		func(ctx context.Context, app *commands.App, f r.FlagValues) (string, error) {
			return app.UnblockContact(ctx, f.String("jid")), nil
		},
	)
	ctUnblock.Doc = r.DocSpec{Short: "Unblock a contact"}
	ctUnblock.Exec = r.Bounded(0)
	ctUnblock.Flags = []r.Flag{
		r.StringFlag{Name: "jid", Help: "JID of the contact to unblock", Required: true},
	}
	reg.Register(ctUnblock)

	// contacts list-blocked
	ctListBlocked := r.MustNewLeafSpec("contacts.list-blocked", r.MustNewPath("contacts", "list-blocked"),
		func(ctx context.Context, app *commands.App, _ r.FlagValues) (string, error) {
			return app.ListBlocked(ctx), nil
		},
	)
	ctListBlocked.Doc = r.DocSpec{Short: "List blocked contacts"}
	ctListBlocked.Exec = r.Bounded(0)
	reg.Register(ctListBlocked)

	// contacts check
	ctCheck := r.MustNewLeafSpec("contacts.check", r.MustNewPath("contacts", "check"),
		func(ctx context.Context, app *commands.App, f r.FlagValues) (string, error) {
			return app.CheckOnWhatsApp(ctx, f.StringSlice("phone")), nil
		},
	)
	ctCheck.Doc = r.DocSpec{Short: "Check if phone numbers are on WhatsApp"}
	ctCheck.Exec = r.Bounded(0)
	ctCheck.Flags = []r.Flag{
		r.StringSliceFlag{Name: "phone", Help: "phone numbers to check (international format with +)", Required: true},
	}
	reg.Register(ctCheck)

	// -----------------------------------------------------------------
	// chats (parent)
	// -----------------------------------------------------------------
	reg.RegisterParent(r.ParentSpec{
		Path:  r.MustNewPath("chats"),
		Short: "List chats",
	})

	// chats list
	chatsList := r.MustNewLeafSpec("chats.list", r.MustNewPath("chats", "list"),
		func(_ context.Context, app *commands.App, f r.FlagValues) (string, error) {
			return app.ListChats(optStr(f, "query"), f.Int("limit"), f.Int("page")), nil
		},
	)
	chatsList.Doc = r.DocSpec{Short: "List recent chats"}
	chatsList.Exec = r.Bounded(0)
	chatsList.Flags = []r.Flag{
		r.StringFlag{Name: "query", Help: "filter chats by name"},
		r.IntFlag{Name: "limit", Help: "maximum chats to return", Default: 20},
		r.IntFlag{Name: "page", Help: "page number"},
	}
	reg.Register(chatsList)

	// -----------------------------------------------------------------
	// media (parent)
	// -----------------------------------------------------------------
	reg.RegisterParent(r.ParentSpec{
		Path:  r.MustNewPath("media"),
		Short: "Download media attachments",
	})

	// media download
	mediaDl := r.MustNewLeafSpec("media.download", r.MustNewPath("media", "download"),
		func(ctx context.Context, app *commands.App, f r.FlagValues) (string, error) {
			return app.DownloadMedia(ctx, f.String("message-id"), optStr(f, "chat"), f.String("output")), nil
		},
	)
	mediaDl.Doc = r.DocSpec{Short: "Download media for a message"}
	mediaDl.Exec = r.Bounded(0)
	mediaDl.Flags = []r.Flag{
		r.StringFlag{Name: "message-id", Help: "message identifier", Required: true},
		r.StringFlag{Name: "chat", Help: "chat JID (optional)"},
		r.StringFlag{Name: "output", Help: "output file or directory"},
	}
	reg.Register(mediaDl)

	// -----------------------------------------------------------------
	// groups (parent)
	// -----------------------------------------------------------------
	reg.RegisterParent(r.ParentSpec{
		Path:  r.MustNewPath("groups"),
		Short: "Manage WhatsApp groups",
	})

	// groups list
	grpList := r.MustNewLeafSpec("groups.list", r.MustNewPath("groups", "list"),
		func(ctx context.Context, app *commands.App, _ r.FlagValues) (string, error) {
			return app.GroupsList(ctx), nil
		},
	)
	grpList.Doc = r.DocSpec{Short: "List joined groups"}
	grpList.Exec = r.Bounded(0)
	reg.Register(grpList)

	// groups info
	grpInfo := r.MustNewLeafSpec("groups.info", r.MustNewPath("groups", "info"),
		func(ctx context.Context, app *commands.App, f r.FlagValues) (string, error) {
			return app.GroupsInfo(ctx, f.String("jid")), nil
		},
	)
	grpInfo.Doc = r.DocSpec{Short: "Get group info"}
	grpInfo.Exec = r.Bounded(0)
	grpInfo.Flags = []r.Flag{
		r.StringFlag{Name: "jid", Help: "group JID", Required: true},
	}
	reg.Register(grpInfo)

	// groups create
	grpCreate := r.MustNewLeafSpec("groups.create", r.MustNewPath("groups", "create"),
		func(ctx context.Context, app *commands.App, f r.FlagValues) (string, error) {
			return app.GroupsCreate(ctx, f.String("name"), f.StringSlice("members")), nil
		},
	)
	grpCreate.Doc = r.DocSpec{Short: "Create a new group"}
	grpCreate.Exec = r.Bounded(0)
	grpCreate.Flags = []r.Flag{
		r.StringFlag{Name: "name", Help: "group name (max 25 characters)", Required: true},
		r.StringSliceFlag{Name: "members", Help: "member JIDs or phone numbers"},
	}
	reg.Register(grpCreate)

	// groups invite-link
	grpInviteLink := r.MustNewLeafSpec("groups.invite-link", r.MustNewPath("groups", "invite-link"),
		func(ctx context.Context, app *commands.App, f r.FlagValues) (string, error) {
			return app.GroupsInviteLink(ctx, f.String("jid"), f.Bool("reset")), nil
		},
	)
	grpInviteLink.Doc = r.DocSpec{Short: "Get or reset group invite link"}
	grpInviteLink.Exec = r.Bounded(0)
	grpInviteLink.Flags = []r.Flag{
		r.StringFlag{Name: "jid", Help: "group JID", Required: true},
		r.BoolFlag{Name: "reset", Help: "revoke old link and generate new one"},
	}
	reg.Register(grpInviteLink)

	// groups join
	grpJoin := r.MustNewLeafSpec("groups.join", r.MustNewPath("groups", "join"),
		func(ctx context.Context, app *commands.App, f r.FlagValues) (string, error) {
			return app.GroupsJoin(ctx, f.String("link")), nil
		},
	)
	grpJoin.Doc = r.DocSpec{Short: "Join a group via invite link"}
	grpJoin.Exec = r.Bounded(0)
	grpJoin.Flags = []r.Flag{
		r.StringFlag{Name: "link", Help: "group invite link", Required: true},
	}
	reg.Register(grpJoin)

	// groups leave
	grpLeave := r.MustNewLeafSpec("groups.leave", r.MustNewPath("groups", "leave"),
		func(ctx context.Context, app *commands.App, f r.FlagValues) (string, error) {
			return app.GroupsLeave(ctx, f.String("jid")), nil
		},
	)
	grpLeave.Doc = r.DocSpec{Short: "Leave a group"}
	grpLeave.Exec = r.Bounded(0)
	grpLeave.Flags = []r.Flag{
		r.StringFlag{Name: "jid", Help: "group JID", Required: true},
	}
	reg.Register(grpLeave)

	// groups add-members
	grpAddMembers := r.MustNewLeafSpec("groups.add-members", r.MustNewPath("groups", "add-members"),
		func(ctx context.Context, app *commands.App, f r.FlagValues) (string, error) {
			return app.GroupsAddMembers(ctx, f.String("jid"), f.StringSlice("members")), nil
		},
	)
	grpAddMembers.Doc = r.DocSpec{Short: "Add members to a group"}
	grpAddMembers.Exec = r.Bounded(0)
	grpAddMembers.Flags = []r.Flag{
		r.StringFlag{Name: "jid", Help: "group JID", Required: true},
		r.StringSliceFlag{Name: "members", Help: "member JIDs or phone numbers to add", Required: true},
	}
	reg.Register(grpAddMembers)

	// groups remove-members
	grpRemoveMembers := r.MustNewLeafSpec("groups.remove-members", r.MustNewPath("groups", "remove-members"),
		func(ctx context.Context, app *commands.App, f r.FlagValues) (string, error) {
			return app.GroupsRemoveMembers(ctx, f.String("jid"), f.StringSlice("members")), nil
		},
	)
	grpRemoveMembers.Doc = r.DocSpec{Short: "Remove members from a group"}
	grpRemoveMembers.Exec = r.Bounded(0)
	grpRemoveMembers.Flags = []r.Flag{
		r.StringFlag{Name: "jid", Help: "group JID", Required: true},
		r.StringSliceFlag{Name: "members", Help: "member JIDs or phone numbers to remove", Required: true},
	}
	reg.Register(grpRemoveMembers)

	// groups set-name
	grpSetName := r.MustNewLeafSpec("groups.set-name", r.MustNewPath("groups", "set-name"),
		func(ctx context.Context, app *commands.App, f r.FlagValues) (string, error) {
			return app.GroupsSetName(ctx, f.String("jid"), f.String("name")), nil
		},
	)
	grpSetName.Doc = r.DocSpec{Short: "Set group name"}
	grpSetName.Exec = r.Bounded(0)
	grpSetName.Flags = []r.Flag{
		r.StringFlag{Name: "jid", Help: "group JID", Required: true},
		r.StringFlag{Name: "name", Help: "new group name (max 25 characters)", Required: true},
	}
	reg.Register(grpSetName)

	// groups set-description
	grpSetDesc := r.MustNewLeafSpec("groups.set-description", r.MustNewPath("groups", "set-description"),
		func(ctx context.Context, app *commands.App, f r.FlagValues) (string, error) {
			return app.GroupsSetDescription(ctx, f.String("jid"), f.String("description")), nil
		},
	)
	grpSetDesc.Doc = r.DocSpec{Short: "Set group description"}
	grpSetDesc.Exec = r.Bounded(0)
	grpSetDesc.Flags = []r.Flag{
		r.StringFlag{Name: "jid", Help: "group JID", Required: true},
		r.StringFlag{Name: "description", Help: "new group description", Required: true},
	}
	reg.Register(grpSetDesc)

	// groups set-photo
	grpSetPhoto := r.MustNewLeafSpec("groups.set-photo", r.MustNewPath("groups", "set-photo"),
		func(ctx context.Context, app *commands.App, f r.FlagValues) (string, error) {
			return app.GroupsSetPhoto(ctx, f.String("jid"), f.String("image")), nil
		},
	)
	grpSetPhoto.Doc = r.DocSpec{Short: "Set group photo (JPEG)"}
	grpSetPhoto.Exec = r.Bounded(0)
	grpSetPhoto.Flags = []r.Flag{
		r.StringFlag{Name: "jid", Help: "group JID", Required: true},
		r.StringFlag{Name: "image", Help: "path to JPEG image file", Required: true},
	}
	reg.Register(grpSetPhoto)
}
