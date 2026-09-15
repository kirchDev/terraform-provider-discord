package provider

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// forumTagCfg renders a discord_forum_channel with the given topic ("" omits it)
// and tags, each given as {key, name}.
func forumTagCfg(topic string, tags ...[2]string) string {
	var b strings.Builder
	for _, t := range tags {
		b.WriteString(`
    { key = "` + t[0] + `", name = "` + t[1] + `" },`)
	}
	topicLine := ""
	if topic != "" {
		topicLine = `
  topic     = "` + topic + `"`
	}
	return `
resource "discord_forum_channel" "test" {
  server_id = "999"
  name      = "ideas"` + topicLine + `

  available_tags = [` + b.String() + `
  ]
}
`
}

// checkForumTagIDsSent asserts the tag ids of the last PATCH the provider sent
// for the channel in state: each must equal the id captured for that tag earlier
// ("" for a tag that must be sent as new).
func checkForumTagIDsSent(m *mockDiscord, rn string, want ...*string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[rn]
		if !ok {
			return fmt.Errorf("resource %s not found in state", rn)
		}
		sent := m.forumTagIDsSent(rs.Primary.ID)
		if len(sent) == 0 {
			return fmt.Errorf("no PATCH carrying available_tags was sent for channel %s", rs.Primary.ID)
		}
		last := sent[len(sent)-1]
		if len(last) != len(want) {
			return fmt.Errorf("PATCH sent %d tags, want %d", len(last), len(want))
		}
		for i, w := range want {
			if last[i] != *w {
				return fmt.Errorf("PATCH available_tags[%d].id = %q, want the existing id %q — "+
					"a tag sent without its id is recreated by Discord and every post loses it", i, last[i], *w)
			}
		}
		return nil
	}
}

// TestAccForumChannelResourceTopicKeepsTagIDs is issue #60's worst reproducer: an
// update to any other attribute of the forum — here only `topic` — planned every
// tag id unknown, so the PATCH carried none and Discord replaced every tag.
func TestAccForumChannelResourceTopicKeepsTagIDs(t *testing.T) {
	m := newMockDiscord(t)
	const rn = "discord_forum_channel.test"
	mod, plugin := [2]string{"mod", "Mod"}, [2]string{"plugin", "Plugin"}

	var modID, pluginID string

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: forumTagCfg("Share your ideas.", mod, plugin),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(rn, "available_tags.#", "2"),
					resource.TestCheckResourceAttr(rn, "available_tags.0.key", "mod"),
					resource.TestCheckResourceAttr(rn, "available_tags.1.key", "plugin"),
					captureAttr(rn, "available_tags.0.id", &modID),
					captureAttr(rn, "available_tags.1.id", &pluginID),
				),
			},
			{
				Config: forumTagCfg("Share your ideas. Be kind.", mod, plugin),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(rn, "topic", "Share your ideas. Be kind."),
					checkForumTagIDsSent(m, rn, &modID, &pluginID),
					checkAttrEquals(rn, "available_tags.0.id", &modID),
					checkAttrEquals(rn, "available_tags.1.id", &pluginID),
				),
			},
		},
	})
}

// TestAccForumChannelResourceInsertKeepsTagIDs covers the first reproducer:
// inserting a tag ahead of the existing ones and reordering them must not shift
// ids onto the wrong tag or reissue the untouched ones.
func TestAccForumChannelResourceInsertKeepsTagIDs(t *testing.T) {
	m := newMockDiscord(t)
	const rn = "discord_forum_channel.test"
	mod, plugin, datapack := [2]string{"mod", "Mod"}, [2]string{"plugin", "Plugin"}, [2]string{"datapack", "Datapack"}

	var modID, pluginID, empty string

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: forumTagCfg("Share your ideas.", mod, plugin),
				Check: resource.ComposeAggregateTestCheckFunc(
					captureAttr(rn, "available_tags.0.id", &modID),
					captureAttr(rn, "available_tags.1.id", &pluginID),
				),
			},
			{
				Config: forumTagCfg("Share your ideas.", datapack, plugin, mod),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(rn, "available_tags.#", "3"),
					resource.TestCheckResourceAttr(rn, "available_tags.0.key", "datapack"),
					resource.TestCheckResourceAttrSet(rn, "available_tags.0.id"),
					checkForumTagIDsSent(m, rn, &empty, &pluginID, &modID),
					checkAttrEquals(rn, "available_tags.1.id", &pluginID),
					checkAttrEquals(rn, "available_tags.2.id", &modID),
				),
			},
		},
	})
}

