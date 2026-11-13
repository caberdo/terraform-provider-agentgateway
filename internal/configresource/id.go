package configresource

import (
	"encoding/json"
	"fmt"
)

// DeriveID returns the config-resource id that the agentgateway server would
// assign for the given kind and value.
func DeriveID(kind Kind, value json.RawMessage) (string, error) {
	if kind == KindLLMSettings || kind == KindMCPSettings || kind == KindModelCatalog {
		return "default", nil
	}

	var body map[string]json.RawMessage
	if err := json.Unmarshal(value, &body); err != nil {
		return "", fmt.Errorf("decode value for %s id derivation: %w", kind, err)
	}

	switch kind {
	case KindLLMProvider, KindLLMVirtualModel, KindMCPTarget,
		KindTrafficGateway, KindTrafficRoute, KindTrafficTCPRoute,
		KindLLMPolicy, KindMCPPolicy, KindUIPolicy, KindFrontendPolicy:
		return stringField(body, "name", kind)
	case KindLLMModel:
		if id, ok := body["id"]; ok && len(id) > 0 {
			return unquoteString(id, kind)
		}
		return stringField(body, "name", kind)
	case KindLLMAPIKey:
		return "", fmt.Errorf("cannot derive id for %s: id is server-generated", kind)
	}

	return "", fmt.Errorf("unknown kind %q", kind)
}

func stringField(body map[string]json.RawMessage, field string, kind Kind) (string, error) {
	raw, ok := body[field]
	if !ok {
		return "", fmt.Errorf("%s value missing %q", kind, field)
	}
	return unquoteString(raw, kind)
}

func unquoteString(raw json.RawMessage, kind Kind) (string, error) {
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", fmt.Errorf("%s id is not a string: %w", kind, err)
	}
	if s == "" {
		return "", fmt.Errorf("%s id is empty", kind)
	}
	return s, nil
}

