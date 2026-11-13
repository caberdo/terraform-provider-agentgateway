package resourceutil

import (
	"slices"
	"strings"

	"github.com/caberdo/terraform-provider-agentgateway/internal/configresource"
)

// FilterAndSort returns a new slice containing the resources whose id starts
// with prefix, sorted by id. An empty prefix keeps every resource. The input
// slice is never reordered.
func FilterAndSort(resources []configresource.Resource, prefix string) []configresource.Resource {
	filtered := make([]configresource.Resource, 0, len(resources))
	for _, resource := range resources {
		if prefix == "" || strings.HasPrefix(resource.ID, prefix) {
			filtered = append(filtered, resource)
		}
	}

	slices.SortFunc(filtered, func(a, b configresource.Resource) int {
		return strings.Compare(a.ID, b.ID)
	})
	return filtered
}
