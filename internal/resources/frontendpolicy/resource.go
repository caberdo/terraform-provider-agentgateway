// Package frontendpolicy implements the agentgateway_frontend_policy resource.
//
// The resource is deliberately thin: the shared CRUD harness (resourceutil)
// owns the lifecycle, the policy schema is the standard one and the model is
// the shared frontend.policy model. Policy bodies are stored verbatim, so the
// body may be any JSON value.
package frontendpolicy

import (
	"github.com/caberdo/terraform-provider-agentgateway/internal/configresource"
	"github.com/caberdo/terraform-provider-agentgateway/internal/resourceutil"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

// New returns the agentgateway_frontend_policy resource.
func New() resource.Resource {
	return resourceutil.CRUD(resourceutil.Spec{
		TypeName: "agentgateway_frontend_policy",
		Kind:     configresource.KindFrontendPolicy,
		Schema:   resourceutil.PolicySchema(configresource.FrontendPolicyKeys),
		New: func() resourceutil.KindResource {
			return resourceutil.NewPolicyModel(configresource.KindFrontendPolicy)
		},
	})
}
