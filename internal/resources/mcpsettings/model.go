package mcpsettings

import (
	"context"
	"encoding/json"

	"github.com/caberdo/terraform-provider-agentgateway/internal/configresource"
	"github.com/caberdo/terraform-provider-agentgateway/internal/resourceutil"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// singletonID is the fixed id of every settings singleton (spec §7.0).
const singletonID = "default"

// model is the Terraform state shape of agentgateway_mcp_settings.
type model struct {
	Gateways               types.List   `tfsdk:"gateways"`
	Port                   types.Int64  `tfsdk:"port"`
	StatefulMode           types.String `tfsdk:"stateful_mode"`
	PrefixMode             types.String `tfsdk:"prefix_mode"`
	FailureMode            types.String `tfsdk:"failure_mode"`
	SSEKeepAlive           types.String `tfsdk:"sse_keep_alive"`
	DNSRebindingProtection types.Bool   `tfsdk:"dns_rebinding_protection"`
	Server                 *serverModel `tfsdk:"server"`
	IDValue                types.String `tfsdk:"id"`
	Revision               types.Int64  `tfsdk:"revision"`
}

// serverModel overrides the MCP serverInfo and the gateway instructions.
type serverModel struct {
	Name         types.String `tfsdk:"name"`
	Version      types.String `tfsdk:"version"`
	Title        types.String `tfsdk:"title"`
	Instructions types.String `tfsdk:"instructions"`
}

var _ resourceutil.KindResource = (*model)(nil)

// Kind returns the config resource kind.
func (m *model) Kind() configresource.Kind { return configresource.KindMCPSettings }

// ID returns the fixed singleton id.
func (m *model) ID() string { return singletonID }

// settingsValue is the JSON value stored by the gateway. Field order is fixed
// so tests can assert the exact payload.
type settingsValue struct {
	Gateways               *[]string    `json:"gateways,omitempty"`
	Port                   *int64       `json:"port,omitempty"`
	StatefulMode           string       `json:"statefulMode,omitempty"`
	PrefixMode             string       `json:"prefixMode,omitempty"`
	FailureMode            string       `json:"failureMode,omitempty"`
	SSEKeepAlive           string       `json:"sseKeepAlive,omitempty"`
	DNSRebindingProtection *bool        `json:"dnsRebindingProtection,omitempty"`
	Server                 *serverValue `json:"server,omitempty"`
}

type serverValue struct {
	Name         string `json:"name,omitempty"`
	Version      string `json:"version,omitempty"`
	Title        string `json:"title,omitempty"`
	Instructions string `json:"instructions,omitempty"`
}

// Value marshals the model to the gateway's mcp.settings value object.
func (m *model) Value(ctx context.Context, diags *diag.Diagnostics) json.RawMessage {
	hasGateways := !m.Gateways.IsNull() && !m.Gateways.IsUnknown()
	hasPort := !m.Port.IsNull() && !m.Port.IsUnknown()
	if hasGateways && hasPort {
		// The schema rejects this at plan time; repeating it here means a
		// hand-built model cannot produce a payload the gateway will reject.
		diags.AddError(
			"Invalid agentgateway_mcp_settings",
			"gateways and port conflict; set gateways, or port for the deprecated listener, but not both",
		)
		return nil
	}

	value := settingsValue{
		Port:                   int64OrNil(m.Port),
		StatefulMode:           stringOrEmpty(m.StatefulMode),
		PrefixMode:             stringOrEmpty(m.PrefixMode),
		FailureMode:            stringOrEmpty(m.FailureMode),
		SSEKeepAlive:           stringOrEmpty(m.SSEKeepAlive),
		DNSRebindingProtection: boolOrNil(m.DNSRebindingProtection),
	}

	if hasGateways {
		var gateways []string
		if d := m.Gateways.ElementsAs(ctx, &gateways, false); d.HasError() {
			diags.Append(d...)
			return nil
		}
		value.Gateways = &gateways
	}

	if m.Server != nil {
		server, ok := m.Server.value(diags)
		if !ok {
			return nil
		}
		value.Server = server
	}

	if diags.HasError() {
		return nil
	}

	raw, err := json.Marshal(value)
	if err != nil {
		diags.AddError(
			"Invalid agentgateway_mcp_settings value",
			"the value could not be encoded as JSON",
		)
		return nil
	}
	return raw
}

// value validates and encodes the server overrides. name and version must be
// set together, matching the gateway's own rule.
func (s *serverModel) value(diags *diag.Diagnostics) (*serverValue, bool) {
	name := stringOrEmpty(s.Name)
	version := stringOrEmpty(s.Version)
	if (name == "") != (version == "") {
		diags.AddError(
			"Invalid agentgateway_mcp_settings server block",
			"server.name and server.version must be set together; set both or neither",
		)
		return nil, false
	}

	return &serverValue{
		Name:         name,
		Version:      version,
		Title:        stringOrEmpty(s.Title),
		Instructions: stringOrEmpty(s.Instructions),
	}, true
}

// SetValue stores the value the gateway returned. Every attribute the response
// omits is preserved from state, because file-owned settings fields are
// dropped by the gateway (spec C9).
func (m *model) SetValue(ctx context.Context, value json.RawMessage, diags *diag.Diagnostics) {
	if len(value) == 0 {
		return
	}

	var decoded settingsValue
	if err := json.Unmarshal(value, &decoded); err != nil {
		diags.AddError(
			"Invalid mcp.settings value",
			"the gateway returned a value that is not a JSON object; this is a gateway bug or an incompatible release",
		)
		return
	}

	gateways := types.ListNull(types.StringType)
	if decoded.Gateways != nil {
		list, d := types.ListValueFrom(ctx, types.StringType, *decoded.Gateways)
		diags.Append(d...)
		if diags.HasError() {
			return
		}
		gateways = list
	}
	m.Gateways = settleList(m.Gateways, gateways)

	port := types.Int64Null()
	if decoded.Port != nil {
		port = types.Int64Value(*decoded.Port)
	}
	m.Port = settleInt64(m.Port, port)

	statefulMode := types.StringNull()
	if decoded.StatefulMode != "" {
		statefulMode = types.StringValue(decoded.StatefulMode)
	}
	m.StatefulMode = settleString(m.StatefulMode, statefulMode)

	prefixMode := types.StringNull()
	if decoded.PrefixMode != "" {
		prefixMode = types.StringValue(decoded.PrefixMode)
	}
	m.PrefixMode = settleString(m.PrefixMode, prefixMode)

	failureMode := types.StringNull()
	if decoded.FailureMode != "" {
		failureMode = types.StringValue(decoded.FailureMode)
	}
	m.FailureMode = settleString(m.FailureMode, failureMode)

	keepAlive := types.StringNull()
	if decoded.SSEKeepAlive != "" {
		keepAlive = types.StringValue(decoded.SSEKeepAlive)
	}
	m.SSEKeepAlive = settleString(m.SSEKeepAlive, keepAlive)

	dnsRebinding := types.BoolNull()
	if decoded.DNSRebindingProtection != nil {
		dnsRebinding = types.BoolValue(*decoded.DNSRebindingProtection)
	}
	m.DNSRebindingProtection = settleBool(m.DNSRebindingProtection, dnsRebinding)

	if decoded.Server != nil {
		m.Server = m.server(decoded.Server)
	}
}

// settleString keeps the stored value when the gateway omitted the attribute,
// and takes the gateway value when it returned one. An unknown stored value
// becomes null: the gateway returned nothing, and an unknown value is invalid
// in state after apply. This is reachable for the Optional+Computed attributes,
// whose plan value is unknown when the configuration omits them.
func settleString(current, server types.String) types.String {
	if !server.IsNull() {
		return server
	}
	if current.IsUnknown() {
		return types.StringNull()
	}
	return current
}

// settleBool is settleString for a boolean attribute.
func settleBool(current, server types.Bool) types.Bool {
	if !server.IsNull() {
		return server
	}
	if current.IsUnknown() {
		return types.BoolNull()
	}
	return current
}

// settleInt64 is settleString for an integer attribute.
func settleInt64(current, server types.Int64) types.Int64 {
	if !server.IsNull() {
		return server
	}
	if current.IsUnknown() {
		return types.Int64Null()
	}
	return current
}

// settleList is settleString for a list of strings. A server-returned empty
// list is the upstream default for `gateways` and means the same as unset, so
// it does not replace a null state value; an explicit empty list in the
// configuration stays empty.
func settleList(current, server types.List) types.List {
	if server.IsUnknown() {
		return current
	}
	if !server.IsNull() {
		if len(server.Elements()) == 0 && current.IsNull() {
			return current
		}
		return server
	}
	if current.IsUnknown() {
		return types.ListNull(types.StringType)
	}
	return current
}

// server merges the returned server overrides over the previous block so an
// omitted field is never zeroed.
func (m *model) server(value *serverValue) *serverModel {
	previous := m.Server
	if previous == nil {
		if value.Name == "" && value.Version == "" && value.Title == "" && value.Instructions == "" {
			// An empty server object carries no override; keeping it as a block
			// of nulls would make Terraform report a block the config omits.
			return nil
		}
		previous = &serverModel{
			Name:         types.StringNull(),
			Version:      types.StringNull(),
			Title:        types.StringNull(),
			Instructions: types.StringNull(),
		}
	}

	merged := &serverModel{
		Name:         preserveString(previous.Name, value.Name),
		Version:      preserveString(previous.Version, value.Version),
		Title:        preserveString(previous.Title, value.Title),
		Instructions: preserveString(previous.Instructions, value.Instructions),
	}
	return merged
}

// preserveString keeps the previous value when the server omitted the field,
// and takes the server value when it returned one.
func preserveString(previous types.String, server string) types.String {
	if server == "" {
		return previous
	}
	return types.StringValue(server)
}

// SetServerMeta stores the computed id and revision.
func (m *model) SetServerMeta(id string, revision int64) {
	m.IDValue = types.StringValue(id)
	m.Revision = types.Int64Value(revision)
}

func stringOrEmpty(v types.String) string {
	if v.IsNull() || v.IsUnknown() {
		return ""
	}
	return v.ValueString()
}

func int64OrNil(v types.Int64) *int64 {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	i := v.ValueInt64()
	return &i
}

func boolOrNil(v types.Bool) *bool {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	b := v.ValueBool()
	return &b
}
