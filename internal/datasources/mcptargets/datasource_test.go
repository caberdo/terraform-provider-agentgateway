package mcptargets

import (
	"context"
	"testing"

	"github.com/caberdo/terraform-provider-agentgateway/internal/agentgatewaytest"
	"github.com/caberdo/terraform-provider-agentgateway/internal/client"
	"github.com/caberdo/terraform-provider-agentgateway/internal/configresource"
	"github.com/caberdo/terraform-provider-agentgateway/internal/providerdata"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func TestMCPTargetsSortedByID(t *testing.T) {
	t.Parallel()

	c := seedTargets(t,
		`{"name":"zeta","mcp":{"host":"zeta.local"}}`,
		`{"name":"alpha","mcp":{"host":"alpha.local"}}`,
		`{"name":"mike","mcp":{"host":"mike.local"}}`,
	)

	state := readTargets(t, c, nil)

	entries := listEntries(t, state.Targets)
	if len(entries) != 3 {
		t.Fatalf("expected 3 targets, got %d", len(entries))
	}
	want := []string{"alpha", "mike", "zeta"}
	for i, name := range want {
		if entries[i].Name.ValueString() != name {
			t.Errorf("entry %d: expected name %q, got %q", i, name, entries[i].Name.ValueString())
		}
	}
	if state.ID.ValueString() != "mcp.target:" {
		t.Errorf("expected id mcp.target:, got %q", state.ID.ValueString())
	}
}

func TestMCPTargetsNamePrefixFilter(t *testing.T) {
	t.Parallel()

	c := seedTargets(t,
		`{"name":"team-alpha","mcp":{"host":"alpha.local"}}`,
		`{"name":"team-beta","mcp":{"host":"beta.local"}}`,
		`{"name":"other","mcp":{"host":"other.local"}}`,
	)

	prefix := "team-"
	state := readTargets(t, c, &prefix)

	entries := listEntries(t, state.Targets)
	if len(entries) != 2 {
		t.Fatalf("expected 2 filtered targets, got %d", len(entries))
	}
	if entries[0].Name.ValueString() != "team-alpha" || entries[1].Name.ValueString() != "team-beta" {
		t.Errorf("unexpected filtered order: %q, %q", entries[0].Name.ValueString(), entries[1].Name.ValueString())
	}
	if state.ID.ValueString() != "mcp.target:team-" {
		t.Errorf("expected id mcp.target:team-, got %q", state.ID.ValueString())
	}
}

func TestMCPTargetsDecodesTransports(t *testing.T) {
	t.Parallel()

	c := seedTargets(t,
		`{"name":"mcp-one","mcp":{"host":"localhost","port":8080,"path":"/mcp"},"unknownField":{"nested":true}}`,
		`{"name":"sse-one","sse":{"host":"sse.local","path":"/sse"}}`,
		`{"name":"stdio-one","stdio":{"cmd":"npx","args":["server"]}}`,
		`{"name":"openapi-one","openapi":{"host":"api.local","port":9000,"path":"/spec"}}`,
	)

	state := readTargets(t, c, nil)
	entries := listEntries(t, state.Targets)
	if len(entries) != 4 {
		t.Fatalf("expected 4 targets, got %d", len(entries))
	}

	want := map[string]struct{ transport, host, path string }{
		"mcp-one":     {"mcp", "localhost", "/mcp"},
		"openapi-one": {"openapi", "api.local", "/spec"},
		"sse-one":     {"sse", "sse.local", "/sse"},
		"stdio-one":   {"stdio", "", ""},
	}
	for _, entry := range entries {
		expected, ok := want[entry.Name.ValueString()]
		if !ok {
			t.Errorf("unexpected target %q", entry.Name.ValueString())
			continue
		}
		if entry.Transport.ValueString() != expected.transport {
			t.Errorf("%s: expected transport %q, got %q", entry.Name.ValueString(), expected.transport, entry.Transport.ValueString())
		}
		if entry.Host.ValueString() != expected.host {
			t.Errorf("%s: expected host %q, got %q", entry.Name.ValueString(), expected.host, entry.Host.ValueString())
		}
		if entry.Path.ValueString() != expected.path {
			t.Errorf("%s: expected path %q, got %q", entry.Name.ValueString(), expected.path, entry.Path.ValueString())
		}
	}
}

func seedTargets(t *testing.T, values ...string) client.Client {
	t.Helper()
	_, c := agentgatewaytest.New(t)
	for _, value := range values {
		if _, err := c.Create(context.Background(), configresource.KindMCPTarget, []byte(value)); err != nil {
			t.Fatalf("seeding target: %v", err)
		}
	}
	return c
}

func readTargets(t *testing.T, c client.Client, prefix *string) readModel {
	t.Helper()

	ds := New()
	configureDataSource(t, ds, c)
	s := schemaOf(t, ds)

	var resp datasource.ReadResponse
	resp.State = emptyState(t, s)
	ds.Read(context.Background(), requestFor(t, s, prefix), &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("read failed: %v", resp.Diagnostics)
	}

	var state readModel
	if err := resp.State.Get(context.Background(), &state); err != nil {
		t.Fatalf("reading state: %v", err)
	}
	return state
}

func listEntries(t *testing.T, list types.List) []targetEntry {
	t.Helper()
	if list.IsNull() || list.IsUnknown() {
		t.Fatal("expected a populated targets list")
	}
	var entries []targetEntry
	if diags := list.ElementsAs(context.Background(), &entries, false); diags.HasError() {
		t.Fatalf("decoding targets: %v", diags)
	}
	return entries
}

func configureDataSource(t *testing.T, ds datasource.DataSource, c client.Client) {
	t.Helper()
	cw, ok := ds.(datasource.DataSourceWithConfigure)
	if !ok {
		t.Fatal("data source does not implement DataSourceWithConfigure")
	}
	var resp datasource.ConfigureResponse
	cw.Configure(context.Background(), datasource.ConfigureRequest{
		ProviderData: &providerdata.Data{Client: c},
	}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("configure failed: %v", resp.Diagnostics)
	}
}

func schemaOf(t *testing.T, ds datasource.DataSource) datasourceschema.Schema {
	t.Helper()
	var resp datasource.SchemaResponse
	ds.Schema(context.Background(), datasource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema failed: %v", resp.Diagnostics)
	}
	return resp.Schema
}

func requestFor(t *testing.T, s datasourceschema.Schema, prefix *string) datasource.ReadRequest {
	t.Helper()
	objType, ok := s.Type().TerraformType(context.Background()).(tftypes.Object)
	if !ok {
		t.Fatal("data source schema is not an object type")
	}
	values := make(map[string]tftypes.Value, len(objType.AttributeTypes))
	for name, attrType := range objType.AttributeTypes {
		values[name] = tftypes.NewValue(attrType, nil)
	}
	if prefix != nil {
		values["name_prefix"] = tftypes.NewValue(tftypes.String, *prefix)
	}
	return datasource.ReadRequest{Config: tfsdk.Config{
		Raw:    tftypes.NewValue(objType, values),
		Schema: s,
	}}
}

func emptyState(t *testing.T, s datasourceschema.Schema) tfsdk.State {
	t.Helper()
	return tfsdk.State{
		Raw:    tftypes.NewValue(s.Type().TerraformType(context.Background()), nil),
		Schema: s,
	}
}
