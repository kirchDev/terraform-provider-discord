package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// --- Stable identity for the onboarding prompts → options tree.
//
// Discord mints the snowflake in `id`, and members' "Channels & Roles"
// selections hang on the *option* ids, so an id has to survive every edit that
// is not a deletion. Terraform matches a ListNestedAttribute's prior state by
// **index**, which cannot express that: insert a prompt and every later element
// inherits its predecessor's id, while the new trailing index has no prior state
// at all and plans as null — the "produced an unexpected new value: .prompts[2].id
// was null, but now ..." apply error.
//
// `title` cannot stand in for identity either. It is Required but freely
// editable, so keying on it turns a rename into "old deleted, new added" and
// re-mints that prompt's id — moving the failure somewhere harder to spot.
//
// So identity is the caller's: a `key` per prompt and per option, and the plan
// modifier below resolves against it. An element whose key was in prior state
// keeps that element's id wherever it now sits in the list; an element with a
// new key plans its computed attributes **unknown** rather than inheriting
// whatever happened to share its index.
//
// This replaces UseStateForUnknown on the two `id` attributes rather than
// dropping it: left on, it would re-apply the same index matching to any id this
// modifier had just marked unknown. ---

// onboardingIdentityModifier keys the planned prompts tree on `key`.
type onboardingIdentityModifier struct{}

func onboardingIdentity() planmodifier.List { return onboardingIdentityModifier{} }

func (onboardingIdentityModifier) Description(_ context.Context) string {
	return "resolves prompt and option identity from the caller's `key` rather than the list index"
}

func (m onboardingIdentityModifier) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (onboardingIdentityModifier) PlanModifyList(ctx context.Context, req planmodifier.ListRequest, resp *planmodifier.ListResponse) {
	// A null plan is a destroy; an unknown or absent config means the tree is
	// wholly computed and there are no keys to resolve against.
	if req.PlanValue.IsNull() || req.PlanValue.IsUnknown() {
		return
	}
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	var cfg []onboardingPromptModel
	if d := req.ConfigValue.ElementsAs(ctx, &cfg, false); d.HasError() {
		resp.Diagnostics.Append(d...)
		return
	}
	priorByKey := make(map[string]onboardingPromptModel, len(cfg))
	for _, p := range onboardingModelsFromList(ctx, req.StateValue, &resp.Diagnostics) {
		if k, ok := stableKey(p.Key); ok {
			priorByKey[k] = p
		}
	}
	if resp.Diagnostics.HasError() {
		return
	}

	planned := make([]onboardingPromptModel, 0, len(cfg))
	for _, c := range cfg {
		p := c // key, type and title are Required — the config value is the answer.
		var prior *onboardingPromptModel
		if k, ok := stableKey(c.Key); ok {
			if match, found := priorByKey[k]; found {
				prior = &match
			}
		}

		var priorOptions types.List
		if prior != nil {
			priorOptions = prior.Options
			p.ID = resolveID(prior.ID)
			p.SingleSelect = resolveBool(c.SingleSelect, prior.SingleSelect)
			p.Required = resolveBool(c.Required, prior.Required)
			p.InOnboarding = resolveBool(c.InOnboarding, prior.InOnboarding)
		} else {
			priorOptions = types.ListNull(types.ObjectType{AttrTypes: onboardingOptionAttrTypes})
			p.ID = types.StringUnknown()
			p.SingleSelect = resolveBool(c.SingleSelect, types.BoolUnknown())
			p.Required = resolveBool(c.Required, types.BoolUnknown())
			p.InOnboarding = resolveBool(c.InOnboarding, types.BoolUnknown())
		}

		options, d := planOptions(ctx, c.Options, priorOptions)
		resp.Diagnostics.Append(d...)
		if resp.Diagnostics.HasError() {
			return
		}
		p.Options = options
		planned = append(planned, p)
	}

	list, d := types.ListValueFrom(ctx, types.ObjectType{AttrTypes: onboardingPromptAttrTypes}, planned)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.PlanValue = list
}

