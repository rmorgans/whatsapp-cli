# Full whatsmeow Wrapping — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Expose the whatsmeow API through whatsapp-cli using a DRY command registry, with a coverage audit tool to detect API drift after dependency bumps. Coverage targets `whatsmeow.Client` methods directly; composite operations (app state patches, multi-step uploads) are tracked separately.

**Architecture:** Replace per-command cobra boilerplate with a declarative registry. Each command is a spec (path, flags, validation, runner). Shared wiring handles cobra setup, app init, JSON stdout, stderr help, exit codes. A separate `go generate` audit tool compares registry coverage against `whatsmeow.Client` methods.

**Tech Stack:** Go 1.25, spf13/cobra, go.mau.fi/whatsmeow (Mar 2026), SQLite

---

## Part 1: Command Registry Framework

### Task 1: Define registry types

**Design principle:** Make invalid states unrepresentable in the type system. Where structural invariants can be enforced by types, use types. Where invariants are cross-field or business-level, validate at registration time (startup panic).

**Files:**
- Create: `cmd/registry/types.go`
- Create: `cmd/registry/flags.go`

**Step 1: Write the types**

```go
package registry

import (
    "context"
    "fmt"
    "time"

    "github.com/vicentereig/whatsapp-cli/internal/commands"
)

// --- Path: non-empty by construction ---

// CommandPath represents a validated, non-empty command path.
// Cannot be constructed with zero segments.
type CommandPath struct {
    parents []string // e.g. ["groups"] for "groups info"
    name    string   // e.g. "info"
}

// NewPath creates a CommandPath. Panics if no segments provided.
func MustNewPath(segments ...string) CommandPath {
    if len(segments) == 0 {
        panic("registry: CommandPath requires at least one segment")
    }
    return CommandPath{
        parents: segments[:len(segments)-1],
        name:    segments[len(segments)-1],
    }
}

func (p CommandPath) Name() string      { return p.name }
func (p CommandPath) Parents() []string  { return p.parents }
func (p CommandPath) Key() string {
    key := ""
    for _, s := range p.parents {
        key += s + "."
    }
    return key + p.name
}

// --- Execution mode: contradictory states eliminated ---

// ExecMode determines how the command runs. Exactly one of:
//   - Bounded(timeout) — normal command with timeout (0 = default 5m)
//   - Streaming() — long-running, cancelled by signal (sync)
//   - Local() — no app init needed (version)
type ExecMode struct {
    streaming bool
    local     bool
    timeout   time.Duration
}

func Bounded(timeout time.Duration) ExecMode { return ExecMode{timeout: timeout} }
func Streaming() ExecMode                    { return ExecMode{streaming: true} }
func Local() ExecMode                        { return ExecMode{local: true} }

func (m ExecMode) IsStreaming() bool      { return m.streaming }
func (m ExecMode) IsLocal() bool          { return m.local }
func (m ExecMode) NeedsApp() bool         { return !m.local }
func (m ExecMode) Timeout() time.Duration { return m.timeout }

// --- Flags: typed interface, capabilities match kind ---

// Flag is implemented by each flag kind.
// Only string flags can have Enum. Only value flags can have DependsOn.
// Mutual exclusion lives on the command, not on flags.
type Flag interface {
    flagName() string
    isRequired() bool
    isHidden() bool
    apply(cmd interface{}) // applies to cobra.Command
}

// StringFlag — optional Enum constraint.
type StringFlag struct {
    Name     string
    Short    string
    Help     string
    Default  string
    Required bool
    Hidden   bool
    Enum     []string // valid values; empty = any
}

func (f StringFlag) flagName() string  { return f.Name }
func (f StringFlag) isRequired() bool  { return f.Required }
func (f StringFlag) isHidden() bool    { return f.Hidden }

// IntFlag — no Enum, no mutual exclusion.
type IntFlag struct {
    Name     string
    Short    string
    Help     string
    Default  int
    Required bool
    Hidden   bool
}

func (f IntFlag) flagName() string  { return f.Name }
func (f IntFlag) isRequired() bool  { return f.Required }
func (f IntFlag) isHidden() bool    { return f.Hidden }

// BoolFlag — no Enum, no Default+Required (bool always has a default).
type BoolFlag struct {
    Name    string
    Short   string
    Help    string
    Default bool
    Hidden  bool
    // Note: no Required — a bool flag always has a value (its default).
}

func (f BoolFlag) flagName() string  { return f.Name }
func (f BoolFlag) isRequired() bool  { return false } // bools are never required
func (f BoolFlag) isHidden() bool    { return f.Hidden }

// StringSliceFlag — for multi-value flags like --members.
type StringSliceFlag struct {
    Name     string
    Short    string
    Help     string
    Default  []string
    Required bool
    Hidden   bool
}

func (f StringSliceFlag) flagName() string  { return f.Name }
func (f StringSliceFlag) isRequired() bool  { return f.Required }
func (f StringSliceFlag) isHidden() bool    { return f.Hidden }

// --- Flag relationships: command-level, not flag-level ---

// RuleSpec declares cross-flag constraints at the command level.
type RuleSpec struct {
    // ExactlyOneOf: exactly one of these flags must be set.
    // e.g. []string{"message", "image", "video"} on send.
    ExactlyOneOf [][]string

    // AtMostOneOf: at most one of these flags may be set.
    // e.g. []string{"image", "video"} — neither is required.
    AtMostOneOf [][]string

    // Requires: if key flag is set, value flags must also be set.
    // e.g. {"caption": ["image"]} — --caption requires --image.
    Requires map[string][]string
}

// --- FlagValues: typed access to parsed values ---

type FlagValues interface {
    String(name string) string
    Int(name string) int
    Bool(name string) bool
    StringSlice(name string) []string
    IsSet(name string) bool
}

// --- Runner: always non-nil on leaf commands ---

// Runner executes a leaf command. ctx is pre-configured (bounded or streaming).
// app is nil only for Local exec mode.
type Runner func(ctx context.Context, app *commands.App, f FlagValues) (string, error)

// --- Doc: documentation fields ---

type DocSpec struct {
    Short      string
    Long       string
    Examples   []string
    Aliases    []string
    Hidden     bool
    Deprecated string
}

// --- Specs: separate types for parent and leaf ---

// ParentSpec declares a non-leaf command (groups, messages, etc.).
// No Run, no Flags, no ExecMode — parents only provide help text
// and route to subcommands.
type ParentSpec struct {
    Path   CommandPath
    Short  string
    Long   string
    Hidden bool
}

// LeafSpec declares an executable command.
// Run is always non-nil (enforced by constructor).
type LeafSpec struct {
    ID    string
    Path  CommandPath
    Doc   DocSpec
    Exec  ExecMode
    Flags []Flag
    Rules RuleSpec
    Run   Runner // never nil

    // Validate runs after flag parsing, before side effects.
    // For cross-flag business logic that RuleSpec can't express.
    Validate func(f FlagValues) error
}

// NewLeafSpec creates a LeafSpec. Panics if Run is nil.
func MustNewLeafSpec(id string, path CommandPath, run Runner) LeafSpec {
    if run == nil {
        panic(fmt.Sprintf("registry: LeafSpec %q requires a non-nil Run", id))
    }
    return LeafSpec{
        ID:   id,
        Path: path,
        Run:  run,
    }
}
```

