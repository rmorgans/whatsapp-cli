# Human-Friendly Output Layer — Design

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a human-readable output mode so the CLI is usable in a terminal without piping through `jq`, while preserving the JSON agent contract.

**Architecture:** Formatting is a presentation concern in the registry layer. Commands still return JSON strings. The registry parses the envelope once and dispatches to a formatter. TTY auto-detection picks the default mode; `--output` overrides.

**Tech Stack:** Go stdlib (`os.IsTerminal`), `encoding/json`, no new dependencies.

---

## Core Types

```go
// output/envelope.go
type Envelope struct {
    Success bool
    Data    json.RawMessage
    Error   *string
}

// output/render.go
type OutputMode int

const (
    ModeJSON  OutputMode = iota
    ModeHuman
)

type RenderContext struct {
    CommandID   string
    CommandPath string
    Mode        OutputMode
}

type Formatter func(ctx RenderContext, env Envelope) (string, error)
```

## Output Mode Resolution

```
1. --output json  → ModeJSON  (explicit override)
2. --output human → ModeHuman (explicit override)
3. stdout is TTY  → ModeHuman (auto-detect)
4. stdout is pipe → ModeJSON  (auto-detect)
```

`--output` is a persistent flag on the root command, same level as `--store`.

## printResult Changes

```
printResult(commandID, commandPath, result string):
    1. Parse JSON envelope → Envelope
       - If parse fails: print raw result, set exit code 1, return
    2. If ModeJSON: print raw result (existing path, byte-identical)
    3. If ModeHuman:
       a. Look up command formatter in map[commandID]Formatter
       b. If found: call it
          - If it errors: fall through to (c)
       c. Generic formatter as fallback
       d. Print formatted result to stdout
    4. Parse success field for exit code (existing logic)
```

**Invariants:**
- JSON mode output is byte-identical to current behavior
- Stderr behavior unchanged (progress, warnings stay on stderr)
- Human mode only changes stdout presentation
- Exit codes unchanged

## Generic Formatter

Handles any command without a custom formatter:

| Data shape | Rendering |
|-----------|-----------|
| `null` | (nothing, or "No results.") |
| Array of objects | Table with column headers from keys |
| Array of scalars | One per line |
| Object with status bool | One-liner: "Done." / key-value summary |
| Scalar | Plain text |

**Errors** (success=false): Print error message to stdout, styled if terminal supports it.

**Table formatting rules:**
- Column widths: auto-sized to content, max 40 chars per column, truncate with `…`
- Timestamps: relative when <7 days ("2h ago", "yesterday"), date otherwise
- Long content fields: truncate to terminal width
- Boolean fields: render as `yes`/`no`

## Per-Command Formatters (initial set)

Only where generic output is noticeably poor:

### `send` (and all send variants)
```
✓ Sent to 61412345678 (ID: ABC123)
```

### `messages list` / `messages search`
```
  TIME        CHAT            SENDER    MESSAGE
  2h ago      Rick Morgans    me        Hey, how's it going?
  yesterday   Family Group    Dad       See you Sunday
  Mar 3       Work Chat       Alice     The deploy is done [Image]
```

### `chats list`
```
  LAST ACTIVE   NAME              JID
  2h ago        Rick Morgans      120363422782024172@g.us
  yesterday     Family Group      120363012345678@g.us
  Mar 3         Work Chat         61412345678@s.whatsapp.net
```

### `groups list`
```
  NAME              MEMBERS   JID
  Family Group      5         120363012345678@g.us
  Work Chat         12        120363098765432@g.us
```

### `contacts search`
```
  NAME          PHONE           JID
  John Doe      61412345678     61412345678@s.whatsapp.net
  Jane Smith    61487654321     61487654321@s.whatsapp.net
```

### `media download`
```
✓ Downloaded image (1.2 MB) → /tmp/photo.jpg
```

### Mutation one-liners
```
messages react:     ✓ Reacted 👍 to message ABC123
messages delete:    ✓ Deleted message ABC123
messages edit:      ✓ Edited message ABC123
messages mark-read: ✓ Marked ABC123 as read
contacts block:     ✓ Blocked 61412345678@s.whatsapp.net
groups create:      ✓ Created group "Family" (120363012345678@g.us)
groups join:        ✓ Joined group (120363012345678@g.us)
```

### Everything else
Generic formatter handles it.

## File Structure

```
internal/output/
├── output.go          # Existing: Success(), Error(), Result type
├── envelope.go        # NEW: Envelope, RenderContext, OutputMode, Formatter type
├── generic.go         # NEW: generic formatter (table, key-value, one-liner)
├── human.go           # NEW: per-command formatters + registry
└── human_test.go      # NEW: tests for formatters
```

## What Changes in Existing Files

- `cmd/registry/register.go`:
  - Add `--output` persistent flag
  - Add TTY detection in `Execute()` or `buildRunE()`
  - Change `printResult` to accept command ID/path and dispatch
  - Registry holds `outputMode OutputMode` and `formatters map[string]Formatter`

- `cmd/registry/types.go` or `register.go`:
  - `LeafSpec` unchanged (no Format field)
  - Command ID already available in `buildRunE` via `spec.ID`
  - Command path available via `spec.Path.Segments()`

- `cmd/commands.go`: **No changes.**
- `internal/commands/*.go`: **No changes.**

## Testing Strategy

- **Unit tests** for each formatter: pass known Envelope, assert output string
- **Unit tests** for generic formatter: arrays, objects, scalars, nulls, errors
- **Integration test**: build binary, run with `--output human`, verify non-JSON stdout
- **Integration test**: build binary, pipe to `cat`, verify JSON output (TTY detection)
- **Existing tests unchanged**: they don't check stdout format (they parse JSON)

## Risks

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| TTY detection wrong in CI | Low | Med | `--output json` always overrides |
| Custom formatter panics | Low | High | Recover in dispatch, fall through to generic |
| Terminal width unknown | Low | Low | Default to 80 columns |
| New commands forget formatter | Expected | Low | Generic handles everything |

## Non-Goals

- Color/ANSI styling (can add later, not in v1)
- Interactive/pager mode
- Changing any command's return value or error handling
- Changing stderr output
- Changing exit codes
