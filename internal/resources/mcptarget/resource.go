package mcptarget

import (
	"github.com/caberdo/terraform-provider-agentgateway/internal/configresource"
	"github.com/caberdo/terraform-provider-agentgateway/internal/resourceutil"
	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/objectvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// New returns the agentgateway_mcp_target resource.
func New() resource.Resource {
	return resourceutil.CRUD(resourceutil.Spec{
		TypeName: "agentgateway_mcp_target",
		Kind:     configresource.KindMCPTarget,
		Schema:   resourceSchema(),
		New:      func() resourceutil.KindResource { return &model{} },
	})
}

// resourceSchema is the Terraform schema for an MCP target. Exactly one
// transport block is required.
func resourceSchema() schema.Schema {
	return schema.Schema{
		MarkdownDescription: "Manages an agentgateway MCP target (`mcp.target`). " +
			"Exactly one transport (`mcp`, `sse`, `stdio` or `openapi`) is required. " +
			"The resource id is the target `name`; renaming updates in place.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Target name, which is also the resource id. Used to prefix tool names when multiplexing.",
			},
			"condition": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Boolean CEL expression that selects this target. Omit to always include the target. Conditions require at least two configured targets.",
			},
			"mcp": schema.SingleNestedAttribute{
				Optional:            true,
				MarkdownDescription: "Streamable HTTP transport. Set either `host` (with an optional `port` and `path`) or `backend` (with an optional `path`), never both.",
				Attributes:          networkAttributes(),
				Validators:          []validator.Object{exactlyOneTransport()},
			},
			"sse": schema.SingleNestedAttribute{
				Optional:            true,
				MarkdownDescription: "Server-Sent Events transport. Set either `host` (with an optional `port` and `path`) or `backend` (with an optional `path`), never both.",
				Attributes:          networkAttributes(),
				Validators:          []validator.Object{exactlyOneTransport()},
			},
			"stdio": schema.SingleNestedAttribute{
				Optional:            true,
				MarkdownDescription: "stdio transport, which starts a local process.",
				Attributes: map[string]schema.Attribute{
					"cmd": schema.StringAttribute{
						Required:            true,
						MarkdownDescription: "Command to start the MCP server.",
					},
					"args": schema.ListAttribute{
						Optional:            true,
						ElementType:         types.StringType,
						MarkdownDescription: "Arguments passed to `cmd`.",
					},
					"env": schema.MapAttribute{
						Optional:            true,
						ElementType:         types.StringType,
						MarkdownDescription: "Environment variables passed to the process.",
					},
					"clear_env": schema.BoolAttribute{
						Optional:            true,
						MarkdownDescription: "When true, the process inherits no environment variables from the gateway.",
					},
				},
				Validators: []validator.Object{exactlyOneTransport()},
			},
			"openapi": schema.SingleNestedAttribute{
				Optional:            true,
				MarkdownDescription: "OpenAPI transport: the operations in an OpenAPI schema are exposed as MCP tools. Set either `host` (with an optional `port` and `path`) or `backend` (with an optional `path`), never both.",
				Attributes:          openAPIAttributes(),
				Validators:          []validator.Object{exactlyOneTransport()},
			},
			"policies_json": schema.StringAttribute{
				Optional:   true,
				CustomType: jsontypes.NormalizedType{},
				// Transport policies carry backend credentials (backendAuth,
				// backendTLS), so the value is a secret in state (design §12).
				Sensitive:           true,
				MarkdownDescription: "Transport policies for connecting to the target's backend, as JSON. Not supported on stdio targets. Marked sensitive because `backendAuth` carries credentials.",
			},
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Target id as stored by the gateway (the `name`).",
			},
			"revision": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Server revision of the target.",
			},
		},
	}
}

// networkAttributes is the shared host/backend shape of the network transports.
func networkAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"host": schema.StringAttribute{
			Optional:            true,
			MarkdownDescription: "Hostname, IP address or full URI of the MCP server, for example `https://example.com` or `example.com:443`. Use only `host` to pass a full URI.",
		},
		"port": schema.Int64Attribute{
			Optional:            true,
			MarkdownDescription: "Port on the MCP server. Only valid with `host`; the `backend` form has no port.",
			Validators: []validator.Int64{
				int64validator.Between(0, 65535),
			},
		},
		"path": schema.StringAttribute{
			Optional:            true,
			MarkdownDescription: "Request path on the MCP server.",
		},
		"backend": schema.StringAttribute{
			Optional:            true,
			MarkdownDescription: "Name of a backend defined in the gateway configuration. Conflicts with `host` and `port`.",
		},
	}
}

// openAPIAttributes is the network shape plus the inline schema.
func openAPIAttributes() map[string]schema.Attribute {
	attributes := networkAttributes()
	attributes["schema_json"] = schema.StringAttribute{
		Optional:   true,
		CustomType: jsontypes.NormalizedType{},
		MarkdownDescription: "OpenAPI schema, as JSON: an inline document string (`jsonencode(\"openapi: 3.0.0...\")`), " +
			"`{url = \"https://...\"}`, or `{file = \"/etc/agentgateway/openapi.json\"}`. The gateway requires a schema for an `openapi` target.",
	}
	return attributes
}

// exactlyOneTransport is the schema-level guard for the transport group; the
// model repeats the check so a plan that bypasses validation still fails.
func exactlyOneTransport() validator.Object {
	return objectvalidator.ExactlyOneOf(
		path.MatchRoot("mcp"),
		path.MatchRoot("sse"),
		path.MatchRoot("stdio"),
		path.MatchRoot("openapi"),
	)
}
