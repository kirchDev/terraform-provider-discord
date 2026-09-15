package provider

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// forumTagCfg renders a discord_forum_channel with the given topic and tag names.
func forumTagCfg(topic string, tags ...string) string {
	var b strings.Builder
	for _, t := range tags {
		b.WriteString(`
    { name = "` + t + `" },`)
	}
	return `
resource "discord_forum_channel" "test" {
  server_id = "999"
  name      = "ideas"
  topic     = "` + topic + `"

  available_tags = [` + b.String() + `
  ]
}
`
}

// checkForumTagIDsSent asserts the tag ids of the last PATCH the provider sent
// for the channel in state: each must equal the id captured for that tag earlier.
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

	var modID, pluginID string

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: forumTagCfg("Share your ideas.", "Mod", "Plugin"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(rn, "available_tags.#", "2"),
					captureAttr(rn, "available_tags.0.id", &modID),
					captureAttr(rn, "available_tags.1.id", &pluginID),
				),
			},
			{
				Config: forumTagCfg("Share your ideas. Be kind.", "Mod", "Plugin"),
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

	var modID, pluginID, empty string

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: forumTagCfg("Share your ideas.", "Mod", "Plugin"),
				Check: resource.ComposeAggregateTestCheckFunc(
					captureAttr(rn, "available_tags.0.id", &modID),
					captureAttr(rn, "available_tags.1.id", &pluginID),
				),
			},
			{
				Config: forumTagCfg("Share your ideas.", "Datapack", "Plugin", "Mod"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(rn, "available_tags.#", "3"),
					resource.TestCheckResourceAttr(rn, "available_tags.0.name", "Datapack"),
					resource.TestCheckResourceAttrSet(rn, "available_tags.0.id"),
					checkForumTagIDsSent(m, rn, &empty, &pluginID, &modID),
					checkAttrEquals(rn, "available_tags.1.id", &pluginID),
					checkAttrEquals(rn, "available_tags.2.id", &modID),
				),
			},
		},
	})
}
