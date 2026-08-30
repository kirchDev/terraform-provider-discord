package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// The plan modifier is the seam the framework itself calls, and the only one that
// reaches the states below: the acceptance harness always plans against the
// current schema, so it can produce neither state written before `key` existed
// nor a hand-rolled mixture of keyed and unkeyed prior elements.

// TestOnboardingIdentityAdoptsStateWrittenBeforeKeysExisted covers the second
// route onto the keyed schema. State written by an earlier provider version has
// no `key` at all; the framework round-trips it against the new schema and fills
// the absent attribute with null. The upgrade then forces the caller to add keys,
// which by definition match nothing in that state — so without a positional
// fallback the very first apply after the upgrade re-mints every id, which is the
// silent, member-facing corruption issue #35 exists to prevent.
func TestOnboardingIdentityAdoptsStateWrittenBeforeKeysExisted(t *testing.T) {
	ctx := context.Background()

	prior := onboardingTestPrompts(t, onboardingTestPrompt(t, types.StringNull(), "111111111111111111",
		onboardingTestOption(types.StringNull(), "222222222222222222", "News"),
		onboardingTestOption(types.StringNull(), "333333333333333333", "Chat"),
	))
	cfg := onboardingTestPrompts(t, onboardingTestPrompt(t, types.StringValue("interests"), "",
		onboardingTestOption(types.StringValue("news"), "", "News"),
		onboardingTestOption(types.StringValue("chat"), "", "Chat"),
	))

	planned := onboardingTestPlan(t, ctx, cfg, prior)

	assertPromptID(t, planned[0], "111111111111111111")
	options := onboardingTestOptionsOf(t, ctx, planned[0])
	assertOptionID(t, options[0], "222222222222222222")
	assertOptionID(t, options[1], "333333333333333333")
}

// TestOnboardingIdentityKeepsRenameDestructiveOnceKeysExist guards the boundary
// the fallback must not cross. A prior element the caller did name is not
// adoptable by position, so renaming its key still retires it and mints a fresh
// id — the documented meaning of a key change, and the reason the fallback cannot
// quietly reintroduce index matching.
func TestOnboardingIdentityKeepsRenameDestructiveOnceKeysExist(t *testing.T) {
	ctx := context.Background()

	prior := onboardingTestPrompts(t, onboardingTestPrompt(t, types.StringValue("interests"), "111111111111111111",
		onboardingTestOption(types.StringValue("news"), "222222222222222222", "News"),
	))
	cfg := onboardingTestPrompts(t, onboardingTestPrompt(t, types.StringValue("hobbies"), "",
		onboardingTestOption(types.StringValue("news"), "", "News"),
	))

	planned := onboardingTestPlan(t, ctx, cfg, prior)

	if !planned[0].ID.IsUnknown() {
		t.Fatalf("prompt id = %v, want unknown: a renamed key must not adopt the prior element by position", planned[0].ID)
	}
}

// TestOnboardingIdentityDoesNotAdoptAPriorElementAlreadyClaimed pins the ordering
// between the two passes. A key match anywhere in the list outranks a positional
// one, so an unkeyed prior element whose id a later configured element already
// claims by key is not handed to whatever happens to share its index.
func TestOnboardingIdentityDoesNotAdoptAPriorElementAlreadyClaimed(t *testing.T) {
	ctx := context.Background()

	prior := onboardingTestPrompts(t,
		onboardingTestPrompt(t, types.StringValue("interests"), "111111111111111111",
			onboardingTestOption(types.StringValue("news"), "222222222222222222", "News")),
	)
	cfg := onboardingTestPrompts(t,
		onboardingTestPrompt(t, types.StringValue("pronouns"), "",
			onboardingTestOption(types.StringValue("they"), "", "they/them")),
		onboardingTestPrompt(t, types.StringValue("interests"), "",
			onboardingTestOption(types.StringValue("news"), "", "News")),
	)

	planned := onboardingTestPlan(t, ctx, cfg, prior)

	if !planned[0].ID.IsUnknown() {
		t.Fatalf("inserted prompt id = %v, want unknown: it must not take the id keyed to \"interests\"", planned[0].ID)
	}
	assertPromptID(t, planned[1], "111111111111111111")
}

