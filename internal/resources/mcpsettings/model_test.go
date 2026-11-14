package mcpsettings

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func stringValue(s string) types.String { return types.StringValue(s) }

func int64Value(i int64) types.Int64 { return types.Int64Value(i) }

func boolValue(b bool) types.Bool { return types.BoolValue(b) }

func listValue(t *testing.T, values ...string) types.List {
	t.Helper()
	list, diags := types.ListValueFrom(context.Background(), types.StringType, values)
	if diags.HasError() {
		t.Fatalf("building list: %v", diags)
	}
	return list
}

func TestMCPSettingsValue(t *testing.T) {
	tests := []struct {
		name    string
		model   *model
		want    string
		wantErr bool
	}{
		{
			name: "every attribute",
			model: &model{
				Gateways:               listValue(t, "default"),
				Port:                   types.Int64Null(),
				StatefulMode:           stringValue("stateful"),
				PrefixMode:             stringValue("conditional"),
				FailureMode:            stringValue("failClosed"),
				SSEKeepAlive:           stringValue("30s"),
				DNSRebindingProtection: boolValue(true),
				Server: &serverModel{
					Name:         stringValue("agentgateway"),
					Version:      stringValue("1.7.0"),
					Title:        stringValue("Gateway"),
					Instructions: stringValue("Use tools."),
				},
			},
			want: `{"gateways":["default"],"statefulMode":"stateful","prefixMode":"conditional","failureMode":"failClosed","sseKeepAlive":"30s","dnsRebindingProtection":true,"server":{"name":"agentgateway","version":"1.7.0","title":"Gateway","instructions":"Use tools."}}`,
		},
		{
			name:  "port only",
			model: &model{Port: int64Value(8080)},
			want:  `{"port":8080}`,
		},
		{
			name:    "port and gateways together",
			model:   &model{Port: int64Value(8080), Gateways: listValue(t, "default")},
			wantErr: true,
		},
		{
			name:  "empty gateways list is sent",
			model: &model{Gateways: types.ListValueMust(types.StringType, []attr.Value{})},
			want:  `{"gateways":[]}`,
		},
		{
			name:  "dns rebinding protection explicitly disabled",
			model: &model{DNSRebindingProtection: boolValue(false)},
			want:  `{"dnsRebindingProtection":false}`,
		},
		{
			name: "server name and version only",
			model: &model{Server: &serverModel{
				Name:         stringValue("gw"),
				Version:      stringValue("1"),
				Title:        types.StringNull(),
				Instructions: types.StringNull(),
			}},
			want: `{"server":{"name":"gw","version":"1"}}`,
		},
		{
			name: "server title only is allowed",
			model: &model{Server: &serverModel{
				Name:         types.StringNull(),
				Version:      types.StringNull(),
				Title:        stringValue("Gateway"),
				Instructions: types.StringNull(),
			}},
			want: `{"server":{"title":"Gateway"}}`,
		},
		{
			name: "server name without version",
			model: &model{Server: &serverModel{
				Name:    stringValue("gw"),
				Version: types.StringNull(),
			}},
			wantErr: true,
		},
		{
			name: "server version without name",
			model: &model{Server: &serverModel{
				Name:    types.StringNull(),
				Version: stringValue("1"),
			}},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var diags diag.Diagnostics
			got := tt.model.Value(context.Background(), &diags)
			if tt.wantErr {
				if !diags.HasError() {
					t.Fatalf("Value() = %s, want a diagnostic", got)
				}
				return
			}
			if diags.HasError() {
				t.Fatalf("Value() diagnostics = %v", diags)
			}
			if string(got) != tt.want {
				t.Errorf("Value() = %s, want %s", got, tt.want)
			}
		})
	}
}

// TestMCPSettingsServerNameVersionTogether is the lane's extra test: server
// name and version are either both set or both unset.
func TestMCPSettingsServerNameVersionTogether(t *testing.T) {
	tests := []struct {
		name    string
		server  *serverModel
		wantErr bool
	}{
		{name: "neither", server: &serverModel{}, wantErr: false},
		{name: "both", server: &serverModel{Name: stringValue("gw"), Version: stringValue("1")}, wantErr: false},
		{name: "name only", server: &serverModel{Name: stringValue("gw")}, wantErr: true},
		{name: "version only", server: &serverModel{Version: stringValue("1")}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &model{Server: tt.server}
			var diags diag.Diagnostics
			_ = m.Value(context.Background(), &diags)
			if got := diags.HasError(); got != tt.wantErr {
				t.Errorf("Value() diagnostics error = %v, want %v (%v)", got, tt.wantErr, diags)
			}
		})
	}
}

func TestMCPSettingsID(t *testing.T) {
	m := &model{}
	if got := m.ID(); got != "default" {
		t.Errorf("ID() = %q, want default", got)
	}
}

func TestMCPSettingsKind(t *testing.T) {
	m := &model{}
	if got := m.Kind(); got != "mcp.settings" {
		t.Errorf("Kind() = %q, want mcp.settings", got)
	}
}
