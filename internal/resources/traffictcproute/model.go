package traffictcproute

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/caberdo/terraform-provider-agentgateway/internal/configresource"
	"github.com/caberdo/terraform-provider-agentgateway/internal/resourceutil"
	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// backendModel is one destination of a TCP route. Exactly one variant may be
// set.
type backendModel struct {
	Weight  types.Int64          `tfsdk:"weight"`
	Host    []hostModel          `tfsdk:"host"`
	Backend []backendRefModel    `tfsdk:"backend"`
	Service jsontypes.Normalized `tfsdk:"service_json"`
	MCP     jsontypes.Normalized `tfsdk:"mcp_json"`
	AI      jsontypes.Normalized `tfsdk:"ai_json"`
	JSON    jsontypes.Normalized `tfsdk:"json"`
}

// hostModel is a direct host backend.
type hostModel struct {
	Host types.String `tfsdk:"host"`
	Port types.Int64  `tfsdk:"port"`
}

// backendRefModel references another config resource.
type backendRefModel struct {
	Name      types.String `tfsdk:"name"`
	Namespace types.String `tfsdk:"namespace"`
}

// model is the Terraform representation of a traffic.tcpRoute resource.
//
// Only exported fields with tfsdk tags survive the framework's reflection based
// Plan.Get and State.Get calls (see resourceutil.KindResource).
type model struct {
	StoredID  types.String `tfsdk:"id"`
	Revision  types.Int64  `tfsdk:"revision"`
	Name      types.String `tfsdk:"name"`
	Gateways  types.List   `tfsdk:"gateways"`
	Namespace types.String `tfsdk:"namespace"`
	RuleName  types.String `tfsdk:"rule_name"`
	Hostnames types.List   `tfsdk:"hostnames"`

	Backends []backendModel       `tfsdk:"backends"`
	Policies jsontypes.Normalized `tfsdk:"policies_json"`
}

// newModel returns a model with every root list seeded with its element type.
// ImportState writes a fresh model straight into state, and a zero-value
// types.List carries no element type, which the framework rejects on the next
// Read.
func newModel() *model {
	return &model{
		Gateways:  types.ListNull(types.StringType),
		Hostnames: types.ListNull(types.StringType),
	}
}

// tcpRoutePayload is the value object sent to the config resource API. Field
// order matches the documented route shape.
type tcpRoutePayload struct {
	Name      string            `json:"name"`
	Gateways  *[]string         `json:"gateways,omitempty"`
	Namespace *string           `json:"namespace,omitempty"`
	RuleName  *string           `json:"ruleName,omitempty"`
	Hostnames *[]string         `json:"hostnames,omitempty"`
	Backends  []json.RawMessage `json:"backends,omitempty"`
	Policies  json.RawMessage   `json:"policies,omitempty"`
}

// backendPayload is the value object for a known backend variant. A generic
// json backend is merged into this object at the top level instead.
type backendPayload struct {
	Weight  *int64             `json:"weight,omitempty"`
	Host    *hostPayload       `json:"host,omitempty"`
	Backend *backendRefPayload `json:"backend,omitempty"`
	Service json.RawMessage    `json:"service,omitempty"`
	MCP     json.RawMessage    `json:"mcp,omitempty"`
	AI      json.RawMessage    `json:"ai,omitempty"`
}

// hostPayload is the value object for a direct host backend.
type hostPayload struct {
	Host string `json:"host"`
	Port int64  `json:"port"`
}

// backendRefPayload is the value object for a backend reference.
type backendRefPayload struct {
	Name      string  `json:"name"`
	Namespace *string `json:"namespace,omitempty"`
}

// tcpRouteValue is the wire representation decoded from the API.
type tcpRouteValue struct {
	Name      *string            `json:"name"`
	Gateways  *[]string          `json:"gateways"`
	Namespace *string            `json:"namespace"`
	RuleName  *string            `json:"ruleName"`
	Hostnames *[]string          `json:"hostnames"`
	Backends  *[]json.RawMessage `json:"backends"`
	Policies  json.RawMessage    `json:"policies"`
}

// backendValue is the known part of a backend. Unknown keys (aws, dynamic,
// internal, routeGroup) are collected into the generic json variant.
type backendValue struct {
	Weight  *int64           `json:"weight"`
	Host    *hostValue       `json:"host"`
	Backend *backendRefValue `json:"backend"`
}

// hostValue is the wire representation of a direct host backend.
type hostValue struct {
	Host *string `json:"host"`
	Port *int64  `json:"port"`
}

