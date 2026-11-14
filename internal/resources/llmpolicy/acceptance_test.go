package llmpolicy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"testing"

	"github.com/caberdo/terraform-provider-agentgateway/internal/acctest"
	"github.com/caberdo/terraform-provider-agentgateway/internal/client"
	"github.com/caberdo/terraform-provider-agentgateway/internal/configresource"
	"github.com/caberdo/terraform-provider-agentgateway/internal/providerdata"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	providerschema "github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// The policy key is enum-constrained (spec §7.5), so an acceptance test cannot
// use a per-run suffix. Rerun safety comes from clearing the keys this package
// owns before a test starts: Create refuses an id that already exists outside
// state, so a policy left behind by an aborted run would poison every later run.
const (
	corsPolicyName   = "cors"
	apiKeyPolicyName = "apiKey"
	resourceAddress  = "agentgateway_llm_policy.test"
)

const apiKeyBody = `{"mode":"strict"}`

var (
	corsConfig = fmt.Sprintf(`
resource "agentgateway_llm_policy" "test" {
  name = %q
  spec = jsonencode({
    allowOrigins = ["https://example.com"]
  })
}`, corsPolicyName)

	corsUpdatedConfig = fmt.Sprintf(`
resource "agentgateway_llm_policy" "test" {
  name = %q
  spec = jsonencode({
    allowOrigins = ["https://example.com", "https://app.example.com"]
    allowMethods = ["GET", "POST"]
  })
}`, corsPolicyName)

	apiKeyConfig = fmt.Sprintf(`
resource "agentgateway_llm_policy" "test" {
  name = %q
  spec = jsonencode({
    mode = "strict"
  })
}`, apiKeyPolicyName)
)

// TestAccLLMPolicyResource covers create, import, update and destroy of a
// policy against the hybrid gateway. It compiles everywhere and only runs when
// TF_ACC is set and the acceptance environment is reachable.
func TestAccLLMPolicyResource(t *testing.T) {
	acctest.PreCheck(t)
	endpoint := acctest.EndpointFor(t, acctest.RoleHybrid)
	clearPolicy(t, endpoint, corsPolicyName)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactories(endpoint),
		CheckDestroy:             checkPolicyDestroyed(endpoint, corsPolicyName),
		Steps: []resource.TestStep{
			{
				Config: corsConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceAddress, "id", corsPolicyName),
					resource.TestCheckResourceAttrSet(resourceAddress, "revision"),
					checkSpec(`{"allowOrigins":["https://example.com"]}`),
				),
			},
			{
				// Policy bodies are stored verbatim, so the import must match the
				// managed state exactly; there is nothing to ignore.
				ResourceName:      resourceAddress,
				ImportState:       true,
				ImportStateId:     corsPolicyName,
				ImportStateVerify: true,
			},
			{
				Config: corsUpdatedConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceAddress, "id", corsPolicyName),
					checkSpec(`{"allowOrigins":["https://example.com","https://app.example.com"],"allowMethods":["GET","POST"]}`),
				),
			},
		},
	})
}

// TestAccLLMPolicyAPIKeyRoundTrip pins C23: the gateway materialises keys but
// must not echo them into the stored policy body. If a gateway does echo
// `keys`, the import verification fails here, and the resource would otherwise
// both plan a diff and reject its own read-back (C11).
func TestAccLLMPolicyAPIKeyRoundTrip(t *testing.T) {
	acctest.PreCheck(t)
	endpoint := acctest.EndpointFor(t, acctest.RoleHybrid)
	clearPolicy(t, endpoint, apiKeyPolicyName)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactories(endpoint),
		CheckDestroy:             checkPolicyDestroyed(endpoint, apiKeyPolicyName),
		Steps: []resource.TestStep{
			{
				Config: apiKeyConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceAddress, "id", apiKeyPolicyName),
					checkSpec(apiKeyBody),
				),
			},
			{
				ResourceName:      resourceAddress,
				ImportState:       true,
				ImportStateId:     apiKeyPolicyName,
				ImportStateVerify: true,
			},
			{
				// The imported state must plan no change: the body round-trips
				// without an injected keys list.
				Config: apiKeyConfig,
				Check:  checkSpec(apiKeyBody),
			},
		},
	})
}

