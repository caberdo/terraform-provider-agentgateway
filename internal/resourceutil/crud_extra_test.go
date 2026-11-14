package resourceutil_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/caberdo/terraform-provider-agentgateway/internal/agentgatewaytest"
	"github.com/caberdo/terraform-provider-agentgateway/internal/client"
	"github.com/caberdo/terraform-provider-agentgateway/internal/configresource"
	"github.com/caberdo/terraform-provider-agentgateway/internal/providerdata"
	"github.com/caberdo/terraform-provider-agentgateway/internal/resourceutil"
	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// apiKeyTestModel mirrors a server-generated-id resource: ID() is empty until
// the server answers.
type apiKeyTestModel struct {
	IDValue  types.String `tfsdk:"id"`
	Revision types.Int64  `tfsdk:"revision"`
	Key      types.String `tfsdk:"key"`
}

func (m *apiKeyTestModel) Kind() configresource.Kind { return configresource.KindLLMAPIKey }

func (m *apiKeyTestModel) ID() string { return "" }

func (m *apiKeyTestModel) Value(_ context.Context, _ *diag.Diagnostics) json.RawMessage {
	return json.RawMessage(`{"key":"` + m.Key.ValueString() + `"}`)
}

func (m *apiKeyTestModel) SetValue(_ context.Context, value json.RawMessage, diags *diag.Diagnostics) {
	var body struct {
		Key string `json:"key"`
	}
	if err := json.Unmarshal(value, &body); err != nil {
		diags.AddError("decode api key", err.Error())
		return
	}
	m.Key = types.StringValue(body.Key)
}

func (m *apiKeyTestModel) SetServerMeta(id string, revision int64) {
	m.IDValue = types.StringValue(id)
	m.Revision = types.Int64Value(revision)
}

func apiKeyTestSchema() schema.Schema {
	return schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id":       schema.StringAttribute{Computed: true},
			"revision": schema.Int64Attribute{Computed: true},
			"key":      schema.StringAttribute{Required: true, Sensitive: true},
		},
	}
}

func apiKeyTestAttrTypes() map[string]tftypes.Type {
	return map[string]tftypes.Type{
		"id":       tftypes.String,
		"revision": tftypes.Number,
		"key":      tftypes.String,
	}
}

func emptyStateWith(t *testing.T, s schema.Schema, attrTypes map[string]tftypes.Type) tfsdk.State {
	t.Helper()
	return tfsdk.State{
		Raw:    tftypes.NewValue(tftypes.Object{AttributeTypes: attrTypes}, nil),
		Schema: s,
	}
}

func TestCreateAPIKeySkipsExistenceCheck(t *testing.T) {
	srv, c := agentgatewaytest.New(t)
	r := resourceutil.CRUD(resourceutil.Spec{
		TypeName: "agentgateway_llm_api_key",
		Kind:     configresource.KindLLMAPIKey,
		Schema:   apiKeyTestSchema(),
		New:      func() resourceutil.KindResource { return &apiKeyTestModel{} },
	})
	configureResource(t, r, c, "hybrid")

	plan := tfsdk.Plan{
		Raw: tftypes.NewValue(tftypes.Object{AttributeTypes: apiKeyTestAttrTypes()}, map[string]tftypes.Value{
			"id":       tftypes.NewValue(tftypes.String, nil),
			"revision": tftypes.NewValue(tftypes.Number, nil),
			"key":      tftypes.NewValue(tftypes.String, "plaintext-key"),
		}),
		Schema: apiKeyTestSchema(),
	}

	var resp resource.CreateResponse
	resp.State = emptyStateWith(t, apiKeyTestSchema(), apiKeyTestAttrTypes())
	r.Create(context.Background(), resource.CreateRequest{Plan: plan}, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}

	var state apiKeyTestModel
	if err := resp.State.Get(context.Background(), &state); err != nil {
		t.Fatalf("reading state: %v", err)
	}
	if state.IDValue.ValueString() == "" {
		t.Error("expected the server-generated id to be stored")
	}
	if state.Revision.ValueInt64() != 1 {
		t.Errorf("expected revision 1, got %d", state.Revision.ValueInt64())
	}

	for _, req := range srv.Requests() {
		if req.Method == "GET" {
			t.Errorf("api key create must not run the pre-flight list, saw GET %s", req.Path)
		}
	}
	if path := lastRequestPath(t, srv, "PUT"); path != "/api/config/resources/llm.apiKey" {
		t.Errorf("expected collection path, got %q", path)
	}
}