// backendRefValue is the wire representation of a backend reference.
type backendRefValue struct {
	Name      *string `json:"name"`
	Namespace *string `json:"namespace"`
}

// Kind returns the config resource kind.
func (m *model) Kind() configresource.Kind { return configresource.KindTrafficTCPRoute }

// ID returns the server-side id, which is the route name (spec §7.0).
func (m *model) ID() string { return m.Name.ValueString() }

// SetServerMeta stores the computed id and revision returned by the server.
func (m *model) SetServerMeta(id string, revision int64) {
	m.StoredID = types.StringValue(id)
	m.Revision = types.Int64Value(revision)
}

// Value marshals the model to the traffic.tcpRoute value object. The mutually
// exclusive backend variants are also enforced here, so a hand-built model
// cannot send an ambiguous payload.
func (m *model) Value(ctx context.Context, diags *diag.Diagnostics) json.RawMessage {
	payload := tcpRoutePayload{
		Name:      m.Name.ValueString(),
		Namespace: stringPointer(m.Namespace),
		RuleName:  stringPointer(m.RuleName),
		Policies:  rawJSON(m.Policies),
	}

	payload.Gateways = stringSlicePointer(ctx, m.Gateways, "gateways", diags)
	payload.Hostnames = stringSlicePointer(ctx, m.Hostnames, "hostnames", diags)
	payload.Backends = m.backendPayloads(diags)
	if diags.HasError() {
		return nil
	}

	value, err := json.Marshal(payload)
	if err != nil {
		diags.AddError(
			"Invalid traffic TCP route value",
			"the traffic.tcpRoute value could not be encoded; check the JSON attributes",
		)
		return nil
	}
	return value
}

// backendPayloads converts the backend blocks and reports ambiguous variants.
func (m *model) backendPayloads(diags *diag.Diagnostics) []json.RawMessage {
	if len(m.Backends) == 0 {
		return nil
	}

	out := make([]json.RawMessage, 0, len(m.Backends))
	for i, backend := range m.Backends {
		backendPath := path.Root("backends").AtListIndex(i)

		if len(backend.Host) > 1 {
			diags.AddAttributeError(
				backendPath.AtName("host"),
				"Invalid traffic TCP route backend",
				"at most one host block may be set per backend",
			)
		}
		if len(backend.Backend) > 1 {
			diags.AddAttributeError(
				backendPath.AtName("backend"),
				"Invalid traffic TCP route backend",
				"at most one backend block may be set per backend",
			)
		}

		variants := countSet(backend.Service, backend.MCP, backend.AI, backend.JSON)
		if len(backend.Host) > 0 {
			variants++
		}
		if len(backend.Backend) > 0 {
			variants++
		}
		if variants > 1 {
			diags.AddAttributeError(
				backendPath,
				"Invalid traffic TCP route backend",
				"exactly one of host, backend, service_json, mcp_json, ai_json or json may be set per backend",
			)
		}

		var weight *int64
		if isSet(backend.Weight) {
			value := backend.Weight.ValueInt64()
			weight = &value
		}

		payload := backendPayload{
			Weight:  weight,
			Service: rawJSON(backend.Service),
			MCP:     rawJSON(backend.MCP),
			AI:      rawJSON(backend.AI),
		}
		if len(backend.Host) == 1 {
			payload.Host = &hostPayload{
				Host: backend.Host[0].Host.ValueString(),
				Port: backend.Host[0].Port.ValueInt64(),
			}
		}
		if len(backend.Backend) == 1 {
			payload.Backend = &backendRefPayload{
				Name:      backend.Backend[0].Name.ValueString(),
				Namespace: stringPointer(backend.Backend[0].Namespace),
			}
		}

		if isSet(backend.JSON) {
			merged, err := mergeBackendJSON(backend.JSON.ValueString(), weight)
			if err != nil {
				diags.AddAttributeError(
					backendPath.AtName("json"),
					"Invalid traffic TCP route backend",
					"json must be a JSON object; correct the json attribute and re-apply",
				)
				continue
			}
			out = append(out, merged)
			continue
		}

		// The server defaults an omitted weight to 1; send it so the stored
		// value is explicit and never unknown.
		if payload.Weight == nil {
			defaultWeight := int64(1)
			payload.Weight = &defaultWeight
		}

		encoded, err := json.Marshal(payload)
		if err != nil {
			diags.AddError(
				"Invalid traffic TCP route value",
				"the traffic.tcpRoute backend could not be encoded; check the JSON attributes",
			)
			continue
		}
		out = append(out, encoded)
	}
	return out
}

