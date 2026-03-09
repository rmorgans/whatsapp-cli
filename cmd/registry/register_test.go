package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vicentereig/whatsapp-cli/internal/commands"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// nopRunner returns a successful JSON result without needing the app.
func nopRunner(_ context.Context, _ *commands.App, _ FlagValues) (string, error) {
	return `{"success":true,"data":null,"error":null}`, nil
}

// dummyLeaf creates a minimal LeafSpec for registration tests.
func dummyLeaf(id string, segments ...string) LeafSpec {
	return MustNewLeafSpec(id, MustNewPath(segments...), nopRunner)
}

// mustPanic asserts that fn panics with a message containing substr.
func mustPanic(t *testing.T, substr string, fn func()) {
	t.Helper()
	defer func() {
		r := recover()
		require.NotNil(t, r, "expected panic but none occurred")
		msg := fmt.Sprint(r)
		assert.Contains(t, msg, substr, "panic message mismatch")
	}()
	fn()
}

// ---------------------------------------------------------------------------
// NewRegistry
// ---------------------------------------------------------------------------

func TestNewRegistry(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	require.NotNil(t, r.Root())
	assert.Equal(t, "whatsapp-cli", r.Root().Use)
	assert.True(t, r.Root().SilenceUsage)
	assert.True(t, r.Root().SilenceErrors)

	// Persistent --store flag exists.
	sf := r.Root().PersistentFlags().Lookup("store")
	require.NotNil(t, sf)
	assert.Equal(t, "./store", sf.DefValue)
}

// ---------------------------------------------------------------------------
// Version
// ---------------------------------------------------------------------------

func TestSetGetVersion(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	r.SetVersion("1.2.3")
	assert.Equal(t, "1.2.3", r.Version())
}

// ---------------------------------------------------------------------------
// Registration panics (table-driven)
// ---------------------------------------------------------------------------

func TestRegister_PanicOnDuplicateID(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	r.Register(dummyLeaf("cmd1", "alpha"))
	mustPanic(t, "duplicate command ID", func() {
		r.Register(dummyLeaf("cmd1", "beta"))
	})
}

func TestRegister_PanicOnDuplicatePath(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	r.Register(dummyLeaf("cmd1", "alpha"))
	mustPanic(t, "duplicate command path", func() {
		r.Register(dummyLeaf("cmd2", "alpha"))
	})
}

func TestRegister_PanicOnDuplicateFlag(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	spec := dummyLeaf("cmd1", "alpha")
	spec.Flags = []Flag{
		StringFlag{Name: "foo", Help: "first"},
		StringFlag{Name: "foo", Help: "second"},
	}
	mustPanic(t, "duplicate flag name", func() {
		r.Register(spec)
	})
}

func TestRegister_PanicOnRequiredStringFlagWithDefault(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	spec := dummyLeaf("cmd1", "alpha")
	spec.Flags = []Flag{
		StringFlag{Name: "name", Default: "bob", Required: true},
	}
	mustPanic(t, "must not have a non-zero default", func() {
		r.Register(spec)
	})
}

func TestRegister_PanicOnRuleGroupTooSmall(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	spec := dummyLeaf("cmd1", "alpha")
	spec.Flags = []Flag{StringFlag{Name: "a"}}
	spec.Rules = RuleSpec{ExactlyOneOf: [][]string{{"a"}}}
	mustPanic(t, "at least 2 entries", func() {
		r.Register(spec)
	})
}

func TestRegister_PanicOnRuleGroupUnknownFlag(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	spec := dummyLeaf("cmd1", "alpha")
	spec.Flags = []Flag{StringFlag{Name: "a"}}
	spec.Rules = RuleSpec{ExactlyOneOf: [][]string{{"a", "ghost"}}}
	mustPanic(t, "unknown flag", func() {
		r.Register(spec)
	})
}

func TestRegister_PanicOnRuleGroupDuplicate(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	spec := dummyLeaf("cmd1", "alpha")
	spec.Flags = []Flag{StringFlag{Name: "a"}, StringFlag{Name: "b"}}
	spec.Rules = RuleSpec{AtMostOneOf: [][]string{{"a", "a"}}}
	mustPanic(t, "duplicate flag", func() {
		r.Register(spec)
	})
}

