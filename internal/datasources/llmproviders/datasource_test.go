package llmproviders

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

func TestLLMProvidersSortedByID(t *testing.T) {
	t.Parallel()

	c := seedProviders(t,
		`{"name":"zeta","provider":"openAI","params":{"model":"gpt-4o","baseUrl":"https://api.openai.com"}}`,
		`{"name":"alpha","provider":"deepseek","params":{"model":"deepseek-chat"}}`,
		`{"name":"mike","provider":"anthropic","params":{"model":"claude-sonnet-4"}}`,
	)

	state := readProviders(t, c, nil)

	entries := listEntries(t, state.Providers)
	if len(entries) != 3 {
		t.Fatalf("expected 3 providers, got %d", len(entries))
	}
	want := []string{"alpha", "mike", "zeta"}
	for i, name := range want {
		if entries[i].Name.ValueString() != name {
			t.Errorf("entry %d: expected name %q, got %q", i, name, entries[i].Name.ValueString())
		}
	}
	if state.ID.ValueString() != "llm.provider:" {
		t.Errorf("expected id llm.provider:, got %q", state.ID.ValueString())
	}
}

func TestLLMProvidersNamePrefixFilter(t *testing.T) {
	t.Parallel()

	c := seedProviders(t,
		`{"name":"team-alpha","provider":"openAI"}`,
		`{"name":"team-beta","provider":"openAI"}`,
		`{"name":"other","provider":"openAI"}`,
	)

	prefix := "team-"
	state := readProviders(t, c, &prefix)

	entries := listEntries(t, state.Providers)
	if len(entries) != 2 {
		t.Fatalf("expected 2 filtered providers, got %d", len(entries))
	}
	if entries[0].Name.ValueString() != "team-alpha" || entries[1].Name.ValueString() != "team-beta" {
		t.Errorf("unexpected filtered order: %q, %q", entries[0].Name.ValueString(), entries[1].Name.ValueString())
	}
	if state.ID.ValueString() != "llm.provider:team-" {
		t.Errorf("expected id llm.provider:team-, got %q", state.ID.ValueString())
	}
}

func TestLLMProvidersDecodesFieldsAndIgnoresUnknown(t *testing.T) {
	t.Parallel()

	c := seedProviders(t,
		`{"name":"team-alpha","provider":"openAI","params":{"model":"gpt-4o","baseUrl":"https://api.openai.com","apiKey":"$OPENAI_API_KEY"},"unknownField":true}`,
	)

	state := readProviders(t, c, nil)
	entries := listEntries(t, state.Providers)
	if len(entries) != 1 {
		t.Fatalf("expected 1 provider, got %d", len(entries))
	}

	entry := entries[0]
	if entry.Name.ValueString() != "team-alpha" {
		t.Errorf("expected name team-alpha, got %q", entry.Name.ValueString())
	}
	if entry.Provider.ValueString() != "openAI" {
		t.Errorf("expected provider openAI, got %q", entry.Provider.ValueString())
	}
	if entry.Model.ValueString() != "gpt-4o" {
		t.Errorf("expected model gpt-4o, got %q", entry.Model.ValueString())
	}
	if entry.BaseURL.ValueString() != "https://api.openai.com" {
		t.Errorf("expected base_url https://api.openai.com, got %q", entry.BaseURL.ValueString())
	}
}

func seedProviders(t *testing.T, values ...string) client.Client {
	t.Helper()
	_, c := agentgatewaytest.New(t)
	for _, value := range values {
		if _, err := c.Create(context.Background(), configresource.KindLLMProvider, []byte(value)); err != nil {
			t.Fatalf("seeding provider: %v", err)
		}
	}
	return c
}

func readProviders(t *testing.T, c client.Client, prefix *string) readModel {
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

func listEntries(t *testing.T, list types.List) []providerEntry {
	t.Helper()
	if list.IsNull() || list.IsUnknown() {
		t.Fatal("expected a populated providers list")
	}
	var entries []providerEntry
	if diags := list.ElementsAs(context.Background(), &entries, false); diags.HasError() {
		t.Fatalf("decoding providers: %v", diags)
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
