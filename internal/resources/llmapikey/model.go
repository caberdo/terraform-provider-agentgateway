package llmapikey

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"

	"github.com/caberdo/terraform-provider-agentgateway/internal/configresource"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

// keyOptions are the two mutually exclusive ways to provide the key material
// (spec C12). The schema rejects zero or two; Value() repeats the check so a
// hand-built model can never produce an ambiguous payload.
var keyOptions = []path.Expression{
	path.MatchRoot("key"),
	path.MatchRoot("key_hash"),
}

// apiKeyHashPattern is the documented sha256 key-hash format (spec C12).
var apiKeyHashPattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)

// budget units and overage actions accepted by the gateway.
const (
	budgetUnitUSD     = "USD"
	budgetUnitTokens  = "Tokens"
	budgetActionAudit = "Audit"
	budgetActionBlock = "Block"
)

// Server-managed metadata keys that carry the key's timestamps. They mirror the
// keys configresource.StripManagedMetadata removes: the config API exposes the
// times only inside the value, so the model reads them before stripping.
const (
	managedCreatedAtKey = "agentgateway.dev/createdAt"
	managedUpdatedAtKey = "agentgateway.dev/updatedAt"
)

// newSchema returns the agentgateway_llm_api_key schema (spec §7.4).
func newSchema() schema.Schema {
	return schema.Schema{
		MarkdownDescription: "A virtual LLM API key with optional metadata, allowed models and budgets.",
		Attributes: map[string]schema.Attribute{
			"key": schema.StringAttribute{
				Optional:            true,
				Sensitive:           true,
				MarkdownDescription: "Plaintext virtual key. Exactly one of `key` or `key_hash` must be set. Rotating the key is an in-place update; the original key stops authenticating.",
				Validators: []validator.String{
					stringvalidator.ExactlyOneOf(keyOptions...),
				},
			},
			"key_hash": schema.StringAttribute{
				Optional:            true,
				Sensitive:           true,
				MarkdownDescription: "SHA-256 hash of the virtual key, in `sha256:<hex>` format. Exactly one of `key` or `key_hash` must be set.",
				Validators: []validator.String{
					stringvalidator.RegexMatches(apiKeyHashPattern, "must be a sha256 key hash of the form sha256:<64 lowercase hex characters>"),
					stringvalidator.ExactlyOneOf(keyOptions...),
				},
			},
			"metadata": schema.MapAttribute{
				Optional:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Metadata attached to requests authenticated with this key. Replaced, not merged, on update. Keys that start with `agentgateway.dev/` are reserved by the gateway.",
				Validators: []validator.Map{
					metadataPrefixValidator{},
				},
			},
			"allowed_models": schema.ListAttribute{
				Optional:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Model name patterns this key may request. Omit the attribute for no constraint; set an empty list to deny every model. Omitted and `[]` are different values and are sent as such.",
			},
			"budgets": schema.ListNestedAttribute{
				Optional:            true,
				MarkdownDescription: "Budgets charged after each LLM response. A key may carry several; they are charged independently.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: "Stable budget name, unique within the key.",
						},
						"limit": schema.SingleNestedAttribute{
							Required:            true,
							MarkdownDescription: "Maximum usage allowed during the window.",
							Attributes: map[string]schema.Attribute{
								"unit": schema.StringAttribute{
									Required:            true,
									MarkdownDescription: "`USD` caps realized cost, `Tokens` caps token usage.",
									Validators: []validator.String{
										stringvalidator.OneOf(budgetUnitUSD, budgetUnitTokens),
									},
								},
								"amount": schema.Float64Attribute{
									Required:            true,
									MarkdownDescription: "Maximum usage. A `Tokens` amount must be a whole number.",
								},
							},
						},
						"window": schema.SingleNestedAttribute{
							Optional:            true,
							MarkdownDescription: "Rolling window over which usage accumulates. Omit it when the budget must never reset.",
							Attributes: map[string]schema.Attribute{
								"rolling": schema.StringAttribute{
									Required:            true,
									MarkdownDescription: "Window length, such as `1h`, `24h` or `30d`. Windows are aligned to the Unix epoch.",
								},
							},
						},
						"on_budget_exceeded": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: "`Block` rejects the next request with `429`; `Audit` records the overage and lets the request through.",
							Validators: []validator.String{
								stringvalidator.OneOf(budgetActionAudit, budgetActionBlock),
							},
						},
					},
				},
			},
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Server-generated UUID of the key.",
			},
			"created_at": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Creation time reported by the gateway.",
			},
			"updated_at": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Last update time reported by the gateway.",
			},
			"revision": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Server revision of the key.",
			},
		},
	}
}

