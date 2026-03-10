# Idiomatic Go Refactor Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Refactor whatsapp-cli so the app layer returns typed Go values instead of pre-serialized JSON strings. Serialization happens once in the transport layer.

**Architecture:** Currently, every `App` method returns `string` by calling `output.Success()`/`output.Error()`. The `Runner` type is `func(...) (string, error)`. This plan changes `Runner` to `func(...) (any, error)`, moves serialization into `printResult`, then incrementally converts App methods to return `(T, error)` with per-command result types whose `json` tags exactly match the current JSON contract.

**Tech Stack:** Go 1.23, cobra, whatsmeow, testify, SQLite

---

## Task 1: Real transport seam — `Runner` returns `(any, error)`

Change the `Runner` type from `(string, error)` to `(any, error)`. Have `printResult` serialize the result to JSON, rather than each App method doing it.

This is the foundational change. Everything else builds on it.

**Files:**
- Modify: `cmd/registry/types.go:222` — change `Runner` signature
- Modify: `cmd/registry/register.go:316-371,378-382,407-444` — update `buildRunE` and `printResult`
- Modify: `internal/output/output.go` — add `SuccessResult` that returns `Result` (not string)
- Modify: `cmd/commands.go` — update all Runner wrappers

### Step 1: Write failing test

Add a test in `cmd/registry/register_test.go` that creates a command whose Runner returns a typed value (e.g. `map[string]any{"version": "1.0"}`) and verifies the output writer receives valid JSON `{"success":true,"data":{"version":"1.0"},"error":null}`.

### Step 2: Run test to verify it fails

```bash
go test ./cmd/registry/ -run TestPrintResultTyped -v
```
Expected: FAIL — `Runner` still expects `string` return.

### Step 3: Change the Runner type

In `cmd/registry/types.go`:
```go
// Before:
type Runner func(ctx context.Context, app *commands.App, f FlagValues) (string, error)

// After:
type Runner func(ctx context.Context, app *commands.App, f FlagValues) (any, error)
```

### Step 4: Replace `output.Success()`/`output.Error()` string helpers with typed envelope API

In `internal/output/output.go`, replace the string-returning helpers with a typed API:

```go
package output

import "encoding/json"

// Result is the typed envelope. The registry is the only caller of Marshal.
type Result struct {
    Success bool    `json:"success"`
    Data    any     `json:"data"`
    Error   *string `json:"error"`
}

// Success builds a Result envelope for successful commands.
func Success(data any) Result {
    return Result{Success: true, Data: data}
}

// Failure builds a Result envelope from an error.
func Failure(err error) Result {
    msg := err.Error()
    return Result{Success: false, Error: &msg}
}

// Marshal serializes a Result to JSON bytes.
func Marshal(r Result) ([]byte, error) {
    return json.Marshal(r)
}
```

This replaces the old `Success(data) string` and `Error(err) string` helpers. The registry calls `output.Marshal()` — it is the only serializer.

### Step 5: Update `buildRunE` to use the typed envelope API

In `cmd/registry/register.go`, change `buildRunE` so that when Runner returns `(result, nil)`:
- If `result` is a `string`, treat it as pre-serialized JSON (**temporary migration bridge — deleted in Task 6**)
- If `result` is an `output.Result`, pass through directly
- If `result` is anything else, wrap via `output.Success(result)`

When Runner returns `(_, err)`:
- Wrap via `output.Failure(err)`

This means `printResult` signature changes from `(commandID, result string)` to `(commandID string, result output.Result)`, and serialization happens there.

```go
func (r *Registry) buildRunE(spec LeafSpec) func(*cobra.Command, []string) error {
    return func(cmd *cobra.Command, args []string) error {
        // ... validation unchanged ...

        result, err := spec.Run(ctx, r.app, fv)
        if err != nil {
            r.printResult(spec.ID, output.Failure(err))
            return nil
        }

        // TEMPORARY MIGRATION BRIDGE (deleted in Task 6):
        // While App methods still return pre-serialized JSON strings,
        // parse them back into a Result so printResult always gets typed data.
        switch v := result.(type) {
        case string:
            env, parseErr := output.ParseEnvelope(v)
            if parseErr != nil {
                r.printResult(spec.ID, output.Success(v))
            } else {
                r.printResult(spec.ID, output.Result{Success: env.Success, Data: env.Data, Error: env.Error})
            }
        case output.Result:
            r.printResult(spec.ID, v)
        default:
            r.printResult(spec.ID, output.Success(v))
        }
        return nil
    }
}
```

### Step 6: Add test proving string bridge and typed path produce identical output

