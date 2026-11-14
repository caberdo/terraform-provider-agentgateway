package mcptarget

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// TestMCPTargetSetValueRoundTrip proves a server value round-trips into the
// model: an imported target (empty state) is filled in and Value() reproduces
// the stored JSON exactly.
func TestMCPTargetSetValueRoundTrip(t *testing.T) {
	server := json.RawMessage(`{"name":"docs","condition":"true","mcp":{"host":"mcp.example.com","port":8080,"path":"/mcp"},"policies":{"cors":{"allowOrigins":["*"]}}}`)

	m := &model{}
	var diags diag.Diagnostics
	m.SetValue(context.Background(), server, &diags)
	if diags.HasError() {
		t.Fatalf("SetValue() diagnostics = %v", diags)
	}
	m.SetServerMeta("docs", 7)

	if got := m.ID(); got != "docs" {
		t.Errorf("ID() = %q, want docs", got)
	}
	if got := m.IDValue.ValueString(); got != "docs" {
		t.Errorf("id = %q, want docs", got)
	}
	if got := m.Revision.ValueInt64(); got != 7 {
		t.Errorf("revision = %d, want 7", got)
	}
	if got := m.Name.ValueString(); got != "docs" {
		t.Errorf("name = %q, want docs", got)
	}
	if got := m.Condition.ValueString(); got != "true" {
		t.Errorf("condition = %q, want true", got)
	}
	if m.MCP == nil {
		t.Fatal("mcp block is nil")
	}
	if got := m.MCP.Host.ValueString(); got != "mcp.example.com" {
		t.Errorf("mcp.host = %q", got)
	}
	if got := m.MCP.Port.ValueInt64(); got != 8080 {
		t.Errorf("mcp.port = %d", got)
	}
	if m.SSE != nil || m.Stdio != nil || m.OpenAPI != nil {
		t.Error("unexpected transport blocks populated")
	}
	if m.PoliciesJSON.IsNull() {
		t.Error("policies_json is null, want the server value")
	}

	var valueDiags diag.Diagnostics
	got := m.Value(context.Background(), &valueDiags)
	if valueDiags.HasError() {
		t.Fatalf("Value() diagnostics = %v", valueDiags)
	}
	if string(got) != string(server) {
		t.Errorf("round trip = %s, want %s", got, server)
	}
}

// TestMCPTargetSetValuePreservesOmittedAttributes proves the model never zeroes
// attributes the server did not return.
func TestMCPTargetSetValuePreservesOmittedAttributes(t *testing.T) {
	m := &model{
		Name:         stringValue("docs"),
		Condition:    stringValue(`request.headers["x-tenant"] == "acme"`),
		MCP:          &networkModel{Host: stringValue("old.example.com"), Port: int64Value(80), Path: stringValue("/mcp"), Backend: types.StringNull()},
		PoliciesJSON: normalized(`{"cors":{"allowOrigins":["*"]}}`),
	}

	var diags diag.Diagnostics
	m.SetValue(context.Background(), json.RawMessage(`{"name":"docs"}`), &diags)
	if diags.HasError() {
		t.Fatalf("SetValue() diagnostics = %v", diags)
	}

	if got := m.Condition.ValueString(); got != `request.headers["x-tenant"] == "acme"` {
		t.Errorf("condition = %q, want the previous value", got)
	}
	if m.PoliciesJSON.IsNull() {
		t.Error("policies_json was zeroed")
	}
	if m.MCP == nil {
		t.Fatal("mcp block was zeroed")
	}
	if got := m.MCP.Host.ValueString(); got != "old.example.com" {
		t.Errorf("mcp.host = %q, want the previous value", got)
	}
}

// TestMCPTargetSetValueTakesServerChanges proves a value the server returns
// replaces the stored state, so an out-of-band change is detected as drift
// instead of being masked by the previous state.
func TestMCPTargetSetValueTakesServerChanges(t *testing.T) {
	m := &model{
		Name:         stringValue("docs"),
		Condition:    stringValue("true"),
		MCP:          &networkModel{Host: stringValue("old.example.com"), Port: int64Value(80), Path: stringValue("/old"), Backend: types.StringNull()},
		PoliciesJSON: normalized(`{"cors":{"allowOrigins":["https://old.example.com"]}}`),
	}

	var diags diag.Diagnostics
	m.SetValue(context.Background(), json.RawMessage(`{"name":"docs","condition":"false","mcp":{"host":"new.example.com","port":443,"path":"/new"},"policies":{"cors":{"allowOrigins":["*"]}}}`), &diags)
	if diags.HasError() {
		t.Fatalf("SetValue() diagnostics = %v", diags)
	}

	if got := m.Condition.ValueString(); got != "false" {
		t.Errorf("condition = %q, want the server value", got)
	}
	if m.MCP == nil {
		t.Fatal("mcp block is nil")
	}
	if got := m.MCP.Host.ValueString(); got != "new.example.com" {
		t.Errorf("mcp.host = %q, want the server value", got)
	}
	if got := m.MCP.Port.ValueInt64(); got != 443 {
		t.Errorf("mcp.port = %d, want the server value", got)
	}
	if got := m.PoliciesJSON.ValueString(); !strings.Contains(got, `"*"`) {
		t.Errorf("policies_json = %q, want the server value", got)
	}
}

