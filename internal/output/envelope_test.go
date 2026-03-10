package output

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseEnvelope(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    Envelope
		wantErr bool
	}{
		{
			name:  "success with object data",
			input: `{"success":true,"data":{"name":"John"},"error":null}`,
			want: Envelope{
				Success: true,
				Data:    json.RawMessage(`{"name":"John"}`),
				Error:   nil,
			},
		},
		{
			name:  "success with array data",
			input: `{"success":true,"data":["a","b"],"error":null}`,
			want: Envelope{
				Success: true,
				Data:    json.RawMessage(`["a","b"]`),
				Error:   nil,
			},
		},
		{
			name:  "success with null data",
			input: `{"success":true,"data":null,"error":null}`,
			want: Envelope{
				Success: true,
				Data:    json.RawMessage("null"),
				Error:   nil,
			},
		},
		{
			name:  "error response",
			input: `{"success":false,"data":null,"error":"something broke"}`,
			want: Envelope{
				Success: false,
				Data:    json.RawMessage("null"),
				Error:   strPtr("something broke"),
			},
		},
		{
			name:    "invalid JSON",
			input:   `not json at all`,
			wantErr: true,
		},
		{
			name:    "empty string",
			input:   ``,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseEnvelope(tt.input)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "parse envelope")
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want.Success, got.Success)
			assert.JSONEq(t, string(tt.want.Data), string(got.Data))

			if tt.want.Error == nil {
				assert.Nil(t, got.Error)
			} else {
				require.NotNil(t, got.Error)
				assert.Equal(t, *tt.want.Error, *got.Error)
			}
		})
	}
}

func TestParseEnvelope_PreservesRawJSON(t *testing.T) {
	t.Parallel()

	// Verify that Data stays as raw JSON and can be decoded later
	// into a specific type -- the whole point of using json.RawMessage.
	input := `{"success":true,"data":{"id":42,"label":"test"},"error":null}`

	env, err := ParseEnvelope(input)
	require.NoError(t, err)

	var payload struct {
		ID    int    `json:"id"`
		Label string `json:"label"`
	}
	err = json.Unmarshal(env.Data, &payload)
	require.NoError(t, err)
	assert.Equal(t, 42, payload.ID)
	assert.Equal(t, "test", payload.Label)
}

func strPtr(s string) *string {
	return &s
}