// planOptions applies the same key matching one level down, within the options
// of a single prompt.
func planOptions(ctx context.Context, cfgList, priorList types.List) (types.List, diag.Diagnostics) {
	var diags diag.Diagnostics
	optType := types.ObjectType{AttrTypes: onboardingOptionAttrTypes}
	if cfgList.IsNull() || cfgList.IsUnknown() {
		return cfgList, diags
	}

	var cfg []onboardingOptionModel
	if d := cfgList.ElementsAs(ctx, &cfg, false); d.HasError() {
		diags.Append(d...)
		return types.ListNull(optType), diags
	}
	priorByKey := make(map[string]onboardingOptionModel, len(cfg))
	for _, o := range onboardingOptionModelsFromList(ctx, priorList, &diags) {
		if k, ok := stableKey(o.Key); ok {
			priorByKey[k] = o
		}
	}
	if diags.HasError() {
		return types.ListNull(optType), diags
	}

	planned := make([]onboardingOptionModel, 0, len(cfg))
	for _, c := range cfg {
		o := c // key, title and the optional display fields come from config.
		if k, ok := stableKey(c.Key); ok {
			if prior, found := priorByKey[k]; found {
				o.ID = resolveID(prior.ID)
				o.ChannelIDs = resolveSet(c.ChannelIDs, prior.ChannelIDs)
				o.RoleIDs = resolveSet(c.RoleIDs, prior.RoleIDs)
				planned = append(planned, o)
				continue
			}
		}
		o.ID = types.StringUnknown()
		o.ChannelIDs = resolveSet(c.ChannelIDs, types.SetUnknown(types.StringType))
		o.RoleIDs = resolveSet(c.RoleIDs, types.SetUnknown(types.StringType))
		planned = append(planned, o)
	}

	list, d := types.ListValueFrom(ctx, optType, planned)
	diags.Append(d...)
	return list, diags
}

// stableKey reports the key an element can be matched on. An unknown or empty
// key identifies nothing, so such an element is treated as new rather than
// matched against an arbitrary neighbour.
func stableKey(k types.String) (string, bool) {
	if k.IsNull() || k.IsUnknown() || k.ValueString() == "" {
		return "", false
	}
	return k.ValueString(), true
}

// resolveID keeps the id already assigned to this key; an element whose key is
// new plans unknown, so Discord's answer is what lands in state.
func resolveID(prior types.String) types.String {
	if prior.IsNull() || prior.IsUnknown() {
		return types.StringUnknown()
	}
	return prior
}

// resolveBool prefers the configured value, carries the prior one across for an
// element that already existed, and otherwise leaves the attribute unknown.
func resolveBool(cfg, prior types.Bool) types.Bool {
	if !cfg.IsNull() {
		return cfg
	}
	if prior.IsNull() {
		return types.BoolUnknown()
	}
	return prior
}

// resolveSet is resolveBool for the Optional+Computed id sets on an option.
func resolveSet(cfg, prior types.Set) types.Set {
	if !cfg.IsNull() {
		return cfg
	}
	if prior.IsNull() {
		return types.SetUnknown(types.StringType)
	}
	return prior
}

func onboardingModelsFromList(ctx context.Context, list types.List, diags *diag.Diagnostics) []onboardingPromptModel {
	if list.IsNull() || list.IsUnknown() {
		return nil
	}
	var out []onboardingPromptModel
	diags.Append(list.ElementsAs(ctx, &out, false)...)
	return out
}

func onboardingOptionModelsFromList(ctx context.Context, list types.List, diags *diag.Diagnostics) []onboardingOptionModel {
	if list.IsNull() || list.IsUnknown() {
		return nil
	}
	var out []onboardingOptionModel
	diags.Append(list.ElementsAs(ctx, &out, false)...)
	return out
}

// --- carrying the keys across a read ---
//
// Discord stores no such key, so a refreshed tree has to recover each one from
// the value the provider already holds — the plan just applied, or the prior
// state being refreshed.

// onboardingPromptKeys matches a read-back prompt to the key it was written
// under: by id first, since ids are stable and a prompt reordered in the Discord
// UI must still be recognised, then positionally, since a create/update response
// echoes the order it was sent. Neither matching (an import) falls back to the
// snowflake itself, which is the only stable name available at that point.
func onboardingPromptKey(wireID string, index int, prior []onboardingPromptModel) (types.String, *onboardingPromptModel) {
	match := onboardingMatchPrompt(wireID, index, prior)
	if match != nil {
		if _, ok := stableKey(match.Key); ok {
			return match.Key, match
		}
	}
	return types.StringValue(wireID), match
}

func onboardingMatchPrompt(wireID string, index int, prior []onboardingPromptModel) *onboardingPromptModel {
	for i := range prior {
		if id, ok := stableKey(prior[i].ID); ok && id == wireID {
			return &prior[i]
		}
	}
	if index < len(prior) {
		return &prior[index]
	}
	return nil
}

func onboardingOptionKey(wireID string, index int, prior []onboardingOptionModel) types.String {
	for i := range prior {
		if id, ok := stableKey(prior[i].ID); ok && id == wireID {
			if _, ok := stableKey(prior[i].Key); ok {
				return prior[i].Key
			}
		}
	}
	if index < len(prior) {
		if _, ok := stableKey(prior[index].Key); ok {
			return prior[index].Key
		}
	}
	return types.StringValue(wireID)
}
