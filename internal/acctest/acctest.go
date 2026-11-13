// Package acctest holds the helpers that every acceptance test of a resource
// or a data source shares.
//
// The acceptance suite runs against a real agentgateway (design §10). The
// helpers here answer three questions for a test: is the suite supposed to
// run at all, which gateway does the test talk to, and is that gateway the
// one the test expects.
//
// The package depends on the standard library only. It must not import
// internal/client: the environment is checked independently of the
// provider's own HTTP code, so a bug in the client can never hide a broken
// acceptance environment.
package acctest

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// Role names one of the gateways in the acceptance environment.
type Role string

const (
	// RoleHybrid is the hybrid-storage gateway: the happy path for every
	// resource, and the gateway the provider talks to by default.
	RoleHybrid Role = "hybrid"
	// RoleReadOnly is the read-only gateway: every write fails with 403.
	RoleReadOnly Role = "readonly"
	// RoleFileOwned is the gateway whose config file owns a settings field:
	// a write that changes that field fails with 409.
	RoleFileOwned Role = "fileowned"
)

// Environment variables that point the suite at the acceptance environment.
const (
	// EnvEndpoint carries the endpoint of the hybrid gateway. It is also the
	// provider's own endpoint variable (design §6).
	EnvEndpoint = "AGENTGATEWAY_ENDPOINT"
	// EnvEndpointReadOnly carries the endpoint of the read-only gateway.
	EnvEndpointReadOnly = "AGENTGATEWAY_ENDPOINT_READONLY"
	// EnvEndpointFileOwned carries the endpoint of the file-owned gateway.
	EnvEndpointFileOwned = "AGENTGATEWAY_ENDPOINT_FILEOWNED"
	// EnvExpectVersion carries the release the suite expects. The version
	// matrix and CI set it; PreCheck fails when the gateway reports another
	// release, so a stale image cannot pass as the release under test.
	EnvExpectVersion = "AGENTGATEWAY_EXPECT_VERSION"
)

// probeTimeout bounds the runtime probe. The gateway answers in
// milliseconds, so a timeout means the endpoint is not the gateway we
// expect.
const probeTimeout = 5 * time.Second

// maxRuntimeBody bounds the runtime document the probe reads.
const maxRuntimeBody = 1 << 20

// PreCheck skips the test unless acceptance testing is enabled, and fails it
// when the environment is not the one the test needs.
//
// A missing capability is an environment failure, never a skip: once TF_ACC
// is set, the suite has to run against the gateway it was told to use, so an
// unreachable gateway or a gateway of the wrong release fails the test
// instead of silently passing it.
func PreCheck(t *testing.T) {
	t.Helper()
	if !acceptanceEnabled() {
		t.Skip("TF_ACC not set; skipping the acceptance test")
	}
	endpoint := EndpointFor(t, RoleHybrid)
	if err := checkGateway(t.Context(), endpoint); err != nil {
		t.Fatal(err)
	}
}

// acceptanceEnabled reports whether the acceptance suite is supposed to run.
func acceptanceEnabled() bool {
	return os.Getenv("TF_ACC") != ""
}

// EndpointFor returns the endpoint of the gateway that plays the role.
//
// The endpoint comes from the environment because the acceptance environment
// runs more than one gateway and every test has to address the one it needs.
// A missing variable is an environment failure, not a skip: a test that
// silently skipped would report success without testing anything.
func EndpointFor(t *testing.T, role Role) string {
	t.Helper()
	name := role.endpointVar()
	if name == "" {
		t.Fatalf("unknown acceptance role %q; expected %q, %q or %q", role, RoleHybrid, RoleReadOnly, RoleFileOwned)
	}
	endpoint, ok := lookupEndpoint(role)
	if !ok {
		t.Fatalf("%s must be set for acceptance tests; run the suite with `make testacc-docker`", name)
	}
	return endpoint
}

// checkGateway probes the gateway and reports why the environment is not
// usable: the endpoint does not answer, it does not answer with a runtime
// document, or it reports another release than the suite expects.
func checkGateway(ctx context.Context, endpoint string) error {
	info, err := fetchRuntime(ctx, endpoint)
	if err != nil {
		return fmt.Errorf("the agentgateway at %s is not reachable: %w", endpoint, err)
	}
	expected := strings.TrimSpace(os.Getenv(EnvExpectVersion))
	if expected == "" {
		return nil
	}
	if !versionMatches(expected, info.Build.Version) {
		return fmt.Errorf("%s=%s but the gateway at %s reports %q; check the image tag and the compose project",
			EnvExpectVersion, expected, endpoint, info.Build.Version)
	}
	return nil
}

// endpointVar returns the environment variable that carries the endpoint of
// the role, or an empty string for a role the environment does not know.
func (r Role) endpointVar() string {
	switch r {
	case RoleHybrid:
		return EnvEndpoint
	case RoleReadOnly:
		return EnvEndpointReadOnly
	case RoleFileOwned:
		return EnvEndpointFileOwned
	default:
		return ""
	}
}

// lookupEndpoint reads the endpoint of the role from the environment.
func lookupEndpoint(role Role) (string, bool) {
	name := role.endpointVar()
	if name == "" {
		return "", false
	}
	endpoint := strings.TrimSpace(os.Getenv(name))
	if endpoint == "" {
		return "", false
	}
	return endpoint, true
}

// runtimeInfo is the part of GET /api/runtime the helpers read. The runtime
// document reports the build of the gateway (design C20).
type runtimeInfo struct {
	Build struct {
		Version string `json:"version"`
	} `json:"build"`
}

// fetchRuntime reads the runtime document from the admin address. The admin
// address has no authentication (design C1), so a plain GET is enough.
func fetchRuntime(ctx context.Context, endpoint string) (runtimeInfo, error) {
	var info runtimeInfo
	url := strings.TrimSuffix(strings.TrimSpace(endpoint), "/") + "/api/runtime"
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return info, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return info, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return info, fmt.Errorf("GET %s returned %s", url, resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxRuntimeBody))
	if err != nil {
		return info, err
	}
	if err := json.Unmarshal(body, &info); err != nil {
		return info, fmt.Errorf("GET %s did not return a runtime document: %w", url, err)
	}
	return info, nil
}

// versionMatches compares the release the suite expects with the release the
// gateway reports. It accepts an exact match and a build suffix, such as
// "1.7.0" for "1.7.0-dev" or "v1.7.0" for "1.7.0", and rejects a different
// release.
func versionMatches(expected, actual string) bool {
	expected = strings.TrimPrefix(strings.TrimSpace(expected), "v")
	actual = strings.TrimPrefix(strings.TrimSpace(actual), "v")
	if expected == "" || actual == "" {
		return false
	}
	if actual == expected {
		return true
	}
	rest := strings.TrimPrefix(actual, expected)
	return rest != actual && (strings.HasPrefix(rest, "-") || strings.HasPrefix(rest, ".") || strings.HasPrefix(rest, "+"))
}
