package resourceutil_test

import (
	"context"
	"testing"

	"github.com/caberdo/terraform-provider-agentgateway/internal/configresource"
	"github.com/caberdo/terraform-provider-agentgateway/internal/resourceutil"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestParseImportID(t *testing.T) {
	cases := []struct {
		name    string
		kind    configresource.Kind
		raw     string
		want    string
		wantErr bool
	}{
		{"bare", configresource.KindLLMProvider, "my-provider", "my-provider", false},
		{"qualified", configresource.KindLLMProvider, "llm.provider/my-provider", "my-provider", false},
		{"wrong kind", configresource.KindLLMProvider, "llm.model/my-provider", "", true},
		{"empty", configresource.KindLLMProvider, "", "", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resourceutil.ParseImportID(tc.kind, tc.raw)
			if (err != nil) != tc.wantErr {
				t.Fatalf("ParseImportID(%q) error = %v, wantErr %v", tc.raw, err, tc.wantErr)
			}
			if got != tc.want {
				t.Errorf("ParseImportID(%q) = %q, want %q", tc.raw, got, tc.want)
			}
			if err != nil && tc.name == "wrong kind" {
				if err.Error() != "expected import id for kind llm.provider" {
					t.Errorf("unexpected error message: %q", err.Error())
				}
			}
		})
	}
}

func TestPreserveIfNull(t *testing.T) {
	src := types.StringValue("preserved")
	dst := types.StringNull()

	got := resourceutil.PreserveIfNull(context.Background(), src, dst)
	if !got.Equal(src) {
		t.Errorf("expected src preserved, got %v", got)
	}

	dst = types.StringValue("new")
	got = resourceutil.PreserveIfNull(context.Background(), src, dst)
	if !got.Equal(dst) {
		t.Errorf("expected dst kept, got %v", got)
	}

	// Non-string values are also handled via the attr.Value interface.
	srcMap, dstNull := types.MapValueMust(types.StringType, map[string]attr.Value{}), types.MapNull(types.StringType)
	got = resourceutil.PreserveIfNull(context.Background(), srcMap, dstNull)
	if !got.Equal(srcMap) {
		t.Errorf("expected null map replaced by src, got %v", got)
	}
}