// metadataPrefixValidator rejects metadata keys in the gateway's reserved
// namespace, because the server manages them itself (spec C12).
type metadataPrefixValidator struct{}

// Description describes the validation in plain text.
func (metadataPrefixValidator) Description(context.Context) string {
	return "must not use the agentgateway.dev/ reserved metadata prefix"
}

// MarkdownDescription describes the validation in Markdown.
func (v metadataPrefixValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

// ValidateMap rejects a metadata key in the reserved namespace.
func (metadataPrefixValidator) ValidateMap(ctx context.Context, req validator.MapRequest, resp *validator.MapResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	metadata := map[string]string{}
	if diags := req.ConfigValue.ElementsAs(ctx, &metadata, false); diags.HasError() {
		// A wrong element type is reported by the framework's own type check.
		return
	}

	if err := configresource.HasReservedMetadataKey(metadata); err != nil {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Reserved metadata key",
			fmt.Sprintf("%s; the gateway manages keys in that namespace itself", err.Error()),
		)
	}
}

// apiKeyModel is the Terraform state shape of agentgateway_llm_api_key.
//
// The framework rebuilds the struct from the schema on every plan and state
// read, so only exported fields with tfsdk tags survive.
type apiKeyModel struct {
	Key           types.String `tfsdk:"key"`
	KeyHash       types.String `tfsdk:"key_hash"`
	Metadata      types.Map    `tfsdk:"metadata"`
	AllowedModels types.List   `tfsdk:"allowed_models"`
	Budgets       types.List   `tfsdk:"budgets"`
	IDValue       types.String `tfsdk:"id"`
	CreatedAt     types.String `tfsdk:"created_at"`
	UpdatedAt     types.String `tfsdk:"updated_at"`
	Revision      types.Int64  `tfsdk:"revision"`
}

// budgetModel is one budget entry. Limit and Window are nested objects because
// the schema models them as nested attributes.
type budgetModel struct {
	Name             types.String `tfsdk:"name"`
	Limit            types.Object `tfsdk:"limit"`
	Window           types.Object `tfsdk:"window"`
	OnBudgetExceeded types.String `tfsdk:"on_budget_exceeded"`
}

// budgetLimitModel is the limit of a budget.
type budgetLimitModel struct {
	Unit   types.String  `tfsdk:"unit"`
	Amount types.Float64 `tfsdk:"amount"`
}

// budgetWindowModel is the optional rolling window of a budget.
type budgetWindowModel struct {
	Rolling types.String `tfsdk:"rolling"`
}

// newModel returns a model with every root collection seeded with its element
// type. ImportState writes a fresh model straight into state, and a zero-value
// types.List or types.Map carries no element type, which the framework rejects
// on the next Read.
func newModel() *apiKeyModel {
	return &apiKeyModel{
		Metadata:      types.MapNull(types.StringType),
		AllowedModels: types.ListNull(types.StringType),
		Budgets:       types.ListNull(budgetObjectType()),
	}
}

// Kind returns the config resource kind.
func (m *apiKeyModel) Kind() configresource.Kind { return configresource.KindLLMAPIKey }

// ID returns the id the server assigned. llm.apiKey is the only kind whose id
// is generated, so Create starts with an empty id (spec §7.0).
func (m *apiKeyModel) ID() string { return m.IDValue.ValueString() }

