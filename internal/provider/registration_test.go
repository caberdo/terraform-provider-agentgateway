package provider_test

import (
	"context"
	"sort"
	"testing"

	"github.com/caberdo/terraform-provider-agentgateway/internal/provider"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	fwprovider "github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

// expectedResourceTypes is the resource set from design §7.
var expectedResourceTypes = []string{
	"agentgateway_llm_provider",
	"agentgateway_llm_model",
	"agentgateway_llm_virtual_model",
	"agentgateway_llm_api_key",
	"agentgateway_llm_policy",
	"agentgateway_llm_settings",
	"agentgateway_mcp_target",
	"agentgateway_mcp_policy",
	"agentgateway_mcp_settings",
	"agentgateway_traffic_gateway",
	"agentgateway_traffic_route",
	"agentgateway_traffic_tcp_route",
	"agentgateway_ui_policy",
	"agentgateway_frontend_policy",
	"agentgateway_model_catalog",
}

// expectedDataSourceTypes is the data-source set from design §8.
var expectedDataSourceTypes = []string{
	"agentgateway_runtime",
	"agentgateway_llm_models",
	"agentgateway_llm_providers",
	"agentgateway_llm_api_keys",
	"agentgateway_mcp_targets",
	"agentgateway_traffic_gateways",
	"agentgateway_traffic_routes",
	"agentgateway_effective_config",
	"agentgateway_config_resources",
}

func TestProviderRegistersEverything(t *testing.T) {
	ctx := context.Background()
	p := provider.New("test")()

	resources := p.Resources(ctx)
	if got, want := len(resources), 15; got != want {
		t.Fatalf("expected %d resources, got %d", want, got)
	}
	dataSources := p.DataSources(ctx)
	if got, want := len(dataSources), 9; got != want {
		t.Fatalf("expected %d data sources, got %d", want, got)
	}

	var providerMeta fwprovider.MetadataResponse
	p.Metadata(ctx, fwprovider.MetadataRequest{}, &providerMeta)

	var gotResources []string
	for i, factory := range resources {
		r := factory()
		if r == nil {
			t.Fatalf("resource factory %d returned nil", i)
		}
		var resp resource.MetadataResponse
		r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: providerMeta.TypeName}, &resp)
		if resp.TypeName == "" {
			t.Fatalf("resource factory %d produced an empty type name", i)
		}
		gotResources = append(gotResources, resp.TypeName)
	}

	var gotDataSources []string
	for i, factory := range dataSources {
		d := factory()
		if d == nil {
			t.Fatalf("data source factory %d returned nil", i)
		}
		var resp datasource.MetadataResponse
		d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: providerMeta.TypeName}, &resp)
		if resp.TypeName == "" {
			t.Fatalf("data source factory %d produced an empty type name", i)
		}
		gotDataSources = append(gotDataSources, resp.TypeName)
	}

	assertTypeNames(t, "resources", gotResources, expectedResourceTypes)
	assertTypeNames(t, "data sources", gotDataSources, expectedDataSourceTypes)
}

func assertTypeNames(t *testing.T, label string, got, want []string) {
	t.Helper()

	gotSet := make(map[string]struct{}, len(got))
	for _, name := range got {
		if _, duplicate := gotSet[name]; duplicate {
			t.Errorf("%s: %q registered more than once", label, name)
		}
		gotSet[name] = struct{}{}
	}

	wantSet := make(map[string]struct{}, len(want))
	for _, name := range want {
		wantSet[name] = struct{}{}
		if _, ok := gotSet[name]; !ok {
			t.Errorf("%s: missing %q", label, name)
		}
	}
	for _, name := range got {
		if _, ok := wantSet[name]; !ok {
			t.Errorf("%s: unexpected %q", label, name)
		}
	}

	if t.Failed() {
		sortedGot := append([]string(nil), got...)
		sort.Strings(sortedGot)
		sortedWant := append([]string(nil), want...)
		sort.Strings(sortedWant)
		t.Logf("%s registered: %v", label, sortedGot)
		t.Logf("%s expected:   %v", label, sortedWant)
	}
}
