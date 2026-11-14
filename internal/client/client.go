package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/caberdo/terraform-provider-agentgateway/internal/configresource"
)

// Config holds the information needed to build a Client.
type Config struct {
	BaseURL    string            // e.g. "http://127.0.0.1:15000"; no trailing slash
	HTTPClient *http.Client      // required; the provider owns TLS and auth headers
	Headers    map[string]string // added to every request
	Timeout    time.Duration     // per-request deadline; 0 means 30s
	UserAgent  string            // e.g. "terraform-provider-agentgateway/0.1.0"
}

// Client is the HTTP client for the agentgateway config-resource API.
type Client interface {
	Runtime(ctx context.Context) (RuntimeInfo, error)
	List(ctx context.Context, kind configresource.Kind) ([]configresource.Resource, error)
	Create(ctx context.Context, kind configresource.Kind, value json.RawMessage) (configresource.Resource, error)
	Update(ctx context.Context, kind configresource.Kind, id string, value json.RawMessage) (configresource.Resource, error)
	Delete(ctx context.Context, kind configresource.Kind, id string) error
	EffectiveConfig(ctx context.Context) (json.RawMessage, error)
}

// httpClient is the concrete implementation of Client.
type httpClient struct {
	baseURL    string
	httpClient *http.Client
	headers    map[string]string
	timeout    time.Duration
	userAgent  string
}

// New validates cfg and returns a Client.
func New(cfg Config) (Client, error) {
	if cfg.HTTPClient == nil {
		return nil, errors.New("HTTPClient is required")
	}
	if cfg.BaseURL == "" {
		return nil, errors.New("BaseURL is required")
	}
	u, err := url.Parse(cfg.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("BaseURL is invalid: %w", err)
	}
	if u.Scheme == "" || u.Host == "" {
		return nil, errors.New("BaseURL must include scheme and host")
	}

	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	return &httpClient{
		baseURL:    strings.TrimRight(cfg.BaseURL, "/"),
		httpClient: cfg.HTTPClient,
		headers:    copyStringMap(cfg.Headers),
		timeout:    timeout,
		userAgent:  cfg.UserAgent,
	}, nil
}

func copyStringMap(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// Runtime fetches /api/runtime.
func (c *httpClient) Runtime(ctx context.Context) (RuntimeInfo, error) {
	status, body, err := c.call(ctx, http.MethodGet, "/api/runtime", nil)
	if err != nil {
		return RuntimeInfo{}, err
	}

	var raw runtimeResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return RuntimeInfo{}, &APIError{Status: status, Message: err.Error(), Kind: ErrServer}
	}

	return RuntimeInfo{
		Version:         raw.Build.Version,
		GitRevision:     raw.Build.GitRevision,
		GatewayMode:     raw.UI.GatewayMode,
		ConfigStoreMode: raw.UI.ConfigStoreMode,
		ConfigReloadOK:  raw.ConfigReload.Synchronized,
		ConfigReloadErr: raw.ConfigReload.LastError,
	}, nil
}

// List fetches all resources of a kind.
func (c *httpClient) List(ctx context.Context, kind configresource.Kind) ([]configresource.Resource, error) {
	path := fmt.Sprintf("/api/config/resources/%s", url.PathEscape(string(kind)))
	status, body, err := c.call(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}

	var raw []resourceEnvelope
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, &APIError{Status: status, Message: err.Error(), Kind: ErrServer}
	}
	if raw == nil {
		return nil, nil
	}
	list := make([]configresource.Resource, len(raw))
	for i, envelope := range raw {
		list[i] = envelope.resource()
	}
	return list, nil
}

// Create upserts a resource on the collection path. Policy kinds are rejected
// because they must use the single-resource path.
func (c *httpClient) Create(ctx context.Context, kind configresource.Kind, value json.RawMessage) (configresource.Resource, error) {
	if kind.IsPolicy() {
		return configresource.Resource{}, &APIError{
			Status:  http.StatusBadRequest,
			Message: fmt.Sprintf("policy resources require an item ID: %s", kind),
			Kind:    ErrInvalid,
		}
	}

	path := fmt.Sprintf("/api/config/resources/%s", url.PathEscape(string(kind)))
	status, body, err := c.call(ctx, http.MethodPut, path, value)
	if err != nil {
		return configresource.Resource{}, err
	}

	var envelope resourceEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return configresource.Resource{}, &APIError{Status: status, Message: err.Error(), Kind: ErrServer}
	}
	return envelope.resource(), nil
}

// Update writes a resource to the single-resource path.
func (c *httpClient) Update(ctx context.Context, kind configresource.Kind, id string, value json.RawMessage) (configresource.Resource, error) {
	path := fmt.Sprintf("/api/config/resources/%s/%s", url.PathEscape(string(kind)), url.PathEscape(id))
	status, body, err := c.call(ctx, http.MethodPut, path, value)
	if err != nil {
		return configresource.Resource{}, err
	}

	var envelope resourceEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return configresource.Resource{}, &APIError{Status: status, Message: err.Error(), Kind: ErrServer}
	}
	return envelope.resource(), nil
}

