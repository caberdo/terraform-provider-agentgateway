package provider

import (
	"context"
	"fmt"

	"github.com/caberdo/terraform-provider-agentgateway/internal/client"
	"github.com/caberdo/terraform-provider-agentgateway/internal/datasources/configresources"
	"github.com/caberdo/terraform-provider-agentgateway/internal/datasources/effectiveconfig"
	"github.com/caberdo/terraform-provider-agentgateway/internal/datasources/llmapikeys"
	"github.com/caberdo/terraform-provider-agentgateway/internal/datasources/llmmodels"
	"github.com/caberdo/terraform-provider-agentgateway/internal/datasources/llmproviders"
	"github.com/caberdo/terraform-provider-agentgateway/internal/datasources/mcptargets"
	"github.com/caberdo/terraform-provider-agentgateway/internal/datasources/runtime"
	"github.com/caberdo/terraform-provider-agentgateway/internal/datasources/trafficgateways"
	"github.com/caberdo/terraform-provider-agentgateway/internal/datasources/trafficroutes"
	"github.com/caberdo/terraform-provider-agentgateway/internal/providerdata"
	"github.com/caberdo/terraform-provider-agentgateway/internal/resources/frontendpolicy"
	"github.com/caberdo/terraform-provider-agentgateway/internal/resources/llmapikey"
	"github.com/caberdo/terraform-provider-agentgateway/internal/resources/llmmodel"
	"github.com/caberdo/terraform-provider-agentgateway/internal/resources/llmpolicy"
	"github.com/caberdo/terraform-provider-agentgateway/internal/resources/llmprovider"
	"github.com/caberdo/terraform-provider-agentgateway/internal/resources/llmsettings"
	"github.com/caberdo/terraform-provider-agentgateway/internal/resources/llmvirtualmodel"
	"github.com/caberdo/terraform-provider-agentgateway/internal/resources/mcppolicy"
	"github.com/caberdo/terraform-provider-agentgateway/internal/resources/mcpsettings"
	"github.com/caberdo/terraform-provider-agentgateway/internal/resources/mcptarget"
	"github.com/caberdo/terraform-provider-agentgateway/internal/resources/modelcatalog"
	"github.com/caberdo/terraform-provider-agentgateway/internal/resources/trafficgateway"
	"github.com/caberdo/terraform-provider-agentgateway/internal/resources/trafficroute"
	"github.com/caberdo/terraform-provider-agentgateway/internal/resources/traffictcproute"
	"github.com/caberdo/terraform-provider-agentgateway/internal/resources/uipolicy"
	"github.com/hashicorp/go-version"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	providerschema "github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

const (
	minimumGatewayVersion     = "1.7.0"
	readOnlyStorageDiagnostic = "The agentgateway control plane is configured with config.storage.mode = readOnly. Set config.storage.mode to hybrid (or file) and restart the gateway before managing resources with Terraform."
)

// AgentgatewayProvider implements the Terraform plugin framework provider.
type AgentgatewayProvider struct {
	version string
}

// New returns a factory for the agentgateway provider.
func New(version string) func() provider.Provider {
	return func() provider.Provider { return &AgentgatewayProvider{version: version} }
}

func (p *AgentgatewayProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "agentgateway"
	resp.Version = p.version
}

func (p *AgentgatewayProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = providerschema.Schema{
		Description: "Configure the agentgateway provider.",
		Attributes: map[string]providerschema.Attribute{
			"endpoint": providerschema.StringAttribute{
				Description: "The admin address or gateway port serving the UI/API. No /ui or /api suffix.",
				Optional:    true,
			},
			"token": providerschema.StringAttribute{
				Description: "Bearer token for the gateway-served UI/API path.",
				Optional:    true,
				Sensitive:   true,
			},
			"username": providerschema.StringAttribute{
				Description: "Username for basic authentication.",
				Optional:    true,
			},
			"password": providerschema.StringAttribute{
				Description: "Password for basic authentication.",
				Optional:    true,
				Sensitive:   true,
			},
			"http_headers": providerschema.MapAttribute{
				Description: "Additional HTTP headers added to every request.",
				Optional:    true,
				Sensitive:   true,
				ElementType: types.StringType,
			},
			"timeout": providerschema.StringAttribute{
				Description: "Per-request timeout as a Go duration string.",
				Optional:    true,
			},
			"ca_cert": providerschema.StringAttribute{
				Description: "PEM-encoded CA certificate.",
				Optional:    true,
			},
			"ca_cert_file": providerschema.StringAttribute{
				Description: "Path to a PEM-encoded CA certificate file.",
				Optional:    true,
			},
			"client_cert": providerschema.StringAttribute{
				Description: "PEM-encoded client certificate for mTLS.",
				Optional:    true,
			},
			"client_cert_file": providerschema.StringAttribute{
				Description: "Path to a PEM-encoded client certificate file for mTLS.",
				Optional:    true,
			},
			"client_key": providerschema.StringAttribute{
				Description: "PEM-encoded client key for mTLS.",
				Optional:    true,
				Sensitive:   true,
			},
			"client_key_file": providerschema.StringAttribute{
				Description: "Path to a PEM-encoded client key file for mTLS.",
				Optional:    true,
			},
			"insecure_skip_verify": providerschema.BoolAttribute{
				Description: "Skip TLS certificate verification. For testing only.",
				Optional:    true,
			},
			"verify": providerschema.BoolAttribute{
				Description: "Probe /api/runtime during provider configuration.",
				Optional:    true,
			},
			"storage_mode_check": providerschema.StringAttribute{
				Description: "Action when the gateway is in readOnly mode: error, warn or ignore.",
				Optional:    true,
				Validators:  []validator.String{stringvalidator.OneOf("error", "warn", "ignore")},
			},
		},
	}
}

func (p *AgentgatewayProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var cfg Config
	if diags := req.Config.Get(ctx, &cfg); diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	resolved := cfg.Resolve()

	if diags := cfg.Validate(); diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	httpClient, err := cfg.BuildHTTPClient()
	if err != nil {
		resp.Diagnostics.AddError("Failed to build HTTP client", err.Error())
		return
	}

	headers, err := cfg.AuthHeaders()
	if err != nil {
		resp.Diagnostics.AddError("Failed to build authentication headers", err.Error())
		return
	}

	c, err := client.New(client.Config{
		BaseURL:    resolved.Endpoint,
		HTTPClient: httpClient,
		Headers:    headers,
		Timeout:    resolved.Timeout,
		UserAgent:  fmt.Sprintf("terraform-provider-agentgateway/%s", p.version),
	})
	if err != nil {
		resp.Diagnostics.AddError("Failed to create agentgateway client", err.Error())
		return
	}

	var storeMode, runtimeVersion string
	if resolved.Verify {
		info, err := c.Runtime(ctx)
		if err != nil {
			tflog.Warn(ctx, "agentgateway runtime probe failed; continuing without storage-mode or version information", map[string]any{"error": err.Error()})
		} else {
			storeMode = info.ConfigStoreMode
			runtimeVersion = info.Version

			if warn := versionWarning(runtimeVersion); warn != "" {
				resp.Diagnostics.AddWarning("Unsupported agentgateway version", warn)
			}
		}
	}

	if resolved.Verify && storeMode == "readOnly" {
		switch resolved.StorageModeCheck {
		case "error":
			resp.Diagnostics.AddError(
				"Gateway storage is read-only",
				readOnlyStorageDiagnostic,
			)
			return
		case "warn":
			tflog.Warn(ctx, "agentgateway storage mode is readOnly; writes will fail")
		}
	}

	data := &providerdata.Data{
		Client:    c,
		Timeout:   resolved.Timeout,
		StoreMode: storeMode,
		Version:   runtimeVersion,
	}
	resp.ResourceData = data
	resp.DataSourceData = data
}

func (p *AgentgatewayProvider) Resources(context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		llmprovider.New,
		llmmodel.New,
		llmvirtualmodel.New,
		llmapikey.New,
		llmpolicy.New,
		llmsettings.New,
		mcptarget.New,
		mcppolicy.New,
		mcpsettings.New,
		trafficgateway.New,
		trafficroute.New,
		traffictcproute.New,
		uipolicy.New,
		frontendpolicy.New,
		modelcatalog.New,
	}
}

func (p *AgentgatewayProvider) DataSources(context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		runtime.New,
		llmmodels.New,
		llmproviders.New,
		llmapikeys.New,
		mcptargets.New,
		trafficgateways.New,
		trafficroutes.New,
		effectiveconfig.New,
		configresources.New,
	}
}

func versionWarning(runtimeVersion string) string {
	if runtimeVersion == "" {
		return ""
	}
	v, err := version.NewVersion(runtimeVersion)
	if err != nil {
		return ""
	}
	floor, err := version.NewVersion(minimumGatewayVersion)
	if err != nil {
		return ""
	}
	if v.LessThan(floor) {
		return fmt.Sprintf("The agentgateway version %s is below the minimum supported version %s. Upgrade the gateway or set verify = false to suppress this warning.", runtimeVersion, minimumGatewayVersion)
	}
	return ""
}
