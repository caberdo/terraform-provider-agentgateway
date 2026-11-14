package llmmodels

import (
	"context"
	"strings"
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

// TestLLMModelsSortedByName proves the list is deterministic. Model ids
// default to the name, so ordering by id is ordering by name for the common
// case (spec §8: every list data source sorts by id).
func TestLLMModelsSortedByName(t *testing.T) {
	t.Parallel()

	c := seedModels(t,
		`{"name":"zeta","provider":"openAI","params":{"model":"gpt-4o"}}`,
		`{"name":"alpha","provider":"openAI","params":{"model":"gpt-4o-mini"}}`,
		`{"name":"mike","provider":"openAI","params":{"model":"o3"}}`,
	)

	state := readModels(t, c, nil)

	entries := listEntries(t, state.Models)
	if len(entries) != 3 {
		t.Fatalf("expected 3 models, got %d", len(entries))
	}
	want := []string{"alpha", "mike", "zeta"}
	for i, name := range want {
		if entries[i].Name.ValueString() != name {
			t.Errorf("entry %d: expected name %q, got %q", i, name, entries[i].Name.ValueString())
		}
		if entries[i].ID.ValueString() != name {
			t.Errorf("entry %d: expected id %q, got %q", i, name, entries[i].ID.ValueString())
		}
	}
	if state.ID.ValueString() != "llm.model:" {
		t.Errorf("expected id llm.model:, got %q", state.ID.ValueString())
	}
}

func TestLLMModelsNamePrefixFilter(t *testing.T) {
	t.Parallel()

	c := seedModels(t,
		`{"name":"team-alpha","provider":"openAI"}`,
		`{"name":"team-beta","provider":"openAI"}`,
		`{"name":"other","provider":"openAI"}`,
	)

	prefix := "team-"
	state := readModels(t, c, &prefix)

	entries := listEntries(t, state.Models)
	if len(entries) != 2 {
		t.Fatalf("expected 2 filtered models, got %d", len(entries))
	}
	if entries[0].Name.ValueString() != "team-alpha" || entries[1].Name.ValueString() != "team-beta" {
		t.Errorf("unexpected filtered order: %q, %q", entries[0].Name.ValueString(), entries[1].Name.ValueString())
	}
	if state.ID.ValueString() != "llm.model:team-" {
		t.Errorf("expected id llm.model:team-, got %q", state.ID.ValueString())
	}
}

func TestLLMModelsDecodesFieldsAndIgnoresUnknown(t *testing.T) {
	t.Parallel()

	c := seedModels(t,
		`{"name":"team-alpha","displayName":"Team Alpha","visibility":"internal","provider":"openAI","params":{"model":"gpt-4o"},"unknownField":{"nested":true}}`,
	)

	state := readModels(t, c, nil)
	entries := listEntries(t, state.Models)
	if len(entries) != 1 {
		t.Fatalf("expected 1 model, got %d", len(entries))
	}

	entry := entries[0]
	if entry.ID.ValueString() != "team-alpha" {
		t.Errorf("expected id team-alpha, got %q", entry.ID.ValueString())
	}
	if entry.DisplayName.ValueString() != "Team Alpha" {
		t.Errorf("expected display_name Team Alpha, got %q", entry.DisplayName.ValueString())
	}
	if entry.Provider.ValueString() != "openAI" {
		t.Errorf("expected provider openAI, got %q", entry.Provider.ValueString())
	}
	if entry.Model.ValueString() != "gpt-4o" {
		t.Errorf("expected model gpt-4o, got %q", entry.Model.ValueString())
	}
	if entry.Visibility.ValueString() != "internal" {
		t.Errorf("expected visibility internal, got %q", entry.Visibility.ValueString())
	}
}

func TestLLMModelsDecodeFailureNamesResource(t *testing.T) {
	t.Parallel()

	ds := &DataSource{client: stubListClient{resources: []configresource.Resource{
		{Kind: configresource.KindLLMModel, ID: "broken", Value: []byte(`not-json`)},
	}}}
	s := schemaOf(t, ds)

	var resp datasource.ReadResponse
	resp.State = emptyState(t, s)
	ds.Read(context.Background(), requestFor(t, s, nil), &resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a decode error diagnostic")
	}
	detail := resp.Diagnostics.Errors()[0].Detail()
	if !strings.Contains(detail, "llm.model") {
		t.Errorf("expected the diagnostic to name the kind, got %q", detail)
	}
	if strings.Contains(detail, "not-json") {
		t.Errorf("the diagnostic must not include the value body, got %q", detail)
	}
}

// stubListClient returns fixed resources without a gateway.
type stubListClient struct {
	resources []configresource.Resource
}

func (c stubListClient) List(context.Context, configresource.Kind) ([]configresource.Resource, error) {
	return c.resources, nil
}

func seedModels(t *testing.T, values ...string) client.Client {
	t.Helper()
	_, c := agentgatewaytest.New(t)
	for _, value := range values {
		if _, err := c.Create(context.Background(), configresource.KindLLMModel, []byte(value)); err != nil {
			t.Fatalf("seeding model: %v", err)
		}
	}
	return c
}

func readModels(t *testing.T, c client.Client, prefix *string) readModel {
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

func listEntries(t *testing.T, list types.List) []modelEntry {
	t.Helper()
	if list.IsNull() || list.IsUnknown() {
		t.Fatal("expected a populated models list")
	}
	var entries []modelEntry
	if diags := list.ElementsAs(context.Background(), &entries, false); diags.HasError() {
		t.Fatalf("decoding models: %v", diags)
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
