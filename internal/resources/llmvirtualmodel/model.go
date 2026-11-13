package llmvirtualmodel

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/caberdo/terraform-provider-agentgateway/internal/configresource"
	"github.com/caberdo/terraform-provider-agentgateway/internal/resourceutil"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// virtualModelModel is the Terraform-side model of an llm.virtualModel. The
// struct is populated by reflection, so only exported fields with tfsdk tags
// survive a plan or state read.
type virtualModelModel struct {
	Name        types.String       `tfsdk:"name"`
	Weighted    []weightedModel    `tfsdk:"weighted"`
	Failover    []failoverModel    `tfsdk:"failover"`
	Conditional []conditionalModel `tfsdk:"conditional"`
	StoredID    types.String       `tfsdk:"id"`
	Revision    types.Int64        `tfsdk:"revision"`
}

type weightedModel struct {
	Targets []weightedTargetModel `tfsdk:"targets"`
}

type weightedTargetModel struct {
	Model  types.String `tfsdk:"model"`
	Weight types.Int64  `tfsdk:"weight"`
}

type failoverModel struct {
	Targets []failoverTargetModel `tfsdk:"targets"`
}

type failoverTargetModel struct {
	Model    types.String `tfsdk:"model"`
	Priority types.Int64  `tfsdk:"priority"`
}

type conditionalModel struct {
	Targets []conditionalTargetModel `tfsdk:"targets"`
}

type conditionalTargetModel struct {
	Model types.String `tfsdk:"model"`
	When  types.String `tfsdk:"when"`
}

// Kind returns the config resource kind.
func (m *virtualModelModel) Kind() configresource.Kind {
	return configresource.KindLLMVirtualModel
}

// ID returns the name-keyed id (spec §7.0).
func (m *virtualModelModel) ID() string {
	return m.Name.ValueString()
}

// SetServerMeta stores the computed id and revision returned by the server.
func (m *virtualModelModel) SetServerMeta(id string, revision int64) {
	m.StoredID = types.StringValue(id)
	m.Revision = types.Int64Value(revision)
}

// Value marshals the model to the API value object. Exactly one routing block
// is required; the routing rules are also enforced here so a hand-built model
// cannot produce an ambiguous payload.
func (m *virtualModelModel) Value(ctx context.Context, diags *diag.Diagnostics) json.RawMessage {
	groups := 0
	for _, present := range []bool{len(m.Weighted) > 0, len(m.Failover) > 0, len(m.Conditional) > 0} {
		if present {
			groups++
		}
	}
	if groups != 1 {
		diags.AddError(
			"Invalid virtual model routing",
			fmt.Sprintf("exactly one of weighted, failover or conditional must be set; got %d", groups),
		)
		return nil
	}

	routing := map[string]any{}

	switch {
	case len(m.Weighted) > 0:
		targets, ok := m.weightedTargets(diags)
		if !ok {
			return nil
		}
		routing["weighted"] = map[string]any{"targets": targets}
	case len(m.Failover) > 0:
		targets, ok := m.failoverTargets(diags)
		if !ok {
			return nil
		}
		routing["failover"] = map[string]any{"targets": targets}
	case len(m.Conditional) > 0:
		targets, ok := m.conditionalTargets(diags)
		if !ok {
			return nil
		}
		routing["conditional"] = map[string]any{"targets": targets}
	}

	value, err := json.Marshal(map[string]any{
		"name":    m.Name.ValueString(),
		"routing": routing,
	})
	if err != nil {
		diags.AddError("Invalid virtual model value", fmt.Sprintf("could not encode the request: %s", err))
		return nil
	}
	return value
}

