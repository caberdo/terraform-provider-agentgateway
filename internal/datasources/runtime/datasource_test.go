package runtime

import (
	"context"
	"testing"

	"github.com/caberdo/terraform-provider-agentgateway/internal/agentgatewaytest"
	"github.com/caberdo/terraform-provider-agentgateway/internal/client"
	"github.com/caberdo/terraform-provider-agentgateway/internal/providerdata"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func TestRuntimeRead(t *testing.T) {
	t.Parallel()

	srv, c := agentgatewaytest.New(t)
	srv.SetStoreMode("hybrid")

	state := readRuntime(t, c)

	if state.ID.ValueString() != "runtime" {
		t.Errorf("expected id runtime, got %q", state.ID.ValueString())
	}
	if state.BuildVersion.ValueString() != "v1.7.0" {
		t.Errorf("expected build_version v1.7.0, got %q", state.BuildVersion.ValueString())
	}
	if state.GitRevision.ValueString() != "fake" {
		t.Errorf("expected git_revision fake, got %q", state.GitRevision.ValueString())
	}
	if state.GatewayMode.ValueString() != "standalone" {
		t.Errorf("expected gateway_mode standalone, got %q", state.GatewayMode.ValueString())
	}
	if state.ConfigStoreMode.ValueString() != "hybrid" {
		t.Errorf("expected config_store_mode hybrid, got %q", state.ConfigStoreMode.ValueString())
	}
	if !state.ConfigReloadSynchronized.ValueBool() {
		t.Error("expected config_reload_synchronized true")
	}
	if state.ConfigReloadLastError.ValueString() != "" {
		t.Errorf("expected an empty config_reload_last_error, got %q", state.ConfigReloadLastError.ValueString())
	}
}

func readRuntime(t *testing.T, c client.Client) readModel {
	t.Helper()

	ds := New()
	cw, ok := ds.(datasource.DataSourceWithConfigure)
	if !ok {
		t.Fatal("data source does not implement DataSourceWithConfigure")
	}
	var configureResp datasource.ConfigureResponse
	cw.Configure(context.Background(), datasource.ConfigureRequest{
		ProviderData: &providerdata.Data{Client: c},
	}, &configureResp)
	if configureResp.Diagnostics.HasError() {
		t.Fatalf("configure failed: %v", configureResp.Diagnostics)
	}

	s := schemaOf(t, ds)
	var resp datasource.ReadResponse
	resp.State = emptyState(t, s)
	ds.Read(context.Background(), datasource.ReadRequest{Config: tfsdk.Config{
		Raw:    tftypes.NewValue(s.Type().TerraformType(context.Background()), nil),
		Schema: s,
	}}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("read failed: %v", resp.Diagnostics)
	}

	var state readModel
	if err := resp.State.Get(context.Background(), &state); err != nil {
		t.Fatalf("reading state: %v", err)
	}
	return state
}

func schemaOf(t *testing.T, ds datasource.DataSource) datasourceschema.Schema {
	t.Helper()
	var resp datasource.SchemaResponse
	ds.Schema(context.Background(), datasource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema failed: %v", resp.Diagnostics)
	}
	return resp.Schema
}

func emptyState(t *testing.T, s datasourceschema.Schema) tfsdk.State {
	t.Helper()
	return tfsdk.State{
		Raw:    tftypes.NewValue(s.Type().TerraformType(context.Background()), nil),
		Schema: s,
	}
}
