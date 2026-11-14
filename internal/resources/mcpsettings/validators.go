package mcpsettings

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

// goDuration validates that `sse_keep_alive` parses as a Go duration, which is
// the format the gateway's `sseKeepAlive` field uses (for example `30s` or
// `1m30s`). Without this check a typo such as `30` reaches the gateway and
// comes back as an opaque 400.
type goDuration struct{}

var _ validator.String = goDuration{}

func (v goDuration) Description(_ context.Context) string {
	return `value must be a Go duration such as "30s" or "1m30s"`
}

func (v goDuration) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v goDuration) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	value := req.ConfigValue.ValueString()
	if _, err := time.ParseDuration(value); err != nil {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid agentgateway_mcp_settings sse_keep_alive",
			fmt.Sprintf("%q is not a Go duration; use a value such as \"30s\" or \"1m30s\", or omit sse_keep_alive to disable keep-alives.", value),
		)
	}
}
