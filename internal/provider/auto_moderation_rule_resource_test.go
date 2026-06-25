package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccAutoModerationRuleResource covers a nested-list resource: the flattened
// trigger_metadata (keyword_filter) and the nested actions list round-trip
// through create → update → import.
func TestAccAutoModerationRuleResource(t *testing.T) {
	newMockDiscord(t)
	const rn = "discord_auto_moderation_rule.test"

	cfg := func(name string) string {
		return fmt.Sprintf(`
resource "discord_auto_moderation_rule" "test" {
  server_id      = "999"
  name           = %q
  event_type     = 1
  trigger_type   = 1
  enabled        = true
  keyword_filter = ["badword"]
  actions        = [{ type = 1 }]
}
`, name)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{ // create
				Config: cfg("no badwords"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(rn, "name", "no badwords"),
					resource.TestCheckResourceAttr(rn, "event_type", "1"),
					resource.TestCheckResourceAttr(rn, "trigger_type", "1"),
					resource.TestCheckResourceAttr(rn, "enabled", "true"),
					resource.TestCheckResourceAttr(rn, "keyword_filter.#", "1"),
					resource.TestCheckResourceAttr(rn, "actions.#", "1"),
					resource.TestCheckResourceAttr(rn, "actions.0.type", "1"),
					resource.TestCheckResourceAttrSet(rn, "id"),
				),
			},
			{ // update name
				Config: cfg("blocked words"),
				Check:  resource.TestCheckResourceAttr(rn, "name", "blocked words"),
			},
			{ // import by "server_id/rule_id"
				ResourceName:      rn,
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: importIDFunc(rn, "server_id", "id"),
			},
		},
	})
}

// TestAccAutoModerationRuleResourceMentionSpam covers the mention-spam trigger
// (trigger_type 5): mention_total_limit and mention_raid_protection_enabled
// round-trip through create → update → import.
func TestAccAutoModerationRuleResourceMentionSpam(t *testing.T) {
	newMockDiscord(t)
	const rn = "discord_auto_moderation_rule.mention"

	cfg := func(limit int, raid bool) string {
		return fmt.Sprintf(`
resource "discord_auto_moderation_rule" "mention" {
  server_id                       = "999"
  name                            = "no mention spam"
  event_type                      = 1
  trigger_type                    = 5
  enabled                         = true
  mention_total_limit             = %d
  mention_raid_protection_enabled = %t
  actions                         = [{ type = 1 }]
}
`, limit, raid)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{ // create with raid protection on
				Config: cfg(5, true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(rn, "trigger_type", "5"),
					resource.TestCheckResourceAttr(rn, "mention_total_limit", "5"),
					resource.TestCheckResourceAttr(rn, "mention_raid_protection_enabled", "true"),
					resource.TestCheckResourceAttrSet(rn, "id"),
				),
			},
			{ // update the limit, keep raid protection on
				Config: cfg(10, true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(rn, "mention_total_limit", "10"),
					resource.TestCheckResourceAttr(rn, "mention_raid_protection_enabled", "true"),
				),
			},
			{ // import by "server_id/rule_id"
				ResourceName:      rn,
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: importIDFunc(rn, "server_id", "id"),
			},
		},
	})
}
