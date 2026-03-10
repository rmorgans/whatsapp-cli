# Human-Friendly Output Layer — Design

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a human-readable output mode so the CLI is usable in a terminal without piping through `jq`, while preserving the JSON agent contract as the default.

**Architecture:** Formatting is a presentation concern in the registry layer. Commands still return JSON strings. The registry parses the envelope once and dispatches to a formatter. JSON is always the default; `--output human` is opt-in.

**Tech Stack:** Go stdlib (`encoding/json`), `golang.org/x/term` (for TTY detection via `term.IsTerminal`). Single new dependency.

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
1. --format json    → ModeJSON  (explicit, default)
2. --format human   → ModeHuman (explicit opt-in)
3. --format auto    → TTY detection: terminal → ModeHuman, pipe → ModeJSON
4. no --format flag → ModeJSON  (AX contract preserved)
```

**Rationale:** JSON stays the default to preserve the AX contract documented in `docs/AX-CONTRACT.md`. Agents never need to pass `--format json` defensively. Humans opt in with `--format human` or `--format auto`. A shell alias (`alias wa='whatsapp-cli --format auto'`) gives humans the ergonomic default.

`--format` is a persistent flag on the root command, same level as `--store`. Named `--format` (not `--output`) to avoid collision with `media download --output` which specifies the destination file path.

**TTY detection:** Uses `term.IsTerminal(int(os.Stdout.Fd()))` from `golang.org/x/term`. Only consulted when `--format auto` is set.

## AX Contract Update

When this ships, `docs/AX-CONTRACT.md` gains one line:

> The `--format` flag controls stdout format. Default is `json`. Agents should not pass `--format` (or pass `--format json` explicitly). The `human` and `auto` modes are for interactive terminal use and are not part of the agent contract.

## printResult Changes

```
printResult(commandID, commandPath, result string):
    1. Parse JSON envelope → Envelope
       - If parse fails: print raw result, set exit code 1, return
    2. Derive exit code from parsed envelope:
       - envelope.Success == true  → exit 0
       - envelope.Success == false → exit 1
    3. If ModeJSON: print raw result (existing path, byte-identical), return
    4. If ModeHuman:
       a. Look up command formatter in map[commandID]Formatter
       b. If found: call it
          - If it returns error: fall through to (c)
       c. Generic formatter as fallback
          - If it returns error: print raw JSON result (last resort)
       d. Print formatted result to stdout
