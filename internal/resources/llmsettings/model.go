package llmsettings

import (
	"context"
	"encoding/json"

	"github.com/caberdo/terraform-provider-agentgateway/internal/configresource"
	"github.com/caberdo/terraform-provider-agentgateway/internal/resourceutil"
	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// newSchema returns the agentgateway_llm_settings schema.
func newSchema() schema.Schema {
	return schema.Schema{
		MarkdownDescription: "The singleton `llm` section overlay: which gateways LLM traffic is attached to, or the deprecated port and TLS listener.",
		Attributes: map[string]schema.Attribute{
			"gateways": schema.ListAttribute{
				Optional:    true,
				ElementType: types.StringType,
				// The empty-list exception is enforced in Value(): a schema
				// ConflictsWith cannot tell an empty list from a populated one.
				MarkdownDescription: "Gateways to attach LLM traffic to, as `<gateway>` or `<gateway>/<listener>`. Defaults to `default` server-side. A non-empty list conflicts with `port`; an empty list does not.",
			},
			"port": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Deprecated upstream. Port the LLM listener binds to. Conflicts with a non-empty `gateways` list; prefer `gateways`.",
				Validators: []validator.Int64{
					int64validator.Between(0, 65535),
				},
			},
			"tls_json": schema.StringAttribute{
				Optional:            true,
				CustomType:          jsontypes.NormalizedType{},
				MarkdownDescription: "Server TLS configuration used with the deprecated `port`, as JSON.",
			},
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Always `default`; the settings resource is a singleton.",
			},
			"revision": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Server revision of the settings overlay.",
			},
		},
	}
}

// llmSettingsModel is the Terraform state shape of agentgateway_llm_settings.
type llmSettingsModel struct {
	Gateways types.List           `tfsdk:"gateways"`
	Port     types.Int64          `tfsdk:"port"`
	TLSJSON  jsontypes.Normalized `tfsdk:"tls_json"`
	IDValue  types.String         `tfsdk:"id"`
	Revision types.Int64          `tfsdk:"revision"`
}

// Kind returns the config resource kind.
func (m *llmSettingsModel) Kind() configresource.Kind { return configresource.KindLLMSettings }

// ID returns the fixed singleton id.
func (m *llmSettingsModel) ID() string { return "default" }

// Value builds the API value object from the model.
func (m *llmSettingsModel) Value(ctx context.Context, diags *diag.Diagnostics) json.RawMessage {
	hasGateways := !m.Gateways.IsNull() && !m.Gateways.IsUnknown()
	hasPort := !m.Port.IsNull() && !m.Port.IsUnknown()
	// A populated gateways list and port are alternatives. An empty list is not
	// a conflicting value: the gateway UI writes `gateways: []` next to `port`
	// when no gateway is bound, and the gateway accepts that. The check lives
	// here rather than in the schema because a schema ConflictsWith cannot tell
	// an empty list from a populated one.
	if hasGateways && len(m.Gateways.Elements()) > 0 && hasPort {
		diags.AddAttributeError(
			path.Root("gateways"),
			"Invalid agentgateway_llm_settings",
			"gateways and port conflict; set gateways, or port for the deprecated listener, but not both. "+
				"An empty gateways list is accepted with port.",
		)
		return nil
	}

	value := map[string]any{}

	if hasGateways {
		var gateways []string
		if listDiags := m.Gateways.ElementsAs(ctx, &gateways, false); listDiags.HasError() {
			diags.Append(listDiags...)
			return nil
		}
		value["gateways"] = gateways
	}

	if hasPort {
		value["port"] = m.Port.ValueInt64()
	}

	if presentJSON(m.TLSJSON) {
		value["tls"] = json.RawMessage(m.TLSJSON.ValueString())
	}

	raw, err := json.Marshal(value)
	if err != nil {
		diags.AddError("Invalid agentgateway_llm_settings", "the settings could not be encoded as JSON")
		return nil
	}
	return raw
}

// SetValue stores the settings the gateway returned. The gateway drops fields
// the config file owns (spec C9), so every attribute is preserved from state
// when the response omits it.
func (m *llmSettingsModel) SetValue(ctx context.Context, value json.RawMessage, diags *diag.Diagnostics) {
	if len(value) == 0 {
		return
	}

	var body struct {
		Gateways json.RawMessage `json:"gateways"`
		Port     json.RawMessage `json:"port"`
		TLS      json.RawMessage `json:"tls"`
	}
	if err := json.Unmarshal(value, &body); err != nil {
		diags.AddError("Failed to read agentgateway_llm_settings", "the gateway returned a value that is not a JSON object")
		return
	}

	gateways := types.ListNull(types.StringType)
	if isPresent(body.Gateways) {
		var names []string
		if err := json.Unmarshal(body.Gateways, &names); err != nil {
			diags.AddError("Failed to read agentgateway_llm_settings", "the gateway returned gateways that are not a list of strings")
			return
		}
		list, listDiags := types.ListValueFrom(ctx, types.StringType, names)
		diags.Append(listDiags...)
		if diags.HasError() {
			return
		}
		gateways = list
	}
	m.Gateways = resourceutil.PreserveIfNull(ctx, m.Gateways, gateways).(types.List)

	port := types.Int64Null()
	if isPresent(body.Port) {
		var number int64
		if err := json.Unmarshal(body.Port, &number); err != nil {
			diags.AddError("Failed to read agentgateway_llm_settings", "the gateway returned a port that is not an integer")
			return
		}
		port = types.Int64Value(number)
	}
	m.Port = resourceutil.PreserveIfNull(ctx, m.Port, port).(types.Int64)

	tls := jsontypes.NewNormalizedNull()
	if isPresent(body.TLS) {
		tls = jsontypes.NewNormalizedValue(string(body.TLS))
	}
	m.TLSJSON = resourceutil.PreserveIfNull(ctx, m.TLSJSON, tls).(jsontypes.Normalized)
}

// SetServerMeta stores the computed id and revision.
func (m *llmSettingsModel) SetServerMeta(id string, revision int64) {
	m.IDValue = types.StringValue(id)
	m.Revision = types.Int64Value(revision)
}

// isPresent reports whether raw carries a JSON value other than null.
func isPresent(raw json.RawMessage) bool {
	return len(raw) > 0 && string(raw) != "null"
}

// presentJSON reports whether a normalized JSON value should be sent.
func presentJSON(value jsontypes.Normalized) bool {
	return !value.IsNull() && !value.IsUnknown() && value.ValueString() != ""
}
