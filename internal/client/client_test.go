package client_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/caberdo/terraform-provider-agentgateway/internal/agentgatewaytest"
	"github.com/caberdo/terraform-provider-agentgateway/internal/client"
	"github.com/caberdo/terraform-provider-agentgateway/internal/configresource"
)

// newFixedEnvelopeClient serves one fixed body and returns a client pointed at
// it, so envelope parsing is tested against the literal API JSON rather than
// the fake server's encoding.
func newFixedEnvelopeClient(t *testing.T, method, path, body string) client.Client {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != method || r.URL.Path != path {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	c, err := client.New(client.Config{BaseURL: srv.URL, HTTPClient: srv.Client()})
	if err != nil {
		t.Fatalf("client.New: %v", err)
	}
	return c
}

func assertEnvelopeTimestamps(t *testing.T, res configresource.Resource, wantCreated, wantUpdated time.Time) {
	t.Helper()

	if !res.CreatedAt.Equal(wantCreated) {
		t.Errorf("expected createdAt %s, got %s", wantCreated.Format(time.RFC3339), res.CreatedAt.Format(time.RFC3339))
	}
	if !res.UpdatedAt.Equal(wantUpdated) {
		t.Errorf("expected updatedAt %s, got %s", wantUpdated.Format(time.RFC3339), res.UpdatedAt.Format(time.RFC3339))
	}
	if got := res.CreatedAt.Format(time.RFC3339); got != wantCreated.Format(time.RFC3339) {
		t.Errorf("expected RFC3339 createdAt %s, got %s", wantCreated.Format(time.RFC3339), got)
	}
}

func TestEnvelopeParsesTimestamps(t *testing.T) {
	t.Parallel()

	created := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	updated := time.Date(2026, 1, 3, 4, 5, 6, 0, time.UTC)
	item := `{"kind":"llm.provider","id":"openai","value":{"name":"openai"},"revision":7,` +
		`"createdAt":"2026-01-02T03:04:05Z","updatedAt":"2026-01-03T04:05:06Z"}`
	ctx := context.Background()

	t.Run("list response", func(t *testing.T) {
		t.Parallel()

		c := newFixedEnvelopeClient(t, http.MethodGet, "/api/config/resources/llm.provider", "["+item+"]")
		list, err := c.List(ctx, configresource.KindLLMProvider)
		if err != nil {
			t.Fatalf("list failed: %v", err)
		}
		if len(list) != 1 {
			t.Fatalf("expected 1 resource, got %d", len(list))
		}
		assertEnvelopeTimestamps(t, list[0], created, updated)
	})

	t.Run("single-resource response", func(t *testing.T) {
		t.Parallel()

		c := newFixedEnvelopeClient(t, http.MethodPut, "/api/config/resources/llm.provider/openai", item)
		res, err := c.Update(ctx, configresource.KindLLMProvider, "openai", []byte(`{"name":"openai"}`))
		if err != nil {
			t.Fatalf("update failed: %v", err)
		}
		assertEnvelopeTimestamps(t, res, created, updated)
	})
}

func TestEnvelopeWithoutTimestampsIsZero(t *testing.T) {
	t.Parallel()

	item := `{"kind":"llm.provider","id":"openai","value":{"name":"openai"},"revision":1}`
	ctx := context.Background()

	t.Run("list response", func(t *testing.T) {
		t.Parallel()

		c := newFixedEnvelopeClient(t, http.MethodGet, "/api/config/resources/llm.provider", "["+item+"]")
		list, err := c.List(ctx, configresource.KindLLMProvider)
		if err != nil {
			t.Fatalf("list failed: %v", err)
		}
		if len(list) != 1 {
			t.Fatalf("expected 1 resource, got %d", len(list))
		}
		if !list[0].CreatedAt.IsZero() || !list[0].UpdatedAt.IsZero() {
			t.Errorf("expected zero timestamps, got createdAt=%s updatedAt=%s",
				list[0].CreatedAt, list[0].UpdatedAt)
		}
	})

	t.Run("single-resource response", func(t *testing.T) {
		t.Parallel()

		c := newFixedEnvelopeClient(t, http.MethodPut, "/api/config/resources/llm.provider/openai", item)
		res, err := c.Update(ctx, configresource.KindLLMProvider, "openai", []byte(`{"name":"openai"}`))
		if err != nil {
			t.Fatalf("update failed: %v", err)
		}
		if !res.CreatedAt.IsZero() || !res.UpdatedAt.IsZero() {
			t.Errorf("expected zero timestamps, got createdAt=%s updatedAt=%s", res.CreatedAt, res.UpdatedAt)
		}
	})
}

func TestCreateUsesCollectionPath(t *testing.T) {
	t.Parallel()

	srv, c := agentgatewaytest.New(t)
	ctx := context.Background()

	res, err := c.Create(ctx, configresource.KindLLMProvider, []byte(`{"name":"openai","provider":"openai"}`))
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if res.ID != "openai" {
		t.Fatalf("expected id openai, got %q", res.ID)
	}

	reqs := srv.Requests()
	if len(reqs) != 1 {
		t.Fatalf("expected 1 request, got %d", len(reqs))
	}
	if reqs[0].Method != http.MethodPut {
		t.Fatalf("expected PUT, got %s", reqs[0].Method)
	}
	if reqs[0].Path != "/api/config/resources/llm.provider" {
		t.Fatalf("expected collection path, got %s", reqs[0].Path)
	}
}

func TestUpdateUnknownIDIsConflict(t *testing.T) {
	t.Parallel()

	_, c := agentgatewaytest.New(t)
	ctx := context.Background()

	_, err := c.Update(ctx, configresource.KindLLMProvider, "missing",
		[]byte(`{"name":"missing","provider":"openai"}`))
	if err == nil {
		t.Fatal("expected error for unknown id")
	}
	if !errors.Is(err, client.ErrConflict) {
		t.Fatalf("expected ErrConflict, got %v", err)
	}
	var apiErr *client.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.Status != 409 {
		t.Fatalf("expected status 409, got %d", apiErr.Status)
	}
}

func TestReadOnlyReturnsErrReadOnly(t *testing.T) {
	t.Parallel()

	srv, c := agentgatewaytest.New(t)
	srv.SetReadOnly(true)
	ctx := context.Background()

	_, err := c.Create(ctx, configresource.KindLLMProvider, []byte(`{"name":"p","provider":"openai"}`))
	if !errors.Is(err, client.ErrReadOnly) {
		t.Fatalf("expected ErrReadOnly, got %v", err)
	}
}

func TestCollectionCreateDoesNotDeleteSiblings(t *testing.T) {
	t.Parallel()

	_, c := agentgatewaytest.New(t)
	ctx := context.Background()

	if _, err := c.Create(ctx, configresource.KindLLMProvider, []byte(`{"name":"a","provider":"openai"}`)); err != nil {
		t.Fatalf("create a failed: %v", err)
	}
	if _, err := c.Create(ctx, configresource.KindLLMProvider, []byte(`{"name":"b","provider":"openai"}`)); err != nil {
		t.Fatalf("create b failed: %v", err)
	}

	list, err := c.List(ctx, configresource.KindLLMProvider)
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 resources, got %d", len(list))
	}
}