// TestOnboardingIdentityRefusesToMigrateAReorderedList closes the one hole a
// purely positional fallback leaves. Adding the keys *and* inserting or
// reordering in the same apply hands each new key the wrong prior element, which
// is the original index bug wearing the migration's clothes — and just as silent.
// Position is only trustworthy while the list has not moved, so where the element
// at that index is plainly a different one the provider refuses the apply instead
// of guessing.
func TestOnboardingIdentityRefusesToMigrateAReorderedList(t *testing.T) {
	ctx := context.Background()

	prior := onboardingTestPrompts(t,
		onboardingTestPromptTitled(t, types.StringNull(), "111111111111111111", "Pick your interests"),
		onboardingTestPromptTitled(t, types.StringNull(), "222222222222222222", "Where are you?"),
	)
	cfg := onboardingTestPrompts(t,
		onboardingTestPromptTitled(t, types.StringValue("pronouns"), "", "Your pronouns"),
		onboardingTestPromptTitled(t, types.StringValue("interests"), "", "Pick your interests"),
		onboardingTestPromptTitled(t, types.StringValue("region"), "", "Where are you?"),
	)

	resp := &planmodifier.ListResponse{PlanValue: cfg}
	onboardingIdentity().PlanModifyList(ctx, planmodifier.ListRequest{
		ConfigValue: cfg, PlanValue: cfg, StateValue: prior,
	}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("planning a key migration over a reordered list succeeded; it must fail rather than " +
			"hand each new key the id of whichever prior prompt shares its index")
	}
}

// --- helpers ---

func onboardingTestPlan(t *testing.T, ctx context.Context, cfg, prior types.List) []onboardingPromptModel {
	t.Helper()
	resp := &planmodifier.ListResponse{PlanValue: cfg}
	onboardingIdentity().PlanModifyList(ctx, planmodifier.ListRequest{
		ConfigValue: cfg,
		PlanValue:   cfg,
		StateValue:  prior,
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("plan modifier returned diagnostics: %v", resp.Diagnostics)
	}
	var out []onboardingPromptModel
	if d := resp.PlanValue.ElementsAs(ctx, &out, false); d.HasError() {
		t.Fatalf("reading the planned prompts: %v", d)
	}
	return out
}

func onboardingTestOptionsOf(t *testing.T, ctx context.Context, p onboardingPromptModel) []onboardingOptionModel {
	t.Helper()
	var out []onboardingOptionModel
	if d := p.Options.ElementsAs(ctx, &out, false); d.HasError() {
		t.Fatalf("reading the planned options: %v", d)
	}
	return out
}

func assertPromptID(t *testing.T, p onboardingPromptModel, want string) {
	t.Helper()
	if p.ID.IsUnknown() || p.ID.ValueString() != want {
		t.Fatalf("prompt %q id = %v, want the id Discord already assigned (%s)", p.Key.ValueString(), p.ID, want)
	}
}

func assertOptionID(t *testing.T, o onboardingOptionModel, want string) {
	t.Helper()
	if o.ID.IsUnknown() || o.ID.ValueString() != want {
		t.Fatalf("option %q id = %v, want the id Discord already assigned (%s)", o.Key.ValueString(), o.ID, want)
	}
}

func onboardingTestID(id string) types.String {
	if id == "" {
		return types.StringNull()
	}
	return types.StringValue(id)
}

func onboardingTestOption(key types.String, id, title string) onboardingOptionModel {
	return onboardingOptionModel{
		Key:         key,
		ID:          onboardingTestID(id),
		Title:       types.StringValue(title),
		Description: types.StringNull(),
		EmojiID:     types.StringNull(),
		EmojiName:   types.StringNull(),
		ChannelIDs:  types.SetNull(types.StringType),
		RoleIDs:     types.SetNull(types.StringType),
	}
}

func onboardingTestPrompt(t *testing.T, key types.String, id string, options ...onboardingOptionModel) onboardingPromptModel {
	t.Helper()
	p := onboardingTestPromptTitled(t, key, id, "Pick your interests")
	list, d := types.ListValueFrom(context.Background(), types.ObjectType{AttrTypes: onboardingOptionAttrTypes}, options)
	if d.HasError() {
		t.Fatalf("building the option list: %v", d)
	}
	p.Options = list
	return p
}

func onboardingTestPromptTitled(t *testing.T, key types.String, id, title string) onboardingPromptModel {
	t.Helper()
	list, d := types.ListValueFrom(context.Background(), types.ObjectType{AttrTypes: onboardingOptionAttrTypes}, []onboardingOptionModel{})
	if d.HasError() {
		t.Fatalf("building the option list: %v", d)
	}
	return onboardingPromptModel{
		Key:          key,
		ID:           onboardingTestID(id),
		Type:         types.Int64Value(0),
		Title:        types.StringValue(title),
		SingleSelect: types.BoolValue(false),
		Required:     types.BoolValue(true),
		InOnboarding: types.BoolValue(true),
		Options:      list,
	}
}

func onboardingTestPrompts(t *testing.T, prompts ...onboardingPromptModel) types.List {
	t.Helper()
	list, d := types.ListValueFrom(context.Background(), types.ObjectType{AttrTypes: onboardingPromptAttrTypes}, prompts)
	if d.HasError() {
		t.Fatalf("building the prompt list: %v", d)
	}
	return list
}
