package trafficgateway

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestTrafficGatewaySetValue(t *testing.T) {
	t.Run("full value round-trips", func(t *testing.T) {
		value := `{
			"name": "gw",
			"port": 8443,
			"bindAddress": "0.0.0.0",
			"protocol": "HTTPS",
			"tls": {"cert": "c"},
			"listeners": [
				{"name": "public", "hostname": "example.com", "protocol": "HTTPS", "tls": {"cert": "lc"}}
			],
			"oidc": {"issuer": "i"},
			"jwtAuth": {"issuer": "j"},
			"authorization": {"rules": []},
			"extAuthz": {"host": "a"},
			"extProc": {"host": "p"},
			"cors": {"allowOrigins": ["*"]},
			"transformations": {"request": {}},
			"basicAuth": {"htpasswdFile": "/h"},
			"apiKey": {"mode": "strict"}
		}`

		m := &model{}
		var diags diag.Diagnostics
		m.SetValue(context.Background(), []byte(value), &diags)
		if diags.HasError() {
			t.Fatalf("SetValue() returned diagnostics: %v", diags)
		}

		if got := m.Name.ValueString(); got != "gw" {
			t.Errorf("Name = %q, want gw", got)
		}
		if got := m.Port.ValueInt64(); got != 8443 {
			t.Errorf("Port = %d, want 8443", got)
		}
		if got := m.BindAddress.ValueString(); got != "0.0.0.0" {
			t.Errorf("BindAddress = %q, want 0.0.0.0", got)
		}
		if got := m.Protocol.ValueString(); got != "HTTPS" {
			t.Errorf("Protocol = %q, want HTTPS", got)
		}

		jsonFields := []struct {
			name string
			got  jsontypes.Normalized
			want string
		}{
			{"TLS", m.TLS, `{"cert": "c"}`},
			{"OIDC", m.OIDC, `{"issuer": "i"}`},
			{"JWTAuth", m.JWTAuth, `{"issuer": "j"}`},
			{"Authorization", m.Authorization, `{"rules": []}`},
			{"ExtAuthz", m.ExtAuthz, `{"host": "a"}`},
			{"ExtProc", m.ExtProc, `{"host": "p"}`},
			{"CORS", m.CORS, `{"allowOrigins": ["*"]}`},
			{"Transformations", m.Transformations, `{"request": {}}`},
			{"BasicAuth", m.BasicAuth, `{"htpasswdFile": "/h"}`},
			{"APIKey", m.APIKey, `{"mode": "strict"}`},
		}
		for _, field := range jsonFields {
			if field.got.IsNull() {
				t.Errorf("%s is null, want %s", field.name, field.want)
				continue
			}
			if got := field.got.ValueString(); got != field.want {
				t.Errorf("%s = %s, want %s", field.name, got, field.want)
			}
		}

		if len(m.Listeners) != 1 {
			t.Fatalf("len(Listeners) = %d, want 1", len(m.Listeners))
		}
		listener := m.Listeners[0]
		if got := listener.Name.ValueString(); got != "public" {
			t.Errorf("listener Name = %q, want public", got)
		}
		if got := listener.Hostname.ValueString(); got != "example.com" {
			t.Errorf("listener Hostname = %q, want example.com", got)
		}
		if got := listener.Protocol.ValueString(); got != "HTTPS" {
			t.Errorf("listener Protocol = %q, want HTTPS", got)
		}
		if got := listener.TLS.ValueString(); got != `{"cert": "lc"}` {
			t.Errorf("listener TLS = %s, want {\"cert\": \"lc\"}", got)
		}
	})

	t.Run("omitted attributes stay null", func(t *testing.T) {
		m := &model{}
		var diags diag.Diagnostics
		m.SetValue(context.Background(), []byte(`{"name":"gw","tls":null}`), &diags)
		if diags.HasError() {
			t.Fatalf("SetValue() returned diagnostics: %v", diags)
		}

		if !m.Port.IsNull() {
			t.Errorf("Port = %d, want null", m.Port.ValueInt64())
		}
		if !m.BindAddress.IsNull() {
			t.Errorf("BindAddress = %q, want null", m.BindAddress.ValueString())
		}
		if !m.Protocol.IsNull() {
			t.Errorf("Protocol = %q, want null", m.Protocol.ValueString())
		}
		if !m.TLS.IsNull() {
			t.Errorf("TLS = %s, want null", m.TLS.ValueString())
		}
		if len(m.Listeners) != 0 {
			t.Errorf("len(Listeners) = %d, want 0", len(m.Listeners))
		}

		for name, value := range map[string]jsontypes.Normalized{
			"OIDC":            m.OIDC,
			"JWTAuth":         m.JWTAuth,
			"Authorization":   m.Authorization,
			"ExtAuthz":        m.ExtAuthz,
			"ExtProc":         m.ExtProc,
			"CORS":            m.CORS,
			"Transformations": m.Transformations,
			"BasicAuth":       m.BasicAuth,
			"APIKey":          m.APIKey,
		} {
			if !value.IsNull() {
				t.Errorf("%s = %s, want null", name, value.ValueString())
			}
		}
	})

	t.Run("omitted attributes are preserved from state", func(t *testing.T) {
		m := &model{
			Name:        types.StringValue("old"),
			Port:        types.Int64Value(8080),
			BindAddress: types.StringValue("127.0.0.1"),
			Protocol:    types.StringValue("HTTP"),
			TLS:         jsonValue(`{"cert":"c"}`),
			OIDC:        jsonValue(`{"issuer":"i"}`),
			Listeners:   []ListenerModel{{Name: types.StringValue("public")}},
		}

		var diags diag.Diagnostics
		m.SetValue(context.Background(), []byte(`{"name":"gw"}`), &diags)
		if diags.HasError() {
			t.Fatalf("SetValue() returned diagnostics: %v", diags)
		}

		if got := m.Name.ValueString(); got != "gw" {
			t.Errorf("Name = %q, want gw", got)
		}
		if got := m.Port.ValueInt64(); got != 8080 {
			t.Errorf("Port = %d, want 8080", got)
		}
		if got := m.BindAddress.ValueString(); got != "127.0.0.1" {
			t.Errorf("BindAddress = %q, want 127.0.0.1", got)
		}
		if got := m.Protocol.ValueString(); got != "HTTP" {
			t.Errorf("Protocol = %q, want HTTP", got)
		}
		if got := m.TLS.ValueString(); got != `{"cert":"c"}` {
			t.Errorf("TLS = %s, want {\"cert\":\"c\"}", got)
		}
		if got := m.OIDC.ValueString(); got != `{"issuer":"i"}` {
			t.Errorf("OIDC = %s, want {\"issuer\":\"i\"}", got)
		}
		if len(m.Listeners) != 1 {
			t.Errorf("len(Listeners) = %d, want 1", len(m.Listeners))
		}
	})

	t.Run("empty listeners clear the list", func(t *testing.T) {
		m := &model{
			Name:      types.StringValue("gw"),
			Listeners: []ListenerModel{{Name: types.StringValue("public")}},
		}

		var diags diag.Diagnostics
		m.SetValue(context.Background(), []byte(`{"name":"gw","listeners":[]}`), &diags)
		if diags.HasError() {
			t.Fatalf("SetValue() returned diagnostics: %v", diags)
		}
		if len(m.Listeners) != 0 {
			t.Errorf("len(Listeners) = %d, want 0", len(m.Listeners))
		}
	})

	t.Run("invalid JSON is reported without the body", func(t *testing.T) {
		m := &model{}
		var diags diag.Diagnostics
		m.SetValue(context.Background(), []byte(`{"name": not-json`), &diags)
		if !diags.HasError() {
			t.Fatal("SetValue() returned no diagnostics, want a decode error")
		}
		for _, diagnostic := range diags.Errors() {
			if strings.Contains(diagnostic.Detail(), "not-json") {
				t.Errorf("diagnostic leaks the response body: %q", diagnostic.Detail())
			}
		}
	})
}
