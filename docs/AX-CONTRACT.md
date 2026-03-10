# Agent Experience (AX) Contract

Machine-readable contract for agents consuming whatsapp-cli.

## JSON Envelope

Every command writes exactly one JSON object to **stdout**:

```json
{"success": bool, "data": <any|null>, "error": <string|null>}
```

- `success: true` → `data` contains the result, `error` is `null`
- `success: false` → `data` is `null`, `error` contains a human-readable message

No other output appears on stdout. All progress, warnings, and diagnostics go to **stderr**.

## Exit Codes

| Code | Meaning | When |
|------|---------|------|
| 0 | Success | `success: true` in JSON |
| 1 | Runtime error | `success: false` in JSON (command ran but failed: network error, not found, permission denied) |
| 2 | Usage error | Command never ran: missing/invalid flags, unknown commands, validation failures (cobra layer) |

## Empty Lists

List commands return `[]`, never `null`, when there are no results:

```json
{"success": true, "data": [], "error": null}
```

Affected commands: `messages list`, `messages search`, `contacts search`, `chats list`, `groups list`, `contacts list-blocked`, `contacts check`.

## Data Shapes

### List commands

`data` is an array of objects. Shape depends on the entity:

**Messages** (`messages list`, `messages search`):
```json
[{"id": "str", "chat_jid": "str", "chat_name": "str", "sender": "str", "content": "str", "timestamp": "RFC3339", "is_from_me": bool, "media_type": "str"}]
```

**Chats** (`chats list`):
```json
[{"jid": "str", "name": "str", "last_message_time": "RFC3339"}]
```

**Contacts** (`contacts search`):
```json
[{"jid": "str", "name": "str", "phone_number": "str"}]
```

**Groups** (`groups list`):
```json
[{"jid": "str", "name": "str", "member_count": int, "members": [...], "created_at": int}]
```

**Blocked** (`contacts list-blocked`):
```json
["jid1@s.whatsapp.net", "jid2@s.whatsapp.net"]
```

### Mutation commands

`data` is an object with a boolean status field and identifiers for chaining:

| Command | Status field | Follow-up handles |
|---------|-------------|-------------------|
| `send` | `"sent": true` | `"id"` (message ID), `"recipient"` |
| `messages react` | `"reacted": true` | `"message_id"`, `"chat_jid"` |
| `messages delete` | `"deleted": true` | `"message_id"`, `"chat_jid"` |
| `messages edit` | `"edited": true` | `"message_id"`, `"chat_jid"` |
| `messages mark-read` | `"marked_read": true` | `"message_id"`, `"chat_jid"` |
| `contacts block` | `"blocked": true` | `"jid"` |
| `contacts unblock` | `"unblocked": true` | `"jid"` |
| `groups create` | full GroupInfo object | `"jid"` |
| `groups join` | `"joined": true` | `"jid"` |
| `groups leave` | `"left": true` | `"jid"` |
| `groups add-members` | `"added": true` | `"jid"`, `"members"` |
| `groups remove-members` | `"removed": true` | `"jid"`, `"members"` |
| `groups set-name` | `"updated": true` | `"jid"` |
| `groups set-description` | `"updated": true` | `"jid"` |
| `groups set-photo` | `"updated": true` | `"jid"` |
| `groups invite-link` | `"link": "str"` | `"jid"` |
| `media download` | `"message_id"`, `"path"`, `"bytes"` | `"path"` (local file) |

### Follow-up chaining

The `id` returned by `send` can be passed to `messages react --message-id`, `messages delete --message-id`, `messages edit --message-id`, or `messages mark-read --message-id`.

The `jid` returned by `groups create` or `groups join` can be passed to any `groups` subcommand via `--jid`.

### Query commands

| Command | Data shape |
|---------|-----------|
| `version` | `{"version": "str"}` |
| `auth` | `{"authenticated": true, "message": "str"}` |
| `groups info` | Single GroupInfo object |
| `contacts check` | Array of `{"query": "str", "jid": "str", "is_on_whatsapp": bool}` |

### Streaming: sync

`sync` writes progress to stderr and a single JSON summary to stdout on exit:

```json
{"success": true, "data": {"synced": true, "messages_count": int}, "error": null}
```

Stderr during sync: connection status, message count progress (`\r` overwritten), history sync notifications, media worker summary. Not machine-parseable.

## Stderr Usage

Stderr carries:
- Cobra help/usage text (on exit 2)
- Sync progress indicators (emoji prefixed, `\r` for in-place updates)
- Store warnings during sync (`failed to store chat/message`)
- Media worker summary (skipped, dropped, failed counts)

Agents should ignore stderr unless debugging. All actionable data is on stdout.

## Output Format

The `--format` flag controls stdout format. It is a persistent flag on the root command.

| Value | Behavior |
|-------|----------|
| `json` (default) | Raw JSON envelope. This is the agent contract — agents should not change this. |
| `human` | Human-readable formatted text. Tables for lists, key-value for objects, one-liners for mutations. |
| `auto` | TTY detection: terminal → `human`, pipe → `json`. |

Agents should not pass `--format` (or pass `--format json` explicitly). The `human` and `auto` modes are for interactive terminal use and are not part of the agent contract.

Note: `--output` on `media download` is a separate flag for the destination file path — it is unrelated to `--format`.

## Flag Conventions

- `--to` accepts either a bare phone number (`61412345678`) or a full JID (`61412345678@s.whatsapp.net`). No `+` prefix. The CLI normalizes automatically.
- `--phone` (on `contacts check`) accepts international format with `+` prefix (`+61412345678`).
- `--chat` and `--jid` accept full JIDs only.
- `--message-id` is the WhatsApp message ID string.
- `--limit` defaults to 20, `--page` defaults to 0 (first page).
- Boolean flags: `--ptt` defaults to true (use `--ptt=false` to disable).
