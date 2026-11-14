package acctest

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// childMarker marks the child processes the tests below start. A child test
// does nothing when the marker is absent, so an ordinary test run only
// exercises the parent.
const childMarker = "ACCTEST_CHILD_PROCESS"

// TestEndpointForRoles pins the mapping from a role to its environment
// variable and the value the helper returns for it.
func TestEndpointForRoles(t *testing.T) {
	t.Setenv(EnvEndpoint, "http://hybrid:15000")
	t.Setenv(EnvEndpointReadOnly, "http://readonly:15001")
	t.Setenv(EnvEndpointFileOwned, "http://fileowned:15002")
	t.Setenv(EnvEndpointFileMode, "http://filemode:15003")

	tests := []struct {
		name string
		role Role
		want string
	}{
		{name: "hybrid", role: RoleHybrid, want: "http://hybrid:15000"},
		{name: "read-only", role: RoleReadOnly, want: "http://readonly:15001"},
		{name: "file-owned", role: RoleFileOwned, want: "http://fileowned:15002"},
		{name: "file-mode", role: RoleFileMode, want: "http://filemode:15003"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := EndpointFor(t, tc.role); got != tc.want {
				t.Errorf("EndpointFor(%q) = %q, want %q", tc.role, got, tc.want)
			}
		})
	}
}

// TestEndpointForFailsWithoutEnvironmentVariable pins that a missing
// environment variable fails the test instead of skipping it.
//
// The failure happens in a child process: a failing subtest would fail the
// parent as well, so the parent cannot observe the failure itself.
func TestEndpointForFailsWithoutEnvironmentVariable(t *testing.T) {
	out, code := runChild(t, "TestEndpointForFatalChild", childMarker+"=1")
	if code == 0 {
		t.Fatalf("EndpointFor returned instead of failing the test; child output:\n%s", out)
	}
	if !strings.Contains(out, EnvEndpointReadOnly+" must be set") {
		t.Fatalf("the child failed for another reason; output:\n%s", out)
	}
}

// TestPreCheckSkipsWithoutTFAcc pins that the suite skips when acceptance
// testing is not enabled. The skip happens in a child process, because a
// skipped test cannot be observed from the inside.
func TestPreCheckSkipsWithoutTFAcc(t *testing.T) {
	out, code := runChild(t, "TestPreCheckSkipsChild", childMarker+"=1")
	if code != 0 {
		t.Fatalf("the child failed instead of skipping; output:\n%s", out)
	}
	if !strings.Contains(out, "--- SKIP: TestPreCheckSkipsChild") {
		t.Fatalf("the child did not skip the test; output:\n%s", out)
	}
}

// TestPreCheckFailsWhenGatewayIsUnreachable pins that an unreachable gateway
// fails the test when acceptance testing is enabled: a missing capability is
// an environment failure, never a skip.
func TestPreCheckFailsWhenGatewayIsUnreachable(t *testing.T) {
	out, code := runChild(t, "TestPreCheckUnreachableChild", childMarker+"=1")
	if code == 0 {
		t.Fatalf("PreCheck returned instead of failing the test; child output:\n%s", out)
	}
	if !strings.Contains(out, "is not reachable") {
		t.Fatalf("the child failed for another reason; output:\n%s", out)
	}
}

