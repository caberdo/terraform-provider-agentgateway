package provider_test

import (
	"context"
	"testing"

	"github.com/caberdo/terraform-provider-agentgateway/internal/agentgatewaytest"
	"github.com/caberdo/terraform-provider-agentgateway/internal/provider"
	"github.com/caberdo/terraform-provider-agentgateway/internal/providerdata"
	fwprovider "github.com/hashicorp/terraform-plugin-framework/provider"
	providerschema "github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func TestConfigureAllowsFileMode(t *testing.T) {
	srv, _ := agentgatewaytest.New(t)
	srv.SetStoreMode("file")

	p := provider.New("test")()
	req := newConfigureRequest(t, srv.URL, true, "")
	var resp fwprovider.ConfigureResponse

	p.Configure(context.Background(), req, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error diagnostics: %v", resp.Diagnostics)
	}
	data, ok := resp.ResourceData.(*providerdata.Data)
	if !ok {
		t.Fatalf("expected *providerdata.Data, got %T", resp.ResourceData)
	}
	if data.StoreMode != "file" {
		t.Errorf("expected store mode file, got %q", data.StoreMode)
	}
}

func TestConfigureErrorsOnReadOnlyByDefault(t *testing.T) {
	srv, _ := agentgatewaytest.New(t)
	srv.SetStoreMode("readOnly")

	p := provider.New("test")()
	req := newConfigureRequest(t, srv.URL, true, "")
	var resp fwprovider.ConfigureResponse

	p.Configure(context.Background(), req, &resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error diagnostic for readOnly storage mode")
	}
	if resp.ResourceData != nil {
		t.Fatal("expected no resource data on error")
	}
}

func TestConfigureSkipsModeCheckWhenVerifyDisabled(t *testing.T) {
	srv, _ := agentgatewaytest.New(t)
	srv.SetStoreMode("readOnly")

	p := provider.New("test")()
	req := newConfigureRequest(t, srv.URL, false, "")
	var resp fwprovider.ConfigureResponse

	p.Configure(context.Background(), req, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error diagnostics: %v", resp.Diagnostics)
	}
	data, ok := resp.ResourceData.(*providerdata.Data)
	if !ok {
		t.Fatalf("expected *providerdata.Data, got %T", resp.ResourceData)
	}
	if data.StoreMode != "" {
		t.Errorf("expected empty store mode when verify is disabled, got %q", data.StoreMode)
	}
}

func TestConfigureWarnOnReadOnly(t *testing.T) {
	srv, _ := agentgatewaytest.New(t)
	srv.SetStoreMode("readOnly")

	p := provider.New("test")()
	req := newConfigureRequest(t, srv.URL, true, "warn")
	var resp fwprovider.ConfigureResponse

	p.Configure(context.Background(), req, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error diagnostics: %v", resp.Diagnostics)
	}
	data, ok := resp.ResourceData.(*providerdata.Data)
	if !ok {
		t.Fatalf("expected *providerdata.Data, got %T", resp.ResourceData)
	}
	if data.StoreMode != "readOnly" {
		t.Errorf("expected store mode readOnly, got %q", data.StoreMode)
	}
}

func TestConfigureIgnoresModeCheck(t *testing.T) {
	srv, _ := agentgatewaytest.New(t)
	srv.SetStoreMode("readOnly")

	p := provider.New("test")()
	req := newConfigureRequest(t, srv.URL, true, "ignore")
	var resp fwprovider.ConfigureResponse

	p.Configure(context.Background(), req, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error diagnostics: %v", resp.Diagnostics)
	}
	if resp.Diagnostics.WarningsCount() != 0 {
		t.Fatalf("expected no diagnostics, got %v", resp.Diagnostics)
	}
	data, ok := resp.ResourceData.(*providerdata.Data)
	if !ok {
		t.Fatalf("expected *providerdata.Data, got %T", resp.ResourceData)
	}
	if data.StoreMode != "readOnly" {
		t.Errorf("expected store mode readOnly, got %q", data.StoreMode)
	}
}