func (m *virtualModelModel) weightedTargets(diags *diag.Diagnostics) ([]map[string]any, bool) {
	targets := m.Weighted[0].Targets
	if len(targets) == 0 {
		diags.AddAttributeError(
			path.Root("weighted").AtListIndex(0).AtName("targets"),
			"Invalid weighted routing",
			"weighted requires at least one target",
		)
		return nil, false
	}

	encoded := make([]map[string]any, 0, len(targets))
	for _, target := range targets {
		body := map[string]any{"model": target.Model.ValueString()}
		// Weight defaults to 1 server-side; omit it when unset so the server
		// applies that default instead of receiving a zero.
		if !target.Weight.IsNull() && !target.Weight.IsUnknown() {
			body["weight"] = target.Weight.ValueInt64()
		}
		encoded = append(encoded, body)
	}
	return encoded, true
}

func (m *virtualModelModel) failoverTargets(diags *diag.Diagnostics) ([]map[string]any, bool) {
	targets := m.Failover[0].Targets
	if len(targets) == 0 {
		diags.AddAttributeError(
			path.Root("failover").AtListIndex(0).AtName("targets"),
			"Invalid failover routing",
			"failover requires at least one target",
		)
		return nil, false
	}

	encoded := make([]map[string]any, 0, len(targets))
	for _, target := range targets {
		body := map[string]any{"model": target.Model.ValueString()}
		if !target.Priority.IsNull() && !target.Priority.IsUnknown() {
			body["priority"] = target.Priority.ValueInt64()
		}
		encoded = append(encoded, body)
	}
	return encoded, true
}

func (m *virtualModelModel) conditionalTargets(diags *diag.Diagnostics) ([]map[string]any, bool) {
	targets := m.Conditional[0].Targets
	if len(targets) == 0 {
		diags.AddAttributeError(
			path.Root("conditional").AtListIndex(0).AtName("targets"),
			"Invalid conditional routing",
			"conditional requires at least one target",
		)
		return nil, false
	}

	encoded := make([]map[string]any, 0, len(targets))
	for i, target := range targets {
		body := map[string]any{"model": target.Model.ValueString()}
		if target.When.IsNull() || target.When.IsUnknown() {
			if i != len(targets)-1 {
				diags.AddAttributeError(
					path.Root("conditional").AtListIndex(0).AtName("targets").AtListIndex(i).AtName("when"),
					"Invalid conditional target",
					fmt.Sprintf("conditional target %d requires when; only the last target may omit it", i),
				)
				return nil, false
			}
			encoded = append(encoded, body)
			continue
		}
		body["when"] = target.When.ValueString()
		encoded = append(encoded, body)
	}
	return encoded, true
}

// serverValue is the part of the API value this resource understands.
type serverValue struct {
	Name    *string        `json:"name"`
	Routing *serverRouting `json:"routing"`
}

type serverRouting struct {
	Weighted    *serverWeighted    `json:"weighted"`
	Failover    *serverFailover    `json:"failover"`
	Conditional *serverConditional `json:"conditional"`
}

type serverWeighted struct {
	Targets []serverWeightedTarget `json:"targets"`
}

type serverWeightedTarget struct {
	Model  *string `json:"model"`
	Weight *int64  `json:"weight"`
}

type serverFailover struct {
	Targets []serverFailoverTarget `json:"targets"`
}

type serverFailoverTarget struct {
	Model    *string `json:"model"`
	Priority *int64  `json:"priority"`
}

type serverConditional struct {
	Targets []serverConditionalTarget `json:"targets"`
}

type serverConditionalTarget struct {
	Model *string `json:"model"`
	When  *string `json:"when"`
}

// SetValue unmarshals an API value into the model. Attributes the server did
// not return are preserved from the plan or state, never zeroed.
func (m *virtualModelModel) SetValue(ctx context.Context, value json.RawMessage, diags *diag.Diagnostics) {
	var server serverValue
	if err := json.Unmarshal(value, &server); err != nil {
		diags.AddError(
			"Invalid gateway response",
			fmt.Sprintf("could not decode the llm.virtualModel value: %s", err),
		)
		return
	}

	if server.Name != nil {
		m.Name = types.StringValue(*server.Name)
	}

	if server.Routing == nil {
		return
	}

	switch {
	case server.Routing.Weighted != nil:
		m.Weighted = m.setWeighted(ctx, server.Routing.Weighted.Targets)
		m.Failover = nil
		m.Conditional = nil
	case server.Routing.Failover != nil:
		m.Failover = m.setFailover(ctx, server.Routing.Failover.Targets)
		m.Weighted = nil
		m.Conditional = nil
	case server.Routing.Conditional != nil:
		m.Conditional = m.setConditional(ctx, server.Routing.Conditional.Targets)
		m.Weighted = nil
		m.Failover = nil
	default:
		// The server returned a routing mode this resource does not model
		// (for example callout). Preserve what the plan or state holds rather
		// than silently dropping it.
	}
}

