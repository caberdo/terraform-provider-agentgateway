package llmmodel

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
)

// TestLLMModelSchema locks the schema contract from spec §7.2: computed id and
// revision, a sensitive api_key, one of two visibilities, and every JSON leaf
// optional and normalized.
func TestLLMModelSchema(t *testing.T) {
	t.Parallel()

	if diags := newSchema().ValidateImplementation(context.Background()); diags.HasError() {
		t.Fatalf("schema failed framework validation: %v", diags)
	}

	attrs := newSchema().Attributes

	required := []string{"name"}
	for _, name := range required {
		attribute, ok := attrs[name]
		if !ok {
			t.Fatalf("schema is missing required attribute %q", name)
		}
		if !attribute.IsRequired() {
			t.Errorf("%s is not required", name)
		}
	}

	id, ok := attrs["id"].(schema.StringAttribute)
	if !ok {
		t.Fatal("id is not a string attribute")
	}
	if !id.Optional || !id.Computed {
		t.Error("id must be Optional and Computed")
	}

	revision, ok := attrs["revision"].(schema.Int64Attribute)
	if !ok {
		t.Fatal("revision is not an int64 attribute")
	}
	if !revision.Computed || revision.Optional || revision.Required {
		t.Error("revision must be Computed only")
	}

	apiKey, ok := attrs["api_key"].(schema.StringAttribute)
	if !ok {
		t.Fatal("api_key is not a string attribute")
	}
	if !apiKey.Sensitive {
		t.Error("api_key must be Sensitive")
	}

	visibility, ok := attrs["visibility"].(schema.StringAttribute)
	if !ok {
		t.Fatal("visibility is not a string attribute")
	}
	if !visibility.Optional || !visibility.Computed {
		t.Error("visibility must be Optional and Computed when it has a default")
	}
	if len(visibility.Validators) == 0 {
		t.Error("visibility must validate its enum")
	}
	if visibility.Default == nil {
		t.Error("visibility must default to public")
	}

	union := []string{"provider_name", "provider_reference", "custom_provider_json"}
	for _, name := range union {
		attribute, ok := attrs[name].(schema.StringAttribute)
		if !ok {
			t.Fatalf("provider option %q is not a string attribute", name)
		}
		if !attribute.Optional {
			t.Errorf("%s must be optional", name)
		}
		if len(attribute.Validators) == 0 {
			t.Errorf("%s must carry the ExactlyOneOf validator", name)
		}
	}

	jsonLeaves := []string{
		"custom_provider_json",
		"passthrough_json",
		"authorization_json",
		"matches_json",
		"defaults_json",
		"overrides_json",
		"transformation_json",
		"final_transformation_json",
		"request_transformation_json",
		"request_headers_json",
		"response_headers_json",
		"backend_tls_json",
		"auth_json",
		"health_json",
		"backend_tunnel_json",
		"guardrails_json",
		"prompt_caching_json",
	}
	for _, name := range jsonLeaves {
		attribute, ok := attrs[name].(schema.StringAttribute)
		if !ok {
			t.Fatalf("%s is not a string attribute", name)
		}
		if !attribute.Optional || attribute.Computed || attribute.Required {
			t.Errorf("%s must be Optional only", name)
		}
		if attribute.CustomType == nil || !attribute.CustomType.Equal(jsontypes.NormalizedType{}) {
			t.Errorf("%s must use jsontypes.Normalized", name)
		}
		if !attribute.Sensitive {
			t.Errorf("%s must be Sensitive because it can carry credentials", name)
		}
	}
}
