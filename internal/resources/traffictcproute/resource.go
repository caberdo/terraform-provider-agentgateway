package traffictcproute

import (
	"github.com/caberdo/terraform-provider-agentgateway/internal/configresource"
	"github.com/caberdo/terraform-provider-agentgateway/internal/resourceutil"
	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// backendOptions are the mutually exclusive backend variants (spec §7.12).
var backendOptions = []path.Expression{
	path.MatchRelative().AtParent().AtName("host"),
	path.MatchRelative().AtParent().AtName("backend"),
	path.MatchRelative().AtParent().AtName("service_json"),
	path.MatchRelative().AtParent().AtName("dynamic_json"),
	path.MatchRelative().AtParent().AtName("json"),
}

// New returns the agentgateway_traffic_tcp_route resource.
func New() resource.Resource {
	return resourceutil.CRUD(resourceutil.Spec{
		TypeName: "agentgateway_traffic_tcp_route",
		Kind:     configresource.KindTrafficTCPRoute,
		Schema:   resourceSchema(),
		New:      func() resourceutil.KindResource { return newModel() },
	})
}

// resourceSchema returns the Terraform schema for a TCP route.
func resourceSchema() schema.Schema {
	return schema.Schema{
		MarkdownDescription: "Manages a TCP traffic route: the backends that agentgateway serves for raw TCP traffic on one or more gateways.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The route name, which is also the server-side id.",
			},
			"revision": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "The server revision of the stored route.",
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The route name. Changing it renames the route in place.",
			},
			"gateways": schema.ListAttribute{
				Optional:            true,
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Gateways that serve this route, as `<gateway>` or `<gateway>/<listener>`. The server defaults to `default`.",
			},
			"namespace": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Namespace of the route.",
			},
			"rule_name": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Rule name used by authorization policies.",
			},
			"hostnames": schema.ListAttribute{
				Optional:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Hostnames served by this route. Wildcards are allowed; unset serves all hostnames.",
			},
			"policies_json": schema.StringAttribute{
				Optional:            true,
				CustomType:          jsontypes.NormalizedType{},
				MarkdownDescription: "Route-level policies as JSON.",
			},
		},
		Blocks: map[string]schema.Block{
			"backends": schema.ListNestedBlock{
				MarkdownDescription: "Destinations of the route. Exactly one variant may be set per backend.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"weight": schema.Int64Attribute{
							Optional:            true,
							Computed:            true,
							MarkdownDescription: "Relative weight of this backend. The server default is `1`.",
							Validators: []validator.Int64{
								int64validator.AtLeast(0),
							},
						},
						"host": schema.StringAttribute{
							Optional:            true,
							MarkdownDescription: "Backend address as `host:port`, for example `postgres.example.com:5432`. Exactly one backend variant may be set.",
							Validators: []validator.String{
								stringvalidator.ExactlyOneOf(backendOptions...),
							},
						},
						"backend": schema.StringAttribute{
							Optional:            true,
							MarkdownDescription: "Name of a backend defined in the gateway's top-level backends list. Exactly one backend variant may be set.",
							Validators: []validator.String{
								stringvalidator.ExactlyOneOf(backendOptions...),
							},
						},
						"service_json": schema.StringAttribute{
							Optional:            true,
							CustomType:          jsontypes.NormalizedType{},
							MarkdownDescription: "Service backend as JSON (`{name: \"<namespace>/<hostname>\", port: <port>}`). Exactly one backend variant may be set.",
							Validators: []validator.String{
								stringvalidator.ExactlyOneOf(backendOptions...),
							},
						},
						"dynamic_json": schema.StringAttribute{
							Optional:            true,
							CustomType:          jsontypes.NormalizedType{},
							MarkdownDescription: "Dynamic backend as JSON (`{target: <CEL expression>}`). Exactly one backend variant may be set.",
							Validators: []validator.String{
								stringvalidator.ExactlyOneOf(backendOptions...),
							},
						},
						"policies_json": schema.StringAttribute{
							Optional:   true,
							CustomType: jsontypes.NormalizedType{},
							// Backend policies carry TLS material, so the whole
							// block is marked sensitive.
							Sensitive:           true,
							MarkdownDescription: "Policies that apply to this backend only (a TCP backend accepts `backendTLS`) as JSON. Sibling of the backend variant, not a variant itself. Typed backend keys are rejected inside `json`; set them here or on the sibling attributes.",
						},
						"json": schema.StringAttribute{
							Optional:            true,
							CustomType:          jsontypes.NormalizedType{},
							MarkdownDescription: "Any other backend as JSON (`aws`, `internal`, `routeGroup`). Typed keys (`weight`, `host`, `backend`, `service`, `dynamic`, `policies`) are rejected inside `json`; set them as the sibling attributes. Exactly one backend variant may be set.",
							Validators: []validator.String{
								stringvalidator.ExactlyOneOf(backendOptions...),
							},
						},
					},
				},
			},
		},
	}
}
