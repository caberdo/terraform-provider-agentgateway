package agentgatewaytest

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/caberdo/terraform-provider-agentgateway/internal/client"
)

// RecordedRequest captures one HTTP request received by the fake server.
type RecordedRequest struct {
	Method string
	Path   string
	Body   json.RawMessage
}

// Server is an httptest server that speaks the agentgateway config-resource
// protocol. It exposes the same operations as the real gateway for unit tests.
type Server struct {
	*httptest.Server
	store *Store
}

// New starts a fake server and returns it together with a client configured to
// talk to it.
func New(t *testing.T) (*Server, client.Client) {
	t.Helper()
	store := NewStore()
	httpSrv := httptest.NewServer(&handler{store: store})
	t.Cleanup(httpSrv.Close)

	cfg := client.Config{
		BaseURL:    httpSrv.URL,
		HTTPClient: httpSrv.Client(),
		UserAgent:  "terraform-provider-agentgateway/test",
	}
	c, err := client.New(cfg)
	if err != nil {
		t.Fatalf("creating client for fake server: %v", err)
	}
	return &Server{Server: httpSrv, store: store}, c
}

// SetReadOnly makes every subsequent write return 403.
func (s *Server) SetReadOnly(ro bool) {
	s.store.SetReadOnly(ro)
}

// SetStoreMode changes the value reported by Runtime().
func (s *Server) SetStoreMode(mode string) {
	s.store.SetStoreMode(mode)
}

// FailNextWrites causes the next n write operations to respond with status.
func (s *Server) FailNextWrites(n, status int) {
	s.store.FailNextWrites(n, status)
}

// Requests returns a copy of every HTTP request received so far.
func (s *Server) Requests() []RecordedRequest {
	return s.store.Requests()
}
