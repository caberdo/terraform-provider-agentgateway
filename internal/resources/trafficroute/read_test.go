package trafficroute

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestTrafficRouteSetValue(t *testing.T) {
	t.Run("full value round-trips", func(t *testing.T) {
		value := `{
			"name": "r",
			"gateways": ["default", "internal/listener"],
			"namespace": "default",
			"ruleName": "api-rule",
			"hostnames": ["example.com", "*.example.org"],
			"matches": [
				{
					"path": {"pathPrefix": "/api"},
					"method": "GET",
					"headers": [
						{"name": "x-tenant", "value": {"exact": "acme"}},
						{"name": "x-trace", "value": {"regex": "^[a-f0-9]+$"}}
					],
					"query": [
						{"name": "page", "value": {"exact": "1"}}
					]
				}
			],
			"backends": [
				{"weight": 2, "host": "127.0.0.1:8080"},
				{"weight": 1, "backend": "svc"},
				{"weight": 1, "service": {"name": "svc", "port": 8080}},
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

		if len(m.Matches) != 1 {
			t.Fatalf("len(Matches) = %d, want 1", len(m.Matches))
		}
		match := m.Matches[0]
		if got := match.Method.ValueString(); got != "GET" {
			t.Errorf("match Method = %q, want GET", got)
		}
		if len(match.Path) != 1 || match.Path[0].Prefix.ValueString() != "/api" {
			t.Errorf("match Path = %+v, want prefix /api", match.Path)
		}
		if len(match.Headers) != 2 {
			t.Fatalf("len(match.Headers) = %d, want 2", len(match.Headers))
		}
		if match.Headers[0].Name.ValueString() != "x-tenant" || match.Headers[0].Exact.ValueString() != "acme" {
			t.Errorf("match header = %+v, want x-tenant=acme", match.Headers[0])
		}
		if match.Headers[1].Name.ValueString() != "x-trace" || match.Headers[1].Regex.ValueString() != "^[a-f0-9]+$" {
			t.Errorf("match header = %+v, want x-trace regex", match.Headers[1])
		}
		if len(match.Query) != 1 || match.Query[0].Name.ValueString() != "page" || match.Query[0].Exact.ValueString() != "1" {
			t.Errorf("match query = %+v, want page=1", match.Query)
		}

		if len(m.Backends) != 6 {
			t.Fatalf("len(Backends) = %d, want 6", len(m.Backends))
		}
		if got := m.Backends[0].Weight.ValueInt64(); got != 2 {
			t.Errorf("backend[0] Weight = %d, want 2", got)
		}
		if got := m.Backends[0].Host.ValueString(); got != "127.0.0.1:8080" {
			t.Errorf("backend[0] Host = %q, want 127.0.0.1:8080", got)
		}
		if got := m.Backends[1].Backend.ValueString(); got != "svc" {
			t.Errorf("backend[1] Backend = %q, want svc", got)
		}

		jsonFields := []struct {
			name string
			got  jsontypes.Normalized
			want string
		}{
			{"backend[2].Service", m.Backends[2].Service, `{"name": "svc", "port": 8080}`},
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
		if len(m.Matches) != 0 {
			t.Errorf("len(Matches) = %d, want 0", len(m.Matches))
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
		m.Matches = []matchModel{{Path: []pathModel{{Prefix: types.StringValue("/old")}}}}
		m.Backends = []backendModel{{Host: types.StringValue("old:1")}}
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
		if len(m.Matches) != 1 || m.Matches[0].Path[0].Prefix.ValueString() != "/old" {
			t.Errorf("Matches = %+v, want the state value preserved", m.Matches)
		}
		if len(m.Backends) != 1 || m.Backends[0].Host.ValueString() != "old:1" {
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
		m.Matches = []matchModel{{Path: []pathModel{{Prefix: types.StringValue("/old")}}}}
		m.Backends = []backendModel{{Host: types.StringValue("old:1")}}

		var diags diag.Diagnostics
		m.SetValue(context.Background(), []byte(`{"name":"r","gateways":[],"hostnames":[],"matches":[],"backends":[]}`), &diags)
		if diags.HasError() {
			t.Fatalf("SetValue() returned diagnostics: %v", diags)
		}

		if len(m.Gateways.Elements()) != 0 {
			t.Errorf("Gateways = %v, want empty", m.Gateways)
		}
		if len(m.Hostnames.Elements()) != 0 {
			t.Errorf("Hostnames = %v, want empty", m.Hostnames)
		}
		if len(m.Matches) != 0 {
			t.Errorf("len(Matches) = %d, want 0", len(m.Matches))
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
		m.SetValue(context.Background(), []byte(`{"name":"r","backends":[{"weight":4,"host":"h:1"}]}`), &diags)
		if diags.HasError() {
			t.Fatalf("SetValue() returned diagnostics: %v", diags)
		}
		if len(m.Backends) != 1 {
			t.Fatalf("len(Backends) = %d, want 1", len(m.Backends))
		}
		if got := m.Backends[0].Weight.ValueInt64(); got != 4 {
			t.Errorf("Weight = %d, want 4", got)
		}
		if got := m.Backends[0].Host.ValueString(); got != "h:1" {
			t.Errorf("Host = %q, want h:1", got)
		}
	})

	t.Run("legacy object host and backend are tolerated as absent", func(t *testing.T) {
		m := newModel()
		var diags diag.Diagnostics
		m.SetValue(context.Background(), []byte(`{"name":"r","backends":[{"host":{"host":"h","port":1},"backend":{"name":"svc","namespace":"default"}}]}`), &diags)
		if diags.HasError() {
			t.Fatalf("SetValue() returned diagnostics: %v", diags)
		}
		if len(m.Backends) != 1 {
			t.Fatalf("len(Backends) = %d, want 1", len(m.Backends))
		}
		if !m.Backends[0].Host.IsNull() {
			t.Errorf("Host = %q, want null", m.Backends[0].Host.ValueString())
		}
		if !m.Backends[0].Backend.IsNull() {
			t.Errorf("Backend = %q, want null", m.Backends[0].Backend.ValueString())
		}
		// A known key in an unexpected shape is not re-labelled as generic json.
		if !m.Backends[0].JSON.IsNull() {
			t.Errorf("JSON = %s, want null", m.Backends[0].JSON.ValueString())
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
