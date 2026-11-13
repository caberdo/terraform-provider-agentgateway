package llmsettings

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/caberdo/terraform-provider-agentgateway/internal/client"
	"github.com/caberdo/terraform-provider-agentgateway/internal/configresource"
	"github.com/caberdo/terraform-provider-agentgateway/internal/providerdata"
	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func TestLLMSettingsValue(t *testing.T) {
	tests := []struct {
		name  string
		model llmSettingsModel
		want  string
	}{
		{
			name: "gateways",
			model: llmSettingsModel{
				Gateways: types.ListValueMust(types.StringType, []attr.Value{
					types.StringValue("default"),
					types.StringValue("llm/listener"),
				}),
			},
			want: `{"gateways": ["default", "llm/listener"]}`,
		},
		{
			name: "empty gateways list is sent as an empty list",
			model: llmSettingsModel{
				Gateways: types.ListValueMust(types.StringType, []attr.Value{}),
			},
			want: `{"gateways": []}`,
		},
		{
			name: "port with tls",
			model: llmSettingsModel{
				Port:    types.Int64Value(8080),
				TLSJSON: jsontypes.NewNormalizedValue(`{"cert":"/etc/tls/tls.crt","key":"/etc/tls/tls.key"}`),
			},
			want: `{"port": 8080, "tls": {"cert": "/etc/tls/tls.crt", "key": "/etc/tls/tls.key"}}`,
		},
		{
			name: "null attributes are omitted from the payload",
			model: llmSettingsModel{
				Gateways: types.ListNull(types.StringType),
				Port:     types.Int64Null(),
				TLSJSON:  jsontypes.NewNormalizedNull(),
			},
			want: `{}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var diags diag.Diagnostics
			got := tt.model.Value(context.Background(), &diags)
			if diags.HasError() {
				t.Fatalf("unexpected diagnostics: %v", diags)
			}
			assertJSONEqual(t, got, tt.want)
		})
	}
}

func TestLLMSettingsValueRejectsGatewaysWithPort(t *testing.T) {
	model := llmSettingsModel{
		Gateways: types.ListValueMust(types.StringType, []attr.Value{types.StringValue("default")}),
		Port:     types.Int64Value(8080),
	}

	var diags diag.Diagnostics
	if got := model.Value(context.Background(), &diags); got != nil {
		t.Errorf("expected no payload, got %s", got)
	}
	if !diags.HasError() {
		t.Fatal("expected a diagnostic for gateways and port together")
	}
}

func TestLLMSettingsIDAndServerMeta(t *testing.T) {
	model := llmSettingsModel{}
	if got := model.ID(); got != "default" {
		t.Errorf("expected the singleton id %q, got %q", "default", got)
	}

	model.SetServerMeta("default", 3)
	if model.IDValue.ValueString() != "default" {
		t.Errorf("expected stored id, got %q", model.IDValue.ValueString())
	}
	if model.Revision.ValueInt64() != 3 {
		t.Errorf("expected revision 3, got %d", model.Revision.ValueInt64())
	}
}

func TestLLMSettingsSchema(t *testing.T) {
	s := newSchema()

	for _, name := range []string{"id", "revision"} {
		attribute, ok := s.Attributes[name]
		if !ok {
			t.Fatalf("expected a %q attribute", name)
		}
		if !attribute.IsComputed() {
			t.Errorf("%q must be computed", name)
		}
	}

	for _, name := range []string{"gateways", "port", "tls_json"} {
		attribute, ok := s.Attributes[name]
		if !ok {
			t.Fatalf("expected a %q attribute", name)
		}
		if !attribute.IsOptional() {
			t.Errorf("%q must be optional", name)
		}
	}
}

// TestLLMSettingsCreateMapsFileOwnedConflict pins the requirement that a 409
// naming a field the config file owns becomes an actionable diagnostic instead
// of the harness's generic conflict message.
func TestLLMSettingsCreateMapsFileOwnedConflict(t *testing.T) {
	c := &stubClient{
		createErr: &client.APIError{
			Status:  409,
			Message: "file-owned llm.settings/default field cannot be updated in hybrid mode: gateways",
			Kind:    client.ErrConflict,
		},
	}

	r := New()
	r.(resource.ResourceWithConfigure).Configure(context.Background(), resource.ConfigureRequest{
		ProviderData: &providerdata.Data{Client: c, StoreMode: "hybrid"},
	}, &resource.ConfigureResponse{})

	plan := newPlan(t, map[string]tftypes.Value{
		"id":       tftypes.NewValue(tftypes.String, nil),
		"revision": tftypes.NewValue(tftypes.Number, nil),
		"gateways": tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, []tftypes.Value{
			tftypes.NewValue(tftypes.String, "default"),
		}),
		"port":     tftypes.NewValue(tftypes.Number, nil),
		"tls_json": tftypes.NewValue(tftypes.String, nil),
	})

	var resp resource.CreateResponse
	resp.State = tfsdk.State(plan)
	r.Create(context.Background(), resource.CreateRequest{Plan: plan}, &resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a diagnostic")
	}
	detail := resp.Diagnostics.Errors()[0].Detail()
	for _, want := range []string{"config file owns", "gateways", "config.storage.mode"} {
		if !strings.Contains(detail, want) {
			t.Errorf("expected %q in detail, got %q", want, detail)
		}
	}
}

// TestLLMSettingsCreateKeepsGenericConflict pins that a 409 which is not about
// a file-owned field keeps the shared harness's diagnostic.
func TestLLMSettingsCreateKeepsGenericConflict(t *testing.T) {
	c := &stubClient{
		createErr: &client.APIError{
			Status:  409,
			Message: "generation conflict",
			Kind:    client.ErrConflict,
		},
	}

	r := New()
	r.(resource.ResourceWithConfigure).Configure(context.Background(), resource.ConfigureRequest{
		ProviderData: &providerdata.Data{Client: c, StoreMode: "hybrid"},
	}, &resource.ConfigureResponse{})

	plan := newPlan(t, map[string]tftypes.Value{
		"id":       tftypes.NewValue(tftypes.String, nil),
		"revision": tftypes.NewValue(tftypes.Number, nil),
		"gateways": tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, nil),
		"port":     tftypes.NewValue(tftypes.Number, nil),
		"tls_json": tftypes.NewValue(tftypes.String, nil),
	})

	var resp resource.CreateResponse
	resp.State = tfsdk.State(plan)
	r.Create(context.Background(), resource.CreateRequest{Plan: plan}, &resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a diagnostic")
	}
	if detail := resp.Diagnostics.Errors()[0].Detail(); !strings.Contains(detail, "conflict detected") {
		t.Errorf("expected the generic conflict detail, got %q", detail)
	}
}

// stubClient is a client.Client whose write operations return a fixed error.
type stubClient struct {
	createErr error
}

func (c *stubClient) Runtime(context.Context) (client.RuntimeInfo, error) {
	return client.RuntimeInfo{}, nil
}

func (c *stubClient) List(context.Context, configresource.Kind) ([]configresource.Resource, error) {
	return nil, nil
}

func (c *stubClient) Create(context.Context, configresource.Kind, json.RawMessage) (configresource.Resource, error) {
	return configresource.Resource{}, c.createErr
}

func (c *stubClient) Update(context.Context, configresource.Kind, string, json.RawMessage) (configresource.Resource, error) {
	return configresource.Resource{}, c.createErr
}

func (c *stubClient) Delete(context.Context, configresource.Kind, string) error {
	return nil
}

func (c *stubClient) EffectiveConfig(context.Context) (json.RawMessage, error) {
	return nil, nil
}

// newPlan builds a plan for the settings schema from attribute values.
func newPlan(t *testing.T, values map[string]tftypes.Value) tfsdk.Plan {
	t.Helper()

	s := newSchema()
	attrTypes := map[string]tftypes.Type{
		"id":       tftypes.String,
		"revision": tftypes.Number,
		"gateways": tftypes.List{ElementType: tftypes.String},
		"port":     tftypes.Number,
		"tls_json": tftypes.String,
	}
	return tfsdk.Plan{
		Raw:    tftypes.NewValue(tftypes.Object{AttributeTypes: attrTypes}, values),
		Schema: s,
	}
}

func assertJSONEqual(t *testing.T, got json.RawMessage, want string) {
	t.Helper()

	var gotValue, wantValue any
	if err := json.Unmarshal(got, &gotValue); err != nil {
		t.Fatalf("decoding value %s: %v", got, err)
	}
	if err := json.Unmarshal([]byte(want), &wantValue); err != nil {
		t.Fatalf("decoding want %s: %v", want, err)
	}
	if !reflect.DeepEqual(gotValue, wantValue) {
		canonical, _ := json.Marshal(gotValue)
		t.Errorf("value mismatch:\n got: %s\nwant: %s", canonical, want)
	}
}
