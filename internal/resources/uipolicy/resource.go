// Package uipolicy implements the agentgateway_ui_policy resource.
//
// The resource is deliberately thin: the shared CRUD harness (resourceutil)
// owns the lifecycle, the policy schema is the standard one and the model is
// the shared ui.policy model. Policy bodies are stored verbatim, so the body
// may be any JSON value.
package uipolicy

import (
	"github.com/caberdo/terraform-provider-agentgateway/internal/configresource"
	"github.com/caberdo/terraform-provider-agentgateway/internal/resourceutil"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

// New returns the agentgateway_ui_policy resource.
func New() resource.Resource {
	return resourceutil.CRUD(resourceutil.Spec{
		TypeName: "agentgateway_ui_policy",
		Kind:     configresource.KindUIPolicy,
		Schema:   resourceutil.PolicySchema(configresource.UIPolicyKeys),
		New: func() resourceutil.KindResource {
			return resourceutil.NewPolicyModel(configresource.KindUIPolicy)
		},
	})
}