// TestMCPTargetSetValueReplacesTransportForm proves a transport block the
// server returns is authoritative: switching from host to backend does not
// leave a stale host behind.
func TestMCPTargetSetValueReplacesTransportForm(t *testing.T) {
	m := &model{
		Name: stringValue("docs"),
		MCP:  &networkModel{Host: stringValue("old.example.com"), Port: int64Value(80), Path: stringValue("/mcp"), Backend: types.StringNull()},
	}

	var diags diag.Diagnostics
	m.SetValue(context.Background(), json.RawMessage(`{"name":"docs","mcp":{"backend":"mcp-backend"}}`), &diags)
	if diags.HasError() {
		t.Fatalf("SetValue() diagnostics = %v", diags)
	}

	if m.MCP == nil {
		t.Fatal("mcp block is nil")
	}
	if !m.MCP.Host.IsNull() {
		t.Errorf("mcp.host = %q, want null", m.MCP.Host.ValueString())
	}
	if !m.MCP.Port.IsNull() {
		t.Errorf("mcp.port = %d, want null", m.MCP.Port.ValueInt64())
	}
	if got := m.MCP.Backend.ValueString(); got != "mcp-backend" {
		t.Errorf("mcp.backend = %q, want mcp-backend", got)
	}
}

// TestMCPTargetSetValueDecodesEveryTransport covers the remaining shapes so a
// gateway value for any transport populates the right block.
func TestMCPTargetSetValueDecodesEveryTransport(t *testing.T) {
	tests := []struct {
		name  string
		value string
		check func(*testing.T, *model)
	}{
		{
			name:  "sse",
			value: `{"name":"events","sse":{"host":"https://sse.example.com"}}`,
			check: func(t *testing.T, m *model) {
				t.Helper()
				if m.SSE == nil || m.SSE.Host.ValueString() != "https://sse.example.com" {
					t.Fatalf("sse block = %+v", m.SSE)
				}
			},
		},
		{
			name:  "stdio",
			value: `{"name":"files","stdio":{"cmd":"npx","args":["-y","server"],"env":{"TOKEN":"abc"},"clear_env":true}}`,
			check: func(t *testing.T, m *model) {
				t.Helper()
				if m.Stdio == nil {
					t.Fatal("stdio block is nil")
				}
				if got := m.Stdio.Cmd.ValueString(); got != "npx" {
					t.Errorf("stdio.cmd = %q", got)
				}
				if got := len(m.Stdio.Args.Elements()); got != 2 {
					t.Errorf("stdio.args has %d elements, want 2", got)
				}
				if got := len(m.Stdio.Env.Elements()); got != 1 {
					t.Errorf("stdio.env has %d elements, want 1", got)
				}
				if !m.Stdio.ClearEnv.ValueBool() {
					t.Error("stdio.clear_env = false, want true")
				}
			},
		},
		{
			name:  "openapi",
			value: `{"name":"api","openapi":{"host":"api.example.com","port":443,"path":"/openapi.json","schema":{"openapi":"3.0.0"}}}`,
			check: func(t *testing.T, m *model) {
				t.Helper()
				if m.OpenAPI == nil {
					t.Fatal("openapi block is nil")
				}
				if got := m.OpenAPI.Port.ValueInt64(); got != 443 {
					t.Errorf("openapi.port = %d", got)
				}
				if m.OpenAPI.SchemaJSON.IsNull() {
					t.Fatal("openapi.schema_json is null")
				}
				var schema map[string]any
				if err := json.Unmarshal([]byte(m.OpenAPI.SchemaJSON.ValueString()), &schema); err != nil {
					t.Fatalf("schema_json is not JSON: %v", err)
				}
				if schema["openapi"] != "3.0.0" {
					t.Errorf("schema_json = %v", schema)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &model{}
			var diags diag.Diagnostics
			m.SetValue(context.Background(), json.RawMessage(tt.value), &diags)
			if diags.HasError() {
				t.Fatalf("SetValue() diagnostics = %v", diags)
			}
			tt.check(t, m)
		})
	}
}

func TestMCPTargetSetValueRejectsNonObject(t *testing.T) {
	m := &model{Name: stringValue("docs")}
	var diags diag.Diagnostics
	m.SetValue(context.Background(), json.RawMessage(`["not","an","object"]`), &diags)
	if !diags.HasError() {
		t.Fatal("SetValue() accepted a non-object value")
	}
	if got := m.Name.ValueString(); got != "docs" {
		t.Errorf("name = %q, want the previous value", got)
	}
}

// TestMCPTargetSetServerMetaSeedsNameFromImport proves an imported resource
// gets its name from the import id.
func TestMCPTargetSetServerMetaSeedsNameFromImport(t *testing.T) {
	m := &model{}
	m.SetServerMeta("docs", 0)
	if got := m.Name.ValueString(); got != "docs" {
		t.Errorf("name = %q, want docs", got)
	}
	if got := m.ID(); got != "docs" {
		t.Errorf("ID() = %q, want docs", got)
	}
}
