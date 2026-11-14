package modelcatalog

import (
	"github.com/caberdo/terraform-provider-agentgateway/internal/configresource"
	"github.com/caberdo/terraform-provider-agentgateway/internal/resourceutil"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

// New returns the agentgateway_model_catalog resource.
func New() resource.Resource {
	return resourceutil.CRUD(resourceutil.Spec{
		TypeName: "agentgateway_model_catalog",
		Kind:     configresource.KindModelCatalog,
		Schema:   newSchema(),
		New:      func() resourceutil.KindResource { return newModel() },
	})
}