func TestRegister_PanicOnFlagInBothExactlyAndAtMost(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	spec := dummyLeaf("cmd1", "alpha")
	spec.Flags = []Flag{
		StringFlag{Name: "a"}, StringFlag{Name: "b"},
		StringFlag{Name: "c"}, StringFlag{Name: "d"},
	}
	spec.Rules = RuleSpec{
		ExactlyOneOf: [][]string{{"a", "b"}},
		AtMostOneOf:  [][]string{{"a", "c"}},
	}
	mustPanic(t, "both ExactlyOneOf and AtMostOneOf", func() {
		r.Register(spec)
	})
}

func TestRegister_PanicOnRequiresUnknownFlag(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		requires map[string][]string
		flags   []Flag
	}{
		{
			name:     "unknown source",
			requires: map[string][]string{"ghost": {"a"}},
			flags:    []Flag{StringFlag{Name: "a"}},
		},
		{
			name:     "unknown target",
			requires: map[string][]string{"a": {"ghost"}},
			flags:    []Flag{StringFlag{Name: "a"}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := NewRegistry()
			spec := dummyLeaf("cmd1", "alpha")
			spec.Flags = tc.flags
			spec.Rules = RuleSpec{Requires: tc.requires}
			mustPanic(t, "unknown flag", func() {
				r.Register(spec)
			})
		})
	}
}

// ---------------------------------------------------------------------------
// RegisterParent
// ---------------------------------------------------------------------------

func TestRegisterParent(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	r.RegisterParent(ParentSpec{
		Path:  MustNewPath("message"),
		Short: "Message commands",
	})

	// Parent should exist and have the right short description.
	msgCmd := r.parents["message"]
	require.NotNil(t, msgCmd)
	assert.Equal(t, "Message commands", msgCmd.Short)

	// Running parent without subcommand should error.
	err := msgCmd.RunE(msgCmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "requires a subcommand")
}

// ---------------------------------------------------------------------------
// Register creates leaf with flags
// ---------------------------------------------------------------------------

func TestRegister_CreatesLeafWithFlags(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	spec := dummyLeaf("msg-list", "message", "list")
	spec.Doc = DocSpec{
		Short:    "List messages",
		Long:     "List all messages in a chat",
		Examples: []string{"whatsapp-cli message list --jid 123"},
		Aliases:  []string{"ls"},
	}
	spec.Flags = []Flag{
		StringFlag{Name: "jid", Short: "j", Help: "Chat JID", Required: true},
		IntFlag{Name: "limit", Short: "l", Help: "Max results", Default: 50},
		BoolFlag{Name: "verbose", Short: "v", Help: "Verbose output"},
		StringSliceFlag{Name: "tags", Short: "t", Help: "Filter tags"},
	}
	spec.Exec = Local()
	r.Register(spec)

	// Find the leaf command through cobra's tree.
	leaf, _, err := r.Root().Find([]string{"message", "list"})
	require.NoError(t, err)
	assert.Equal(t, "list", leaf.Use)
	assert.Equal(t, "List messages", leaf.Short)
	assert.Equal(t, []string{"ls"}, leaf.Aliases)

	// Verify flags exist.
	require.NotNil(t, leaf.Flags().Lookup("jid"))
	require.NotNil(t, leaf.Flags().Lookup("limit"))
	require.NotNil(t, leaf.Flags().Lookup("verbose"))
	require.NotNil(t, leaf.Flags().Lookup("tags"))
}

// ---------------------------------------------------------------------------
// ensureParent creates intermediate parents
// ---------------------------------------------------------------------------

func TestEnsureParent_CreatesIntermediates(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	// Register a leaf deep in the tree.
	spec := dummyLeaf("deep-cmd", "a", "b", "c", "leaf")
	spec.Exec = Local()
	r.Register(spec)

	// Intermediate parents should have been created.
	assert.Contains(t, r.parents, "a")
	assert.Contains(t, r.parents, "a b")
	assert.Contains(t, r.parents, "a b c")
}

// ---------------------------------------------------------------------------
// errorJSON
// ---------------------------------------------------------------------------

