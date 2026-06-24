package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccMemberRoleResource covers the non-authoritative single role assignment:
// PUT to add, read-back by scanning the member's roles, and import by
// "server_id/user_id/role_id". It does not touch the member's other roles.
func TestAccMemberRoleResource(t *testing.T) {
	newMockDiscord(t)
	const rn = "discord_member_role.test"
	const cfg = `
resource "discord_member_role" "test" {
  server_id = "999"
  user_id   = "555"
  role_id   = "111111111111111111"
}
`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{ // assign the role
				Config: cfg,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(rn, "server_id", "999"),
					resource.TestCheckResourceAttr(rn, "user_id", "555"),
					resource.TestCheckResourceAttr(rn, "role_id", "111111111111111111"),
				),
			},
			{ // import by "server_id/user_id/role_id" (no `id` attribute)
				ResourceName:                         rn,
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "role_id",
				ImportStateIdFunc:                    importIDFunc(rn, "server_id", "user_id", "role_id"),
			},
		},
	})
}
