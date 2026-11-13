package trafficgateway

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/caberdo/terraform-provider-agentgateway/internal/configresource"
	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// ListenerModel is one named listener under a traffic gateway.
type ListenerModel struct {
	Name     types.String         `tfsdk:"name"`
	Hostname types.String         `tfsdk:"hostname"`
	Protocol types.String         `tfsdk:"protocol"`
	TLS      jsontypes.Normalized `tfsdk:"tls_json"`
}

// model is the Terraform representation of a traffic.gateway resource.
//
// Only exported fields with a tfsdk tag survive the framework's reflection
// based Plan.Get and State.Get calls (see resourceutil.KindResource).
type model struct {
	StoredID    types.String `tfsdk:"id"`
	Revision    types.Int64  `tfsdk:"revision"`
	Name        types.String `tfsdk:"name"`
	Port        types.Int64  `tfsdk:"port"`
	BindAddress types.String `tfsdk:"bind_address"`
	Protocol    types.String `tfsdk:"protocol"`

	TLS             jsontypes.Normalized `tfsdk:"tls_json"`
	Listeners       []ListenerModel      `tfsdk:"listeners"`
	OIDC            jsontypes.Normalized `tfsdk:"oidc_json"`
	JWTAuth         jsontypes.Normalized `tfsdk:"jwt_auth_json"`
	Authorization   jsontypes.Normalized `tfsdk:"authorization_json"`
	ExtAuthz        jsontypes.Normalized `tfsdk:"ext_authz_json"`
	ExtProc         jsontypes.Normalized `tfsdk:"ext_proc_json"`
	CORS            jsontypes.Normalized `tfsdk:"cors_json"`
	Transformations jsontypes.Normalized `tfsdk:"transformations_json"`
	BasicAuth       jsontypes.Normalized `tfsdk:"basic_auth_json"`
	APIKey          jsontypes.Normalized `tfsdk:"api_key_json"`
}

// gatewayPayload is the value object sent to and read from the config
// resource API. Field order matches the documented gateway shape.
type gatewayPayload struct {
	Name            string            `json:"name"`
	Port            *int64            `json:"port,omitempty"`
	BindAddress     *string           `json:"bindAddress,omitempty"`
	Protocol        *string           `json:"protocol,omitempty"`
	TLS             json.RawMessage   `json:"tls,omitempty"`
	Listeners       []listenerPayload `json:"listeners,omitempty"`
	OIDC            json.RawMessage   `json:"oidc,omitempty"`
	JWTAuth         json.RawMessage   `json:"jwtAuth,omitempty"`
	Authorization   json.RawMessage   `json:"authorization,omitempty"`
	ExtAuthz        json.RawMessage   `json:"extAuthz,omitempty"`
	ExtProc         json.RawMessage   `json:"extProc,omitempty"`
	CORS            json.RawMessage   `json:"cors,omitempty"`
	Transformations json.RawMessage   `json:"transformations,omitempty"`
	BasicAuth       json.RawMessage   `json:"basicAuth,omitempty"`
	APIKey          json.RawMessage   `json:"apiKey,omitempty"`
}

// listenerPayload is the value object for one listener.
type listenerPayload struct {
	Name     *string         `json:"name,omitempty"`
	Hostname *string         `json:"hostname,omitempty"`
	Protocol *string         `json:"protocol,omitempty"`
	TLS      json.RawMessage `json:"tls,omitempty"`
}

// gatewayValue is the wire representation decoded from the API.
type gatewayValue struct {
	Name            *string          `json:"name"`
	Port            *int64           `json:"port"`
	BindAddress     *string          `json:"bindAddress"`
	Protocol        *string          `json:"protocol"`
	TLS             json.RawMessage  `json:"tls"`
	Listeners       *[]listenerValue `json:"listeners"`
	OIDC            json.RawMessage  `json:"oidc"`
	JWTAuth         json.RawMessage  `json:"jwtAuth"`
	Authorization   json.RawMessage  `json:"authorization"`
	ExtAuthz        json.RawMessage  `json:"extAuthz"`
	ExtProc         json.RawMessage  `json:"extProc"`
	CORS            json.RawMessage  `json:"cors"`
	Transformations json.RawMessage  `json:"transformations"`
	BasicAuth       json.RawMessage  `json:"basicAuth"`
	APIKey          json.RawMessage  `json:"apiKey"`
}

