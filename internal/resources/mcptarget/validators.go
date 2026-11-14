package mcptarget

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// networkTransport enforces the host/backend/port rules of a network transport
// at plan time, so an invalid combination fails during `terraform plan`
// instead of at apply time.
//
// All three rules live in this object validator on purpose: the framework stops
// validating a nested object's children once the object itself reports an
// error, so a child-level ConflictsWith/AlsoRequires pair would be silently
// skipped for exactly the configurations that need it most. The validator skips
// a null block, so a target that uses another transport is unaffected.
type networkTransport struct {
	label string
}

var _ validator.Object = networkTransport{}

func (v networkTransport) Description(_ context.Context) string {
	return fmt.Sprintf("%s takes either host (with an optional port and path) or backend (with an optional path), never both", v.label)
}

func (v networkTransport) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v networkTransport) ValidateObject(_ context.Context, req validator.ObjectRequest, resp *validator.ObjectResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	attributes := req.ConfigValue.Attributes()
	host := isSet(attributes["host"])
	backend := isSet(attributes["backend"])
	port := isSet(attributes["port"])

	var detail string
	switch {
	case host && backend:
		detail = "host and backend are mutually exclusive; set either host (with an optional port and path) or backend (with an optional path)"
	case !host && port:
		detail = "port requires host; the backend form takes only backend and an optional path"
	case !host && !backend:
		detail = "host or backend is required; set host (with an optional port and path) or backend (with an optional path)"
	}
	if detail == "" {
		return
	}

	resp.Diagnostics.AddAttributeError(
		req.Path,
		fmt.Sprintf("Invalid agentgateway_mcp_target %s transport", v.label),
		detail,
	)
}

// noPoliciesOnStdio rejects transport policies on a stdio target at plan time.
// The gateway only applies `policies` to mcp, sse and openapi targets, so
// failing here saves a round trip and names the attribute.
type noPoliciesOnStdio struct{}

var _ validator.String = noPoliciesOnStdio{}

func (v noPoliciesOnStdio) Description(_ context.Context) string {
	return "policies_json is not supported on stdio targets"
}

func (v noPoliciesOnStdio) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v noPoliciesOnStdio) ValidateString(ctx context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	var stdio types.Object
	diags := req.Config.GetAttribute(ctx, path.Root("stdio"), &stdio)
	resp.Diagnostics.Append(diags...)
	if diags.HasError() {
		return
	}
	if stdio.IsNull() || stdio.IsUnknown() {
		return
	}

	resp.Diagnostics.AddAttributeError(
		req.Path,
		"Invalid agentgateway_mcp_target policies_json",
		"policies_json is not supported on a stdio target: the gateway applies transport policies only to mcp, sse and openapi targets. "+
			"Remove policies_json, or move the policies to the route or backend that serves this target.",
	)
}

// isSet reports whether an object attribute carries a known, non-null value.
func isSet(value attr.Value) bool {
	return value != nil && !value.IsNull() && !value.IsUnknown()
}
