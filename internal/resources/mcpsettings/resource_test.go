package mcpsettings

import (
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
)

// TestRewriteFileOwned proves a 409 that names a file-owned field becomes a
// diagnostic that says the config file owns it, and keeps the gateway message.
func TestRewriteFileOwned(t *testing.T) {
	diags := diag.Diagnostics{}
	diags.AddError(
		"create agentgateway_mcp_settings failed",
		"create mcp.settings/default failed. conflict detected; see the gateway message below. "+
			"Gateway message: file-owned mcp.settings/default field cannot be updated in hybrid mode: statefulMode",
	)

	got := rewriteFileOwned(diags)
	if len(got) != 1 {
		t.Fatalf("rewriteFileOwned() returned %d diagnostics, want 1", len(got))
	}
	detail := got[0].Detail()
	for _, want := range []string{
		"config file owns the statefulMode field",
		"config.storage.mode = hybrid",
		"Gateway message: file-owned mcp.settings/default field cannot be updated in hybrid mode: statefulMode",
	} {
		if !strings.Contains(detail, want) {
			t.Errorf("detail = %q, want it to contain %q", detail, want)
		}
	}
	if got[0].Summary() != "create agentgateway_mcp_settings failed" {
		t.Errorf("summary = %q, want the original summary", got[0].Summary())
	}
}

// TestRewriteFileOwnedLeavesOtherDiagnostics proves unrelated diagnostics pass
// through untouched.
func TestRewriteFileOwnedLeavesOtherDiagnostics(t *testing.T) {
	diags := diag.Diagnostics{}
	diags.AddError("create agentgateway_mcp_settings failed", "create mcp.settings/default failed. resource not found")

	got := rewriteFileOwned(diags)
	if len(got) != 1 {
		t.Fatalf("rewriteFileOwned() returned %d diagnostics, want 1", len(got))
	}
	if detail := got[0].Detail(); detail != "create mcp.settings/default failed. resource not found" {
		t.Errorf("detail = %q, want it unchanged", detail)
	}
}