func TestReadPopulatesStateFromServer(t *testing.T) {
	_, c := agentgatewaytest.New(t)
	seeded, err := c.Create(context.Background(), configresource.KindLLMProvider, json.RawMessage(`{"name":"p1","type":"openai"}`))
	if err != nil {
		t.Fatalf("seeding resource: %v", err)
	}

	r := resourceutil.CRUD(newCRUDSpec(configresource.KindLLMProvider))
	configureResource(t, r, c, "hybrid")

	id := "p1"
	rev := int64(1)
	reqState := newState(t, crudTestSchema(), modelValues("p1", &id, &rev, nil))
	var resp resource.ReadResponse
	resp.State = reqState
	r.Read(context.Background(), resource.ReadRequest{State: reqState}, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}

	var state crudTestModel
	if err := resp.State.Get(context.Background(), &state); err != nil {
		t.Fatalf("reading state: %v", err)
	}
	if state.Name.ValueString() != "p1" {
		t.Errorf("expected name p1, got %q", state.Name.ValueString())
	}
	if state.Revision.ValueInt64() != seeded.Revision {
		t.Errorf("expected revision %d, got %d", seeded.Revision, state.Revision.ValueInt64())
	}
	if !state.ValueJSON.Equal(jsontypes.NewNormalizedValue(string(seeded.Value))) {
		t.Errorf("expected value %s, got %s", seeded.Value, state.ValueJSON.ValueString())
	}
}

