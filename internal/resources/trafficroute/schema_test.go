package trafficroute

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// nullAttributes seeds every attribute of an object type with null.
func nullAttributes(object tftypes.Object) map[string]tftypes.Value {
	values := make(map[string]tftypes.Value, len(object.AttributeTypes))
	for name, attributeType := range object.AttributeTypes {
		values[name] = tftypes.NewValue(attributeType, nil)
	}
	return values
}

// TestTrafficRouteSchemaReflection proves the schema and the model struct
// agree, including the matches and backends blocks. The framework reads a plan
// into the model with reflection, so a mismatch only shows up at apply time
// otherwise.
func TestTrafficRouteSchemaReflection(t *testing.T) {
	ctx := context.Background()
	resourceSchema := resourceSchema()

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

	matchesType, ok := objectType.AttributeTypes["matches"].(tftypes.List)
	if !ok {
		t.Fatalf("matches type is %T, want tftypes.List", objectType.AttributeTypes["matches"])
	}
	matchType, ok := matchesType.ElementType.(tftypes.Object)
	if !ok {
		t.Fatalf("match type is %T, want tftypes.Object", matchesType.ElementType)
	}
	matchValues := nullAttributes(matchType)
	matchValues["method"] = tftypes.NewValue(tftypes.String, "GET")

	pathListType, ok := matchType.AttributeTypes["path"].(tftypes.List)
	if !ok {
		t.Fatalf("path type is %T, want tftypes.List", matchType.AttributeTypes["path"])
	}
	pathType, ok := pathListType.ElementType.(tftypes.Object)
	if !ok {
		t.Fatalf("path element type is %T, want tftypes.Object", pathListType.ElementType)
	}
	pathValues := nullAttributes(pathType)
	pathValues["prefix"] = tftypes.NewValue(tftypes.String, "/api")
	matchValues["path"] = tftypes.NewValue(pathListType, []tftypes.Value{
		tftypes.NewValue(pathType, pathValues),
	})

	headersType, ok := matchType.AttributeTypes["headers"].(tftypes.List)
	if !ok {
		t.Fatalf("headers type is %T, want tftypes.List", matchType.AttributeTypes["headers"])
	}
	headerType, ok := headersType.ElementType.(tftypes.Object)
	if !ok {
		t.Fatalf("header element type is %T, want tftypes.Object", headersType.ElementType)
	}
	headerValues := nullAttributes(headerType)
	headerValues["name"] = tftypes.NewValue(tftypes.String, "x-tenant")
	headerValues["exact"] = tftypes.NewValue(tftypes.String, "acme")
	matchValues["headers"] = tftypes.NewValue(headersType, []tftypes.Value{
		tftypes.NewValue(headerType, headerValues),
	})

	values["matches"] = tftypes.NewValue(matchesType, []tftypes.Value{
		tftypes.NewValue(matchType, matchValues),
	})

	backendsType, ok := objectType.AttributeTypes["backends"].(tftypes.List)
	if !ok {
		t.Fatalf("backends type is %T, want tftypes.List", objectType.AttributeTypes["backends"])
	}
	backendType, ok := backendsType.ElementType.(tftypes.Object)
	if !ok {
		t.Fatalf("backend element type is %T, want tftypes.Object", backendsType.ElementType)
	}
	backendValues := nullAttributes(backendType)
	backendValues["weight"] = tftypes.NewValue(tftypes.Number, 2)

	hostListType, ok := backendType.AttributeTypes["host"].(tftypes.List)
	if !ok {
		t.Fatalf("host type is %T, want tftypes.List", backendType.AttributeTypes["host"])
	}
	hostType, ok := hostListType.ElementType.(tftypes.Object)
	if !ok {
		t.Fatalf("host element type is %T, want tftypes.Object", hostListType.ElementType)
	}
	hostValues := nullAttributes(hostType)
	hostValues["host"] = tftypes.NewValue(tftypes.String, "127.0.0.1")
	hostValues["port"] = tftypes.NewValue(tftypes.Number, 8080)
	backendValues["host"] = tftypes.NewValue(hostListType, []tftypes.Value{
		tftypes.NewValue(hostType, hostValues),
	})

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

	if len(m.Matches) != 1 {
		t.Fatalf("len(Matches) = %d, want 1", len(m.Matches))
	}
	if got := m.Matches[0].Method.ValueString(); got != "GET" {
		t.Errorf("match Method = %q, want GET", got)
	}
	if len(m.Matches[0].Path) != 1 || m.Matches[0].Path[0].Prefix.ValueString() != "/api" {
		t.Errorf("match Path = %+v, want prefix /api", m.Matches[0].Path)
	}
	if len(m.Matches[0].Headers) != 1 {
		t.Fatalf("len(match.Headers) = %d, want 1", len(m.Matches[0].Headers))
	}
	if got := m.Matches[0].Headers[0].Name.ValueString(); got != "x-tenant" {
		t.Errorf("match header Name = %q, want x-tenant", got)
	}

	if len(m.Backends) != 1 {
		t.Fatalf("len(Backends) = %d, want 1", len(m.Backends))
	}
	if got := m.Backends[0].Weight.ValueInt64(); got != 2 {
		t.Errorf("backend Weight = %d, want 2", got)
	}
	if len(m.Backends[0].Host) != 1 || m.Backends[0].Host[0].Port.ValueInt64() != 8080 {
		t.Errorf("backend Host = %+v, want 127.0.0.1:8080", m.Backends[0].Host)
	}
}