// TestAccForumChannelResourceRenameKeepsTagID is what `key` buys over matching by
// name: renaming a tag, changing its emoji and toggling `moderated` all keep the
// id, so the posts carrying the tag keep it.
func TestAccForumChannelResourceRenameKeepsTagID(t *testing.T) {
	m := newMockDiscord(t)
	const rn = "discord_forum_channel.test"

	cfg := func(name, emoji string, moderated bool) string {
		return fmt.Sprintf(`
resource "discord_forum_channel" "test" {
  server_id = "999"
  name      = "ideas"

  available_tags = [
    { key = "rule", name = %q, emoji_name = %q, moderated = %t },
  ]
}
`, name, emoji, moderated)
	}

	var ruleID string

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: cfg("Regel", "📜", false),
				Check:  captureAttr(rn, "available_tags.0.id", &ruleID),
			},
			{
				Config: cfg("Rule", "📏", true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(rn, "available_tags.0.name", "Rule"),
					resource.TestCheckResourceAttr(rn, "available_tags.0.emoji_name", "📏"),
					resource.TestCheckResourceAttr(rn, "available_tags.0.moderated", "true"),
					checkForumTagIDsSent(m, rn, &ruleID),
					checkAttrEquals(rn, "available_tags.0.id", &ruleID),
				),
			},
		},
	})
}

// TestAccForumChannelResourceImportThenAddKeys covers the route onto the keyed
// schema. Discord stores no keys, so an import falls back to each tag's snowflake;
// the apply that replaces those with readable keys must keep every id, or the
// migration is itself the reissue issue #60 exists to prevent.
func TestAccForumChannelResourceImportThenAddKeys(t *testing.T) {
	m := newMockDiscord(t)
	const rn = "discord_forum_channel.test"
	const channelID, modID, pluginID = "700000000000000001", "111111111111111111", "222222222222222222"
	m.seedForumChannel(channelID, "999", "ideas", [2]string{modID, "Mod"}, [2]string{pluginID, "Plugin"})

	cfg := forumTagCfg("", [2]string{"mod", "Mod"}, [2]string{"plugin", "Plugin"})
	modWant, pluginWant := modID, pluginID

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config:             cfg,
				ResourceName:       rn,
				ImportState:        true,
				ImportStatePersist: true,
				ImportStateId:      channelID,
			},
			{
				Config: cfg,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(rn, "available_tags.0.key", "mod"),
					resource.TestCheckResourceAttr(rn, "available_tags.1.key", "plugin"),
					resource.TestCheckResourceAttr(rn, "available_tags.0.id", modID),
					resource.TestCheckResourceAttr(rn, "available_tags.1.id", pluginID),
					checkForumTagIDsSent(m, rn, &modWant, &pluginWant),
				),
			},
		},
	})
}

// TestAccForumChannelResourceAddKeysWhileReorderingFails is the other half of the
// migration: position only carries identity while the list has not moved, so
// adding the keys and reordering in one apply must fail rather than hand each new
// key the id of whichever tag happens to share its index.
func TestAccForumChannelResourceAddKeysWhileReorderingFails(t *testing.T) {
	m := newMockDiscord(t)
	const rn = "discord_forum_channel.test"
	const channelID = "700000000000000001"
	m.seedForumChannel(channelID, "999", "ideas",
		[2]string{"111111111111111111", "Mod"}, [2]string{"222222222222222222", "Plugin"})

	cfg := forumTagCfg("", [2]string{"plugin", "Plugin"}, [2]string{"mod", "Mod"})

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config:             cfg,
				ResourceName:       rn,
				ImportState:        true,
				ImportStatePersist: true,
				ImportStateId:      channelID,
			},
			{
				Config:      cfg,
				ExpectError: regexp.MustCompile(`Cannot tell which tag this is`),
			},
		},
	})
}