// apiKeyPayload is the value object sent to the config resource API. Pointer
// fields distinguish an omitted attribute from an empty one: allowedModels is
// omitted when null and sent as [] when empty (spec C13).
type apiKeyPayload struct {
	Key           *string           `json:"key,omitempty"`
	KeyHash       *string           `json:"keyHash,omitempty"`
	Metadata      map[string]string `json:"metadata,omitempty"`
	AllowedModels *[]string         `json:"allowedModels,omitempty"`
	Budgets       []apiKeyBudget    `json:"budgets,omitempty"`
}

// apiKeyBudget is one budget in the API payload.
type apiKeyBudget struct {
	Name             string              `json:"name"`
	Limit            apiKeyBudgetLimit   `json:"limit"`
	Window           *apiKeyBudgetWindow `json:"window,omitempty"`
	OnBudgetExceeded string              `json:"onBudgetExceeded"`
}

// apiKeyBudgetLimit is the limit of a budget in the API payload.
type apiKeyBudgetLimit struct {
	Unit   string  `json:"unit"`
	Amount float64 `json:"amount"`
}

// apiKeyBudgetWindow is the rolling window of a budget in the API payload.
type apiKeyBudgetWindow struct {
	Rolling string `json:"rolling"`
}

// Value builds the API value object from the model.
func (m *apiKeyModel) Value(ctx context.Context, diags *diag.Diagnostics) json.RawMessage {
	payload := apiKeyPayload{}

	if !m.keyMaterial(&payload, diags) {
		return nil
	}
	if !m.metadataValue(ctx, &payload, diags) {
		return nil
	}
	if !m.allowedModelsValue(ctx, &payload, diags) {
		return nil
	}
	if !m.budgetsValue(ctx, &payload, diags) {
		return nil
	}

	value, err := json.Marshal(payload)
	if err != nil {
		diags.AddError(
			"Invalid agentgateway_llm_api_key",
			"the API key value could not be encoded; check the metadata and budgets attributes",
		)
		return nil
	}
	return value
}

// keyMaterial encodes the key xor key_hash union. The values are secrets: the
// diagnostics never repeat them.
func (m *apiKeyModel) keyMaterial(payload *apiKeyPayload, diags *diag.Diagnostics) bool {
	set := 0
	for _, value := range []types.String{m.Key, m.KeyHash} {
		if isSet(value) {
			set++
		}
	}

	switch set {
	case 0:
		diags.AddError(
			"Invalid agentgateway_llm_api_key",
			"exactly one of key or key_hash must be set",
		)
		return false
	case 1:
	default:
		diags.AddError(
			"Invalid agentgateway_llm_api_key",
			"only one of key or key_hash may be set",
		)
		return false
	}

	if isSet(m.Key) {
		if m.Key.ValueString() == "" {
			diags.AddAttributeError(
				path.Root("key"),
				"Invalid agentgateway_llm_api_key",
				"key must be a non-empty string",
			)
			return false
		}
		key := m.Key.ValueString()
		payload.Key = &key
		return true
	}

	if m.KeyHash.ValueString() == "" {
		diags.AddAttributeError(
			path.Root("key_hash"),
			"Invalid agentgateway_llm_api_key",
			"key_hash must be a non-empty string",
		)
		return false
	}
	keyHash := m.KeyHash.ValueString()
	payload.KeyHash = &keyHash
	return true
}

// metadataValue encodes metadata. An empty map is omitted: the gateway adds its
// own managed keys to every key, so an empty map is indistinguishable from an
// unset one (spec §7.4).
func (m *apiKeyModel) metadataValue(ctx context.Context, payload *apiKeyPayload, diags *diag.Diagnostics) bool {
	if !isSet(m.Metadata) {
		return true
	}

	metadata := map[string]string{}
	if d := m.Metadata.ElementsAs(ctx, &metadata, false); d.HasError() {
		diags.AddAttributeError(
			path.Root("metadata"),
			"Invalid agentgateway_llm_api_key",
			"metadata must be a map of strings",
		)
		return false
	}
	if err := configresource.HasReservedMetadataKey(metadata); err != nil {
		diags.AddAttributeError(
			path.Root("metadata"),
			"Invalid agentgateway_llm_api_key",
			fmt.Sprintf("%s; the gateway manages keys in that namespace itself", err.Error()),
		)
		return false
	}

	if len(metadata) > 0 {
		payload.Metadata = metadata
	}
	return true
}

