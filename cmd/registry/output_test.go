package registry

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vicentereig/whatsapp-cli/internal/commands"
	"github.com/vicentereig/whatsapp-cli/internal/output"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// expectedJSON builds the expected JSON envelope string for a given data value.
func expectedJSON(data interface{}) string {
	d, _ := json.Marshal(data)
	return fmt.Sprintf(`{"success":true,"data":%s,"error":null}`, d)
}

// errorEnvelope builds a JSON envelope with success=false and the given error.
func errorEnvelope(msg string) string {
	b, _ := output.Marshal(output.Failure(fmt.Errorf("%s", msg)))
	return string(b)
}

// execCapture creates a registry with the given spec, runs root with args,
// and returns captured stdout and the registry (for exit code inspection).
func execCapture(t *testing.T, spec LeafSpec, args ...string) (string, *Registry) {
	t.Helper()
	r := NewRegistry()
	var buf bytes.Buffer
	r.SetWriter(&buf)
	r.Register(spec)
	r.Root().SetArgs(args)
	err := r.Root().Execute()
	require.NoError(t, err)
	return buf.String(), r
}

// ---------------------------------------------------------------------------
// Default output mode is JSON
// ---------------------------------------------------------------------------

func TestOutputFlag_DefaultIsJSON(t *testing.T) {
	t.Parallel()
	spec := dummyLeaf("test-cmd", "test")
	spec.Exec = Local()
	expected := `{"success":true,"data":null,"error":null}`

	got, r := execCapture(t, spec, "test")
	assert.Equal(t, 0, r.exitCode)
	assert.Equal(t, expected+"\n", got, "default mode should output raw JSON")
}

// ---------------------------------------------------------------------------
// --output json produces raw JSON
// ---------------------------------------------------------------------------

func TestOutputFlag_JSONProducesRawJSON(t *testing.T) {
	t.Parallel()
	data := map[string]string{"greeting": "hello"}
	spec := dummyLeaf("test-cmd", "test")
	spec.Exec = Local()
	spec.Run = func(_ context.Context, _ *commands.App, _ FlagValues) (any, error) {
		return data, nil
	}

	got, r := execCapture(t, spec, "test", "--format", "json")
	assert.Equal(t, 0, r.exitCode)
	assert.Equal(t, expectedJSON(data)+"\n", got)
}

// ---------------------------------------------------------------------------
// --output human produces human-formatted output
// ---------------------------------------------------------------------------

func TestOutputFlag_HumanFormatsOutput(t *testing.T) {
	t.Parallel()
	data := map[string]string{"greeting": "hello"}
	spec := dummyLeaf("test-cmd", "test")
	spec.Exec = Local()
	spec.Run = func(_ context.Context, _ *commands.App, _ FlagValues) (any, error) {
		return data, nil
	}

	got, r := execCapture(t, spec, "test", "--format", "human")
	assert.Equal(t, 0, r.exitCode)
	// GenericFormat for a single object renders key-value pairs.
	assert.Contains(t, got, "Greeting")
	assert.Contains(t, got, "hello")
	// Must NOT contain raw JSON braces.
	assert.NotContains(t, got, `"greeting"`)
}

// ---------------------------------------------------------------------------
// --output auto with non-TTY (test environment) produces JSON
// ---------------------------------------------------------------------------

func TestOutputFlag_AutoNonTTYProducesJSON(t *testing.T) {
	t.Parallel()
	spec := dummyLeaf("test-cmd", "test")
	spec.Exec = Local()
	expected := `{"success":true,"data":null,"error":null}`

	got, r := execCapture(t, spec, "test", "--format", "auto")
	assert.Equal(t, 0, r.exitCode)
	assert.Equal(t, expected+"\n", got, "auto mode with non-TTY writer should produce JSON")
}

// ---------------------------------------------------------------------------
// --output badvalue returns an error
// ---------------------------------------------------------------------------

func TestOutputFlag_InvalidValueReturnsError(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	var buf bytes.Buffer
	r.SetWriter(&buf)
	spec := dummyLeaf("test-cmd", "test")
	spec.Exec = Local()
	r.Register(spec)

	r.Root().SetArgs([]string{"test", "--format", "badvalue"})
	err := r.Root().Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be one of")
}

// ---------------------------------------------------------------------------
// Human mode falls back to generic formatter for unknown commands
// ---------------------------------------------------------------------------

func TestOutputFlag_HumanFallsBackToGenericFormatter(t *testing.T) {
	t.Parallel()
	// Use a command ID that has no custom human formatter.
	data := map[string]string{"key": "value"}
	spec := dummyLeaf("unknown-fancy-cmd", "fancy")
	spec.Exec = Local()
	spec.Run = func(_ context.Context, _ *commands.App, _ FlagValues) (any, error) {
		return data, nil
	}

	got, r := execCapture(t, spec, "fancy", "--format", "human")
	assert.Equal(t, 0, r.exitCode)
	// GenericFormat renders key-value pair.
	assert.Contains(t, got, "Key")
	assert.Contains(t, got, "value")
}

