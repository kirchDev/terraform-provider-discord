package provider

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
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

	// The guild #55 reports: the app's own role at the top, a second bot role
	// below it, and two ordinary roles the configuration wants swapped. The ids
	// are ordered oldest first, because roles sharing a position are ranked by
	// snowflake — the app role has to be the oldest of a tie to sit on top of it.
	roleOrderSwapGuildID = "900000000000000005"
	roleOrderSwapBot     = "900000000000000510"
	roleOrderSwapMEE6    = "900000000000000520"
	roleOrderSwapHelpers = "900000000000000530"
	roleOrderSwapMembers = "900000000000000540"
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

// TestAccRoleOrderResource_swapsBelowTheAppRole is the reported repro in the shape
// where the order is reachable: the app's own role on top, a second bot role under
// it, and two ordinary roles the configuration wants the other way round. The swap
// happens in the slots those two already hold, so nothing is written at or above
// the app's role and Discord's hierarchy rule is never engaged.
func TestAccRoleOrderResource_swapsBelowTheAppRole(t *testing.T) {
	m := newMockDiscord(t)
	m.seedRole(roleOrderSwapGuildID, "@everyone", 0, false)
	m.seedRole(roleOrderSwapMembers, "Members", 1, false)
	m.seedRole(roleOrderSwapHelpers, "Helpers", 2, false)
	m.seedRole(roleOrderSwapMEE6, "MEE6", 3, true)
	m.seedBotRole(roleOrderSwapBot, "kirchbot", 4)

	cfg := fmt.Sprintf(`
resource "discord_role_order" "test" {
  server_id = %q
  role_ids  = [%q, %q]
}
`, roleOrderSwapGuildID, roleOrderSwapMembers, roleOrderSwapHelpers)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{{
			Config: cfg,
			Check: resource.ComposeAggregateTestCheckFunc(
				checkOrder(m.rolePositions, true, roleOrderSwapMembers, roleOrderSwapHelpers),
				checkAtSlot(m.rolePositions, roleOrderSwapMembers, 2),
				checkAtSlot(m.rolePositions, roleOrderSwapHelpers, 1),
				// Neither bot role was touched: the swap stayed inside the two slots
				// the listed roles already held.
				checkAtSlot(m.rolePositions, roleOrderSwapMEE6, 3),
				checkAtSlot(m.rolePositions, roleOrderSwapBot, 4),
				checkNoSharedSlot(m.rolePositions),
			),
		}},
	})
}

// TestAccRoleOrderResource_skipsAPatchThatChangesNothing pins the other half of
// the same guild: with the hierarchy already in the configured order there is
// nothing to sort, so no request is made at all. Discord only ever refuses a
// request on what it would move, which is why the resource could reorder these
// guilds for as long as the order happened to be right already — and why the
// no-op is worth not sending.
func TestAccRoleOrderResource_skipsAPatchThatChangesNothing(t *testing.T) {
	m := newMockDiscord(t)
	m.seedRole(roleOrderSwapGuildID, "@everyone", 0, false)
	m.seedRole(roleOrderSwapHelpers, "Helpers", 1, false)
	m.seedRole(roleOrderSwapMembers, "Members", 2, false)
	m.seedRole(roleOrderSwapMEE6, "MEE6", 3, true)
	m.seedBotRole(roleOrderSwapBot, "kirchbot", 4)

	cfg := fmt.Sprintf(`
resource "discord_role_order" "test" {
  server_id = %q
  role_ids  = [%q, %q]
}
`, roleOrderSwapGuildID, roleOrderSwapMembers, roleOrderSwapHelpers)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{{
			Config: cfg,
			Check: resource.ComposeAggregateTestCheckFunc(
				checkOrder(m.rolePositions, true, roleOrderSwapMembers, roleOrderSwapHelpers),
				func(*terraform.State) error {
					if got := m.rolePatchCount(); got != 0 {
						return fmt.Errorf("modify-role-positions was called %d time(s), want none", got)
					}
					return nil
				},
			),
		}},
	})
}