**What's unrepresentable now (enforced by types):**

| Invalid state | How it's prevented |
|---|---|
| Empty path | `NewPath` panics on zero segments |
| `LongRunning + Timeout` | `ExecMode` is one of `Bounded`, `Streaming`, `Local` — can't combine |
| Nil Run on leaf | `NewLeafSpec` panics |
| Parent with Run/Flags | `ParentSpec` has no Run or Flags fields |
| Enum on BoolFlag | `BoolFlag` has no Enum field |
| Required BoolFlag | `BoolFlag` has no Required field (always has a default) |
| Mutual exclusion on individual flags | `RuleSpec.ExactlyOneOf` / `AtMostOneOf` on command, not flag |
| DependsOn on individual flags | `RuleSpec.Requires` on command, not flag |

**What's validated at registration time (startup panic):**

| Invalid state | Validation |
|---|---|
| Duplicate command ID | Check `ids` map |
| Duplicate command path | Check `parents` map + leaf paths |
| Duplicate alias collides with existing path | Check all registered paths |
| Duplicate flag names within a command | Check at registration |
| `Required` StringFlag with non-zero default | Check at registration |
| `ExactlyOneOf` / `AtMostOneOf` references unknown flag | Check all names exist in Flags |
| `Requires` references unknown flag | Check all names exist in Flags |
| Empty rule group (`ExactlyOneOf: [][]string{{}}`) | Reject groups with < 2 entries |
| Duplicate name inside a rule group | Reject `[]string{"a", "a"}` |
| Same flag in contradictory rules | Reject flag in both ExactlyOneOf and AtMostOneOf groups |

**Step 2: Verify it compiles**

Run: `go build ./cmd/registry/`

**Step 3: Commit**

```bash
git add cmd/registry/types.go cmd/registry/flags.go
git commit -m "feat(registry): add type-safe command and flag specs"
```

---

### Task 2: Implement FlagValues adapter

**Files:**
- Create: `cmd/registry/flags.go`

**Step 1: Write the cobra FlagValues adapter**

```go
package registry

import "github.com/spf13/cobra"

// cobraFlags adapts a cobra command's flag set to FlagValues.
type cobraFlags struct {
    cmd *cobra.Command
}

func NewFlagValues(cmd *cobra.Command) FlagValues {
    return &cobraFlags{cmd: cmd}
}

func (f *cobraFlags) String(name string) string {
    v, _ := f.cmd.Flags().GetString(name)
    return v
}

func (f *cobraFlags) Int(name string) int {
    v, _ := f.cmd.Flags().GetInt(name)
    return v
}

func (f *cobraFlags) Bool(name string) bool {
    v, _ := f.cmd.Flags().GetBool(name)
    return v
}

func (f *cobraFlags) StringSlice(name string) []string {
    v, _ := f.cmd.Flags().GetStringSlice(name)
    return v
}

func (f *cobraFlags) IsSet(name string) bool {
    return f.cmd.Flags().Changed(name)
}
```

**Step 2: Commit**

```bash
git add cmd/registry/flags.go
git commit -m "feat(registry): implement cobra FlagValues adapter"
```

---

### Task 3: Implement the wiring engine

**Files:**
- Create: `cmd/registry/register.go`

This is the core: takes a `CommandSpec`, creates cobra commands, wires flags, validation, app init, JSON output, exit codes.

**Step 1: Write the wiring**