Add a test in `cmd/registry/register_test.go` that registers two commands returning the same data — one via legacy string, one via typed value — and asserts identical stdout and exit code:

```go
func TestPrintResult_StringBridge_MatchesTypedPath(t *testing.T) {
    // Command A: Runner returns output.Success(map) as pre-serialized string (legacy)
    // Command B: Runner returns map directly (new path)
    // Assert: both produce identical JSON on stdout and same exit code.
}
```

This test is the canary for the bridge. When the bridge is deleted in Task 6, this test is also deleted.

### Step 7: Update `printResult` to accept `output.Result`

```go
func (r *Registry) printResult(commandID string, result output.Result) {
    if !result.Success {
        r.exitCode = 1
    }

    mode := r.resolveOutputMode()

    if mode == output.ModeHuman {
        // Build envelope for human formatters.
        rawData, _ := json.Marshal(result.Data)
        env := output.Envelope{
            Success: result.Success,
            Data:    rawData,
            Error:   result.Error,
        }
        if formatted, ok, err := output.FormatHuman(commandID, env); ok {
            fmt.Fprintln(r.writer, formatted)
            return
        } else if err != nil {
            fmt.Fprintf(os.Stderr, "⚠ %v\n", err)
        }
        if formatted, err := output.GenericFormat(env); err == nil {
            fmt.Fprintln(r.writer, formatted)
            return
        }
    }

    // JSON mode (or human fallback): serialize once via output.Marshal.
    b, _ := output.Marshal(result)
    fmt.Fprintln(r.writer, string(b))
}
```

### Step 6: Remove `errorJSON` helper

The `errorJSON` function in `register.go:378-382` is no longer needed — `printResult` handles error serialization.

### Step 7: Update `cmd/commands.go` Runner wrappers

All Runner funcs currently return `(string, error)`. They now return `(any, error)`. Since App methods still return `string` during migration, the Runners just pass through:

```go
// Before:
func(ctx context.Context, app *commands.App, _ r.FlagValues) (string, error) {
    return app.Auth(ctx), nil
}

// After:
func(ctx context.Context, app *commands.App, _ r.FlagValues) (any, error) {
    return app.Auth(ctx), nil
}
```

The string backward-compat path in `buildRunE` handles these until each method is converted.

### Step 8: Run all tests

```bash
go test ./... -race
```
Expected: All pass. JSON output unchanged for every command.

### Step 9: Commit

```bash
git add cmd/registry/types.go cmd/registry/register.go cmd/registry/register_test.go cmd/commands.go internal/output/output.go
git commit -m "refactor: change Runner to return (any, error), serialize in transport layer"
```

---

## Task 2: Fix `%v` → `%w` error wrapping

Smallest change, zero risk.

**Files:**
- Modify: `internal/client/client.go:61,68,76,110,136`
- Modify: `internal/store/store.go:97,102,137`

### Step 1: Fix all 8 instances

In `internal/client/client.go`:
```go
// Line 61: "failed to create store directory: %v" → %w
// Line 68: "failed to connect to database: %v"    → %w
// Line 76: "failed to get device: %v"              → %w
// Line 110: "failed to connect: %v"                → %w
// Line 136: "failed to connect: %v"                → %w
```

In `internal/store/store.go`:
```go
// Line 97: "failed to create directory: %v"  → %w
// Line 102: "failed to open database: %v"    → %w
// Line 137: "failed to create tables: %v"    → %w
```

### Step 2: Run tests

```bash
go test ./... -race
```
Expected: All pass.

### Step 3: Commit

```bash
git add internal/client/client.go internal/store/store.go
git commit -m "fix: use %w for error wrapping to preserve error chains"
```

---

## Task 3: Typed read methods

Convert methods that return lists/query results. These are the easiest because they already pass through data from the store — just change the return type from `string` to `(T, error)` and stop calling `output.Success()`/`output.Error()`.

**Methods to convert:**
- `ListMessages` → `([]store.Message, error)` — currently returns `output.Success(messages)` where `messages` comes from `store.ListMessages`
- `SearchContacts` → `([]store.Contact, error)` — from `store.SearchContacts`
- `ListChats` → `([]store.Chat, error)` — from `store.ListChats`
- `GroupsList` → `([]types.GroupInfo, error)` — from `client.GetJoinedGroups`
- `GroupsInfo` → `(*types.GroupDetailedInfo, error)` — from `client.GetGroupInfo`
- `ListBlocked` → `([]string, error)` — from `client.GetBlocklist`
- `CheckOnWhatsApp` → `([]types.IsOnWhatsAppResponse, error)` — from `client.IsOnWhatsApp`

