package trafficgateway

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// jsonValue returns a normalized JSON value for tests.
func jsonValue(value string) jsontypes.Normalized {
	return jsontypes.NewNormalizedValue(value)
}

// newTestModel returns a model with the required name set.
func newTestModel() *model {
	return &model{Name: types.StringValue("gw")}
}

func TestTrafficGatewayValue(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*model)
		want   string
	}{
		{
			name:   "minimal",
			mutate: func(*model) {},
			want:   `{"name":"gw"}`,
		},
		{
			name: "typed fields",
			mutate: func(m *model) {
				m.Port = types.Int64Value(8080)
				m.BindAddress = types.StringValue("127.0.0.1")
				m.Protocol = types.StringValue("HTTPS")
			},
			want: `{"name":"gw","port":8080,"bindAddress":"127.0.0.1","protocol":"HTTPS"}`,
		},
		{
			name: "tls",
			mutate: func(m *model) {
				m.TLS = jsonValue(`{"cert":"c"}`)
			},
			want: `{"name":"gw","tls":{"cert":"c"}}`,
		},
		{
			name: "oidc",
			mutate: func(m *model) {
				m.OIDC = jsonValue(`{"issuer":"https://example.com"}`)
			},
			want: `{"name":"gw","oidc":{"issuer":"https://example.com"}}`,
		},
		{
			name: "jwt auth",
			mutate: func(m *model) {
				m.JWTAuth = jsonValue(`{"issuer":"https://example.com"}`)
			},
			want: `{"name":"gw","jwtAuth":{"issuer":"https://example.com"}}`,
		},
		{
			name: "authorization",
			mutate: func(m *model) {
				m.Authorization = jsonValue(`{"rules":[]}`)
			},
			want: `{"name":"gw","authorization":{"rules":[]}}`,
		},
		{
			name: "ext authz",
			mutate: func(m *model) {
				m.ExtAuthz = jsonValue(`{"host":"authz.example.com"}`)
			},
			want: `{"name":"gw","extAuthz":{"host":"authz.example.com"}}`,
		},
		{
			name: "ext proc",
			mutate: func(m *model) {
				m.ExtProc = jsonValue(`{"host":"proc.example.com"}`)
			},
			want: `{"name":"gw","extProc":{"host":"proc.example.com"}}`,
		},
		{
			name: "cors",
			mutate: func(m *model) {
				m.CORS = jsonValue(`{"allowOrigins":["https://example.com"]}`)
			},
			want: `{"name":"gw","cors":{"allowOrigins":["https://example.com"]}}`,
		},
		{
			name: "transformations",
			mutate: func(m *model) {
				m.Transformations = jsonValue(`{"request":{"set":[]}}`)
			},
			want: `{"name":"gw","transformations":{"request":{"set":[]}}}`,
		},
		{
			name: "basic auth",
			mutate: func(m *model) {
				m.BasicAuth = jsonValue(`{"htpasswdFile":"/etc/htpasswd"}`)
			},
			want: `{"name":"gw","basicAuth":{"htpasswdFile":"/etc/htpasswd"}}`,
		},
		{
			name: "api key",
			mutate: func(m *model) {
				m.APIKey = jsonValue(`{"mode":"strict"}`)
			},
			want: `{"name":"gw","apiKey":{"mode":"strict"}}`,
		},
		{
			name: "null json leaves are omitted",
			mutate: func(m *model) {
				m.TLS = jsontypes.NewNormalizedNull()
				m.OIDC = jsontypes.NewNormalizedNull()
				m.APIKey = jsontypes.NewNormalizedNull()
			},
			want: `{"name":"gw"}`,
		},
		{
			name: "listeners",
			mutate: func(m *model) {
				m.Port = types.Int64Value(8443)
				m.BindAddress = types.StringValue("0.0.0.0")
				m.Listeners = []ListenerModel{
					{
						Name:     types.StringValue("public"),
						Hostname: types.StringValue("example.com"),
						Protocol: types.StringValue("HTTPS"),
						TLS:      jsonValue(`{"cert":"c"}`),
					},
					{
						Name: types.StringValue("internal"),
					},
				}
			},
			want: `{"name":"gw","port":8443,"bindAddress":"0.0.0.0","listeners":[{"name":"public","hostname":"example.com","protocol":"HTTPS","tls":{"cert":"c"}},{"name":"internal"}]}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestModel()
			tt.mutate(m)

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

// TestTrafficGatewayListenersConflictWithTLSAndPolicies covers C17: a gateway
// with listeners may only set port and bind_address at the top level.
func TestTrafficGatewayListenersConflictWithTLSAndPolicies(t *testing.T) {
	conflicts := []struct {
		name string
		set  func(*model)
	}{
		{name: "protocol", set: func(m *model) { m.Protocol = types.StringValue("HTTPS") }},
		{name: "tls_json", set: func(m *model) { m.TLS = jsonValue(`{"cert":"c"}`) }},
		{name: "oidc_json", set: func(m *model) { m.OIDC = jsonValue(`{}`) }},
		{name: "jwt_auth_json", set: func(m *model) { m.JWTAuth = jsonValue(`{}`) }},
		{name: "authorization_json", set: func(m *model) { m.Authorization = jsonValue(`{}`) }},
		{name: "ext_authz_json", set: func(m *model) { m.ExtAuthz = jsonValue(`{}`) }},
		{name: "ext_proc_json", set: func(m *model) { m.ExtProc = jsonValue(`{}`) }},
		{name: "cors_json", set: func(m *model) { m.CORS = jsonValue(`{}`) }},
		{name: "transformations_json", set: func(m *model) { m.Transformations = jsonValue(`{}`) }},
		{name: "basic_auth_json", set: func(m *model) { m.BasicAuth = jsonValue(`{}`) }},
		{name: "api_key_json", set: func(m *model) { m.APIKey = jsonValue(`{}`) }},
	}

	for _, tt := range conflicts {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestModel()
			m.Listeners = []ListenerModel{{Name: types.StringValue("public")}}
			tt.set(m)

			var diags diag.Diagnostics
			if got := m.Value(context.Background(), &diags); got != nil {
				t.Errorf("Value() = %s, want nil when the configuration conflicts", got)
			}
			if !diags.HasError() {
				t.Fatal("Value() returned no diagnostics, want a conflict error")
			}

			found := false
			for _, d := range diags.Errors() {
				if withPath, ok := d.(diag.DiagnosticWithPath); ok && withPath.Path().String() == tt.name {
					found = true
				}
			}
			if !found {
				t.Errorf("diagnostics do not name the conflicting attribute %q: %v", tt.name, diags)
			}
		})
	}
}
