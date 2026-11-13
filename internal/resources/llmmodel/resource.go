package llmmodel

import (
	"github.com/caberdo/terraform-provider-agentgateway/internal/configresource"
	"github.com/caberdo/terraform-provider-agentgateway/internal/resourceutil"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

// New returns the agentgateway_llm_model resource.
func New() resource.Resource {
	return resourceutil.CRUD(resourceutil.Spec{
		TypeName: "agentgateway_llm_model",
		Kind:     configresource.KindLLMModel,
		Schema:   newSchema(),
		New:      func() resourceutil.KindResource { return &model{} },
	})
}