// TestPreCheckAcceptsAReachableGateway pins the happy path of the guard:
// with TF_ACC set and a gateway that answers, PreCheck neither skips nor
// fails the test.
func TestPreCheckAcceptsAReachableGateway(t *testing.T) {
	t.Setenv("TF_ACC", "1")
	t.Setenv(EnvExpectVersion, "")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"build":{"version":"v1.7.0"}}`))
	}))
	defer server.Close()
	t.Setenv(EnvEndpoint, server.URL)
	PreCheck(t)
}

// TestAcceptanceEnabled pins the switch that decides whether the suite runs.
func TestAcceptanceEnabled(t *testing.T) {
	t.Setenv("TF_ACC", "1")
	if !acceptanceEnabled() {
		t.Error("acceptanceEnabled() = false, want true with TF_ACC set")
	}
	t.Setenv("TF_ACC", "")
	if acceptanceEnabled() {
		t.Error("acceptanceEnabled() = true, want false without TF_ACC")
	}
}

// TestEndpointForFatalChild runs only in the child process that
// TestEndpointForFailsWithoutEnvironmentVariable starts. The environment
// variable of the role is unset there, so EndpointFor has to fail the test.
func TestEndpointForFatalChild(t *testing.T) {
	if os.Getenv(childMarker) == "" {
		t.Skip("child process only")
	}
	EndpointFor(t, RoleReadOnly)
	t.Error("EndpointFor returned instead of failing the test")
}

// TestPreCheckSkipsChild runs only in the child process that
// TestPreCheckSkipsWithoutTFAcc starts. Without TF_ACC the helper has to
// skip the test.
func TestPreCheckSkipsChild(t *testing.T) {
	if os.Getenv(childMarker) == "" {
		t.Skip("child process only")
	}
	PreCheck(t)
	t.Error("PreCheck returned instead of skipping the test")
}

// TestPreCheckUnreachableChild runs only in the child process that
// TestPreCheckFailsWhenGatewayIsUnreachable starts. TF_ACC is set and the
// endpoint points at a closed port, so the helper has to fail the test.
func TestPreCheckUnreachableChild(t *testing.T) {
	if os.Getenv(childMarker) == "" {
		t.Skip("child process only")
	}
	t.Setenv("TF_ACC", "1")
	t.Setenv(EnvEndpoint, "http://127.0.0.1:1")
	PreCheck(t)
	t.Error("PreCheck returned instead of failing the test")
}

// TestRoleEndpointVariable pins the environment variable of every role.
func TestRoleEndpointVariable(t *testing.T) {
	tests := []struct {
		role Role
		want string
	}{
		{role: RoleHybrid, want: EnvEndpoint},
		{role: RoleReadOnly, want: EnvEndpointReadOnly},
		{role: RoleFileOwned, want: EnvEndpointFileOwned},
		{role: RoleFileMode, want: EnvEndpointFileMode},
		{role: Role("unknown"), want: ""},
	}
	for _, tc := range tests {
		if got := tc.role.endpointVar(); got != tc.want {
			t.Errorf("Role(%q).endpointVar() = %q, want %q", tc.role, got, tc.want)
		}
	}
}

// TestLookupEndpoint pins that the endpoint is read from the environment of
// the role, and that an unset or blank variable is not an endpoint.
func TestLookupEndpoint(t *testing.T) {
	t.Setenv(EnvEndpoint, "http://hybrid:15000")
	t.Setenv(EnvEndpointReadOnly, "  http://readonly:15001  ")
	t.Setenv(EnvEndpointFileOwned, "")

	if got, ok := lookupEndpoint(RoleHybrid); !ok || got != "http://hybrid:15000" {
		t.Errorf("lookupEndpoint(hybrid) = %q, %v; want %q, true", got, ok, "http://hybrid:15000")
	}
	if got, ok := lookupEndpoint(RoleReadOnly); !ok || got != "http://readonly:15001" {
		t.Errorf("lookupEndpoint(read-only) = %q, %v; want %q, true", got, ok, "http://readonly:15001")
	}
	if _, ok := lookupEndpoint(RoleFileOwned); ok {
		t.Error("lookupEndpoint(file-owned) found an endpoint, want none")
	}
	if _, ok := lookupEndpoint(Role("unknown")); ok {
		t.Error("lookupEndpoint(unknown) found an endpoint, want none")
	}
}

// TestCheckGateway pins the environment checks that run before a test
// touches the gateway.
func TestCheckGateway(t *testing.T) {
	t.Run("reports an unreachable gateway", func(t *testing.T) {
		err := checkGateway(t.Context(), "http://127.0.0.1:1")
		if err == nil {
			t.Fatal("checkGateway accepted an unreachable gateway")
		}
		if !strings.Contains(err.Error(), "is not reachable") {
			t.Errorf("error = %q, want it to report an unreachable gateway", err)
		}
	})

	t.Run("accepts the expected release", func(t *testing.T) {
		t.Setenv(EnvExpectVersion, "v1.7.0")
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"build":{"version":"v1.7.0"}}`))
		}))
		defer server.Close()
		if err := checkGateway(t.Context(), server.URL); err != nil {
			t.Fatalf("checkGateway: %v", err)
		}
	})

	t.Run("reports another release", func(t *testing.T) {
		t.Setenv(EnvExpectVersion, "v1.7.0")
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"build":{"version":"v1.6.0"}}`))
		}))
		defer server.Close()
		err := checkGateway(t.Context(), server.URL)
		if err == nil {
			t.Fatal("checkGateway accepted another release")
		}
		if !strings.Contains(err.Error(), EnvExpectVersion) {
			t.Errorf("error = %q, want it to name %s", err, EnvExpectVersion)
		}
	})

	t.Run("accepts any release when none is expected", func(t *testing.T) {
		t.Setenv(EnvExpectVersion, "")
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"build":{"version":"v1.6.0"}}`))
		}))
		defer server.Close()
		if err := checkGateway(t.Context(), server.URL); err != nil {
			t.Fatalf("checkGateway: %v", err)
		}
	})
}

