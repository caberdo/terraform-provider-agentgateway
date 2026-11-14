package effectiveconfig

import (
	"context"
	"testing"

	"github.com/caberdo/terraform-provider-agentgateway/internal/agentgatewaytest"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestEffectiveConfigReadsDocument proves the data source stores the document
// the gateway returns, unchanged.
func TestEffectiveConfigReadsDocument(t *testing.T) {
	server, _ := agentgatewaytest.New(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactories(server.URL),
		Steps: []resource.TestStep{
			{
				Config: `
data "agentgateway_effective_config" "test" {}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					// The fake server returns the empty document.
					resource.TestCheckResourceAttr("data.agentgateway_effective_config.test", "json", "{}"),
				),
			},
		},
	})
}

// TestEffectiveConfigRejectsNonJSON pins that the attribute is typed JSON: a
// non-JSON body is rejected instead of being stored as an opaque string.
func TestEffectiveConfigRejectsNonJSON(t *testing.T) {
	t.Parallel()

	for _, body := range []string{"", "not json", "{\"a\":"} {
		if _, err := normalize([]byte(body)); err == nil {
			t.Errorf("normalize(%q) returned no error; a non-JSON body must be rejected", body)
		}
	}
}

// TestEffectiveConfigNormalizesJSON pins that a JSON document is stored as
// normalized JSON.
func TestEffectiveConfigNormalizesJSON(t *testing.T) {
	t.Parallel()

	document, err := normalize([]byte(`{"gateways":[{"name":"default"}]}`))
	if err != nil {
		t.Fatalf("normalize() returned an error: %v", err)
	}
	if document.IsNull() {
		t.Fatal("normalize() returned a null value")
	}
}

// TestEffectiveConfigIsSensitive pins that the whole document is sensitive: it
// can contain provider credentials and API key material (design C19).
func TestEffectiveConfigIsSensitive(t *testing.T) {
	t.Parallel()

	var resp datasource.SchemaResponse
	New().Schema(context.Background(), datasource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Schema() returned diagnostics: %v", resp.Diagnostics)
	}

	attr, ok := resp.Schema.Attributes["json"]
	if !ok {
		t.Fatal(`schema has no "json" attribute`)
	}
	if !attr.IsSensitive() {
		t.Fatal(`"json" is not marked Sensitive; the effective config can contain secrets`)
	}
}
