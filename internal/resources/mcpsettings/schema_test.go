package mcpsettings

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// TestMCPSettingsSchema pins the shape of the settings schema: the root list
// carries its element type, the attributes the gateway defaults are
// Optional+Computed so a server-returned default is stored instead of
// discarded, and the singleton's computed attributes are present.
func TestMCPSettingsSchema(t *testing.T) {
	var resp resource.SchemaResponse
	New().Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Schema() diagnostics = %v", resp.Diagnostics)
	}
	attributes := resp.Schema.Attributes

	gateways, ok := attributes["gateways"].(schema.ListAttribute)
	if !ok {
		t.Fatalf("gateways is %T, want schema.ListAttribute", attributes["gateways"])
	}
	if gateways.ElementType != types.StringType {
		t.Errorf("gateways element type is %v, want types.StringType", gateways.ElementType)
	}
	if !gateways.IsOptional() {
		t.Error("gateways is not optional")
	}
	// gateways is deliberately not computed: an empty server list means the
	// same as unset upstream, and Computed would make the attribute sticky so
	// removing it from the configuration could never detach the routes.
	if gateways.IsComputed() {
		t.Error("gateways is computed; removing it from the configuration must detach the routes")
	}

	statefulMode, ok := attributes["stateful_mode"].(schema.StringAttribute)
	if !ok {
		t.Fatalf("stateful_mode is %T, want schema.StringAttribute", attributes["stateful_mode"])
	}
	// `mcp.statefulMode` defaults to stateless upstream.
	if !statefulMode.IsComputed() {
		t.Error("stateful_mode is not computed")
	}
	if len(statefulMode.PlanModifiers) == 0 {
		t.Error("stateful_mode has no plan modifiers")
	}

	dnsRebinding, ok := attributes["dns_rebinding_protection"].(schema.BoolAttribute)
	if !ok {
		t.Fatalf("dns_rebinding_protection is %T, want schema.BoolAttribute", attributes["dns_rebinding_protection"])
	}
	// `mcp.dnsRebindingProtection` is a plain bool upstream and defaults to
	// false.
	if !dnsRebinding.IsComputed() {
		t.Error("dns_rebinding_protection is not computed")
	}
	if len(dnsRebinding.PlanModifiers) == 0 {
		t.Error("dns_rebinding_protection has no plan modifiers")
	}

	port, ok := attributes["port"].(schema.Int64Attribute)
	if !ok {
		t.Fatalf("port is %T, want schema.Int64Attribute", attributes["port"])
	}
	if port.IsComputed() {
		t.Error("port is computed; it has no server-side default")
	}

	// prefix_mode, failure_mode and sse_keep_alive are plain optional values.
	for _, name := range []string{"prefix_mode", "failure_mode", "sse_keep_alive"} {
		attribute, ok := attributes[name].(schema.StringAttribute)
		if !ok {
			t.Fatalf("%s is %T, want schema.StringAttribute", name, attributes[name])
		}
		if !attribute.IsOptional() {
			t.Errorf("%s is not optional", name)
		}
	}

	server, ok := attributes["server"].(schema.SingleNestedAttribute)
	if !ok {
		t.Fatalf("server is %T, want schema.SingleNestedAttribute", attributes["server"])
	}
	if !server.IsOptional() {
		t.Error("server is not optional")
	}
	for _, name := range []string{"name", "version", "title", "instructions"} {
		if _, ok := server.Attributes[name].(schema.StringAttribute); !ok {
			t.Errorf("server.%s is %T, want schema.StringAttribute", name, server.Attributes[name])
		}
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
