package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// --- Stable identity for a forum's available_tags.
//
// Discord has no per-tag endpoint: the channel PATCH carries the whole list, and a
// tag sent without an id is a new tag, so every tag sent without one is deleted
// and recreated — and every post that carried it is left untagged.
//
// The nested `id` is Computed only, so as soon as the resource has any diff at
// all (even just `topic`) the framework plans it unknown on every element. A
// list-level UseStateForUnknown does not help: it only fires when the whole list
// is unknown. This modifier carries each prior id back into the plan instead.
//
// Identity is the caller's `key`, exactly as for onboarding prompts and options
// (server_onboarding_identity.go), and matching shares its `matchPrior`: a tag
// whose key was in prior state keeps its id wherever it now sits, so renaming,
// re-emojiing, toggling `moderated` or reordering all keep it; a new key plans
// the id unknown. State that predates `key`, and an import, are adopted by
// position while `name` corroborates it, and the plan is refused where it does
// not. ---

var forumTagFields = identityFields[forumTagModel]{
	noun:   "tag",
	titled: "named",
	stakes: "every post carrying a tag loses it when its id changes",
	key:    func(t forumTagModel) types.String { return t.Key },
	id:     func(t forumTagModel) types.String { return t.ID },
	title:  func(t forumTagModel) types.String { return t.Name },
}

type forumTagIdentityModifier struct{}

func forumTagIdentity() planmodifier.List { return forumTagIdentityModifier{} }

func (forumTagIdentityModifier) Description(_ context.Context) string {
	return "resolves tag identity from the caller's `key` rather than the list index"
}

func (m forumTagIdentityModifier) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (forumTagIdentityModifier) PlanModifyList(ctx context.Context, req planmodifier.ListRequest, resp *planmodifier.ListResponse) {
	// A null plan is a destroy; an unknown one means the list is wholly computed
	// and there are no keys to resolve against.
	if req.PlanValue.IsNull() || req.PlanValue.IsUnknown() {
		return
	}

	// The planned elements, not the config ones: they already carry the
	// `moderated` default.
	var planned []forumTagModel
	if d := req.PlanValue.ElementsAs(ctx, &planned, false); d.HasError() {
		resp.Diagnostics.Append(d...)
		return
	}
	prior := forumTagModelsFromList(ctx, req.StateValue, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	matches, d := matchPrior(planned, prior, forumTagFields)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}

	for i := range planned {
		if prior := matches[i]; prior != nil {
			planned[i].ID = resolveID(prior.ID)
			continue
		}
		planned[i].ID = types.StringUnknown()
	}

	list, d := types.ListValueFrom(ctx, types.ObjectType{AttrTypes: forumTagAttrTypes}, planned)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.PlanValue = list
}

// forumTagKey recovers the key a read-back tag was written under. By id first,
// since ids are stable and a tag reordered in the Discord UI must still be
// recognised; then by position, but only onto an element that has no id yet —
// a tag this very create or update sent as new, which Discord echoes in the
// order it was sent. Neither (an import, or a tag created by hand in Discord)
// falls back to the snowflake, the only stable name available.
func forumTagKey(wireID string, index int, prior []forumTagModel) types.String {
	for i := range prior {
		if id, ok := stableKey(prior[i].ID); ok && id == wireID {
			if _, ok := stableKey(prior[i].Key); ok {
				return prior[i].Key
			}
			return types.StringValue(wireID)
		}
	}
	if index < len(prior) {
		if _, hasID := stableKey(prior[index].ID); !hasID {
			if _, ok := stableKey(prior[index].Key); ok {
				return prior[index].Key
			}
		}
	}
	return types.StringValue(wireID)
}

func forumTagModelsFromList(ctx context.Context, list types.List, diags *diag.Diagnostics) []forumTagModel {
	if list.IsNull() || list.IsUnknown() {
		return nil
	}
	var out []forumTagModel
	diags.Append(list.ElementsAs(ctx, &out, false)...)
	return out
}
