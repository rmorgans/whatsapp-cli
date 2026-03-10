package output

import (
	"encoding/json"
	"fmt"
)

// Envelope mirrors Result but preserves raw JSON in Data so formatters
// can decode command-specific payloads without losing type information.
type Envelope struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Error   *string         `json:"error"`
}

// OutputMode controls whether output is machine-readable JSON or
// human-friendly formatted text.
type OutputMode int

const (
	ModeJSON  OutputMode = iota
	ModeHuman
)

// ParseEnvelope unmarshals a JSON string into an Envelope.
// It returns an error if the input is not valid JSON or does not
// conform to the expected structure.
func ParseEnvelope(raw string) (Envelope, error) {
	var env Envelope
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		return Envelope{}, fmt.Errorf("parse envelope: %w", err)
	}
	return env, nil
}
