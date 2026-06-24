package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccMemberRolesResource covers the authoritative member→role set: the live
// roles array is read back into the set (drift detection), updates replace the
// set, and import is by "server_id/user_id".
func TestAccMemberRolesResource(t *testing.T) {
	newMockDiscord(t)
	const rn = "discord_member_roles.test"

	cfg := func(roles string) string {
		return fmt.Sprintf(`
resource "discord_member_roles" "test" {
  server_id = "999"
  user_id   = "555"
  roles     = %s
}
`, roles)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{ // assign two roles
				Config: cfg(`["111111111111111111", "222222222222222222"]`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(rn, "roles.#", "2"),
					resource.TestCheckTypeSetElemAttr(rn, "roles.*", "111111111111111111"),
					resource.TestCheckTypeSetElemAttr(rn, "roles.*", "222222222222222222"),
				),
			},
			{ // shrink to one role (authoritative replace)
				Config: cfg(`["111111111111111111"]`),
				Check:  resource.TestCheckResourceAttr(rn, "roles.#", "1"),
			},
			{ // import by "server_id/user_id" (no `id` attribute)
				ResourceName:                         rn,
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "user_id",
				ImportStateIdFunc:                    importIDFunc(rn, "server_id", "user_id"),
			},
		},
	})
}
