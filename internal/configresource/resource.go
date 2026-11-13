package configresource

import (
	"encoding/json"
	"time"
)

// Resource is a config resource as stored by the agentgateway API.
type Resource struct {
	Kind      Kind
	ID        string
	Value     json.RawMessage
	Revision  int64
	CreatedAt time.Time
	UpdatedAt time.Time
}
