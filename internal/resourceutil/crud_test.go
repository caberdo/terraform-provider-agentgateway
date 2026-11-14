package resourceutil_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/caberdo/terraform-provider-agentgateway/internal/agentgatewaytest"
	"github.com/caberdo/terraform-provider-agentgateway/internal/client"
	"github.com/caberdo/terraform-provider-agentgateway/internal/configresource"
	"github.com/caberdo/terraform-provider-agentgateway/internal/providerdata"
	"github.com/caberdo/terraform-provider-agentgateway/internal/resourceutil"
	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// crudTestModel is a minimal KindResource implementation used to exercise the
// CRUD harness.
type crudTestModel struct {
	kind      configresource.Kind
	ID_       types.String         `tfsdk:"id"`
	Revision  types.Int64          `tfsdk:"revision"`
	Name      types.String         `tfsdk:"name"`
	ValueJSON jsontypes.Normalized `tfsdk:"value_json"`
}

func (m *crudTestModel) Kind() configresource.Kind { return m.kind }

func (m *crudTestModel) ID() string { return m.Name.ValueString() }

func (m *crudTestModel) Value(_ context.Context, _ *diag.Diagnostics) json.RawMessage {
	if !m.ValueJSON.IsNull() {
		return json.RawMessage(m.ValueJSON.ValueString())
	}
	b, _ := json.Marshal(map[string]string{"name": m.Name.ValueString()})
	return b
}

func (m *crudTestModel) SetValue(_ context.Context, value json.RawMessage, _ *diag.Diagnostics) {
	m.ValueJSON = jsontypes.NewNormalizedValue(string(value))
}

func (m *crudTestModel) SetServerMeta(id string, revision int64) {
	m.ID_ = types.StringValue(id)
	m.Revision = types.Int64Value(revision)
}

func crudTestSchema() schema.Schema {
	return schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id":       schema.StringAttribute{Computed: true},
			"revision": schema.Int64Attribute{Computed: true},
			"name":     schema.StringAttribute{Required: true},
			"value_json": schema.StringAttribute{
				Optional:   true,
				CustomType: jsontypes.NormalizedType{},
			},
		},
	}
}

func crudAttrTypes() map[string]tftypes.Type {
	return map[string]tftypes.Type{
		"id":         tftypes.String,
		"revision":   tftypes.Number,
		"name":       tftypes.String,
		"value_json": tftypes.String,
	}
}

func newCRUDSpec(kind configresource.Kind) resourceutil.Spec {
	return resourceutil.Spec{
		TypeName: "agentgateway_test_resource",
		Kind:     kind,
		Schema:   crudTestSchema(),
		New: func() resourceutil.KindResource {
			return &crudTestModel{kind: kind}
		},
	}
}

func newPlan(t *testing.T, s schema.Schema, values map[string]tftypes.Value) tfsdk.Plan {
	t.Helper()
	return tfsdk.Plan{
		Raw:    tftypes.NewValue(tftypes.Object{AttributeTypes: crudAttrTypes()}, values),
		Schema: s,
	}
}

func newConfig(t *testing.T, s schema.Schema, values map[string]tftypes.Value) tfsdk.Config {
	t.Helper()
	return tfsdk.Config{
		Raw:    tftypes.NewValue(tftypes.Object{AttributeTypes: crudAttrTypes()}, values),
		Schema: s,
	}
}

func newState(t *testing.T, s schema.Schema, values map[string]tftypes.Value) tfsdk.State {
	t.Helper()
	return tfsdk.State{
		Raw:    tftypes.NewValue(tftypes.Object{AttributeTypes: crudAttrTypes()}, values),
		Schema: s,
	}
}

func emptyResponseState(t *testing.T, s schema.Schema) tfsdk.State {
	t.Helper()
	return tfsdk.State{
		Raw:    tftypes.NewValue(tftypes.Object{AttributeTypes: crudAttrTypes()}, nil),
		Schema: s,
	}
}

func modelValues(name string, id *string, revision *int64, valueJSON *string) map[string]tftypes.Value {
	values := map[string]tftypes.Value{
		"name":       tftypes.NewValue(tftypes.String, name),
		"id":         tftypes.NewValue(tftypes.String, nil),
		"revision":   tftypes.NewValue(tftypes.Number, nil),
		"value_json": tftypes.NewValue(tftypes.String, nil),
	}
	if id != nil {
		values["id"] = tftypes.NewValue(tftypes.String, *id)
	}
	if revision != nil {
		values["revision"] = tftypes.NewValue(tftypes.Number, float64(*revision))
	}
	if valueJSON != nil {
		values["value_json"] = tftypes.NewValue(tftypes.String, *valueJSON)
	}
	return values
}

