package mcpsettings

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// TestMCPSettingsSetValueRoundTrip proves a server value round-trips into the
// model: every DB-stored field is filled in and Value() reproduces it.
func TestMCPSettingsSetValueRoundTrip(t *testing.T) {
	server := json.RawMessage(`{
		"gateways": ["default", "mcp/listener"],
		"statefulMode": "stateless",
		"prefixMode": "always",
		"failureMode": "failOpen",
		"sseKeepAlive": "15s",
		"dnsRebindingProtection": true,
		"server": {"name": "gw", "version": "1.7.0", "title": "Gateway", "instructions": "Use tools."}
	}`)

	m := &model{Gateways: types.ListNull(types.StringType)}
	var diags diag.Diagnostics
	m.SetValue(context.Background(), server, &diags)
	if diags.HasError() {
		t.Fatalf("SetValue() diagnostics = %v", diags)
	}
	m.SetServerMeta("default", 3)

	if got := m.IDValue.ValueString(); got != "default" {
		t.Errorf("id = %q, want default", got)
	}
	if got := m.Revision.ValueInt64(); got != 3 {
		t.Errorf("revision = %d, want 3", got)
	}
	if m.Gateways.IsNull() {
		t.Fatal("gateways = null, want the server list")
	}
	if got := len(m.Gateways.Elements()); got != 2 {
		t.Errorf("len(gateways) = %d, want 2", got)
	}
	if got := m.StatefulMode.ValueString(); got != "stateless" {
		t.Errorf("stateful_mode = %q", got)
	}
	if got := m.PrefixMode.ValueString(); got != "always" {
		t.Errorf("prefix_mode = %q", got)
	}
	if got := m.FailureMode.ValueString(); got != "failOpen" {
		t.Errorf("failure_mode = %q", got)
	}
	if got := m.SSEKeepAlive.ValueString(); got != "15s" {
		t.Errorf("sse_keep_alive = %q", got)
	}
	if !m.DNSRebindingProtection.ValueBool() {
		t.Error("dns_rebinding_protection = false, want true")
	}
	if m.Server == nil {
		t.Fatal("server block is nil")
	}
	if got := m.Server.Name.ValueString(); got != "gw" {
		t.Errorf("server.name = %q", got)
	}
	if got := m.Server.Version.ValueString(); got != "1.7.0" {
		t.Errorf("server.version = %q", got)
	}

	var valueDiags diag.Diagnostics
	got := m.Value(context.Background(), &valueDiags)
	if valueDiags.HasError() {
		t.Fatalf("Value() diagnostics = %v", valueDiags)
	}
	var decoded, want map[string]any
	if err := json.Unmarshal(got, &decoded); err != nil {
		t.Fatalf("Value() is not JSON: %v", err)
	}
	if err := json.Unmarshal(server, &want); err != nil {
		t.Fatalf("test fixture is not JSON: %v", err)
	}
	if !equalJSON(decoded, want) {
		t.Errorf("round trip = %s, want %s", got, server)
	}
}

// TestMCPSettingsSetValuePreservesFileOwnedFields proves a field the config
// file owns — which the gateway therefore omits from the response — keeps the
// value Terraform stored instead of being zeroed (spec C9).
func TestMCPSettingsSetValuePreservesFileOwnedFields(t *testing.T) {
	m := &model{
		Gateways:     listValue(t, "default"),
		StatefulMode: stringValue("stateful"),
		Server: &serverModel{
			Name:         stringValue("gw"),
			Version:      stringValue("1"),
			Title:        types.StringNull(),
			Instructions: types.StringNull(),
		},
	}

	var diags diag.Diagnostics
	m.SetValue(context.Background(), json.RawMessage(`{"prefixMode":"conditional"}`), &diags)
	if diags.HasError() {
		t.Fatalf("SetValue() diagnostics = %v", diags)
	}

	if m.Gateways.IsNull() {
		t.Fatal("gateways = null, want the state value preserved")
	}
	if got := m.StatefulMode.ValueString(); got != "stateful" {
		t.Errorf("stateful_mode = %q, want the state value", got)
	}
	if m.Server == nil {
		t.Fatal("server block was zeroed")
	}
	if got := m.Server.Name.ValueString(); got != "gw" {
		t.Errorf("server.name = %q, want the state value", got)
	}
	if got := m.PrefixMode.ValueString(); got != "conditional" {
		t.Errorf("prefix_mode = %q, want the server value", got)
	}
}

// TestMCPSettingsSetValueTakesServerChanges proves a value the server returns
// replaces the stored state, so drift is detected.
func TestMCPSettingsSetValueTakesServerChanges(t *testing.T) {
	m := &model{
		Gateways:               listValue(t, "old"),
		StatefulMode:           stringValue("stateful"),
		DNSRebindingProtection: boolValue(true),
	}

	var diags diag.Diagnostics
	m.SetValue(context.Background(), json.RawMessage(`{"gateways":["new"],"statefulMode":"stateless","dnsRebindingProtection":false}`), &diags)
	if diags.HasError() {
		t.Fatalf("SetValue() diagnostics = %v", diags)
	}

	var gateways []string
	if listDiags := m.Gateways.ElementsAs(context.Background(), &gateways, false); listDiags.HasError() {
		t.Fatalf("decoding gateways: %v", listDiags)
	}
	if len(gateways) != 1 || gateways[0] != "new" {
		t.Errorf("gateways = %v, want the server value [new]", gateways)
	}
	if got := m.StatefulMode.ValueString(); got != "stateless" {
		t.Errorf("stateful_mode = %q, want the server value", got)
	}
	if m.DNSRebindingProtection.ValueBool() {
		t.Error("dns_rebinding_protection = true, want the server value false")
	}
}

