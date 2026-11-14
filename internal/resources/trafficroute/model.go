package trafficroute

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
	Host    types.String         `tfsdk:"host"`
	Backend types.String         `tfsdk:"backend"`
	Service jsontypes.Normalized `tfsdk:"service_json"`
	MCP     jsontypes.Normalized `tfsdk:"mcp_json"`
	AI      jsontypes.Normalized `tfsdk:"ai_json"`
	JSON    jsontypes.Normalized `tfsdk:"json"`
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
	Weight  *int64          `json:"weight,omitempty"`
	Host    *string         `json:"host,omitempty"`
	Backend *string         `json:"backend,omitempty"`
	Service json.RawMessage `json:"service,omitempty"`
	MCP     json.RawMessage `json:"mcp,omitempty"`
	AI      json.RawMessage `json:"ai,omitempty"`
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

		if countSet(backend.Host, backend.Backend, backend.Service, backend.MCP, backend.AI, backend.JSON) > 1 {
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
			Host:    stringPointer(backend.Host),
			Backend: stringPointer(backend.Backend),
			Service: rawJSON(backend.Service),
			MCP:     rawJSON(backend.MCP),
			AI:      rawJSON(backend.AI),
		}

		if isSet(backend.JSON) {
			merged, err := mergeBackendJSON(backend.JSON.ValueString(), weight)
			if err != nil {
				var keyErr *backendJSONKeyError
				if errors.As(err, &keyErr) {
					diags.AddAttributeError(
						backendPath.AtName("json"),
						"Invalid traffic route backend",
						backendJSONKeyMessage(keyErr.key),
					)
				} else {
					diags.AddAttributeError(
						backendPath.AtName("json"),
						"Invalid traffic route backend",
						"json must be a JSON object; correct the json attribute and re-apply",
					)
				}
				continue
			}
			out = append(out, merged)
			continue
		}

		// The server defaults an omitted weight to 1 (spec §7.11); send it so
		// the stored value is explicit and never unknown. The generic json
		// variant never gets a synthetic weight: it must round-trip unchanged.
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

// backendJSONKeyError reports a typed backend key that the generic json
// variant may not carry.
type backendJSONKeyError struct {
	key string
}

func (e *backendJSONKeyError) Error() string { return e.key }

// backendJSONKeyMessages maps a typed backend key to the sibling Terraform
// attribute that must carry it instead. A key inside json would be extracted
// into the typed attribute on read, so the json object would never round-trip.
var backendJSONKeyMessages = map[string]string{
	"weight":  "weight must be set as the sibling weight attribute, not inside json",
	"host":    "host must be set as the sibling host attribute, not inside json",
	"backend": "backend must be set as the sibling backend attribute, not inside json",
	"service": "service must be set as the sibling service_json attribute, not inside json",
	"mcp":     "mcp must be set as the sibling mcp_json attribute, not inside json",
	"ai":      "ai must be set as the sibling ai_json attribute, not inside json",
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

	backend := backendModel{}
	if weightRaw, ok := body["weight"]; ok && !isJSONNull(weightRaw) {
		var weight int64
		if err := json.Unmarshal(weightRaw, &weight); err != nil {
			return backendModel{}, err
		}
		backend.Weight = types.Int64Value(weight)
	}
	// host and backend are bare strings upstream. The old object form is
	// tolerated but treated as absent instead of failing the whole read.
	if host, ok := bareString(body["host"]); ok {
		backend.Host = types.StringValue(host)
	}
	if name, ok := bareString(body["backend"]); ok {
		backend.Backend = types.StringValue(name)
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
	// A backend with no typed variant is the generic json variant, including an
	// empty object: json = {} must read back as {} rather than null. A known key
	// in an unexpected shape (the old host/backend object form) still counts as
	// typed, so its data is not re-labelled as generic json.
	typedVariant := presentJSON(body, "host") || presentJSON(body, "backend") ||
		isSet(backend.Service) || isSet(backend.MCP) || isSet(backend.AI)
	if len(extras) > 0 || !typedVariant {
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
	if isJSONNull(raw) {
		return jsontypes.Normalized{}, false
	}
	trimmed := bytes.TrimSpace(raw)
	return jsontypes.NewNormalizedValue(string(trimmed)), true
}

// isJSONNull reports whether raw is absent or an explicit JSON null.
func isJSONNull(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null"))
}

// presentJSON reports whether a key is present and not null, regardless of the
// shape of its value.
func presentJSON(body map[string]json.RawMessage, key string) bool {
	raw, ok := body[key]
	return ok && !isJSONNull(raw)
}

// bareString returns a bare JSON string value. Anything that is not a JSON
// string, including the legacy object form of host and backend, is reported as
// absent instead of failing the read.
func bareString(raw json.RawMessage) (string, bool) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '"' {
		return "", false
	}
	var value string
	if err := json.Unmarshal(trimmed, &value); err != nil {
		return "", false
	}
	return value, true
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