// listenerValue is the wire representation of one listener.
type listenerValue struct {
	Name     *string         `json:"name"`
	Hostname *string         `json:"hostname"`
	Protocol *string         `json:"protocol"`
	TLS      json.RawMessage `json:"tls"`
}

// Kind returns the config resource kind.
func (m *model) Kind() configresource.Kind { return configresource.KindTrafficGateway }

// ID returns the server-side id, which is the gateway name (spec §7.0).
func (m *model) ID() string { return m.Name.ValueString() }

// SetServerMeta stores the computed id and revision returned by the server.
func (m *model) SetServerMeta(id string, revision int64) {
	m.StoredID = types.StringValue(id)
	m.Revision = types.Int64Value(revision)
}

// Value marshals the model to the traffic.gateway value object. It enforces
// C17: a gateway with listeners may only set port and bind_address at the top
// level.
func (m *model) Value(_ context.Context, diags *diag.Diagnostics) json.RawMessage {
	payload := gatewayPayload{
		Name:            m.Name.ValueString(),
		Port:            int64Pointer(m.Port),
		BindAddress:     stringPointer(m.BindAddress),
		Protocol:        stringPointer(m.Protocol),
		TLS:             rawJSON(m.TLS),
		OIDC:            rawJSON(m.OIDC),
		JWTAuth:         rawJSON(m.JWTAuth),
		Authorization:   rawJSON(m.Authorization),
		ExtAuthz:        rawJSON(m.ExtAuthz),
		ExtProc:         rawJSON(m.ExtProc),
		CORS:            rawJSON(m.CORS),
		Transformations: rawJSON(m.Transformations),
		BasicAuth:       rawJSON(m.BasicAuth),
		APIKey:          rawJSON(m.APIKey),
	}

	if len(m.Listeners) > 0 {
		payload.Listeners = make([]listenerPayload, 0, len(m.Listeners))
		for _, listener := range m.Listeners {
			payload.Listeners = append(payload.Listeners, listenerPayload{
				Name:     stringPointer(listener.Name),
				Hostname: stringPointer(listener.Hostname),
				Protocol: stringPointer(listener.Protocol),
				TLS:      rawJSON(listener.TLS),
			})
		}
		m.checkListenerConflicts(diags)
		if diags.HasError() {
			return nil
		}
	}

	value, err := json.Marshal(payload)
	if err != nil {
		diags.AddError(
			"Invalid traffic gateway value",
			"the traffic.gateway value could not be encoded; check the JSON attributes",
		)
		return nil
	}
	return value
}

// checkListenerConflicts adds a diagnostic for every top-level attribute that
// is not allowed together with listeners (C17).
func (m *model) checkListenerConflicts(diags *diag.Diagnostics) {
	conflicts := []struct {
		name   string
		set    bool
		detail string
	}{
		{name: "protocol", set: isSet(m.Protocol), detail: "set protocol on each listener instead"},
		{name: "tls_json", set: isSet(m.TLS), detail: "set tls_json on each listener instead"},
		{name: "oidc_json", set: isSet(m.OIDC), detail: "gateway-level policies cannot be combined with listeners"},
		{name: "jwt_auth_json", set: isSet(m.JWTAuth), detail: "gateway-level policies cannot be combined with listeners"},
		{name: "authorization_json", set: isSet(m.Authorization), detail: "gateway-level policies cannot be combined with listeners"},
		{name: "ext_authz_json", set: isSet(m.ExtAuthz), detail: "gateway-level policies cannot be combined with listeners"},
		{name: "ext_proc_json", set: isSet(m.ExtProc), detail: "gateway-level policies cannot be combined with listeners"},
		{name: "cors_json", set: isSet(m.CORS), detail: "gateway-level policies cannot be combined with listeners"},
		{name: "transformations_json", set: isSet(m.Transformations), detail: "gateway-level policies cannot be combined with listeners"},
		{name: "basic_auth_json", set: isSet(m.BasicAuth), detail: "gateway-level policies cannot be combined with listeners"},
		{name: "api_key_json", set: isSet(m.APIKey), detail: "gateway-level policies cannot be combined with listeners"},
	}

	for _, conflict := range conflicts {
		if !conflict.set {
			continue
		}
		diags.AddAttributeError(
			path.Root(conflict.name),
			"Invalid traffic gateway configuration",
			conflict.name+" cannot be set when listeners is set; a gateway with listeners may only set port and bind_address at the top level. "+conflict.detail+".",
		)
	}
}

