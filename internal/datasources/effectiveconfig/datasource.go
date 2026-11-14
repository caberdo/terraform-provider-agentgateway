// Package effectiveconfig implements the agentgateway_effective_config data
// source (design §8): the merged configuration the gateway is currently
// serving, read from GET /api/config/effective.
package effectiveconfig

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/caberdo/terraform-provider-agentgateway/internal/providerdata"
	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

const (
	// typeName is the Terraform type name of the data source.
	typeName = "agentgateway_effective_config"
	// defaultTimeout matches the provider's default timeout (design §6).
	defaultTimeout = 30 * time.Second
)

// model is the data source state. The whole document is a single attribute so
// it can be marked Sensitive as a whole: the effective config contains key
// material for apiKey policies and provider credentials.
type model struct {
	JSON jsontypes.Normalized `tfsdk:"json"`
}

// dataSource is the agentgateway_effective_config implementation.
type dataSource struct {
	data *providerdata.Data
}

// New returns the data source.
func New() datasource.DataSource {
	return &dataSource{}
}

// Metadata returns the Terraform type name.
func (d *dataSource) Metadata(_ context.Context, _ datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = typeName
}

// Schema returns the data source schema.
func (d *dataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "The effective configuration the gateway is serving, as returned by `/api/config/effective`. " +
			"The document is marked sensitive because it can contain provider credentials and API key material.",
		Attributes: map[string]schema.Attribute{
			"json": schema.StringAttribute{
				Computed:            true,
				Sensitive:           true,
				CustomType:          jsontypes.NormalizedType{},
				MarkdownDescription: "The effective configuration document as normalized JSON. Marked sensitive as a whole.",
			},
		},
	}
}

// Configure stores the provider data for later reads.
func (d *dataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	data, ok := req.ProviderData.(*providerdata.Data)
	if !ok {
		resp.Diagnostics.AddError(
			typeName+" configure failed",
			fmt.Sprintf("expected *providerdata.Data, got %T", req.ProviderData),
		)
		return
	}
	d.data = data
}

// Read fetches the effective configuration and stores it in state.
func (d *dataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	data := d.data
	if data == nil {
		resp.Diagnostics.AddError(
			typeName+" read failed",
			"the provider was not configured; set the gateway endpoint in the provider block or AGENTGATEWAY_ENDPOINT and re-apply",
		)
		return
	}

	ctx, cancel := context.WithTimeout(ctx, timeout(data.Timeout))
	defer cancel()

	var model model
	resp.Diagnostics.Append(req.Config.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body, err := data.Client.EffectiveConfig(ctx)
	if err != nil {
		resp.Diagnostics.AddError(
			typeName+" read failed",
			fmt.Sprintf("the gateway could not return its effective configuration: %s. Check that the gateway is reachable and retry.", err.Error()),
		)
		return
	}

	document, err := normalize(body)
	if err != nil {
		resp.Diagnostics.AddError(
			typeName+" read failed",
			"the gateway returned an effective configuration that is not valid JSON; check that /api/config/effective answers with a JSON document and retry",
		)
		return
	}

	model.JSON = document

	// Only the size is logged: the document itself may contain secrets.
	tflog.Debug(ctx, "read effective config", map[string]any{
		"bytes": len(body),
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

// normalize validates the body and stores it as normalized JSON. A non-JSON
// body is rejected: the attribute is typed JSON, so storing an arbitrary string
// would only move the failure to the next plan.
func normalize(body []byte) (jsontypes.Normalized, error) {
	if len(body) == 0 {
		return jsontypes.NewNormalizedNull(), fmt.Errorf("empty effective configuration")
	}
	if !json.Valid(body) {
		return jsontypes.NewNormalizedNull(), fmt.Errorf("effective configuration is not valid JSON")
	}
	return jsontypes.NewNormalizedValue(string(body)), nil
}

// timeout returns the effective read timeout.
func timeout(d time.Duration) time.Duration {
	if d <= 0 {
		return defaultTimeout
	}
	return d
}