// TestMCPSettingsSetValueOmitsEmptyServerBlock proves an empty server object
// from the server does not invent a block the config omits.
func TestMCPSettingsSetValueOmitsEmptyServerBlock(t *testing.T) {
	m := &model{}
	var diags diag.Diagnostics
	m.SetValue(context.Background(), json.RawMessage(`{"server":{}}`), &diags)
	if diags.HasError() {
		t.Fatalf("SetValue() diagnostics = %v", diags)
	}
	if m.Server != nil {
		t.Errorf("server block = %+v, want nil", m.Server)
	}
}

// TestMCPSettingsSetValueEmptyResponseLeavesStateAlone covers the file-mode
// shape: the gateway returns an empty object, so every attribute stays null.
func TestMCPSettingsSetValueEmptyResponseLeavesStateAlone(t *testing.T) {
	m := &model{Gateways: types.ListNull(types.StringType)}
	var diags diag.Diagnostics
	m.SetValue(context.Background(), json.RawMessage(`{}`), &diags)
	if diags.HasError() {
		t.Fatalf("SetValue() diagnostics = %v", diags)
	}

	if !m.Gateways.IsNull() {
		t.Errorf("gateways = %v, want null", m.Gateways)
	}
	if !m.Port.IsNull() {
		t.Errorf("port = %v, want null", m.Port)
	}
	if !m.StatefulMode.IsNull() {
		t.Errorf("stateful_mode = %v, want null", m.StatefulMode)
	}
	if !m.DNSRebindingProtection.IsNull() {
		t.Errorf("dns_rebinding_protection = %v, want null", m.DNSRebindingProtection)
	}
	if m.Server != nil {
		t.Errorf("server block = %+v, want nil", m.Server)
	}
}

func TestMCPSettingsSetValueRejectsNonObject(t *testing.T) {
	m := &model{}
	var diags diag.Diagnostics
	m.SetValue(context.Background(), json.RawMessage(`["not","an","object"]`), &diags)
	if !diags.HasError() {
		t.Fatal("SetValue() accepted a non-object value")
	}
}

// TestMCPSettingsSetValueIgnoresDefaultEmptyGateways proves the gateway's
// default empty gateways list does not become state when the configuration
// leaves `gateways` unset: an empty list and an unset list mean the same thing
// upstream (attach to the default gateway), so storing `[]` would create a
// permanent diff against a null configuration.
func TestMCPSettingsSetValueIgnoresDefaultEmptyGateways(t *testing.T) {
	m := &model{Gateways: types.ListNull(types.StringType)}
	var diags diag.Diagnostics
	m.SetValue(context.Background(), json.RawMessage(`{"gateways":[]}`), &diags)
	if diags.HasError() {
		t.Fatalf("SetValue() diagnostics = %v", diags)
	}
	if !m.Gateways.IsNull() {
		t.Errorf("gateways = %v, want null", m.Gateways)
	}
}

// TestMCPSettingsSetValueKeepsExplicitEmptyGateways proves an explicit empty
// list survives a read, because the configuration asked for it.
func TestMCPSettingsSetValueKeepsExplicitEmptyGateways(t *testing.T) {
	m := &model{Gateways: types.ListValueMust(types.StringType, nil)}
	var diags diag.Diagnostics
	m.SetValue(context.Background(), json.RawMessage(`{"gateways":[]}`), &diags)
	if diags.HasError() {
		t.Fatalf("SetValue() diagnostics = %v", diags)
	}
	if m.Gateways.IsNull() {
		t.Fatal("gateways = null, want the explicit empty list")
	}
	if got := len(m.Gateways.Elements()); got != 0 {
		t.Errorf("len(gateways) = %d, want 0", got)
	}
}

// TestMCPSettingsSetValueResolvesUnknownDefaults proves a value the plan left
// unknown, because the attribute is Optional+Computed and the configuration
// omitted it, becomes null when the gateway returns nothing. An unknown value
// is invalid in state after apply.
func TestMCPSettingsSetValueResolvesUnknownDefaults(t *testing.T) {
	m := &model{
		StatefulMode:           types.StringUnknown(),
		DNSRebindingProtection: types.BoolUnknown(),
	}
	var diags diag.Diagnostics
	m.SetValue(context.Background(), json.RawMessage(`{}`), &diags)
	if diags.HasError() {
		t.Fatalf("SetValue() diagnostics = %v", diags)
	}
	if !m.StatefulMode.IsNull() {
		t.Errorf("stateful_mode = %v, want null", m.StatefulMode)
	}
	if !m.DNSRebindingProtection.IsNull() {
		t.Errorf("dns_rebinding_protection = %v, want null", m.DNSRebindingProtection)
	}
}

// equalJSON compares two decoded JSON documents structurally.
func equalJSON(a, b any) bool {
	left, err := json.Marshal(a)
	if err != nil {
		return false
	}
	right, err := json.Marshal(b)
	if err != nil {
		return false
	}
	return string(left) == string(right)
}
