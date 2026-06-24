package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccRoleResource drives discord_role through create → update → import,
// covering the role-modify body, the separate modify-positions endpoint, the
// decimal-string permission bitfield, and "server_id/role_id" import.
func TestAccRoleResource(t *testing.T) {
	newMockDiscord(t)
	const rn = "discord_role.test"

	cfg := func(name string, color int) string {
		return fmt.Sprintf(`
resource "discord_role" "test" {
  server_id   = "999"
  name        = %q
  color       = %d
  permissions = "3072"
  position    = 2
  hoist       = true
}
`, name, color)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{ // create
				Config: cfg("Mods", 5814783),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(rn, "name", "Mods"),
					resource.TestCheckResourceAttr(rn, "color", "5814783"),
					resource.TestCheckResourceAttr(rn, "permissions", "3072"),
					resource.TestCheckResourceAttr(rn, "position", "2"),
					resource.TestCheckResourceAttr(rn, "hoist", "true"),
					resource.TestCheckResourceAttr(rn, "managed", "false"),
					resource.TestCheckResourceAttrSet(rn, "id"),
				),
			},
			{ // update name + color
				Config: cfg("Moderators", 16711680),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(rn, "name", "Moderators"),
					resource.TestCheckResourceAttr(rn, "color", "16711680"),
				),
			},
			{ // import by "server_id/role_id"
				ResourceName:      rn,
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: importIDFunc(rn, "server_id", "id"),
			},
		},
	})
}