// Delete removes a resource.
func (c *httpClient) Delete(ctx context.Context, kind configresource.Kind, id string) error {
	path := fmt.Sprintf("/api/config/resources/%s/%s", url.PathEscape(string(kind)), url.PathEscape(id))
	_, _, err := c.call(ctx, http.MethodDelete, path, nil)
	return err
}

// EffectiveConfig fetches /api/config/effective.
func (c *httpClient) EffectiveConfig(ctx context.Context) (json.RawMessage, error) {
	_, body, err := c.call(ctx, http.MethodGet, "/api/config/effective", nil)
	if err != nil {
		return nil, err
	}
	return body, nil
}

// call performs an HTTP request and retries transient failures.
func (c *httpClient) call(ctx context.Context, method, path string, body []byte) (int, []byte, error) {
	return withRetries(ctx, func() (int, []byte, error) {
		status, respBody, err := c.do(ctx, method, path, body)
		if err != nil {
			return status, respBody, err
		}
		return status, respBody, classifyError(status, respBody)
	})
}

// do performs a single HTTP request with a per-attempt timeout.
func (c *httpClient) do(ctx context.Context, method, path string, body []byte) (int, []byte, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	reqURL := c.baseURL + path
	var bodyReader io.Reader
	if len(body) > 0 {
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, reqURL, bodyReader)
	if err != nil {
		return 0, nil, &APIError{Status: 0, Message: err.Error(), Kind: ErrInvalid}
	}

	for k, v := range c.headers {
		req.Header.Set(k, v)
	}
	if c.userAgent != "" {
		req.Header.Set("User-Agent", c.userAgent)
	}
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, nil, &APIError{Status: 0, Message: err.Error(), Kind: ErrTransient}
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, &APIError{Status: resp.StatusCode, Message: err.Error(), Kind: ErrTransient}
	}
	return resp.StatusCode, respBody, nil
}

// classifyError maps HTTP statuses to sentinel errors.
func classifyError(status int, body []byte) error {
	if status >= 200 && status < 300 {
		return nil
	}

	msg := extractMessage(body)
	switch status {
	case http.StatusNotFound:
		return &APIError{Status: status, Message: msg, Kind: ErrNotFound}
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		return &APIError{Status: status, Message: msg, Kind: ErrInvalid}
	case http.StatusForbidden:
		if msg == "UI is configured as read-only" {
			return &APIError{Status: status, Message: msg, Kind: ErrReadOnly}
		}
		return &APIError{Status: status, Message: msg, Kind: ErrServer}
	case http.StatusConflict:
		return &APIError{Status: status, Message: msg, Kind: ErrConflict}
	case http.StatusTooManyRequests, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return &APIError{Status: status, Message: msg, Kind: ErrTransient}
	}
	if status >= 500 {
		return &APIError{Status: status, Message: msg, Kind: ErrServer}
	}
	return &APIError{Status: status, Message: msg, Kind: ErrServer}
}

// extractMessage unquotes a JSON string body, falling back to the raw bytes.
func extractMessage(body []byte) string {
	var s string
	if err := json.Unmarshal(body, &s); err == nil {
		return s
	}
	return string(body)
}

// resourceEnvelope is the camelCase JSON shape of a config resource as the API
// returns it (spec §5.2). Decoding is explicit so createdAt and updatedAt are
// parsed into time.Time; they stay zero when the server omits them.
type resourceEnvelope struct {
	Kind      configresource.Kind `json:"kind"`
	ID        string              `json:"id"`
	Value     json.RawMessage     `json:"value"`
	Revision  int64               `json:"revision"`
	CreatedAt time.Time           `json:"createdAt"`
	UpdatedAt time.Time           `json:"updatedAt"`
}

// resource converts the decoded envelope into a configresource.Resource.
func (e resourceEnvelope) resource() configresource.Resource {
	return configresource.Resource{
		Kind:      e.Kind,
		ID:        e.ID,
		Value:     e.Value,
		Revision:  e.Revision,
		CreatedAt: e.CreatedAt,
		UpdatedAt: e.UpdatedAt,
	}
}

// runtimeResponse is the camelCase JSON shape of /api/runtime.
type runtimeResponse struct {
	Build struct {
		Version     string `json:"version"`
		GitRevision string `json:"gitRevision"`
	} `json:"build"`
	UI struct {
		GatewayMode     string `json:"gatewayMode"`
		ConfigStoreMode string `json:"configStoreMode"`
	} `json:"ui"`
	ConfigReload struct {
		Synchronized bool   `json:"synchronized"`
		LastError    string `json:"lastError"`
	} `json:"configReload"`
}
