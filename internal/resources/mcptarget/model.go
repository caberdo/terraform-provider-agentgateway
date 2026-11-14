package mcptarget

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/caberdo/terraform-provider-agentgateway/internal/configresource"
	"github.com/caberdo/terraform-provider-agentgateway/internal/resourceutil"
	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// model is the Terraform state shape of agentgateway_mcp_target.
//
// Transport blocks are pointers: a nil pointer means the block is absent, which
// is how the schema's ExactlyOneOf group and the value builder tell "unset"
// from "set to an empty object".
type model struct {
	Name         types.String         `tfsdk:"name"`
	Condition    types.String         `tfsdk:"condition"`
	MCP          *networkModel        `tfsdk:"mcp"`
	SSE          *networkModel        `tfsdk:"sse"`
	Stdio        *stdioModel          `tfsdk:"stdio"`
	OpenAPI      *openAPIModel        `tfsdk:"openapi"`
	PoliciesJSON jsontypes.Normalized `tfsdk:"policies_json"`
	IDValue      types.String         `tfsdk:"id"`
	Revision     types.Int64          `tfsdk:"revision"`
}

// networkModel is the shared shape of the mcp and sse transports.
type networkModel struct {
	Host    types.String `tfsdk:"host"`
	Port    types.Int64  `tfsdk:"port"`
	Path    types.String `tfsdk:"path"`
	Backend types.String `tfsdk:"backend"`
}

// stdioModel is the stdio transport.
type stdioModel struct {
	Cmd      types.String `tfsdk:"cmd"`
	Args     types.List   `tfsdk:"args"`
	Env      types.Map    `tfsdk:"env"`
	ClearEnv types.Bool   `tfsdk:"clear_env"`
}

// openAPIModel is the openapi transport; it adds the inline schema to the
// network shape.
type openAPIModel struct {
	Host       types.String         `tfsdk:"host"`
	Port       types.Int64          `tfsdk:"port"`
	Path       types.String         `tfsdk:"path"`
	Backend    types.String         `tfsdk:"backend"`
	SchemaJSON jsontypes.Normalized `tfsdk:"schema_json"`
}

var _ resourceutil.KindResource = (*model)(nil)

// Kind returns the config resource kind.
func (m *model) Kind() configresource.Kind { return configresource.KindMCPTarget }

// ID derives the resource id from name (spec §7.0).
func (m *model) ID() string { return m.Name.ValueString() }

// targetValue is the JSON value stored by the gateway. Field order is fixed so
// tests can assert the exact payload.
type targetValue struct {
	Name      string          `json:"name"`
	Condition string          `json:"condition,omitempty"`
	MCP       *networkValue   `json:"mcp,omitempty"`
	SSE       *networkValue   `json:"sse,omitempty"`
	Stdio     *stdioValue     `json:"stdio,omitempty"`
	OpenAPI   *openAPIValue   `json:"openapi,omitempty"`
	Policies  json.RawMessage `json:"policies,omitempty"`
}

type networkValue struct {
	Host    string `json:"host,omitempty"`
	Port    *int64 `json:"port,omitempty"`
	Path    string `json:"path,omitempty"`
	Backend string `json:"backend,omitempty"`
}

type stdioValue struct {
	Cmd      string             `json:"cmd"`
	Args     *[]string          `json:"args,omitempty"`
	Env      *map[string]string `json:"env,omitempty"`
	ClearEnv *bool              `json:"clear_env,omitempty"`
}

type openAPIValue struct {
	Host    string          `json:"host,omitempty"`
	Port    *int64          `json:"port,omitempty"`
	Path    string          `json:"path,omitempty"`
	Backend string          `json:"backend,omitempty"`
	Schema  json.RawMessage `json:"schema,omitempty"`
}

