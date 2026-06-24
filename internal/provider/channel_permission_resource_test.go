package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccChannelPermissionResource covers the overwrite pattern: PUT the
// overwrite, read it back by scanning the channel's permission_overwrites, update
// the bits, and import by "channel_id/overwrite_id". The channel is created by a
// sibling text-channel resource.
func TestAccChannelPermissionResource(t *testing.T) {
	newMockDiscord(t)
	const rn = "discord_channel_permission.test"

	cfg := func(allow, deny string) string {
		return fmt.Sprintf(`
resource "discord_text_channel" "c" {
  server_id = "999"
  name      = "general"
}

resource "discord_channel_permission" "test" {
  channel_id   = discord_text_channel.c.id
  overwrite_id = "999"
  type         = "role"
  allow        = %q
  deny         = %q
}
`, allow, deny)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{ // create the overwrite
				Config: cfg("1024", "2048"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(rn, "type", "role"),
					resource.TestCheckResourceAttr(rn, "allow", "1024"),
					resource.TestCheckResourceAttr(rn, "deny", "2048"),
				),
			},
			{ // update the bits
				Config: cfg("3072", "0"),
				Check:  resource.TestCheckResourceAttr(rn, "allow", "3072"),
			},
			{ // import by "channel_id/overwrite_id" (no `id` attribute)
				ResourceName:                         rn,
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "overwrite_id",
				ImportStateIdFunc:                    importIDFunc(rn, "channel_id", "overwrite_id"),
			},
		},
	})
}