// allowedModelsValue encodes allowed_models, keeping null and the empty list
// distinct (spec C13).
func (m *apiKeyModel) allowedModelsValue(ctx context.Context, payload *apiKeyPayload, diags *diag.Diagnostics) bool {
	if !isSet(m.AllowedModels) {
		return true
	}

	models := make([]string, 0, len(m.AllowedModels.Elements()))
	if d := m.AllowedModels.ElementsAs(ctx, &models, false); d.HasError() {
		diags.AddAttributeError(
			path.Root("allowed_models"),
			"Invalid agentgateway_llm_api_key",
			"allowed_models must be a list of strings",
		)
		return false
	}
	if models == nil {
		models = []string{}
	}
	payload.AllowedModels = &models
	return true
}

// budgetsValue encodes the budgets list. A null list is omitted; an empty list
// sends no budgets, which is the same thing.
func (m *apiKeyModel) budgetsValue(ctx context.Context, payload *apiKeyPayload, diags *diag.Diagnostics) bool {
	if !isSet(m.Budgets) {
		return true
	}

	var budgets []budgetModel
	if d := m.Budgets.ElementsAs(ctx, &budgets, false); d.HasError() {
		diags.AddAttributeError(
			path.Root("budgets"),
			"Invalid agentgateway_llm_api_key",
			"budgets must be a list of budget objects",
		)
		return false
	}

	for i, budget := range budgets {
		budgetPath := path.Root("budgets").AtListIndex(i)
		encoded, ok := encodeBudget(ctx, budget, budgetPath, diags)
		if !ok {
			return false
		}
		payload.Budgets = append(payload.Budgets, encoded)
	}
	return true
}

// encodeBudget converts one budget model into its payload and validates the
// values the schema validators would have caught at plan time.
func encodeBudget(ctx context.Context, budget budgetModel, budgetPath path.Path, diags *diag.Diagnostics) (apiKeyBudget, bool) {
	encoded := apiKeyBudget{Name: budget.Name.ValueString()}
	if encoded.Name == "" {
		diags.AddAttributeError(budgetPath.AtName("name"), "Invalid agentgateway_llm_api_key", "budget name must be a non-empty string")
		return apiKeyBudget{}, false
	}

	if !isSet(budget.Limit) || budget.Limit.IsNull() {
		diags.AddAttributeError(budgetPath.AtName("limit"), "Invalid agentgateway_llm_api_key", "every budget requires a limit block")
		return apiKeyBudget{}, false
	}

	var limit budgetLimitModel
	if d := budget.Limit.As(ctx, &limit, basetypes.ObjectAsOptions{}); d.HasError() {
		diags.AddAttributeError(budgetPath.AtName("limit"), "Invalid agentgateway_llm_api_key", "limit must be an object with unit and amount")
		return apiKeyBudget{}, false
	}

	switch limit.Unit.ValueString() {
	case budgetUnitUSD, budgetUnitTokens:
	default:
		diags.AddAttributeError(
			budgetPath.AtName("limit").AtName("unit"),
			"Invalid agentgateway_llm_api_key",
			fmt.Sprintf("limit unit must be %q or %q", budgetUnitUSD, budgetUnitTokens),
		)
		return apiKeyBudget{}, false
	}
	if !isSet(limit.Amount) {
		diags.AddAttributeError(budgetPath.AtName("limit").AtName("amount"), "Invalid agentgateway_llm_api_key", "limit amount must be set")
		return apiKeyBudget{}, false
	}

	encoded.Limit = apiKeyBudgetLimit{Unit: limit.Unit.ValueString(), Amount: limit.Amount.ValueFloat64()}

	if isSet(budget.Window) && !budget.Window.IsNull() {
		var window budgetWindowModel
		if d := budget.Window.As(ctx, &window, basetypes.ObjectAsOptions{}); d.HasError() {
			diags.AddAttributeError(budgetPath.AtName("window"), "Invalid agentgateway_llm_api_key", "window must be an object with rolling")
			return apiKeyBudget{}, false
		}
		if window.Rolling.ValueString() == "" {
			diags.AddAttributeError(budgetPath.AtName("window").AtName("rolling"), "Invalid agentgateway_llm_api_key", "window rolling must be a non-empty duration")
			return apiKeyBudget{}, false
		}
		encoded.Window = &apiKeyBudgetWindow{Rolling: window.Rolling.ValueString()}
	}

	switch budget.OnBudgetExceeded.ValueString() {
	case budgetActionAudit, budgetActionBlock:
		encoded.OnBudgetExceeded = budget.OnBudgetExceeded.ValueString()
	default:
		diags.AddAttributeError(
			budgetPath.AtName("on_budget_exceeded"),
			"Invalid agentgateway_llm_api_key",
			fmt.Sprintf("on_budget_exceeded must be %q or %q", budgetActionAudit, budgetActionBlock),
		)
		return apiKeyBudget{}, false
	}

	return encoded, true
}

