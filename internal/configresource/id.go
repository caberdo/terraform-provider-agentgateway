package configresource

import (
	"encoding/json"
	"fmt"
)

// DeriveID returns the config-resource id that the agentgateway server would
// assign for the given kind and value. It is defined only for non-policy kinds;
// policy ids come from the resource's name attribute, not from the policy body,
// so callers must use KindResource.ID() for those.
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
		KindTrafficGateway, KindTrafficRoute, KindTrafficTCPRoute:
		return stringField(body, "name", kind)
	case KindLLMModel:
		if idRaw, ok := body["id"]; ok && len(idRaw) > 0 {
			var id string
			if err := json.Unmarshal(idRaw, &id); err == nil && id != "" {
				return id, nil
			}
		}
		return stringField(body, "name", kind)
	case KindLLMAPIKey:
		return "", fmt.Errorf("cannot derive id for %s: id is server-generated", kind)
	case KindLLMPolicy, KindMCPPolicy, KindUIPolicy, KindFrontendPolicy:
		return "", fmt.Errorf("%s id is the policy key and comes from the resource name attribute, not the value", kind)
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