func TestDeleteMissingIsSuccess(t *testing.T) {
	_, c := agentgatewaytest.New(t)
	r := resourceutil.CRUD(newCRUDSpec(configresource.KindLLMProvider))
	configureResource(t, r, c, "hybrid")

	id := "gone"
	rev := int64(1)
	reqState := newState(t, crudTestSchema(), modelValues("gone", &id, &rev, nil))
	var resp resource.DeleteResponse
	resp.State = reqState
	r.Delete(context.Background(), resource.DeleteRequest{State: reqState}, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("delete of a missing resource must succeed, got %v", resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Error("expected state to be removed")
	}
}

func TestUpdateConflictDiagnostic(t *testing.T) {
	_, c := agentgatewaytest.New(t)
	r := resourceutil.CRUD(newCRUDSpec(configresource.KindLLMProvider))
	configureResource(t, r, c, "hybrid")

	id := "gone"
	rev := int64(1)
	stateValues := modelValues("gone", &id, &rev, nil)
	planValues := modelValues("renamed", &id, &rev, nil)

	var resp resource.UpdateResponse
	resp.State = emptyResponseState(t, crudTestSchema())
	r.Update(context.Background(), resource.UpdateRequest{
		Plan:  newPlan(t, crudTestSchema(), planValues),
		State: newState(t, crudTestSchema(), stateValues),
	}, &resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a conflict diagnostic")
	}
	detail := resp.Diagnostics.Errors()[0].Detail()
	if !strings.Contains(detail, "conflict detected") {
		t.Errorf("expected conflict hint, got %q", detail)
	}
	if !strings.Contains(detail, "llm.provider/gone") {
		t.Errorf("expected kind/id in detail, got %q", detail)
	}
}

func TestCreateReadOnlyDiagnostic(t *testing.T) {
	srv, c := agentgatewaytest.New(t)
	srv.SetReadOnly(true)

	r := resourceutil.CRUD(newCRUDSpec(configresource.KindLLMProvider))
	configureResource(t, r, c, "hybrid")

	values := modelValues("p1", nil, nil, nil)
	var resp resource.CreateResponse
	resp.State = emptyResponseState(t, crudTestSchema())
	r.Create(context.Background(), resource.CreateRequest{
		Plan: newPlan(t, crudTestSchema(), values),
	}, &resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a read-only diagnostic")
	}
	if title := resp.Diagnostics.Errors()[0].Summary(); !strings.Contains(title, "create") {
		t.Errorf("expected operation in title, got %q", title)
	}
	detail := resp.Diagnostics.Errors()[0].Detail()
	if !strings.Contains(detail, "config.storage.mode") {
		t.Errorf("expected storage-mode hint, got %q", detail)
	}
}

func TestCreateInvalidDiagnostic(t *testing.T) {
	srv, c := agentgatewaytest.New(t)
	srv.FailNextWrites(1, 400)

	r := resourceutil.CRUD(newCRUDSpec(configresource.KindLLMProvider))
	configureResource(t, r, c, "hybrid")

	values := modelValues("p1", nil, nil, nil)
	var resp resource.CreateResponse
	resp.State = emptyResponseState(t, crudTestSchema())
	r.Create(context.Background(), resource.CreateRequest{
		Plan: newPlan(t, crudTestSchema(), values),
	}, &resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an invalid-request diagnostic")
	}
	if detail := resp.Diagnostics.Errors()[0].Detail(); !strings.Contains(detail, "the request was invalid") {
		t.Errorf("expected invalid-request hint, got %q", detail)
	}
}

func TestImportStateAcceptsQualifiedAndBareIDs(t *testing.T) {
	r := resourceutil.CRUD(newCRUDSpec(configresource.KindLLMProvider))
	importer, ok := r.(resource.ResourceWithImportState)
	if !ok {
		t.Fatal("CRUD resource does not implement ResourceWithImportState")
	}

	for _, raw := range []string{"foo", "llm.provider/foo"} {
		t.Run(raw, func(t *testing.T) {
			var resp resource.ImportStateResponse
			resp.State = emptyResponseState(t, crudTestSchema())
			importer.ImportState(context.Background(), resource.ImportStateRequest{ID: raw}, &resp)
			if resp.Diagnostics.HasError() {
				t.Fatalf("unexpected error: %v", resp.Diagnostics)
			}

			var state crudTestModel
			if err := resp.State.Get(context.Background(), &state); err != nil {
				t.Fatalf("reading state: %v", err)
			}
			if state.ID_.ValueString() != "foo" {
				t.Errorf("expected id foo, got %q", state.ID_.ValueString())
			}
		})
	}

	var resp resource.ImportStateResponse
	resp.State = emptyResponseState(t, crudTestSchema())
	importer.ImportState(context.Background(), resource.ImportStateRequest{ID: "llm.model/foo"}, &resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a wrong-kind import diagnostic")
	}
	if detail := resp.Diagnostics.Errors()[0].Detail(); !strings.Contains(detail, "expected import id for kind llm.provider") {
		t.Errorf("unexpected detail: %q", detail)
	}
}

func TestConfigureRejectsUnexpectedProviderData(t *testing.T) {
	r := resourceutil.CRUD(newCRUDSpec(configresource.KindLLMProvider))
	configurable, ok := r.(resource.ResourceWithConfigure)
	if !ok {
		t.Fatal("CRUD resource does not implement ResourceWithConfigure")
	}

	var resp resource.ConfigureResponse
	configurable.Configure(context.Background(), resource.ConfigureRequest{ProviderData: "not-provider-data"}, &resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a diagnostic for the wrong provider data type")
	}

	resp = resource.ConfigureResponse{}
	configurable.Configure(context.Background(), resource.ConfigureRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Errorf("nil provider data must be accepted, got %v", resp.Diagnostics)
	}
}

func TestOperationsRequireConfiguredProvider(t *testing.T) {
	r := resourceutil.CRUD(newCRUDSpec(configresource.KindLLMProvider))

	var resp resource.CreateResponse
	r.Create(context.Background(), resource.CreateRequest{}, &resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a not-configured diagnostic")
	}
	if detail := resp.Diagnostics.Errors()[0].Detail(); !strings.Contains(detail, "no provider data") {
		t.Errorf("unexpected detail: %q", detail)
	}
}

// deadlineClient records the deadline of the context handed to the harness's
// first client call, so a test can prove the configured timeout is applied.
type deadlineClient struct {
	client.Client
	deadline    time.Time
	hasDeadline bool
}

func (c *deadlineClient) record(ctx context.Context) {
	c.deadline, c.hasDeadline = ctx.Deadline()
}

func (c *deadlineClient) List(ctx context.Context, kind configresource.Kind) ([]configresource.Resource, error) {
	c.record(ctx)
	return c.Client.List(ctx, kind)
}

func (c *deadlineClient) Create(ctx context.Context, kind configresource.Kind, value json.RawMessage) (configresource.Resource, error) {
	c.record(ctx)
	return c.Client.Create(ctx, kind, value)
}

func TestCreateUsesConfiguredTimeout(t *testing.T) {
	_, c := agentgatewaytest.New(t)
	recorder := &deadlineClient{Client: c}

	r := resourceutil.CRUD(newCRUDSpec(configresource.KindLLMProvider))
	configurable := r.(resource.ResourceWithConfigure)
	var configResp resource.ConfigureResponse
	configurable.Configure(context.Background(), resource.ConfigureRequest{
		ProviderData: &providerdata.Data{Client: recorder, Timeout: 2 * time.Minute, StoreMode: "hybrid"},
	}, &configResp)
	if configResp.Diagnostics.HasError() {
		t.Fatalf("configure failed: %v", configResp.Diagnostics)
	}

	values := modelValues("p1", nil, nil, nil)
	var resp resource.CreateResponse
	resp.State = emptyResponseState(t, crudTestSchema())
	r.Create(context.Background(), resource.CreateRequest{Plan: newPlan(t, crudTestSchema(), values)}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}

	if !recorder.hasDeadline {
		t.Fatal("client call did not receive a context deadline")
	}
	remaining := time.Until(recorder.deadline)
	if remaining > 2*time.Minute || remaining < 2*time.Minute-5*time.Second {
		t.Errorf("expected a deadline about 2m away, got %s", remaining)
	}
}

func TestUnknownStoreModeDoesNotPreJudgeSingletons(t *testing.T) {
	_, c := agentgatewaytest.New(t)
	r := resourceutil.CRUD(newCRUDSpec(configresource.KindLLMSettings))
	configureResource(t, r, c, "")

	values := modelValues("default", nil, nil, nil)
	var resp resource.CreateResponse
	resp.State = emptyResponseState(t, crudTestSchema())
	r.Create(context.Background(), resource.CreateRequest{Plan: newPlan(t, crudTestSchema(), values)}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unknown store mode must defer to the server, got %v", resp.Diagnostics)
	}
}

// errorClient returns a fixed error from every operation.
type errorClient struct {
	err error
}

func (c *errorClient) Runtime(context.Context) (client.RuntimeInfo, error) {
	return client.RuntimeInfo{}, c.err
}

func (c *errorClient) List(context.Context, configresource.Kind) ([]configresource.Resource, error) {
	return nil, c.err
}

func (c *errorClient) Create(context.Context, configresource.Kind, json.RawMessage) (configresource.Resource, error) {
	return configresource.Resource{}, c.err
}

func (c *errorClient) Update(context.Context, configresource.Kind, string, json.RawMessage) (configresource.Resource, error) {
	return configresource.Resource{}, c.err
}

func (c *errorClient) Delete(context.Context, configresource.Kind, string) error {
	return c.err
}

func (c *errorClient) EffectiveConfig(context.Context) (json.RawMessage, error) {
	return nil, c.err
}

func TestCreateMapsEveryClientSentinel(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"not found", &client.APIError{Status: 404, Message: "gone", Kind: client.ErrNotFound}, "resource not found"},
		{"read only", &client.APIError{Status: 403, Message: "UI is configured as read-only", Kind: client.ErrReadOnly}, "config.storage.mode"},
		{"conflict", &client.APIError{Status: 409, Message: "owned by file", Kind: client.ErrConflict}, "conflict detected"},
		{"invalid", &client.APIError{Status: 400, Message: "bad field", Kind: client.ErrInvalid}, "the request was invalid"},
		{"transient", &client.APIError{Status: 503, Message: "unavailable", Kind: client.ErrTransient}, "transient failure"},
		{"server", &client.APIError{Status: 500, Message: "boom", Kind: client.ErrServer}, "gateway error"},
		{"unknown api", &client.APIError{Status: 418, Message: "teapot", Kind: errors.New("weird")}, "unexpected error"},
		{"plain", errors.New("dial failed"), "unexpected error"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := resourceutil.CRUD(newCRUDSpec(configresource.KindLLMProvider))
			configureResource(t, r, &errorClient{err: tc.err}, "hybrid")

			values := modelValues("p1", nil, nil, nil)
			var resp resource.CreateResponse
			resp.State = emptyResponseState(t, crudTestSchema())
			r.Create(context.Background(), resource.CreateRequest{
				Plan: newPlan(t, crudTestSchema(), values),
			}, &resp)

			if !resp.Diagnostics.HasError() {
				t.Fatal("expected a diagnostic")
			}
			diagnostic := resp.Diagnostics.Errors()[0]
			if !strings.Contains(diagnostic.Summary(), "agentgateway_test_resource") {
				t.Errorf("expected the resource name in the summary, got %q", diagnostic.Summary())
			}
			if !strings.Contains(diagnostic.Detail(), tc.want) {
				t.Errorf("expected %q in detail, got %q", tc.want, diagnostic.Detail())
			}
			if !strings.Contains(diagnostic.Detail(), "llm.provider/p1") {
				t.Errorf("expected kind/id in detail, got %q", diagnostic.Detail())
			}
		})
	}
}