// apiKeyValue is the wire shape of an llm.apiKey value. RawMessage fields keep
// an absent field distinct from an explicit null.
type apiKeyValue struct {
	Key           json.RawMessage `json:"key"`
	KeyHash       json.RawMessage `json:"keyHash"`
	Metadata      json.RawMessage `json:"metadata"`
	AllowedModels json.RawMessage `json:"allowedModels"`
	Budgets       json.RawMessage `json:"budgets"`
	CreatedAt     string          `json:"createdAt"`
	UpdatedAt     string          `json:"updatedAt"`
}

// apiKeyBudgetValue is one budget as the gateway returns it. Amount is decoded
// as any because the API has returned it both as a number and as a decimal
// string.
type apiKeyBudgetValue struct {
	Name  string `json:"name"`
	Limit struct {
		Unit   string `json:"unit"`
		Amount any    `json:"amount"`
	} `json:"limit"`
	Window *struct {
		Rolling string `json:"rolling"`
	} `json:"window"`
	OnBudgetExceeded string `json:"onBudgetExceeded"`
}

// SetValue stores the value returned by the gateway. Attributes the server did
// not return keep the value already in state; server-managed metadata keys are
// stripped first (spec §7.4).
func (m *apiKeyModel) SetValue(ctx context.Context, value json.RawMessage, diags *diag.Diagnostics) {
	if len(value) == 0 {
		return
	}

	var decoded apiKeyValue
	if err := json.Unmarshal(value, &decoded); err != nil {
		diags.AddError(
			"Failed to read agentgateway_llm_api_key",
			"the gateway returned a value that is not a JSON object",
		)
		return
	}

	m.setTimestamps(value, decoded)

	stripped := configresource.StripManagedMetadata(value)
	var body apiKeyValue
	if err := json.Unmarshal(stripped, &body); err != nil {
		diags.AddError(
			"Failed to read agentgateway_llm_api_key",
			"the gateway returned a value that is not a JSON object",
		)
		return
	}

	m.setKeyMaterial(body)
	m.setMetadata(ctx, body, diags)
	m.setAllowedModels(ctx, body, diags)
	m.setBudgets(ctx, body, diags)
}

// setTimestamps stores created_at and updated_at when the gateway reports them.
// The config API carries the creation time inside the value, either as a
// top-level field or as a managed metadata key.
func (m *apiKeyModel) setTimestamps(value json.RawMessage, decoded apiKeyValue) {
	created := decoded.CreatedAt
	updated := decoded.UpdatedAt

	if created == "" || updated == "" {
		var metadata map[string]string
		if raw := decoded.Metadata; isPresent(raw) {
			_ = json.Unmarshal(raw, &metadata)
		}
		if created == "" {
			created = metadata[managedCreatedAtKey]
		}
		if updated == "" {
			updated = metadata[managedUpdatedAtKey]
		}
	}

	if created != "" {
		m.CreatedAt = types.StringValue(created)
	} else if m.CreatedAt.IsUnknown() {
		m.CreatedAt = types.StringNull()
	}
	if updated != "" {
		m.UpdatedAt = types.StringValue(updated)
	} else if m.UpdatedAt.IsUnknown() {
		m.UpdatedAt = types.StringNull()
	}
}