func TestErrorJSON(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		msg  string
	}{
		{"simple", "something broke"},
		{"with quotes", `she said "hello"`},
		{"with newlines", "line1\nline2"},
		{"with backslash", `path\to\file`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := errorJSON(tc.msg)

			// Must be valid JSON.
			var envelope struct {
				Success bool        `json:"success"`
				Data    interface{} `json:"data"`
				Error   string      `json:"error"`
			}
			err := json.Unmarshal([]byte(result), &envelope)
			require.NoError(t, err, "errorJSON must produce valid JSON")
			assert.False(t, envelope.Success)
			assert.Nil(t, envelope.Data)
			assert.Equal(t, tc.msg, envelope.Error)
		})
	}
}

// ---------------------------------------------------------------------------
// printResult
// ---------------------------------------------------------------------------

func TestPrintResult_SetsExitCodeOnFailure(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		json     string
		wantCode int
	}{
		{"success", `{"success":true,"data":{},"error":null}`, 0},
		{"failure", `{"success":false,"data":null,"error":"boom"}`, 1},
		{"invalid json", "not json at all", 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := NewRegistry()
			// Capture stdout by temporarily redirecting.
			// printResult writes to stdout via fmt.Println; we just check exitCode.
			r.printResult(tc.json)
			assert.Equal(t, tc.wantCode, r.exitCode)
		})
	}
}

// ---------------------------------------------------------------------------
// buildRunE: runtime validation
// ---------------------------------------------------------------------------

// execCmd creates a command, sets flag values, and runs it, returning the error.
func execCmd(t *testing.T, spec LeafSpec, flagArgs ...string) error {
	t.Helper()
	r := NewRegistry()
	r.Register(spec)

	// Build args: path segments + flags.
	args := spec.Path.Segments()
	args = append(args, flagArgs...)
	r.Root().SetArgs(args)

	return r.Root().Execute()
}

func TestBuildRunE_ValidatesExactlyOneOf(t *testing.T) {
	t.Parallel()
	spec := dummyLeaf("cmd1", "test")
	spec.Exec = Local()
	spec.Flags = []Flag{
		StringFlag{Name: "a", Help: "flag a"},
		StringFlag{Name: "b", Help: "flag b"},
	}
	spec.Rules = RuleSpec{ExactlyOneOf: [][]string{{"a", "b"}}}

	// Neither set: should fail.
	err := execCmd(t, spec)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exactly one of")

	// Both set: should fail.
	err = execCmd(t, spec, "--a", "x", "--b", "y")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exactly one of")

	// Exactly one: should succeed.
	err = execCmd(t, spec, "--a", "x")
	require.NoError(t, err)
}

func TestBuildRunE_ValidatesAtMostOneOf(t *testing.T) {
	t.Parallel()
	spec := dummyLeaf("cmd1", "test")
	spec.Exec = Local()
	spec.Flags = []Flag{
		StringFlag{Name: "a", Help: "flag a"},
		StringFlag{Name: "b", Help: "flag b"},
	}
	spec.Rules = RuleSpec{AtMostOneOf: [][]string{{"a", "b"}}}

	// Neither set: OK.
	err := execCmd(t, spec)
	require.NoError(t, err)

	// One set: OK.
	err = execCmd(t, spec, "--a", "x")
	require.NoError(t, err)

	// Both set: should fail.
	err = execCmd(t, spec, "--a", "x", "--b", "y")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "at most one of")
}

func TestBuildRunE_ValidatesRequires(t *testing.T) {
	t.Parallel()
	spec := dummyLeaf("cmd1", "test")
	spec.Exec = Local()
	spec.Flags = []Flag{
		StringFlag{Name: "format", Help: "output format"},
		StringFlag{Name: "output", Help: "output file"},
	}
	spec.Rules = RuleSpec{Requires: map[string][]string{"output": {"format"}}}

	// output without format: should fail.
	err := execCmd(t, spec, "--output", "file.txt")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "requires")

	// Both set: OK.
	err = execCmd(t, spec, "--output", "file.txt", "--format", "json")
	require.NoError(t, err)

	// Neither set: OK.
	err = execCmd(t, spec)
	require.NoError(t, err)
}

func TestBuildRunE_ValidatesEnum(t *testing.T) {
	t.Parallel()
	spec := dummyLeaf("cmd1", "test")
	spec.Exec = Local()
	spec.Flags = []Flag{
		StringFlag{Name: "format", Help: "output format", Enum: []string{"json", "text"}},
	}

	// Valid enum value.
	err := execCmd(t, spec, "--format", "json")
	require.NoError(t, err)

	// Invalid enum value.
	err = execCmd(t, spec, "--format", "xml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be one of")

	// No value (empty): OK (not set).
	err = execCmd(t, spec)
	require.NoError(t, err)
}

