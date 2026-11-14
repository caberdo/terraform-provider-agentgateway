// Package llmpolicy implements the agentgateway_llm_policy resource.
//
// The resource is deliberately thin: the shared CRUD harness (resourceutil)
// owns the lifecycle, the policy schema is the standard one and the model is
// the shared llm.policy model. Policy bodies are stored verbatim, so the body
// may be any JSON value.
package llmpolicy

import (
	"github.com/caberdo/terraform-provider-agentgateway/internal/configresource"
	"github.com/caberdo/terraform-provider-agentgateway/internal/resourceutil"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

// New returns the agentgateway_llm_policy resource.
func New() resource.Resource {
	return resourceutil.CRUD(resourceutil.Spec{
		TypeName: "agentgateway_llm_policy",
		Kind:     configresource.KindLLMPolicy,
		Schema:   resourceutil.PolicySchema(configresource.LLMPolicyKeys),
		New: func() resourceutil.KindResource {
			return resourceutil.NewPolicyModel(configresource.KindLLMPolicy)
		},
	})
}