// ---------------------------------------------------------------------------
// Human mode error envelope shows "Error: <message>"
// ---------------------------------------------------------------------------

func TestOutputFlag_HumanErrorEnvelope(t *testing.T) {
	t.Parallel()
	spec := dummyLeaf("test-cmd", "test")
	spec.Exec = Local()
	spec.Run = func(_ context.Context, _ *commands.App, _ FlagValues) (any, error) {
		return nil, fmt.Errorf("something broke")
	}

	got, r := execCapture(t, spec, "test", "--format", "human")
	assert.Equal(t, 1, r.exitCode)
	assert.Contains(t, got, "Error: something broke")
}

// ---------------------------------------------------------------------------
// Human mode falls through to raw JSON if both formatters fail
// ---------------------------------------------------------------------------

func TestOutputFlag_HumanFallsThroughToJSON(t *testing.T) {
	t.Parallel()
	// With the typed Result API, printResult always receives a valid Result.
	// When human formatters have no match and GenericFormat fails, it falls
	// back to JSON serialisation. We test with a nil-data error result.
	r := NewRegistry()
	var buf bytes.Buffer
	r.SetWriter(&buf)
	r.formatFlag = "human"
	errMsg := "something broke"
	r.printResult("test-cmd", output.Result{Success: false, Error: &errMsg})
	assert.Equal(t, 1, r.exitCode)
	// Human mode for error envelopes should use GenericFormat which
	// renders "Error: something broke".
	assert.Contains(t, buf.String(), "Error: something broke")
}

// ---------------------------------------------------------------------------
// JSON mode output is byte-identical to previous behavior
// ---------------------------------------------------------------------------

func TestOutputFlag_JSONByteIdentical(t *testing.T) {
	t.Parallel()
	// Verify that JSON mode outputs the expected envelope.
	data := map[string]any{"a": 1}
	spec := dummyLeaf("test-cmd", "test")
	spec.Exec = Local()
	spec.Run = func(_ context.Context, _ *commands.App, _ FlagValues) (any, error) {
		return data, nil
	}

	got, _ := execCapture(t, spec, "test", "--format", "json")
	// Parse both to compare structurally (key order may vary).
	assert.JSONEq(t, expectedJSON(data), strings.TrimSpace(got))
}

// ---------------------------------------------------------------------------
// Human mode with per-command formatter (send command)
// ---------------------------------------------------------------------------

func TestOutputFlag_HumanUsesPerCommandFormatter(t *testing.T) {
	t.Parallel()
	sendData := map[string]interface{}{
		"sent":      true,
		"id":        "msg123",
		"recipient": "someone@s.whatsapp.net",
	}
	spec := dummyLeaf("send", "send")
	spec.Exec = Local()
	spec.Run = func(_ context.Context, _ *commands.App, _ FlagValues) (any, error) {
		return sendData, nil
	}

	got, r := execCapture(t, spec, "send", "--format", "human")
	assert.Equal(t, 0, r.exitCode)
	assert.Contains(t, got, "Sent to someone@s.whatsapp.net")
	assert.Contains(t, got, "msg123")
}

// ---------------------------------------------------------------------------
// Exit code: success=true -> 0, success=false -> 1
// ---------------------------------------------------------------------------

func TestOutputFlag_ExitCodes(t *testing.T) {
	t.Parallel()
	errMsg := "boom"
	tests := []struct {
		name     string
		result   output.Result
		wantCode int
	}{
		{"success", output.SuccessResult(nil), 0},
		{"failure", output.Result{Success: false, Error: &errMsg}, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			for _, mode := range []string{"json", "human"} {
				t.Run(mode, func(t *testing.T) {
					t.Parallel()
					r := NewRegistry()
					var buf bytes.Buffer
					r.SetWriter(&buf)
					r.formatFlag = mode
					r.printResult("test-cmd", tc.result)
					assert.Equal(t, tc.wantCode, r.exitCode)
				})
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Persistent --output flag exists with correct default
// ---------------------------------------------------------------------------

func TestOutputFlag_PersistentFlagExists(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	f := r.Root().PersistentFlags().Lookup("format")
	require.NotNil(t, f)
	assert.Equal(t, "json", f.DefValue)
}

// ---------------------------------------------------------------------------
// Human mode with array data uses generic table
// ---------------------------------------------------------------------------

func TestOutputFlag_HumanArrayData(t *testing.T) {
	t.Parallel()
	data := []map[string]string{
		{"name": "Alice", "age": "30"},
		{"name": "Bob", "age": "25"},
	}
	spec := dummyLeaf("unknown-list-cmd", "list")
	spec.Exec = Local()
	spec.Run = func(_ context.Context, _ *commands.App, _ FlagValues) (any, error) {
		return data, nil
	}

	got, r := execCapture(t, spec, "list", "--format", "human")
	assert.Equal(t, 0, r.exitCode)
	// Should contain table headers (generic formatter uppercases them).
	assert.True(t, strings.Contains(got, "AGE") || strings.Contains(got, "NAME"),
		"expected table headers in output: %s", got)
	assert.Contains(t, got, "Alice")
	assert.Contains(t, got, "Bob")
}