// TestAccForumChannelResourceDuplicateTagKey guards the invariant key-based
// identity rests on: two tags sharing a key would both claim the same prior tag.
func TestAccForumChannelResourceDuplicateTagKey(t *testing.T) {
	newMockDiscord(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config:      forumTagCfg("", [2]string{"mod", "Mod"}, [2]string{"mod", "Plugin"}),
				ExpectError: regexp.MustCompile(`tag key "mod" is already used at index 0`),
			},
		},
	})
}

// TestForumTagIdentityAdoptsStateWrittenBeforeKeysExisted covers state written by
// a provider version before `key` existed, which the acceptance harness cannot
// produce: the framework fills the absent attribute with null, and the first
// apply with keys must still keep every id.
func TestForumTagIdentityAdoptsStateWrittenBeforeKeysExisted(t *testing.T) {
	ctx := context.Background()

	prior := forumTestTags(t,
		forumTestTag(types.StringNull(), "111111111111111111", "Mod"),
		forumTestTag(types.StringNull(), "222222222222222222", "Plugin"),
	)
	cfg := forumTestTags(t,
		forumTestTag(types.StringValue("mod"), "", "Mod"),
		forumTestTag(types.StringValue("plugin"), "", "Plugin"),
	)

	resp := &planmodifier.ListResponse{PlanValue: cfg}
	forumTagIdentity().PlanModifyList(ctx, planmodifier.ListRequest{ConfigValue: cfg, PlanValue: cfg, StateValue: prior}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("plan modifier returned diagnostics: %v", resp.Diagnostics)
	}
	var planned []forumTagModel
	if d := resp.PlanValue.ElementsAs(ctx, &planned, false); d.HasError() {
		t.Fatalf("reading the planned tags: %v", d)
	}
	for i, want := range []string{"111111111111111111", "222222222222222222"} {
		if planned[i].ID.IsUnknown() || planned[i].ID.ValueString() != want {
			t.Fatalf("tag %q id = %v, want the id Discord already assigned (%s)", planned[i].Key.ValueString(), planned[i].ID, want)
		}
	}
}

func forumTestTag(key types.String, id, name string) forumTagModel {
	idVal := types.StringNull()
	if id != "" {
		idVal = types.StringValue(id)
	}
	return forumTagModel{
		Key: key, ID: idVal, Name: types.StringValue(name), Moderated: types.BoolValue(false),
		EmojiID: types.StringNull(), EmojiName: types.StringNull(),
	}
}

func forumTestTags(t *testing.T, tags ...forumTagModel) types.List {
	t.Helper()
	list, d := types.ListValueFrom(context.Background(), types.ObjectType{AttrTypes: forumTagAttrTypes}, tags)
	if d.HasError() {
		t.Fatalf("building the tag list: %v", d)
	}
	return list
}

// forumRequireTagCfg renders a bare discord_forum_channel; requireTag "" omits the
// attribute, otherwise it is the literal HCL value.
func forumRequireTagCfg(requireTag string) string {
	line := ""
	if requireTag != "" {
		line = `
  require_tag = ` + requireTag
	}
	return `
resource "discord_forum_channel" "test" {
  server_id = "999"
  name      = "ideas"` + line + `
}
`
}

// otherChannelFlag stands in for any flag bit the provider does not manage; a
// require_tag write must leave it exactly as Discord has it.
const otherChannelFlag = 1 << 4