```go
package registry

import (
    "context"
    "encoding/json"
    "fmt"
    "os"
    "os/signal"
    "path/filepath"
    "syscall"
    "time"

    "github.com/spf13/cobra"
    "github.com/vicentereig/whatsapp-cli/internal/commands"
)

const defaultTimeout = 5 * time.Minute

// Registry holds all execution state. No package globals.
// Create one per process via NewRegistry().
type Registry struct {
    version  string
    storeDir string
    app      *commands.App
    exitCode int
    root     *cobra.Command
    parents  map[string]*cobra.Command
}

func NewRegistry() *Registry {
    r := &Registry{
        version: "dev",
        parents: make(map[string]*cobra.Command),
    }
    r.root = r.newRootCmd()
    return r
}

func (r *Registry) SetVersion(v string) { r.version = v }
func (r *Registry) GetVersion() string  { return r.version }
func (r *Registry) Root() *cobra.Command { return r.root }

// ParentSpec declares metadata for a parent (non-leaf) command.
type ParentSpec struct {
    Path  []string
    Short string
    Long  string
}

func errorJSON(msg string) string {
    escaped, _ := json.Marshal(msg)
    return fmt.Sprintf(`{"success":false,"data":null,"error":%s}`, escaped)
}

func printResult(result string) {
    fmt.Println(result)
    var envelope struct {
        Success bool `json:"success"`
    }
    if err := json.Unmarshal([]byte(result), &envelope); err != nil || !envelope.Success {
        exitCode = 1
    }
}

func initApp() error {
    if app != nil {
        return nil
    }
    absStore, err := filepath.Abs(storeDir)
    if err != nil {
        return fmt.Errorf("invalid store path: %w", err)
    }
    app, err = commands.NewApp(absStore, version)
    if err != nil {
        return fmt.Errorf("failed to initialize: %w", err)
    }
    return nil
}

func closeApp() {
    if app != nil {
        app.Close()
        app = nil
    }
}

func newContext(longRunning bool, timeout time.Duration) (context.Context, context.CancelFunc) {
    if longRunning {
        ctx, cancel := context.WithCancel(context.Background())
        sigChan := make(chan os.Signal, 1)
        signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
        go func() {
            <-sigChan
            cancel()
        }()
        return ctx, cancel
    }
    if timeout == 0 {
        timeout = defaultTimeout
    }
    return context.WithTimeout(context.Background(), timeout)
}

// RegisterParent registers a parent command with explicit metadata.
func RegisterParent(root *cobra.Command, spec ParentSpec) {
    key := joinPath(spec.Path)
    if _, exists := parents[key]; exists {
        return
    }
    cmd := &cobra.Command{
        Use:   spec.Path[len(spec.Path)-1],
        Short: spec.Short,
        Long:  spec.Long,
        Args:  cobra.NoArgs,
        RunE: func(cmd *cobra.Command, args []string) error {
            return fmt.Errorf("requires a subcommand; see --help")
        },
    }
    cmd.SetOut(os.Stderr)
    cmd.SetErr(os.Stderr)
    parent := ensureParent(root, spec.Path[:len(spec.Path)-1])
    parent.AddCommand(cmd)
    parents[key] = cmd
}

// Register wires a CommandSpec into a cobra command tree.
func Register(root *cobra.Command, spec CommandSpec) {
    leaf := &cobra.Command{
        Use:        spec.Path[len(spec.Path)-1],
        Short:      spec.Short,
        Long:       spec.Long,
        Aliases:    spec.Aliases,
        Args:       cobra.NoArgs,
        Hidden:     spec.Hidden,
        Deprecated: spec.Deprecated,
    }

    // Add examples
    if len(spec.Examples) > 0 {
        for _, ex := range spec.Examples {
            leaf.Example += ex + "\n"
        }
    }

    // Register flags
    for _, f := range spec.Flags {
        switch f.Type {
        case String:
            leaf.Flags().StringP(f.Name, f.Short, f.StringDefault, f.Help)
        case Int:
            leaf.Flags().IntP(f.Name, f.Short, f.IntDefault, f.Help)
        case Bool:
            leaf.Flags().BoolP(f.Name, f.Short, f.BoolDefault, f.Help)
        case StringSlice:
            leaf.Flags().StringSliceP(f.Name, f.Short, f.StringSliceDefault, f.Help)
        }
        if f.Required {
            leaf.MarkFlagRequired(f.Name)
        }
        if f.Hidden {
            leaf.Flags().MarkHidden(f.Name)
        }
    }

    leaf.SetOut(os.Stderr)
    leaf.SetErr(os.Stderr)

    // Wire RunE
    if spec.Run != nil {
        leaf.RunE = buildRunE(spec)
    }

    // Attach to parent
    parent := ensureParent(root, spec.Path[:len(spec.Path)-1])
    parent.AddCommand(leaf)
}

func buildRunE(spec CommandSpec) func(*cobra.Command, []string) error {
    return func(cmd *cobra.Command, args []string) error {
        fv := NewFlagValues(cmd)

        // Validate mutual exclusion and dependencies
        if err := validateFlagRelationships(cmd, spec.Flags); err != nil {
            return err
        }

        // Validate enum constraints
        if err := validateEnums(fv, spec.Flags); err != nil {
            return err
        }

        // Custom validation
        if spec.Validate != nil {
            if err := spec.Validate(fv); err != nil {
                return err
            }
        }

        // Local commands (no app init)
        if spec.Exec.IsLocal() {
            result, err := spec.Run(context.Background(), nil, fv)
            if err != nil {
                r.printResult(errorJSON(err.Error()))
                return nil
            }
            r.printResult(result)
            return nil
        }

        // Init app
        if err := r.initApp(); err != nil {
            r.printResult(errorJSON(err.Error()))
            return nil
        }
        defer r.closeApp()

        ctx, cancel := newContext(spec.Exec.IsStreaming(), spec.Exec.Timeout())
        defer cancel()

        result, err := spec.Run(ctx, r.app, fv)
        if err != nil {
            r.printResult(errorJSON(err.Error()))
            return nil
        }
        r.printResult(result)
        return nil
    }
}

// validateRules checks ExactlyOneOf, AtMostOneOf, and Requires constraints.
func validateRules(cmd *cobra.Command, rules RuleSpec) error {
    for _, group := range rules.ExactlyOneOf {
        count := 0
        for _, name := range group {
            if cmd.Flags().Changed(name) {
                count++
            }
        }
        if count != 1 {
            return fmt.Errorf("exactly one of --%s is required", strings.Join(group, ", --"))
        }
    }
    for _, group := range rules.AtMostOneOf {
        count := 0
        for _, name := range group {
            if cmd.Flags().Changed(name) {
                count++
            }
        }
        if count > 1 {
            return fmt.Errorf("--%s are mutually exclusive", strings.Join(group, ", --"))
        }
    }
    for flag, deps := range rules.Requires {
        if !cmd.Flags().Changed(flag) {
            continue
        }
        for _, dep := range deps {
            if !cmd.Flags().Changed(dep) {
                return fmt.Errorf("--%s requires --%s", flag, dep)
            }
        }
    }
    return nil
}

// validateEnums checks Enum constraints on StringFlag types.
func validateEnums(fv FlagValues, flags []Flag) error {
    for _, f := range flags {
        sf, ok := f.(StringFlag)
        if !ok || len(sf.Enum) == 0 || !fv.IsSet(sf.Name) {
            continue
        }
        val := fv.String(sf.Name)
        valid := false
        for _, allowed := range sf.Enum {
            if val == allowed {
                valid = true
                break
            }
        }
        if !valid {
            return fmt.Errorf("--%s must be one of: %v (got %q)", sf.Name, sf.Enum, val)
        }
    }
    return nil
}

// ensureParent finds or creates parent commands along the path.
func (r *Registry) ensureParent(parents []string) *cobra.Command {
    if len(parents) == 0 {
        return r.root
    }
    key := strings.Join(parents, ".")
    if cmd, ok := r.parents[key]; ok {
        return cmd
    }
    // Auto-create parent (RegisterParent preferred for good help text)
    parent := r.ensureParent(parents[:len(parents)-1])
    cmd := &cobra.Command{
        Use:   parents[len(parents)-1],
        Short: parents[len(parents)-1] + " commands",
        Args:  cobra.NoArgs,
        RunE: func(cmd *cobra.Command, args []string) error {
            return fmt.Errorf("requires a subcommand; see --help")
        },
    }
    cmd.SetOut(os.Stderr)
    cmd.SetErr(os.Stderr)
    parent.AddCommand(cmd)
    r.parents[key] = cmd
    return cmd
}
```

