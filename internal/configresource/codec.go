package configresource

import (
	"encoding/json"
)

// EncodeValue marshals a Go value to JSON. It is a thin wrapper around
// encoding/json.Marshal that preserves the distinction between absent and
// empty values (for example, a nil pointer is omitted while a pointer to an
// empty slice encodes as []).
func EncodeValue(v any) (json.RawMessage, error) {
	return json.Marshal(v)
}

// DecodeValue unmarshals JSON into the provided target. It is a thin wrapper
// around encoding/json.Unmarshal.
func DecodeValue(raw json.RawMessage, v any) error {
	return json.Unmarshal(raw, v)
}
