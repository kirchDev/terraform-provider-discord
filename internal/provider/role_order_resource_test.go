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
	roleOrderDenseBooster = "900000000000000460"
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

// TestAccRoleOrderResource_placesAFreshRoleInADenseHierarchy is the case the
// resource used to refuse. Discord creates every new role on position 1 without
// renumbering anything, so adding one role to a managed order leaves two listed
// roles sharing a slot and the set needs one more position than it holds. In a
// dense hierarchy there is no free position below the app's own role — which is
// `managed`, so unlisted by construction — and topping the shortfall up from
// below is impossible.
//
// The way out is the one an operator reaches for by hand: dragging the new role
// in the Discord UI makes Discord renumber, and a position appears. The resource
// now plans that renumbering itself — it writes the listed roles over the whole
// dense range, including the position the app role stands on, and Discord's own
// re-sort bumps the app role up. What is relaxed is that role's *absolute*
// position; its place relative to every listed role is unchanged, which the
// resource checks before the write and against the read-back after it.
func TestAccRoleOrderResource_placesAFreshRoleInADenseHierarchy(t *testing.T) {
	m := newMockDiscord(t)
	m.seedRole(roleOrderDenseGuildID, "@everyone", 0, false)
	m.seedRole(roleOrderDenseMembers, "Members", 1, false)
	m.seedRole(roleOrderDenseMods, "Mods", 2, false)
	m.seedRole(roleOrderDenseAdmins, "Admins", 3, false)
	m.seedBotRole(roleOrderDenseBot, "kirchbot", 4)
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
			Config: cfg,
			Check: resource.ComposeAggregateTestCheckFunc(
				checkOrder(m.rolePositions, true,
					roleOrderDenseAdmins, roleOrderDenseMods, roleOrderDenseFresh, roleOrderDenseMembers),
				checkAtSlot(m.rolePositions, roleOrderDenseMembers, 1),
				checkAtSlot(m.rolePositions, roleOrderDenseFresh, 2),
				checkAtSlot(m.rolePositions, roleOrderDenseMods, 3),
				checkAtSlot(m.rolePositions, roleOrderDenseAdmins, 4),
				// The app role was never in the body — Discord's re-sort moved it,
				// and it is still above every listed role.
				checkAtSlot(m.rolePositions, roleOrderDenseBot, 5),
				checkNoSharedSlot(m.rolePositions),
			),
		}},
	})
}

// TestAccRoleOrderResource_refusesToCrossAnUnlistedRole is the case renumbering
// cannot rescue: an unlisted, integration-managed role stands *between* two
// listed ones, and the configured order asks those two to swap sides around it.
// No layout satisfies that while leaving the unlisted role where it stands
// relative to both, so the resource says so, naming it, and writes nothing —
// relaxing an unlisted role's absolute position is allowed, changing its relative
// one never is.
func TestAccRoleOrderResource_refusesToCrossAnUnlistedRole(t *testing.T) {
	m := newMockDiscord(t)
	m.seedRole(roleOrderDenseGuildID, "@everyone", 0, false)
	m.seedRole(roleOrderDenseMembers, "Members", 1, false)
	m.seedRole(roleOrderDenseFresh, "Helpers", 1, false)
	m.seedRole(roleOrderDenseBooster, "Server Booster", 2, true)
	m.seedRole(roleOrderDenseAdmins, "Admins", 3, false)
	m.seedBotRole(roleOrderDenseBot, "kirchbot", 4)

	cfg := fmt.Sprintf(`
resource "discord_role_order" "test" {
  server_id = %q
  role_ids  = [%q, %q, %q]
}
`, roleOrderDenseGuildID,
		roleOrderDenseMembers, roleOrderDenseAdmins, roleOrderDenseFresh)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{{
			Config:      cfg,
			ExpectError: regexp.MustCompile(`(?s)Server Booster.*managed by an integration`),
		}},
	})

	// Nothing may have been written: the point of the refusal is that no request
	// Discord would reject is ever sent.
	seeded := map[string]int64{
		roleOrderDenseGuildID: 0,
		roleOrderDenseMembers: 1,
		roleOrderDenseFresh:   1,
		roleOrderDenseBooster: 2,
		roleOrderDenseAdmins:  3,
		roleOrderDenseBot:     4,
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

// TestAccRoleOrderResource_rejectsUnknownRole is the role half of the same guard:
// a role deleted out of band contributes neither a slot to reuse nor a sibling to
// route around, so it silently turns into a shortfall the top-up then has to
// invent positions for. The resource names the missing role instead.
func TestAccRoleOrderResource_rejectsUnknownRole(t *testing.T) {
	m := newMockDiscord(t)
	m.seedRole(roleOrderDenseGuildID, "@everyone", 0, false)
	m.seedRole(roleOrderDenseMembers, "Members", 1, false)

	cfg := fmt.Sprintf(`
resource "discord_role_order" "test" {
  server_id = %q
  role_ids  = [%q, %q]
}
`, roleOrderDenseGuildID, roleOrderDenseAdmins, roleOrderDenseMembers)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{{
			Config: cfg,
			// Matched on the provider's own wording: the id alone also appears in
			// the "produced an unexpected new value" failure this guard replaces.
			ExpectError: regexp.MustCompile(`(?s)no\s+role\s+` + roleOrderDenseAdmins),
		}},
	})

	if got := m.rolePositions()[roleOrderDenseMembers]; got != 1 {
		t.Errorf("Members moved to position %d, want it left on 1", got)
	}
}