**Registry struct — all state is instance-local:**

```go
// Registry holds all execution state. No package globals.
type Registry struct {
    version  string
    storeDir string
    app      *commands.App
    exitCode int
    root     *cobra.Command
    parents  map[string]*cobra.Command
    ids      map[string]bool // duplicate ID detection
    paths    map[string]bool // duplicate path detection
}

func NewRegistry() *Registry {
    r := &Registry{
        version: "dev",
        parents: make(map[string]*cobra.Command),
        ids:     make(map[string]bool),
        paths:   make(map[string]bool),
    }
    r.root = r.newRootCmd()
    return r
}

func (r *Registry) SetVersion(v string) { r.version = v }
func (r *Registry) GetVersion() string  { return r.version }
func (r *Registry) Root() *cobra.Command { return r.root }

func (r *Registry) Execute() {
    if err := r.root.Execute(); err != nil {
        fmt.Println(errorJSON(err.Error()))
        os.Exit(2)
    }
    if r.exitCode != 0 {
        os.Exit(r.exitCode)
    }
}
```

All mutating functions (`printResult`, `initApp`, `closeApp`, `Register`, `RegisterParent`, `ensureParent`) are receiver methods on `*Registry`. `errorJSON` and `newContext` remain package-level (pure functions).

**Registration-time validation** (in `Register`):

```go
func (r *Registry) Register(spec LeafSpec) {
    // Structural: already unrepresentable (nil Run panics in NewLeafSpec)
    // Cross-field: validate here
    if r.ids[spec.ID] {
        panic(fmt.Sprintf("registry: duplicate command ID %q", spec.ID))
    }
    r.ids[spec.ID] = true

    flagNames := make(map[string]bool)
    for _, f := range spec.Flags {
        name := f.flagName()
        if flagNames[name] {
            panic(fmt.Sprintf("registry: %s has duplicate flag --%s", spec.ID, name))
        }
        flagNames[name] = true
    }

    // Validate path uniqueness
    pathKey := spec.Path.Key()
    if r.paths[pathKey] {
        panic(fmt.Sprintf("registry: duplicate path %q", pathKey))
    }
    r.paths[pathKey] = true

    // Validate rule groups
    validateRuleGroups := func(label string, groups [][]string) {
        for i, group := range groups {
            if len(group) < 2 {
                panic(fmt.Sprintf("registry: %s %s[%d] needs at least 2 flags", spec.ID, label, i))
            }
            seen := make(map[string]bool)
            for _, name := range group {
                if !flagNames[name] {
                    panic(fmt.Sprintf("registry: %s %s references unknown flag --%s", spec.ID, label, name))
                }
                if seen[name] {
                    panic(fmt.Sprintf("registry: %s %s has duplicate --%s", spec.ID, label, name))
                }
                seen[name] = true
            }
        }
    }
    validateRuleGroups("ExactlyOneOf", spec.Rules.ExactlyOneOf)
    validateRuleGroups("AtMostOneOf", spec.Rules.AtMostOneOf)

    // Check for contradictory rules (same flag in both ExactlyOneOf and AtMostOneOf)
    exactFlags := make(map[string]bool)
    for _, group := range spec.Rules.ExactlyOneOf {
        for _, name := range group {
            exactFlags[name] = true
        }
    }
    for _, group := range spec.Rules.AtMostOneOf {
        for _, name := range group {
            if exactFlags[name] {
                panic(fmt.Sprintf("registry: %s flag --%s in both ExactlyOneOf and AtMostOneOf", spec.ID, name))
            }
        }
    }

    // Validate Requires references
    for flag, deps := range spec.Rules.Requires {
        if !flagNames[flag] {
            panic(fmt.Sprintf("registry: %s Requires references unknown flag --%s", spec.ID, flag))
        }
        for _, dep := range deps {
            if !flagNames[dep] {
                panic(fmt.Sprintf("registry: %s Requires[--%s] references unknown flag --%s", spec.ID, flag, dep))
            }
        }
    }

    // Wire cobra command...
}
```

**Step 2: Verify it compiles**

Run: `go build ./cmd/registry/`

**Step 3: Commit**

```bash
git add cmd/registry/register.go
git commit -m "feat(registry): implement command wiring engine"
```

---

### Task 4: Migrate existing commands to registry

**Files:**
- Create: `cmd/commands.go` — all command registrations
- Modify: `cmd/root.go` — remove per-command wiring, use registry
- Delete: `cmd/auth.go`, `cmd/sync.go`, `cmd/version.go`, `cmd/send.go`, `cmd/messages.go`, `cmd/contacts.go`, `cmd/chats.go`, `cmd/media.go`

**Step 1: Create commands.go with all existing commands as registry specs**