// SetValue unmarshals an API value into the model. Attributes the server did
// not return keep their current value instead of being zeroed.
func (m *model) SetValue(_ context.Context, value json.RawMessage, diags *diag.Diagnostics) {
	var decoded gatewayValue
	if err := json.Unmarshal(value, &decoded); err != nil {
		diags.AddError(
			"Failed to decode traffic.gateway value",
			"the gateway returned a value that is not a valid traffic.gateway object",
		)
		return
	}

	if decoded.Name != nil {
		m.Name = types.StringValue(*decoded.Name)
	}
	if decoded.Port != nil {
		m.Port = types.Int64Value(*decoded.Port)
	}
	if decoded.BindAddress != nil {
		m.BindAddress = types.StringValue(*decoded.BindAddress)
	}
	if decoded.Protocol != nil {
		m.Protocol = types.StringValue(*decoded.Protocol)
	}

	m.TLS = preserveJSON(m.TLS, decoded.TLS)
	m.OIDC = preserveJSON(m.OIDC, decoded.OIDC)
	m.JWTAuth = preserveJSON(m.JWTAuth, decoded.JWTAuth)
	m.Authorization = preserveJSON(m.Authorization, decoded.Authorization)
	m.ExtAuthz = preserveJSON(m.ExtAuthz, decoded.ExtAuthz)
	m.ExtProc = preserveJSON(m.ExtProc, decoded.ExtProc)
	m.CORS = preserveJSON(m.CORS, decoded.CORS)
	m.Transformations = preserveJSON(m.Transformations, decoded.Transformations)
	m.BasicAuth = preserveJSON(m.BasicAuth, decoded.BasicAuth)
	m.APIKey = preserveJSON(m.APIKey, decoded.APIKey)

	if decoded.Listeners != nil {
		listeners := make([]ListenerModel, 0, len(*decoded.Listeners))
		for _, decodedListener := range *decoded.Listeners {
			listener := ListenerModel{}
			if decodedListener.Name != nil {
				listener.Name = types.StringValue(*decodedListener.Name)
			}
			if decodedListener.Hostname != nil {
				listener.Hostname = types.StringValue(*decodedListener.Hostname)
			}
			if decodedListener.Protocol != nil {
				listener.Protocol = types.StringValue(*decodedListener.Protocol)
			}
			if tls, ok := normalizedFromRaw(decodedListener.TLS); ok {
				listener.TLS = tls
			}
			listeners = append(listeners, listener)
		}
		m.Listeners = listeners
	}
}

// preserveJSON returns the decoded value when the server returned it and the
// current value otherwise, so an omitted attribute stays null (never zeroed).
func preserveJSON(current jsontypes.Normalized, raw json.RawMessage) jsontypes.Normalized {
	if value, ok := normalizedFromRaw(raw); ok {
		return value
	}
	return current
}

// normalizedFromRaw converts a raw JSON leaf to a normalized value. Absent and
// explicit null leaves are reported as not set.
func normalizedFromRaw(raw json.RawMessage) (jsontypes.Normalized, bool) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return jsontypes.Normalized{}, false
	}
	return jsontypes.NewNormalizedValue(string(trimmed)), true
}

// rawJSON returns the raw JSON for a normalized value, or nil when unset.
func rawJSON(value jsontypes.Normalized) json.RawMessage {
	if !isSet(value) {
		return nil
	}
	return json.RawMessage(value.ValueString())
}

// isSet reports whether a value is known and not null.
func isSet(value interface {
	IsNull() bool
	IsUnknown() bool
}) bool {
	return !value.IsNull() && !value.IsUnknown()
}

// int64Pointer converts an optional Terraform integer to a payload pointer.
func int64Pointer(value types.Int64) *int64 {
	if !isSet(value) {
		return nil
	}
	converted := value.ValueInt64()
	return &converted
}

// stringPointer converts an optional Terraform string to a payload pointer.
func stringPointer(value types.String) *string {
	if !isSet(value) {
		return nil
	}
	converted := value.ValueString()
	return &converted
}
