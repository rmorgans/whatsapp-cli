// Package registry provides type-safe command and flag specifications for
// building a CLI command registry. All command definitions flow through
// these types before being wired to cobra.
package registry

import (
	"context"
	"strconv"
	"time"

	"github.com/vicentereig/whatsapp-cli/internal/commands"
)

// ---------------------------------------------------------------------------
// CommandPath
// ---------------------------------------------------------------------------

// CommandPath represents a non-empty path to a command in the CLI tree.
// For example, ["message", "list"] maps to `cli message list`.
// The zero value is not usable; use MustNewPath to construct.
type CommandPath struct {
	parents []string
	name    string
}

// MustNewPath creates a CommandPath from one or more segments.
// It panics if no segments are provided.
func MustNewPath(segments ...string) CommandPath {
	if len(segments) == 0 {
		panic("registry.MustNewPath: at least one segment is required")
	}
	for i, s := range segments {
		if s == "" {
			panic("registry.MustNewPath: segment at index " + strconv.Itoa(i) + " is empty")
		}
	}
	parents := make([]string, len(segments)-1)
	copy(parents, segments[:len(segments)-1])
	return CommandPath{
		parents: parents,
		name:    segments[len(segments)-1],
	}
}

// Parents returns the parent segments (everything except the leaf name).
func (p CommandPath) Parents() []string {
	out := make([]string, len(p.parents))
	copy(out, p.parents)
	return out
}

// Name returns the leaf command name.
func (p CommandPath) Name() string { return p.name }

// Segments returns all path segments (parents + name).
func (p CommandPath) Segments() []string {
	out := make([]string, len(p.parents)+1)
	copy(out, p.parents)
	out[len(p.parents)] = p.name
	return out
}

// ---------------------------------------------------------------------------
// ExecMode
// ---------------------------------------------------------------------------

// execKind is the internal discriminant for ExecMode.
type execKind int

const (
	execBounded   execKind = iota // default zero value
	execStreaming
	execLocal
)

// ExecMode describes how a command executes. Exactly one of Bounded,
// Streaming, or Local. The zero value is Bounded with zero timeout
// (which the wiring layer should treat as a sensible default).
type ExecMode struct {
	kind    execKind
	timeout time.Duration
}

// Bounded returns an ExecMode for commands that complete within a timeout.
// It panics if timeout is negative.
func Bounded(timeout time.Duration) ExecMode {
	if timeout < 0 {
		panic("registry.Bounded: timeout must not be negative")
	}
	return ExecMode{kind: execBounded, timeout: timeout}
}

// Streaming returns an ExecMode for long-running commands (e.g. sync)
// that stream output until cancelled.
func Streaming() ExecMode {
	return ExecMode{kind: execStreaming}
}

// Local returns an ExecMode for commands that run entirely locally
// (no network, no timeout needed).
func Local() ExecMode {
	return ExecMode{kind: execLocal}
}

// IsStreaming reports whether this mode is Streaming.
func (m ExecMode) IsStreaming() bool { return m.kind == execStreaming }

// IsLocal reports whether this mode is Local.
func (m ExecMode) IsLocal() bool { return m.kind == execLocal }

// IsBounded reports whether this mode is Bounded (the default).
func (m ExecMode) IsBounded() bool { return m.kind == execBounded }

// Timeout returns the timeout for Bounded mode, or zero for other modes.
func (m ExecMode) Timeout() time.Duration { return m.timeout }

// ---------------------------------------------------------------------------
// Flag types
// ---------------------------------------------------------------------------

// Flag is implemented by every concrete flag type (StringFlag, IntFlag,
// BoolFlag, StringSliceFlag). The methods are unexported to prevent
// external implementations — the set of flag types is closed.
type Flag interface {
	flagName() string
	isRequired() bool
	isHidden() bool
}

// StringFlag defines a string flag. It is the only flag type that
// supports an Enum constraint.
type StringFlag struct {
	Name     string
	Short    string
	Help     string
	Default  string
	Required bool
	Hidden   bool
	Enum     []string
}

