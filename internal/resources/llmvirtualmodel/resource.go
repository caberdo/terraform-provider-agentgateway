package llmvirtualmodel

import (
	"context"
	"fmt"

	"github.com/caberdo/terraform-provider-agentgateway/internal/configresource"
	"github.com/caberdo/terraform-provider-agentgateway/internal/resourceutil"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// TypeName is the Terraform resource type.
const TypeName = "agentgateway_llm_virtual_model"

// New returns the llm.virtualModel resource.
func New() resource.Resource {
	return resourceutil.CRUD(resourceutil.Spec{
		TypeName: TypeName,
		Kind:     configresource.KindLLMVirtualModel,
		Schema: schema.Schema{
			MarkdownDescription: "Manages an `llm.virtualModel`: one client-facing model name routed across concrete models by weight, priority or request conditions.",
			Attributes:          attributes(),
			Blocks:              blocks(),
		},
		New: func() resourceutil.KindResource { return &virtualModelModel{} },
	})
}

func attributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"name": schema.StringAttribute{
			Required:            true,
			MarkdownDescription: "Public model name clients request.",
			Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
		},
		"id": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "Stored id, derived from `name`.",
		},
		"revision": schema.Int64Attribute{
			Computed:            true,
			MarkdownDescription: "Server revision of the stored resource; `0` in file mode.",
		},
	}
}

func blocks() map[string]schema.Block {
	return map[string]schema.Block{
		"weighted": schema.ListNestedBlock{
			MarkdownDescription: "Split traffic between targets by relative weight.",
			Validators:          routingValidators("weighted"),
			NestedObject: schema.NestedBlockObject{
				Blocks: map[string]schema.Block{
					"targets": schema.ListNestedBlock{
						MarkdownDescription: "Models that receive traffic, with an optional relative weight.",
						Validators:          []validator.List{listvalidator.SizeAtLeast(1)},
						NestedObject: schema.NestedBlockObject{
							Attributes: map[string]schema.Attribute{
								"model": schema.StringAttribute{
									Required:            true,
									MarkdownDescription: "Name of an `agentgateway_llm_model` to route to.",
									Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
								},
								"weight": schema.Int64Attribute{
									Optional:            true,
									Computed:            true,
									MarkdownDescription: "Relative weight; defaults to `1` server-side when omitted.",
									Validators:          []validator.Int64{int64validator.AtLeast(0)},
								},
							},
						},
					},
				},
			},
		},
		"failover": schema.ListNestedBlock{
			MarkdownDescription: "Prefer lower priority groups and fail over when a model degrades.",
			Validators:          routingValidators("failover"),
			NestedObject: schema.NestedBlockObject{
				Blocks: map[string]schema.Block{
					"targets": schema.ListNestedBlock{
						MarkdownDescription: "Models grouped by failover priority.",
						Validators:          []validator.List{listvalidator.SizeAtLeast(1)},
						NestedObject: schema.NestedBlockObject{
							Attributes: map[string]schema.Attribute{
								"model": schema.StringAttribute{
									Required:            true,
									MarkdownDescription: "Name of an `agentgateway_llm_model` to route to.",
									Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
								},
								"priority": schema.Int64Attribute{
									Optional:            true,
									Computed:            true,
									MarkdownDescription: "Priority group; lower values are preferred.",
									Validators:          []validator.Int64{int64validator.AtLeast(0)},
								},
							},
						},
					},
				},
			},
		},
		"conditional": schema.ListNestedBlock{
			MarkdownDescription: "Select a target by request context.",
			Validators:          routingValidators("conditional"),
			NestedObject: schema.NestedBlockObject{
				Blocks: map[string]schema.Block{
					"targets": schema.ListNestedBlock{
						MarkdownDescription: "Models selected by CEL `when` expressions; the final target may omit `when` as the fallback.",
						Validators:          []validator.List{listvalidator.SizeAtLeast(1)},
						NestedObject: schema.NestedBlockObject{
							Attributes: map[string]schema.Attribute{
								"model": schema.StringAttribute{
									Required:            true,
									MarkdownDescription: "Name of an `agentgateway_llm_model` to route to.",
									Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
								},
								"when": schema.StringAttribute{
									Optional:            true,
									MarkdownDescription: "CEL expression selecting this target. May be omitted only on the final target.",
									Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
								},
							},
						},
					},
				},
			},
		},
	}
}

// routingValidators enforces the exactly-one-of rule across the three routing
// blocks and rejects a declared block with no targets at plan time. Value()
// repeats the exclusivity check so a hand-built model cannot bypass it.
func routingValidators(mode string) []validator.List {
	return []validator.List{
		listvalidator.SizeAtLeast(1),
		listvalidator.SizeAtMost(1),
		listvalidator.ExactlyOneOf(
			path.MatchRoot("weighted"),
			path.MatchRoot("failover"),
			path.MatchRoot("conditional"),
		),
		routingBlockHasTargets{mode: mode},
	}
}

// routingBlockHasTargets rejects a routing block that is declared but carries
// no targets block. SizeAtLeast on the routing block only counts block
// instances, and `weighted {}` is one instance, so the nested targets list
// would stay null and its own SizeAtLeast validator would skip it. This
// validator looks inside the block so the failure happens at plan time instead
// of in Value() at apply time.
type routingBlockHasTargets struct {
	mode string
}

var _ validator.List = routingBlockHasTargets{}

func (v routingBlockHasTargets) Description(_ context.Context) string {
	return fmt.Sprintf("%s requires at least one targets block", v.mode)
}

func (v routingBlockHasTargets) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v routingBlockHasTargets) ValidateList(ctx context.Context, req validator.ListRequest, resp *validator.ListResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	for i, element := range req.ConfigValue.Elements() {
		object, ok := element.(types.Object)
		if !ok || object.IsNull() || object.IsUnknown() {
			continue
		}
		targets, ok := object.Attributes()["targets"].(types.List)
		if !ok || targets.IsNull() || len(targets.Elements()) == 0 {
			resp.Diagnostics.AddAttributeError(
				req.Path.AtListIndex(i).AtName("targets"),
				fmt.Sprintf("Invalid %s routing", v.mode),
				v.Description(ctx),
			)
		}
	}
}

// Ensure the constructor satisfies the frozen resource constructor signature.
var _ func() resource.Resource = New
