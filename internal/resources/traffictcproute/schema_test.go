package traffictcproute

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// TestTrafficTCPRoutePoliciesAreSensitive pins that the route-level
// policies_json is sensitive: it carries the same credential families as the
// gateway-level policies (basicAuth, apiKey, extAuthz, JWT/OIDC).
func TestTrafficTCPRoutePoliciesAreSensitive(t *testing.T) {
	t.Parallel()

	attribute, ok := resourceSchema().Attributes["policies_json"].(schema.StringAttribute)
	if !ok {
		t.Fatal(`"policies_json" is not a string attribute`)
	}
	if !attribute.Sensitive {
		t.Fatal(`"policies_json" is not marked Sensitive; route policies carry credentials`)
	}
}

// nullAttributes seeds every attribute of an object type with null.
func nullAttributes(object tftypes.Object) map[string]tftypes.Value {
	values := make(map[string]tftypes.Value, len(object.AttributeTypes))
	for name, attributeType := range object.AttributeTypes {
		values[name] = tftypes.NewValue(attributeType, nil)
	}
	return values
}

// TestTrafficTCPRouteSchemaReflection proves the schema and the model struct
// agree, including the backends block. The framework reads a plan into the
// model with reflection, so a mismatch only shows up at apply time otherwise.
func TestTrafficTCPRouteSchemaReflection(t *testing.T) {
	ctx := context.Background()
	resourceSchema := resourceSchema()

	if _, ok := resourceSchema.Attributes["matches"]; ok {
		t.Error("schema declares matches, but a TCP route has no HTTP matches")
	}

	objectType, ok := resourceSchema.Type().TerraformType(ctx).(tftypes.Object)
	if !ok {
		t.Fatalf("schema type is %T, want tftypes.Object", resourceSchema.Type().TerraformType(ctx))
	}

	values := nullAttributes(objectType)
	values["name"] = tftypes.NewValue(tftypes.String, "r")
	values["gateways"] = tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, []tftypes.Value{
		tftypes.NewValue(tftypes.String, "default"),
	})
	values["hostnames"] = tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, []tftypes.Value{
		tftypes.NewValue(tftypes.String, "example.com"),
	})

	backendsType, ok := objectType.AttributeTypes["backends"].(tftypes.List)
	if !ok {
		t.Fatalf("backends type is %T, want tftypes.List", objectType.AttributeTypes["backends"])
	}
	backendType, ok := backendsType.ElementType.(tftypes.Object)
	if !ok {
		t.Fatalf("backend type is %T, want tftypes.Object", backendsType.ElementType)
	}
	backendValues := nullAttributes(backendType)
	backendValues["weight"] = tftypes.NewValue(tftypes.Number, 2)
	backendValues["host"] = tftypes.NewValue(tftypes.String, "127.0.0.1:8080")
	backendValues["backend"] = tftypes.NewValue(tftypes.String, "postgres")

	values["backends"] = tftypes.NewValue(backendsType, []tftypes.Value{
		tftypes.NewValue(backendType, backendValues),
	})

	plan := tfsdk.Plan{
		Schema: resourceSchema,
		Raw:    tftypes.NewValue(objectType, values),
	}

	m := newModel()
	if diags := plan.Get(ctx, m); diags.HasError() {
		t.Fatalf("Plan.Get() returned diagnostics: %v", diags)
	}

	if got := m.Name.ValueString(); got != "r" {
		t.Errorf("Name = %q, want r", got)
	}
	var gateways []string
	if d := m.Gateways.ElementsAs(ctx, &gateways, false); d.HasError() {
		t.Fatalf("decoding gateways: %v", d)
	}
	if len(gateways) != 1 || gateways[0] != "default" {
		t.Errorf("Gateways = %v, want [default]", gateways)
	}

	if len(m.Backends) != 1 {
		t.Fatalf("len(Backends) = %d, want 1", len(m.Backends))
	}
	if got := m.Backends[0].Weight.ValueInt64(); got != 2 {
		t.Errorf("backend Weight = %d, want 2", got)
	}
	if got := m.Backends[0].Host.ValueString(); got != "127.0.0.1:8080" {
		t.Errorf("backend Host = %q, want 127.0.0.1:8080", got)
	}
	if got := m.Backends[0].Backend.ValueString(); got != "postgres" {
		t.Errorf("backend Backend = %q, want postgres", got)
	}
}

// TestNewModelSeedsTypedNullLists proves a fresh model can be written straight
// into state: ImportState does exactly that, and a zero-value list has no
// element type.
func TestNewModelSeedsTypedNullLists(t *testing.T) {
	m := newModel()
	ctx := context.Background()

	for name, list := range map[string]types.List{
		"gateways":  m.Gateways,
		"hostnames": m.Hostnames,
	} {
		if !list.IsNull() {
			t.Errorf("%s is not null, want a null list", name)
		}
		if got := list.ElementType(ctx); got == nil {
			t.Errorf("%s has no element type, want types.StringType", name)
		}
	}
}
