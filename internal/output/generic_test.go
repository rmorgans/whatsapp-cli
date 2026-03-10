package output

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// frozenNow is a fixed point in time for deterministic tests.
// 2026-03-10T14:30:00Z (a Tuesday)
var frozenNow = time.Date(2026, 3, 10, 14, 30, 0, 0, time.UTC)

func freezeTime(t *testing.T) {
	t.Helper()
	orig := timeNow
	timeNow = func() time.Time { return frozenNow }
	t.Cleanup(func() { timeNow = orig })
}

func envelope(success bool, data interface{}, errMsg *string) Envelope {
	d, _ := json.Marshal(data)
	return Envelope{
		Success: success,
		Data:    json.RawMessage(d),
		Error:   errMsg,
	}
}

func errEnvelope(msg string) Envelope {
	return Envelope{
		Success: false,
		Data:    json.RawMessage("null"),
		Error:   &msg,
	}
}

func TestGenericFormat_ErrorEnvelope(t *testing.T) {
	t.Parallel()

	got, err := GenericFormat(errEnvelope("connection refused"))
	require.NoError(t, err)
	assert.Equal(t, "Error: connection refused", got)
}

func TestGenericFormat_ErrorEnvelope_NilMessage(t *testing.T) {
	t.Parallel()

	env := Envelope{Success: false, Data: json.RawMessage("null"), Error: nil}
	got, err := GenericFormat(env)
	require.NoError(t, err)
	assert.Equal(t, "Error: unknown error", got)
}

func TestGenericFormat_NullData(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		env  Envelope
	}{
		{
			name: "explicit null",
			env:  envelope(true, nil, nil),
		},
		{
			name: "empty data field",
			env:  Envelope{Success: true, Data: json.RawMessage{}, Error: nil},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := GenericFormat(tt.env)
			require.NoError(t, err)
			assert.Equal(t, "No results.", got)
		})
	}
}

func TestGenericFormat_EmptyArray(t *testing.T) {
	t.Parallel()

	env := envelope(true, []interface{}{}, nil)
	got, err := GenericFormat(env)
	require.NoError(t, err)
	assert.Equal(t, "No results.", got)
}

func TestGenericFormat_ArrayOfScalars(t *testing.T) {
	t.Parallel()

	env := envelope(true, []interface{}{"alpha", "bravo", "charlie"}, nil)
	got, err := GenericFormat(env)
	require.NoError(t, err)
	assert.Equal(t, "alpha\nbravo\ncharlie", got)
}

func TestGenericFormat_ArrayOfNumbers(t *testing.T) {
	t.Parallel()

	env := envelope(true, []interface{}{1.0, 2.0, 3.5}, nil)
	got, err := GenericFormat(env)
	require.NoError(t, err)
	assert.Equal(t, "1\n2\n3.5", got)
}

func TestGenericFormat_ScalarString(t *testing.T) {
	t.Parallel()

	env := envelope(true, "hello world", nil)
	got, err := GenericFormat(env)
	require.NoError(t, err)
	assert.Equal(t, "hello world", got)
}

func TestGenericFormat_ScalarNumber(t *testing.T) {
	t.Parallel()

	env := envelope(true, 42.0, nil)
	got, err := GenericFormat(env)
	require.NoError(t, err)
	assert.Equal(t, "42", got)
}

