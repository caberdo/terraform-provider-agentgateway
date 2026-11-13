package agentgatewaytest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/caberdo/terraform-provider-agentgateway/internal/configresource"
)

// resourceKey indexes the in-memory config store.
type resourceKey struct {
	kind configresource.Kind
	id   string
}

// BuildInfo is the build block of /api/runtime.
type BuildInfo struct {
	Version     string `json:"version"`
	GitRevision string `json:"gitRevision"`
}

// UIInfo is the ui block of /api/runtime.
type UIInfo struct {
	GatewayMode     string `json:"gatewayMode"`
	ConfigStoreMode string `json:"configStoreMode"`
}

// ConfigReloadInfo is the configReload block of /api/runtime.
type ConfigReloadInfo struct {
	Synchronized bool   `json:"synchronized"`
	LastError    string `json:"lastError,omitempty"`
}

// RuntimeResponse is the JSON shape returned by GET /api/runtime.
type RuntimeResponse struct {
	Build        BuildInfo        `json:"build"`
	UI           UIInfo           `json:"ui"`
	ConfigReload ConfigReloadInfo `json:"configReload"`
}

// Store implements the agentgateway config-resource semantics in memory.
type Store struct {
	mu            sync.RWMutex
	resources     map[resourceKey]configresource.Resource
	readOnly      bool
	storeMode     string
	failNext      int
	failStatus    int
	apiKeyCounter int
	requests      []RecordedRequest
}

// NewStore returns an empty hybrid-mode store.
func NewStore() *Store {
	return &Store{
		resources: make(map[resourceKey]configresource.Resource),
		storeMode: "hybrid",
	}
}

// SetReadOnly makes every write return 403.
func (s *Store) SetReadOnly(ro bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.readOnly = ro
}

// SetStoreMode drives the value of ui.configStoreMode in /api/runtime.
func (s *Store) SetStoreMode(mode string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.storeMode = mode
}

// FailNextWrites causes the next n write operations to respond with status.
func (s *Store) FailNextWrites(n, status int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failNext = n
	s.failStatus = status
}

// Runtime returns the current /api/runtime payload.
func (s *Store) Runtime() RuntimeResponse {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return RuntimeResponse{
		Build: BuildInfo{Version: "v1.7.0", GitRevision: "fake"},
		UI: UIInfo{
			GatewayMode:     "standalone",
			ConfigStoreMode: s.storeMode,
		},
		ConfigReload: ConfigReloadInfo{Synchronized: true},
	}
}

// Create upserts a resource on the collection path and returns the stored
// resource. It never deletes siblings. Policies are rejected because they must
// use the single-resource path.
func (s *Store) Create(kind configresource.Kind, value json.RawMessage) (configresource.Resource, int, string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if status, fail := s.checkWriteLocked(); fail {
		return configresource.Resource{}, status, s.failMessage(status)
	}

	if kind.IsPolicy() {
		return configresource.Resource{}, 400,
			fmt.Sprintf("config resource %s requires an item ID", kind)
	}

	id, err := s.deriveIDLocked(kind, value)
	if err != nil {
		return configresource.Resource{}, 400, err.Error()
	}

	if kind == configresource.KindLLMAPIKey {
		value = injectAPIKeyMetadata(value, id, epoch)
	}

	res := configresource.Resource{
		Kind:      kind,
		ID:        id,
		Value:     value,
		Revision:  1,
		CreatedAt: epoch,
		UpdatedAt: epoch,
	}
	s.resources[resourceKey{kind, id}] = res
	return res, 0, ""
}

// Update writes to the single-resource path. Non-policy resources return 409
// when the id is unknown (hybrid update-only semantics). Policy kinds are
// created when absent.
func (s *Store) Update(kind configresource.Kind, id string, value json.RawMessage) (configresource.Resource, int, string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if status, fail := s.checkWriteLocked(); fail {
		return configresource.Resource{}, status, s.failMessage(status)
	}

	key := resourceKey{kind, id}

	if !kind.IsPolicy() {
		existing, ok := s.resources[key]
		if !ok {
			return configresource.Resource{}, 409,
				fmt.Sprintf("file-owned config resource cannot be updated in hybrid mode: %s/%s", kind, id)
		}
		existing.Value = value
		existing.Revision++
		existing.UpdatedAt = epoch
		s.resources[key] = existing
		return existing, 0, ""
	}

	res := configresource.Resource{
		Kind:      kind,
		ID:        id,
		Value:     value,
		Revision:  1,
		CreatedAt: epoch,
		UpdatedAt: epoch,
	}
	if existing, ok := s.resources[key]; ok {
		res.Revision = existing.Revision + 1
		res.CreatedAt = existing.CreatedAt
	}
	s.resources[key] = res
	return res, 0, ""
}

// Delete removes a resource. It returns 404 when absent.
func (s *Store) Delete(kind configresource.Kind, id string) (int, string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if status, fail := s.checkWriteLocked(); fail {
		return status, s.failMessage(status)
	}

	key := resourceKey{kind, id}
	if _, ok := s.resources[key]; !ok {
		return 404, fmt.Sprintf("config resource not found: %s/%s", kind, id)
	}
	delete(s.resources, key)
	return 0, ""
}

// List returns all resources of a kind sorted by id.
func (s *Store) List(kind configresource.Kind) ([]configresource.Resource, int, string) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]configresource.Resource, 0)
	for k, v := range s.resources {
		if k.kind == kind {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, 0, ""
}

// EffectiveConfig returns the merged effective configuration.
func (s *Store) EffectiveConfig() (json.RawMessage, int, string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return json.RawMessage(`{}`), 0, ""
}

func (s *Store) record(req RecordedRequest) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = append(s.requests, req)
}

// Requests returns a copy of every request received by the server.
func (s *Store) Requests() []RecordedRequest {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]RecordedRequest, len(s.requests))
	copy(out, s.requests)
	return out
}

func (s *Store) checkWriteLocked() (int, bool) {
	if s.readOnly {
		return 403, true
	}
	if s.failNext > 0 {
		s.failNext--
		return s.failStatus, true
	}
	return 0, false
}

func (s *Store) failMessage(status int) string {
	if status == 403 {
		return "UI is configured as read-only"
	}
	return http.StatusText(status)
}

func (s *Store) deriveIDLocked(kind configresource.Kind, value json.RawMessage) (string, error) {
	if kind == configresource.KindLLMAPIKey {
		s.apiKeyCounter++
		return fmt.Sprintf("00000000-0000-0000-0000-%012d", s.apiKeyCounter), nil
	}
	return configresource.DeriveID(kind, value)
}

var epoch = time.Unix(0, 0).UTC()

func injectAPIKeyMetadata(value json.RawMessage, id string, createdAt time.Time) json.RawMessage {
	var body map[string]json.RawMessage
	if err := json.Unmarshal(value, &body); err != nil {
		return value
	}

	metadata := map[string]json.RawMessage{}
	if raw, ok := body["metadata"]; ok {
		_ = json.Unmarshal(raw, &metadata)
	}

	metadata["agentgateway.dev/id"] = json.RawMessage(fmt.Sprintf("%q", id))
	metadata["agentgateway.dev/createdAt"] = json.RawMessage(fmt.Sprintf("%q", createdAt.Format(time.RFC3339)))

	metaJSON, err := json.Marshal(metadata)
	if err != nil {
		return value
	}
	body["metadata"] = metaJSON

	out, err := json.Marshal(body)
	if err != nil {
		return value
	}
	return out
}