// TestFetchRuntime pins the probe against a gateway that answers, a gateway
// that fails and a gateway that answers with something else than a runtime
// document.
func TestFetchRuntime(t *testing.T) {
	t.Run("reads the build version", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/runtime" {
				t.Errorf("the probe requested %q, want %q", r.URL.Path, "/api/runtime")
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"build":{"version":"v1.7.0"}}`))
		}))
		defer server.Close()
		info, err := fetchRuntime(t.Context(), server.URL)
		if err != nil {
			t.Fatalf("fetchRuntime: %v", err)
		}
		if info.Build.Version != "v1.7.0" {
			t.Errorf("build version = %q, want %q", info.Build.Version, "v1.7.0")
		}
	})

	t.Run("rejects a failing status", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()
		if _, err := fetchRuntime(t.Context(), server.URL); err == nil {
			t.Fatal("fetchRuntime accepted a failing status")
		}
	})

	t.Run("rejects a document that is not JSON", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("not a runtime document"))
		}))
		defer server.Close()
		if _, err := fetchRuntime(t.Context(), server.URL); err == nil {
			t.Fatal("fetchRuntime accepted a document that is not JSON")
		}
	})
}

// TestVersionMatches pins the comparison between the expected release and
// the release the gateway reports.
func TestVersionMatches(t *testing.T) {
	tests := []struct {
		expected string
		actual   string
		want     bool
	}{
		{expected: "v1.7.0", actual: "v1.7.0", want: true},
		{expected: "v1.7.0", actual: "1.7.0", want: true},
		{expected: "1.7.0", actual: "v1.7.0", want: true},
		{expected: "v1.7.0", actual: "v1.7.0-dev", want: true},
		{expected: "v1.7.0", actual: "v1.7.1", want: false},
		{expected: "v1.7.0", actual: "v1.7.01", want: false},
		{expected: "v1.7.0", actual: "", want: false},
		{expected: "", actual: "v1.7.0", want: false},
	}
	for _, tc := range tests {
		if got := versionMatches(tc.expected, tc.actual); got != tc.want {
			t.Errorf("versionMatches(%q, %q) = %v, want %v", tc.expected, tc.actual, got, tc.want)
		}
	}
}

// runChild runs one test of this binary in a child process and returns its
// output and exit code. The child sees none of the variables that steer the
// acceptance helpers, so the test decides exactly what the child sees.
func runChild(t *testing.T, testName string, extra ...string) (string, int) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^"+testName+"$", "-test.v")
	cmd.Env = append(childEnv(), extra...)
	out, err := cmd.CombinedOutput()
	code := 0
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("run the %s child process: %v", testName, err)
		}
		code = exitErr.ExitCode()
	}
	return string(out), code
}

// childEnv returns the environment of the test process without the variables
// that steer the acceptance helpers.
func childEnv() []string {
	drop := map[string]bool{
		"TF_ACC":             true,
		EnvEndpoint:          true,
		EnvEndpointReadOnly:  true,
		EnvEndpointFileOwned: true,
		EnvEndpointFileMode:  true,
		EnvExpectVersion:     true,
	}
	kept := make([]string, 0, len(os.Environ()))
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if !drop[name] {
			kept = append(kept, entry)
		}
	}
	return kept
}
