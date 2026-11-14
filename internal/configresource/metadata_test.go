package configresource

import (
	"bytes"
	"encoding/json"
	"testing"
)

func jsonEqual(t *testing.T, got, want json.RawMessage) bool {
	t.Helper()

	var gotV, wantV any
	if err := json.Unmarshal(got, &gotV); err != nil {
		t.Fatalf("got is not valid JSON: %v", err)
	}
	if err := json.Unmarshal(want, &wantV); err != nil {
		t.Fatalf("want is not valid JSON: %v", err)
	}

	gotNorm, err := json.Marshal(gotV)
	if err != nil {
		t.Fatalf("remarshal got: %v", err)
	}
	wantNorm, err := json.Marshal(wantV)
	if err != nil {
		t.Fatalf("remarshal want: %v", err)
	}
	return bytes.Equal(gotNorm, wantNorm)
}

func TestStripManagedMetadata(t *testing.T) {
	cases := []struct {
		name  string
		value string
		want  string
	}{
		{
			name:  "removes managed keys and keeps user field",
			value: `{"key":"secret","metadata":{"agentgateway.dev/id":"x","agentgateway.dev/createdAt":"t","agentgateway.dev/owner":"o","team":"platform"}}`,
			want:  `{"key":"secret","metadata":{"team":"platform"}}`,
		},
		{
			name:  "removes empty metadata object",
			value: `{"key":"k","metadata":{"agentgateway.dev/id":"x"}}`,
			want:  `{"key":"k"}`,
		},
		{
			name:  "leaves value without metadata unchanged",
			value: `{"key":"secret"}`,
			want:  `{"key":"secret"}`,
		},
		{
			name:  "leaves non-managed metadata semantically unchanged",
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
			if !jsonEqual(t, got, json.RawMessage(tc.want)) {
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