func TestSingletonReadRejectedInFileMode(t *testing.T) {
	srv, c := agentgatewaytest.New(t)
	srv.SetStoreMode("file")

	r := resourceutil.CRUD(newCRUDSpec(configresource.KindLLMSettings))
	configureResource(t, r, c, "file")

	id := "default"
	rev := int64(1)
	reqState := newState(t, crudTestSchema(), modelValues("default", &id, &rev, nil))
	var resp resource.ReadResponse
	resp.State = reqState
	r.Read(context.Background(), resource.ReadRequest{State: reqState}, &resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected the file-mode singleton guard to reject Read")
	}
	detail := resp.Diagnostics.Errors()[0].Detail()
	if !strings.Contains(detail, "requires config.storage.mode = hybrid") {
		t.Errorf("expected hybrid-mode hint, got %q", detail)
	}
	if requests := srv.Requests(); len(requests) != 0 {
		t.Errorf("Read must not call the gateway, got %d requests", len(requests))
	}
	if resp.State.Raw.IsNull() {
		t.Error("Read must not remove the singleton from state")
	}
}

func TestSingletonUpdateRejectedInFileMode(t *testing.T) {
	srv, c := agentgatewaytest.New(t)
	srv.SetStoreMode("file")

	r := resourceutil.CRUD(newCRUDSpec(configresource.KindLLMSettings))
	configureResource(t, r, c, "file")

	id := "default"
	rev := int64(1)
	stateValues := modelValues("default", &id, &rev, nil)
	planValues := modelValues("default", &id, &rev, nil)

	var resp resource.UpdateResponse
	resp.State = emptyResponseState(t, crudTestSchema())
	r.Update(context.Background(), resource.UpdateRequest{
		Plan:  newPlan(t, crudTestSchema(), planValues),
		State: newState(t, crudTestSchema(), stateValues),
	}, &resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected the file-mode singleton guard to reject Update")
	}
	detail := resp.Diagnostics.Errors()[0].Detail()
	if !strings.Contains(detail, "requires config.storage.mode = hybrid") {
		t.Errorf("expected hybrid-mode hint, got %q", detail)
	}
	if requests := srv.Requests(); len(requests) != 0 {
		t.Errorf("Update must not call the gateway, got %d requests", len(requests))
	}
}

