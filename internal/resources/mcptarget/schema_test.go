package mcptarget

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
)

// TestMCPTargetSchema pins the shape of the resource schema: the transport
// group, the JSON leaves and the computed attributes. A drift here is a
// breaking change for users, so it is asserted rather than assumed.
func TestMCPTargetSchema(t *testing.T) {
	var resp resource.SchemaResponse
	New().Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Schema() diagnostics = %v", resp.Diagnostics)
	}
	attributes := resp.Schema.Attributes

	name, ok := attributes["name"].(schema.StringAttribute)
	if !ok {
		t.Fatalf("name is %T, want schema.StringAttribute", attributes["name"])
	}
	if !name.IsRequired() {
		t.Error("name is not required")
	}

	for _, transport := range []string{"mcp", "sse", "stdio", "openapi"} {
		attribute, ok := attributes[transport].(schema.SingleNestedAttribute)
		if !ok {
			t.Fatalf("%s is %T, want schema.SingleNestedAttribute", transport, attributes[transport])
		}
		if !attribute.IsOptional() {
			t.Errorf("%s is not optional", transport)
		}
		if attribute.IsComputed() {
			t.Errorf("%s is computed; a transport block must come from the configuration", transport)
		}
	}

	stdio := attributes["stdio"].(schema.SingleNestedAttribute)
	cmd, ok := stdio.Attributes["cmd"].(schema.StringAttribute)
	if !ok {
		t.Fatalf("stdio.cmd is %T, want schema.StringAttribute", stdio.Attributes["cmd"])
	}
	if !cmd.IsRequired() {
		t.Error("stdio.cmd is not required")
	}

	openapi := attributes["openapi"].(schema.SingleNestedAttribute)
	schemaJSON, ok := openapi.Attributes["schema_json"].(schema.StringAttribute)
	if !ok {
		t.Fatalf("openapi.schema_json is %T, want schema.StringAttribute", openapi.Attributes["schema_json"])
	}
	if _, ok := schemaJSON.CustomType.(jsontypes.NormalizedType); !ok {
		t.Errorf("openapi.schema_json custom type is %T, want jsontypes.NormalizedType", schemaJSON.CustomType)
	}
	if !schemaJSON.IsOptional() {
		t.Error("openapi.schema_json is not optional")
	}

	policies, ok := attributes["policies_json"].(schema.StringAttribute)
	if !ok {
		t.Fatalf("policies_json is %T, want schema.StringAttribute", attributes["policies_json"])
	}
	if _, ok := policies.CustomType.(jsontypes.NormalizedType); !ok {
		t.Errorf("policies_json custom type is %T, want jsontypes.NormalizedType", policies.CustomType)
	}
	// Transport policies carry backendAuth credentials, so the value must be
	// marked sensitive (design §12).
	if !policies.IsSensitive() {
		t.Error("policies_json is not sensitive; transport policies can carry credentials")
	}

	for _, computed := range []string{"id", "revision"} {
		attribute, ok := attributes[computed]
		if !ok {
			t.Fatalf("%s is missing", computed)
		}
		if !attribute.IsComputed() {
			t.Errorf("%s is not computed", computed)
		}
	}
}