func TestCreatePolicyIsInvalid(t *testing.T) {
	t.Parallel()

	_, c := agentgatewaytest.New(t)
	ctx := context.Background()

	_, err := c.Create(ctx, configresource.KindLLMPolicy, []byte(`{"ttl":3600}`))
	if !errors.Is(err, client.ErrInvalid) {
		t.Fatalf("expected ErrInvalid, got %v", err)
	}
}

func TestUpdatePolicyCreates(t *testing.T) {
	t.Parallel()

	_, c := agentgatewaytest.New(t)
	ctx := context.Background()

	res, err := c.Update(ctx, configresource.KindLLMPolicy, "apiKey", []byte(`{}`))
	if err != nil {
		t.Fatalf("update policy failed: %v", err)
	}
	if res.ID != "apiKey" {
		t.Fatalf("expected id apiKey, got %q", res.ID)
	}

	list, err := c.List(ctx, configresource.KindLLMPolicy)
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 policy, got %d", len(list))
	}
}

func TestDeleteMissingIsNotFound(t *testing.T) {
	t.Parallel()

	_, c := agentgatewaytest.New(t)
	ctx := context.Background()

	err := c.Delete(ctx, configresource.KindLLMProvider, "gone")
	if !errors.Is(err, client.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestAPIKeyCreateInjectsMetadata(t *testing.T) {
	t.Parallel()

	_, c := agentgatewaytest.New(t)
	ctx := context.Background()

	res, err := c.Create(ctx, configresource.KindLLMAPIKey, []byte(`{"key":"secret"}`))
	if err != nil {
		t.Fatalf("create api key failed: %v", err)
	}
	if res.ID == "" {
		t.Fatal("expected server-generated id")
	}
	if res.Revision != 1 {
		t.Fatalf("expected revision 1, got %d", res.Revision)
	}
}

func TestRuntimeParsesCamelCase(t *testing.T) {
	t.Parallel()

	srv, c := agentgatewaytest.New(t)
	srv.SetStoreMode("hybrid")
	ctx := context.Background()

	rt, err := c.Runtime(ctx)
	if err != nil {
		t.Fatalf("runtime failed: %v", err)
	}
	if rt.Version != "v1.7.0" {
		t.Fatalf("expected version v1.7.0, got %q", rt.Version)
	}
	if rt.ConfigStoreMode != "hybrid" {
		t.Fatalf("expected hybrid store mode, got %q", rt.ConfigStoreMode)
	}
	if !rt.ConfigReloadOK {
		t.Fatal("expected config reload ok")
	}
}

func TestNewValidatesConfig(t *testing.T) {
	t.Parallel()

	if _, err := client.New(client.Config{}); err == nil {
		t.Fatal("expected error for empty config")
	}
	if _, err := client.New(client.Config{BaseURL: "http://localhost:15000"}); err == nil {
		t.Fatal("expected error for nil HTTPClient")
	}
	if _, err := client.New(client.Config{HTTPClient: http.DefaultClient}); err == nil {
		t.Fatal("expected error for empty BaseURL")
	}
	if _, err := client.New(client.Config{BaseURL: "://invalid", HTTPClient: http.DefaultClient}); err == nil {
		t.Fatal("expected error for invalid BaseURL")
	}
}
