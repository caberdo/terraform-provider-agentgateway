package provider

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Config is the raw provider configuration as read from Terraform.
type Config struct {
	Endpoint           types.String `tfsdk:"endpoint"`
	Token              types.String `tfsdk:"token"`
	Username           types.String `tfsdk:"username"`
	Password           types.String `tfsdk:"password"`
	HTTPHeaders        types.Map    `tfsdk:"http_headers"`
	Timeout            types.String `tfsdk:"timeout"`
	CACert             types.String `tfsdk:"ca_cert"`
	CACertFile         types.String `tfsdk:"ca_cert_file"`
	ClientCert         types.String `tfsdk:"client_cert"`
	ClientCertFile     types.String `tfsdk:"client_cert_file"`
	ClientKey          types.String `tfsdk:"client_key"`
	ClientKeyFile      types.String `tfsdk:"client_key_file"`
	InsecureSkipVerify types.Bool   `tfsdk:"insecure_skip_verify"`
	Verify             types.Bool   `tfsdk:"verify"`
	StorageModeCheck   types.String `tfsdk:"storage_mode_check"`
}

// ResolvedConfig holds provider configuration after env fallbacks and defaults.
type ResolvedConfig struct {
	Endpoint           string
	Token              string
	Username           string
	Password           string
	HTTPHeaders        map[string]string
	Timeout            time.Duration
	CACert             string
	CACertFile         string
	ClientCert         string
	ClientCertFile     string
	ClientKey          string
	ClientKeyFile      string
	InsecureSkipVerify bool
	Verify             bool
	StorageModeCheck   string
}

// Resolve applies environment-variable fallbacks and defaults.
func (c Config) Resolve() ResolvedConfig {
	return ResolvedConfig{
		Endpoint:           resolveString(c.Endpoint, "AGENTGATEWAY_ENDPOINT", "http://127.0.0.1:15000"),
		Token:              resolveStringNoDefault(c.Token, "AGENTGATEWAY_TOKEN"),
		Username:           resolveStringNoDefault(c.Username, "AGENTGATEWAY_USERNAME"),
		Password:           resolveStringNoDefault(c.Password, "AGENTGATEWAY_PASSWORD"),
		HTTPHeaders:        resolveMap(c.HTTPHeaders),
		Timeout:            resolveDuration(c.Timeout, "AGENTGATEWAY_TIMEOUT", 30*time.Second),
		CACert:             resolveStringNoDefault(c.CACert, "AGENTGATEWAY_CA_CERT"),
		CACertFile:         resolveStringNoDefault(c.CACertFile, "AGENTGATEWAY_CA_CERT_FILE"),
		ClientCert:         resolveStringNoDefault(c.ClientCert, "AGENTGATEWAY_CLIENT_CERT"),
		ClientCertFile:     resolveStringNoDefault(c.ClientCertFile, "AGENTGATEWAY_CLIENT_CERT_FILE"),
		ClientKey:          resolveStringNoDefault(c.ClientKey, "AGENTGATEWAY_CLIENT_KEY"),
		ClientKeyFile:      resolveStringNoDefault(c.ClientKeyFile, "AGENTGATEWAY_CLIENT_KEY_FILE"),
		InsecureSkipVerify: resolveBool(c.InsecureSkipVerify, "AGENTGATEWAY_INSECURE_SKIP_VERIFY", false),
		Verify:             resolveBool(c.Verify, "AGENTGATEWAY_VERIFY", true),
		StorageModeCheck:   resolveString(c.StorageModeCheck, "", "error"),
	}
}

// Validate checks the resolved configuration for semantic errors.
func (c Config) Validate() diag.Diagnostics {
	var diags diag.Diagnostics
	r := c.Resolve()

	if err := validateEndpoint(r.Endpoint); err != nil {
		diags.AddError("Invalid endpoint", err.Error())
	}

	if r.Token != "" && (r.Username != "" || r.Password != "") {
		diags.AddError(
			"Conflicting authentication configuration",
			"token cannot be used with username or password",
		)
	}

	if r.Password != "" && r.Username == "" {
		diags.AddError(
			"Missing username",
			"password requires username to be set",
		)
	}

	if r.Username != "" || r.Password != "" {
		u, err := url.Parse(r.Endpoint)
		if err == nil && u.Scheme == "http" {
			diags.AddError(
				"Insecure basic authentication",
				"basic authentication requires an https endpoint",
			)
		}
	}

	if err := exactlyOneOf("ca_cert", r.CACert, "ca_cert_file", r.CACertFile); err != nil {
		diags.AddError("Invalid CA certificate configuration", err.Error())
	}
	if err := exactlyOneOf("client_cert", r.ClientCert, "client_cert_file", r.ClientCertFile); err != nil {
		diags.AddError("Invalid client certificate configuration", err.Error())
	}
	if err := exactlyOneOf("client_key", r.ClientKey, "client_key_file", r.ClientKeyFile); err != nil {
		diags.AddError("Invalid client key configuration", err.Error())
	}

	hasCert := r.ClientCert != "" || r.ClientCertFile != ""
	hasKey := r.ClientKey != "" || r.ClientKeyFile != ""
	if hasCert != hasKey {
		diags.AddError(
			"Incomplete mTLS configuration",
			"client_cert/client_cert_file and client_key/client_key_file must both be set",
		)
	}

	if r.Timeout <= 0 {
		diags.AddError("Invalid timeout", "timeout must be a positive duration")
	}

	return diags
}

