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

func TestRetriesTransientThenSucceeds(t *testing.T) {
	t.Parallel()

	srv, c := agentgatewaytest.New(t)
	srv.FailNextWrites(2, http.StatusServiceUnavailable)
	ctx := context.Background()

	_, err := c.Create(ctx, configresource.KindLLMProvider, []byte(`{"name":"p","provider":"openai"}`))
	if err != nil {
		t.Fatalf("expected success after retries, got %v", err)
	}

	reqs := srv.Requests()
	if len(reqs) != 3 {
		t.Fatalf("expected 3 requests (2 retries + success), got %d", len(reqs))
	}
}

func TestDoesNotRetryInvalid(t *testing.T) {
	t.Parallel()

	srv, c := agentgatewaytest.New(t)
	srv.FailNextWrites(1, http.StatusBadRequest)
	ctx := context.Background()

	_, err := c.Create(ctx, configresource.KindLLMProvider, []byte(`{"name":"p","provider":"openai"}`))
	if !errors.Is(err, client.ErrInvalid) {
		t.Fatalf("expected ErrInvalid, got %v", err)
	}

	reqs := srv.Requests()
	if len(reqs) != 1 {
		t.Fatalf("expected 1 request, got %d", len(reqs))
	}
}