// TestAccRoleOrderResource_refusesASwapWithNoRoomUnderTheAppRole is #55 itself.
// Discord creates every role on position 1 without renumbering, so a guild built
// entirely through the API has its whole hierarchy on that one position, ranked by
// snowflake — the app's role on top because it is the oldest. The app may sort only
// roles lower than its own highest role, and only to positions below it, so with
// that role on position 1 there is no position left to sort anything into: every
// order but the one already in place is unreachable through the API.
//
// The resource used to compute positions above the app's role and let Discord
// answer the whole request with a bare 50013, which is what made the resource
// unusable — any actual move failed, and only a hierarchy that needed no move
// applied. It now says which role closes the range and what has to change.
func TestAccRoleOrderResource_refusesASwapWithNoRoomUnderTheAppRole(t *testing.T) {
	m := newMockDiscord(t)
	m.seedRole(roleOrderSwapGuildID, "@everyone", 0, false)
	m.seedBotRole(roleOrderSwapBot, "kirchbot", 1)
	m.seedRole(roleOrderSwapMEE6, "MEE6", 1, true)
	m.seedRole(roleOrderSwapHelpers, "Helpers", 1, false)
	m.seedRole(roleOrderSwapMembers, "Members", 1, false)

	// Members is the youngest, so it ranks lowest of the tie: the live order is
	// Helpers above Members, and the configuration asks for the opposite.
	cfg := fmt.Sprintf(`
resource "discord_role_order" "test" {
  server_id = %q
  role_ids  = [%q, %q]
}
`, roleOrderSwapGuildID, roleOrderSwapMembers, roleOrderSwapHelpers)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{{
			Config:      cfg,
			ExpectError: regexp.MustCompile(`(?s)kirchbot.*this\s+app's\s+own\s+role.*only\s+0\s+are\s+free\s+below\s+position\s+1`),
		}},
	})

	// Nothing was sent: a request Discord would refuse is never made, so the bare
	// 50013 has no occasion to reach the operator.
	if got := m.rolePatchCount(); got != 0 {
		t.Errorf("modify-role-positions was called %d time(s), want none", got)
	}
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
// dense range, including the positions unlisted roles stand on, and Discord's own
// re-sort bumps those roles up. What is relaxed is their *absolute* position;
// their place relative to every listed role is unchanged, which the resource
// checks before the write and against the read-back after it.
//
// The app's own role is the one exception to what may be written over: Discord
// refuses a position at or above it outright, so the renumbering only has this
// way out while the roles fit *below* that role. Here they do — the position the
// plan writes over belongs to another integration's role, not to the app's.
func TestAccRoleOrderResource_placesAFreshRoleInADenseHierarchy(t *testing.T) {
	m := newMockDiscord(t)
	m.seedRole(roleOrderDenseGuildID, "@everyone", 0, false)
	m.seedRole(roleOrderDenseMembers, "Members", 1, false)
	m.seedRole(roleOrderDenseMods, "Mods", 2, false)
	m.seedRole(roleOrderDenseAdmins, "Admins", 3, false)
	m.seedRole(roleOrderDenseBooster, "Server Booster", 4, true)
	m.seedBotRole(roleOrderDenseBot, "kirchbot", 5)
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
				// Neither unlisted role was in the body — Discord's re-sort moved
				// them, and both are still above every listed role.
				checkAtSlot(m.rolePositions, roleOrderDenseBooster, 5),
				checkAtSlot(m.rolePositions, roleOrderDenseBot, 6),
				checkNoSharedSlot(m.rolePositions),
			),
		}},
	})
}

// TestAccRoleOrderResource_refusesToRenumberPastTheAppRole is the same dense
// hierarchy with the app's own role one position lower, which is what turns the
// renumbering from a way out into a request Discord refuses: four roles need four
// distinct positions and only positions 1 to 3 lie below the app's role. The
// resource used to plan a write onto that role's own position and rely on
// Discord's re-sort to bump it, which Discord answers with the bare 50013 of #55
// — a user may sort only roles lower than its highest role, and only to positions
// below it. It now names the role that closes the range and writes nothing.
func TestAccRoleOrderResource_refusesToRenumberPastTheAppRole(t *testing.T) {
	m := newMockDiscord(t)
	m.seedRole(roleOrderDenseGuildID, "@everyone", 0, false)
	m.seedRole(roleOrderDenseMembers, "Members", 1, false)
	m.seedRole(roleOrderDenseMods, "Mods", 2, false)
	m.seedRole(roleOrderDenseAdmins, "Admins", 3, false)
	m.seedBotRole(roleOrderDenseBot, "kirchbot", 4)
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
			ExpectError: regexp.MustCompile(
				`(?s)kirchbot.*this\s+app's\s+own\s+role.*need\s+4\s+distinct\s+positions\s+and\s+only\s+3\s+are\s+free\s+below\s+position\s+4`),
		}},
	})

	if got := m.rolePatchCount(); got != 0 {
		t.Errorf("modify-role-positions was called %d time(s), want none", got)
	}
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
