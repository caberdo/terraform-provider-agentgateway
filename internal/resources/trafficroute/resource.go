package trafficroute

import (
	"github.com/caberdo/terraform-provider-agentgateway/internal/configresource"
	"github.com/caberdo/terraform-provider-agentgateway/internal/resourceutil"
	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// pathMatchOptions are the mutually exclusive path match variants (spec §7.11).
var pathMatchOptions = []path.Expression{
	path.MatchRelative().AtParent().AtName("exact"),
	path.MatchRelative().AtParent().AtName("prefix"),
	path.MatchRelative().AtParent().AtName("regex"),
}

// valueMatchOptions are the mutually exclusive exact/regex variants of a header
// or query match.
var valueMatchOptions = []path.Expression{
	path.MatchRelative().AtParent().AtName("exact"),
	path.MatchRelative().AtParent().AtName("regex"),
}

// backendOptions are the mutually exclusive backend variants (spec §7.11).
var backendOptions = []path.Expression{
	path.MatchRelative().AtParent().AtName("host"),
	path.MatchRelative().AtParent().AtName("backend"),
	path.MatchRelative().AtParent().AtName("service_json"),
	path.MatchRelative().AtParent().AtName("mcp_json"),
	path.MatchRelative().AtParent().AtName("ai_json"),
	path.MatchRelative().AtParent().AtName("json"),
}

// New returns the agentgateway_traffic_route resource.
func New() resource.Resource {
	return resourceutil.CRUD(resourceutil.Spec{
		TypeName: "agentgateway_traffic_route",
		Kind:     configresource.KindTrafficRoute,
		Schema:   resourceSchema(),
		New:      func() resourceutil.KindResource { return newModel() },
	})
}

// resourceSchema returns the Terraform schema for a traffic route.
func resourceSchema() schema.Schema {
	return schema.Schema{
		MarkdownDescription: "Manages a traffic route: the HTTP matches and backends that agentgateway serves on one or more gateways.",
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
				Optional:   true,
				CustomType: jsontypes.NormalizedType{},
				// Route-level policies carry the same credential families as
				// the gateway-level ones (basicAuth, apiKey, extAuthz,
				// JWT/OIDC), so the whole attribute is marked sensitive.
				Sensitive:           true,
				MarkdownDescription: "Route-level policies as JSON.",
			},
		},
		Blocks: map[string]schema.Block{
			"matches": schema.ListNestedBlock{
				MarkdownDescription: "Request matches. A request is served when any match applies.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"method": schema.StringAttribute{
							Optional:            true,
							MarkdownDescription: "HTTP method to match, for example `GET`.",
						},
					},
					Blocks: map[string]schema.Block{
						"path": schema.ListNestedBlock{
							MarkdownDescription: "Path match. Exactly one of `exact`, `prefix` or `regex` may be set.",
							Validators: []validator.List{
								listvalidator.SizeAtMost(1),
							},
							NestedObject: schema.NestedBlockObject{
								Attributes: map[string]schema.Attribute{
									"exact": schema.StringAttribute{
										Optional:            true,
										MarkdownDescription: "Match the path exactly.",
										Validators: []validator.String{
											stringvalidator.ExactlyOneOf(pathMatchOptions...),
										},
									},
									"prefix": schema.StringAttribute{
										Optional:            true,
										MarkdownDescription: "Match a path prefix.",
										Validators: []validator.String{
											stringvalidator.ExactlyOneOf(pathMatchOptions...),
										},
									},
									"regex": schema.StringAttribute{
										Optional:            true,
										MarkdownDescription: "Match the path with a regular expression.",
										Validators: []validator.String{
											stringvalidator.ExactlyOneOf(pathMatchOptions...),
										},
									},
								},
							},
						},
						"headers": schema.ListNestedBlock{
							MarkdownDescription: "Header matches. All headers must match.",
							NestedObject: schema.NestedBlockObject{
								Attributes: map[string]schema.Attribute{
									"name": schema.StringAttribute{
										Required:            true,
										MarkdownDescription: "Header name.",
									},
									"exact": schema.StringAttribute{
										Optional:            true,
										MarkdownDescription: "Match the header value exactly. Exactly one of `exact` or `regex` may be set.",
										Validators: []validator.String{
											stringvalidator.ExactlyOneOf(valueMatchOptions...),
										},
									},
									"regex": schema.StringAttribute{
										Optional:            true,
										MarkdownDescription: "Match the header value with a regular expression. Exactly one of `exact` or `regex` may be set.",
										Validators: []validator.String{
											stringvalidator.ExactlyOneOf(valueMatchOptions...),
										},
									},
								},
							},
						},
						"query": schema.ListNestedBlock{
							MarkdownDescription: "Query parameter matches. All parameters must match.",
							NestedObject: schema.NestedBlockObject{
								Attributes: map[string]schema.Attribute{
									"name": schema.StringAttribute{
										Required:            true,
										MarkdownDescription: "Query parameter name.",
									},
									"exact": schema.StringAttribute{
										Optional:            true,
										MarkdownDescription: "Match the parameter value exactly. Exactly one of `exact` or `regex` may be set.",
										Validators: []validator.String{
											stringvalidator.ExactlyOneOf(valueMatchOptions...),
										},
									},
									"regex": schema.StringAttribute{
										Optional:            true,
										MarkdownDescription: "Match the parameter value with a regular expression. Exactly one of `exact` or `regex` may be set.",
										Validators: []validator.String{
											stringvalidator.ExactlyOneOf(valueMatchOptions...),
										},
									},
								},
							},
						},
					},
				},
			},
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
							MarkdownDescription: "Direct host backend as a bare string, for example `example.com:8000` or a URI such as `http://127.0.0.1:8080`. Exactly one backend variant may be set.",
							Validators: []validator.String{
								stringvalidator.ExactlyOneOf(backendOptions...),
							},
						},
						"backend": schema.StringAttribute{
							Optional:            true,
							MarkdownDescription: "Name of a backend to route to, as a bare string. Exactly one backend variant may be set.",
							Validators: []validator.String{
								stringvalidator.ExactlyOneOf(backendOptions...),
							},
						},
						"service_json": schema.StringAttribute{
							Optional:            true,
							CustomType:          jsontypes.NormalizedType{},
							MarkdownDescription: "Service backend as JSON, `{name, port}`. Exactly one backend variant may be set.",
							Validators: []validator.String{
								stringvalidator.ExactlyOneOf(backendOptions...),
							},
						},
						"mcp_json": schema.StringAttribute{
							Optional:            true,
							CustomType:          jsontypes.NormalizedType{},
							MarkdownDescription: "MCP backend as JSON. Exactly one backend variant may be set.",
							Validators: []validator.String{
								stringvalidator.ExactlyOneOf(backendOptions...),
							},
						},
						"ai_json": schema.StringAttribute{
							Optional:            true,
							CustomType:          jsontypes.NormalizedType{},
							MarkdownDescription: "AI backend as JSON. Exactly one backend variant may be set.",
							Validators: []validator.String{
								stringvalidator.ExactlyOneOf(backendOptions...),
							},
						},
						"policies_json": schema.StringAttribute{
							Optional:   true,
							CustomType: jsontypes.NormalizedType{},
							// Backend policies carry TLS material and authorization
							// rules, so the whole block is marked sensitive.
							Sensitive:           true,
							MarkdownDescription: "Policies that apply to this backend only (for example `backendTLS` or `authorization`) as JSON. Sibling of the backend variant, not a variant itself. Typed backend keys are rejected inside `json`; set them here or on the sibling attributes.",
						},
						"json": schema.StringAttribute{
							Optional:            true,
							CustomType:          jsontypes.NormalizedType{},
							MarkdownDescription: "Any other backend as JSON (`aws`, `dynamic`, `internal`, `routeGroup`). Typed keys (`weight`, `host`, `backend`, `service`, `mcp`, `ai`, `policies`) are rejected inside `json`; set them as the sibling attributes. Exactly one backend variant may be set.",
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