func (f StringFlag) flagName() string { return f.Name }
func (f StringFlag) isRequired() bool { return f.Required }
func (f StringFlag) isHidden() bool   { return f.Hidden }

// IntFlag defines an integer flag.
type IntFlag struct {
	Name     string
	Short    string
	Help     string
	Default  int
	Required bool
	Hidden   bool
}

func (f IntFlag) flagName() string { return f.Name }
func (f IntFlag) isRequired() bool { return f.Required }
func (f IntFlag) isHidden() bool   { return f.Hidden }

// BoolFlag defines a boolean flag. It has no Required field (a bool
// always has a default) and no Enum field.
type BoolFlag struct {
	Name    string
	Short   string
	Help    string
	Default bool
	Hidden  bool
}

func (f BoolFlag) flagName() string { return f.Name }
func (f BoolFlag) isRequired() bool { return false }
func (f BoolFlag) isHidden() bool   { return f.Hidden }

// StringSliceFlag defines a flag that accepts multiple string values.
type StringSliceFlag struct {
	Name     string
	Short    string
	Help     string
	Default  []string
	Required bool
	Hidden   bool
}

func (f StringSliceFlag) flagName() string { return f.Name }
func (f StringSliceFlag) isRequired() bool { return f.Required }
func (f StringSliceFlag) isHidden() bool   { return f.Hidden }

// ---------------------------------------------------------------------------
// RuleSpec
// ---------------------------------------------------------------------------

// RuleSpec defines command-level flag constraints. Mutual exclusion and
// co-dependency live here, not on individual flags.
type RuleSpec struct {
	// ExactlyOneOf groups where exactly one flag must be set.
	ExactlyOneOf [][]string
	// AtMostOneOf groups where at most one flag may be set.
	AtMostOneOf [][]string
	// Requires maps a flag name to flags it depends on.
	Requires map[string][]string
}

// ---------------------------------------------------------------------------
// FlagValues
// ---------------------------------------------------------------------------

// FlagValues provides type-safe access to resolved flag values at runtime.
type FlagValues interface {
	String(name string) string
	Int(name string) int
	Bool(name string) bool
	StringSlice(name string) []string
	IsSet(name string) bool
}

// ---------------------------------------------------------------------------
// Runner
// ---------------------------------------------------------------------------

// Runner is the function signature for command execution. It receives a
// context, the application instance, and the resolved flag values.
type Runner func(ctx context.Context, app *commands.App, f FlagValues) (string, error)

// ---------------------------------------------------------------------------
// DocSpec
// ---------------------------------------------------------------------------

// DocSpec holds documentation metadata for a leaf command.
type DocSpec struct {
	Short      string
	Long       string
	Examples   []string
	Aliases    []string
	Hidden     bool
	Deprecated string
}

// ---------------------------------------------------------------------------
// ParentSpec
// ---------------------------------------------------------------------------

// ParentSpec defines a non-leaf command (a grouping node). It has no Run,
// no Flags, and no ExecMode — it exists only to organise sub-commands.
type ParentSpec struct {
	Path   CommandPath
	Short  string
	Long   string
	Hidden bool
}

// ---------------------------------------------------------------------------
// LeafSpec
// ---------------------------------------------------------------------------

// LeafSpec defines a runnable leaf command. Run is always non-nil.
type LeafSpec struct {
	ID       string
	Path     CommandPath
	Doc      DocSpec
	Exec     ExecMode
	Flags    []Flag
	Rules    RuleSpec
	Run      Runner
	Validate func(f FlagValues) error
}

// MustNewLeafSpec creates a LeafSpec with the required fields. It panics
// if run is nil (a leaf command without a runner is a bug).
func MustNewLeafSpec(id string, path CommandPath, run Runner) LeafSpec {
	if run == nil {
		panic("registry.MustNewLeafSpec: run must not be nil")
	}
	if id == "" {
		panic("registry.MustNewLeafSpec: id must not be empty")
	}
	if path.Name() == "" {
		panic("registry.MustNewLeafSpec: path must not be zero-value (empty name)")
	}
	return LeafSpec{
		ID:   id,
		Path: path,
		Run:  run,
	}
}
