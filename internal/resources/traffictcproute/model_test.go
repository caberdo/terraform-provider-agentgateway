package traffictcproute

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// stringList returns a typed list of strings for tests.
func stringList(t *testing.T, values ...string) types.List {
	t.Helper()
	elements := make([]attr.Value, 0, len(values))
	for _, value := range values {
		elements = append(elements, types.StringValue(value))
	}
	list, diags := types.ListValue(types.StringType, elements)
	if diags.HasError() {
		t.Fatalf("building string list: %v", diags)
	}
	return list
}

// jsonValue returns a normalized JSON value for tests.
func jsonValue(value string) jsontypes.Normalized {
	return jsontypes.NewNormalizedValue(value)
}

func TestTrafficTCPRouteValue(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(t *testing.T, m *model)
		want   string
	}{
		{
			name:   "minimal",
			mutate: func(*testing.T, *model) {},
			want:   `{"name":"r"}`,
		},
		{
			name: "root attributes",
			mutate: func(t *testing.T, m *model) {
				m.Gateways = stringList(t, "default", "internal/listener")
				m.Namespace = types.StringValue("default")
				m.RuleName = types.StringValue("api-rule")
				m.Hostnames = stringList(t, "example.com", "*.example.org")
			},
			want: `{"name":"r","gateways":["default","internal/listener"],"namespace":"default","ruleName":"api-rule","hostnames":["example.com","*.example.org"]}`,
		},
		{
			name: "empty root lists are sent",
			mutate: func(t *testing.T, m *model) {
				m.Gateways = stringList(t)
				m.Hostnames = stringList(t)
			},
			want: `{"name":"r","gateways":[],"hostnames":[]}`,
		},
		{
			name: "backend host",
			mutate: func(t *testing.T, m *model) {
				m.Backends = []backendModel{{
					Host: types.StringValue("127.0.0.1:8080"),
				}}
			},
			want: `{"name":"r","backends":[{"weight":1,"host":"127.0.0.1:8080"}]}`,
		},
		{
			name: "backend reference with weight",
			mutate: func(t *testing.T, m *model) {
				m.Backends = []backendModel{{
					Weight:  types.Int64Value(2),
					Backend: types.StringValue("svc"),
				}}
			},
			want: `{"name":"r","backends":[{"weight":2,"backend":"svc"}]}`,
		},
		{
			name: "service json backend",
			mutate: func(t *testing.T, m *model) {
				m.Backends = []backendModel{{
					Service: jsonValue(`{"name":"default/svc","port":8080}`),
				}}
			},
			want: `{"name":"r","backends":[{"weight":1,"service":{"name":"default/svc","port":8080}}]}`,
		},
		{
			name: "mcp json backend",
			mutate: func(t *testing.T, m *model) {
				m.Backends = []backendModel{{MCP: jsonValue(`{"targets":[{"name":"tools"}]}`)}}
			},
			want: `{"name":"r","backends":[{"weight":1,"mcp":{"targets":[{"name":"tools"}]}}]}`,
		},
		{
			name: "ai json backend",
			mutate: func(t *testing.T, m *model) {
				m.Backends = []backendModel{{AI: jsonValue(`{"provider":"openai"}`)}}
			},
			want: `{"name":"r","backends":[{"weight":1,"ai":{"provider":"openai"}}]}`,
		},
		{
			name: "dynamic json backend",
			mutate: func(t *testing.T, m *model) {
				m.Backends = []backendModel{{Dynamic: jsonValue(`{"target":"source.address"}`)}}
			},
			want: `{"name":"r","backends":[{"weight":1,"dynamic":{"target":"source.address"}}]}`,
		},
		{
			name: "generic json backend",
			mutate: func(t *testing.T, m *model) {
				m.Backends = []backendModel{{JSON: jsonValue(`{"aws":{"region":"us-east-1"}}`)}}
			},
			want: `{"name":"r","backends":[{"aws":{"region":"us-east-1"},"weight":1}]}`,
		},
		{
			name: "policies",
			mutate: func(t *testing.T, m *model) {
				m.Policies = jsonValue(`{"backendTLS":{"hostname":"db.example.com"}}`)
			},
			want: `{"name":"r","policies":{"backendTLS":{"hostname":"db.example.com"}}}`,
		},
		{
			name: "null json leaves are omitted",
			mutate: func(t *testing.T, m *model) {
				m.Policies = jsontypes.NewNormalizedNull()
				m.Backends = []backendModel{{
					Service: jsontypes.NewNormalizedNull(),
					MCP:     jsontypes.NewNormalizedNull(),
					AI:      jsontypes.NewNormalizedNull(),
					Dynamic: jsontypes.NewNormalizedNull(),
					JSON:    jsontypes.NewNormalizedNull(),
				}}
			},
			want: `{"name":"r","backends":[{"weight":1}]}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newModel()
			m.Name = types.StringValue("r")
			tt.mutate(t, m)

			var diags diag.Diagnostics
			got := m.Value(context.Background(), &diags)
			if diags.HasError() {
				t.Fatalf("Value() returned diagnostics: %v", diags)
			}
			if string(got) != tt.want {
				t.Errorf("Value() = %s, want %s", got, tt.want)
			}
		})
	}
}

// TestTrafficTCPRouteBackendWeightDefaultsToOne proves an unset weight is sent
// as the server default of 1 instead of being omitted.
func TestTrafficTCPRouteBackendWeightDefaultsToOne(t *testing.T) {
	m := newModel()
	m.Name = types.StringValue("r")
	m.Backends = []backendModel{{Host: types.StringValue("127.0.0.1:8080")}}

	var diags diag.Diagnostics
	got := m.Value(context.Background(), &diags)
	if diags.HasError() {
		t.Fatalf("Value() returned diagnostics: %v", diags)
	}
	want := `{"name":"r","backends":[{"weight":1,"host":"127.0.0.1:8080"}]}`
	if string(got) != want {
		t.Errorf("Value() = %s, want %s", got, want)
	}
}

// TestTrafficTCPRouteBackendVariantIsExclusive proves two backend variants
// cannot be sent together and that the diagnostic names the offending backend.
func TestTrafficTCPRouteBackendVariantIsExclusive(t *testing.T) {
	tests := []struct {
		name   string
		first  func(*backendModel)
		second func(*backendModel)
	}{
		{
			name: "host and service",
			first: func(b *backendModel) {
				b.Host = types.StringValue("127.0.0.1:8080")
			},
			second: func(b *backendModel) { b.Service = jsonValue(`{"name":"svc"}`) },
		},
		{
			name:   "backend reference and generic json",
			first:  func(b *backendModel) { b.Backend = types.StringValue("svc") },
			second: func(b *backendModel) { b.JSON = jsonValue(`{"internal":{}}`) },
		},
		{
			name:   "mcp and ai",
			first:  func(b *backendModel) { b.MCP = jsonValue(`{"targets":[]}`) },
			second: func(b *backendModel) { b.AI = jsonValue(`{"provider":"openai"}`) },
		},
		{
			name:   "dynamic and host",
			first:  func(b *backendModel) { b.Dynamic = jsonValue(`{"target":"source.address"}`) },
			second: func(b *backendModel) { b.Host = types.StringValue("127.0.0.1:8080") },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newModel()
			m.Name = types.StringValue("r")
			backend := backendModel{}
			tt.first(&backend)
			tt.second(&backend)
			m.Backends = []backendModel{backend}

			var diags diag.Diagnostics
			if got := m.Value(context.Background(), &diags); got != nil {
				t.Errorf("Value() = %s, want nil when the backend is ambiguous", got)
			}
			if !diags.HasError() {
				t.Fatal("Value() returned no diagnostics, want a backend variant error")
			}

			found := false
			for _, diagnostic := range diags.Errors() {
				withPath, ok := diagnostic.(diag.DiagnosticWithPath)
				if !ok {
					continue
				}
				if strings.Contains(withPath.Path().String(), "backends[0]") {
					found = true
				}
			}
			if !found {
				t.Errorf("diagnostics do not name the backend: %v", diags)
			}
		})
	}
}
