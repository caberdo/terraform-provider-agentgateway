package effectiveconfig

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/caberdo/terraform-provider-agentgateway/internal/client"
	"github.com/caberdo/terraform-provider-agentgateway/internal/configresource"
	"github.com/caberdo/terraform-provider-agentgateway/internal/providerdata"
	"github.com/caberdo/terraform-provider-agentgateway/internal/resourceutil"
	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	providerschema "github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
)

const (
	// testResourceTypeName is the type name of the throwaway resource that
	// creates the backing traffic.gateway resources for these tests.
	testResourceTypeName = "agentgateway_test_resource"
	// testKind is the kind the throwaway resource writes.
	testKind = configresource.KindTrafficGateway
)

// testFactories builds a provider that registers only the data source under
// test and one generic resource used to create its backing resources. The
// data-source package must not import internal/provider (design §4), so the
// test supplies the minimal provider it needs.
func testFactories(endpoint string) map[string]func() (tfprotov6.ProviderServer, error) {
	p := &testProvider{endpoint: endpoint}
	return map[string]func() (tfprotov6.ProviderServer, error){
		"agentgateway": providerserver.NewProtocol6WithError(p),
	}
}

// testProvider is the smallest provider that can host the data source and the
// throwaway resource in a test.
type testProvider struct {
	endpoint string
}

// Metadata returns the provider type name.
func (p *testProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "agentgateway"
	resp.Version = "test"
}

// Schema returns an empty provider schema.
func (p *testProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = providerschema.Schema{
		MarkdownDescription: "Minimal provider used by the data source's tests.",
	}
}

// Configure builds a client for the endpoint under test.
func (p *testProvider) Configure(_ context.Context, _ provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	c, err := client.New(client.Config{
		BaseURL:    p.endpoint,
		HTTPClient: http.DefaultClient,
		UserAgent:  "terraform-provider-agentgateway/test",
	})
	if err != nil {
		resp.Diagnostics.AddError("Could not create the test client", err.Error())
		return
	}

	data := &providerdata.Data{Client: c}
	resp.ResourceData = data
	resp.DataSourceData = data
}

// DataSources returns the data source under test.
func (p *testProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{New}
}

// Resources returns the throwaway backing resource.
func (p *testProvider) Resources(_ context.Context) []func() frameworkresource.Resource {
	return []func() frameworkresource.Resource{newTestResource}
}

// testResourceModel is the model of the throwaway resource. Its value is the
// raw JSON object written to the gateway, so a test can create any gateway
// shape without duplicating the resource's typed schema.
type testResourceModel struct {
	StoredID types.String         `tfsdk:"id"`
	Revision types.Int64          `tfsdk:"revision"`
	Name     types.String         `tfsdk:"name"`
	Payload  jsontypes.Normalized `tfsdk:"value_json"`
}

// Kind returns the kind the resource writes.
func (m *testResourceModel) Kind() configresource.Kind { return testKind }

// ID returns the gateway name, which is the server-side id.
func (m *testResourceModel) ID() string { return m.Name.ValueString() }

// Value returns the configured raw JSON object.
func (m *testResourceModel) Value(_ context.Context, diags *diag.Diagnostics) json.RawMessage {
	raw := m.Payload.ValueString()
	if raw == "" || !json.Valid([]byte(raw)) {
		diags.AddError("Invalid test resource value", "value_json must be a valid JSON object")
		return nil
	}
	return json.RawMessage(raw)
}

// SetValue stores the value the server returned.
func (m *testResourceModel) SetValue(_ context.Context, value json.RawMessage, _ *diag.Diagnostics) {
	m.Payload = jsontypes.NewNormalizedValue(string(value))
}

// SetServerMeta stores the computed id and revision.
func (m *testResourceModel) SetServerMeta(id string, revision int64) {
	m.StoredID = types.StringValue(id)
	m.Revision = types.Int64Value(revision)
}

// newTestResource returns the throwaway backing resource.
func newTestResource() frameworkresource.Resource {
	return resourceutil.CRUD(resourceutil.Spec{
		TypeName: testResourceTypeName,
		Kind:     testKind,
		Schema: resourceschema.Schema{
			MarkdownDescription: "Test-only resource that writes a raw JSON value for a config resource kind.",
			Attributes: map[string]resourceschema.Attribute{
				"id": resourceschema.StringAttribute{
					Computed: true,
				},
				"revision": resourceschema.Int64Attribute{
					Computed: true,
				},
				"name": resourceschema.StringAttribute{
					Required: true,
				},
				"value_json": resourceschema.StringAttribute{
					Required:   true,
					CustomType: jsontypes.NormalizedType{},
				},
			},
		},
		New: func() resourceutil.KindResource { return &testResourceModel{} },
	})
}
