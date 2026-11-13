package configresource

import (
	"slices"
	"testing"
)

func TestAllKindsAreUniqueAndFifteen(t *testing.T) {
	all := All()
	if len(all) != 15 {
		t.Fatalf("expected 15 kinds, got %d: %v", len(all), all)
	}

	seen := make(map[Kind]struct{}, len(all))
	for _, k := range all {
		if _, ok := seen[k]; ok {
			t.Fatalf("duplicate kind %q", k)
		}
		seen[k] = struct{}{}
	}

	wantPolicies := []Kind{
		KindLLMPolicy,
		KindMCPPolicy,
		KindUIPolicy,
		KindFrontendPolicy,
	}
	for _, k := range all {
		got := k.IsPolicy()
		want := slices.Contains(wantPolicies, k)
		if got != want {
			t.Errorf("IsPolicy() for %q = %v, want %v", k, got, want)
		}
	}

	wantSingletons := []Kind{
		KindLLMSettings,
		KindMCPSettings,
		KindModelCatalog,
	}
	for _, k := range all {
		got := k.IsSingleton()
		want := slices.Contains(wantSingletons, k)
		if got != want {
			t.Errorf("IsSingleton() for %q = %v, want %v", k, got, want)
		}
	}
}
