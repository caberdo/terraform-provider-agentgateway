package configresource

import (
	"encoding/json"
	"fmt"
	"strings"
)

const reservedMetadataPrefix = "agentgateway.dev/"

// managedMetadataKeys are injected by the server and must be ignored during
// drift detection.
var managedMetadataKeys = map[string]struct{}{
	reservedMetadataPrefix + "id":        {},
	reservedMetadataPrefix + "createdAt": {},
	reservedMetadataPrefix + "owner":     {},
}

// StripManagedMetadata removes server-managed metadata keys from llm.apiKey
// values. For all other kinds it returns the value unchanged.
func StripManagedMetadata(value json.RawMessage) json.RawMessage {
	// We cannot determine the kind here; callers that manage llm.apiKey pass
	// the value through this helper before comparison.
	var body map[string]json.RawMessage
	if err := json.Unmarshal(value, &body); err != nil {
		// Not a JSON object: return as-is.
		return value
	}

	rawMeta, ok := body["metadata"]
	if !ok {
		return value
	}

	var metadata map[string]json.RawMessage
	if err := json.Unmarshal(rawMeta, &metadata); err != nil {
		// metadata is not an object: return as-is.
		return value
	}

	changed := false
	for k := range managedMetadataKeys {
		if _, ok := metadata[k]; ok {
			delete(metadata, k)
			changed = true
		}
	}
	if !changed {
		return value
	}

	newMeta, err := json.Marshal(metadata)
	if err != nil {
		// Should never happen for a map we just decoded.
		return value
	}
	body["metadata"] = newMeta

	out, err := json.Marshal(body)
	if err != nil {
		// Should never happen for a map we just decoded.
		return value
	}
	return out
}

// HasReservedMetadataKey returns an error if any metadata key uses the
// agentgateway.dev/ reserved prefix.
func HasReservedMetadataKey(metadata map[string]string) error {
	for k := range metadata {
		if strings.HasPrefix(k, reservedMetadataPrefix) {
			return fmt.Errorf("metadata key %q uses reserved prefix %q", k, reservedMetadataPrefix)
		}
	}
	return nil
}
