package configresource

import (
	"encoding/json"
	"testing"
)

func TestStripManagedMetadata(t *testing.T) {
	cases := []struct {
		name  string
		value string
		want  string
	}{
		{
			name:  "removes managed keys",
			value: `{"key":"secret","metadata":{"agentgateway.dev/id":"x","agentgateway.dev/createdAt":"t","agentgateway.dev/owner":"o","team":"platform"}}`,
			want:  `{"key":"secret","metadata":{"team":"platform"}}`,
		},
		{
			name:  "leaves value without metadata unchanged",
			value: `{"key":"secret"}`,
			want:  `{"key":"secret"}`,
		},
		{
			name:  "leaves non-managed metadata unchanged",
			value: `{"metadata":{"team":"platform","env":"prod"}}`,
			want:  `{"metadata":{"team":"platform","env":"prod"}}`,
		},
		{
			name:  "non-object metadata is unchanged",
			value: `{"metadata":["not","an","object"]}`,
			want:  `{"metadata":["not","an","object"]}`,
		},
		{
			name:  "non-object value is unchanged",
			value: `"not-an-object"`,
			want:  `"not-an-object"`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := StripManagedMetadata(json.RawMessage(tc.value))
			if string(got) != tc.want {
				t.Fatalf("StripManagedMetadata() = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestHasReservedMetadataKey(t *testing.T) {
	cases := []struct {
		name    string
		meta    map[string]string
		wantErr bool
	}{
		{
			name:    "reserved prefix rejected",
			meta:    map[string]string{"agentgateway.dev/id": "x"},
			wantErr: true,
		},
		{
			name: "ordinary key allowed",
			meta: map[string]string{"team": "platform"},
		},
		{
			name: "empty map allowed",
			meta: map[string]string{},
		},
		{
			name:    "any reserved prefix key rejected",
			meta:    map[string]string{"agentgateway.dev/custom": "v"},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := HasReservedMetadataKey(tc.meta)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