func newConfigureRequest(t *testing.T, endpoint string, verify bool, storageModeCheck string) fwprovider.ConfigureRequest {
	t.Helper()

	schema := providerschema.Schema{
		Attributes: map[string]providerschema.Attribute{
			"endpoint": providerschema.StringAttribute{Optional: true},
			"token":    providerschema.StringAttribute{Optional: true, Sensitive: true},
			"username": providerschema.StringAttribute{Optional: true},
			"password": providerschema.StringAttribute{Optional: true, Sensitive: true},
			"http_headers": providerschema.MapAttribute{
				Optional:    true,
				Sensitive:   true,
				ElementType: types.StringType,
			},
			"timeout":              providerschema.StringAttribute{Optional: true},
			"ca_cert":              providerschema.StringAttribute{Optional: true},
			"ca_cert_file":         providerschema.StringAttribute{Optional: true},
			"client_cert":          providerschema.StringAttribute{Optional: true},
			"client_cert_file":     providerschema.StringAttribute{Optional: true},
			"client_key":           providerschema.StringAttribute{Optional: true, Sensitive: true},
			"client_key_file":      providerschema.StringAttribute{Optional: true},
			"insecure_skip_verify": providerschema.BoolAttribute{Optional: true},
			"verify":               providerschema.BoolAttribute{Optional: true},
			"storage_mode_check":   providerschema.StringAttribute{Optional: true},
		},
	}

	attrTypes := map[string]tftypes.Type{
		"endpoint":             tftypes.String,
		"token":                tftypes.String,
		"username":             tftypes.String,
		"password":             tftypes.String,
		"http_headers":         tftypes.Map{ElementType: tftypes.String},
		"timeout":              tftypes.String,
		"ca_cert":              tftypes.String,
		"ca_cert_file":         tftypes.String,
		"client_cert":          tftypes.String,
		"client_cert_file":     tftypes.String,
		"client_key":           tftypes.String,
		"client_key_file":      tftypes.String,
		"insecure_skip_verify": tftypes.Bool,
		"verify":               tftypes.Bool,
		"storage_mode_check":   tftypes.String,
	}

	storageModeCheckValue := tftypes.NewValue(tftypes.String, nil)
	if storageModeCheck != "" {
		storageModeCheckValue = tftypes.NewValue(tftypes.String, storageModeCheck)
	}

	values := map[string]tftypes.Value{
		"endpoint":             tftypes.NewValue(tftypes.String, endpoint),
		"token":                tftypes.NewValue(tftypes.String, nil),
		"username":             tftypes.NewValue(tftypes.String, nil),
		"password":             tftypes.NewValue(tftypes.String, nil),
		"http_headers":         tftypes.NewValue(tftypes.Map{ElementType: tftypes.String}, nil),
		"timeout":              tftypes.NewValue(tftypes.String, nil),
		"ca_cert":              tftypes.NewValue(tftypes.String, nil),
		"ca_cert_file":         tftypes.NewValue(tftypes.String, nil),
		"client_cert":          tftypes.NewValue(tftypes.String, nil),
		"client_cert_file":     tftypes.NewValue(tftypes.String, nil),
		"client_key":           tftypes.NewValue(tftypes.String, nil),
		"client_key_file":      tftypes.NewValue(tftypes.String, nil),
		"insecure_skip_verify": tftypes.NewValue(tftypes.Bool, nil),
		"verify":               tftypes.NewValue(tftypes.Bool, verify),
		"storage_mode_check":   storageModeCheckValue,
	}

	return fwprovider.ConfigureRequest{
		Config: tfsdk.Config{
			Raw:    tftypes.NewValue(tftypes.Object{AttributeTypes: attrTypes}, values),
			Schema: schema,
		},
	}
}
