package mcptargets

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/caberdo/terraform-provider-agentgateway/internal/acctest"
	"github.com/caberdo/terraform-provider-agentgateway/internal/client"
	"github.com/caberdo/terraform-provider-agentgateway/internal/configresource"
	"github.com/caberdo/terraform-provider-agentgateway/internal/providerdata"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	providerschema "github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

const acceptanceTarget = "acctest-mcp-target"

// TestAccMCPTargetsDataSource creates an MCP target through the provider under
// test and reads it back through the data source. It is compiled by `go test`
// and only runs when TF_ACC is set and the environment is reachable.
func TestAccMCPTargetsDataSource(t *testing.T) {
	acctest.PreCheck(t)
	endpoint := acctest.EndpointFor(t, acctest.RoleHybrid)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactories(endpoint),
		CheckDestroy:             checkBackingDestroyed(endpoint, configresource.KindMCPTarget, acceptanceTarget),
		Steps: []resource.TestStep{
			{
				Config: targetsDataSourceConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.agentgateway_mcp_targets.test", "id", "mcp.target:"+acceptanceTarget),
					resource.TestCheckResourceAttr("data.agentgateway_mcp_targets.test", "targets.#", "1"),
					resource.TestCheckResourceAttr("data.agentgateway_mcp_targets.test", "targets.0.name", acceptanceTarget),
					resource.TestCheckResourceAttr("data.agentgateway_mcp_targets.test", "targets.0.transport", "mcp"),
					resource.TestCheckResourceAttr("data.agentgateway_mcp_targets.test", "targets.0.host", "127.0.0.1"),
					resource.TestCheckResourceAttr("data.agentgateway_mcp_targets.test", "targets.0.path", "/mcp"),
				),
			},
		},
	})
}

const targetsDataSourceConfig = `
resource "agentgateway_backing" "target" {
  kind = "mcp.target"
  value_json = jsonencode({
    name = "` + acceptanceTarget + `"
    mcp = {
      host = "127.0.0.1"
      port = 8080
      path = "/mcp"
    }
  })
}

data "agentgateway_mcp_targets" "test" {
  name_prefix = "` + acceptanceTarget + `"
  depends_on  = [agentgateway_backing.target]
}
`

// testFactories builds a provider that registers the data source and the one
// backing resource type the test needs. Data-source packages must not import
// internal/provider, so the acceptance test supplies the minimal provider.
func testFactories(endpoint string) map[string]func() (tfprotov6.ProviderServer, error) {
	p := &testProvider{endpoint: endpoint}
	return map[string]func() (tfprotov6.ProviderServer, error){
		"agentgateway": providerserver.NewProtocol6WithError(p),
	}
}

func checkBackingDestroyed(endpoint string, kind configresource.Kind, ids ...string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		c, err := client.New(client.Config{
			BaseURL:    endpoint,
			HTTPClient: http.DefaultClient,
			UserAgent:  "terraform-provider-agentgateway/acctest",
		})
		if err != nil {
			return fmt.Errorf("creating acceptance client: %w", err)
		}

		list, err := c.List(context.Background(), kind)
		if err != nil {
			return fmt.Errorf("listing %s: %w", kind, err)
		}
		for _, res := range list {
			for _, id := range ids {
				if res.ID == id {
					return fmt.Errorf("%s/%s still exists after destroy", kind, id)
				}
			}
		}
		return nil
	}
}

// testProvider is the smallest provider that can host this data source and its
// backing resource in an acceptance test.
type testProvider struct {
	endpoint string
}

func (p *testProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "agentgateway"
	resp.Version = "acctest"
}

func (p *testProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = providerschema.Schema{
		MarkdownDescription: "Minimal provider used by this data source's acceptance test.",
	}
}

func (p *testProvider) Configure(_ context.Context, _ provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	c, err := client.New(client.Config{
		BaseURL:    p.endpoint,
		HTTPClient: http.DefaultClient,
		UserAgent:  "terraform-provider-agentgateway/acctest",
	})
	if err != nil {
		resp.Diagnostics.AddError("Could not create the acceptance client", err.Error())
		return
	}

	data := &providerdata.Data{Client: c}
	resp.ResourceData = data
	resp.DataSourceData = data
}