func TestBuildRunE_CallsValidateCallback(t *testing.T) {
	t.Parallel()
	spec := dummyLeaf("cmd1", "test")
	spec.Exec = Local()
	spec.Flags = []Flag{
		IntFlag{Name: "count", Help: "a count"},
	}
	spec.Validate = func(f FlagValues) error {
		if f.Int("count") < 0 {
			return fmt.Errorf("count must be non-negative")
		}
		return nil
	}

	err := execCmd(t, spec, "--count", "-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "count must be non-negative")

	err = execCmd(t, spec, "--count", "5")
	require.NoError(t, err)
}

// ---------------------------------------------------------------------------
// Local mode runs without app init
// ---------------------------------------------------------------------------

func TestBuildRunE_LocalModeNoApp(t *testing.T) {
	t.Parallel()
	var calledWithNilApp bool
	spec := dummyLeaf("cmd1", "test")
	spec.Exec = Local()
	spec.Run = func(ctx context.Context, app *commands.App, f FlagValues) (string, error) {
		calledWithNilApp = app == nil
		return `{"success":true,"data":null,"error":null}`, nil
	}

	err := execCmd(t, spec)
	require.NoError(t, err)
	assert.True(t, calledWithNilApp, "local mode should call runner with nil app")
}

// ---------------------------------------------------------------------------
// validateRules (unit tests on the pure function)
// ---------------------------------------------------------------------------

func TestValidateRules_ExactlyOneOf(t *testing.T) {
	t.Parallel()
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("a", "", "")
	cmd.Flags().String("b", "", "")

	rules := RuleSpec{ExactlyOneOf: [][]string{{"a", "b"}}}

	// None set.
	err := validateRules(cmd, rules)
	require.Error(t, err)

	// Set a.
	require.NoError(t, cmd.Flags().Set("a", "val"))
	err = validateRules(cmd, rules)
	require.NoError(t, err)

	// Set b too.
	require.NoError(t, cmd.Flags().Set("b", "val"))
	err = validateRules(cmd, rules)
	require.Error(t, err)
}

func TestValidateRules_AtMostOneOf(t *testing.T) {
	t.Parallel()
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("x", "", "")
	cmd.Flags().String("y", "", "")

	rules := RuleSpec{AtMostOneOf: [][]string{{"x", "y"}}}

	// None set: OK.
	require.NoError(t, validateRules(cmd, rules))

	// Set x: OK.
	require.NoError(t, cmd.Flags().Set("x", "val"))
	require.NoError(t, validateRules(cmd, rules))

	// Set y too: fail.
	require.NoError(t, cmd.Flags().Set("y", "val"))
	require.Error(t, validateRules(cmd, rules))
}

func TestValidateRules_Requires(t *testing.T) {
	t.Parallel()
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("src", "", "")
	cmd.Flags().String("dep", "", "")

	rules := RuleSpec{Requires: map[string][]string{"src": {"dep"}}}

	// src without dep: fail.
	require.NoError(t, cmd.Flags().Set("src", "val"))
	require.Error(t, validateRules(cmd, rules))

	// Add dep: OK.
	require.NoError(t, cmd.Flags().Set("dep", "val"))
	require.NoError(t, validateRules(cmd, rules))
}

// ---------------------------------------------------------------------------
// validateEnums (unit tests on the pure function)
// ---------------------------------------------------------------------------

func TestValidateEnums(t *testing.T) {
	t.Parallel()
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("format", "", "")
	fv := NewFlagValues(cmd)

	flags := []Flag{
		StringFlag{Name: "format", Enum: []string{"json", "csv"}},
	}

	// Empty value: OK.
	require.NoError(t, validateEnums(fv, flags))

	// Valid value.
	require.NoError(t, cmd.Flags().Set("format", "json"))
	require.NoError(t, validateEnums(fv, flags))

	// Invalid value.
	require.NoError(t, cmd.Flags().Set("format", "yaml"))
	err := validateEnums(fv, flags)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be one of")
}

// ---------------------------------------------------------------------------
// newContext (pure function)
// ---------------------------------------------------------------------------

func TestNewContext_Bounded(t *testing.T) {
	t.Parallel()
	ctx, cancel := newContext(Bounded(2 * defaultTimeout))
	defer cancel()
	deadline, ok := ctx.Deadline()
	require.True(t, ok, "bounded context should have a deadline")
	_ = deadline
}

func TestNewContext_BoundedDefaultTimeout(t *testing.T) {
	t.Parallel()
	ctx, cancel := newContext(Bounded(0))
	defer cancel()
	_, ok := ctx.Deadline()
	require.True(t, ok, "bounded context with zero timeout should default to 5 min deadline")
}

func TestNewContext_Streaming(t *testing.T) {
	t.Parallel()
	ctx, cancel := newContext(Streaming())
	defer cancel()
	_, ok := ctx.Deadline()
	require.False(t, ok, "streaming context should have no deadline")
}

// ---------------------------------------------------------------------------
// Hidden parent
// ---------------------------------------------------------------------------

func TestRegisterParent_Hidden(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	r.RegisterParent(ParentSpec{
		Path:   MustNewPath("debug"),
		Short:  "Debug commands",
		Hidden: true,
	})
	cmd := r.parents["debug"]
	require.NotNil(t, cmd)
	assert.True(t, cmd.Hidden)
}

// ---------------------------------------------------------------------------
// Runner error goes through printResult, not cobra
// ---------------------------------------------------------------------------

func TestBuildRunE_RunnerError_SetExitCode(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	spec := dummyLeaf("cmd1", "test")
	spec.Exec = Local()
	spec.Run = func(ctx context.Context, app *commands.App, f FlagValues) (string, error) {
		return "", fmt.Errorf("something failed")
	}
	r.Register(spec)

	r.Root().SetArgs([]string{"test"})
	err := r.Root().Execute()
	// No error returned to cobra — runner errors go through printResult.
	require.NoError(t, err)
	assert.Equal(t, 1, r.exitCode, "runner error should set exitCode to 1")
}

// ---------------------------------------------------------------------------
// Integration: full command execution
// ---------------------------------------------------------------------------

func TestIntegration_LocalCommand(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	spec := dummyLeaf("greet", "greet")
	spec.Exec = Local()
	spec.Doc = DocSpec{Short: "Say hello"}
	spec.Flags = []Flag{
		StringFlag{Name: "name", Short: "n", Help: "Name to greet", Default: "world"},
	}
	spec.Run = func(ctx context.Context, app *commands.App, f FlagValues) (string, error) {
		name := f.String("name")
		data, _ := json.Marshal(map[string]string{"greeting": "hello " + name})
		return fmt.Sprintf(`{"success":true,"data":%s,"error":null}`, data), nil
	}
	r.Register(spec)

	r.Root().SetArgs([]string{"greet", "--name", "rick"})
	err := r.Root().Execute()
	require.NoError(t, err)
	assert.Equal(t, 0, r.exitCode)
}

// ---------------------------------------------------------------------------
// Rule group < 2 for AtMostOneOf
// ---------------------------------------------------------------------------

func TestRegister_PanicOnAtMostOneOfGroupTooSmall(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	spec := dummyLeaf("cmd1", "alpha")
	spec.Flags = []Flag{StringFlag{Name: "a"}}
	spec.Rules = RuleSpec{AtMostOneOf: [][]string{{"a"}}}
	mustPanic(t, "at least 2 entries", func() {
		r.Register(spec)
	})
}

// ---------------------------------------------------------------------------
// String representation of paths in parent key
// ---------------------------------------------------------------------------

func TestRegisterParent_NestedPath(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	r.RegisterParent(ParentSpec{
		Path:  MustNewPath("message"),
		Short: "Message commands",
	})
	r.RegisterParent(ParentSpec{
		Path:  MustNewPath("message", "media"),
		Short: "Media commands",
	})

	assert.Contains(t, r.parents, "message")
	assert.Contains(t, r.parents, "message media")

	// Register leaf under nested parent.
	spec := dummyLeaf("media-list", "message", "media", "list")
	spec.Exec = Local()
	r.Register(spec)

	leaf, _, err := r.Root().Find([]string{"message", "media", "list"})
	require.NoError(t, err)
	assert.Equal(t, "list", leaf.Use)
}

// ---------------------------------------------------------------------------
// Enum with explicitly set empty string
// ---------------------------------------------------------------------------

func TestValidateEnums_EmptyDefault_NotChecked(t *testing.T) {
	t.Parallel()
	// If the flag is not set (value is empty string default), skip enum check.
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("color", "", "")
	fv := NewFlagValues(cmd)

	flags := []Flag{
		StringFlag{Name: "color", Enum: []string{"red", "blue"}},
	}
	// Not set, empty default: should pass.
	require.NoError(t, validateEnums(fv, flags))
}

// ---------------------------------------------------------------------------
// Multiple ExactlyOneOf groups
// ---------------------------------------------------------------------------

func TestBuildRunE_MultipleExactlyOneOfGroups(t *testing.T) {
	t.Parallel()
	spec := dummyLeaf("cmd1", "test")
	spec.Exec = Local()
	spec.Flags = []Flag{
		StringFlag{Name: "a"},
		StringFlag{Name: "b"},
		StringFlag{Name: "c"},
		StringFlag{Name: "d"},
	}
	spec.Rules = RuleSpec{
		ExactlyOneOf: [][]string{{"a", "b"}, {"c", "d"}},
	}

	// Both groups satisfied.
	err := execCmd(t, spec, "--a", "x", "--c", "y")
	require.NoError(t, err)

	// First group not satisfied.
	err = execCmd(t, spec, "--c", "y")
	require.Error(t, err)
}

// ---------------------------------------------------------------------------
// Hidden and Deprecated leaf
// ---------------------------------------------------------------------------

func TestRegister_HiddenAndDeprecatedLeaf(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	spec := dummyLeaf("old-cmd", "oldcmd")
	spec.Exec = Local()
	spec.Doc = DocSpec{
		Short:      "Old command",
		Hidden:     true,
		Deprecated: "use newcmd instead",
	}
	r.Register(spec)

	leaf, _, err := r.Root().Find([]string{"oldcmd"})
	require.NoError(t, err)
	assert.True(t, leaf.Hidden)
	assert.Equal(t, "use newcmd instead", leaf.Deprecated)
}

// ---------------------------------------------------------------------------
// Examples
// ---------------------------------------------------------------------------

func TestRegister_ExamplesJoined(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	spec := dummyLeaf("cmd1", "demo")
	spec.Exec = Local()
	spec.Doc = DocSpec{
		Short:    "Demo",
		Examples: []string{"demo --foo bar", "demo --baz qux"},
	}
	r.Register(spec)

	leaf, _, _ := r.Root().Find([]string{"demo"})
	assert.Equal(t, "demo --foo bar\ndemo --baz qux", leaf.Example)
}

// ---------------------------------------------------------------------------
// StringSliceFlag default
// ---------------------------------------------------------------------------

func TestRegister_PanicOnRequiredIntFlagWithDefault(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	spec := dummyLeaf("cmd1", "alpha")
	spec.Flags = []Flag{
		IntFlag{Name: "count", Default: 10, Required: true},
	}
	mustPanic(t, "must not have a non-zero default", func() {
		r.Register(spec)
	})
}

func TestRegister_PanicOnDuplicateShortFlag(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	spec := dummyLeaf("cmd1", "alpha")
	spec.Flags = []Flag{
		StringFlag{Name: "foo", Short: "f", Help: "first"},
		IntFlag{Name: "bar", Short: "f", Help: "second"},
	}
	mustPanic(t, "duplicate short flag -f", func() {
		r.Register(spec)
	})
}

func TestRegister_StringSliceFlagDefault(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	spec := dummyLeaf("cmd1", "slice")
	spec.Exec = Local()
	spec.Flags = []Flag{
		StringSliceFlag{Name: "tags", Default: []string{"a", "b"}},
	}
	spec.Run = func(ctx context.Context, app *commands.App, f FlagValues) (string, error) {
		tags := f.StringSlice("tags")
		return fmt.Sprintf(`{"success":true,"data":{"tags":"%s"},"error":null}`, strings.Join(tags, ",")), nil
	}
	r.Register(spec)

	r.Root().SetArgs([]string{"slice"})
	err := r.Root().Execute()
	require.NoError(t, err)
	assert.Equal(t, 0, r.exitCode)
}
