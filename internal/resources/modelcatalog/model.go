package modelcatalog

import (
	"context"
	"encoding/json"

	"github.com/caberdo/terraform-provider-agentgateway/internal/configresource"
	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// singletonID is the fixed id of the model catalog overlay (spec §7.0).
const singletonID = "default"

// catalogOptions are the two catalog leaves: a base catalog (with
// metadata.generatedAt) and a custom overlay. Either or both may be set
// (spec §7.15): the gateway value carries `base` and `custom` together, and the
// overlay is applied on top of the base catalog.
var catalogOptions = []path.Expression{
	path.MatchRoot("base_json"),
	path.MatchRoot("custom_json"),
}

// newSchema returns the agentgateway_model_catalog schema.
func newSchema() schema.Schema {
	return schema.Schema{
		MarkdownDescription: "The database-stored model catalog overlay. A base catalog replaces the built-in one; a custom catalog is applied on top of it.",
		Attributes: map[string]schema.Attribute{
			"base_json": schema.StringAttribute{
				Optional:            true,
				CustomType:          jsontypes.NormalizedType{},
				MarkdownDescription: "Base catalog as JSON, including `metadata.generatedAt`. At least one of `base_json` or `custom_json` must be set; both may be set together.",
				Validators: []validator.String{
					stringvalidator.AtLeastOneOf(catalogOptions...),
				},
			},
			"custom_json": schema.StringAttribute{
				Optional:            true,
				CustomType:          jsontypes.NormalizedType{},
				MarkdownDescription: "Custom catalog overlay as JSON, without `metadata`. At least one of `base_json` or `custom_json` must be set; both may be set together.",
				Validators: []validator.String{
					stringvalidator.AtLeastOneOf(catalogOptions...),
				},
			},
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Always `default`: the model catalog is a singleton.",
			},
			"revision": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Server revision of the overlay.",
			},
		},
	}
}

// modelCatalogModel is the Terraform state shape of agentgateway_model_catalog.
type modelCatalogModel struct {
	BaseJSON   jsontypes.Normalized `tfsdk:"base_json"`
	CustomJSON jsontypes.Normalized `tfsdk:"custom_json"`
	IDValue    types.String         `tfsdk:"id"`
	Revision   types.Int64          `tfsdk:"revision"`
}

// newModel returns a model with typed null JSON leaves. ImportState writes a
// fresh model straight into state, and an unset jsontypes value must still
// carry the Normalized type.
func newModel() *modelCatalogModel {
	return &modelCatalogModel{
		BaseJSON:   jsontypes.NewNormalizedNull(),
		CustomJSON: jsontypes.NewNormalizedNull(),
	}
}

// Kind returns the config resource kind.
func (m *modelCatalogModel) Kind() configresource.Kind { return configresource.KindModelCatalog }

// ID returns the singleton id.
func (m *modelCatalogModel) ID() string { return singletonID }

// catalogPayload is the value object sent to the config resource API. The two
// leaves are inserted verbatim and omitted when null (spec §7.15).
type catalogPayload struct {
	Base   json.RawMessage `json:"base,omitempty"`
	Custom json.RawMessage `json:"custom,omitempty"`
}

// Value builds the API value object from the model. At least one leaf is
// required, and both may be set: a base catalog replaces the built-in one and a
// custom overlay is applied on top of it (spec §7.15).
func (m *modelCatalogModel) Value(_ context.Context, diags *diag.Diagnostics) json.RawMessage {
	payload := catalogPayload{}

	if presentJSON(m.BaseJSON) {
		payload.Base = json.RawMessage(m.BaseJSON.ValueString())
	}
	if presentJSON(m.CustomJSON) {
		payload.Custom = json.RawMessage(m.CustomJSON.ValueString())
	}

	if payload.Base == nil && payload.Custom == nil {
		diags.AddError(
			"Invalid agentgateway_model_catalog",
			"at least one of base_json or custom_json must be set",
		)
		return nil
	}

	value, err := json.Marshal(payload)
	if err != nil {
		diags.AddError(
			"Invalid agentgateway_model_catalog",
			"the catalog value could not be encoded; check the JSON attributes",
		)
		return nil
	}
	return value
}

// catalogValue is the wire shape of a modelCatalog value.
type catalogValue struct {
	Base   json.RawMessage `json:"base"`
	Custom json.RawMessage `json:"custom"`
}

// SetValue stores the value returned by the gateway. A leaf the server did not
// return keeps the value already in state; a null leaf on a computed plan value
// becomes null so it can never stay unknown after apply.
func (m *modelCatalogModel) SetValue(_ context.Context, value json.RawMessage, diags *diag.Diagnostics) {
	if len(value) == 0 {
		return
	}

	var decoded catalogValue
	if err := json.Unmarshal(value, &decoded); err != nil {
		diags.AddError(
			"Failed to read agentgateway_model_catalog",
			"the gateway returned a value that is not a JSON object",
		)
		return
	}

	m.BaseJSON = preservedJSON(m.BaseJSON, decoded.Base)
	m.CustomJSON = preservedJSON(m.CustomJSON, decoded.Custom)
}

// SetServerMeta stores the computed id and revision.
func (m *modelCatalogModel) SetServerMeta(id string, revision int64) {
	m.IDValue = types.StringValue(id)
	m.Revision = types.Int64Value(revision)
}

// presentJSON reports whether a JSON leaf should be sent.
func presentJSON(value jsontypes.Normalized) bool {
	return !value.IsNull() && !value.IsUnknown() && value.ValueString() != ""
}

// isPresent reports whether a raw JSON field carries a value other than null.
func isPresent(raw json.RawMessage) bool {
	return len(raw) > 0 && string(raw) != "null"
}

// preservedJSON returns the server value when it returned one and keeps the
// current value otherwise. An unknown current value becomes null so state never
// carries an unknown after apply.
func preservedJSON(current jsontypes.Normalized, raw json.RawMessage) jsontypes.Normalized {
	if !isPresent(raw) {
		if current.IsUnknown() {
			return jsontypes.NewNormalizedNull()
		}
		return current
	}
	return jsontypes.NewNormalizedValue(string(raw))
}