// Value marshals the model to the gateway's mcp.target value object.
func (m *model) Value(ctx context.Context, diags *diag.Diagnostics) json.RawMessage {
	name := m.Name.ValueString()
	if name == "" {
		diags.AddError(
			"Invalid agentgateway_mcp_target name",
			"name is required; it is also the resource id",
		)
		return nil
	}

	value := targetValue{
		Name:      name,
		Condition: stringOrEmpty(m.Condition),
	}

	transports := 0
	if m.MCP != nil {
		transports++
		value.MCP = m.MCP.value(ctx, "mcp", diags)
	}
	if m.SSE != nil {
		transports++
		value.SSE = m.SSE.value(ctx, "sse", diags)
	}
	if m.Stdio != nil {
		transports++
		value.Stdio = m.Stdio.value(ctx, diags)
	}
	if m.OpenAPI != nil {
		transports++
		value.OpenAPI = m.OpenAPI.value(ctx, diags)
	}
	if transports != 1 {
		diags.AddError(
			"Invalid agentgateway_mcp_target transport",
			fmt.Sprintf("exactly one of mcp, sse, stdio or openapi must be set; found %d", transports),
		)
	}

	if !m.PoliciesJSON.IsNull() && !m.PoliciesJSON.IsUnknown() && presentJSON(m.PoliciesJSON.ValueString()) {
		value.Policies = json.RawMessage(m.PoliciesJSON.ValueString())
	}

	if diags.HasError() {
		return nil
	}

	raw, err := json.Marshal(value)
	if err != nil {
		diags.AddError(
			"Invalid agentgateway_mcp_target value",
			"the value could not be encoded as JSON",
		)
		return nil
	}
	return raw
}

// value validates and encodes one network transport block.
func (n *networkModel) value(_ context.Context, label string, diags *diag.Diagnostics) *networkValue {
	host := stringOrEmpty(n.Host)
	backend := stringOrEmpty(n.Backend)
	path := stringOrEmpty(n.Path)
	port := int64OrNil(n.Port)

	switch {
	case host != "" && backend != "":
		diags.AddError(
			fmt.Sprintf("Invalid agentgateway_mcp_target %s transport", label),
			"host/port and backend are mutually exclusive; set either host (with an optional port and path) or backend (with an optional path)",
		)
		return nil
	case host == "" && backend == "":
		diags.AddError(
			fmt.Sprintf("Invalid agentgateway_mcp_target %s transport", label),
			"host or backend is required",
		)
		return nil
	case host == "" && port != nil:
		diags.AddError(
			fmt.Sprintf("Invalid agentgateway_mcp_target %s transport", label),
			"port requires host; the backend form takes only backend and an optional path",
		)
		return nil
	}

	return &networkValue{
		Host:    host,
		Port:    port,
		Path:    path,
		Backend: backend,
	}
}

// value validates and encodes the stdio transport.
func (s *stdioModel) value(ctx context.Context, diags *diag.Diagnostics) *stdioValue {
	cmd := stringOrEmpty(s.Cmd)
	if cmd == "" {
		diags.AddError(
			"Invalid agentgateway_mcp_target stdio transport",
			"cmd is required",
		)
		return nil
	}

	value := &stdioValue{Cmd: cmd}
	if !s.Args.IsNull() && !s.Args.IsUnknown() {
		var args []string
		if d := s.Args.ElementsAs(ctx, &args, false); d.HasError() {
			diags.Append(d...)
			return nil
		}
		value.Args = &args
	}
	if !s.Env.IsNull() && !s.Env.IsUnknown() {
		var env map[string]string
		if d := s.Env.ElementsAs(ctx, &env, false); d.HasError() {
			diags.Append(d...)
			return nil
		}
		value.Env = &env
	}
	if !s.ClearEnv.IsNull() && !s.ClearEnv.IsUnknown() {
		clear := s.ClearEnv.ValueBool()
		value.ClearEnv = &clear
	}
	return value
}

// value validates and encodes the openapi transport.
func (o *openAPIModel) value(ctx context.Context, diags *diag.Diagnostics) *openAPIValue {
	network := (&networkModel{
		Host:    o.Host,
		Port:    o.Port,
		Path:    o.Path,
		Backend: o.Backend,
	}).value(ctx, "openapi", diags)
	if network == nil {
		return nil
	}

	value := &openAPIValue{
		Host:    network.Host,
		Port:    network.Port,
		Path:    network.Path,
		Backend: network.Backend,
	}
	if !o.SchemaJSON.IsNull() && !o.SchemaJSON.IsUnknown() && presentJSON(o.SchemaJSON.ValueString()) {
		value.Schema = json.RawMessage(o.SchemaJSON.ValueString())
	}
	return value
}