// emptyIDModel always reports an empty id, like a broken non-api-key model.
type emptyIDModel struct {
	IDValue   types.String         `tfsdk:"id"`
	Revision  types.Int64          `tfsdk:"revision"`
	Name      types.String         `tfsdk:"name"`
	ValueJSON jsontypes.Normalized `tfsdk:"value_json"`
}

func (m *emptyIDModel) Kind() configresource.Kind { return configresource.KindLLMProvider }

func (m *emptyIDModel) ID() string { return "" }

func (m *emptyIDModel) Value(_ context.Context, _ *diag.Diagnostics) json.RawMessage {
	return json.RawMessage(`{"name":"` + m.Name.ValueString() + `"}`)
}

func (m *emptyIDModel) SetValue(_ context.Context, value json.RawMessage, _ *diag.Diagnostics) {
	m.ValueJSON = jsontypes.NewNormalizedValue(string(value))
}

func (m *emptyIDModel) SetServerMeta(id string, revision int64) {
	m.IDValue = types.StringValue(id)
	m.Revision = types.Int64Value(revision)
}

func TestCreateWithEmptyIDFails(t *testing.T) {
	srv, c := agentgatewaytest.New(t)
	r := resourceutil.CRUD(resourceutil.Spec{
		TypeName: "agentgateway_test_resource",
		Kind:     configresource.KindLLMProvider,
		Schema:   crudTestSchema(),
		New:      func() resourceutil.KindResource { return &emptyIDModel{} },
	})
	configureResource(t, r, c, "hybrid")

	values := modelValues("p1", nil, nil, nil)
	var resp resource.CreateResponse
	resp.State = emptyResponseState(t, crudTestSchema())
	r.Create(context.Background(), resource.CreateRequest{Plan: newPlan(t, crudTestSchema(), values)}, &resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a diagnostic for an empty derived id")
	}
	detail := resp.Diagnostics.Errors()[0].Detail()
	if !strings.Contains(detail, "requires an id derived from the plan") {
		t.Errorf("expected an actionable id diagnostic, got %q", detail)
	}
	if requests := srv.Requests(); len(requests) != 0 {
		t.Errorf("Create must not write without an id, got %d requests", len(requests))
	}
}

