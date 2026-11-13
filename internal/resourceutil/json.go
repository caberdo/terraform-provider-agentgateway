package resourceutil

import (
	"encoding/json"
)

// decodeJSONObject decodes raw into a JSON object. isObject is false when the
// value is valid JSON but not an object (an array, string, number, bool or
// null). A decode failure is returned as err.
func decodeJSONObject(raw json.RawMessage) (body map[string]json.RawMessage, isObject bool, err error) {
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, false, err
	}
	if body == nil {
		return nil, false, nil
	}
	return body, true, nil
}