```go
package cmd

import (
    "context"
    "encoding/json"
    "fmt"

    r "github.com/vicentereig/whatsapp-cli/cmd/registry"
    "github.com/vicentereig/whatsapp-cli/internal/commands"
)

func registerAll(reg *r.Registry) {
    // Parents (explicit metadata — no auto-generated help text)
    reg.RegisterParent(r.ParentSpec{
        Path:  r.MustNewPath("messages"),
        Short: "List and search messages",
    })
    reg.RegisterParent(r.ParentSpec{
        Path:  r.MustNewPath("chats"),
        Short: "List chats",
    })
    reg.RegisterParent(r.ParentSpec{
        Path:  r.MustNewPath("contacts"),
        Short: "Search contacts",
    })
    reg.RegisterParent(r.ParentSpec{
        Path:  r.MustNewPath("media"),
        Short: "Download media attachments",
    })

    // version — Local exec mode (no app init)
    spec := r.MustNewLeafSpec("version", r.MustNewPath("version"),
        func(ctx context.Context, app *commands.App, f r.FlagValues) (string, error) {
            v, _ := json.Marshal(reg.GetVersion())
            return fmt.Sprintf(`{"success":true,"data":{"version":%s},"error":null}`, v), nil
        },
    )
    spec.Doc.Short = "Print CLI version information"
    spec.Exec = r.Local()
    reg.Register(spec)

    // auth — Bounded exec (default timeout)
    spec = r.MustNewLeafSpec("auth", r.MustNewPath("auth"),
        func(ctx context.Context, app *commands.App, f r.FlagValues) (string, error) {
            return app.Auth(ctx), nil
        },
    )
    spec.Doc.Short = "Authenticate with WhatsApp (scan QR code)"
    spec.Exec = r.Bounded(0) // default timeout
    reg.Register(spec)

    // sync — Streaming exec (signal-based cancel)
    spec = r.MustNewLeafSpec("sync", r.MustNewPath("sync"),
        func(ctx context.Context, app *commands.App, f r.FlagValues) (string, error) {
            return app.Sync(ctx), nil
        },
    )
    spec.Doc.Short = "Sync messages continuously (run until Ctrl+C)"
    spec.Exec = r.Streaming()
    reg.Register(spec)

    // send — ExactlyOneOf for content type, Requires for caption
    spec = r.MustNewLeafSpec("send", r.MustNewPath("send"),
        func(ctx context.Context, app *commands.App, f r.FlagValues) (string, error) {
            if f.IsSet("image") {
                return app.SendImage(ctx, f.String("to"), f.String("image"), f.String("caption")), nil
            }
            return app.SendMessage(ctx, f.String("to"), f.String("message")), nil
        },
    )
    spec.Doc.Short = "Send a message or image"
    spec.Exec = r.Bounded(0)
    spec.Flags = []r.Flag{
        r.StringFlag{Name: "to", Required: true, Help: "recipient JID or phone number"},
        r.StringFlag{Name: "message", Help: "message text"},
        r.StringFlag{Name: "image", Help: "image file path"},
        r.StringFlag{Name: "caption", Help: "image caption"},
    }
    spec.Rules = r.RuleSpec{
        ExactlyOneOf: [][]string{{"message", "image"}},
        Requires:     map[string][]string{"caption": {"image"}},
    }
    reg.Register(spec)

    // messages list
    // Real signature: App.ListMessages(chatJID *string, query *string, limit, page int) string
    spec = r.MustNewLeafSpec("messages.list", r.MustNewPath("messages", "list"),
        func(ctx context.Context, app *commands.App, f r.FlagValues) (string, error) {
            return app.ListMessages(optStr(f, "chat"), nil, f.Int("limit"), f.Int("page")), nil
        },
    )
    spec.Doc.Short = "List recent messages"
    spec.Exec = r.Bounded(0)
    spec.Flags = []r.Flag{
        r.StringFlag{Name: "chat", Help: "chat JID to filter by"},
        r.IntFlag{Name: "limit", Default: 20, Help: "maximum messages to return"},
        r.IntFlag{Name: "page", Help: "page number"},
    }
    reg.Register(spec)

    // messages search — uses same App.ListMessages with query param set
    spec = r.MustNewLeafSpec("messages.search", r.MustNewPath("messages", "search"),
        func(ctx context.Context, app *commands.App, f r.FlagValues) (string, error) {
            q := f.String("query")
            return app.ListMessages(nil, &q, f.Int("limit"), f.Int("page")), nil
        },
    )
    spec.Doc.Short = "Search messages by text"
    spec.Exec = r.Bounded(0)
    spec.Flags = []r.Flag{
        r.StringFlag{Name: "query", Required: true, Help: "search text"},
        r.IntFlag{Name: "limit", Default: 20, Help: "maximum results"},
        r.IntFlag{Name: "page", Help: "page number"},
    }
    reg.Register(spec)

    // contacts search
    spec = r.MustNewLeafSpec("contacts.search", r.MustNewPath("contacts", "search"),
        func(ctx context.Context, app *commands.App, f r.FlagValues) (string, error) {
            return app.SearchContacts(f.String("query")), nil
        },
    )
    spec.Doc.Short = "Search contacts by name"
    spec.Exec = r.Bounded(0)
    spec.Flags = []r.Flag{
        r.StringFlag{Name: "query", Required: true, Help: "search text"},
    }
    reg.Register(spec)

    // chats list
    spec = r.MustNewLeafSpec("chats.list", r.MustNewPath("chats", "list"),
        func(ctx context.Context, app *commands.App, f r.FlagValues) (string, error) {
            return app.ListChats(optStr(f, "query"), f.Int("limit"), f.Int("page")), nil
        },
    )
    spec.Doc.Short = "List recent chats"
    spec.Exec = r.Bounded(0)
    spec.Flags = []r.Flag{
        r.StringFlag{Name: "query", Help: "filter chats by name"},
        r.IntFlag{Name: "limit", Default: 20, Help: "maximum chats to return"},
        r.IntFlag{Name: "page", Help: "page number"},
    }
    reg.Register(spec)

    // media download
    spec = r.MustNewLeafSpec("media.download", r.MustNewPath("media", "download"),
        func(ctx context.Context, app *commands.App, f r.FlagValues) (string, error) {
            return app.DownloadMedia(ctx, f.String("message-id"), optStr(f, "chat"), f.String("output")), nil
        },
    )
    spec.Doc.Short = "Download media for a message"
    spec.Exec = r.Bounded(0)
    spec.Flags = []r.Flag{
        r.StringFlag{Name: "message-id", Required: true, Help: "message identifier"},
        r.StringFlag{Name: "chat", Help: "chat JID (optional)"},
        r.StringFlag{Name: "output", Help: "output file or directory"},
    }
    reg.Register(spec)
}

// optStr returns *string (nil if flag not set) for optional string flags.
func optStr(f r.FlagValues, name string) *string {
    if !f.IsSet(name) {
        return nil
    }
    v := f.String(name)
    return &v
}
```

