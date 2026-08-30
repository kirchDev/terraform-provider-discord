package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

const (
	roleOrderGuildID = "900000000000000001"
	roleOrderLow     = "900000000000000010"
	roleOrderForeign = "900000000000000020"
	roleOrderMid     = "900000000000000030"
	roleOrderHigh    = "900000000000000040"
)

// TestAccRoleOrderResource_subsetWithForeignRole orders a strict subset of the
// guild's roles while an unmanaged role sits inside the same position range —
// someone's hand-made role, an app-owned bot role. The listed roles must come out
// in the configured order without any of them being written onto the slot that
// foreign role holds.
func TestAccRoleOrderResource_subsetWithForeignRole(t *testing.T) {
	m := newMockDiscord(t)
	m.seedRole(roleOrderGuildID, "@everyone", 0, false)
	m.seedRole(roleOrderLow, "Members", 1, false)
	m.seedRole(roleOrderForeign, "Hand made", 2, false)
	m.seedRole(roleOrderMid, "Mods", 3, false)
	m.seedRole(roleOrderHigh, "Admins", 4, false)

	const rn = "discord_role_order.test"
	cfg := fmt.Sprintf(`
resource "discord_role_order" "test" {
  server_id = %q
  role_ids  = [%q, %q, %q]
}
`, roleOrderGuildID, roleOrderMid, roleOrderHigh, roleOrderLow)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{{
			Config: cfg,
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr(rn, "role_ids.#", "3"),
				resource.TestCheckResourceAttr(rn, "role_ids.0", roleOrderMid),
				resource.TestCheckResourceAttr(rn, "role_ids.1", roleOrderHigh),
				resource.TestCheckResourceAttr(rn, "role_ids.2", roleOrderLow),
				checkOrder(m.rolePositions, true, roleOrderMid, roleOrderHigh, roleOrderLow),
				// The listed roles swap among the three slots they already held;
				// the foreign role keeps the one between them.
				checkAtSlot(m.rolePositions, roleOrderMid, 4),
				checkAtSlot(m.rolePositions, roleOrderHigh, 3),
				checkAtSlot(m.rolePositions, roleOrderForeign, 2),
				checkAtSlot(m.rolePositions, roleOrderLow, 1),
				checkNoSharedSlot(m.rolePositions),
			),
		}},
	})
}
