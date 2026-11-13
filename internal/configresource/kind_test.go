package configresource

import (
	"slices"
	"testing"
)

func TestAllKindsAreUniqueAndFifteen(t *testing.T) {
	want := []Kind{
		KindModelCatalog,
		KindLLMProvider,
		KindLLMModel,
		KindLLMVirtualModel,
		KindLLMAPIKey,
		KindLLMPolicy,
		KindLLMSettings,
		KindMCPTarget,
		KindMCPPolicy,
		KindMCPSettings,
		KindTrafficGateway,
		KindTrafficRoute,
		KindTrafficTCPRoute,
		KindUIPolicy,
		KindFrontendPolicy,
	}

	all := All()
	if !slices.Equal(all, want) {
		t.Fatalf("All() = %v, want %v", all, want)
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
