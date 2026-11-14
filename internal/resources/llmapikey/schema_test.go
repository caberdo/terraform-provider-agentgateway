package llmapikey

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/caberdo/terraform-provider-agentgateway/internal/agentgatewaytest"
	"github.com/caberdo/terraform-provider-agentgateway/internal/client"
	"github.com/caberdo/terraform-provider-agentgateway/internal/configresource"
	"github.com/caberdo/terraform-provider-agentgateway/internal/providerdata"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAPIKeyRequiresKeyOrHash pins both halves of the key xor key_hash rule:
// the declarative validator rejects the configuration at plan time, and Value()
// repeats the check for a model that was not built from a plan.
func TestAPIKeyRequiresKeyOrHash(t *testing.T) {
	t.Run("neither attribute", func(t *testing.T) {
		t.Parallel()

		server, _ := agentgatewaytest.New(t)
		resource.UnitTest(t, resource.TestCase{
			ProtoV6ProviderFactories: testFactories(server.URL),
			Steps: []resource.TestStep{
				{
					Config: `
resource "agentgateway_llm_api_key" "test" {
  metadata = { team = "research" }
}`,
					ExpectError: regexp.MustCompile(`one \(and only one\)`),
				},
			},
		})
	})

	t.Run("both attributes", func(t *testing.T) {
		t.Parallel()

		server, _ := agentgatewaytest.New(t)
		resource.UnitTest(t, resource.TestCase{
			ProtoV6ProviderFactories: testFactories(server.URL),
			Steps: []resource.TestStep{
				{
					Config: `
resource "agentgateway_llm_api_key" "test" {
  key      = "sk-acctest-0001"
  key_hash = "` + apiKeyHash + `"
}`,
					ExpectError: regexp.MustCompile(`one \(and only one\)`),
				},
			},
		})
	})

	t.Run("value rejects a model without key material", func(t *testing.T) {
		t.Parallel()

		model := newModel()
		var diags diag.Diagnostics
		model.Value(context.Background(), &diags)

		if !diags.HasError() {
			t.Fatal("Value() returned no error without key material")
		}
		if detail := diags[0].Detail(); !strings.Contains(detail, "exactly one of key or key_hash must be set") {
			t.Errorf("detail = %q, want the exactly-one rule", detail)
		}
	})
}

// TestAPIKeyHashFormat pins the documented sha256 hash format (spec C12).
func TestAPIKeyHashFormat(t *testing.T) {
	t.Run("valid hash is accepted", func(t *testing.T) {
		t.Parallel()

		server, _ := agentgatewaytest.New(t)
		resource.UnitTest(t, resource.TestCase{
			ProtoV6ProviderFactories: testFactories(server.URL),
			Steps: []resource.TestStep{
				{
					Config: `
resource "agentgateway_llm_api_key" "test" {
  key_hash = "` + apiKeyHash + `"
}`,
					Check: resource.TestCheckResourceAttrSet("agentgateway_llm_api_key.test", "id"),
				},
			},
		})
	})

	tests := []struct {
		name    string
		keyHash string
	}{
		{name: "uppercase hex", keyHash: strings.ToUpper(apiKeyHash)},
		{name: "missing prefix", keyHash: strings.TrimPrefix(apiKeyHash, "sha256:")},
		{name: "truncated digest", keyHash: "sha256:9f86d081884c7d659a2feaa0c55ad015"},
		{name: "wrong algorithm", keyHash: strings.Replace(apiKeyHash, "sha256:", "sha512:", 1)},
	}

	for _, tt := range tests {
		t.Run(tt.name+" is rejected", func(t *testing.T) {
			t.Parallel()

			server, _ := agentgatewaytest.New(t)
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testFactories(server.URL),
				Steps: []resource.TestStep{
					{
						Config: `
resource "agentgateway_llm_api_key" "test" {
  key_hash = "` + tt.keyHash + `"
}`,
						ExpectError: regexp.MustCompile(`must be a sha256 key hash`),
					},
				},
			})
		})
	}
}

// TestAPIKeyReservedMetadataRejected pins the reserved-prefix rule (spec C12):
// the gateway owns agentgateway.dev/ and the schema rejects it before a request
// is sent.
func TestAPIKeyReservedMetadataRejected(t *testing.T) {
	t.Parallel()

	server, _ := agentgatewaytest.New(t)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactories(server.URL),
		Steps: []resource.TestStep{
			{
				Config: `
resource "agentgateway_llm_api_key" "test" {
  key = "sk-acctest-0002"

  metadata = {
    "team"                 = "research"
    "agentgateway.dev/team" = "research"
  }
}`,
				ExpectError: regexp.MustCompile(`reserved prefix`),
			},
		},
	})
}

