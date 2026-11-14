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
			want: `{"name":"r","backends":[{"aws":{"region":"us-east-1"}}]}`,
		},
		{
			// Backend policies are a sibling of the variant, not a variant:
			// host and policies_json travel together in one backend object.
			name: "host backend with policies",
			mutate: func(t *testing.T, m *model) {
				m.Backends = []backendModel{{
					Host:     types.StringValue("postgres.example.com:5432"),
					Policies: jsonValue(`{"backendTLS":{"hostname":"postgres.example.com"}}`),
				}}
			},
			want: `{"name":"r","backends":[{"weight":1,"host":"postgres.example.com:5432","policies":{"backendTLS":{"hostname":"postgres.example.com"}}}]}`,
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
					Service:  jsontypes.NewNormalizedNull(),
					Dynamic:  jsontypes.NewNormalizedNull(),
					Policies: jsontypes.NewNormalizedNull(),
					JSON:     jsontypes.NewNormalizedNull(),
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
			name:   "service and dynamic",
			first:  func(b *backendModel) { b.Service = jsonValue(`{"name":"default/svc"}`) },
			second: func(b *backendModel) { b.Dynamic = jsonValue(`{"target":"source.address"}`) },
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

// TestTrafficTCPRouteBackendJSONRejectsTypedKeys proves a typed backend key may
// not be smuggled inside the generic json object. Read-back extracts those keys
// into their own attributes, so allowing them would drop them from json and
// produce a perpetual diff.
func TestTrafficTCPRouteBackendJSONRejectsTypedKeys(t *testing.T) {
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
			json:   `{"aws":{"region":"us-east-1"},"host":"127.0.0.1:8080"}`,
			detail: "host must be set as the sibling host attribute, not inside json",
		},
		{
			name:   "backend",
			json:   `{"aws":{"region":"us-east-1"},"backend":"svc"}`,
			detail: "backend must be set as the sibling backend attribute, not inside json",
		},
		{
			name:   "service",
			json:   `{"aws":{"region":"us-east-1"},"service":{"name":"default/svc","port":8080}}`,
			detail: "service must be set as the sibling service_json attribute, not inside json",
		},
		{
			name:   "dynamic",
			json:   `{"aws":{"region":"us-east-1"},"dynamic":{"target":"source.address"}}`,
			detail: "dynamic must be set as the sibling dynamic_json attribute, not inside json",
		},
		{
			// A policies key inside json would be decoded into policies_json on
			// read and dropped from json, so it is rejected up front instead of
			// silently deleting the backend's TLS policies.
			name:   "policies",
			json:   `{"aws":{"region":"us-east-1"},"policies":{"backendTLS":{"hostname":"postgres.example.com"}}}`,
			detail: "policies must be set as the sibling policies_json attribute, not inside json",
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

// TestTrafficTCPRouteBackendJSONAcceptsUntypedKeys proves a legitimate generic
// backend object is still accepted and round-trips unchanged.
func TestTrafficTCPRouteBackendJSONAcceptsUntypedKeys(t *testing.T) {
	const generic = `{"routeGroup":{"name":"group","namespace":"default"}}`

	m := newModel()
	m.Name = types.StringValue("r")
	m.Backends = []backendModel{{JSON: jsonValue(generic)}}

	var diags diag.Diagnostics
	got := m.Value(context.Background(), &diags)
	if diags.HasError() {
		t.Fatalf("Value() returned diagnostics: %v", diags)
	}
	want := `{"name":"r","backends":[{"routeGroup":{"name":"group","namespace":"default"}}]}`
	if string(got) != want {
		t.Fatalf("Value() = %s, want %s", got, want)
	}

	decoded := newModel()
	decoded.SetValue(context.Background(), got, &diags)
	if diags.HasError() {
		t.Fatalf("SetValue() returned diagnostics: %v", diags)
	}
	if len(decoded.Backends) != 1 {
		t.Fatalf("len(Backends) = %d, want 1", len(decoded.Backends))
	}
	if got := decoded.Backends[0].JSON.ValueString(); got != generic {
		t.Errorf("decoded json = %s, want %s", got, generic)
	}
}

// TestTrafficTCPRouteBackendJSONEmptyObjectRoundTrips proves the generic json
// variant is sent verbatim: no synthetic weight is injected, so json = {}
// reads back as {} instead of null.
func TestTrafficTCPRouteBackendJSONEmptyObjectRoundTrips(t *testing.T) {
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
		})
	}
}
