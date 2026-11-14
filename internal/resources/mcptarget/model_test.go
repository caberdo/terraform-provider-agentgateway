package mcptarget

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
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

func mapValue(t *testing.T, values map[string]string) types.Map {
	t.Helper()
	m, diags := types.MapValueFrom(context.Background(), types.StringType, values)
	if diags.HasError() {
		t.Fatalf("building map: %v", diags)
	}
	return m
}

func normalized(raw string) jsontypes.Normalized {
	return jsontypes.NewNormalizedValue(raw)
}

func TestMCPTargetValue(t *testing.T) {
	tests := []struct {
		name    string
		model   *model
		want    string
		wantErr bool
	}{
		{
			name: "streamable http with host port and path",
			model: &model{
				Name: stringValue("docs"),
				MCP: &networkModel{
					Host:    stringValue("mcp.example.com"),
					Port:    int64Value(8080),
					Path:    stringValue("/mcp"),
					Backend: types.StringNull(),
				},
			},
			want: `{"name":"docs","mcp":{"host":"mcp.example.com","port":8080,"path":"/mcp"}}`,
		},
		{
			name: "streamable http with backend reference",
			model: &model{
				Name: stringValue("docs"),
				MCP: &networkModel{
					Host:    types.StringNull(),
					Port:    types.Int64Null(),
					Path:    stringValue("/mcp"),
					Backend: stringValue("mcp-backend"),
				},
			},
			want: `{"name":"docs","mcp":{"path":"/mcp","backend":"mcp-backend"}}`,
		},
		{
			name: "sse with host only",
			model: &model{
				Name: stringValue("events"),
				SSE: &networkModel{
					Host:    stringValue("https://sse.example.com"),
					Port:    types.Int64Null(),
					Path:    types.StringNull(),
					Backend: types.StringNull(),
				},
			},
			want: `{"name":"events","sse":{"host":"https://sse.example.com"}}`,
		},
		{
			name: "stdio with arguments environment and clear_env",
			model: &model{
				Name: stringValue("files"),
				Stdio: &stdioModel{
					Cmd:      stringValue("npx"),
					Args:     listValue(t, "-y", "@modelcontextprotocol/server-filesystem"),
					Env:      mapValue(t, map[string]string{"TOKEN": "abc"}),
					ClearEnv: boolValue(true),
				},
			},
			want: `{"name":"files","stdio":{"cmd":"npx","args":["-y","@modelcontextprotocol/server-filesystem"],"env":{"TOKEN":"abc"},"clear_env":true}}`,
		},
		{
			name: "stdio without optional fields",
			model: &model{
				Name: stringValue("files"),
				Stdio: &stdioModel{
					Cmd:      stringValue("mcp-server"),
					Args:     types.ListNull(types.StringType),
					Env:      types.MapNull(types.StringType),
					ClearEnv: types.BoolNull(),
				},
			},
			want: `{"name":"files","stdio":{"cmd":"mcp-server"}}`,
		},
		{
			name: "openapi with schema json",
			model: &model{
				Name: stringValue("api"),
				OpenAPI: &openAPIModel{
					Host:       stringValue("api.example.com"),
					Port:       int64Value(443),
					Path:       stringValue("/openapi.json"),
					Backend:    types.StringNull(),
					SchemaJSON: normalized(`{"url":"https://api.example.com/openapi.json"}`),
				},
			},
			want: `{"name":"api","openapi":{"host":"api.example.com","port":443,"path":"/openapi.json","schema":{"url":"https://api.example.com/openapi.json"}}}`,
		},
		{
			// A null JSON leaf is omitted from the payload, never sent as null.
			name: "openapi without schema json",
			model: &model{
				Name: stringValue("api"),
				OpenAPI: &openAPIModel{
					Host:       stringValue("api.example.com"),
					Port:       int64Value(443),
					Path:       stringValue("/openapi.json"),
					Backend:    types.StringNull(),
					SchemaJSON: jsontypes.NewNormalizedNull(),
				},
			},
			want: `{"name":"api","openapi":{"host":"api.example.com","port":443,"path":"/openapi.json"}}`,
		},
		{
			name: "condition and policies json",
			model: &model{
				Name:         stringValue("docs"),
				Condition:    stringValue(`request.headers["x-tenant"] == "acme"`),
				MCP:          &networkModel{Host: stringValue("h"), Port: int64Value(1), Path: stringValue("/p"), Backend: types.StringNull()},
				PoliciesJSON: normalized(`{"cors":{"allowOrigins":["*"]}}`),
			},
			want: `{"name":"docs","condition":"request.headers[\"x-tenant\"] == \"acme\"","mcp":{"host":"h","port":1,"path":"/p"},"policies":{"cors":{"allowOrigins":["*"]}}}`,
		},
		{
			name: "null policies json is omitted",
			model: &model{
				Name:         stringValue("docs"),
				MCP:          &networkModel{Host: stringValue("h"), Port: int64Value(1), Path: stringValue("/p"), Backend: types.StringNull()},
				PoliciesJSON: jsontypes.NewNormalizedNull(),
			},
			want: `{"name":"docs","mcp":{"host":"h","port":1,"path":"/p"}}`,
		},
		{
			name:    "no transport",
			model:   &model{Name: stringValue("docs")},
			wantErr: true,
		},
		{
			name: "two transports",
			model: &model{
				Name: stringValue("docs"),
				MCP:  &networkModel{Host: stringValue("h"), Port: int64Value(1), Path: stringValue("/p"), Backend: types.StringNull()},
				SSE:  &networkModel{Host: stringValue("h"), Port: int64Value(1), Path: stringValue("/p"), Backend: types.StringNull()},
			},
			wantErr: true,
		},
		{
			name: "host and backend together",
			model: &model{
				Name: stringValue("docs"),
				MCP:  &networkModel{Host: stringValue("h"), Port: int64Value(1), Path: stringValue("/p"), Backend: stringValue("b")},
			},
			wantErr: true,
		},
		{
			name: "neither host nor backend",
			model: &model{
				Name: stringValue("docs"),
				MCP:  &networkModel{Host: types.StringNull(), Port: types.Int64Null(), Path: stringValue("/p"), Backend: types.StringNull()},
			},
			wantErr: true,
		},
		{
			name: "host and port without path",
			model: &model{
				Name: stringValue("docs"),
				MCP:  &networkModel{Host: stringValue("h"), Port: int64Value(1), Path: types.StringNull(), Backend: types.StringNull()},
			},
			want: `{"name":"docs","mcp":{"host":"h","port":1}}`,
		},
		{
			name: "backend with port",
			model: &model{
				Name: stringValue("docs"),
				MCP:  &networkModel{Host: types.StringNull(), Port: int64Value(1), Path: stringValue("/p"), Backend: stringValue("b")},
			},
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

// TestMCPTargetRequiresExactlyOneTransport is the lane's extra test: one
// transport is valid, none and two are rejected with a diagnostic.
func TestMCPTargetRequiresExactlyOneTransport(t *testing.T) {
	network := func() *networkModel {
		return &networkModel{Host: stringValue("h"), Port: int64Value(1), Path: stringValue("/p"), Backend: types.StringNull()}
	}

	tests := []struct {
		name    string
		model   *model
		wantErr bool
	}{
		{name: "one", model: &model{Name: stringValue("t"), MCP: network()}},
		{name: "none", model: &model{Name: stringValue("t")}, wantErr: true},
		{name: "two", model: &model{Name: stringValue("t"), MCP: network(), Stdio: &stdioModel{Cmd: stringValue("c")}}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var diags diag.Diagnostics
			_ = tt.model.Value(context.Background(), &diags)
			if got := diags.HasError(); got != tt.wantErr {
				t.Errorf("Value() diagnostics error = %v, want %v (%v)", got, tt.wantErr, diags)
			}
		})
	}
}

func TestMCPTargetID(t *testing.T) {
	m := &model{Name: stringValue("docs")}
	if got := m.ID(); got != "docs" {
		t.Errorf("ID() = %q, want %q", got, "docs")
	}
}

func TestMCPTargetKind(t *testing.T) {
	m := &model{}
	if got := m.Kind(); got != "mcp.target" {
		t.Errorf("Kind() = %q, want %q", got, "mcp.target")
	}
}
