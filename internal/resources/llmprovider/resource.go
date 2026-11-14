package llmprovider

import (
	"github.com/caberdo/terraform-provider-agentgateway/internal/configresource"
	"github.com/caberdo/terraform-provider-agentgateway/internal/resourceutil"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

// New returns the agentgateway_llm_provider resource.
func New() resource.Resource {
	return resourceutil.CRUD(resourceutil.Spec{
		TypeName: "agentgateway_llm_provider",
		Kind:     configresource.KindLLMProvider,
		Schema:   newSchema(),
		New:      func() resourceutil.KindResource { return &llmProviderModel{} },
	})
}