func configureResource(t *testing.T, r resource.Resource, c client.Client, storeMode string) {
	t.Helper()
	cw, ok := r.(resource.ResourceWithConfigure)
	if !ok {
		t.Fatal("resource does not implement ResourceWithConfigure")
	}
	var resp resource.ConfigureResponse
	cw.Configure(context.Background(), resource.ConfigureRequest{
		ProviderData: &providerdata.Data{
			Client:    c,
			Timeout:   0,
			StoreMode: storeMode,
		},
	}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("configure failed: %v", resp.Diagnostics)
	}
}

func lastRequestPath(t *testing.T, srv *agentgatewaytest.Server, method string) string {
	t.Helper()
	for i := len(srv.Requests()) - 1; i >= 0; i-- {
		req := srv.Requests()[i]
		if req.Method == method {
			return req.Path
		}
	}
	t.Fatalf("no %s request found", method)
	return ""
}

func TestCreateUsesCollectionForNonPolicy(t *testing.T) {
	srv, c := agentgatewaytest.New(t)
	r := resourceutil.CRUD(newCRUDSpec(configresource.KindLLMProvider))
	configureResource(t, r, c, "hybrid")

	values := modelValues("provider-one", nil, nil, nil)
	var resp resource.CreateResponse
	resp.State = emptyResponseState(t, crudTestSchema())
	r.Create(context.Background(), resource.CreateRequest{
		Plan:   newPlan(t, crudTestSchema(), values),
		Config: newConfig(t, crudTestSchema(), values),
	}, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}

	var state crudTestModel
	if err := resp.State.Get(context.Background(), &state); err != nil {
		t.Fatalf("reading state: %v", err)
	}
	if state.ID_.ValueString() != "provider-one" {
		t.Errorf("expected id provider-one, got %q", state.ID_.ValueString())
	}
	if state.Revision.ValueInt64() != 1 {
		t.Errorf("expected revision 1, got %d", state.Revision.ValueInt64())
	}

	path := lastRequestPath(t, srv, "PUT")
	if path != "/api/config/resources/llm.provider" {
		t.Errorf("expected collection path, got %q", path)
	}
}

func TestCreatePolicyUsesUpdatePath(t *testing.T) {
	srv, c := agentgatewaytest.New(t)
	r := resourceutil.CRUD(newCRUDSpec(configresource.KindLLMPolicy))
	configureResource(t, r, c, "hybrid")

	values := modelValues("cors", nil, nil, nil)
	var resp resource.CreateResponse
	resp.State = emptyResponseState(t, crudTestSchema())
	r.Create(context.Background(), resource.CreateRequest{
		Plan:   newPlan(t, crudTestSchema(), values),
		Config: newConfig(t, crudTestSchema(), values),
	}, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}

	var state crudTestModel
	if err := resp.State.Get(context.Background(), &state); err != nil {
		t.Fatalf("reading state: %v", err)
	}
	if state.ID_.ValueString() != "cors" {
		t.Errorf("expected id cors, got %q", state.ID_.ValueString())
	}

	path := lastRequestPath(t, srv, "PUT")
	if path != "/api/config/resources/llm.policy/cors" {
		t.Errorf("expected single-resource path, got %q", path)
	}
}

func TestCreateRefusesExistingID(t *testing.T) {
	srv, c := agentgatewaytest.New(t)

	// Seed an out-of-band resource.
	_, err := c.Create(context.Background(), configresource.KindLLMProvider, json.RawMessage(`{"name":"existing"}`))
	if err != nil {
		t.Fatalf("seeding resource: %v", err)
	}

	r := resourceutil.CRUD(newCRUDSpec(configresource.KindLLMProvider))
	configureResource(t, r, c, "hybrid")

	values := modelValues("existing", nil, nil, nil)
	var resp resource.CreateResponse
	resp.State = emptyResponseState(t, crudTestSchema())
	r.Create(context.Background(), resource.CreateRequest{
		Plan:   newPlan(t, crudTestSchema(), values),
		Config: newConfig(t, crudTestSchema(), values),
	}, &resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error diagnostic for existing id")
	}
	msg := resp.Diagnostics.Errors()[0].Detail()
	want := "llm.provider/existing already exists outside Terraform state; import it with terraform import <address> llm.provider/existing"
	if msg != want {
		t.Errorf("unexpected diagnostic detail:\n got: %q\nwant: %q", msg, want)
	}

	// Ensure no additional write was attempted.
	putCount := 0
	for _, req := range srv.Requests() {
		if req.Method == "PUT" {
			putCount++
		}
	}
	if putCount != 1 {
		t.Errorf("expected 1 PUT (the seed), got %d", putCount)
	}
}