// TestAccForumChannelResourceRequireTagCreate covers setting the flag when the
// forum is created, and clearing it again later.
func TestAccForumChannelResourceRequireTagCreate(t *testing.T) {
	m := newMockDiscord(t)
	const rn = "discord_forum_channel.test"
	var channelID string

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: forumRequireTagCfg("true"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(rn, "require_tag", "true"),
					captureAttr(rn, "id", &channelID),
					func(*terraform.State) error {
						if got := m.channelFlags(channelID); got != forumChannelFlagRequireTag {
							return fmt.Errorf("live flags = %d, want %d", got, forumChannelFlagRequireTag)
						}
						return nil
					},
				),
			},
			{
				Config: forumRequireTagCfg("false"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(rn, "require_tag", "false"),
					func(*terraform.State) error {
						if got := m.channelFlags(channelID); got != 0 {
							return fmt.Errorf("live flags = %d, want 0", got)
						}
						return nil
					},
				),
			},
		},
	})
}

// TestAccForumChannelResourceRequireTagPreservesOtherFlags is the brief's write
// rule: toggling require_tag sets or clears bit 15 only, never sends a bare value.
func TestAccForumChannelResourceRequireTagPreservesOtherFlags(t *testing.T) {
	m := newMockDiscord(t)
	const rn = "discord_forum_channel.test"
	const channelID = "700000000000000001"
	m.seedForumChannel(channelID, "999", "ideas")
	m.setChannelFlags(channelID, otherChannelFlag)

	checkLastFlagsSent := func(want int64) resource.TestCheckFunc {
		return func(*terraform.State) error {
			sent := m.channelFlagsSent(channelID)
			if len(sent) == 0 {
				return fmt.Errorf("no PATCH carrying flags was sent")
			}
			if got := sent[len(sent)-1]; got != want {
				return fmt.Errorf("PATCH flags = %d, want %d — every other bit must be preserved", got, want)
			}
			return nil
		}
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config:             forumRequireTagCfg(""),
				ResourceName:       rn,
				ImportState:        true,
				ImportStatePersist: true,
				ImportStateId:      channelID,
			},
			{
				Config: forumRequireTagCfg("true"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(rn, "require_tag", "true"),
					checkLastFlagsSent(otherChannelFlag|forumChannelFlagRequireTag),
				),
			},
			{
				Config: forumRequireTagCfg("false"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(rn, "require_tag", "false"),
					checkLastFlagsSent(otherChannelFlag),
				),
			},
		},
	})
}

// TestAccForumChannelResourceRequireTagOmittedPlansNoChange keeps today's
// behaviour for configs without the attribute: a forum already requiring a tag
// is read as such and no change is planned or sent.
func TestAccForumChannelResourceRequireTagOmittedPlansNoChange(t *testing.T) {
	m := newMockDiscord(t)
	const rn = "discord_forum_channel.test"
	const channelID = "700000000000000001"
	m.seedForumChannel(channelID, "999", "ideas")
	m.setChannelFlags(channelID, forumChannelFlagRequireTag)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config:             forumRequireTagCfg(""),
				ResourceName:       rn,
				ImportState:        true,
				ImportStatePersist: true,
				ImportStateId:      channelID,
			},
			{
				Config: forumRequireTagCfg(""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(rn, "require_tag", "true"),
					func(*terraform.State) error {
						if got := m.channelFlags(channelID); got != forumChannelFlagRequireTag {
							return fmt.Errorf("live flags = %d, want %d", got, forumChannelFlagRequireTag)
						}
						if sent := m.channelFlagsSent(channelID); len(sent) != 0 {
							return fmt.Errorf("flags were sent (%v) for a config that omits require_tag", sent)
						}
						return nil
					},
				),
			},
			{
				Config:   forumRequireTagCfg(""),
				PlanOnly: true,
			},
		},
	})
}

// TestAccForumChannelResourceRequireTagManualToggleIsDrift covers the read rule: a
// flag toggled by hand in the Discord client shows up in the plan.
func TestAccForumChannelResourceRequireTagManualToggleIsDrift(t *testing.T) {
	m := newMockDiscord(t)
	const rn = "discord_forum_channel.test"
	var channelID string

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: forumRequireTagCfg("true"),
				Check:  captureAttr(rn, "id", &channelID),
			},
			{
				PreConfig:          func() { m.setChannelFlags(channelID, 0) },
				Config:             forumRequireTagCfg("true"),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}
