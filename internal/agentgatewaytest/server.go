package agentgatewaytest

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/caberdo/terraform-provider-agentgateway/internal/configresource"
)

// handler implements the agentgateway config-resource HTTP protocol.
type handler struct {
	store *Store
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.recordRequest(r)

	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")

	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/api/runtime":
		h.serveRuntime(w)
		return
	case r.Method == http.MethodGet && r.URL.Path == "/api/config/effective":
		h.serveEffectiveConfig(w)
		return
	case len(parts) == 4 && parts[0] == "api" && parts[1] == "config" && parts[2] == "resources":
		kind := configresource.Kind(parts[3])
		switch r.Method {
		case http.MethodGet:
			h.serveList(w, kind)
		case http.MethodPut:
			h.serveCreate(w, r, kind)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
		return
	case len(parts) == 5 && parts[0] == "api" && parts[1] == "config" && parts[2] == "resources":
		kind := configresource.Kind(parts[3])
		id := parts[4]
		switch r.Method {
		case http.MethodPut:
			h.serveUpdate(w, r, kind, id)
		case http.MethodDelete:
			h.serveDelete(w, kind, id)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}

	http.Error(w, "not found", http.StatusNotFound)
}

func (h *handler) recordRequest(r *http.Request) {
	var body json.RawMessage
	if r.Body != nil {
		b, err := io.ReadAll(r.Body)
		if err == nil {
			body = b
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
	}
	h.store.record(RecordedRequest{Method: r.Method, Path: r.URL.Path, Body: body})
}

func (h *handler) serveRuntime(w http.ResponseWriter) {
	h.writeJSON(w, http.StatusOK, h.store.Runtime())
}

func (h *handler) serveEffectiveConfig(w http.ResponseWriter) {
	body, status, msg := h.store.EffectiveConfig()
	if status != 0 {
		h.writeError(w, status, msg)
		return
	}
	h.writeRawJSON(w, http.StatusOK, body)
}

func (h *handler) serveList(w http.ResponseWriter, kind configresource.Kind) {
	list, status, msg := h.store.List(kind)
	if status != 0 {
		h.writeError(w, status, msg)
		return
	}
	h.writeJSON(w, http.StatusOK, resourcesResponse{Resources: list})
}

func (h *handler) serveCreate(w http.ResponseWriter, r *http.Request, kind configresource.Kind) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// The collection PUT takes {"resources": [{"value": ...}]} and answers
	// with the resources it wrote. The item id is derived from the value, so
	// the request does not carry one.
	var request resourcesRequest
	if err := json.Unmarshal(body, &request); err != nil {
		h.writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	written := make([]configresource.Resource, 0, len(request.Resources))
	for _, item := range request.Resources {
		if len(item.Value) == 0 {
			h.writeError(w, http.StatusUnprocessableEntity, "missing field `value`")
			return
		}
		res, status, msg := h.store.Create(kind, item.Value)
		if status != 0 {
			h.writeError(w, status, msg)
			return
		}
		written = append(written, res)
	}
	h.writeJSON(w, http.StatusOK, resourcesResponse{Resources: written})
}

func (h *handler) serveUpdate(w http.ResponseWriter, r *http.Request, kind configresource.Kind, id string) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// The single-resource PUT takes {"value": ...}; the id comes from the path.
	var request valueRequest
	if err := json.Unmarshal(body, &request); err != nil {
		h.writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	if len(request.Value) == 0 {
		h.writeError(w, http.StatusUnprocessableEntity, "missing field `value`")
		return
	}

	res, status, msg := h.store.Update(kind, id, request.Value)
	if status != 0 {
		h.writeError(w, status, msg)
		return
	}
	h.writeJSON(w, http.StatusOK, resourcesResponse{Resources: []configresource.Resource{res}})
}

func (h *handler) serveDelete(w http.ResponseWriter, kind configresource.Kind, id string) {
	status, msg := h.store.Delete(kind, id)
	if status != 0 {
		h.writeError(w, status, msg)
		return
	}
	h.writeJSON(w, http.StatusOK, deleteResponse{Status: "success", Message: "Configuration resource deleted successfully"})
}

// resourcesResponse is the envelope of every config-resource answer, matching
// the real gateway.
type resourcesResponse struct {
	Resources []configresource.Resource `json:"resources"`
}

// resourcesRequest is the body of a collection PUT.
type resourcesRequest struct {
	Resources []resourceRequest `json:"resources"`
}

// resourceRequest is one item of a collection PUT.
type resourceRequest struct {
	Value json.RawMessage `json:"value"`
}

// valueRequest is the body of a single-resource PUT.
type valueRequest struct {
	Value json.RawMessage `json:"value"`
}

// deleteResponse is the body of a successful DELETE.
type deleteResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

func (h *handler) writeJSON(w http.ResponseWriter, status int, v any) {
	// Config-resource responses are configresource.Resource values, whose JSON
	// tags are the API envelope, so the fake's wire format stays byte-compatible
	// with the real gateway (camelCase keys, RFC3339 timestamps).
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (h *handler) writeRawJSON(w http.ResponseWriter, status int, body json.RawMessage) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if len(body) > 0 {
		_, _ = w.Write(body)
	}
}

func (h *handler) writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(msg)
}