func TestGenericFormat_ScalarBool(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		val  bool
		want string
	}{
		{"true", true, "yes"},
		{"false", false, "no"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			env := envelope(true, tt.val, nil)
			got, err := GenericFormat(env)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGenericFormat_SingleObject(t *testing.T) {
	t.Parallel()

	obj := map[string]interface{}{
		"chat_jid":   "123@s.whatsapp.net",
		"name":       "Alice",
		"push_name":  "Ali",
	}

	env := envelope(true, obj, nil)
	got, err := GenericFormat(env)
	require.NoError(t, err)

	// Keys sorted alphabetically: chat_jid, name, push_name
	// Title cased: Chat Jid, Name, Push Name
	lines := strings.Split(got, "\n")
	require.Len(t, lines, 3)
	assert.Contains(t, lines[0], "Chat Jid:")
	assert.Contains(t, lines[0], "123@s.whatsapp.net")
	assert.Contains(t, lines[1], "Name:")
	assert.Contains(t, lines[1], "Alice")
	assert.Contains(t, lines[2], "Push Name:")
	assert.Contains(t, lines[2], "Ali")

	// Values should be aligned (same column start).
	idx0 := strings.Index(lines[0], "123")
	idx1 := strings.Index(lines[1], "Alice")
	idx2 := strings.Index(lines[2], "Ali")
	assert.Equal(t, idx0, idx1, "values should be aligned")
	assert.Equal(t, idx1, idx2, "values should be aligned")
}

func TestGenericFormat_Table(t *testing.T) {
	t.Parallel()

	data := []interface{}{
		map[string]interface{}{"name": "Alice", "age": 30.0},
		map[string]interface{}{"name": "Bob", "age": 25.0},
	}

	env := envelope(true, data, nil)
	got, err := GenericFormat(env)
	require.NoError(t, err)

	lines := strings.Split(got, "\n")
	require.Len(t, lines, 3) // header + 2 rows

	// Header: alphabetical keys -> age, name -> AGE, NAME
	assert.Contains(t, lines[0], "AGE")
	assert.Contains(t, lines[0], "NAME")
	assert.True(t, strings.Index(lines[0], "AGE") < strings.Index(lines[0], "NAME"),
		"AGE should appear before NAME (alphabetical)")
}

func TestGenericFormat_TableBooleans(t *testing.T) {
	t.Parallel()

	data := []interface{}{
		map[string]interface{}{"active": true, "name": "X"},
		map[string]interface{}{"active": false, "name": "Y"},
	}

	env := envelope(true, data, nil)
	got, err := GenericFormat(env)
	require.NoError(t, err)

	lines := strings.Split(got, "\n")
	require.Len(t, lines, 3)
	assert.Contains(t, lines[1], "yes")
	assert.Contains(t, lines[2], "no")
}

func TestGenericFormat_TableNullValues(t *testing.T) {
	t.Parallel()

	data := []interface{}{
		map[string]interface{}{"name": "Alice", "email": nil},
		map[string]interface{}{"name": "Bob", "email": "bob@test.com"},
	}

	env := envelope(true, data, nil)
	got, err := GenericFormat(env)
	require.NoError(t, err)

	lines := strings.Split(got, "\n")
	require.Len(t, lines, 3)
	// Alice row: email column should be blank (just spaces), not "null" or "<nil>"
	assert.NotContains(t, lines[1], "null")
	assert.NotContains(t, lines[1], "<nil>")
}

func TestGenericFormat_TableNestedCollapse(t *testing.T) {
	t.Parallel()

	data := []interface{}{
		map[string]interface{}{
			"id":   1.0,
			"meta": map[string]interface{}{"foo": "bar"},
			"tags": []interface{}{"a", "b"},
		},
	}

	env := envelope(true, data, nil)
	got, err := GenericFormat(env)
	require.NoError(t, err)

	assert.Contains(t, got, "{...}")
	assert.Contains(t, got, "[...]")
}

func TestGenericFormat_TableTimestamps(t *testing.T) {
	freezeTime(t)

	now := frozenNow

	data := []interface{}{
		map[string]interface{}{
			"label": "just now",
			"ts":    now.Add(-30 * time.Second).Format(time.RFC3339),
		},
		map[string]interface{}{
			"label": "2 hours",
			"ts":    now.Add(-2 * time.Hour).Format(time.RFC3339),
		},
		map[string]interface{}{
			"label": "yesterday",
			"ts":    now.Add(-30 * time.Hour).Format(time.RFC3339),
		},
		map[string]interface{}{
			"label": "3 days",
			"ts":    now.Add(-72 * time.Hour).Format(time.RFC3339),
		},
		map[string]interface{}{
			"label": "old",
			"ts":    time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC).Format(time.RFC3339),
		},
		map[string]interface{}{
			"label": "same year",
			"ts":    time.Date(2026, 1, 5, 10, 0, 0, 0, time.UTC).Format(time.RFC3339),
		},
	}

	env := envelope(true, data, nil)
	got, err := GenericFormat(env)
	require.NoError(t, err)

	lines := strings.Split(got, "\n")
	// header + 6 rows
	require.Len(t, lines, 7)

	assert.Contains(t, lines[1], "just now")     // <1 min
	assert.Contains(t, lines[2], "2h ago")        // hours
	assert.Contains(t, lines[3], "yesterday")     // ~30h ago
	assert.Contains(t, lines[4], "3 days ago")    // 72h
	assert.Contains(t, lines[5], "Jan 15 2025")   // different year
	assert.Contains(t, lines[6], "Jan 5")         // same year, no year suffix
}

func TestGenericFormat_KeyValueTimestampAndBool(t *testing.T) {
	freezeTime(t)

	obj := map[string]interface{}{
		"active":     true,
		"created_at": frozenNow.Add(-3 * time.Hour).Format(time.RFC3339),
	}

	env := envelope(true, obj, nil)
	got, err := GenericFormat(env)
	require.NoError(t, err)

	assert.Contains(t, got, "yes")
	assert.Contains(t, got, "3h ago")
}

func TestGenericFormat_KeyValueNestedCollapse(t *testing.T) {
	t.Parallel()

	obj := map[string]interface{}{
		"config": map[string]interface{}{"a": 1},
		"items":  []interface{}{1, 2},
	}

	env := envelope(true, obj, nil)
	got, err := GenericFormat(env)
	require.NoError(t, err)

	assert.Contains(t, got, "{...}")
	assert.Contains(t, got, "[...]")
}

func TestGenericFormat_TableColumnWidthCap(t *testing.T) {
	t.Parallel()

	longVal := strings.Repeat("x", 50) // exceeds 40-char cap

	data := []interface{}{
		map[string]interface{}{"value": longVal},
	}

	env := envelope(true, data, nil)
	got, err := GenericFormat(env)
	require.NoError(t, err)

	lines := strings.Split(got, "\n")
	require.Len(t, lines, 2)

	// The data row should be truncated to 40 chars (39 + ellipsis)
	dataLine := lines[1]
	assert.LessOrEqual(t, len([]rune(dataLine)), maxColWidth)
	assert.True(t, strings.HasSuffix(dataLine, "\u2026"),
		"truncated value should end with ellipsis")
}

func TestGenericFormat_TableTotalWidthCap(t *testing.T) {
	t.Parallel()

	// Create an object with many columns that would exceed 120 chars total.
	obj := map[string]interface{}{}
	for i := 0; i < 20; i++ {
		key := fmt.Sprintf("column_%02d", i)
		obj[key] = strings.Repeat("a", 15)
	}

	data := []interface{}{obj}
	env := envelope(true, data, nil)
	got, err := GenericFormat(env)
	require.NoError(t, err)

	lines := strings.Split(got, "\n")
	for _, line := range lines {
		assert.LessOrEqual(t, len([]rune(line)), maxTotalWidth,
			"no line should exceed max total width: %q", line)
	}
}

func TestGenericFormat_NoTrailingWhitespace(t *testing.T) {
	t.Parallel()

	data := []interface{}{
		map[string]interface{}{"name": "Alice", "x": "short"},
		map[string]interface{}{"name": "Bob", "x": "a bit longer value"},
	}

	env := envelope(true, data, nil)
	got, err := GenericFormat(env)
	require.NoError(t, err)

	for i, line := range strings.Split(got, "\n") {
		assert.Equal(t, strings.TrimRight(line, " \t"), line,
			"line %d has trailing whitespace", i)
	}
}

func TestGenericFormat_TableHeaderFormat(t *testing.T) {
	t.Parallel()

	data := []interface{}{
		map[string]interface{}{"chat_jid": "123", "push_name": "Alice"},
	}

	env := envelope(true, data, nil)
	got, err := GenericFormat(env)
	require.NoError(t, err)

	lines := strings.Split(got, "\n")
	require.True(t, len(lines) >= 1)
	assert.Contains(t, lines[0], "CHAT JID")
	assert.Contains(t, lines[0], "PUSH NAME")
}

func TestGenericFormat_Deterministic(t *testing.T) {
	t.Parallel()

	data := []interface{}{
		map[string]interface{}{"b": 2, "a": 1, "c": 3},
		map[string]interface{}{"a": 4, "c": 6, "b": 5},
	}

	env := envelope(true, data, nil)

	// Run multiple times — output must be identical.
	first, err := GenericFormat(env)
	require.NoError(t, err)

	for i := 0; i < 10; i++ {
		got, err := GenericFormat(env)
		require.NoError(t, err)
		assert.Equal(t, first, got, "iteration %d produced different output", i)
	}
}