**Step 2: Slim down root.go**

```go
package cmd

import "github.com/vicentereig/whatsapp-cli/cmd/registry"

var reg *registry.Registry

func SetVersion(v string) {
    reg = registry.NewRegistry()
    reg.SetVersion(v)
    registerAll(reg)
}

func Execute() {
    reg.Execute()
}
```

**Step 3: Run tests**

Run: `go test ./... -race -count=1`
All existing CLI tests must still pass — same commands, same flags, same behavior.

**Step 4: Commit**

```bash
git add cmd/commands.go cmd/root.go
git rm cmd/auth.go cmd/sync.go cmd/version.go cmd/send.go cmd/messages.go cmd/contacts.go cmd/chats.go cmd/media.go
git commit -m "refactor: migrate all commands to declarative registry"
```

---

## Part 2: New Commands (High Priority)

### Task 5: Message metadata lookup support (prerequisite for reply/react/mark-read)

**Why:** Tasks 6-10 (reply, react, delete, edit, mark-read) need message metadata that the current store doesn't expose. `BuildReaction` needs chat JID + sender JID + message ID. `MarkRead` needs message IDs + timestamp + chat + sender. Reply needs `ContextInfo.StanzaID` + `ContextInfo.Participant`. The current store only has `GetMessageForDownload` which returns media-specific fields.

**Files:**
- Modify: `internal/store/store.go` — add `GetMessageMetadata(id string, chatJID *string) (MessageMetadata, error)`
- Modify: `internal/commands/interfaces.go` — add `GetMessageMetadata` to `MessageStore` interface
- Create: `internal/store/types.go` (or extend existing) — `MessageMetadata` struct

**Step 1: Define MessageMetadata**

```go
type MessageMetadata struct {
    ID        string
    ChatJID   string
    Sender    string
    Timestamp time.Time
    IsFromMe  bool
    Content   string
}
```

**Step 2: Add store query**

Similar to `GetMessageForDownload` but returns the fields needed for message operations (not media fields). Query by message ID with optional chat JID filter.

**Step 3: Add to MessageStore interface**

```go
GetMessageMetadata(id string, chatJID *string) (store.MessageMetadata, error)
```

**Step 4: Tests, commit**

Run: `go test ./internal/store/ -race -count=1`

---

### Task 6: Send video/audio/document (no dependency on Task 5)

**Files:**
- Modify: `internal/commands/interfaces.go` — add WAClient methods
- Modify: `internal/client/client.go` — implement upload+send for each media type
- Modify: `internal/commands/commands.go` — add App methods
- Modify: `cmd/commands.go` — extend send spec with new flags

**Step 1: Extend WAClient interface**

Add to `WAClient`:
```go
SendVideoMessage(ctx context.Context, recipient, videoPath, caption string) (string, error)
SendAudioMessage(ctx context.Context, recipient, audioPath string) (string, error)
SendDocumentMessage(ctx context.Context, recipient, docPath, filename string) (string, error)
```

**Step 2: Implement in client.go**

Follow the existing `SendImageMessage` pattern:
1. Read file
2. `Upload(ctx, data, whatsmeow.MediaVideo)` (or MediaAudio, MediaDocument)
3. Build proto message with upload response
4. `SendMessage(ctx, jid, msg)`

**Step 3: Add App methods**

```go
func (a *App) SendVideo(ctx context.Context, to, path, caption string) string
func (a *App) SendAudio(ctx context.Context, to, path string) string
func (a *App) SendDocument(ctx context.Context, to, path, filename string) string
```

**Step 4: Update send command spec**

Add flags to existing send spec:
```go
{Name: "video", Type: String, Help: "video file path",
    MutuallyExclusiveWith: []string{"message", "image", "audio", "document"}},
{Name: "audio", Type: String, Help: "audio file path",
    MutuallyExclusiveWith: []string{"message", "image", "video", "document"}},
{Name: "document", Type: String, Help: "document file path",
    MutuallyExclusiveWith: []string{"message", "image", "video", "audio"}},
{Name: "filename", Type: String, Help: "document filename override", DependsOn: []string{"document"}},
```

Update Validate:
```go
Validate: func(f FlagValues) error {
    contentFlags := []string{"message", "image", "video", "audio", "document"}
    set := 0
    for _, name := range contentFlags {
        if f.IsSet(name) { set++ }
    }
    if set != 1 {
        return fmt.Errorf("exactly one of --%s required", strings.Join(contentFlags, ", --"))
    }
    return nil
},
```

**Step 5: Tests, commit**

---

### Task 7: Reply to message (depends on Task 5: message metadata)

**Files:**
- Modify: `internal/client/client.go` — add ContextInfo to sent messages
- Modify: `cmd/commands.go` — add `--reply-to` flag to send

The `--reply-to` flag works with any content type. It sets `ContextInfo.StanzaID` and `ContextInfo.Participant` on the proto message.

The App method uses Task 5's `GetMessageMetadata` to look up the sender JID for `ContextInfo.Participant` from the message ID. The user provides `--reply-to MESSAGE_ID`; the app resolves the rest.

**Response must include:** `message_id`, `reply_to` (confirming the quote).

---

### Task 8: React to message (depends on Task 5: message metadata)

**Files:**
- Modify: `internal/commands/interfaces.go` — add `ReactToMessage`
- Modify: `internal/client/client.go` — use `BuildReaction`
- Modify: `internal/commands/commands.go` — add `ReactToMessage` method
- Modify: `cmd/commands.go` — register `messages react`

```go
spec := r.MustNewLeafSpec("messages.react", r.MustNewPath("messages", "react"),
    func(ctx context.Context, app *commands.App, f r.FlagValues) (string, error) {
        return app.ReactToMessage(ctx, f.String("chat"), f.String("message-id"), f.String("emoji")), nil
    },
)
spec.Doc.Short = "React to a message with an emoji"
spec.Exec = r.Bounded(0)
spec.Flags = []r.Flag{
    r.StringFlag{Name: "message-id", Required: true, Help: "message to react to"},
    r.StringFlag{Name: "chat", Required: true, Help: "chat JID"},
    r.StringFlag{Name: "emoji", Required: true, Help: "reaction emoji (use empty string to remove)"},
}
reg.Register(spec)
```

