package traffictcproute

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestTrafficTCPRouteSetValue(t *testing.T) {
	t.Run("full value round-trips", func(t *testing.T) {
		value := `{
			"name": "r",
			"gateways": ["default", "internal/listener"],
			"namespace": "default",
			"ruleName": "api-rule",
			"hostnames": ["example.com", "*.example.org"],
			"backends": [
				{"weight": 2, "host": {"host": "127.0.0.1", "port": 8080}},
				{"weight": 1, "backend": {"name": "svc", "namespace": "default"}},
				{"weight": 1, "service": {"name": "svc"}},
				{"weight": 1, "mcp": {"targets": []}},
				{"weight": 1, "ai": {"provider": "openai"}},
				{"weight": 3, "aws": {"region": "us-east-1"}}
			],
			"policies": {"cors": {"allowOrigins": ["https://example.com"]}}
		}`

		m := newModel()
		var diags diag.Diagnostics
		m.SetValue(context.Background(), []byte(value), &diags)
		if diags.HasError() {
			t.Fatalf("SetValue() returned diagnostics: %v", diags)
		}

		if got := m.Name.ValueString(); got != "r" {
			t.Errorf("Name = %q, want r", got)
		}
		if got := m.Namespace.ValueString(); got != "default" {
			t.Errorf("Namespace = %q, want default", got)
		}
		if got := m.RuleName.ValueString(); got != "api-rule" {
			t.Errorf("RuleName = %q, want api-rule", got)
		}

		var gateways []string
		if d := m.Gateways.ElementsAs(context.Background(), &gateways, false); d.HasError() {
			t.Fatalf("decoding gateways: %v", d)
		}
		if len(gateways) != 2 || gateways[0] != "default" || gateways[1] != "internal/listener" {
			t.Errorf("Gateways = %v, want [default internal/listener]", gateways)
		}

		var hostnames []string
		if d := m.Hostnames.ElementsAs(context.Background(), &hostnames, false); d.HasError() {
			t.Fatalf("decoding hostnames: %v", d)
		}
		if len(hostnames) != 2 || hostnames[0] != "example.com" || hostnames[1] != "*.example.org" {
			t.Errorf("Hostnames = %v, want [example.com *.example.org]", hostnames)
		}

		if len(m.Backends) != 6 {
			t.Fatalf("len(Backends) = %d, want 6", len(m.Backends))
		}
		if got := m.Backends[0].Weight.ValueInt64(); got != 2 {
			t.Errorf("backend[0] Weight = %d, want 2", got)
		}
		if len(m.Backends[0].Host) != 1 || m.Backends[0].Host[0].Host.ValueString() != "127.0.0.1" || m.Backends[0].Host[0].Port.ValueInt64() != 8080 {
			t.Errorf("backend[0] Host = %+v, want 127.0.0.1:8080", m.Backends[0].Host)
		}
		if len(m.Backends[1].Backend) != 1 || m.Backends[1].Backend[0].Name.ValueString() != "svc" || m.Backends[1].Backend[0].Namespace.ValueString() != "default" {
			t.Errorf("backend[1] Backend = %+v, want svc/default", m.Backends[1].Backend)
		}

		jsonFields := []struct {
			name string
			got  jsontypes.Normalized
			want string
		}{
			{"backend[2].Service", m.Backends[2].Service, `{"name": "svc"}`},
			{"backend[3].MCP", m.Backends[3].MCP, `{"targets": []}`},
			{"backend[4].AI", m.Backends[4].AI, `{"provider": "openai"}`},
			{"backend[5].JSON", m.Backends[5].JSON, `{"aws":{"region":"us-east-1"}}`},
			{"Policies", m.Policies, `{"cors": {"allowOrigins": ["https://example.com"]}}`},
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
	})

	t.Run("omitted attributes stay null", func(t *testing.T) {
		m := newModel()
		var diags diag.Diagnostics
		m.SetValue(context.Background(), []byte(`{"name":"r","policies":null}`), &diags)
		if diags.HasError() {
			t.Fatalf("SetValue() returned diagnostics: %v", diags)
		}

		if !m.Gateways.IsNull() {
			t.Errorf("Gateways = %v, want null", m.Gateways)
		}
		if !m.Hostnames.IsNull() {
			t.Errorf("Hostnames = %v, want null", m.Hostnames)
		}
		if !m.Namespace.IsNull() {
			t.Errorf("Namespace = %q, want null", m.Namespace.ValueString())
		}
		if !m.RuleName.IsNull() {
			t.Errorf("RuleName = %q, want null", m.RuleName.ValueString())
		}
		if len(m.Backends) != 0 {
			t.Errorf("len(Backends) = %d, want 0", len(m.Backends))
		}
		if !m.Policies.IsNull() {
			t.Errorf("Policies = %s, want null", m.Policies.ValueString())
		}
	})

	t.Run("omitted attributes are preserved from state", func(t *testing.T) {
		m := newModel()
		m.Name = types.StringValue("old")
		m.Gateways = stringList(t, "default")
		m.Hostnames = stringList(t, "example.com")
		m.Namespace = types.StringValue("old-ns")
		m.RuleName = types.StringValue("old-rule")
		m.Backends = []backendModel{{Host: []hostModel{{Host: types.StringValue("old"), Port: types.Int64Value(1)}}}}
		m.Policies = jsonValue(`{"cors":{}}`)

		var diags diag.Diagnostics
		m.SetValue(context.Background(), []byte(`{"name":"r"}`), &diags)
		if diags.HasError() {
			t.Fatalf("SetValue() returned diagnostics: %v", diags)
		}

		if got := m.Name.ValueString(); got != "r" {
			t.Errorf("Name = %q, want r", got)
		}
		if got := m.Namespace.ValueString(); got != "old-ns" {
			t.Errorf("Namespace = %q, want old-ns", got)
		}
		if got := m.RuleName.ValueString(); got != "old-rule" {
			t.Errorf("RuleName = %q, want old-rule", got)
		}

		var gateways []string
		if d := m.Gateways.ElementsAs(context.Background(), &gateways, false); d.HasError() {
			t.Fatalf("decoding gateways: %v", d)
		}
		if len(gateways) != 1 || gateways[0] != "default" {
			t.Errorf("Gateways = %v, want [default]", gateways)
		}
		var hostnames []string
		if d := m.Hostnames.ElementsAs(context.Background(), &hostnames, false); d.HasError() {
			t.Fatalf("decoding hostnames: %v", d)
		}
		if len(hostnames) != 1 || hostnames[0] != "example.com" {
			t.Errorf("Hostnames = %v, want [example.com]", hostnames)
		}
		if len(m.Backends) != 1 || m.Backends[0].Host[0].Host.ValueString() != "old" {
			t.Errorf("Backends = %+v, want the state value preserved", m.Backends)
		}
		if got := m.Policies.ValueString(); got != `{"cors":{}}` {
			t.Errorf("Policies = %s, want the state value preserved", got)
		}
	})

	t.Run("empty lists clear the lists", func(t *testing.T) {
		m := newModel()
		m.Gateways = stringList(t, "default")
		m.Hostnames = stringList(t, "example.com")
		m.Backends = []backendModel{{Host: []hostModel{{Host: types.StringValue("old"), Port: types.Int64Value(1)}}}}

		var diags diag.Diagnostics
		m.SetValue(context.Background(), []byte(`{"name":"r","gateways":[],"hostnames":[],"backends":[]}`), &diags)
		if diags.HasError() {
			t.Fatalf("SetValue() returned diagnostics: %v", diags)
		}

		if len(m.Gateways.Elements()) != 0 {
			t.Errorf("Gateways = %v, want empty", m.Gateways)
		}
		if len(m.Hostnames.Elements()) != 0 {
			t.Errorf("Hostnames = %v, want empty", m.Hostnames)
		}
		if len(m.Backends) != 0 {
			t.Errorf("len(Backends) = %d, want 0", len(m.Backends))
		}
	})

	t.Run("unknown computed lists become null instead of staying unknown", func(t *testing.T) {
		m := newModel()
		m.Gateways = types.ListUnknown(types.StringType)

		var diags diag.Diagnostics
		m.SetValue(context.Background(), []byte(`{"name":"r"}`), &diags)
		if diags.HasError() {
			t.Fatalf("SetValue() returned diagnostics: %v", diags)
		}
		if !m.Gateways.IsNull() {
			t.Errorf("Gateways = %v, want null", m.Gateways)
		}
	})

	t.Run("server returned weight is stored", func(t *testing.T) {
		m := newModel()
		var diags diag.Diagnostics
		m.SetValue(context.Background(), []byte(`{"name":"r","backends":[{"weight":4,"host":{"host":"h","port":1}}]}`), &diags)
		if diags.HasError() {
			t.Fatalf("SetValue() returned diagnostics: %v", diags)
		}
		if len(m.Backends) != 1 {
			t.Fatalf("len(Backends) = %d, want 1", len(m.Backends))
		}
		if got := m.Backends[0].Weight.ValueInt64(); got != 4 {
			t.Errorf("Weight = %d, want 4", got)
		}
	})

	t.Run("string host backend stays in the json variant", func(t *testing.T) {
		m := newModel()
		var diags diag.Diagnostics
		m.SetValue(context.Background(), []byte(`{"name":"r","backends":[{"weight":2,"host":"tcp.example:80"}]}`), &diags)
		if diags.HasError() {
			t.Fatalf("SetValue() returned diagnostics: %v", diags)
		}
		if len(m.Backends) != 1 {
			t.Fatalf("len(Backends) = %d, want 1", len(m.Backends))
		}
		backend := m.Backends[0]
		if len(backend.Host) != 0 {
			t.Errorf("Host = %+v, want the string form kept in json", backend.Host)
		}
		if got := backend.Weight.ValueInt64(); got != 2 {
			t.Errorf("Weight = %d, want 2", got)
		}
		if backend.JSON.IsNull() {
			t.Fatal("JSON is null, want the string host form")
		}
		if got, want := backend.JSON.ValueString(), `{"host":"tcp.example:80"}`; got != want {
			t.Errorf("JSON = %s, want %s", got, want)
		}
	})

	t.Run("string backend reference stays in the json variant", func(t *testing.T) {
		m := newModel()
		var diags diag.Diagnostics
		m.SetValue(context.Background(), []byte(`{"name":"r","backends":[{"backend":"postgres"}]}`), &diags)
		if diags.HasError() {
			t.Fatalf("SetValue() returned diagnostics: %v", diags)
		}
		if len(m.Backends) != 1 {
			t.Fatalf("len(Backends) = %d, want 1", len(m.Backends))
		}
		backend := m.Backends[0]
		if len(backend.Backend) != 0 {
			t.Errorf("Backend = %+v, want the string form kept in json", backend.Backend)
		}
		if got, want := backend.JSON.ValueString(), `{"backend":"postgres"}`; got != want {
			t.Errorf("JSON = %s, want %s", got, want)
		}
	})

	t.Run("service backend with a port round-trips", func(t *testing.T) {
		m := newModel()
		var diags diag.Diagnostics
		m.SetValue(context.Background(), []byte(`{"name":"r","backends":[{"weight":1,"service":{"name":"default/postgres","port":5432}}]}`), &diags)
		if diags.HasError() {
			t.Fatalf("SetValue() returned diagnostics: %v", diags)
		}
		if len(m.Backends) != 1 {
			t.Fatalf("len(Backends) = %d, want 1", len(m.Backends))
		}
		if m.Backends[0].Service.IsNull() {
			t.Fatal("Service is null, want the service backend")
		}
		if got, want := m.Backends[0].Service.ValueString(), `{"name":"default/postgres","port":5432}`; got != want {
			t.Errorf("Service = %s, want %s", got, want)
		}
	})

	t.Run("invalid JSON is reported without the body", func(t *testing.T) {
		m := newModel()
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

	t.Run("invalid backend is reported without the body", func(t *testing.T) {
		m := newModel()
		var diags diag.Diagnostics
		m.SetValue(context.Background(), []byte(`{"name":"r","backends":[{"weight":"heavy"}]}`), &diags)
		if !diags.HasError() {
			t.Fatal("SetValue() returned no diagnostics, want a backend decode error")
		}
		for _, diagnostic := range diags.Errors() {
			if strings.Contains(diagnostic.Detail(), "heavy") {
				t.Errorf("diagnostic leaks the response body: %q", diagnostic.Detail())
			}
		}
	})
}
