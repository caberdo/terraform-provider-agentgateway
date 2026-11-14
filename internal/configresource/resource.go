package configresource

import (
	"encoding/json"
	"time"
)

// Resource is a config resource as stored by the agentgateway API. The JSON
// tags are the API envelope (spec §5.2), so this type is the one wire shape
// shared by the client, the protocol fake and any other consumer. The tags are
// spelled explicitly rather than relying on encoding/json's case-insensitive
// field fallback, so a wire-format change fails loudly instead of silently
// zeroing a field.
type Resource struct {
	Kind      Kind            `json:"kind"`
	ID        string          `json:"id"`
	Value     json.RawMessage `json:"value"`
	Revision  int64           `json:"revision"`
	CreatedAt time.Time       `json:"createdAt"`
	UpdatedAt time.Time       `json:"updatedAt"`
}