// BuildHTTPClient builds an HTTP client with the configured TLS settings.
func (c Config) BuildHTTPClient() (*http.Client, error) {
	r := c.Resolve()

	tlsConfig := &tls.Config{
		InsecureSkipVerify: r.InsecureSkipVerify,
		MinVersion:         tls.VersionTLS12,
	}

	if r.CACert != "" || r.CACertFile != "" {
		pool, err := buildRootCAs(r.CACert, r.CACertFile)
		if err != nil {
			return nil, fmt.Errorf("loading CA certificate: %w", err)
		}
		tlsConfig.RootCAs = pool
	}

	if r.ClientCert != "" || r.ClientCertFile != "" {
		cert, err := loadClientCert(r.ClientCert, r.ClientCertFile, r.ClientKey, r.ClientKeyFile)
		if err != nil {
			return nil, fmt.Errorf("loading client certificate: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	return &http.Client{
		Transport: &http.Transport{TLSClientConfig: tlsConfig},
	}, nil
}

// AuthHeaders returns the request headers for authentication and custom headers.
func (c Config) AuthHeaders() (map[string]string, error) {
	r := c.Resolve()
	headers := make(map[string]string, len(r.HTTPHeaders)+1)

	for k, v := range r.HTTPHeaders {
		headers[k] = v
	}

	switch {
	case r.Token != "":
		headers["Authorization"] = "Bearer " + r.Token
	case r.Username != "" || r.Password != "":
		if r.Username == "" {
			return nil, errors.New("password requires username")
		}
		creds := base64.StdEncoding.EncodeToString([]byte(r.Username + ":" + r.Password))
		headers["Authorization"] = "Basic " + creds
	}

	return headers, nil
}

func resolveString(v types.String, env, def string) string {
	if !v.IsNull() && !v.IsUnknown() {
		return v.ValueString()
	}
	if e := os.Getenv(env); e != "" {
		return e
	}
	return def
}

func resolveStringNoDefault(v types.String, env string) string {
	if !v.IsNull() && !v.IsUnknown() {
		return v.ValueString()
	}
	return os.Getenv(env)
}

func resolveDuration(v types.String, env string, def time.Duration) time.Duration {
	var s string
	if !v.IsNull() && !v.IsUnknown() {
		s = v.ValueString()
	} else if e := os.Getenv(env); e != "" {
		s = e
	} else {
		return def
	}

	d, err := time.ParseDuration(s)
	if err != nil {
		return def
	}
	return d
}

func resolveBool(v types.Bool, env string, def bool) bool {
	if !v.IsNull() && !v.IsUnknown() {
		return v.ValueBool()
	}
	s := os.Getenv(env)
	if s == "" {
		return def
	}
	b, err := strconv.ParseBool(s)
	if err != nil {
		return def
	}
	return b
}

func resolveMap(v types.Map) map[string]string {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	out := make(map[string]string, len(v.Elements()))
	for k, val := range v.Elements() {
		sv, ok := val.(types.String)
		if !ok {
			continue
		}
		out[k] = sv.ValueString()
	}
	return out
}

func validateEndpoint(endpoint string) error {
	u, err := url.Parse(endpoint)
	if err != nil {
		return fmt.Errorf("endpoint must be a valid URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return errors.New("endpoint scheme must be http or https")
	}
	if u.Host == "" {
		return errors.New("endpoint must include a host")
	}
	if u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("endpoint must not include a path, query or fragment")
	}
	return nil
}

func exactlyOneOf(nameA, valueA, nameB, valueB string) error {
	hasA := valueA != ""
	hasB := valueB != ""
	if hasA && hasB {
		return fmt.Errorf("%s and %s are mutually exclusive", nameA, nameB)
	}
	return nil
}

func buildRootCAs(certPEM, certFile string) (*x509.CertPool, error) {
	var raw []byte
	if certPEM != "" {
		raw = []byte(certPEM)
	} else {
		var err error
		raw, err = os.ReadFile(certFile)
		if err != nil {
			return nil, err
		}
	}

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(raw) {
		return nil, errors.New("failed to parse CA certificate")
	}
	return pool, nil
}

func loadClientCert(certPEM, certFile, keyPEM, keyFile string) (tls.Certificate, error) {
	if certPEM != "" && keyPEM != "" {
		return tls.X509KeyPair([]byte(certPEM), []byte(keyPEM))
	}
	return tls.LoadX509KeyPair(certFile, keyFile)
}
