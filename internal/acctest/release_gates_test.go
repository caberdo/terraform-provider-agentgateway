// Package acctest_test holds the cross-resource acceptance cases that the
// release gates in spec §14 require: the 403 read-only diagnostic, the 409
// missing-apiKey-policy diagnostic, the collection-PUT upsert regression (C21),
// drift detection and the file-mode singleton rejection.
//
// These cases use the real provider, not a per-resource minimal test provider,
// because the behaviour under test is in the provider's Configure (storage
// mode) or in the shared CRUD harness. They live in the external test package
// so the acctest helpers stay free of a dependency on internal/provider.
package acctest_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/caberdo/terraform-provider-agentgateway/internal/acctest"
	"github.com/caberdo/terraform-provider-agentgateway/internal/client"
	"github.com/caberdo/terraform-provider-agentgateway/internal/configresource"
	"github.com/caberdo/terraform-provider-agentgateway/internal/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// releaseGateProvider returns the real provider. The endpoint and the storage
// mode policy come from the generated configuration, so the test exercises the
// same Configure path a user does.
func releaseGateProvider() map[string]func() (tfprotov6.ProviderServer, error) {
	return map[string]func() (tfprotov6.ProviderServer, error){
		"agentgateway": providerserver.NewProtocol6WithError(provider.New("acctest")()),
	}
}

// providerConfig renders a provider block for the endpoint, with extra
// attribute lines such as `storage_mode_check = "ignore"`.
func providerConfig(endpoint string, extra ...string) string {
	lines := []string{
		`provider "agentgateway" {`,
		fmt.Sprintf("  endpoint = %q", endpoint),
	}
	lines = append(lines, extra...)
	lines = append(lines, "}")
	return strings.Join(lines, "\n")
}

// releaseGateClient builds a client pointed at the acceptance gateway. The
// admin address has no authentication (C1), so the default HTTP client is
// enough.
func releaseGateClient(t *testing.T, endpoint string) client.Client {
	t.Helper()

	c, err := client.New(client.Config{
		BaseURL:    endpoint,
		HTTPClient: http.DefaultClient,
		UserAgent:  "terraform-provider-agentgateway/acctest",
	})
	if err != nil {
		t.Fatalf("creating acceptance client for %s: %v", endpoint, err)
	}
	return c
}

// TestAccReadOnlyStorageDiagnostic pins spec §14.3's 403 case: with
// `storage_mode_check = "ignore"` the provider lets the write through, the
// gateway answers 403, and the diagnostic says the storage is read-only and
// names the setting that fixes it.
func TestAccReadOnlyStorageDiagnostic(t *testing.T) {
	acctest.PreCheck(t)
	endpoint := acctest.EndpointFor(t, acctest.RoleReadOnly)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: releaseGateProvider(),
		Steps: []resource.TestStep{
			{
				Config: providerConfig(endpoint, `  storage_mode_check = "ignore"`) + `
resource "agentgateway_llm_provider" "readonly" {
  name          = "acctest-readonly"
  provider_name = "openAI"
}
`,
				// The harness maps ErrReadOnly to a detail that says the storage
				// is read-only and points at config.storage.mode, and it keeps
				// the gateway's own message.
				ExpectError: regexp.MustCompile(`(?is)set config\.storage\.mode .*read-only`),
			},
		},
	})
}

// TestAccMissingAPIKeyPolicyDiagnostic pins spec §14.3's 409 case: creating an
// API key while the gateway has no llm.policies.apiKey must fail with a
// diagnostic that names the missing policy and the resource that creates it.
func TestAccMissingAPIKeyPolicyDiagnostic(t *testing.T) {
	acctest.PreCheck(t)
	endpoint := acctest.EndpointFor(t, acctest.RoleHybrid)
	c := releaseGateClient(t, endpoint)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: releaseGateProvider(),
		Steps: []resource.TestStep{
			{
				// The hybrid fixture has no apiKey policy; delete one an earlier
				// test may have left behind so this step always starts from the
				// state the case needs.
				PreConfig: func() {
					err := c.Delete(context.Background(), configresource.KindLLMPolicy, "apiKey")
					if err != nil && !isNotFound(err) {
						t.Fatalf("deleting llm.policy/apiKey before the test: %v", err)
					}
				},
				Config: providerConfig(endpoint) + `
resource "agentgateway_llm_api_key" "gate" {
  key = "sk-acctest-release-gate"
}
`,
				ExpectError: regexp.MustCompile(`(?is)requires the apiKey policy.*agentgateway_llm_policy`),
			},
		},
	})
}