// TestAPIKeyAllowedModelsNullVsEmpty pins spec C13 at the protocol level: an
// omitted attribute sends no allowedModels key, and an empty list sends [].
func TestAPIKeyAllowedModelsNullVsEmpty(t *testing.T) {
	t.Run("omitted sends no allowedModels", func(t *testing.T) {
		t.Parallel()

		server, _ := agentgatewaytest.New(t)
		resource.UnitTest(t, resource.TestCase{
			ProtoV6ProviderFactories: testFactories(server.URL),
			Steps: []resource.TestStep{
				{
					Config: `
resource "agentgateway_llm_api_key" "test" {
  key = "sk-acctest-0003"
}`,
					Check: resource.TestCheckNoResourceAttr("agentgateway_llm_api_key.test", "allowed_models"),
				},
			},
		})

		body := createdAPIKeyBody(t, server)
		if raw, ok := body["allowedModels"]; ok {
			t.Errorf("allowedModels = %s in the payload, want the key omitted", raw)
		}
	})

	t.Run("empty list sends allowedModels", func(t *testing.T) {
		t.Parallel()

		server, _ := agentgatewaytest.New(t)
		resource.UnitTest(t, resource.TestCase{
			ProtoV6ProviderFactories: testFactories(server.URL),
			Steps: []resource.TestStep{
				{
					Config: `
resource "agentgateway_llm_api_key" "test" {
  key            = "sk-acctest-0004"
  allowed_models = []
}`,
					Check: resource.TestCheckResourceAttr("agentgateway_llm_api_key.test", "allowed_models.#", "0"),
				},
			},
		})

		body := createdAPIKeyBody(t, server)
		raw, ok := body["allowedModels"]
		if !ok {
			t.Fatal("allowedModels is missing from the payload, want []")
		}
		if string(raw) != "[]" {
			t.Errorf("allowedModels = %s, want []", raw)
		}
	})
}

// TestAPIKeyMissingPolicyIsActionable pins the diagnostic the resource adds when
// the gateway refuses a key because llm.policies.apiKey is missing (spec C10).
// The gateway message is authoritative when the harness surfaces it.
func TestAPIKeyMissingPolicyIsActionable(t *testing.T) {
	t.Parallel()

	diags := diag.Diagnostics{
		diag.NewErrorDiagnostic(
			"agentgateway_llm_api_key create failed",
			`create llm.apiKey/x failed. conflict detected; see the gateway message below. Gateway message: DB-backed API keys require llm.policies.apiKey`,
		),
	}
	r := &apiKeyResource{}
	r.addPolicyPrerequisite(context.Background(), &diags)

	assertPolicyPrerequisiteDiagnostic(t, diags)
}

// TestAPIKeyConflictProbesTheMergedConfig covers the create path: a conflict
// with a server-generated id carries no gateway message, so the resource probes
// the merged configuration to tell a missing policy from a generation conflict.
func TestAPIKeyConflictProbesTheMergedConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		effective string
		err       error
		wantHint  bool
	}{
		{
			name:      "policy missing",
			effective: `{"llm":{"policies":{"cors":{}}}}`,
			wantHint:  true,
		},
		{
			name:      "no llm section at all",
			effective: `{}`,
			wantHint:  true,
		},
		{
			name:      "file-owned policy satisfies the prerequisite",
			effective: `{"llm":{"policies":{"apiKey":{"mode":"strict"}}}}`,
			wantHint:  false,
		},
		{
			name:     "probe failure leaves the harness wording",
			err:      errors.New("gateway unreachable"),
			wantHint: false,
		},
		{
			name:      "unparseable effective config leaves the harness wording",
			effective: `{"llm":`,
			wantHint:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := &apiKeyResource{data: &providerdata.Data{Client: stubClient{effective: tt.effective, err: tt.err}}}
			diags := diag.Diagnostics{
				diag.NewErrorDiagnostic(
					"agentgateway_llm_api_key create failed",
					"create for llm.apiKey. conflict detected; see the gateway message below.",
				),
			}
			r.addPolicyPrerequisite(context.Background(), &diags)

			if tt.wantHint {
				assertPolicyPrerequisiteDiagnostic(t, diags)
				return
			}
			if got := diags[0].Detail(); !strings.Contains(got, conflictDiagnosticMarker) {
				t.Errorf("detail = %q, want the harness wording kept", got)
			}
		})
	}
}

