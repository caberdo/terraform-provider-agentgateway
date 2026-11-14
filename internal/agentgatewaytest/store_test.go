package agentgatewaytest

import (
	"encoding/json"
	"testing"

	"github.com/caberdo/terraform-provider-agentgateway/internal/configresource"
)

func TestStoreCreateUpsertsByID(t *testing.T) {
	t.Parallel()

	s := NewStore()
	res, status, msg := s.Create(configresource.KindLLMProvider, []byte(`{"name":"p","provider":"openai"}`))
	if status != 0 {
		t.Fatalf("unexpected error %d: %s", status, msg)
	}
	if res.ID != "p" {
		t.Fatalf("expected id p, got %q", res.ID)
	}

	res2, status, msg := s.Create(configresource.KindLLMProvider, []byte(`{"name":"p","provider":"anthropic"}`))
	if status != 0 {
		t.Fatalf("unexpected error %d: %s", status, msg)
	}
	if res2.Revision != 1 {
		t.Fatalf("expected revision 1 on upsert, got %d", res2.Revision)
	}
}

func TestStoreCreateDoesNotDeleteSiblings(t *testing.T) {
	t.Parallel()

	s := NewStore()
	if _, status, msg := s.Create(configresource.KindLLMProvider, []byte(`{"name":"a","provider":"openai"}`)); status != 0 {
		t.Fatalf("create a failed %d: %s", status, msg)
	}
	if _, status, msg := s.Create(configresource.KindLLMProvider, []byte(`{"name":"b","provider":"openai"}`)); status != 0 {
		t.Fatalf("create b failed %d: %s", status, msg)
	}

	list, status, msg := s.List(configresource.KindLLMProvider)
	if status != 0 {
		t.Fatalf("list failed %d: %s", status, msg)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 resources, got %d", len(list))
	}
}

func TestStoreUpdateUnknownNonPolicyIsConflict(t *testing.T) {
	t.Parallel()

	s := NewStore()
	_, status, msg := s.Update(configresource.KindLLMProvider, "missing", []byte(`{"name":"missing"}`))
	if status != 409 {
		t.Fatalf("expected 409, got %d", status)
	}
	want := "file-owned config resource cannot be updated in hybrid mode: llm.provider/missing"
	if msg != want {
		t.Fatalf("expected %q, got %q", want, msg)
	}
}

func TestStoreUpdatePolicyCreates(t *testing.T) {
	t.Parallel()

	s := NewStore()
	res, status, msg := s.Update(configresource.KindLLMPolicy, "apiKey", []byte(`{}`))
	if status != 0 {
		t.Fatalf("update failed %d: %s", status, msg)
	}
	if res.ID != "apiKey" {
		t.Fatalf("expected id apiKey, got %q", res.ID)
	}
}

func TestStoreDeleteMissingIsNotFound(t *testing.T) {
	t.Parallel()

	s := NewStore()
	status, msg := s.Delete(configresource.KindLLMProvider, "gone")
	if status != 404 {
		t.Fatalf("expected 404, got %d", status)
	}
	want := "config resource not found: llm.provider/gone"
	if msg != want {
		t.Fatalf("expected %q, got %q", want, msg)
	}
}

func TestStoreReadOnlyBlocksWrites(t *testing.T) {
	t.Parallel()

	s := NewStore()
	s.SetReadOnly(true)

	_, status, msg := s.Create(configresource.KindLLMProvider, []byte(`{"name":"p"}`))
	if status != 403 {
		t.Fatalf("expected 403, got %d", status)
	}
	if msg != "UI is configured as read-only" {
		t.Fatalf("expected read-only message, got %q", msg)
	}
}

func TestStoreFailNextWrites(t *testing.T) {
	t.Parallel()

	s := NewStore()
	s.FailNextWrites(2, 503)

	_, status, _ := s.Create(configresource.KindLLMProvider, []byte(`{"name":"a"}`))
	if status != 503 {
		t.Fatalf("expected first 503, got %d", status)
	}
	_, status, _ = s.Create(configresource.KindLLMProvider, []byte(`{"name":"b"}`))
	if status != 503 {
		t.Fatalf("expected second 503, got %d", status)
	}
	_, status, msg := s.Create(configresource.KindLLMProvider, []byte(`{"name":"c"}`))
	if status != 0 {
		t.Fatalf("expected success after failures, got %d: %s", status, msg)
	}
}

func TestStoreAPIKeyMetadataInjection(t *testing.T) {
	t.Parallel()

	s := NewStore()
	res, status, msg := s.Create(configresource.KindLLMAPIKey, []byte(`{"key":"secret"}`))
	if status != 0 {
		t.Fatalf("create failed %d: %s", status, msg)
	}
	if res.ID == "" {
		t.Fatal("expected generated id")
	}

	var body map[string]json.RawMessage
	if err := json.Unmarshal(res.Value, &body); err != nil {
		t.Fatalf("decode value: %v", err)
	}
	var meta map[string]json.RawMessage
	if err := json.Unmarshal(body["metadata"], &meta); err != nil {
		t.Fatalf("decode metadata: %v", err)
	}
	if _, ok := meta["agentgateway.dev/id"]; !ok {
		t.Fatal("missing agentgateway.dev/id metadata")
	}
	if _, ok := meta["agentgateway.dev/createdAt"]; !ok {
		t.Fatal("missing agentgateway.dev/createdAt metadata")
	}
}

func TestStoreRuntimeReflectsStoreMode(t *testing.T) {
	t.Parallel()

	s := NewStore()
	s.SetStoreMode("file")
	rt := s.Runtime()
	if rt.UI.ConfigStoreMode != "file" {
		t.Fatalf("expected file mode, got %q", rt.UI.ConfigStoreMode)
	}
}
