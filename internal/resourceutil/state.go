package resourceutil

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// PreserveIfNull returns src when dst is null, otherwise dst. It is used by
// model SetValue implementations to keep attributes the server did not return.
func PreserveIfNull(_ context.Context, src, dst attr.Value) attr.Value {
	if dst.IsNull() {
		return src
	}
	return dst
}

// stateID reads the stored id attribute from Terraform state.
func stateID(ctx context.Context, state tfsdkState) (string, diag.Diagnostics) {
	var id types.String
	diags := state.GetAttribute(ctx, path.Root("id"), &id)
	if diags.HasError() {
		return "", diags
	}
	return id.ValueString(), diags
}

// tfsdkState is the subset of tfsdk.State used by stateID.
type tfsdkState interface {
	GetAttribute(ctx context.Context, path path.Path, target interface{}) diag.Diagnostics
}