**Files:**
- Modify: `internal/commands/commands.go` — `ListMessages`, `SearchContacts`, `ListChats`
- Modify: `internal/commands/contacts.go` — `ListBlocked`, `CheckOnWhatsApp`
- Modify: `internal/commands/groups.go` — `GroupsList`, `GroupsInfo`
- Modify: `cmd/commands.go` — update Runner wrappers for these commands
- Modify: `internal/commands/mocks_test.go` — update mock calls if any
- Modify: test files as needed

**JSON contract:** These methods return arrays or objects that are already properly tagged via store/types structs. The JSON shape is unchanged because the same struct gets serialized — just in the transport layer now instead of the app layer.

### Step 1: Write failing test

Pick `ListMessages`. Test that calling it returns `([]store.Message, error)` instead of a JSON string.

### Step 2: Run test to verify it fails

```bash
go test ./internal/commands/ -run TestListMessages -v
```

### Step 3: Convert `ListMessages`

```go
// Before:
func (a *App) ListMessages(chatJID *string, query *string, limit, page int) string {
    messages, err := a.store.ListMessages(store.ListMessagesParams{...})
    if err != nil {
        return output.Error(err)
    }
    return output.Success(messages)
}

// After:
func (a *App) ListMessages(chatJID *string, query *string, limit, page int) (any, error) {
    messages, err := a.store.ListMessages(store.ListMessagesParams{...})
    if err != nil {
        return nil, err
    }
    if messages == nil {
        messages = []store.Message{}
    }
    return messages, nil
}
```

### Step 4: Update Runner wrapper in `cmd/commands.go`

```go
// Before:
return app.ListMessages(chatJID, query, limit, page), nil

// After:
return app.ListMessages(chatJID, query, limit, page)
```

### Step 5: Convert remaining read methods

Apply the same pattern to `SearchContacts`, `ListChats`, `GroupsList`, `GroupsInfo`, `ListBlocked`, `CheckOnWhatsApp`.

### Step 6: Run tests

```bash
go test ./... -race
```

### Step 7: Commit

```bash
git add internal/commands/commands.go internal/commands/contacts.go internal/commands/groups.go cmd/commands.go
git commit -m "refactor: return typed data from read methods, serialize in transport"
```

---

## Task 4: Typed mutation methods with contract-preserving result types

Convert methods that return `output.Success(map[string]interface{}{...})`. Each needs a result type whose `json` tags match the existing JSON contract exactly.

**Critical constraint:** The human formatters in `internal/output/human.go` unmarshal specific field names. Result type `json` tags MUST match:
- `sendData` expects: `sent`, `id`, `recipient`
- `reactData` expects: `message_id`, `emoji`
- `messageIDData` expects: `message_id`
- `jidData` expects: `jid`
- `mediaDownloadData` expects: `message_id`, `path`, `bytes`, `media_type`
- `inviteLinkData` expects: `jid`, `link`
- `groupInfoData` expects: `jid`, `name`

**Files:**
- Create: `internal/commands/results.go` — per-command result types
- Modify: `internal/commands/commands.go` — all Send/React/Delete/Edit/Mark/Download methods
- Modify: `internal/commands/contacts.go` — Block/Unblock
- Modify: `internal/commands/groups.go` — all mutation methods
- Modify: `cmd/commands.go` — update Runner wrappers

### Step 1: Define result types in `internal/commands/results.go`

```go
package commands

// SendResult is returned by SendMessage, SendImage, SendVideo, SendAudio, SendDocument.
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
    MessageID string `json:"message_id"`
    ChatJID   string `json:"chat_jid"`
    Deleted   bool   `json:"deleted,omitempty"`
    Edited    bool   `json:"edited,omitempty"`
    MarkedRead bool  `json:"marked_read,omitempty"`
    NewText   string `json:"new_text,omitempty"`
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
    JID string `json:"jid"`
    // Optional fields set per-command:
    Joined  bool     `json:"joined,omitempty"`
    Left    bool     `json:"left,omitempty"`
    Blocked bool     `json:"blocked,omitempty"`
    Unblocked bool   `json:"unblocked,omitempty"`
    Added   bool     `json:"added,omitempty"`
    Removed bool     `json:"removed,omitempty"`
    Updated bool     `json:"updated,omitempty"`
    Members []string `json:"members,omitempty"`
    Name    string   `json:"name,omitempty"`
    Description string `json:"description,omitempty"`
}

// InviteLinkResult is returned by GroupsInviteLink.
type InviteLinkResult struct {
    JID  string `json:"jid"`
    Link string `json:"link"`
}
```

