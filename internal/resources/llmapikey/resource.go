package llmapikey

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/caberdo/terraform-provider-agentgateway/internal/configresource"
	"github.com/caberdo/terraform-provider-agentgateway/internal/providerdata"
	"github.com/caberdo/terraform-provider-agentgateway/internal/resourceutil"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

const (
	// apiKeyPolicyPrerequisite is the gateway's 409 message when the merged
	// configuration has no llm.policies.apiKey policy (spec §7.4, C10).
	apiKeyPolicyPrerequisite = "DB-backed API keys require llm.policies.apiKey"

	// conflictDiagnosticMarker is the shared harness's wording for a 409. It is
	// only used to decide whether a probe of the merged configuration is worth
	// it; the replacement diagnostic never repeats it.
	conflictDiagnosticMarker = "conflict detected"
)

// New returns the agentgateway_llm_api_key resource.
func New() resource.Resource {
	return &apiKeyResource{
		Resource: resourceutil.CRUD(resourceutil.Spec{
			TypeName: "agentgateway_llm_api_key",
			Kind:     configresource.KindLLMAPIKey,
			Schema:   newSchema(),
			New:      func() resourceutil.KindResource { return newModel() },
		}),
	}
}

// apiKeyResource decorates the shared CRUD harness with the one diagnostic that
// is specific to this kind: a key cannot exist without the apiKey policy.
//
// The harness owns create/read/update/delete and error mapping. Only the
// diagnostics of a write are inspected afterwards, so no shared code changes
// and every other failure keeps the harness wording.
//
// Configure and ImportState are forwarded explicitly: the framework discovers
// them by type assertion on the resource it is given, and an embedded
// resource.Resource interface alone would hide both.
type apiKeyResource struct {
	resource.Resource
	data *providerdata.Data
}

// Create writes the key and rewrites the missing-policy conflict.
func (r *apiKeyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	r.Resource.Create(ctx, req, resp)
	r.addPolicyPrerequisite(ctx, &resp.Diagnostics)
}

// Update writes the key and rewrites the missing-policy conflict.
func (r *apiKeyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.Resource.Update(ctx, req, resp)
	r.addPolicyPrerequisite(ctx, &resp.Diagnostics)
}

// Configure passes the provider data to the shared harness and keeps a copy for
// the prerequisite probe.
func (r *apiKeyResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	configured, ok := r.Resource.(resource.ResourceWithConfigure)
	if !ok {
		resp.Diagnostics.AddError(
			"agentgateway_llm_api_key is not configurable",
			"the shared CRUD harness does not implement Configure",
		)
		return
	}
	configured.Configure(ctx, req, resp)

	if req.ProviderData == nil {
		return
	}
	// The harness has already reported a provider-data type mismatch, so an
	// unexpected type is not reported twice here.
	if data, ok := req.ProviderData.(*providerdata.Data); ok {
		r.data = data
	}
}

// ImportState forwards to the shared harness, which parses the import id.
func (r *apiKeyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importable, ok := r.Resource.(resource.ResourceWithImportState)
	if !ok {
		resp.Diagnostics.AddError(
			"agentgateway_llm_api_key cannot be imported",
			"the shared CRUD harness does not implement ImportState",
		)
		return
	}
	importable.ImportState(ctx, req, resp)
}

// addPolicyPrerequisite replaces a conflict diagnostic that reports the missing
// apiKey policy with one that names the fix.
//
// Two signals are used. A diagnostic that quotes the gateway message is
// authoritative and is replaced directly. A create conflict carries no message
// because the server generated the id, so the merged configuration is probed:
// only when it definitively has no llm.policies.apiKey is the diagnostic
// replaced. A probe that fails leaves the harness wording untouched.
func (r *apiKeyResource) addPolicyPrerequisite(ctx context.Context, diags *diag.Diagnostics) {
	conflict := -1
	for i, diagnostic := range *diags {
		if diagnostic.Severity() != diag.SeverityError {
			continue
		}
		if strings.Contains(diagnostic.Detail(), apiKeyPolicyPrerequisite) {
			(*diags)[i] = policyPrerequisiteDiagnostic()
			return
		}
		if conflict < 0 && strings.Contains(diagnostic.Detail(), conflictDiagnosticMarker) {
			conflict = i
		}
	}

	if conflict < 0 || !r.apiKeyPolicyMissing(ctx) {
		return
	}
	(*diags)[conflict] = policyPrerequisiteDiagnostic()
}

// apiKeyPolicyMissing reports whether the merged configuration definitively has
// no llm.policies.apiKey policy. A file-owned policy satisfies the prerequisite
// just like a Terraform-managed one, so the merged configuration is the only
// reliable source (spec C10, C23).
func (r *apiKeyResource) apiKeyPolicyMissing(ctx context.Context) bool {
	if r.data == nil || r.data.Client == nil {
		return false
	}

	effective, err := r.data.Client.EffectiveConfig(ctx)
	if err != nil {
		return false
	}

	var config struct {
		LLM struct {
			Policies map[string]json.RawMessage `json:"policies"`
		} `json:"llm"`
	}
	if err := json.Unmarshal(effective, &config); err != nil {
		return false
	}

	_, ok := config.LLM.Policies["apiKey"]
	return !ok
}

// policyPrerequisiteDiagnostic names the prerequisite and both ways to satisfy
// it. It quotes no server message, so no request value can reach it.
func policyPrerequisiteDiagnostic() diag.Diagnostic {
	return diag.NewErrorDiagnostic(
		"agentgateway_llm_api_key requires the apiKey policy",
		"The gateway rejected this API key because llm.policies.apiKey is not configured. "+
			"Create an agentgateway_llm_policy resource with name = \"apiKey\", or add llm.policies.apiKey to the gateway configuration file, then re-apply.",
	)
}
