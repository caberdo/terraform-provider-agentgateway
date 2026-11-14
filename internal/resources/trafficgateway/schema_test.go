package trafficgateway

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// TestTrafficGatewaySchemaReflection proves the schema and the model struct
// agree, including the listeners block. The framework reads a plan into the
// model with reflection, so a mismatch only shows up at apply time otherwise.
func TestTrafficGatewaySchemaReflection(t *testing.T) {
	ctx := context.Background()
	resourceSchema := resourceSchema()

	objectType, ok := resourceSchema.Type().TerraformType(ctx).(tftypes.Object)
	if !ok {
		t.Fatalf("schema type is %T, want tftypes.Object", resourceSchema.Type().TerraformType(ctx))
	}

	values := make(map[string]tftypes.Value, len(objectType.AttributeTypes))
	for name, attributeType := range objectType.AttributeTypes {
		values[name] = tftypes.NewValue(attributeType, nil)
	}
	values["name"] = tftypes.NewValue(tftypes.String, "gw")
	values["port"] = tftypes.NewValue(tftypes.Number, 8080)

	listType, ok := objectType.AttributeTypes["listeners"].(tftypes.List)
	if !ok {
		t.Fatalf("listeners type is %T, want tftypes.List", objectType.AttributeTypes["listeners"])
	}
	listenerType, ok := listType.ElementType.(tftypes.Object)
	if !ok {
		t.Fatalf("listener type is %T, want tftypes.Object", listType.ElementType)
	}
	listenerValues := make(map[string]tftypes.Value, len(listenerType.AttributeTypes))
	for name, attributeType := range listenerType.AttributeTypes {
		listenerValues[name] = tftypes.NewValue(attributeType, nil)
	}
	listenerValues["name"] = tftypes.NewValue(tftypes.String, "public")
	values["listeners"] = tftypes.NewValue(listType, []tftypes.Value{
		tftypes.NewValue(listenerType, listenerValues),
	})

	plan := tfsdk.Plan{
		Schema: resourceSchema,
		Raw:    tftypes.NewValue(objectType, values),
	}

	m := &model{}
	if diags := plan.Get(ctx, m); diags.HasError() {
		t.Fatalf("Plan.Get() returned diagnostics: %v", diags)
	}

	if got := m.Name.ValueString(); got != "gw" {
		t.Errorf("Name = %q, want gw", got)
	}
	if got := m.Port.ValueInt64(); got != 8080 {
		t.Errorf("Port = %d, want 8080", got)
	}
	if len(m.Listeners) != 1 {
		t.Fatalf("len(Listeners) = %d, want 1", len(m.Listeners))
	}
	if got := m.Listeners[0].Name.ValueString(); got != "public" {
		t.Errorf("listener Name = %q, want public", got)
	}
}
