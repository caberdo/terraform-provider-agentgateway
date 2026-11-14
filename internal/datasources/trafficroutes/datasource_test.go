package trafficroutes

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/caberdo/terraform-provider-agentgateway/internal/agentgatewaytest"
	"github.com/caberdo/terraform-provider-agentgateway/internal/configresource"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestTrafficRoutesBackendsAreSensitive pins that the backends listing is
// sensitive: each backend is raw JSON and exposes its `policies`, which carry
// TLS material and authorization rules.
func TestTrafficRoutesBackendsAreSensitive(t *testing.T) {
	t.Parallel()

	var resp datasource.SchemaResponse
	New().Schema(context.Background(), datasource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Schema() returned diagnostics: %v", resp.Diagnostics)
	}

	routes, ok := resp.Schema.Attributes["routes"].(schema.ListNestedAttribute)
	if !ok {
		t.Fatal(`"routes" is not a list nested attribute`)
	}
	attribute, ok := routes.NestedObject.Attributes["backends"]
	if !ok {
		t.Fatal(`"routes" has no "backends" attribute`)
	}
	if !attribute.IsSensitive() {
		t.Fatal(`"backends" is not marked Sensitive; backend policies carry credentials`)
	}
}

// TestFlattenRouteOmittedCollectionsAreEmpty pins that omitted arrays become
// empty collections, not null: a null list cannot be iterated in Terraform,
// and examples iterate route.backends.
func TestFlattenRouteOmittedCollectionsAreEmpty(t *testing.T) {
	t.Parallel()

	routes, diags := flatten([]configresource.Resource{
		{ID: "bare", Value: json.RawMessage(`{"name":"bare"}`)},
	})
	if diags.HasError() {
		t.Fatalf("flatten() returned diagnostics: %v", diags)
	}
	if len(routes) != 1 {
		t.Fatalf("flatten() returned %d routes, want 1", len(routes))
	}
	if routes[0].Gateways == nil {
		t.Fatal("gateways is null; an omitted array must be an empty collection")
	}
	if routes[0].Hostnames == nil {
		t.Fatal("hostnames is null; an omitted array must be an empty collection")
	}
	if routes[0].Backends == nil {
		t.Fatal("backends is null; an omitted array must be an empty collection")
	}
	if len(routes[0].Gateways) != 0 || len(routes[0].Hostnames) != 0 || len(routes[0].Backends) != 0 {
		t.Fatalf("expected empty collections, got gateways=%d hostnames=%d backends=%d",
			len(routes[0].Gateways), len(routes[0].Hostnames), len(routes[0].Backends))
	}
}

// TestTrafficRoutesDataSource pins the listing contract: deterministic order
// by id, the name_prefix filter and the computed "<kind>:<prefix>" id.
func TestTrafficRoutesDataSource(t *testing.T) {
	server, _ := agentgatewaytest.New(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactories(server.URL),
		Steps: []resource.TestStep{
			{
				Config: `
resource "agentgateway_test_resource" "a" {
  name = "route-a"

  value_json = jsonencode({
    name      = "route-a"
    gateways  = ["default"]
    hostnames = ["a.example"]
    backends = [
      { host = { host = "127.0.0.1", port = 8080 } },
    ]
  })
}

resource "agentgateway_test_resource" "b" {
  name = "route-b"

  value_json = jsonencode({
    name      = "route-b"
    gateways  = ["default", "other"]
    hostnames = ["b.example"]
    backends = [
      { weight = 2, service = { name = "svc", namespace = "default" } },
      { host = { host = "10.0.0.1", port = 9090 } },
    ]
  })
}

resource "agentgateway_test_resource" "other" {
  name       = "other-route"
  value_json = jsonencode({ name = "other-route" })
}

data "agentgateway_traffic_routes" "test" {
  name_prefix = "route-"

  depends_on = [
    agentgateway_test_resource.a,
    agentgateway_test_resource.b,
    agentgateway_test_resource.other,
  ]
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.agentgateway_traffic_routes.test", "id", "traffic.route:route-"),
					resource.TestCheckResourceAttr("data.agentgateway_traffic_routes.test", "routes.#", "2"),
					// Sorted by id: route-a before route-b.
					resource.TestCheckResourceAttr("data.agentgateway_traffic_routes.test", "routes.0.name", "route-a"),
					resource.TestCheckResourceAttr("data.agentgateway_traffic_routes.test", "routes.0.gateways.#", "1"),
					resource.TestCheckResourceAttr("data.agentgateway_traffic_routes.test", "routes.0.gateways.0", "default"),
					resource.TestCheckResourceAttr("data.agentgateway_traffic_routes.test", "routes.0.hostnames.#", "1"),
					resource.TestCheckResourceAttr("data.agentgateway_traffic_routes.test", "routes.0.hostnames.0", "a.example"),
					resource.TestCheckResourceAttr("data.agentgateway_traffic_routes.test", "routes.0.backends.#", "1"),
					resource.TestCheckResourceAttrWith("data.agentgateway_traffic_routes.test", "routes.0.backends.0", func(value string) error {
						var backend map[string]any
						return json.Unmarshal([]byte(value), &backend)
					}),
					resource.TestCheckResourceAttr("data.agentgateway_traffic_routes.test", "routes.1.name", "route-b"),
					resource.TestCheckResourceAttr("data.agentgateway_traffic_routes.test", "routes.1.gateways.#", "2"),
					resource.TestCheckResourceAttr("data.agentgateway_traffic_routes.test", "routes.1.gateways.1", "other"),
					resource.TestCheckResourceAttr("data.agentgateway_traffic_routes.test", "routes.1.hostnames.0", "b.example"),
					resource.TestCheckResourceAttr("data.agentgateway_traffic_routes.test", "routes.1.backends.#", "2"),
				),
			},
		},
	})
}