func TestImportUnwrittenSingletonFails(t *testing.T) {
	_, c := agentgatewaytest.New(t)
	r := resourceutil.CRUD(newCRUDSpec(configresource.KindLLMSettings))
	configureResource(t, r, c, "hybrid")

	importer := r.(resource.ResourceWithImportState)
	var resp resource.ImportStateResponse
	resp.State = emptyResponseState(t, crudTestSchema())
	importer.ImportState(context.Background(), resource.ImportStateRequest{ID: "default"}, &resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a diagnostic for importing an unwritten singleton")
	}
	detail := resp.Diagnostics.Errors()[0].Detail()
	if !strings.Contains(detail, "apply the resource before importing it") {
		t.Errorf("expected an apply-first hint, got %q", detail)
	}
}

func TestImportWrittenSingletonSucceeds(t *testing.T) {
	_, c := agentgatewaytest.New(t)
	if _, err := c.Create(context.Background(), configresource.KindLLMSettings, json.RawMessage(`{"port":8080}`)); err != nil {
		t.Fatalf("seeding singleton: %v", err)
	}

	r := resourceutil.CRUD(newCRUDSpec(configresource.KindLLMSettings))
	configureResource(t, r, c, "hybrid")

	importer := r.(resource.ResourceWithImportState)
	var resp resource.ImportStateResponse
	resp.State = emptyResponseState(t, crudTestSchema())
	importer.ImportState(context.Background(), resource.ImportStateRequest{ID: "llm.settings/default"}, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	var state crudTestModel
	if err := resp.State.Get(context.Background(), &state); err != nil {
		t.Fatalf("reading state: %v", err)
	}
	if state.ID_.ValueString() != "default" {
		t.Errorf("expected id default, got %q", state.ID_.ValueString())
	}
}

// attributeTestModel is a provider model with a snake_case attribute the
// gateway can name in a 400/422 message.
type attributeTestModel struct {
	IDValue      types.String `tfsdk:"id"`
	Revision     types.Int64  `tfsdk:"revision"`
	Name         types.String `tfsdk:"name"`
	StatefulMode types.String `tfsdk:"stateful_mode"`
}

func (m *attributeTestModel) Kind() configresource.Kind { return configresource.KindLLMProvider }

func (m *attributeTestModel) ID() string { return m.Name.ValueString() }

func (m *attributeTestModel) Value(_ context.Context, _ *diag.Diagnostics) json.RawMessage {
	return json.RawMessage(`{"name":"` + m.Name.ValueString() + `"}`)
}

func (m *attributeTestModel) SetValue(context.Context, json.RawMessage, *diag.Diagnostics) {}

func (m *attributeTestModel) SetServerMeta(id string, revision int64) {
	m.IDValue = types.StringValue(id)
	m.Revision = types.Int64Value(revision)
}

func attributeTestSchema() schema.Schema {
	return schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id":            schema.StringAttribute{Computed: true},
			"revision":      schema.Int64Attribute{Computed: true},
			"name":          schema.StringAttribute{Required: true},
			"stateful_mode": schema.StringAttribute{Optional: true},
		},
	}
}

func attributeTestAttrTypes() map[string]tftypes.Type {
	return map[string]tftypes.Type{
		"id":            tftypes.String,
		"revision":      tftypes.Number,
		"name":          tftypes.String,
		"stateful_mode": tftypes.String,
	}
}

func attributeTestSpec() resourceutil.Spec {
	return resourceutil.Spec{
		TypeName: "agentgateway_test_resource",
		Kind:     configresource.KindLLMProvider,
		Schema:   attributeTestSchema(),
		New:      func() resourceutil.KindResource { return &attributeTestModel{} },
	}
}