// setKeyMaterial stores the variant the gateway returned and clears the other
// one, so state can never carry both key and key_hash. A value that omits both
// leaves the state untouched: the gateway does not echo key material back on
// every read.
func (m *apiKeyModel) setKeyMaterial(decoded apiKeyValue) {
	switch {
	case isPresent(decoded.Key):
		m.Key = types.StringValue(unquote(decoded.Key))
		m.KeyHash = types.StringNull()
	case isPresent(decoded.KeyHash):
		m.KeyHash = types.StringValue(unquote(decoded.KeyHash))
		m.Key = types.StringNull()
	}
}

// setMetadata stores the user metadata the gateway returned. Managed keys are
// already stripped, and an empty result reads back as unset (spec §7.4).
func (m *apiKeyModel) setMetadata(ctx context.Context, decoded apiKeyValue, diags *diag.Diagnostics) {
	if !isPresent(decoded.Metadata) {
		return
	}

	var metadata map[string]string
	if err := json.Unmarshal(decoded.Metadata, &metadata); err != nil {
		diags.AddAttributeError(
			path.Root("metadata"),
			"Failed to read agentgateway_llm_api_key",
			"the gateway returned metadata that is not a map of strings",
		)
		return
	}
	if len(metadata) == 0 {
		m.Metadata = types.MapNull(types.StringType)
		return
	}

	value, d := types.MapValueFrom(ctx, types.StringType, metadata)
	if d.HasError() {
		diags.AddAttributeError(
			path.Root("metadata"),
			"Failed to read agentgateway_llm_api_key",
			"the gateway returned metadata that is not a map of strings",
		)
		return
	}
	m.Metadata = value
}

// setAllowedModels stores the server list. An explicit null clears the
// constraint; an absent field keeps the state value.
func (m *apiKeyModel) setAllowedModels(ctx context.Context, decoded apiKeyValue, diags *diag.Diagnostics) {
	if len(decoded.AllowedModels) == 0 {
		return
	}
	if !isPresent(decoded.AllowedModels) {
		m.AllowedModels = types.ListNull(types.StringType)
		return
	}

	var models []string
	if err := json.Unmarshal(decoded.AllowedModels, &models); err != nil {
		diags.AddAttributeError(
			path.Root("allowed_models"),
			"Failed to read agentgateway_llm_api_key",
			"the gateway returned allowedModels that is not a list of strings",
		)
		return
	}

	elements := make([]attr.Value, 0, len(models))
	for _, model := range models {
		elements = append(elements, types.StringValue(model))
	}
	value, d := types.ListValue(types.StringType, elements)
	if d.HasError() {
		diags.AddAttributeError(
			path.Root("allowed_models"),
			"Failed to read agentgateway_llm_api_key",
			"the gateway returned allowedModels that is not a list of strings",
		)
		return
	}
	m.AllowedModels = value
}

// setBudgets stores the server budgets. An absent field keeps the state value.
func (m *apiKeyModel) setBudgets(ctx context.Context, decoded apiKeyValue, diags *diag.Diagnostics) {
	if len(decoded.Budgets) == 0 {
		return
	}
	if !isPresent(decoded.Budgets) {
		m.Budgets = types.ListNull(budgetObjectType())
		return
	}

	var budgets []apiKeyBudgetValue
	if err := json.Unmarshal(decoded.Budgets, &budgets); err != nil {
		diags.AddAttributeError(
			path.Root("budgets"),
			"Failed to read agentgateway_llm_api_key",
			"the gateway returned budgets that is not a list of budget objects",
		)
		return
	}

	elements := make([]attr.Value, 0, len(budgets))
	for i, budget := range budgets {
		element, ok := decodeBudget(ctx, budget, i, diags)
		if !ok {
			return
		}
		elements = append(elements, element)
	}

	value, d := types.ListValue(budgetObjectType(), elements)
	if d.HasError() {
		diags.AddAttributeError(
			path.Root("budgets"),
			"Failed to read agentgateway_llm_api_key",
			"the gateway returned budgets that are not budget objects",
		)
		return
	}
	m.Budgets = value
}