### Step 2: Write tests for key result types

Test that marshaling a `SendResult` produces the same JSON shape as the current `map[string]interface{}`:
```go
func TestSendResultJSON(t *testing.T) {
    r := commands.SendResult{Sent: true, ID: "abc", Recipient: "123"}
    b, _ := json.Marshal(r)
    // Must contain "sent", "id", "recipient" keys (matching human formatter expectations)
    var m map[string]interface{}
    json.Unmarshal(b, &m)
    assert.Equal(t, true, m["sent"])
    assert.Equal(t, "abc", m["id"])
    assert.Equal(t, "123", m["recipient"])
}
```

### Step 3: Convert mutation methods

Example — `SendMessage`:
```go
// Before:
func (a *App) SendMessage(ctx context.Context, recipient, message string) string {
    // ... business logic ...
    return output.Success(map[string]interface{}{
        "sent": true, "id": msgID, "recipient": recipient, "message": message,
    })
}

// After:
func (a *App) SendMessage(ctx context.Context, recipient, message string) (any, error) {
    // ... business logic, return nil, err for errors ...
    return SendResult{
        Sent: true, ID: msgID, Recipient: recipient, Message: message,
    }, nil
}
```

Apply to all mutation methods: `SendMessage`, `SendReply`, `SendImage`, `SendVideo`, `SendAudio`, `SendDocument`, `ReactToMessage`, `DeleteMessage`, `EditMessage`, `MarkMessageRead`, `DownloadMedia`, `BlockContact`, `UnblockContact`, `GroupsCreate`, `GroupsInviteLink`, `GroupsJoin`, `GroupsLeave`, `GroupsAddMembers`, `GroupsRemoveMembers`, `GroupsSetName`, `GroupsSetDescription`, `GroupsSetPhoto`.

### Step 4: Update Runner wrappers

Same pattern as Task 3 — remove the `, nil` wrapper since methods now return `(any, error)`.

### Step 5: Run tests

```bash
go test ./... -race
```

Verify human formatters still work by running human output tests.

### Step 6: Commit

```bash
git add internal/commands/results.go internal/commands/commands.go internal/commands/contacts.go internal/commands/groups.go cmd/commands.go
git commit -m "refactor: typed result structs for mutation methods, preserving JSON contract"
```

---

## Task 5: Auth and Sync typed returns

These are special cases — Auth has side effects and Sync is a streaming command.

**Files:**
- Modify: `internal/commands/commands.go` — `Auth`, `Sync`
- Modify: `cmd/commands.go` — update Runner wrappers

### Step 1: Define result types

```go
type AuthResult struct {
    Authenticated bool   `json:"authenticated"`
    Message       string `json:"message"`
}

type SyncResult struct {
    Synced        bool  `json:"synced"`
    MessagesCount int64 `json:"messages_count"`
}
```

### Step 2: Convert Auth

```go
// Before: returns string via output.Success/output.Error
// After:
func (a *App) Auth(ctx context.Context) (any, error) {
    if a.client.IsAuthenticated() {
        return AuthResult{Authenticated: true, Message: "Already authenticated"}, nil
    }
    if err := a.client.Authenticate(ctx); err != nil {
        return nil, err
    }
    return AuthResult{Authenticated: true, Message: "Successfully authenticated"}, nil
}
```

### Step 3: Convert Sync

Sync is the most complex. It currently calls `output.Error(err)` in multiple places during event handling. After conversion, it returns `(any, error)`:

```go
func (a *App) Sync(ctx context.Context) (any, error) {
    // ... existing connect/repair/handler logic ...
    // Replace all `return output.Error(err)` with `return nil, err`
    // Replace final `return output.Success(map[...])` with:
    return SyncResult{Synced: true, MessagesCount: messageCount.Load()}, nil
}
```

### Step 4: Run tests

```bash
go test ./... -race
```

### Step 5: Commit

```bash
git add internal/commands/commands.go internal/commands/results.go cmd/commands.go
git commit -m "refactor: typed returns for Auth and Sync"
```

---

## Task 6: Delete the string migration bridge and old string helpers

At this point all App methods return `(any, error)`. Three things to remove:

1. **The `case string:` branch in `buildRunE`** (the temporary migration bridge from Task 1)
2. **`TestPrintResult_StringBridge_MatchesTypedPath`** (the canary test from Task 1 — its purpose is fulfilled)
3. **The old `output.Success(data) string` / `output.Error(err) string`** functions if any callers remain (there shouldn't be)

**Files:**
- Modify: `cmd/registry/register.go` — delete the `case string:` branch in `buildRunE`
- Modify: `cmd/registry/register_test.go` — delete `TestPrintResult_StringBridge_MatchesTypedPath`
- Modify: `internal/output/output.go` — verify no old string-returning helpers remain
- Verify: no imports of the old `output.Success`/`output.Error` string helpers in `internal/commands/`

### Step 1: Verify no legacy string callers remain

```bash
grep -rn 'output\.Error(' internal/commands/
```

Expected: zero matches. If any remain, convert them first — do not proceed until all App methods return `(any, error)`.

### Step 2: Delete the string bridge in `buildRunE`

Remove the `case string:` branch. After deletion, the switch becomes:

```go
switch v := result.(type) {
case output.Result:
    r.printResult(spec.ID, v)
default:
    r.printResult(spec.ID, output.Success(v))
}
```

Any Runner still returning a raw string will now be wrapped in `output.Success(string)` — which would produce `{"success":true,"data":"...","error":null}` and immediately fail tests. This is the intended tripwire.

### Step 3: Delete the bridge canary test

Remove `TestPrintResult_StringBridge_MatchesTypedPath` from `register_test.go`.

### Step 4: Clean up old string helpers if present

If `internal/output/output.go` still has the old `Success(data) string` or `Error(err) string` functions, remove them. The typed `Success(data any) Result` and `Failure(err error) Result` from Task 1 are the replacements.

### Step 5: Run tests

```bash
go test ./... -race
```

All tests must pass. Any failure here means a Runner was missed in Tasks 3-5.

### Step 6: Commit

```bash
git add cmd/registry/register.go cmd/registry/register_test.go internal/output/output.go
git commit -m "refactor: delete string migration bridge, output package is typed-only"
```

---

## Task 7: Split WAClient interface (optional, lower priority)

The WAClient interface in `internal/commands/interfaces.go` has 30 methods. Split into role-based interfaces at the consumer side.

**Note:** This is a meaningful improvement only if different consumers need different subsets. Currently `App` is the only consumer and uses all methods. Evaluate whether this is worth doing — if not, skip.

**If proceeding:**

Split into:
- `Authenticator` — `IsAuthenticated`, `Authenticate`, `GetOwnJID`
- `Messenger` — `Connect`, `SendMessage`, `SendTextReply`, `SendImageMessage`, `SendVideoMessage`, `SendAudioMessage`, `SendDocumentMessage`, `ReactToMessage`, `RevokeMessage`, `EditMessage`, `MarkRead`
- `MediaDownloader` — `DownloadMedia`
- `ContactManager` — `GetBlocklist`, `UpdateBlocklist`, `IsOnWhatsApp`
- `GroupManager` — group methods
- `Syncer` — `StartSync`, `ResolveChatName`

`App.client` stays as `WAClient` (which embeds all sub-interfaces). The benefit is that individual methods can accept narrower interfaces in their signatures, improving testability.

### Step 1: Define sub-interfaces

### Step 2: Have WAClient embed them

### Step 3: Update mocks if desired

### Step 4: Run tests

```bash
go test ./... -race
```

### Step 5: Commit

---

## Task 8: Type `ResolveChatName` parameter (cleanup)

`ResolveChatName` in `internal/client/client.go` accepts `evt interface{}` but only ever receives `*events.Message`. Change to the concrete type.

**Files:**
- Modify: `internal/client/client.go`
- Modify: `internal/commands/interfaces.go`
- Modify: `internal/commands/mocks_test.go`

### Step 1: Change signature

```go
// Before:
ResolveChatName(ctx context.Context, chatJID string, evt interface{}) string

// After:
ResolveChatName(ctx context.Context, chatJID string, evt *events.Message) string
```

### Step 2: Update all callers and mock

### Step 3: Run tests

```bash
go test ./... -race
```

### Step 4: Commit

```bash
git commit -m "refactor: type ResolveChatName parameter as *events.Message"
```

---

## Risks and Mitigations

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| JSON shape changes break agent consumers | Medium | High | Per-command result types with exact `json` tags. Run `go test ./... -race` after every change. |
| Human formatters break | Medium | High | Formatters unmarshal from `json.RawMessage` — as long as struct tags match, they work. Test coverage exists. |
| String backward-compat path in Task 1 masks bugs | Low | Medium | Remove it in Task 6 once all methods are converted. |
| Sync conversion breaks streaming behavior | Low | High | Sync's internal event handling doesn't change — only the return path. |

## Rollback Plan

Each task is a separate commit. `git revert <sha>` for any task.

## Open Questions

- [ ] Task 7 (interface split): evaluate after Tasks 1-6 whether it delivers meaningful benefit with App as the only consumer.