// mergeBackendJSON merges a generic backend object with the weight attribute.
// An explicit weight wins; otherwise the object's own weight is kept, and a
// missing weight defaults to 1.
func mergeBackendJSON(raw string, weight *int64) (json.RawMessage, error) {
	var body map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		return nil, err
	}
	if body == nil {
		body = make(map[string]json.RawMessage)
	}
	switch {
	case weight != nil:
		body["weight"] = json.RawMessage(strconv.FormatInt(*weight, 10))
	case body["weight"] == nil:
		body["weight"] = json.RawMessage("1")
	}
	return json.Marshal(body)
}

// SetValue unmarshals an API value into the model. Attributes the server did
// not return keep their current value instead of being zeroed.
func (m *model) SetValue(ctx context.Context, value json.RawMessage, diags *diag.Diagnostics) {
	var decoded tcpRouteValue
	if err := json.Unmarshal(value, &decoded); err != nil {
		diags.AddError(
			"Failed to decode traffic.tcpRoute value",
			"the gateway returned a value that is not a valid traffic.tcpRoute object",
		)
		return
	}

	if decoded.Name != nil {
		m.Name = types.StringValue(*decoded.Name)
	}
	m.Namespace = preservedString(ctx, m.Namespace, decoded.Namespace)
	m.RuleName = preservedString(ctx, m.RuleName, decoded.RuleName)

	m.Gateways = preservedStringList(ctx, m.Gateways, decoded.Gateways, "gateways", diags)
	m.Hostnames = preservedStringList(ctx, m.Hostnames, decoded.Hostnames, "hostnames", diags)
	m.Policies = preservedJSON(ctx, m.Policies, decoded.Policies)

	if decoded.Backends != nil {
		backends := make([]backendModel, 0, len(*decoded.Backends))
		for _, raw := range *decoded.Backends {
			backend, err := decodeBackend(raw)
			if err != nil {
				diags.AddError(
					"Failed to decode traffic.tcpRoute backend",
					"the gateway returned a backend that is not a valid traffic.tcpRoute backend object",
				)
				return
			}
			backends = append(backends, backend)
		}
		m.Backends = backends
	}
}

// decodeBackend converts one wire backend into the model. Keys the typed schema
// does not know are collected into the generic json variant so the resource
// reads back fully.
//
// The gateway accepts two shapes for the host and backend variants: the object
// form this provider writes ({"host":{"host":"h","port":1}}) and the string form
// a TCP route uses ({"host":"h:1"}, {"backend":"name"}). A value in the string
// form cannot be represented by the typed blocks, so it stays whole in the
// generic json variant instead of failing the read.
func decodeBackend(raw json.RawMessage) (backendModel, error) {
	var body map[string]json.RawMessage
	if err := json.Unmarshal(raw, &body); err != nil {
		return backendModel{}, err
	}
	if body == nil {
		return backendModel{}, errors.New("backend is not a JSON object")
	}

	backend, err := decodeTypedBackend(raw, body)
	if err == nil {
		return backend, nil
	}
	return decodeOpaqueBackend(body)
}

// decodeTypedBackend decodes the object form of a backend.
func decodeTypedBackend(raw json.RawMessage, body map[string]json.RawMessage) (backendModel, error) {
	var decoded backendValue
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return backendModel{}, err
	}

	backend := backendModel{}
	if decoded.Weight != nil {
		backend.Weight = types.Int64Value(*decoded.Weight)
	}
	if decoded.Host != nil {
		host := hostModel{}
		if decoded.Host.Host != nil {
			host.Host = types.StringValue(*decoded.Host.Host)
		}
		if decoded.Host.Port != nil {
			host.Port = types.Int64Value(*decoded.Host.Port)
		}
		backend.Host = []hostModel{host}
	}
	if decoded.Backend != nil {
		ref := backendRefModel{}
		if decoded.Backend.Name != nil {
			ref.Name = types.StringValue(*decoded.Backend.Name)
		}
		if decoded.Backend.Namespace != nil {
			ref.Namespace = types.StringValue(*decoded.Backend.Namespace)
		}
		backend.Backend = []backendRefModel{ref}
	}

	backend.Service = normalizedFromRaw(body["service"])
	backend.MCP = normalizedFromRaw(body["mcp"])
	backend.AI = normalizedFromRaw(body["ai"])

	extras := make(map[string]json.RawMessage)
	for key, value := range body {
		switch key {
		case "weight", "host", "backend", "service", "mcp", "ai":
			continue
		}
		extras[key] = value
	}
	if len(extras) > 0 {
		encoded, err := json.Marshal(extras)
		if err != nil {
			return backendModel{}, err
		}
		backend.JSON = jsontypes.NewNormalizedValue(string(encoded))
	}

	return backend, nil
}