// decodeBudget converts one wire budget into its Terraform object.
func decodeBudget(ctx context.Context, budget apiKeyBudgetValue, index int, diags *diag.Diagnostics) (attr.Value, bool) {
	budgetPath := path.Root("budgets").AtListIndex(index)

	amount, ok := decodeBudgetAmount(budget.Limit.Amount)
	if !ok {
		diags.AddAttributeError(
			budgetPath.AtName("limit").AtName("amount"),
			"Failed to read agentgateway_llm_api_key",
			fmt.Sprintf("the gateway returned a non-numeric amount for budget %q", budget.Name),
		)
		return nil, false
	}

	limit, d := types.ObjectValue(budgetLimitAttrTypes(), map[string]attr.Value{
		"unit":   types.StringValue(budget.Limit.Unit),
		"amount": types.Float64Value(amount),
	})
	if d.HasError() {
		diags.AddAttributeError(
			budgetPath.AtName("limit"),
			"Failed to read agentgateway_llm_api_key",
			fmt.Sprintf("the gateway returned an invalid limit for budget %q", budget.Name),
		)
		return nil, false
	}

	window := types.ObjectNull(budgetWindowAttrTypes())
	if budget.Window != nil {
		value, windowDiags := types.ObjectValue(budgetWindowAttrTypes(), map[string]attr.Value{
			"rolling": types.StringValue(budget.Window.Rolling),
		})
		if windowDiags.HasError() {
			diags.AddAttributeError(
				budgetPath.AtName("window"),
				"Failed to read agentgateway_llm_api_key",
				fmt.Sprintf("the gateway returned an invalid window for budget %q", budget.Name),
			)
			return nil, false
		}
		window = value
	}

	element, d := types.ObjectValue(budgetAttrTypes(), map[string]attr.Value{
		"name":               types.StringValue(budget.Name),
		"limit":              limit,
		"window":             window,
		"on_budget_exceeded": types.StringValue(budget.OnBudgetExceeded),
	})
	if d.HasError() {
		diags.AddAttributeError(
			budgetPath,
			"Failed to read agentgateway_llm_api_key",
			fmt.Sprintf("the gateway returned an invalid budget %q", budget.Name),
		)
		return nil, false
	}
	return element, true
}

// decodeBudgetAmount accepts the number the schema documents and the decimal
// string the status API uses.
func decodeBudgetAmount(raw any) (float64, bool) {
	switch value := raw.(type) {
	case float64:
		return value, true
	case json.Number:
		amount, err := value.Float64()
		return amount, err == nil
	case string:
		amount, err := strconv.ParseFloat(value, 64)
		return amount, err == nil
	default:
		return 0, false
	}
}

// SetServerMeta stores the computed id and revision.
func (m *apiKeyModel) SetServerMeta(id string, revision int64) {
	m.IDValue = types.StringValue(id)
	m.Revision = types.Int64Value(revision)
}

// budgetObjectType is the element type of the budgets list.
func budgetObjectType() attr.Type {
	return types.ObjectType{AttrTypes: budgetAttrTypes()}
}

// budgetAttrTypes are the attributes of one budget entry.
func budgetAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"name":               types.StringType,
		"limit":              types.ObjectType{AttrTypes: budgetLimitAttrTypes()},
		"window":             types.ObjectType{AttrTypes: budgetWindowAttrTypes()},
		"on_budget_exceeded": types.StringType,
	}
}

// budgetLimitAttrTypes are the attributes of a budget limit.
func budgetLimitAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"unit":   types.StringType,
		"amount": types.Float64Type,
	}
}

// budgetWindowAttrTypes are the attributes of a budget window.
func budgetWindowAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"rolling": types.StringType,
	}
}

// isSet reports whether a Terraform value is known and not null.
func isSet(value attr.Value) bool {
	return !value.IsNull() && !value.IsUnknown()
}

// isPresent reports whether a raw JSON field carries a value other than null.
func isPresent(raw json.RawMessage) bool {
	return len(raw) > 0 && string(raw) != "null"
}

// unquote decodes a JSON string field.
func unquote(raw json.RawMessage) string {
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return ""
	}
	return value
}
