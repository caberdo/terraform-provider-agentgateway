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
	h.writeJSON(w, http.StatusOK, list)
}

func (h *handler) serveCreate(w http.ResponseWriter, r *http.Request, kind configresource.Kind) {
	value, err := io.ReadAll(r.Body)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	res, status, msg := h.store.Create(kind, value)
	if status != 0 {
		h.writeError(w, status, msg)
		return
	}
	h.writeJSON(w, http.StatusOK, res)
}

func (h *handler) serveUpdate(w http.ResponseWriter, r *http.Request, kind configresource.Kind, id string) {
	value, err := io.ReadAll(r.Body)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	res, status, msg := h.store.Update(kind, id, value)
	if status != 0 {
		h.writeError(w, status, msg)
		return
	}
	h.writeJSON(w, http.StatusOK, res)
}

func (h *handler) serveDelete(w http.ResponseWriter, kind configresource.Kind, id string) {
	status, msg := h.store.Delete(kind, id)
	if status != 0 {
		h.writeError(w, status, msg)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *handler) writeJSON(w http.ResponseWriter, status int, v any) {
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
