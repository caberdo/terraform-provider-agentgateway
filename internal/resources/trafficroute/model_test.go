package trafficroute

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

func TestTrafficRouteValue(t *testing.T) {
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
			name: "path exact",
			mutate: func(t *testing.T, m *model) {
				m.Matches = []matchModel{{
					Path:   []pathModel{{Exact: types.StringValue("/health")}},
					Method: types.StringValue("GET"),
				}}
			},
			want: `{"name":"r","matches":[{"path":{"exact":"/health"},"method":"GET"}]}`,
		},
		{
			name: "path prefix",
			mutate: func(t *testing.T, m *model) {
				m.Matches = []matchModel{{Path: []pathModel{{Prefix: types.StringValue("/api")}}}}
			},
			want: `{"name":"r","matches":[{"path":{"prefix":"/api"}}]}`,
		},
		{
			name: "path regex",
			mutate: func(t *testing.T, m *model) {
				m.Matches = []matchModel{{Path: []pathModel{{Regex: types.StringValue("^/v[0-9]+/")}}}}
			},
			want: `{"name":"r","matches":[{"path":{"regex":"^/v[0-9]+/"}}]}`,
		},
		{
			name: "headers and query",
			mutate: func(t *testing.T, m *model) {
				m.Matches = []matchModel{{
					Path: []pathModel{{Prefix: types.StringValue("/api")}},
					Headers: []headerModel{
						{Name: types.StringValue("x-tenant"), Exact: types.StringValue("acme")},
						{Name: types.StringValue("x-trace"), Regex: types.StringValue("^[a-f0-9]+$")},
					},
					Query: []queryModel{
						{Name: types.StringValue("page"), Exact: types.StringValue("1")},
					},
				}}
			},
			want: `{"name":"r","matches":[{"path":{"prefix":"/api"},"headers":[{"name":"x-tenant","exact":"acme"},{"name":"x-trace","regex":"^[a-f0-9]+$"}],"query":[{"name":"page","exact":"1"}]}]}`,
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
					Service: jsonValue(`{"name":"svc","port":8080}`),
				}}
			},
			want: `{"name":"r","backends":[{"weight":1,"service":{"name":"svc","port":8080}}]}`,
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
			name: "generic json backend",
			mutate: func(t *testing.T, m *model) {
				m.Backends = []backendModel{{JSON: jsonValue(`{"aws":{"region":"us-east-1"}}`)}}
			},
			want: `{"name":"r","backends":[{"aws":{"region":"us-east-1"}}]}`,
		},
		{
			name: "policies",
			mutate: func(t *testing.T, m *model) {
				m.Policies = jsonValue(`{"cors":{"allowOrigins":["https://example.com"]}}`)
			},
			want: `{"name":"r","policies":{"cors":{"allowOrigins":["https://example.com"]}}}`,
		},
		{
			name: "null json leaves are omitted",
			mutate: func(t *testing.T, m *model) {
				m.Policies = jsontypes.NewNormalizedNull()
				m.Backends = []backendModel{{
					Service: jsontypes.NewNormalizedNull(),
					MCP:     jsontypes.NewNormalizedNull(),
					AI:      jsontypes.NewNormalizedNull(),
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

// TestTrafficRouteBackendWeightDefaultsToOne proves an unset weight is sent as
// the server default of 1 instead of being omitted.
func TestTrafficRouteBackendWeightDefaultsToOne(t *testing.T) {
	m := newModel()
	m.Name = types.StringValue("r")
	m.Backends = []backendModel{{
		Host: types.StringValue("127.0.0.1:8080"),
	}}

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

// TestTrafficRouteBackendJSONRejectsTypedKeys proves a typed backend key may
// not be smuggled inside the generic json object. Read-back extracts those keys
// into their own attributes, so allowing them would drop them from json and
// produce a perpetual diff.
func TestTrafficRouteBackendJSONRejectsTypedKeys(t *testing.T) {
	tests := []struct {
		name   string
		json   string
		detail string
	}{
		{
			name:   "weight",
			json:   `{"aws":{"region":"us-east-1"},"weight":5}`,
			detail: "weight must be set as the sibling weight attribute, not inside json",
		},
		{
			name:   "host",
			json:   `{"host":{"host":"127.0.0.1","port":8080}}`,
			detail: "host must be set as the sibling host attribute, not inside json",
		},
		{
			name:   "backend",
			json:   `{"backend":{"name":"svc"}}`,
			detail: "backend must be set as the sibling backend attribute, not inside json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newModel()
			m.Name = types.StringValue("r")
			m.Backends = []backendModel{{JSON: jsonValue(tt.json)}}

			var diags diag.Diagnostics
			if got := m.Value(context.Background(), &diags); got != nil {
				t.Errorf("Value() = %s, want nil when json carries a typed key", got)
			}
			if !diags.HasError() {
				t.Fatal("Value() returned no diagnostics, want a typed-key error")
			}

			found := false
			for _, diagnostic := range diags.Errors() {
				withPath, ok := diagnostic.(diag.DiagnosticWithPath)
				if !ok {
					continue
				}
				if withPath.Path().String() != "backends[0].json" {
					continue
				}
				if strings.Contains(diagnostic.Detail(), tt.detail) {
					found = true
				}
			}
			if !found {
				t.Errorf("diagnostics do not name backends[0].json with %q: %v", tt.detail, diags)
			}
		})
	}
}

// TestTrafficRouteBackendJSONEmptyObjectRoundTrips proves the generic json
// variant is sent verbatim: no synthetic weight is injected, so json = {}
// reads back as {} instead of null.
func TestTrafficRouteBackendJSONEmptyObjectRoundTrips(t *testing.T) {
	tests := []struct {
		name   string
		weight types.Int64
		want   string
	}{
		{
			name:   "without weight",
			weight: types.Int64Null(),
			want:   `{"name":"r","backends":[{}]}`,
		},
		{
			name:   "with weight",
			weight: types.Int64Value(2),
			want:   `{"name":"r","backends":[{"weight":2}]}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newModel()
			m.Name = types.StringValue("r")
			m.Backends = []backendModel{{
				Weight: tt.weight,
				JSON:   jsonValue(`{}`),
			}}

			var diags diag.Diagnostics
			got := m.Value(context.Background(), &diags)
			if diags.HasError() {
				t.Fatalf("Value() returned diagnostics: %v", diags)
			}
			if string(got) != tt.want {
				t.Fatalf("Value() = %s, want %s", got, tt.want)
			}

			decoded := newModel()
			decoded.SetValue(context.Background(), got, &diags)
			if diags.HasError() {
				t.Fatalf("SetValue() returned diagnostics: %v", diags)
			}
			if len(decoded.Backends) != 1 {
				t.Fatalf("len(Backends) = %d, want 1", len(decoded.Backends))
			}
			if decoded.Backends[0].JSON.IsNull() || decoded.Backends[0].JSON.ValueString() != "{}" {
				t.Errorf("decoded json = %v, want {}", decoded.Backends[0].JSON)
			}
			if tt.weight.IsNull() {
				if !decoded.Backends[0].Weight.IsNull() {
					t.Errorf("decoded weight = %d, want null", decoded.Backends[0].Weight.ValueInt64())
				}
			} else if weight := decoded.Backends[0].Weight.ValueInt64(); weight != 2 {
				t.Errorf("decoded weight = %d, want 2", weight)
			}

			var roundTrip diag.Diagnostics
			if again := decoded.Value(context.Background(), &roundTrip); string(again) != tt.want {
				t.Errorf("round-trip Value() = %s, want %s", again, tt.want)
			}
			if roundTrip.HasError() {
				t.Errorf("round-trip Value() returned diagnostics: %v", roundTrip)
			}
		})
	}
}

// TestTrafficRoutePathMatchIsExclusive proves two path variants cannot be sent
// together and that the diagnostic names the offending attribute.
func TestTrafficRoutePathMatchIsExclusive(t *testing.T) {
	m := newModel()
	m.Name = types.StringValue("r")
	m.Matches = []matchModel{{
		Path: []pathModel{{
			Exact:  types.StringValue("/health"),
			Prefix: types.StringValue("/health"),
		}},
	}}

	var diags diag.Diagnostics
	if got := m.Value(context.Background(), &diags); got != nil {
		t.Errorf("Value() = %s, want nil when the path match is ambiguous", got)
	}
	if !diags.HasError() {
		t.Fatal("Value() returned no diagnostics, want a path match error")
	}

	found := false
	for _, diagnostic := range diags.Errors() {
		withPath, ok := diagnostic.(diag.DiagnosticWithPath)
		if !ok {
			continue
		}
		if strings.Contains(withPath.Path().String(), "matches[0].path[0]") {
			found = true
		}
	}
	if !found {
		t.Errorf("diagnostics do not name the path match: %v", diags)
	}
}

// TestTrafficRouteBackendVariantIsExclusive proves two backend variants cannot
// be sent together and that the diagnostic names the offending backend.
func TestTrafficRouteBackendVariantIsExclusive(t *testing.T) {
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
