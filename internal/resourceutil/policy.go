package resourceutil

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/caberdo/terraform-provider-agentgateway/internal/configresource"
	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// apiKeyPolicyKeysDiagnostic is the single diagnostic used for every invalid
// llm.policy/apiKey body: an object containing keys, or any non-object body.
const apiKeyPolicyKeysDiagnostic = "llm.policy/apiKey must not include keys; use agentgateway_llm_api_key resources"

// PolicySchema returns the standard schema for a policy resource. keys is the
// policy enum for the kind; it is never retyped by a resource package.
func PolicySchema(keys []string) schema.Schema {
	return schema.Schema{
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Policy key, which is also the resource id. Changing this replaces the policy.",
				Validators: []validator.String{
					stringvalidator.OneOf(keys...),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"spec": schema.StringAttribute{
				Required:            true,
				CustomType:          jsontypes.NormalizedType{},
				MarkdownDescription: "Policy body as JSON. Any JSON value is accepted; use `jsonencode(...)`.",
			},
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Policy key as stored by the gateway.",
			},
			"revision": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Server revision of the policy.",
			},
		},
	}
}

// policyFields is the state shape shared by the four policy kinds. It is
// embedded by value so the framework's reflection can populate the promoted
// attributes.
type policyFields struct {
	Name     types.String         `tfsdk:"name"`
	Spec     jsontypes.Normalized `tfsdk:"spec"`
	IDValue  types.String         `tfsdk:"id"`
	Revision types.Int64          `tfsdk:"revision"`
}

// ID returns the policy key, which is the resource id.
func (m *policyFields) ID() string { return m.Name.ValueString() }

// value returns the policy body verbatim. Policy bodies are not always
// objects, so no shape is assumed beyond the llm.policy/apiKey rule.
func (m *policyFields) value(kind configresource.Kind, _ context.Context, diags *diag.Diagnostics) json.RawMessage {
	if m.Spec.IsNull() || m.Spec.IsUnknown() {
		diags.AddError(
			fmt.Sprintf("Invalid %s spec", kind),
			"spec is required and must be a JSON value; use jsonencode(...)",
		)
		return nil
	}

	raw := json.RawMessage(m.Spec.ValueString())
	if kind == configresource.KindLLMPolicy && m.Name.ValueString() == "apiKey" {
		if err := validateAPIKeyPolicy(raw); err != nil {
			diags.AddError(fmt.Sprintf("Invalid %s/apiKey spec", kind), err.Error())
			return nil
		}
	}
	return raw
}

// SetValue stores the policy body returned by the gateway.
func (m *policyFields) SetValue(_ context.Context, value json.RawMessage, _ *diag.Diagnostics) {
	if len(value) == 0 {
		return
	}
	m.Spec = jsontypes.NewNormalizedValue(string(value))
}

// SetServerMeta stores the computed id and revision. An imported policy has no
// name in state yet, so the name is seeded from the id.
func (m *policyFields) SetServerMeta(id string, revision int64) {
	m.IDValue = types.StringValue(id)
	m.Revision = types.Int64Value(revision)
	if m.Name.IsNull() || m.Name.IsUnknown() {
		m.Name = types.StringValue(id)
	}
}

// The four concrete models exist because the framework rebuilds the model
// struct when reading a plan or state, which would discard a kind stored in a
// field. The kind therefore lives in the type.

var (
	_ KindResource = (*llmPolicyModel)(nil)
	_ KindResource = (*mcpPolicyModel)(nil)
	_ KindResource = (*uiPolicyModel)(nil)
	_ KindResource = (*frontendPolicyModel)(nil)
)

type llmPolicyModel struct{ policyFields }

func (m *llmPolicyModel) Kind() configresource.Kind { return configresource.KindLLMPolicy }

func (m *llmPolicyModel) Value(ctx context.Context, diags *diag.Diagnostics) json.RawMessage {
	return m.value(configresource.KindLLMPolicy, ctx, diags)
}

type mcpPolicyModel struct{ policyFields }

func (m *mcpPolicyModel) Kind() configresource.Kind { return configresource.KindMCPPolicy }

func (m *mcpPolicyModel) Value(ctx context.Context, diags *diag.Diagnostics) json.RawMessage {
	return m.value(configresource.KindMCPPolicy, ctx, diags)
}

type uiPolicyModel struct{ policyFields }

func (m *uiPolicyModel) Kind() configresource.Kind { return configresource.KindUIPolicy }

func (m *uiPolicyModel) Value(ctx context.Context, diags *diag.Diagnostics) json.RawMessage {
	return m.value(configresource.KindUIPolicy, ctx, diags)
}

type frontendPolicyModel struct{ policyFields }

func (m *frontendPolicyModel) Kind() configresource.Kind { return configresource.KindFrontendPolicy }

func (m *frontendPolicyModel) Value(ctx context.Context, diags *diag.Diagnostics) json.RawMessage {
	return m.value(configresource.KindFrontendPolicy, ctx, diags)
}

// NewPolicyModel returns a KindResource for a policy kind.
func NewPolicyModel(kind configresource.Kind) KindResource {
	switch kind {
	case configresource.KindLLMPolicy:
		return &llmPolicyModel{}
	case configresource.KindMCPPolicy:
		return &mcpPolicyModel{}
	case configresource.KindUIPolicy:
		return &uiPolicyModel{}
	case configresource.KindFrontendPolicy:
		return &frontendPolicyModel{}
	default:
		return nil
	}
}

// validateAPIKeyPolicy enforces C11: the apiKey policy body must be an object
// and must not carry inline keys, because keys are managed by
// agentgateway_llm_api_key resources.
func validateAPIKeyPolicy(raw json.RawMessage) error {
	body, isObject, err := decodeJSONObject(raw)
	if err != nil || !isObject {
		return errors.New(apiKeyPolicyKeysDiagnostic)
	}
	if _, ok := body["keys"]; ok {
		return errors.New(apiKeyPolicyKeysDiagnostic)
	}
	return nil
}
