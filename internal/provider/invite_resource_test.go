package provider

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

const (
	inviteGuildID   = "910000000000000001"
	inviteChannelID = "910000000000000002"
	inviteMinecraft = "910000000000000010"
	inviteGamers    = "910000000000000020"
	inviteBot       = "910000000000000030"
	inviteAdmins    = "910000000000000040"
	inviteBooster   = "910000000000000050"
	inviteUnknown   = "910000000000000099"
)

// seedInviteGuild plants a guild with one channel and a hierarchy where the app's
// own role sits between the grantable roles and one above it.
func seedInviteGuild(m *mockDiscord) {
	m.seedChannel(inviteChannelID, inviteGuildID, "welcome", "", 0)
	m.seedRole(inviteGuildID, "@everyone", 0, false)
	m.seedRole(inviteMinecraft, "Minecraft Server", 1, false)
	m.seedRole(inviteGamers, "Gamers", 2, false)
	m.seedRole(inviteBooster, "Server Booster", 3, true)
	m.seedBotRole(inviteBot, "kirchbot", 4)
	m.seedRole(inviteAdmins, "Admins", 5, false)
}

func inviteConfig(channelID string, roleIDs ...string) string {
	roles := ""
	for i, id := range roleIDs {
		if i > 0 {
			roles += ", "
		}
		roles += fmt.Sprintf("%q", id)
	}
	return fmt.Sprintf(`
resource "discord_invite" "test" {
  channel_id = %q
  max_age    = 0
  role_ids   = [%s]
}
`, channelID, roles)
}

// TestAccInviteResource_grantsRoles is the consumer's case: a permanent invite
// that hands out an opt-in role. The role ids reach Discord on creation, come
// back from GET /invites/{code} on refresh and import, and a change to the set is
// a new invite, since invites are immutable.
func TestAccInviteResource_grantsRoles(t *testing.T) {
	m := newMockDiscord(t)
	seedInviteGuild(m)

	const rn = "discord_invite.test"
	var firstCode string
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: inviteConfig(inviteChannelID, inviteMinecraft),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(rn, "role_ids.#", "1"),
					resource.TestCheckTypeSetElemAttr(rn, "role_ids.*", inviteMinecraft),
					captureAttr(rn, "code", &firstCode),
					checkInviteRoles(m, rn, inviteMinecraft),
				),
			},
			{
				ResourceName:                         rn,
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "code",
				ImportStateIdFunc:                    importIDFunc(rn, "code"),
				// GET /invites/{code} omits the invite metadata; those stay
				// create-time values and cannot be imported.
				ImportStateVerifyIgnore: []string{"max_age", "max_uses", "temporary", "unique"},
			},
			{
				Config: inviteConfig(inviteChannelID, inviteMinecraft, inviteGamers),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(rn, "role_ids.#", "2"),
					checkInviteRoles(m, rn, inviteMinecraft, inviteGamers),
					checkAttrChanged(rn, "code", &firstCode),
				),
			},
		},
	})
}

// TestAccInviteResource_detectsRoleDrift pins the refresh: a granted role set
// changed out of band shows up as a diff instead of going unseen.
func TestAccInviteResource_detectsRoleDrift(t *testing.T) {
	m := newMockDiscord(t)
	seedInviteGuild(m)

	const rn = "discord_invite.test"
	var code string
	cfg := inviteConfig(inviteChannelID, inviteMinecraft)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{Config: cfg, Check: captureAttr(rn, "code", &code)},
			{
				PreConfig:          func() { m.setInviteRoles(code, inviteGamers) },
				Config:             cfg,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

// TestAccInviteResource_withoutRoles keeps an invite that grants nothing free of
// a spurious diff: Discord answers an empty `roles` array for it.
func TestAccInviteResource_withoutRoles(t *testing.T) {
	m := newMockDiscord(t)
	seedInviteGuild(m)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{{
			Config: fmt.Sprintf(`
resource "discord_invite" "test" {
  channel_id = %q
}
`, inviteChannelID),
			Check: resource.TestCheckNoResourceAttr("discord_invite.test", "role_ids.#"),
		}},
	})
}

