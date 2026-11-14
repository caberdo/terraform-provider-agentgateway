package trafficgateway

import (
	"github.com/caberdo/terraform-provider-agentgateway/internal/configresource"
	"github.com/caberdo/terraform-provider-agentgateway/internal/resourceutil"
	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

// New returns the agentgateway_traffic_gateway resource.
func New() resource.Resource {
	return resourceutil.CRUD(resourceutil.Spec{
		TypeName: "agentgateway_traffic_gateway",
		Kind:     configresource.KindTrafficGateway,
		Schema:   resourceSchema(),
		New:      func() resourceutil.KindResource { return &model{} },
	})
}

// resourceSchema returns the Terraform schema for a traffic gateway.
func resourceSchema() schema.Schema {
	return schema.Schema{
		Description: "Manages a traffic gateway: the listener that routes HTTP, HTTPS, TCP and TLS traffic through agentgateway.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "The gateway name, which is also the server-side id.",
			},
			"revision": schema.Int64Attribute{
				Computed:    true,
				Description: "The server revision of the stored gateway.",
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "The gateway name referenced by routes.",
			},
			"port": schema.Int64Attribute{
				Optional:    true,
				Description: "The port to listen on. Omit it to use the server default.",
			},
			"bind_address": schema.StringAttribute{
				Optional:    true,
				Description: "The IPv4 or IPv6 address to listen on. Omit it to listen on all interfaces.",
			},
			"protocol": schema.StringAttribute{
				Optional:    true,
				Description: "The gateway protocol: HTTP, HTTPS, TCP or TLS. Omit it to default to HTTP, or HTTPS when tls_json is set.",
				Validators:  []validator.String{stringvalidator.OneOf("HTTP", "HTTPS", "TCP", "TLS")},
			},
			"tls_json": schema.StringAttribute{
				Optional:    true,
				CustomType:  jsontypes.NormalizedType{},
				Sensitive:   true,
				Description: "Server TLS configuration. Mutually exclusive with listeners.",
			},
			"oidc_json": schema.StringAttribute{
				Optional:    true,
				CustomType:  jsontypes.NormalizedType{},
				Sensitive:   true,
				Description: "OIDC authorization code flow policy. Mutually exclusive with listeners.",
			},
			"jwt_auth_json": schema.StringAttribute{
				Optional:    true,
				CustomType:  jsontypes.NormalizedType{},
				Sensitive:   true,
				Description: "JWT bearer token authentication policy. Mutually exclusive with listeners.",
			},
			"authorization_json": schema.StringAttribute{
				Optional:    true,
				CustomType:  jsontypes.NormalizedType{},
				Sensitive:   true,
				Description: "Authorization rules for incoming HTTP requests. Mutually exclusive with listeners.",
			},
			"ext_authz_json": schema.StringAttribute{
				Optional:    true,
				CustomType:  jsontypes.NormalizedType{},
				Sensitive:   true,
				Description: "External authorization service policy. Mutually exclusive with listeners.",
			},
			"ext_proc_json": schema.StringAttribute{
				Optional:    true,
				CustomType:  jsontypes.NormalizedType{},
				Sensitive:   true,
				Description: "External processing service policy. Mutually exclusive with listeners.",
			},
			"cors_json": schema.StringAttribute{
				Optional:    true,
				CustomType:  jsontypes.NormalizedType{},
				Sensitive:   true,
				Description: "CORS policy. Mutually exclusive with listeners.",
			},
			"transformations_json": schema.StringAttribute{
				Optional:    true,
				CustomType:  jsontypes.NormalizedType{},
				Sensitive:   true,
				Description: "Request and response transformation policy. Mutually exclusive with listeners.",
			},
			"basic_auth_json": schema.StringAttribute{
				Optional:    true,
				CustomType:  jsontypes.NormalizedType{},
				Sensitive:   true,
				Description: "Basic Auth policy. Mutually exclusive with listeners.",
			},
			"api_key_json": schema.StringAttribute{
				Optional:    true,
				CustomType:  jsontypes.NormalizedType{},
				Sensitive:   true,
				Description: "API key authentication policy. Mutually exclusive with listeners.",
			},
		},
		Blocks: map[string]schema.Block{
			"listeners": schema.ListNestedBlock{
				Description: "Named listeners. When set, only port and bind_address may be configured at the top level.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Optional:    true,
							Description: "The listener name used by gateway references like gateway/listener.",
						},
						"hostname": schema.StringAttribute{
							Optional:    true,
							Description: "The hostname served by this listener. Wildcards are allowed; unset serves all domains.",
						},
						"protocol": schema.StringAttribute{
							Optional:    true,
							Description: "The listener protocol: HTTP, HTTPS, TCP or TLS.",
							Validators:  []validator.String{stringvalidator.OneOf("HTTP", "HTTPS", "TCP", "TLS")},
						},
						"tls_json": schema.StringAttribute{
							Optional:    true,
							CustomType:  jsontypes.NormalizedType{},
							Sensitive:   true,
							Description: "TLS configuration for this listener.",
						},
					},
				},
			},
		},
	}
}
