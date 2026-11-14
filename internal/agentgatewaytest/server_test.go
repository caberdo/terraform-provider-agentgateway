package agentgatewaytest

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/caberdo/terraform-provider-agentgateway/internal/configresource"
)

// TestServerEnvelopeUsesCamelCaseKeys pins the fake's raw wire format to the
// real API envelope. Decoding into a map keeps the key spelling exact, so a
// PascalCase regression (encoding configresource.Resource without tags) fails
// here instead of silently passing through the client's case-insensitive
// decoder.
func TestServerEnvelopeUsesCamelCaseKeys(t *testing.T) {
	t.Parallel()

	srv, c := New(t)
	ctx := context.Background()

	if _, err := c.Create(ctx, configresource.KindLLMProvider, []byte(`{"name":"openai","provider":"openai"}`)); err != nil {
		t.Fatalf("create failed: %v", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/api/config/resources/llm.provider", nil)
	if err != nil {
		t.Fatalf("building the list request: %v", err)
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("list request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var envelope struct {
		Resources []map[string]json.RawMessage `json:"resources"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decoding the list response: %v", err)
	}
	if len(envelope.Resources) != 1 {
		t.Fatalf("expected 1 resource, got %d", len(envelope.Resources))
	}
	item := envelope.Resources[0]

	for _, key := range []string{"kind", "id", "value", "revision", "createdAt", "updatedAt"} {
		if _, ok := item[key]; !ok {
			t.Errorf("missing camelCase envelope key %q in %v", key, item)
		}
	}
	for _, key := range []string{"Kind", "ID", "Value", "Revision", "CreatedAt", "UpdatedAt"} {
		if _, ok := item[key]; ok {
			t.Errorf("unexpected PascalCase envelope key %q in %v", key, item)
		}
	}
}
