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
//
// Upstream (LocalTCPRouteBackend) models host and backend as bare strings:
// `{"host":"example.com:8000"}` and `{"backend":"name"}`.
type backendModel struct {
	Weight  types.Int64          `tfsdk:"weight"`
	Host    types.String         `tfsdk:"host"`
	Backend types.String         `tfsdk:"backend"`
	Service jsontypes.Normalized `tfsdk:"service_json"`
	MCP     jsontypes.Normalized `tfsdk:"mcp_json"`
	AI      jsontypes.Normalized `tfsdk:"ai_json"`
	Dynamic jsontypes.Normalized `tfsdk:"dynamic_json"`
	JSON    jsontypes.Normalized `tfsdk:"json"`
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
	Weight  *int64          `json:"weight,omitempty"`
	Host    *string         `json:"host,omitempty"`
	Backend *string         `json:"backend,omitempty"`
	Service json.RawMessage `json:"service,omitempty"`
	MCP     json.RawMessage `json:"mcp,omitempty"`
	AI      json.RawMessage `json:"ai,omitempty"`
	Dynamic json.RawMessage `json:"dynamic,omitempty"`
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

		variants := countSet(backend.Host, backend.Backend, backend.Service, backend.MCP, backend.AI, backend.Dynamic, backend.JSON)
		if variants > 1 {
			diags.AddAttributeError(
				backendPath,
				"Invalid traffic TCP route backend",
				"exactly one of host, backend, service_json, mcp_json, ai_json, dynamic_json or json may be set per backend",
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
			Dynamic: rawJSON(backend.Dynamic),
		}
		if isSet(backend.Host) {
			host := backend.Host.ValueString()
			payload.Host = &host
		}
		if isSet(backend.Backend) {
			name := backend.Backend.ValueString()
			payload.Backend = &name
		}

		if isSet(backend.JSON) {
			merged, err := mergeBackendJSON(backend.JSON.ValueString(), weight)
			if err != nil {
				var keyErr *backendJSONKeyError
				if errors.As(err, &keyErr) {
					diags.AddAttributeError(
						backendPath.AtName("json"),
						"Invalid traffic TCP route backend",
						backendJSONKeyMessage(keyErr.key),
					)
				} else {
					diags.AddAttributeError(
						backendPath.AtName("json"),
						"Invalid traffic TCP route backend",
						"json must be a JSON object; correct the json attribute and re-apply",
					)
				}
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

// backendJSONKeyError reports a typed backend key that the generic json
// variant may not carry.
type backendJSONKeyError struct {
	key string
}

func (e *backendJSONKeyError) Error() string { return e.key }

// backendJSONKeyMessages maps a typed backend key to the sibling Terraform
// attribute that must carry it instead. A key inside json would be extracted
// into the typed attribute on read, so the json object would never round-trip
// and the plan would never converge.
var backendJSONKeyMessages = map[string]string{
	"weight":  "weight must be set as the sibling weight attribute, not inside json",
	"host":    "host must be set as the sibling host attribute, not inside json",
	"backend": "backend must be set as the sibling backend attribute, not inside json",
	"service": "service must be set as the sibling service_json attribute, not inside json",
	"mcp":     "mcp must be set as the sibling mcp_json attribute, not inside json",
	"ai":      "ai must be set as the sibling ai_json attribute, not inside json",
	"dynamic": "dynamic must be set as the sibling dynamic_json attribute, not inside json",
}

// backendJSONKeyMessage returns the diagnostic detail for a typed key found
// inside the generic json object.
func backendJSONKeyMessage(key string) string {
	if message, ok := backendJSONKeyMessages[key]; ok {
		return message
	}
	return key + " must not be set inside json"
}

// mergeBackendJSON merges a generic backend object with the weight attribute.
// An explicit weight wins; the object is otherwise sent verbatim. Typed keys
// are rejected because read-back extracts them into their own attributes, which
// would drop them from json and cause a perpetual diff.
func mergeBackendJSON(raw string, weight *int64) (json.RawMessage, error) {
	var body map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		return nil, err
	}
	if body == nil {
		body = make(map[string]json.RawMessage)
	}
	for key := range backendJSONKeyMessages {
		if _, ok := body[key]; ok {
			return nil, &backendJSONKeyError{key: key}
		}
	}
	if weight != nil {
		body["weight"] = json.RawMessage(strconv.FormatInt(*weight, 10))
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

// decodeBackend converts one wire backend into the model.
//
// Upstream (LocalTCPRouteBackend) writes host and backend as bare strings
// ({"host":"h:1"}, {"backend":"name"}); those populate the typed attributes.
// Keys the typed schema does not know are collected into the generic json
// variant so the resource reads back fully. An older object form of host or
// backend is treated as absent and therefore kept whole in the json variant.
func decodeBackend(raw json.RawMessage) (backendModel, error) {
	var body map[string]json.RawMessage
	if err := json.Unmarshal(raw, &body); err != nil {
		return backendModel{}, err
	}
	if body == nil {
		return backendModel{}, errors.New("backend is not a JSON object")
	}

	backend := backendModel{}
	if rawWeight, ok := body["weight"]; ok && !isJSONNull(rawWeight) {
		var weight int64
		if err := json.Unmarshal(rawWeight, &weight); err != nil {
			return backendModel{}, err
		}
		backend.Weight = types.Int64Value(weight)
	}

	host, typedHost, err := stringLeaf(body["host"])
	if err != nil {
		return backendModel{}, err
	}
	if typedHost {
		backend.Host = types.StringValue(host)
	}

	reference, typedBackend, err := stringLeaf(body["backend"])
	if err != nil {
		return backendModel{}, err
	}
	if typedBackend {
		backend.Backend = types.StringValue(reference)
	}

	backend.Service = normalizedFromRaw(body["service"])
	backend.MCP = normalizedFromRaw(body["mcp"])
	backend.AI = normalizedFromRaw(body["ai"])
	backend.Dynamic = normalizedFromRaw(body["dynamic"])

	extras := make(map[string]json.RawMessage)
	for key, value := range body {
		switch key {
		case "weight", "service", "mcp", "ai", "dynamic":
			continue
		case "host":
			if typedHost {
				continue
			}
		case "backend":
			if typedBackend {
				continue
			}
		}
		extras[key] = value
	}
	// A backend with no typed variant is the generic json variant, including an
	// empty object: json = {} must read back as {} rather than null. A known key
	// in an unexpected shape (the old host/backend object form) still counts as
	// typed, so its data is not re-labelled as generic json.
	typedVariant := presentJSON(body, "host") || presentJSON(body, "backend") ||
		isSet(backend.Service) || isSet(backend.MCP) || isSet(backend.AI) || isSet(backend.Dynamic)
	if len(extras) > 0 || !typedVariant {
		encoded, err := json.Marshal(extras)
		if err != nil {
			return backendModel{}, err
		}
		backend.JSON = jsontypes.NewNormalizedValue(string(encoded))
	}

	return backend, nil
}

// presentJSON reports whether the body carries the key with a non-null value.
func presentJSON(body map[string]json.RawMessage, key string) bool {
	raw, ok := body[key]
	return ok && !isJSONNull(raw)
}

// stringLeaf reads a bare-string leaf. It reports false when the leaf is absent
// or null, and also when it holds another JSON type, so a value the typed
// attribute cannot represent is left to the generic json variant instead of
// failing the read.
func stringLeaf(raw json.RawMessage) (string, bool, error) {
	if isJSONNull(raw) {
		return "", false, nil
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", false, nil
	}
	return value, true, nil
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
