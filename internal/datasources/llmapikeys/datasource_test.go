package llmapikeys

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

func TestAPIKeysSortedByID(t *testing.T) {
	t.Parallel()

	c := seedKeys(t,
		`{"key":"sk-one","metadata":{"name":"one"}}`,
		`{"key":"sk-two","metadata":{"name":"two"}}`,
		`{"key":"sk-three","metadata":{"name":"three"}}`,
	)

	state := readKeys(t, c, nil)
	entries := listEntries(t, state.Keys)
	if len(entries) != 3 {
		t.Fatalf("expected 3 keys, got %d", len(entries))
	}
	for i := 1; i < len(entries); i++ {
		if entries[i-1].ID.ValueString() >= entries[i].ID.ValueString() {
			t.Errorf("keys are not sorted by id: %q then %q", entries[i-1].ID.ValueString(), entries[i].ID.ValueString())
		}
	}
	if state.ID.ValueString() != "llm.apiKey:" {
		t.Errorf("expected id llm.apiKey:, got %q", state.ID.ValueString())
	}
}

func TestAPIKeysNamePrefixFilter(t *testing.T) {
	t.Parallel()

	c := seedKeys(t,
		`{"key":"sk-one","metadata":{"name":"one"}}`,
		`{"key":"sk-two","metadata":{"name":"two"}}`,
	)

	all := readKeys(t, c, nil)
	entries := listEntries(t, all.Keys)
	if len(entries) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(entries))
	}
	prefix := entries[1].ID.ValueString()
	filtered := readKeys(t, c, &prefix)

	filteredEntries := listEntries(t, filtered.Keys)
	if len(filteredEntries) != 1 {
		t.Fatalf("expected 1 filtered key, got %d", len(filteredEntries))
	}
	if filteredEntries[0].ID.ValueString() != prefix {
		t.Errorf("expected key %q, got %q", prefix, filteredEntries[0].ID.ValueString())
	}
	if filtered.ID.ValueString() != "llm.apiKey:"+prefix {
		t.Errorf("expected id llm.apiKey:%s, got %q", prefix, filtered.ID.ValueString())
	}
}

func TestAPIKeysExposeMetadataWithoutKeyMaterial(t *testing.T) {
	t.Parallel()

	c := seedKeys(t,
		`{"key":"sk-secret-material","metadata":{"name":"team-a","user":"alice","group":"engineering"},"allowedModels":["gpt-4o","claude-*"]}`,
	)

	state := readKeys(t, c, nil)
	entries := listEntries(t, state.Keys)
	if len(entries) != 1 {
		t.Fatalf("expected 1 key, got %d", len(entries))
	}

	entry := entries[0]
	if entry.Name.ValueString() != "team-a" {
		t.Errorf("expected name team-a, got %q", entry.Name.ValueString())
	}
	if entry.User.ValueString() != "alice" {
		t.Errorf("expected user alice, got %q", entry.User.ValueString())
	}
	if entry.Group.ValueString() != "engineering" {
		t.Errorf("expected group engineering, got %q", entry.Group.ValueString())
	}
	if entry.CreatedAt.ValueString() != "1970-01-01T00:00:00Z" {
		t.Errorf("expected created_at 1970-01-01T00:00:00Z, got %q", entry.CreatedAt.ValueString())
	}

	var models []string
	if diags := entry.AllowedModels.ElementsAs(context.Background(), &models, false); diags.HasError() {
		t.Fatalf("decoding allowed_models: %v", diags)
	}
	if len(models) != 2 || models[0] != "gpt-4o" || models[1] != "claude-*" {
		t.Errorf("unexpected allowed_models: %v", models)
	}
}

// TestAPIKeysNeverExposeKeyMaterial is the security gate for spec §8: the
// schema has no key or key_hash attribute at any level.
func TestAPIKeysNeverExposeKeyMaterial(t *testing.T) {
	t.Parallel()

	s := schemaOf(t, New())

	for name := range s.Attributes {
		if forbidden(name) {
			t.Errorf("top-level attribute %q must not exist", name)
		}
	}

	keys, ok := s.Attributes["keys"].(datasourceschema.ListNestedAttribute)
	if !ok {
		t.Fatalf("expected a keys list nested attribute, got %T", s.Attributes["keys"])
	}
	for name := range keys.NestedObject.Attributes {
		if forbidden(name) {
			t.Errorf("keys attribute %q must not exist", name)
		}
	}
}

// TestAPIKeysAllowedModelsNullAndEmpty pins spec C13: an omitted allowedModels
// is null (no constraint), while an empty list denies every model.
func TestAPIKeysAllowedModelsNullAndEmpty(t *testing.T) {
	t.Parallel()

	c := seedKeys(t,
		`{"key":"sk-unconstrained","metadata":{"name":"unconstrained"}}`,
		`{"key":"sk-denied","metadata":{"name":"denied"},"allowedModels":[]}`,
	)

	state := readKeys(t, c, nil)
	entries := listEntries(t, state.Keys)
	if len(entries) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(entries))
	}

	byName := map[string]keyEntry{}
	for _, entry := range entries {
		byName[entry.Name.ValueString()] = entry
	}

	if !byName["unconstrained"].AllowedModels.IsNull() {
		t.Errorf("expected omitted allowed_models to be null, got %v", byName["unconstrained"].AllowedModels)
	}
	denied := byName["denied"].AllowedModels
	if denied.IsNull() || denied.IsUnknown() {
		t.Fatalf("expected an empty allowed_models list, got %v", denied)
	}
	if len(denied.Elements()) != 0 {
		t.Errorf("expected an empty allowed_models list, got %v", denied.Elements())
	}
}

func forbidden(name string) bool {
	return name == "key" || name == "key_hash" || strings.EqualFold(name, "keyHash")
}

func seedKeys(t *testing.T, values ...string) client.Client {
	t.Helper()
	_, c := agentgatewaytest.New(t)
	for _, value := range values {
		if _, err := c.Create(context.Background(), configresource.KindLLMAPIKey, []byte(value)); err != nil {
			t.Fatalf("seeding api key: %v", err)
		}
	}
	return c
}

func readKeys(t *testing.T, c client.Client, prefix *string) readModel {
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

func listEntries(t *testing.T, list types.List) []keyEntry {
	t.Helper()
	if list.IsNull() || list.IsUnknown() {
		t.Fatal("expected a populated keys list")
	}
	var entries []keyEntry
	if diags := list.ElementsAs(context.Background(), &entries, false); diags.HasError() {
		t.Fatalf("decoding keys: %v", diags)
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