Uses `BuildReaction(chat, sender, id, reaction)` → `SendMessage`.

---

### Task 9: Delete/revoke message

```go
spec := r.MustNewLeafSpec("messages.delete", r.MustNewPath("messages", "delete"),
    func(ctx context.Context, app *commands.App, f r.FlagValues) (string, error) {
        return app.DeleteMessage(ctx, f.String("chat"), f.String("message-id")), nil
    },
)
spec.Doc.Short = "Delete a sent message (revoke for everyone)"
spec.Exec = r.Bounded(0)
spec.Flags = []r.Flag{
    r.StringFlag{Name: "message-id", Required: true, Help: "message to delete"},
    r.StringFlag{Name: "chat", Required: true, Help: "chat JID"},
}
reg.Register(spec)
```

Uses `RevokeMessage(ctx, chat, id)`.

---

### Task 10: Edit message

```go
spec := r.MustNewLeafSpec("messages.edit", r.MustNewPath("messages", "edit"),
    func(ctx context.Context, app *commands.App, f r.FlagValues) (string, error) {
        return app.EditMessage(ctx, f.String("chat"), f.String("message-id"), f.String("text")), nil
    },
)
spec.Doc.Short = "Edit a sent message"
spec.Exec = r.Bounded(0)
spec.Flags = []r.Flag{
    r.StringFlag{Name: "message-id", Required: true, Help: "message to edit"},
    r.StringFlag{Name: "chat", Required: true, Help: "chat JID"},
    r.StringFlag{Name: "text", Required: true, Help: "new message text"},
}
reg.Register(spec)
```

Uses `BuildEdit(chat, id, newContent)` → `SendMessage`.

---

### Task 11: Mark message as read (depends on Task 5: message metadata)

```go
spec := r.MustNewLeafSpec("messages.mark-read", r.MustNewPath("messages", "mark-read"),
    func(ctx context.Context, app *commands.App, f r.FlagValues) (string, error) {
        return app.MarkMessageRead(ctx, f.String("chat"), f.String("message-id")), nil
    },
)
spec.Doc.Short = "Mark a message as read"
spec.Exec = r.Bounded(0)
spec.Flags = []r.Flag{
    r.StringFlag{Name: "message-id", Required: true, Help: "message ID to mark read"},
    r.StringFlag{Name: "chat", Required: true, Help: "chat JID"},
}
reg.Register(spec)
```

Uses `MarkRead(ctx, ids, timestamp, chat, sender)`.

**Why single message ID, not batch:** `MarkRead` requires all IDs to share the same sender and a single timestamp. Accepting multiple IDs at the CLI level would require grouping by sender, choosing a timestamp per group, and making multiple `MarkRead` calls — hidden compound behavior that violates AX principle #5 (atomic operations). One command = one message marked = one `MarkRead` call.