func (m *virtualModelModel) setWeighted(ctx context.Context, targets []serverWeightedTarget) []weightedModel {
	var existing []weightedTargetModel
	if len(m.Weighted) > 0 {
		existing = m.Weighted[0].Targets
	}

	decoded := make([]weightedTargetModel, 0, len(targets))
	for i, target := range targets {
		var old weightedTargetModel
		if i < len(existing) {
			old = existing[i]
		}

		decoded = append(decoded, weightedTargetModel{
			Model:  preserveString(ctx, old.Model, stringOrNull(target.Model)),
			Weight: preserveDefaultInt64(ctx, old.Weight, int64OrNull(target.Weight), 1),
		})
	}
	return []weightedModel{{Targets: decoded}}
}

func (m *virtualModelModel) setFailover(ctx context.Context, targets []serverFailoverTarget) []failoverModel {
	var existing []failoverTargetModel
	if len(m.Failover) > 0 {
		existing = m.Failover[0].Targets
	}

	decoded := make([]failoverTargetModel, 0, len(targets))
	for i, target := range targets {
		var old failoverTargetModel
		if i < len(existing) {
			old = existing[i]
		}

		decoded = append(decoded, failoverTargetModel{
			Model:    preserveString(ctx, old.Model, stringOrNull(target.Model)),
			Priority: preserveDefaultInt64(ctx, old.Priority, int64OrNull(target.Priority), 0),
		})
	}
	return []failoverModel{{Targets: decoded}}
}

func (m *virtualModelModel) setConditional(ctx context.Context, targets []serverConditionalTarget) []conditionalModel {
	var existing []conditionalTargetModel
	if len(m.Conditional) > 0 {
		existing = m.Conditional[0].Targets
	}

	decoded := make([]conditionalTargetModel, 0, len(targets))
	for i, target := range targets {
		var old conditionalTargetModel
		if i < len(existing) {
			old = existing[i]
		}

		decoded = append(decoded, conditionalTargetModel{
			Model: preserveString(ctx, old.Model, stringOrNull(target.Model)),
			When:  preserveString(ctx, old.When, stringOrNull(target.When)),
		})
	}
	return []conditionalModel{{Targets: decoded}}
}

// preserveString keeps the existing value when the server did not return one.
func preserveString(ctx context.Context, existing, server types.String) types.String {
	preserved, ok := resourceutil.PreserveIfNull(ctx, existing, server).(types.String)
	if !ok {
		return existing
	}
	return preserved
}

// preserveDefaultInt64 keeps the existing value when the server omitted the
// field. When the server applied its default to an attribute the user left
// unset, the attribute stays null so the plan and the refreshed state agree.
func preserveDefaultInt64(ctx context.Context, existing, server types.Int64, defaultValue int64) types.Int64 {
	if !server.IsNull() && existing.IsNull() && server.ValueInt64() == defaultValue {
		return existing
	}
	preserved, ok := resourceutil.PreserveIfNull(ctx, existing, server).(types.Int64)
	if !ok {
		return existing
	}
	return preserved
}

func stringOrNull(value *string) types.String {
	if value == nil {
		return types.StringNull()
	}
	return types.StringValue(*value)
}

func int64OrNull(value *int64) types.Int64 {
	if value == nil {
		return types.Int64Null()
	}
	return types.Int64Value(*value)
}