func (p *testProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{New}
}

func (p *testProvider) Resources(_ context.Context) []func() frameworkresource.Resource {
	return []func() frameworkresource.Resource{newBackingResource}
}

// backingModel is the state of the generic test-only resource.
type backingModel struct {
	Kind      types.String `tfsdk:"kind"`
	ID        types.String `tfsdk:"id"`
	ValueJSON types.String `tfsdk:"value_json"`
}

// backingResource creates a config resource of any kind through the same
// client the provider uses. It exists so a data-source acceptance test can seed
// its own backing data without importing a sibling resource package.
type backingResource struct {
	client client.Client
}

func newBackingResource() frameworkresource.Resource { return &backingResource{} }

func (r *backingResource) Metadata(_ context.Context, _ frameworkresource.MetadataRequest, resp *frameworkresource.MetadataResponse) {
	resp.TypeName = "agentgateway_backing"
}

func (r *backingResource) Schema(_ context.Context, _ frameworkresource.SchemaRequest, resp *frameworkresource.SchemaResponse) {
	resp.Schema = resourceschema.Schema{
		MarkdownDescription: "Test-only resource that seeds a config resource.",
		Attributes: map[string]resourceschema.Attribute{
			"kind": resourceschema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Config resource kind to create.",
			},
			"id": resourceschema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Resource id; required for policy kinds.",
			},
			"value_json": resourceschema.StringAttribute{
				Required:            true,
				MarkdownDescription: "JSON value sent to the config API.",
			},
		},
	}
}

func (r *backingResource) Configure(_ context.Context, req frameworkresource.ConfigureRequest, resp *frameworkresource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	data, ok := req.ProviderData.(*providerdata.Data)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("expected *providerdata.Data, got %T", req.ProviderData))
		return
	}
	r.client = data.Client
}

func (r *backingResource) Create(ctx context.Context, req frameworkresource.CreateRequest, resp *frameworkresource.CreateResponse) {
	var plan backingModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.apply(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *backingResource) Read(ctx context.Context, req frameworkresource.ReadRequest, resp *frameworkresource.ReadResponse) {
	var state backingModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if r.client == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	list, err := r.client.List(ctx, configresource.Kind(state.Kind.ValueString()))
	if err != nil {
		resp.Diagnostics.AddError("Backing resource read failed", err.Error())
		return
	}
	for _, res := range list {
		if res.ID == state.ID.ValueString() {
			return
		}
	}
	resp.State.RemoveResource(ctx)
}

func (r *backingResource) Update(ctx context.Context, req frameworkresource.UpdateRequest, resp *frameworkresource.UpdateResponse) {
	var plan backingModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.apply(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *backingResource) Delete(ctx context.Context, req frameworkresource.DeleteRequest, resp *frameworkresource.DeleteResponse) {
	var state backingModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.Delete(ctx, configresource.Kind(state.Kind.ValueString()), state.ID.ValueString())
	if err != nil && !errors.Is(err, client.ErrNotFound) {
		resp.Diagnostics.AddError("Backing resource delete failed", err.Error())
		return
	}
	resp.State.RemoveResource(ctx)
}

// apply writes the value. Policy kinds are written through the single-resource
// path, which is the only path that accepts a policy create.
func (r *backingResource) apply(ctx context.Context, plan *backingModel, diags *diag.Diagnostics) {
	kind := configresource.Kind(plan.Kind.ValueString())
	value := json.RawMessage(plan.ValueJSON.ValueString())

	var res configresource.Resource
	var err error
	if kind.IsPolicy() {
		res, err = r.client.Update(ctx, kind, plan.ID.ValueString(), value)
	} else {
		res, err = r.client.Create(ctx, kind, value)
	}
	if err != nil {
		diags.AddError("Backing resource write failed", err.Error())
		return
	}
	plan.ID = types.StringValue(res.ID)
}