The App method looks up message metadata (via Task 5's `GetMessageMetadata`) to populate the `timestamp` and `sender` fields. The CLI only needs `--message-id` and `--chat` from the user. If the agent needs to mark multiple messages read, it calls the command multiple times — composability over hidden batching.

---

## Part 3: New Commands (Medium Priority)

### Task 12: Groups commands

Register parent:
```go
registry.RegisterParent(root, ParentSpec{
    Path: []string{"groups"}, Short: "Manage WhatsApp groups",
})
```

Commands:

| Spec ID | Path | Key Flags | whatsmeow Method |
|---------|------|-----------|-----------------|
| `groups.list` | `["groups","list"]` | none | `GetJoinedGroups` |
| `groups.info` | `["groups","info"]` | `--jid` (req) | `GetGroupInfo` |
| `groups.create` | `["groups","create"]` | `--name` (req), `--members` (StringSlice) | `CreateGroup` |
| `groups.invite-link` | `["groups","invite-link"]` | `--jid` (req), `--reset` (Bool) | `GetGroupInviteLink` |
| `groups.join` | `["groups","join"]` | `--link` (req) | `JoinGroupWithLink` |
| `groups.leave` | `["groups","leave"]` | `--jid` (req) | `LeaveGroup` |
| `groups.add-members` | `["groups","add-members"]` | `--jid` (req), `--members` (StringSlice, req) | `UpdateGroupParticipants(..., "add")` |
| `groups.remove-members` | `["groups","remove-members"]` | `--jid` (req), `--members` (StringSlice, req) | `UpdateGroupParticipants(..., "remove")` |
| `groups.set-name` | `["groups","set-name"]` | `--jid` (req), `--name` (req) | `SetGroupName` |
| `groups.set-description` | `["groups","set-description"]` | `--jid` (req), `--description` (req) | `SetGroupDescription` |
| `groups.set-photo` | `["groups","set-photo"]` | `--jid` (req), `--image` (req) | `SetGroupPhoto` |

---

### Task 13: Contacts block/unblock

| Spec ID | Path | Key Flags | whatsmeow Method |
|---------|------|-----------|-----------------|
| `contacts.block` | `["contacts","block"]` | `--jid` (req) | `UpdateBlocklist(..., "block")` |
| `contacts.unblock` | `["contacts","unblock"]` | `--jid` (req) | `UpdateBlocklist(..., "unblock")` |
| `contacts.list-blocked` | `["contacts","list-blocked"]` | none | `GetBlocklist` |
| `contacts.check` | `["contacts","check"]` | `--phone` (StringSlice, req) | `IsOnWhatsApp` |

---

### Task 14: Chat management

| Spec ID | Path | Key Flags | whatsmeow Method |
|---------|------|-----------|-----------------|
| `chats.archive` | `["chats","archive"]` | `--jid` (req) | `FetchAppState` + patch |
| `chats.unarchive` | `["chats","unarchive"]` | `--jid` (req) | `FetchAppState` + patch |
| `chats.mute` | `["chats","mute"]` | `--jid` (req), `--duration` | `FetchAppState` + patch |
| `chats.unmute` | `["chats","unmute"]` | `--jid` (req) | `FetchAppState` + patch |

Note: Chat archive/mute use `SendAppState` with app state patches, not direct methods. These are more complex — implement after groups.

---

## Part 4: New Commands (Low Priority)

### Task 15: Presence & status

| Spec ID | Path | Key Flags | whatsmeow Method |
|---------|------|-----------|-----------------|
| `presence.send` | `["presence","send"]` | `--state` (StringFlag, Enum: ["available","unavailable"]) | `SendPresence` |
| `presence.subscribe` | `["presence","subscribe"]` | `--jid` (StringFlag, req) | `SubscribePresence` |
| `presence.typing` | `["presence","typing"]` | `--chat` (StringFlag, req), `--state` (StringFlag, Enum: ["composing","paused"]) | `SendChatPresence` |
| `status.set` | `["status","set"]` | `--text` (StringFlag, req) | `SetStatusMessage` |

---

### Task 16: Polls

| Spec ID | Path | Key Flags | whatsmeow Method |
|---------|------|-----------|-----------------|
| `send` (with `--poll`) | `["send"]` | `--poll` (name), `--options` (StringSlice), `--max-choices` (Int) | `BuildPollCreation` → `SendMessage` |

Polls are a content type on `send`, not a separate resource. Add `--poll` to the mutual exclusion group.

---

### Task 17: Privacy settings

| Spec ID | Path | Key Flags | whatsmeow Method |
|---------|------|-----------|-----------------|
| `privacy.get` | `["privacy","get"]` | none | `GetPrivacySettings` |
| `privacy.set` | `["privacy","set"]` | `--setting` (Enum, req), `--value` (Enum, req) | `SetPrivacySetting` |

---

## Part 5: whatsmeow Coverage Audit Tool

### Task 18: Build audit tool

**Files:**
- Create: `tools/whatsmeow-audit/main.go`

A Go program that:
1. Uses `go doc go.mau.fi/whatsmeow Client` (or `go/packages`) to list all exported methods on `whatsmeow.Client`
2. Reads a `tools/whatsmeow-audit/coverage.yaml` file that maps each method to one of: `mapped` (with command ID), `ignored` (with reason), `composite` (used internally by multi-step commands), or `unreviewed`
3. Prints a coverage report

**Scope:** This measures `whatsmeow.Client` method coverage only. Some CLI commands are composite (send image = Upload + SendMessage, poll vote = BuildPollVote + EncryptPollVote + SendMessage) and some use app state patches rather than direct methods (chat archive/mute). The `composite` status tracks methods used internally by multi-step operations. The `ignored` status covers methods not suitable for CLI exposure.

```
whatsmeow.Client method coverage:
  Mapped:     23/50 (46%)  — direct CLI command
  Composite:   5/50 (10%)  — used internally by mapped commands
  Ignored:    12/50 (24%)  — not suitable for CLI
  Unreviewed: 10/50 (20%)  — needs review

Unreviewed methods:
  - AcceptTOSNotice
  - BuildHistorySyncRequest
  - ...
```

**coverage.yaml format:**
```yaml
methods:
  SendMessage: {status: mapped, command: send}
  BuildReaction: {status: mapped, command: messages.react}
  RevokeMessage: {status: mapped, command: messages.delete}
  Upload: {status: composite, used_by: [send], reason: "media upload step"}
  EncryptPollVote: {status: composite, used_by: [send], reason: "poll vote encryption"}
  Connect: {status: mapped, command: auth}
  Disconnect: {status: composite, used_by: [internal], reason: "connection lifecycle"}
  DangerousInternals: {status: ignored, reason: "unsafe, not for CLI"}
  ParseWebMessage: {status: ignored, reason: "internal to sync"}
  SendAppState: {status: ignored, reason: "used indirectly via chat archive/mute patches"}
  AcceptTOSNotice: {status: unreviewed}
```

Run after `go get go.mau.fi/whatsmeow@latest` to detect drift.

**Step 1: Write the tool**
**Step 2: Create initial coverage.yaml with current mappings**
**Step 3: Commit**

```bash
git add tools/whatsmeow-audit/
git commit -m "feat: add whatsmeow API coverage audit tool"
```

---

## Part 6: Update Skill & Docs

### Task 19: Update whatsapp skill

**Files:**
- Modify: whatsapp skill SKILL.md (user-local, not repo-local — update separately after implementation)

This task is **not part of the repo**. After all commands are implemented, update the skill documentation to reflect:
- All new commands in Quick Reference table
- AX contract section (exit codes, stream discipline)
- New media types (video, audio, document)
- Group management examples
- Reaction/reply/edit examples

### Task 20: Update README

**Files:**
- Modify: `README.md`

Add all new commands with examples.

---

## Implementation Order

```
Task 1-3: Registry framework (foundation — everything depends on this)
Task 4: Migrate existing commands (proves the registry works)
Task 5: Message metadata lookup (prerequisite for reply/react/mark-read)
Task 6: Send video/audio/document (independent of Task 5)
Task 7: Reply to message (depends on Task 5)
Task 8: React to message (depends on Task 5)
Task 9-10: Delete + edit message
Task 11: Mark messages as read (depends on Task 5)
Task 12: Groups (medium priority, many commands)
Task 13-14: Contacts block + chat management
Task 15-17: Presence, polls, privacy (low priority)
Task 18: Audit tool (run after all commands registered)
Task 19-20: Docs update (after all commands exist)
```

Tasks 5 and 6 can run in parallel (no dependency between them).

## Risks and Mitigations

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| Registry abstraction doesn't fit all commands | Low | High | sync and send are the hardest cases; test with those first |
| whatsmeow methods require complex composition | Medium | Medium | Run function can be as complex as needed; registry just removes boilerplate |
| Chat archive/mute use app state patches, not direct methods | High | Low | Defer to Part 4; implement after understanding app state API |
| Go 1.25 requirement from whatsmeow bump | Low | Medium | CI may need Go version update |

## Rollback Plan

- Registry is additive — old per-file commands work until Task 4 deletes them
- Each task is independently committable
- `git revert` any task's commits to undo

## Open Questions

- [ ] Should `send --sticker` and `send --location` and `send --contact` be in the first implementation pass, or deferred?
- [ ] Should newsletter commands be exposed? (WhatsApp Channels — `GetSubscribedNewsletters`, `FollowNewsletter`, etc.)
- [ ] Should `calls reject` be exposed? (Only reject, can't initiate calls via whatsmeow)
