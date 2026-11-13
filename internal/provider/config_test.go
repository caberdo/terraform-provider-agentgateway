package provider_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/caberdo/terraform-provider-agentgateway/internal/provider"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestResolveDefaults(t *testing.T) {
	cfg := provider.Config{}
	resolved := cfg.Resolve()

	if resolved.Endpoint != "http://127.0.0.1:15000" {
		t.Errorf("endpoint default: expected http://127.0.0.1:15000, got %q", resolved.Endpoint)
	}
	if resolved.Timeout != 30*time.Second {
		t.Errorf("timeout default: expected 30s, got %v", resolved.Timeout)
	}
	if resolved.Verify != true {
		t.Error("verify default: expected true")
	}
	if resolved.StorageModeCheck != "error" {
		t.Errorf("storage_mode_check default: expected error, got %q", resolved.StorageModeCheck)
	}
}

func TestResolveEnvFallback(t *testing.T) {
	t.Setenv("AGENTGATEWAY_ENDPOINT", "http://env.example.com:15000")
	t.Setenv("AGENTGATEWAY_TOKEN", "env-token")
	t.Setenv("AGENTGATEWAY_TIMEOUT", "5s")
	t.Setenv("AGENTGATEWAY_INSECURE_SKIP_VERIFY", "true")
	t.Setenv("AGENTGATEWAY_VERIFY", "false")

	cfg := provider.Config{
		Endpoint:           types.StringNull(),
		Token:              types.StringNull(),
		Timeout:            types.StringNull(),
		InsecureSkipVerify: types.BoolNull(),
		Verify:             types.BoolNull(),
	}

	resolved := cfg.Resolve()

	if resolved.Endpoint != "http://env.example.com:15000" {
		t.Errorf("endpoint: expected env value, got %q", resolved.Endpoint)
	}
	if resolved.Token != "env-token" {
		t.Errorf("token: expected env value, got %q", resolved.Token)
	}
	if resolved.Timeout != 5*time.Second {
		t.Errorf("timeout: expected 5s, got %v", resolved.Timeout)
	}
	if !resolved.InsecureSkipVerify {
		t.Error("insecure_skip_verify: expected true")
	}
	if resolved.Verify {
		t.Error("verify: expected false")
	}
}

func TestResolveExplicitValueOverridesEnv(t *testing.T) {
	t.Setenv("AGENTGATEWAY_ENDPOINT", "http://env.example.com:15000")

	cfg := provider.Config{Endpoint: types.StringValue("http://explicit.example.com:15000")}
	resolved := cfg.Resolve()

	if resolved.Endpoint != "http://explicit.example.com:15000" {
		t.Errorf("endpoint: expected explicit value, got %q", resolved.Endpoint)
	}
}