// TestAccInviteDataSource_roleIDs reads the granted roles through the data source.
func TestAccInviteDataSource_roleIDs(t *testing.T) {
	m := newMockDiscord(t)
	seedInviteGuild(m)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{{
			Config: inviteConfig(inviteChannelID, inviteMinecraft, inviteGamers) + `
data "discord_invite" "test" {
  code = discord_invite.test.code
}
`,
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("data.discord_invite.test", "role_ids.#", "2"),
				resource.TestCheckTypeSetElemAttr("data.discord_invite.test", "role_ids.*", inviteMinecraft),
				resource.TestCheckTypeSetElemAttr("data.discord_invite.test", "role_ids.*", inviteGamers),
				resource.TestCheckResourceAttr("data.discord_invite.test", "guild_id", inviteGuildID),
			),
		}},
	})
}

// TestAccInviteResource_refusesRolesItCannotGrant covers the rejections the
// provider catches before the write, following discord_role_order: a role that
// does not exist, one managed by an integration, and one not below the app's own
// highest role. Each fails naming the role, and no invite is created.
func TestAccInviteResource_refusesRolesItCannotGrant(t *testing.T) {
	cases := []struct {
		name string
		role string
		want *regexp.Regexp
	}{
		{"unknown", inviteUnknown, regexp.MustCompile(`no role ` + inviteUnknown)},
		{"managed", inviteBooster, regexp.MustCompile(`Server Booster`)},
		{"above the app role", inviteAdmins, regexp.MustCompile(`Admins[\s\S]*kirchbot`)},
		{"the app role itself", inviteBot, regexp.MustCompile(`kirchbot`)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newMockDiscord(t)
			seedInviteGuild(m)
			resource.Test(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
				Steps: []resource.TestStep{{
					Config:      inviteConfig(inviteChannelID, inviteMinecraft, tc.role),
					ExpectError: tc.want,
				}},
			})
			if n := m.invitePostCount(); n != 0 {
				t.Fatalf("create-invite requests = %d, want 0: the refusal must come before the write", n)
			}
		})
	}
}

// TestAccInviteResource_explainsMissingPermissions covers the best-effort half:
// where the hierarchy cannot be read (here the channel is unknown to the role
// lookup), the invite is sent anyway, and a 50013 Discord answers is reported
// as the permissions it points at rather than as a bare API error.
func TestAccInviteResource_explainsMissingPermissions(t *testing.T) {
	m := newMockDiscord(t)
	seedInviteGuild(m)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{{
			Config:      inviteConfig("910000000000000003", inviteAdmins),
			ExpectError: regexp.MustCompile(`Manage Roles[\s\S]*Create Instant Invite`),
		}},
	})
	if n := m.invitePostCount(); n != 1 {
		t.Fatalf("create-invite requests = %d, want 1: a failed lookup must not stop the write", n)
	}
}

// checkInviteRoles asserts the roles the mock actually stored for the invite in
// state, not what the provider put in state.
func checkInviteRoles(m *mockDiscord, rn string, want ...string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[rn]
		if !ok {
			return fmt.Errorf("resource %s not found in state", rn)
		}
		m.mu.Lock()
		defer m.mu.Unlock()
		inv, ok := m.invites[rs.Primary.Attributes["code"]]
		if !ok {
			return fmt.Errorf("invite %s was not created", rs.Primary.Attributes["code"])
		}
		got, _ := inv["role_ids"].([]any)
		if len(got) != len(want) {
			return fmt.Errorf("invite role_ids = %v, want %v", got, want)
		}
		held := map[any]bool{}
		for _, id := range got {
			held[id] = true
		}
		for _, id := range want {
			if !held[id] {
				return fmt.Errorf("invite role_ids = %v, missing %s", got, id)
			}
		}
		return nil
	}
}

// checkAttrChanged asserts an attribute no longer equals a value captured earlier.
func checkAttrChanged(rn, key string, before *string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[rn]
		if !ok {
			return fmt.Errorf("resource %s not found in state", rn)
		}
		if got := rs.Primary.Attributes[key]; got == *before {
			return fmt.Errorf("%s = %q, want a new value after replacement", key, got)
		}
		return nil
	}
}
