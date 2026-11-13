package configresource

import (
	"encoding/json"
	"testing"
)

func TestDeriveID(t *testing.T) {
	cases := []struct {
		name    string
		kind    Kind
		value   string
		want    string
		wantErr bool
	}{
		{
			name:  "provider uses name",
			kind:  KindLLMProvider,
			value: `{"name":"openai-prod"}`,
			want:  "openai-prod",
		},
		{
			name:  "model prefers id over name",
			kind:  KindLLMModel,
			value: `{"id":"gpt-prod","name":"gpt-*"}`,
			want:  "gpt-prod",
		},
		{
			name:  "model falls back to name",
			kind:  KindLLMModel,
			value: `{"name":"gpt-4o"}`,
			want:  "gpt-4o",
		},
		{
			name:  "settings singleton",
			kind:  KindLLMSettings,
			value: `{}`,
			want:  "default",
		},
		{
			name:  "model catalog singleton",
			kind:  KindModelCatalog,
			value: `{}`,
			want:  "default",
		},
		{
			name:    "provider without name errors",
			kind:    KindLLMProvider,
			value:   `{}`,
			wantErr: true,
		},
		{
			name:  "model falls back to name when id is null",
			kind:  KindLLMModel,
			value: `{"id":null,"name":"gpt-4o"}`,
			want:  "gpt-4o",
		},
		{
			name:  "model falls back to name when id is empty string",
			kind:  KindLLMModel,
			value: `{"id":"","name":"gpt-4o"}`,
			want:  "gpt-4o",
		},
		{
			name:    "policy id is not derived from value",
			kind:    KindLLMPolicy,
			value:   `{"name":"apiKey"}`,
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := DeriveID(tc.kind, json.RawMessage(tc.value))
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("DeriveID() = %q, want %q", got, tc.want)
			}
		})
	}
}
