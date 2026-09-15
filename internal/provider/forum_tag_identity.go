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
// is unknown. This modifier carries each prior id back into the plan instead,
// matched by `name` rather than by index, so an insert or reorder does not shift
// ids onto the wrong tag. A renamed tag matches nothing and plans a fresh id. ---

type forumTagIdentityModifier struct{}

func forumTagIdentity() planmodifier.List { return forumTagIdentityModifier{} }

func (forumTagIdentityModifier) Description(_ context.Context) string {
	return "carries each tag's prior id onto the planned tag with the same name, regardless of position"
}

func (m forumTagIdentityModifier) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (forumTagIdentityModifier) PlanModifyList(ctx context.Context, req planmodifier.ListRequest, resp *planmodifier.ListResponse) {
	if req.PlanValue.IsNull() || req.PlanValue.IsUnknown() {
		return
	}

	var planned []forumTagModel
	if d := req.PlanValue.ElementsAs(ctx, &planned, false); d.HasError() {
		resp.Diagnostics.Append(d...)
		return
	}
	prior := forumTagModelsFromList(ctx, req.StateValue, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	claimed := make([]bool, len(prior))
	for i := range planned {
		planned[i].ID = types.StringUnknown()
		for j := range prior {
			if claimed[j] || prior[j].Name.ValueString() != planned[i].Name.ValueString() {
				continue
			}
			claimed[j] = true
			planned[i].ID = resolveID(prior[j].ID)
			break
		}
	}

	list, d := types.ListValueFrom(ctx, types.ObjectType{AttrTypes: forumTagAttrTypes}, planned)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.PlanValue = list
}

func forumTagModelsFromList(ctx context.Context, list types.List, diags *diag.Diagnostics) []forumTagModel {
	if list.IsNull() || list.IsUnknown() {
		return nil
	}
	var out []forumTagModel
	diags.Append(list.ElementsAs(ctx, &out, false)...)
	return out
}
