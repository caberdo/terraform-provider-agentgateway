package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	providerschema "github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

type AgentgatewayProvider struct{ version string }

func New(version string) func() provider.Provider {
	return func() provider.Provider { return &AgentgatewayProvider{version: version} }
}

func (p *AgentgatewayProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "agentgateway"
	resp.Version = p.version
}

func (p *AgentgatewayProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = providerschema.Schema{Description: "Manage agentgateway standalone configuration."}
}

func (p *AgentgatewayProvider) Configure(context.Context, provider.ConfigureRequest, *provider.ConfigureResponse) {}

func (p *AgentgatewayProvider) Resources(context.Context) []func() resource.Resource { return nil }

func (p *AgentgatewayProvider) DataSources(context.Context) []func() datasource.DataSource { return nil }