// decodeOpaqueBackend keeps a backend the typed blocks cannot represent in the
// generic json variant, including its weight.
func decodeOpaqueBackend(body map[string]json.RawMessage) (backendModel, error) {
	backend := backendModel{}
	if rawWeight, ok := body["weight"]; ok && !isJSONNull(rawWeight) {
		var weight int64
		if err := json.Unmarshal(rawWeight, &weight); err != nil {
			return backendModel{}, err
		}
		backend.Weight = types.Int64Value(weight)
	}

	extras := make(map[string]json.RawMessage, len(body))
	for key, value := range body {
		if key == "weight" {
			continue
		}
		extras[key] = value
	}
	encoded, err := json.Marshal(extras)
	if err != nil {
		return backendModel{}, err
	}
	backend.JSON = jsontypes.NewNormalizedValue(string(encoded))

	return backend, nil
}

// stringSlicePointer converts an optional Terraform string list to a payload
// pointer. A null or unknown list is omitted; an empty known list is sent as
// [].
func stringSlicePointer(ctx context.Context, value types.List, name string, diags *diag.Diagnostics) *[]string {
	if !isSet(value) {
		return nil
	}
	var values []string
	if d := value.ElementsAs(ctx, &values, false); d.HasError() {
		diags.AddAttributeError(
			path.Root(name),
			"Invalid traffic TCP route value",
			fmt.Sprintf("%s must be a list of strings", name),
		)
		return nil
	}
	return &values
}

// preservedString returns the server value when it returned one, and keeps the
// current value otherwise.
func preservedString(ctx context.Context, current types.String, decoded *string) types.String {
	return resourceutil.PreserveIfNull(ctx, current, stringOrNull(decoded)).(types.String)
}

// preservedStringList returns the server list when it returned one, and keeps
// the current list otherwise. A current unknown value becomes null so it can
// never leak into state.
func preservedStringList(ctx context.Context, current types.List, decoded *[]string, name string, diags *diag.Diagnostics) types.List {
	if decoded == nil {
		if current.IsUnknown() {
			return types.ListNull(types.StringType)
		}
		return current
	}
	list, d := types.ListValueFrom(ctx, types.StringType, *decoded)
	if d.HasError() {
		diags.AddAttributeError(
			path.Root(name),
			"Failed to decode traffic.tcpRoute value",
			fmt.Sprintf("the gateway returned %s that is not a list of strings", name),
		)
		return current
	}
	return list
}

// preservedJSON returns the decoded value when the server returned it and the
// current value otherwise, so an omitted attribute stays null (never zeroed).
func preservedJSON(ctx context.Context, current jsontypes.Normalized, raw json.RawMessage) jsontypes.Normalized {
	return resourceutil.PreserveIfNull(ctx, current, normalizedFromRaw(raw)).(jsontypes.Normalized)
}

// normalizedFromRaw converts a raw JSON leaf to a normalized value. Absent and
// explicit null leaves are reported as not set.
func normalizedFromRaw(raw json.RawMessage) jsontypes.Normalized {
	value, _ := normalizedFromRawOK(raw)
	return value
}

// normalizedFromRawOK converts a raw JSON leaf to a normalized value and
// reports whether the leaf carried a usable value.
func normalizedFromRawOK(raw json.RawMessage) (jsontypes.Normalized, bool) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return jsontypes.Normalized{}, false
	}
	return jsontypes.NewNormalizedValue(string(trimmed)), true
}

// isJSONNull reports whether a raw leaf is absent or an explicit JSON null.
func isJSONNull(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null"))
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

// countSet counts the values that are known and not null.
func countSet(values ...interface {
	IsNull() bool
	IsUnknown() bool
}) int {
	count := 0
	for _, value := range values {
		if isSet(value) {
			count++
		}
	}
	return count
}

// stringOrNull converts an optional decoded string to a Terraform value.
func stringOrNull(value *string) types.String {
	if value == nil {
		return types.StringNull()
	}
	return types.StringValue(*value)
}

// stringPointer converts an optional Terraform string to a payload pointer.
func stringPointer(value types.String) *string {
	if !isSet(value) {
		return nil
	}
	converted := value.ValueString()
	return &converted
}

// Ensure the model satisfies the shared harness interface.
var _ resourceutil.KindResource = (*model)(nil)