// TestAPIKeyOtherDiagnosticsAreUntouched proves the prerequisite rewrite is
// narrow: a non-conflict error and a warning that carries the marker keep their
// wording.
func TestAPIKeyOtherDiagnosticsAreUntouched(t *testing.T) {
	t.Parallel()

	invalid := diag.NewErrorDiagnostic("agentgateway_llm_api_key update failed", "the request was invalid; correct the key_hash attribute")
	warning := diag.NewWarningDiagnostic("agentgateway_llm_api_key update warning", apiKeyPolicyPrerequisite)

	diags := diag.Diagnostics{invalid, warning}
	r := &apiKeyResource{data: &providerdata.Data{Client: stubClient{effective: `{}`}}}
	r.addPolicyPrerequisite(context.Background(), &diags)

	if got := diags[0].Detail(); got != "the request was invalid; correct the key_hash attribute" {
		t.Errorf("invalid detail = %q, want it untouched", got)
	}
	if got := diags[1].Detail(); got != apiKeyPolicyPrerequisite {
		t.Errorf("warning detail = %q, want it untouched", got)
	}
}

// TestAPIKeyCreateConflictIsActionable drives the whole path against the
// protocol fake: the gateway answers 409, the fake's effective config has no
// apiKey policy, and the user sees the actionable diagnostic.
func TestAPIKeyCreateConflictIsActionable(t *testing.T) {
	t.Parallel()

	server, _ := agentgatewaytest.New(t)
	server.FailNextWrites(1, 409)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactories(server.URL),
		Steps: []resource.TestStep{
			{
				Config: `
resource "agentgateway_llm_api_key" "test" {
  key = "sk-acctest-0009"
}`,
				ExpectError: regexp.MustCompile(`agentgateway_llm_policy`),
			},
		},
	})
}

func assertPolicyPrerequisiteDiagnostic(t *testing.T, diags diag.Diagnostics) {
	t.Helper()

	if len(diags) != 1 {
		t.Fatalf("diagnostics = %v, want exactly one", diags)
	}
	detail := diags[0].Detail()
	for _, want := range []string{"agentgateway_llm_policy", `name = "apiKey"`, "llm.policies.apiKey"} {
		if !strings.Contains(detail, want) {
			t.Errorf("detail = %q, want it to mention %q", detail, want)
		}
	}
	if strings.Contains(detail, "Gateway message") {
		t.Errorf("detail = %q, want the raw gateway message replaced", detail)
	}
}

// stubClient is a client.Client whose effective config is fixed, so the probe
// can be tested without an HTTP server.
type stubClient struct {
	effective string
	err       error
}

func (s stubClient) Runtime(context.Context) (client.RuntimeInfo, error) {
	return client.RuntimeInfo{}, s.err
}

func (s stubClient) List(context.Context, configresource.Kind) ([]configresource.Resource, error) {
	return nil, s.err
}

func (s stubClient) Create(context.Context, configresource.Kind, json.RawMessage) (configresource.Resource, error) {
	return configresource.Resource{}, s.err
}

func (s stubClient) Update(context.Context, configresource.Kind, string, json.RawMessage) (configresource.Resource, error) {
	return configresource.Resource{}, s.err
}

func (s stubClient) Delete(context.Context, configresource.Kind, string) error {
	return s.err
}

func (s stubClient) EffectiveConfig(context.Context) (json.RawMessage, error) {
	if s.err != nil {
		return nil, s.err
	}
	return json.RawMessage(s.effective), nil
}

// createdAPIKeyBody returns the value object of the collection PUT the resource
// sent to the protocol fake.
func createdAPIKeyBody(t *testing.T, server *agentgatewaytest.Server) map[string]json.RawMessage {
	t.Helper()

	for _, request := range server.Requests() {
		if request.Method != "PUT" || request.Path != "/api/config/resources/llm.apiKey" {
			continue
		}
		var body map[string]json.RawMessage
		if err := json.Unmarshal(request.Body, &body); err != nil {
			t.Fatalf("decoding create request body %s: %v", request.Body, err)
		}
		return body
	}

	t.Fatal("the resource sent no llm.apiKey create request")
	return nil
}

// TestAPIKeySchemaMarksSecretsSensitive guards the sensitive flags: key and
// key_hash must never be readable from a non-sensitive attribute.
func TestAPIKeySchemaMarksSecretsSensitive(t *testing.T) {
	t.Parallel()

	apiKeySchema := newSchema()
	for _, name := range []string{"key", "key_hash"} {
		attribute, ok := apiKeySchema.Attributes[name]
		if !ok {
			t.Fatalf("%s is missing from the schema", name)
		}
		if !attribute.IsSensitive() {
			t.Errorf("%s is not sensitive", name)
		}
	}

	if !apiKeySchema.Attributes["id"].IsComputed() {
		t.Error("id is not computed; the server generates it")
	}
	if got := newModel().AllowedModels; !got.IsNull() || got.ElementType(context.Background()) != types.StringType {
		t.Errorf("allowed_models is not a typed null list: %v", got)
	}
}