func TestSingletonRejectedInFileMode(t *testing.T) {
	srv, c := agentgatewaytest.New(t)
	srv.SetStoreMode("file")

	r := resourceutil.CRUD(newCRUDSpec(configresource.KindLLMSettings))
	configureResource(t, r, c, "file")

	values := modelValues("default", nil, nil, nil)
	var resp resource.CreateResponse
	resp.State = emptyResponseState(t, crudTestSchema())
	r.Create(context.Background(), resource.CreateRequest{
		Plan:   newPlan(t, crudTestSchema(), values),
		Config: newConfig(t, crudTestSchema(), values),
	}, &resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error diagnostic for singleton in file mode")
	}
	msg := resp.Diagnostics.Errors()[0].Detail()
	if !strings.Contains(msg, "requires config.storage.mode = hybrid") {
		t.Errorf("expected hybrid-mode hint, got %q", msg)
	}
}

func TestReadMissingRemovesFromState(t *testing.T) {
	_, c := agentgatewaytest.New(t)
	r := resourceutil.CRUD(newCRUDSpec(configresource.KindLLMProvider))
	configureResource(t, r, c, "hybrid")

	id := "missing"
	rev := int64(1)
	reqState := newState(t, crudTestSchema(), modelValues("missing", &id, &rev, nil))
	var resp resource.ReadResponse
	resp.State = reqState
	r.Read(context.Background(), resource.ReadRequest{
		State: reqState,
	}, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Error("expected state to be removed (null) for missing resource")
	}
}

// renameClient is a hand-written fake that returns a renamed id on Update.
type renameClient struct {
	updateKind string
	updateID   string
}

func (c *renameClient) Runtime(context.Context) (client.RuntimeInfo, error) {
	return client.RuntimeInfo{}, nil
}

func (c *renameClient) List(context.Context, configresource.Kind) ([]configresource.Resource, error) {
	return nil, nil
}

func (c *renameClient) Create(context.Context, configresource.Kind, json.RawMessage) (configresource.Resource, error) {
	return configresource.Resource{}, nil
}

func (c *renameClient) Update(_ context.Context, kind configresource.Kind, id string, value json.RawMessage) (configresource.Resource, error) {
	c.updateKind = string(kind)
	c.updateID = id
	var body map[string]string
	_ = json.Unmarshal(value, &body)
	newID := body["name"]
	return configresource.Resource{Kind: kind, ID: newID, Value: value, Revision: 2}, nil
}

func (c *renameClient) Delete(context.Context, configresource.Kind, string) error {
	return nil
}

func (c *renameClient) EffectiveConfig(context.Context) (json.RawMessage, error) {
	return nil, nil
}

func TestRenameSendsOldID(t *testing.T) {
	fc := &renameClient{}
	r := resourceutil.CRUD(newCRUDSpec(configresource.KindLLMProvider))
	configureResource(t, r, fc, "hybrid")

	oldID := "old"
	oldRev := int64(1)
	stateValues := modelValues("old", &oldID, &oldRev, nil)
	planValues := modelValues("new", &oldID, &oldRev, nil)

	var resp resource.UpdateResponse
	resp.State = emptyResponseState(t, crudTestSchema())
	r.Update(context.Background(), resource.UpdateRequest{
		Plan:  newPlan(t, crudTestSchema(), planValues),
		State: newState(t, crudTestSchema(), stateValues),
	}, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}

	if fc.updateID != "old" {
		t.Errorf("expected update called with old id, got %q", fc.updateID)
	}

	var state crudTestModel
	if err := resp.State.Get(context.Background(), &state); err != nil {
		t.Fatalf("reading state: %v", err)
	}
	if state.ID_.ValueString() != "new" {
		t.Errorf("expected renamed id new, got %q", state.ID_.ValueString())
	}
}
