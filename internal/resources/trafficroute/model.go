package trafficroute

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/caberdo/terraform-provider-agentgateway/internal/configresource"
	"github.com/caberdo/terraform-provider-agentgateway/internal/resourceutil"
	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// matchModel is one request match of a route.
type matchModel struct {
	Path    []pathModel   `tfsdk:"path"`
	Method  types.String  `tfsdk:"method"`
	Headers []headerModel `tfsdk:"headers"`
	Query   []queryModel  `tfsdk:"query"`
}

// pathModel is the path match. Exactly one of exact, prefix and regex may be
// set (spec §7.11).
type pathModel struct {
	Exact  types.String `tfsdk:"exact"`
	Prefix types.String `tfsdk:"prefix"`
	Regex  types.String `tfsdk:"regex"`
}

// headerModel is one header match.
type headerModel struct {
	Name  types.String `tfsdk:"name"`
	Exact types.String `tfsdk:"exact"`
	Regex types.String `tfsdk:"regex"`
}

// queryModel is one query parameter match.
type queryModel struct {
	Name  types.String `tfsdk:"name"`
	Exact types.String `tfsdk:"exact"`
	Regex types.String `tfsdk:"regex"`
}

// backendModel is one destination of a route. Exactly one variant may be set.
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

// model is the Terraform representation of a traffic.route resource.
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

	Matches  []matchModel         `tfsdk:"matches"`
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

// routePayload is the value object sent to the config resource API. Field order
// matches the documented route shape.
type routePayload struct {
	Name      string            `json:"name"`
	Gateways  *[]string         `json:"gateways,omitempty"`
	Namespace *string           `json:"namespace,omitempty"`
	RuleName  *string           `json:"ruleName,omitempty"`
	Hostnames *[]string         `json:"hostnames,omitempty"`
	Matches   []matchPayload    `json:"matches,omitempty"`
	Backends  []json.RawMessage `json:"backends,omitempty"`
	Policies  json.RawMessage   `json:"policies,omitempty"`
}

// matchPayload is the value object for one request match.
type matchPayload struct {
	Path    *pathPayload    `json:"path,omitempty"`
	Method  *string         `json:"method,omitempty"`
	Headers []headerPayload `json:"headers,omitempty"`
	Query   []queryPayload  `json:"query,omitempty"`
}

// pathPayload is the value object for a path match.
type pathPayload struct {
	Exact  *string `json:"exact,omitempty"`
	Prefix *string `json:"prefix,omitempty"`
	Regex  *string `json:"regex,omitempty"`
}

// headerPayload is the value object for one header match.
type headerPayload struct {
	Name  string  `json:"name"`
	Exact *string `json:"exact,omitempty"`
	Regex *string `json:"regex,omitempty"`
}