// checkSpec asserts the stored policy body is semantically equal to want, which
// also pins the absence of any key the gateway might have injected.
func checkSpec(want string) resource.TestCheckFunc {
	return resource.TestCheckResourceAttrWith(resourceAddress, "spec", func(got string) error {
		var gotValue, wantValue any
		if err := json.Unmarshal([]byte(got), &gotValue); err != nil {
			return fmt.Errorf("spec is not valid JSON: %s", got)
		}
		if err := json.Unmarshal([]byte(want), &wantValue); err != nil {
			return fmt.Errorf("expected spec is not valid JSON: %s", want)
		}
		if !reflect.DeepEqual(gotValue, wantValue) {
			return fmt.Errorf("spec = %s, want %s", got, want)
		}
		return nil
	})
}

// testFactories builds a provider that registers only this resource. The
// resource package must not import internal/provider (design §4), so the
// acceptance test supplies the minimal provider it needs.
func testFactories(endpoint string) map[string]func() (tfprotov6.ProviderServer, error) {
	p := &testProvider{endpoint: endpoint}
	return map[string]func() (tfprotov6.ProviderServer, error){
		"agentgateway": providerserver.NewProtocol6WithError(p),
	}
}

// newAcceptanceClient returns a client for the gateway under test.
func newAcceptanceClient(t *testing.T, endpoint string) client.Client {
	t.Helper()

	c, err := client.New(client.Config{
		BaseURL:    endpoint,
		HTTPClient: http.DefaultClient,
		UserAgent:  "terraform-provider-agentgateway/acctest",
	})
	if err != nil {
		t.Fatalf("creating acceptance client: %v", err)
	}
	return c
}

// clearPolicy removes a policy an aborted run left behind, so a rerun can
// create the key again. The key itself is fixed because policy names are
// enum-constrained, so cleanup is the rerun-safety mechanism. A cleanup failure
// must never be silent.
func clearPolicy(t *testing.T, endpoint, name string) {
	t.Helper()

	c := newAcceptanceClient(t, endpoint)
	ctx := context.Background()

	list, err := c.List(ctx, configresource.KindLLMPolicy)
	if err != nil {
		t.Fatalf("listing llm policies before the run: %v", err)
	}

	found := false
	for _, res := range list {
		if res.ID == name {
			found = true
			break
		}
	}
	if !found {
		return
	}

	if err := c.Delete(ctx, configresource.KindLLMPolicy, name); err != nil && !errors.Is(err, client.ErrNotFound) {
		t.Fatalf("clearing leftover llm policy %q: %v", name, err)
	}
}

// checkPolicyDestroyed fails when one of the policies survives destroy.
func checkPolicyDestroyed(endpoint string, names ...string) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		c, err := client.New(client.Config{
			BaseURL:    endpoint,
			HTTPClient: http.DefaultClient,
			UserAgent:  "terraform-provider-agentgateway/acctest",
		})
		if err != nil {
			return fmt.Errorf("creating acceptance client: %w", err)
		}

		list, err := c.List(context.Background(), configresource.KindLLMPolicy)
		if err != nil {
			return fmt.Errorf("listing llm policies: %w", err)
		}
		for _, res := range list {
			for _, name := range names {
				if res.ID == name {
					return fmt.Errorf("llm policy %q still exists after destroy", res.ID)
				}
			}
		}
		return nil
	}
}

// testProvider is the smallest provider that can host this resource in an
// acceptance test.
type testProvider struct {
	endpoint string
}

func (p *testProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "agentgateway"
	resp.Version = "acctest"
}

func (p *testProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = providerschema.Schema{
		MarkdownDescription: "Minimal provider used by this resource's acceptance test.",
	}
}

func (p *testProvider) Configure(_ context.Context, _ provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	c, err := client.New(client.Config{
		BaseURL:    p.endpoint,
		HTTPClient: http.DefaultClient,
		UserAgent:  "terraform-provider-agentgateway/acctest",
	})
	if err != nil {
		resp.Diagnostics.AddError("Could not create the acceptance client", err.Error())
		return
	}

	data := &providerdata.Data{Client: c, StoreMode: "hybrid"}
	resp.ResourceData = data
	resp.DataSourceData = data
}

func (p *testProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return nil
}

func (p *testProvider) Resources(_ context.Context) []func() frameworkresource.Resource {
	return []func() frameworkresource.Resource{New}
}