// SetValue stores the value the gateway returned. A transport block the
// response omits is preserved from state; a block the response contains is
// authoritative, so a change of transport form never leaves stale fields.
func (m *model) SetValue(ctx context.Context, value json.RawMessage, diags *diag.Diagnostics) {
	if len(value) == 0 {
		return
	}

	var decoded targetValue
	if err := json.Unmarshal(value, &decoded); err != nil {
		diags.AddError(
			"Invalid mcp.target value",
			"the gateway returned a value that is not a JSON object; this is a gateway bug or an incompatible release",
		)
		return
	}

	if decoded.Name != "" {
		m.Name = resourceutil.PreserveIfNull(ctx, m.Name, types.StringValue(decoded.Name)).(types.String)
	}
	if decoded.Condition != "" {
		m.Condition = resourceutil.PreserveIfNull(ctx, m.Condition, types.StringValue(decoded.Condition)).(types.String)
	}
	if decoded.MCP != nil {
		m.MCP = decoded.MCP.model()
	}
	if decoded.SSE != nil {
		m.SSE = decoded.SSE.model()
	}
	if decoded.Stdio != nil {
		m.Stdio = decoded.Stdio.model(ctx, diags)
	}
	if decoded.OpenAPI != nil {
		m.OpenAPI = decoded.OpenAPI.model()
	}
	if presentJSON(string(decoded.Policies)) {
		m.PoliciesJSON = resourceutil.PreserveIfNull(
			ctx,
			m.PoliciesJSON,
			jsontypes.NewNormalizedValue(string(decoded.Policies)),
		).(jsontypes.Normalized)
	}
}

// SetServerMeta stores the computed id and revision.
func (m *model) SetServerMeta(id string, revision int64) {
	m.IDValue = types.StringValue(id)
	m.Revision = types.Int64Value(revision)
	if m.Name.IsNull() || m.Name.IsUnknown() {
		m.Name = types.StringValue(id)
	}
}

func (v *networkValue) model() *networkModel {
	return &networkModel{
		Host:    stringOrNull(v.Host),
		Port:    int64OrNull(v.Port),
		Path:    stringOrNull(v.Path),
		Backend: stringOrNull(v.Backend),
	}
}

func (v *stdioValue) model(ctx context.Context, diags *diag.Diagnostics) *stdioModel {
	m := &stdioModel{
		Cmd:      stringOrNull(v.Cmd),
		Args:     types.ListNull(types.StringType),
		Env:      types.MapNull(types.StringType),
		ClearEnv: types.BoolNull(),
	}
	if v.Args != nil {
		list, d := types.ListValueFrom(ctx, types.StringType, *v.Args)
		diags.Append(d...)
		m.Args = list
	}
	if v.Env != nil {
		env, d := types.MapValueFrom(ctx, types.StringType, *v.Env)
		diags.Append(d...)
		m.Env = env
	}
	if v.ClearEnv != nil {
		m.ClearEnv = types.BoolValue(*v.ClearEnv)
	}
	return m
}

func (v *openAPIValue) model() *openAPIModel {
	m := &openAPIModel{
		Host:       stringOrNull(v.Host),
		Port:       int64OrNull(v.Port),
		Path:       stringOrNull(v.Path),
		Backend:    stringOrNull(v.Backend),
		SchemaJSON: jsontypes.NewNormalizedNull(),
	}
	if presentJSON(string(v.Schema)) {
		m.SchemaJSON = jsontypes.NewNormalizedValue(string(v.Schema))
	}
	return m
}

// presentJSON reports whether a JSON leaf carries a value other than an
// explicit null. An omitted or null leaf is left out of the payload.
func presentJSON(raw string) bool {
	trimmed := strings.TrimSpace(raw)
	return trimmed != "" && trimmed != "null"
}

func stringOrEmpty(v types.String) string {
	if v.IsNull() || v.IsUnknown() {
		return ""
	}
	return v.ValueString()
}

func stringOrNull(s string) types.String {
	if s == "" {
		return types.StringNull()
	}
	return types.StringValue(s)
}

func int64OrNil(v types.Int64) *int64 {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	i := v.ValueInt64()
	return &i
}

func int64OrNull(i *int64) types.Int64 {
	if i == nil {
		return types.Int64Null()
	}
	return types.Int64Value(*i)
}
