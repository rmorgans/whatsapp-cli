package output

import "encoding/json"

// Result is the typed envelope. The registry is the only caller of Marshal.
type Result struct {
	Success bool    `json:"success"`
	Data    any     `json:"data"`
	Error   *string `json:"error"`
}

// SuccessResult builds a Result envelope for successful commands.
func SuccessResult(data any) Result {
	return Result{Success: true, Data: data}
}

// Failure builds a Result envelope from an error.
func Failure(err error) Result {
	msg := err.Error()
	return Result{Success: false, Error: &msg}
}

// Marshal serializes a Result to JSON bytes.
func Marshal(r Result) ([]byte, error) {
	return json.Marshal(r)
}