func attributeTestPlan(t *testing.T) tfsdk.Plan {
	t.Helper()
	return tfsdk.Plan{
		Raw: tftypes.NewValue(tftypes.Object{AttributeTypes: attributeTestAttrTypes()}, map[string]tftypes.Value{
			"id":            tftypes.NewValue(tftypes.String, nil),
			"revision":      tftypes.NewValue(tftypes.Number, nil),
			"name":          tftypes.NewValue(tftypes.String, "p1"),
			"stateful_mode": tftypes.NewValue(tftypes.String, "stateful"),
		}),
		Schema: attributeTestSchema(),
	}
}

func TestInvalidErrorMapsNamedAttribute(t *testing.T) {
	cases := []struct {
		name          string
		message       string
		wantAttribute string
	}{
		{"snake case", "unsupported field: stateful_mode", "stateful_mode"},
		{"camel case", "unsupported field: statefulMode", "stateful_mode"},
		{"no attribute", "the request is malformed", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := resourceutil.CRUD(attributeTestSpec())
			configureResource(t, r, &errorClient{
				err: &client.APIError{Status: 422, Message: tc.message, Kind: client.ErrInvalid},
			}, "hybrid")

			var resp resource.CreateResponse
			resp.State = emptyStateWith(t, attributeTestSchema(), attributeTestAttrTypes())
			r.Create(context.Background(), resource.CreateRequest{Plan: attributeTestPlan(t)}, &resp)

			if !resp.Diagnostics.HasError() {
				t.Fatal("expected a diagnostic")
			}
			diagnostic := resp.Diagnostics.Errors()[0]
			withPath, hasPath := diagnostic.(diag.DiagnosticWithPath)

			if tc.wantAttribute == "" {
				if hasPath {
					t.Errorf("expected a resource-level diagnostic, got attribute path %s", withPath.Path())
				}
				if !strings.Contains(diagnostic.Detail(), "the request was invalid") {
					t.Errorf("expected invalid-request hint, got %q", diagnostic.Detail())
				}
				return
			}

			if !hasPath {
				t.Fatalf("expected an attribute-level diagnostic, got %T", diagnostic)
			}
			if !withPath.Path().Equal(path.Root(tc.wantAttribute)) {
				t.Errorf("expected attribute path %s, got %s", path.Root(tc.wantAttribute), withPath.Path())
			}
			if !strings.Contains(diagnostic.Detail(), tc.wantAttribute) {
				t.Errorf("expected the attribute name in the detail, got %q", diagnostic.Detail())
			}
			if !strings.Contains(diagnostic.Summary(), "create failed") {
				t.Errorf("expected the operation in the summary, got %q", diagnostic.Summary())
			}
		})
	}
}

// TestConflictWithoutIDKeepsServerMessage covers the server-generated-id path
// (llm.apiKey): a create conflict has no id to name, but the gateway message
// must still reach the diagnostic.
func TestConflictWithoutIDKeepsServerMessage(t *testing.T) {
	const message = "DB-backed API keys require llm.policies.apiKey"

	r := resourceutil.CRUD(resourceutil.Spec{
		TypeName: "agentgateway_llm_api_key",
		Kind:     configresource.KindLLMAPIKey,
		Schema:   apiKeyTestSchema(),
		New:      func() resourceutil.KindResource { return &apiKeyTestModel{} },
	})
	configureResource(t, r, &errorClient{
		err: &client.APIError{Status: 409, Message: message, Kind: client.ErrConflict},
	}, "hybrid")

	plan := tfsdk.Plan{
		Raw: tftypes.NewValue(tftypes.Object{AttributeTypes: apiKeyTestAttrTypes()}, map[string]tftypes.Value{
			"id":       tftypes.NewValue(tftypes.String, nil),
			"revision": tftypes.NewValue(tftypes.Number, nil),
			"key":      tftypes.NewValue(tftypes.String, "plaintext-key"),
		}),
		Schema: apiKeyTestSchema(),
	}

	var resp resource.CreateResponse
	resp.State = emptyStateWith(t, apiKeyTestSchema(), apiKeyTestAttrTypes())
	r.Create(context.Background(), resource.CreateRequest{Plan: plan}, &resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a conflict diagnostic")
	}
	detail := resp.Diagnostics.Errors()[0].Detail()
	if !strings.Contains(detail, message) {
		t.Errorf("expected the gateway message in the detail, got %q", detail)
	}
	if !strings.Contains(detail, "conflict detected") {
		t.Errorf("expected the conflict hint in the detail, got %q", detail)
	}
	if strings.Contains(detail, "llm.apiKey/") {
		t.Errorf("expected no id in the detail, got %q", detail)
	}
}
