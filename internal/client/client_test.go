package client_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/caberdo/terraform-provider-agentgateway/internal/agentgatewaytest"
	"github.com/caberdo/terraform-provider-agentgateway/internal/client"
	"github.com/caberdo/terraform-provider-agentgateway/internal/configresource"
)

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
