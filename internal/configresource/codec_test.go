package configresource

import (
	"encoding/json"
	"testing"
)

type codecValue struct {
	AllowedModels *[]string `json:"allowedModels,omitempty"`
}

func TestCodecRoundTripPreservesNullVsEmpty(t *testing.T) {
	t.Run("nil pointer omitted", func(t *testing.T) {
		v := codecValue{}
		encoded, err := EncodeValue(v)
		if err != nil {
			t.Fatalf("EncodeValue error: %v", err)
		}
		if string(encoded) != "{}" {
			t.Fatalf("expected {}, got %s", encoded)
		}

		var decoded codecValue
		if err := DecodeValue(encoded, &decoded); err != nil {
			t.Fatalf("DecodeValue error: %v", err)
		}
		if decoded.AllowedModels != nil {
			t.Fatalf("expected nil AllowedModels, got %v", decoded.AllowedModels)
		}
	})

	t.Run("empty slice preserved", func(t *testing.T) {
		empty := []string{}
		v := codecValue{AllowedModels: &empty}
		encoded, err := EncodeValue(v)
		if err != nil {
			t.Fatalf("EncodeValue error: %v", err)
		}
		if string(encoded) != `{"allowedModels":[]}` {
			t.Fatalf("expected {\"allowedModels\":[]}, got %s", encoded)
		}

		var decoded codecValue
		if err := DecodeValue(encoded, &decoded); err != nil {
			t.Fatalf("DecodeValue error: %v", err)
		}
		if decoded.AllowedModels == nil {
			t.Fatal("expected non-nil AllowedModels, got nil")
		}
		if len(*decoded.AllowedModels) != 0 {
			t.Fatalf("expected empty AllowedModels, got %v", *decoded.AllowedModels)
		}
	})

	t.Run("decode from raw json", func(t *testing.T) {
		encoded := json.RawMessage(`{"allowedModels":["gpt-4o"]}`)
		var decoded codecValue
		if err := DecodeValue(encoded, &decoded); err != nil {
			t.Fatalf("DecodeValue error: %v", err)
		}
		if decoded.AllowedModels == nil {
			t.Fatal("expected non-nil AllowedModels, got nil")
		}
		if len(*decoded.AllowedModels) != 1 || (*decoded.AllowedModels)[0] != "gpt-4o" {
			t.Fatalf("expected [gpt-4o], got %v", *decoded.AllowedModels)
		}
	})
}
