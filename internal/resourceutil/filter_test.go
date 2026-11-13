package resourceutil_test

import (
	"testing"

	"github.com/caberdo/terraform-provider-agentgateway/internal/configresource"
	"github.com/caberdo/terraform-provider-agentgateway/internal/resourceutil"
)

func TestFilterAndSortOrdersAndFilters(t *testing.T) {
	resources := []configresource.Resource{
		{Kind: configresource.KindLLMProvider, ID: "beta"},
		{Kind: configresource.KindLLMProvider, ID: "gamma"},
		{Kind: configresource.KindLLMProvider, ID: "alpha"},
		{Kind: configresource.KindLLMProvider, ID: "alphabet"},
	}

	got := resourceutil.FilterAndSort(resources, "")
	if len(got) != 4 {
		t.Fatalf("expected 4 resources, got %d", len(got))
	}
	wantAll := []string{"alpha", "alphabet", "beta", "gamma"}
	for i, want := range wantAll {
		if got[i].ID != want {
			t.Errorf("all[%d] = %q, want %q", i, got[i].ID, want)
		}
	}

	filtered := resourceutil.FilterAndSort(resources, "alph")
	if len(filtered) != 2 {
		t.Fatalf("expected 2 resources for prefix alph, got %d", len(filtered))
	}
	if filtered[0].ID != "alpha" || filtered[1].ID != "alphabet" {
		t.Errorf("unexpected filtered order: %v", []string{filtered[0].ID, filtered[1].ID})
	}

	if none := resourceutil.FilterAndSort(resources, "zzz"); len(none) != 0 {
		t.Errorf("expected no matches, got %v", none)
	}

	// The input slice must not be reordered in place.
	if resources[0].ID != "beta" || resources[2].ID != "alpha" {
		t.Errorf("FilterAndSort mutated its input: %v", resources)
	}
}
