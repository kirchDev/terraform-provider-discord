package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccTextChannelResource drives discord_text_channel through create →
// update → import. It also covers the channel pattern's import-by-id: server_id
// is recovered from the channel's guild_id on read, so the import id is just the
// channel id.
func TestAccTextChannelResource(t *testing.T) {
	newMockDiscord(t)
	const rn = "discord_text_channel.test"

	cfg := func(name, topic string) string {
		return fmt.Sprintf(`
resource "discord_text_channel" "test" {
  server_id = "999"
  name      = %q
  topic     = %q
  nsfw      = true
}
`, name, topic)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{ // create
				Config: cfg("general", "Welcome!"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(rn, "name", "general"),
					resource.TestCheckResourceAttr(rn, "topic", "Welcome!"),
					resource.TestCheckResourceAttr(rn, "nsfw", "true"),
					resource.TestCheckResourceAttr(rn, "server_id", "999"),
					resource.TestCheckResourceAttrSet(rn, "id"),
				),
			},
			{ // update topic
				Config: cfg("general", "Read the rules"),
				Check:  resource.TestCheckResourceAttr(rn, "topic", "Read the rules"),
			},
			{ // import by channel id; sync flag is an apply-time action, not stored
				ResourceName:            rn,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateIdFunc:       importIDFunc(rn, "id"),
				ImportStateVerifyIgnore: []string{"sync_perms_with_category"},
			},
		},
	})
}
