package provider

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

const (
	roleOrderGuildID = "900000000000000001"
	roleOrderLow     = "900000000000000010"
	roleOrderForeign = "900000000000000020"
	roleOrderMid     = "900000000000000030"
	roleOrderHigh    = "900000000000000040"
	roleOrderBooster = "900000000000000050"

	// A hierarchy for the two shortfall tests: a freshly created role shares
	// another's position, so the listed roles hold fewer distinct slots than there
	// are of them. Only the bot role's position differs between the two, which is
	// exactly what decides whether there is room to make up the shortfall.
	roleOrderDenseGuildID = "900000000000000004"
	roleOrderDenseMembers = "900000000000000410"
	roleOrderDenseMods    = "900000000000000420"
	roleOrderDenseAdmins  = "900000000000000430"
	roleOrderDenseBot     = "900000000000000440"
	roleOrderDenseFresh   = "900000000000000450"
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

// TestAccRoleOrderResource_rejectsManagedRole covers the second failure listed on
// the issue: an integration-managed role (Server Booster, a Twitch subscriber tier)
// cannot be moved by anyone, whatever the bot's own place in the hierarchy, and
// Discord answers the whole PATCH with a bare 50013 "Missing Permissions". The
// provider names the offending role instead.
func TestAccRoleOrderResource_rejectsManagedRole(t *testing.T) {
	m := newMockDiscord(t)
	m.seedRole(roleOrderGuildID, "@everyone", 0, false)
	m.seedRole(roleOrderLow, "Members", 1, false)
	m.seedRole(roleOrderBooster, "Server Booster", 2, true)

	cfg := fmt.Sprintf(`
resource "discord_role_order" "test" {
  server_id = %q
  role_ids  = [%q, %q]
}
`, roleOrderGuildID, roleOrderBooster, roleOrderLow)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{{
			Config:      cfg,
			ExpectError: regexp.MustCompile(`Server Booster`),
		}},
	})
}

// TestAccRoleOrderResource_refusesToCrossAnUnlistedRole is the role side of the
// shortfall: Discord creates every new role at position 1, so adding one role to a
// managed order leaves two listed roles sharing a slot and the set needs one more
// position than it holds. In a dense hierarchy the only free position is above
// every occupied one — including the app-owned bot role, which is `managed` and so
// unlisted by construction. Writing there makes Discord reject the whole PATCH
// with the bare 50013 this resource exists to make legible, so the resource
// refuses instead, naming the role that closes the range, and writes nothing.
func TestAccRoleOrderResource_refusesToCrossAnUnlistedRole(t *testing.T) {
	m := newMockDiscord(t)
	m.seedRole(roleOrderDenseGuildID, "@everyone", 0, false)
	m.seedRole(roleOrderDenseMembers, "Members", 1, false)
	m.seedRole(roleOrderDenseMods, "Mods", 2, false)
	m.seedRole(roleOrderDenseAdmins, "Admins", 3, false)
	m.seedRole(roleOrderDenseBot, "kirchbot", 4, true)
	// Freshly created, so Discord put it on position 1 beside Members.
	m.seedRole(roleOrderDenseFresh, "Helpers", 1, false)

	cfg := fmt.Sprintf(`
resource "discord_role_order" "test" {
  server_id = %q
  role_ids  = [%q, %q, %q, %q]
}
`, roleOrderDenseGuildID,
		roleOrderDenseAdmins, roleOrderDenseMods, roleOrderDenseFresh, roleOrderDenseMembers)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{{
			Config:      cfg,
			ExpectError: regexp.MustCompile(`(?s)kirchbot.*only\s+3\s+are\s+free`),
		}},
	})

	// Nothing may have been written: the point of the refusal is that no request
	// Discord would reject is ever sent.
	seeded := map[string]int64{
		roleOrderDenseGuildID: 0,
		roleOrderDenseMembers: 1,
		roleOrderDenseMods:    2,
		roleOrderDenseAdmins:  3,
		roleOrderDenseBot:     4,
		roleOrderDenseFresh:   1,
	}
	live := m.rolePositions()
	for id, want := range seeded {
		if got := live[id]; got != want {
			t.Errorf("%s moved to position %d, want it left on %d", id, got, want)
		}
	}
}

// TestAccRoleOrderResource_freshRoleSharesASlot is the same shortfall where the
// hierarchy has room: the bot role sits high enough that a free position exists
// below it. The set must be topped up from there rather than refused — the refusal
// above is the answer to no room, not to a shortfall as such.
func TestAccRoleOrderResource_freshRoleSharesASlot(t *testing.T) {
	m := newMockDiscord(t)
	m.seedRole(roleOrderDenseGuildID, "@everyone", 0, false)
	m.seedRole(roleOrderDenseMembers, "Members", 1, false)
	m.seedRole(roleOrderDenseMods, "Mods", 2, false)
	m.seedRole(roleOrderDenseAdmins, "Admins", 3, false)
	m.seedRole(roleOrderDenseBot, "kirchbot", 6, true)
	m.seedRole(roleOrderDenseFresh, "Helpers", 1, false)

	cfg := fmt.Sprintf(`
resource "discord_role_order" "test" {
  server_id = %q
  role_ids  = [%q, %q, %q, %q]
}
`, roleOrderDenseGuildID,
		roleOrderDenseAdmins, roleOrderDenseMods, roleOrderDenseFresh, roleOrderDenseMembers)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{{
			Config: cfg,
			Check: resource.ComposeAggregateTestCheckFunc(
				checkOrder(m.rolePositions, true,
					roleOrderDenseAdmins, roleOrderDenseMods, roleOrderDenseFresh, roleOrderDenseMembers),
				// The shortfall is made up from position 4, the lowest free one —
				// never from 7, which is above the bot.
				checkAtSlot(m.rolePositions, roleOrderDenseAdmins, 4),
				checkAtSlot(m.rolePositions, roleOrderDenseBot, 6),
				checkNoSharedSlot(m.rolePositions),
			),
		}},
	})
}