// queryPayload is the value object for one query parameter match.
type queryPayload struct {
	Name  string  `json:"name"`
	Exact *string `json:"exact,omitempty"`
	Regex *string `json:"regex,omitempty"`
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

// routeValue is the wire representation decoded from the API.
type routeValue struct {
	Name      *string            `json:"name"`
	Gateways  *[]string          `json:"gateways"`
	Namespace *string            `json:"namespace"`
	RuleName  *string            `json:"ruleName"`
	Hostnames *[]string          `json:"hostnames"`
	Matches   *[]matchValue      `json:"matches"`
	Backends  *[]json.RawMessage `json:"backends"`
	Policies  json.RawMessage    `json:"policies"`
}

// matchValue is the wire representation of one request match.
type matchValue struct {
	Path    *pathValue     `json:"path"`
	Method  *string        `json:"method"`
	Headers *[]headerValue `json:"headers"`
	Query   *[]queryValue  `json:"query"`
}

// pathValue is the wire representation of a path match.
type pathValue struct {
	Exact  *string `json:"exact"`
	Prefix *string `json:"prefix"`
	Regex  *string `json:"regex"`
}

// headerValue is the wire representation of one header match.
type headerValue struct {
	Name  *string `json:"name"`
	Exact *string `json:"exact"`
	Regex *string `json:"regex"`
}

// queryValue is the wire representation of one query parameter match.
type queryValue struct {
	Name  *string `json:"name"`
	Exact *string `json:"exact"`
	Regex *string `json:"regex"`
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
func (m *model) Kind() configresource.Kind { return configresource.KindTrafficRoute }

// ID returns the server-side id, which is the route name (spec §7.0).
func (m *model) ID() string { return m.Name.ValueString() }

// SetServerMeta stores the computed id and revision returned by the server.
func (m *model) SetServerMeta(id string, revision int64) {
	m.StoredID = types.StringValue(id)
	m.Revision = types.Int64Value(revision)
}

// Value marshals the model to the traffic.route value object. The mutually
// exclusive groups are also enforced here, so a hand-built model cannot send an
// ambiguous payload.
func (m *model) Value(ctx context.Context, diags *diag.Diagnostics) json.RawMessage {
	payload := routePayload{
		Name:      m.Name.ValueString(),
		Namespace: stringPointer(m.Namespace),
		RuleName:  stringPointer(m.RuleName),
		Policies:  rawJSON(m.Policies),
	}

	payload.Gateways = stringSlicePointer(ctx, m.Gateways, "gateways", diags)
	payload.Hostnames = stringSlicePointer(ctx, m.Hostnames, "hostnames", diags)
	payload.Matches = m.matchPayloads(diags)
	payload.Backends = m.backendPayloads(diags)
	if diags.HasError() {
		return nil
	}

	value, err := json.Marshal(payload)
	if err != nil {
		diags.AddError(
			"Invalid traffic route value",
			"the traffic.route value could not be encoded; check the JSON attributes",
		)
		return nil
	}
	return value
}

// matchPayloads converts the match blocks and reports ambiguous path and
// header/query matches.
func (m *model) matchPayloads(diags *diag.Diagnostics) []matchPayload {
	if len(m.Matches) == 0 {
		return nil
	}

	out := make([]matchPayload, 0, len(m.Matches))
	for i, match := range m.Matches {
		matchPath := path.Root("matches").AtListIndex(i)
		payload := matchPayload{Method: stringPointer(match.Method)}

		switch {
		case len(match.Path) > 1:
			diags.AddAttributeError(
				matchPath.AtName("path"),
				"Invalid traffic route path match",
				"at most one path block may be set per match",
			)
		case len(match.Path) == 1:
			pathMatch := match.Path[0]
			if countSet(pathMatch.Exact, pathMatch.Prefix, pathMatch.Regex) > 1 {
				diags.AddAttributeError(
					matchPath.AtName("path").AtListIndex(0).AtName("exact"),
					"Invalid traffic route path match",
					"exactly one of exact, prefix or regex may be set in a path block",
				)
			}
			payload.Path = &pathPayload{
				Exact:  stringPointer(pathMatch.Exact),
				Prefix: stringPointer(pathMatch.Prefix),
				Regex:  stringPointer(pathMatch.Regex),
			}
		}

		for j, header := range match.Headers {
			if countSet(header.Exact, header.Regex) > 1 {
				diags.AddAttributeError(
					matchPath.AtName("headers").AtListIndex(j).AtName("exact"),
					"Invalid traffic route header match",
					"exactly one of exact or regex may be set in a header block",
				)
			}
			payload.Headers = append(payload.Headers, headerPayload{
				Name:  header.Name.ValueString(),
				Exact: stringPointer(header.Exact),
				Regex: stringPointer(header.Regex),
			})
		}

		for j, query := range match.Query {
			if countSet(query.Exact, query.Regex) > 1 {
				diags.AddAttributeError(
					matchPath.AtName("query").AtListIndex(j).AtName("exact"),
					"Invalid traffic route query match",
					"exactly one of exact or regex may be set in a query block",
				)
			}
			payload.Query = append(payload.Query, queryPayload{
				Name:  query.Name.ValueString(),
				Exact: stringPointer(query.Exact),
				Regex: stringPointer(query.Regex),
			})
		}

		out = append(out, payload)
	}
	return out
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
				"Invalid traffic route backend",
				"at most one host block may be set per backend",
			)
		}
		if len(backend.Backend) > 1 {
			diags.AddAttributeError(
				backendPath.AtName("backend"),
				"Invalid traffic route backend",
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
				"Invalid traffic route backend",
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
					"Invalid traffic route backend",
					"json must be a JSON object; correct the json attribute and re-apply",
				)
				continue
			}
			out = append(out, merged)
			continue
		}

		// The server defaults an omitted weight to 1 (spec §7.11); send it so
		// the stored value is explicit and never unknown.
		if payload.Weight == nil {
			defaultWeight := int64(1)
			payload.Weight = &defaultWeight
		}

		encoded, err := json.Marshal(payload)
		if err != nil {
			diags.AddError(
				"Invalid traffic route value",
				"the traffic.route backend could not be encoded; check the JSON attributes",
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
	var decoded routeValue
	if err := json.Unmarshal(value, &decoded); err != nil {
		diags.AddError(
			"Failed to decode traffic.route value",
			"the gateway returned a value that is not a valid traffic.route object",
		)
		return
	}

	if decoded.Name != nil {
		m.Name = types.StringValue(*decoded.Name)
	}
	if decoded.Namespace != nil {
		m.Namespace = types.StringValue(*decoded.Namespace)
	}
	if decoded.RuleName != nil {
		m.RuleName = types.StringValue(*decoded.RuleName)
	}

	m.Gateways = preservedStringList(ctx, m.Gateways, decoded.Gateways, "gateways", diags)
	m.Hostnames = preservedStringList(ctx, m.Hostnames, decoded.Hostnames, "hostnames", diags)
	m.Policies = preserveJSON(m.Policies, decoded.Policies)

	if decoded.Matches != nil {
		matches := make([]matchModel, 0, len(*decoded.Matches))
		for _, matchValue := range *decoded.Matches {
			match := matchModel{Method: stringOrNull(matchValue.Method)}
			if matchValue.Path != nil {
				match.Path = []pathModel{{
					Exact:  stringOrNull(matchValue.Path.Exact),
					Prefix: stringOrNull(matchValue.Path.Prefix),
					Regex:  stringOrNull(matchValue.Path.Regex),
				}}
			}
			if matchValue.Headers != nil {
				for _, header := range *matchValue.Headers {
					match.Headers = append(match.Headers, headerModel{
						Name:  stringOrNull(header.Name),
						Exact: stringOrNull(header.Exact),
						Regex: stringOrNull(header.Regex),
					})
				}
			}
			if matchValue.Query != nil {
				for _, query := range *matchValue.Query {
					match.Query = append(match.Query, queryModel{
						Name:  stringOrNull(query.Name),
						Exact: stringOrNull(query.Exact),
						Regex: stringOrNull(query.Regex),
					})
				}
			}
			matches = append(matches, match)
		}
		m.Matches = matches
	}

	if decoded.Backends != nil {
		backends := make([]backendModel, 0, len(*decoded.Backends))
		for _, raw := range *decoded.Backends {
			backend, err := decodeBackend(raw)
			if err != nil {
				diags.AddError(
					"Failed to decode traffic.route backend",
					"the gateway returned a backend that is not a valid traffic.route backend object",
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
func decodeBackend(raw json.RawMessage) (backendModel, error) {
	var body map[string]json.RawMessage
	if err := json.Unmarshal(raw, &body); err != nil {
		return backendModel{}, err
	}

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
			"Invalid traffic route value",
			fmt.Sprintf("%s must be a list of strings", name),
		)
		return nil
	}
	return &values
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
			"Failed to decode traffic.route value",
			fmt.Sprintf("the gateway returned %s that is not a list of strings", name),
		)
		return current
	}
	return list
}

// preserveJSON returns the decoded value when the server returned it and the
// current value otherwise, so an omitted attribute stays null (never zeroed).
func preserveJSON(current jsontypes.Normalized, raw json.RawMessage) jsontypes.Normalized {
	if value, ok := normalizedFromRawOK(raw); ok {
		return value
	}
	return current
}

// normalizedFromRaw converts a raw JSON leaf to a normalized value. Absent and
// explicit null leaves are reported as not set.
func normalizedFromRaw(raw json.RawMessage) jsontypes.Normalized {
	value, _ := normalizedFromRawOK(raw)
	return value
}

func normalizedFromRawOK(raw json.RawMessage) (jsontypes.Normalized, bool) {
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