func TestValidateRejectsInvalidEndpoint(t *testing.T) {
	cases := []struct {
		name     string
		endpoint string
	}{
		{"with path", "http://example.com:15000/api"},
		{"with query", "http://example.com:15000?foo=bar"},
		{"missing host", "http://"},
		{"invalid scheme", "ftp://example.com:15000"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := provider.Config{Endpoint: types.StringValue(tc.endpoint)}
			if !cfg.Validate().HasError() {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestValidateRejectsBasicAuthOverHTTP(t *testing.T) {
	cfg := provider.Config{
		Endpoint: types.StringValue("http://example.com:15000"),
		Username: types.StringValue("admin"),
		Password: types.StringValue("secret"),
	}

	diags := cfg.Validate()
	if !diags.HasError() {
		t.Fatal("expected validation error for basic auth over HTTP")
	}
}

func TestValidateRejectsTokenWithBasicAuth(t *testing.T) {
	cfg := provider.Config{
		Endpoint: types.StringValue("https://example.com:15000"),
		Token:    types.StringValue("bearer-token"),
		Username: types.StringValue("admin"),
		Password: types.StringValue("secret"),
	}

	diags := cfg.Validate()
	if !diags.HasError() {
		t.Fatal("expected validation error when token is used with basic auth")
	}
}

func TestValidateRejectsPasswordWithoutUsername(t *testing.T) {
	cfg := provider.Config{
		Endpoint: types.StringValue("https://example.com:15000"),
		Password: types.StringValue("secret"),
	}

	diags := cfg.Validate()
	if !diags.HasError() {
		t.Fatal("expected validation error when password is set without username")
	}
}

func TestValidateRejectsCertWithoutKey(t *testing.T) {
	cfg := provider.Config{
		Endpoint:   types.StringValue("https://example.com:15000"),
		ClientCert: types.StringValue("-----BEGIN CERTIFICATE-----\nMIIBkTCB+wIJAJHGTVDEssSSMA0GCSqGSIb3DQEBCwUAMBExDzANBgNVBAMMBnRlc3RjYTAeFw0yNDAxMDEwMDAwMDBaFw0yNTAxMDEwMDAwMDBaMBExDzANBgNVBAMMBnRlc3RjYTBcMA0GCSqGSIb3DQEBAQUAA0sAMEgCQQC5kzmkCRnEGOrWNwux5MYi/htz7j8LFpXoLGP8xWr5V6iHpFqokYAqV15K4/7mOFsoFZiHgZXZSlvH0xKiWokvAgMBAAGjUDBOMB0GA1UdDgQWBBQI0fKlBJhcG7lW9A0L7VH6GhLmaTAfBgNVHSMEGDAWgBQI0fKlBJhcG7lW9A0L7VH6GhLmaTAMBgNVHRMEBTADAQH/MA0GCSqGSIb3DQEBCwUAA0EAq1rryuD7T7zmjC7OXq4LqTtw/xJ2fPhJpXzsfkqxM9+kHXN2xB8vALMyqeT+G5ZWLHBAjOKoH0U4zEODl9vQhg==\n-----END CERTIFICATE-----"),
	}

	diags := cfg.Validate()
	if !diags.HasError() {
		t.Fatal("expected validation error when client certificate is provided without key")
	}
}

func TestValidateRejectsBothCAOptions(t *testing.T) {
	cfg := provider.Config{
		Endpoint:   types.StringValue("https://example.com:15000"),
		CACert:     types.StringValue("cert"),
		CACertFile: types.StringValue("/path/to/ca.crt"),
	}

	diags := cfg.Validate()
	if !diags.HasError() {
		t.Fatal("expected validation error when both ca_cert and ca_cert_file are set")
	}
}

func TestValidateAcceptsTokenOverHTTPS(t *testing.T) {
	cfg := provider.Config{
		Endpoint: types.StringValue("https://example.com:15000"),
		Token:    types.StringValue("bearer-token"),
	}

	diags := cfg.Validate()
	if diags.HasError() {
		t.Fatalf("unexpected validation error: %v", diags)
	}
}

func TestBuildHTTPClientWithInsecureSkipVerify(t *testing.T) {
	cfg := provider.Config{
		Endpoint:           types.StringValue("https://example.com:15000"),
		InsecureSkipVerify: types.BoolValue(true),
	}

	httpClient, err := cfg.BuildHTTPClient()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	transport, ok := httpClient.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport, got %T", httpClient.Transport)
	}
	if !transport.TLSClientConfig.InsecureSkipVerify {
		t.Error("expected InsecureSkipVerify to be true")
	}
}

func TestAuthHeadersBearer(t *testing.T) {
	cfg := provider.Config{Token: types.StringValue("my-token")}
	headers, err := cfg.AuthHeaders()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if headers["Authorization"] != "Bearer my-token" {
		t.Errorf("expected Bearer token, got %q", headers["Authorization"])
	}
}

func TestAuthHeadersBasic(t *testing.T) {
	cfg := provider.Config{
		Username: types.StringValue("admin"),
		Password: types.StringValue("secret"),
	}
	headers, err := cfg.AuthHeaders()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if headers["Authorization"] != "Basic YWRtaW46c2VjcmV0" {
		t.Errorf("expected Basic header, got %q", headers["Authorization"])
	}
}
