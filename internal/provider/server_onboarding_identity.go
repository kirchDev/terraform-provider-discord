package provider

import (
	"context"
	"strconv"

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
// modifier had just marked unknown.
//
// Getting *onto* the scheme needs one more rule, since state that predates `key`
// and state fresh from an import both hold no key the caller chose — see
// `matchPrior` for the positional fallback that carries their ids across, and the
// guard that stops it guessing once the list has moved. ---

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
	priorPrompts := onboardingModelsFromList(ctx, req.StateValue, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	matches, d := matchPrior(cfg, priorPrompts, promptFields)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}

	planned := make([]onboardingPromptModel, 0, len(cfg))
	for i, c := range cfg {
		p := c // key, type and title are Required — the config value is the answer.
		prior := matches[i]

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
	priorOptions := onboardingOptionModelsFromList(ctx, priorList, &diags)
	if diags.HasError() {
		return types.ListNull(optType), diags
	}
	matches, d := matchPrior(cfg, priorOptions, optionFields)
	diags.Append(d...)
	if diags.HasError() {
		return types.ListNull(optType), diags
	}

	planned := make([]onboardingOptionModel, 0, len(cfg))
	for i, c := range cfg {
		o := c // key, title and the optional display fields come from config.
		if prior := matches[i]; prior != nil {
			o.ID = resolveID(prior.ID)
			o.ChannelIDs = resolveSet(c.ChannelIDs, prior.ChannelIDs)
			o.RoleIDs = resolveSet(c.RoleIDs, prior.RoleIDs)
			planned = append(planned, o)
			continue
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

// identityFields reads the three attributes matching needs off an element, so
// prompts and options share one matcher.
type identityFields[T any] struct {
	noun           string
	key, id, title func(T) types.String
}

var promptFields = identityFields[onboardingPromptModel]{
	noun:  "prompt",
	key:   func(p onboardingPromptModel) types.String { return p.Key },
	id:    func(p onboardingPromptModel) types.String { return p.ID },
	title: func(p onboardingPromptModel) types.String { return p.Title },
}

var optionFields = identityFields[onboardingOptionModel]{
	noun:  "option",
	key:   func(o onboardingOptionModel) types.String { return o.Key },
	id:    func(o onboardingOptionModel) types.String { return o.ID },
	title: func(o onboardingOptionModel) types.String { return o.Title },
}

// matchPrior pairs each configured element with the prior element whose identity
// it continues, returning nil where there is none. Two passes, because a key
// match at any config index outranks a positional one anywhere:
//
//  1. **By key** — the whole point of the scheme. An element the caller named
//     keeps its id wherever it now sits in the list.
//  2. **By position, and only onto an unkeyed prior element** — the way *onto*
//     the scheme. State written before `key` existed carries a null one, and an
//     import can only fall back to the snowflake; in both the caller's first
//     apply introduces keys that by definition match nothing, and without this
//     pass every id would be re-minted on exactly the apply every existing
//     consumer has to run. Once real keys are in state no prior element is
//     adoptable, so the index bug this whole file exists to fix is not
//     reintroduced — the fallback is reachable only from those two states.
//
// Position is only worth trusting while the list has not moved, so the second
// pass corroborates it with `title` and **refuses the plan** where the element at
// that index is plainly a different one. Adding the keys and reordering in one
// apply would otherwise hand each new key the wrong id — the same silent
// corruption, reached through the migration instead of an insert.
func matchPrior[T any](cfg, prior []T, f identityFields[T]) ([]*T, diag.Diagnostics) {
	var diags diag.Diagnostics
	out := make([]*T, len(cfg))
	claimed := make([]bool, len(prior))

	byKey := make(map[string]int, len(prior))
	for i := range prior {
		if k, ok := stableKey(f.key(prior[i])); ok {
			if _, dup := byKey[k]; !dup {
				byKey[k] = i
			}
		}
	}
	for i := range cfg {
		if k, ok := stableKey(f.key(cfg[i])); ok {
			if j, found := byKey[k]; found && !claimed[j] {
				claimed[j] = true
				out[i] = &prior[j]
			}
		}
	}
	for i := range cfg {
		if out[i] != nil || i >= len(prior) || claimed[i] {
			continue
		}
		if !adoptableByPosition(f.key(prior[i]), f.id(prior[i])) {
			continue
		}
		if was, now := f.title(prior[i]).ValueString(), f.title(cfg[i]).ValueString(); was != now {
			diags.AddError(
				"Cannot tell which "+f.noun+" this is",
				"The "+f.noun+" at index "+strconv.Itoa(i)+" carries no key from a previous apply, so the only "+
					"thing left to identify it by is its position — and the "+f.noun+" that held that position is "+
					"titled "+strconv.Quote(was)+", not "+strconv.Quote(now)+".\n\n"+
					"That happens when keys are added in the same apply that reorders, inserts or renames "+
					"something. Guessing here would attach the id Discord assigned to the wrong "+f.noun+", and "+
					"members' \"Channels & Roles\" selections hang on those ids.\n\n"+
					"Add the keys on their own first, leaving every "+f.noun+" where and as it is, and apply. "+
					"Reordering, renaming and inserting are all safe once the keys are in state.",
			)
			continue
		}
		claimed[i] = true
		out[i] = &prior[i]
	}
	return out, diags
}

// adoptableByPosition reports whether a prior element's key names nothing the
// caller ever chose, leaving position the only identity it has.
//
// That is true of exactly two states: one written before `key` existed, where the
// attribute round-trips as null, and a freshly imported one, where the key is the
// snowflake because Discord stores no key to recover. A key the caller chose
// makes the element unadoptable, so renaming one still retires the element and
// mints a fresh id, as documented.
//
// A caller who picks a key that happens to equal its own element's snowflake
// keeps that element adoptable, so a later rename preserves the id instead of
// re-minting it. That is the safe direction of the two, and the docs steer
// callers to readable keys.
func adoptableByPosition(key, id types.String) bool {
	k, ok := stableKey(key)
	if !ok {
		return true
	}
	i, ok := stableKey(id)
	return ok && k == i
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
