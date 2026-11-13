package configresource

import (
	"slices"
	"testing"
)

func TestEnumSlices(t *testing.T) {
	cases := []struct {
		name     string
		slice    []string
		wantLen  int
		sentinels []string
	}{
		{
			name:      "BuiltinProviders",
			slice:     BuiltinProviders,
			wantLen:   8,
			sentinels: []string{"openai", "openAI"},
		},
		{
			name:      "ProviderPresets",
			slice:     ProviderPresets,
			wantLen:   17,
			sentinels: []string{"cohere", "ollama", "nebius"},
		},
		{
			name:      "LLMPolicyKeys",
			slice:     LLMPolicyKeys,
			wantLen:   13,
			sentinels: []string{"extAuthz", "localRateLimit", "apiKey"},
		},
		{
			name:      "MCPPolicyKeys",
			slice:     MCPPolicyKeys,
			wantLen:   24,
			sentinels: []string{"mcpAuthentication", "backendTLS", "directResponse"},
		},
		{
			name:      "UIPolicyKeys",
			slice:     UIPolicyKeys,
			wantLen:   8,
			sentinels: []string{"oidc", "csrf", "apiKey"},
		},
		{
			name:      "FrontendPolicyKeys",
			slice:     FrontendPolicyKeys,
			wantLen:   10,
			sentinels: []string{"http", "networkAuthorization", "tracing"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if len(tc.slice) != tc.wantLen {
				t.Fatalf("len(%s) = %d, want %d", tc.name, len(tc.slice), tc.wantLen)
			}
			for _, s := range tc.sentinels {
				if !slices.Contains(tc.slice, s) {
					t.Fatalf("%s missing expected value %q", tc.name, s)
				}
			}
		})
	}
}