// TestAccCollectionPUTKeepsSiblings pins C21: the collection PUT is an upsert
// and never replaces the kind, so creating a third provider leaves the first
// two in place. The check reads the gateway through the client, not state.
func TestAccCollectionPUTKeepsSiblings(t *testing.T) {
	acctest.PreCheck(t)
	endpoint := acctest.EndpointFor(t, acctest.RoleHybrid)
	c := releaseGateClient(t, endpoint)

	ids := []string{"acctest-c21-a", "acctest-c21-b", "acctest-c21-c"}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: releaseGateProvider(),
		CheckDestroy:             checkProvidersGone(c, ids),
		Steps: []resource.TestStep{
			{
				Config: providerConfig(endpoint) + twoProvidersConfig,
			},
			{
				// Adding the third provider must not delete the first two: a
				// replace-all collection PUT would fail this check.
				Config: providerConfig(endpoint) + twoProvidersConfig + thirdProviderConfig,
				Check:  checkProvidersExist(c, ids),
			},
		},
	})
}

// TestAccDriftDetected pins spec §14's drift requirement: a value changed
// out-of-band through the gateway is detected by the next refresh, so the next
// plan is not empty.
func TestAccDriftDetected(t *testing.T) {
	acctest.PreCheck(t)
	endpoint := acctest.EndpointFor(t, acctest.RoleHybrid)
	c := releaseGateClient(t, endpoint)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: releaseGateProvider(),
		CheckDestroy:             checkProvidersGone(c, []string{driftProviderName}),
		Steps: []resource.TestStep{
			{
				Config: providerConfig(endpoint) + driftProviderConfig("gpt-4o"),
			},
			{
				// Mutate the stored value behind Terraform's back. The step's
				// refresh must read it back and the plan must want the
				// configured value again.
				PreConfig: func() {
					if _, err := c.Update(context.Background(), configresource.KindLLMProvider, driftProviderName,
						[]byte(`{"name":"`+driftProviderName+`","provider":"openAI","params":{"model":"gpt-4o-out-of-band"}}`)); err != nil {
						t.Fatalf("mutating llm.provider/%s out of band: %v", driftProviderName, err)
					}
				},
				Config:             providerConfig(endpoint) + driftProviderConfig("gpt-4o"),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

// TestAccFileModeSingletonRejected pins spec §14.3's file-mode case: the
// gateway cannot read the singleton kinds back in file mode, so the provider
// refuses them with a diagnostic naming the required storage mode.
func TestAccFileModeSingletonRejected(t *testing.T) {
	acctest.PreCheck(t)
	endpoint := acctest.EndpointFor(t, acctest.RoleFileMode)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: releaseGateProvider(),
		Steps: []resource.TestStep{
			{
				Config: providerConfig(endpoint) + `
resource "agentgateway_llm_settings" "singleton" {
  gateways = ["default"]
}
`,
				ExpectError: regexp.MustCompile(`requires config\.storage\.mode = hybrid`),
			},
		},
	})
}

const driftProviderName = "acctest-drift"

// driftProviderConfig is a built-in provider with the given model.
func driftProviderConfig(model string) string {
	return `
resource "agentgateway_llm_provider" "drift" {
  name          = "` + driftProviderName + `"
  provider_name = "openAI"
  model         = "` + model + `"
}
`
}

const twoProvidersConfig = `
resource "agentgateway_llm_provider" "a" {
  name          = "acctest-c21-a"
  provider_name = "openAI"
}

resource "agentgateway_llm_provider" "b" {
  name          = "acctest-c21-b"
  provider_name = "openAI"
}
`

const thirdProviderConfig = `
resource "agentgateway_llm_provider" "c" {
  name          = "acctest-c21-c"
  provider_name = "openAI"
}
`

// checkProvidersExist asserts every id is stored on the gateway.
func checkProvidersExist(c client.Client, ids []string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		list, err := c.List(context.Background(), configresource.KindLLMProvider)
		if err != nil {
			return fmt.Errorf("listing llm.provider: %w", err)
		}
		found := make(map[string]bool, len(list))
		for _, res := range list {
			found[res.ID] = true
		}
		for _, id := range ids {
			if !found[id] {
				return fmt.Errorf("llm.provider/%s is missing after the collection PUT; the server replaced the kind instead of upserting it", id)
			}
		}
		return nil
	}
}

// checkProvidersGone asserts the test cleaned up after itself.
func checkProvidersGone(c client.Client, ids []string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		list, err := c.List(context.Background(), configresource.KindLLMProvider)
		if err != nil {
			return fmt.Errorf("listing llm.provider: %w", err)
		}
		remaining := make(map[string]bool, len(list))
		for _, res := range list {
			remaining[res.ID] = true
		}
		for _, id := range ids {
			if remaining[id] {
				return fmt.Errorf("llm.provider/%s still exists after destroy", id)
			}
		}
		return nil
	}
}

// isNotFound reports whether err is the client's not-found sentinel.
func isNotFound(err error) bool {
	return errors.Is(err, client.ErrNotFound)
}