```

**Key change from current code:** Step 4 in the old `printResult` re-parsed the JSON to check `success`. Now step 2 derives exit code from the already-parsed Envelope. Single parse, no duplication.

**Invariants:**
- JSON mode output is byte-identical to current behavior
- Stderr behavior unchanged (progress, warnings stay on stderr)
- Human mode only changes stdout presentation
- Exit codes unchanged
- Parse failure → raw output + exit 1 (deterministic)

## Generic Formatter — Deterministic Spec

Handles any command without a custom formatter. Output is fully deterministic given the same input.

### Dispatch rules (checked in order):

1. **Error** (success=false): Print `Error: <error message>` to stdout.
2. **Null data** (success=true, data is JSON `null`): Print `No results.`
3. **Empty array** (success=true, data is `[]`): Print `No results.`
4. **Array of objects**: Render as table (see table rules below).
5. **Array of scalars**: One value per line, no header.
6. **Object**: Render as key-value block (see key-value rules below).
7. **Scalar** (string/number/bool): Print the value directly.

### Table rules (arrays of objects):

- **Column order**: Determined by key order in the *first* object in the array. JSON object key order from `encoding/json` is preserved (Go maps are random, but `json.RawMessage` preserves source order; we unmarshal to `[]map[string]interface{}` but iterate in insertion order via a helper).
- **Alternative**: Use an explicit column-priority list per known data shape. For unknown shapes, alphabetical order.
- **Column headers**: Uppercase of key name, underscores replaced with spaces. `chat_jid` → `CHAT JID`.
- **Column widths**: Auto-sized to max content width, capped at 40 chars. Values exceeding cap are truncated with `…`.
- **Total width**: Capped at 120 chars. Rightmost columns dropped if they don't fit.
- **Timestamps** (any value matching RFC3339): Render as relative time if <7 days (`2h ago`, `yesterday`, `3 days ago`), otherwise short date (`Mar 3`, `Jan 15 2025`).
- **Booleans**: `yes` / `no`.
- **Nested objects/arrays**: Render as `{...}` / `[...]` (collapsed, not expanded). These are rare in practice.
- **Null values**: Render as empty string (blank cell).

### Key-value rules (single objects):

- **Key order**: Same as source JSON key order (or alphabetical fallback).
- **Format**: `Key: value` one per line, key right-padded to align values.
- **Timestamps/booleans/nested**: Same rules as table cells.

## Per-Command Formatters (initial set)

Only where generic output is noticeably poor:

### `send` (and all send variants)
```
Sent to 61412345678 (ID: ABC123)
```

### `messages.list` / `messages.search`
```
TIME        CHAT            SENDER    MESSAGE
2h ago      Rick Morgans    me        Hey, how's it going?
yesterday   Family Group    Dad       See you Sunday
Mar 3       Work Chat       Alice     The deploy is done [Image]
```

### `chats.list`
```
LAST ACTIVE   NAME              JID
2h ago        Rick Morgans      120363422782024172@g.us
yesterday     Family Group      120363012345678@g.us
Mar 3         Work Chat         61412345678@s.whatsapp.net
```

### `groups.list`
```
NAME              MEMBERS   JID
Family Group      5         120363012345678@g.us
Work Chat         12        120363098765432@g.us
```

### `contacts.search`
```
NAME          PHONE           JID
John Doe      61412345678     61412345678@s.whatsapp.net
Jane Smith    61487654321     61487654321@s.whatsapp.net
```

### `media.download`
```
Downloaded image (1.2 MB) -> /tmp/photo.jpg
```

### Mutation one-liners (send, react, delete, edit, mark-read, block, groups)
```
Reacted 👍 to message ABC123
Deleted message ABC123
Edited message ABC123
Marked ABC123 as read
Blocked 61412345678@s.whatsapp.net
Created group "Family" (120363012345678@g.us)
Joined group (120363012345678@g.us)
```

### Everything else
Generic formatter handles it.

## File Structure

```
internal/output/
├── output.go          # Existing: Success(), Error(), Result type
├── envelope.go        # NEW: Envelope, RenderContext, OutputMode, Formatter type
├── generic.go         # NEW: generic formatter (table, key-value, one-liner)
├── generic_test.go    # NEW: deterministic tests for generic formatter
├── human.go           # NEW: per-command formatters + formatter registry
└── human_test.go      # NEW: tests for per-command formatters
```

## What Changes in Existing Files

- `cmd/registry/register.go`:
  - Add `--output` persistent flag (string, default "json", enum: json/human/auto)
  - Resolve OutputMode in `Execute()` before command runs
  - Change `printResult` to accept command ID/path, parse envelope once, dispatch
  - Registry holds `outputMode OutputMode` and `formatters map[string]Formatter`

- `cmd/registry/types.go` or `register.go`:
  - `LeafSpec` unchanged (no Format field)
  - Command ID already available in `buildRunE` via `spec.ID`
  - Command path available via `spec.Path.Segments()`

- `cmd/commands.go`: **No changes.**
- `internal/commands/*.go`: **No changes.**
- `go.mod`: Add `golang.org/x/term` dependency.

## Testing Strategy

- **Unit tests** for generic formatter: every dispatch rule (null, empty array, array of objects, array of scalars, object, scalar, error). Assert exact output strings.
- **Unit tests** for each per-command formatter: pass known Envelope, assert output string.
- **Unit tests** for table formatter: column ordering, truncation, timestamp rendering, boolean rendering, nested collapse, null cells.
- **Integration test**: build binary, run with `--output human`, verify non-JSON stdout.
- **Integration test**: build binary, run with no `--output`, verify JSON stdout (default preserved).
- **Existing tests unchanged**: they don't pass `--output`, so they get JSON (default).

## Risks

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| `x/term` not available | Very Low | Med | Widely used, stable, no CGO |
| Custom formatter panics | Low | High | Recover in dispatch, fall through to generic |
| Terminal width unknown | Low | Low | Default to 120 columns |
| New commands forget formatter | Expected | Low | Generic handles everything |
| JSON key order non-deterministic | Med | Low | Use ordered unmarshaling or explicit column lists |

## Non-Goals

- Color/ANSI styling (can add later, not in v1)
- Interactive/pager mode
- Changing any command's return value or error handling
- Changing stderr output
- Changing exit codes
- Making human mode the default (JSON stays default per AX contract)
