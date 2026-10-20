package provider

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccTextChannelPermissions_CreateUpdate(t *testing.T) {
	m := newMockDiscord(t)
	const (
		role = "permissions.role_overwrites.300"
		user = "permissions.user_overwrites.400"
	)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{ // create: overwrites travel in the POST body, never a follow-up write
				Config: permsCfg(`permissions = {
    role_overwrites = {
      "300" = { allow = ["view_channel"], deny = ["send_messages"] }
      "200" = { deny_bits = "1024" }
    }
    user_overwrites = { "400" = { allow = ["manage_messages"], allow_bits = "8192" } }
  }`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(permsChannel, role+".allow.#", "1"),
					resource.TestCheckResourceAttr(permsChannel, role+".allow_bits", "1024"),
					resource.TestCheckResourceAttr(permsChannel, role+".deny_bits", "2048"),
					resource.TestCheckResourceAttr(permsChannel, "permissions.role_overwrites.200.deny.#", "1"),
					resource.TestCheckResourceAttr(permsChannel, "permissions.role_overwrites.200.deny.0", "view_channel"),
					resource.TestCheckResourceAttr(permsChannel, "permissions.role_overwrites.200.allow_bits", "0"),
					resource.TestCheckResourceAttr(permsChannel, user+".allow_bits", "8192"),
					checkOverwrites(m, permsChannel,
						permissionOverwrite{ID: "200", Type: 0, Allow: "0", Deny: "1024"},
						permissionOverwrite{ID: "300", Type: 0, Allow: "1024", Deny: "2048"},
						permissionOverwrite{ID: "400", Type: 1, Allow: "8192", Deny: "0"},
					),
					func(*terraform.State) error {
						if _, ok := m.lastChannelPost()["permission_overwrites"]; !ok {
							return fmt.Errorf("create POST body carried no permission_overwrites: %v", m.lastChannelPost())
						}
						if n := len(m.channelPatches[m.channelIDByName("private")]); n != 0 {
							return fmt.Errorf("create followed up with %d PATCH requests", n)
						}
						return nil
					},
				),
			},
			{ // idempotent
				Config: permsCfg(`permissions = {
    role_overwrites = {
      "300" = { allow = ["view_channel"], deny = ["send_messages"] }
      "200" = { deny_bits = "1024" }
    }
    user_overwrites = { "400" = { allow = ["manage_messages"], allow_bits = "8192" } }
  }`),
				PlanOnly: true,
			},
			{ // update: change one, remove one, add one, switch a form
				Config: permsCfg(`permissions = {
    role_overwrites = {
      "300" = { allow_bits = "3072" }
      "500" = { deny = ["connect"] }
    }
  }`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(permsChannel, role+".allow.#", "2"),
					resource.TestCheckResourceAttr(permsChannel, role+".deny.#", "0"),
					resource.TestCheckResourceAttr(permsChannel, "permissions.user_overwrites.%", "0"),
					resource.TestCheckNoResourceAttr(permsChannel, "permissions.role_overwrites.200.allow_bits"),
					checkOverwrites(m, permsChannel,
						permissionOverwrite{ID: "300", Type: 0, Allow: "3072", Deny: "0"},
						permissionOverwrite{ID: "500", Type: 0, Allow: "0", Deny: "1048576"},
					),
				),
			},
			{ // permissions = {} declares "no overwrites" and clears them all
				Config: permsCfg(`permissions = {}`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(permsChannel, "permissions.role_overwrites.%", "0"),
					checkOverwrites(m, permsChannel),
				),
			},
		},
	})
}

func TestAccTextChannelPermissions_Drift(t *testing.T) {
	m := newMockDiscord(t)
	cfg := permsCfg(`permissions = { role_overwrites = { "300" = { allow = ["view_channel"] } } }`)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{Config: cfg},
			{ // an overwrite added in the Discord UI
				PreConfig: seedFromState(m,
					permissionOverwrite{ID: "300", Type: 0, Allow: "1024", Deny: "0"},
					permissionOverwrite{ID: "600", Type: 1, Allow: "0", Deny: "2048"},
				),
				Config:             cfg,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{ // apply removes it
				Config: cfg,
				Check: checkOverwrites(m, permsChannel,
					permissionOverwrite{ID: "300", Type: 0, Allow: "1024", Deny: "0"},
				),
			},
			{ // a changed overwrite
				PreConfig: seedFromState(m,
					permissionOverwrite{ID: "300", Type: 0, Allow: "1025", Deny: "0"},
				),
				Config:             cfg,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

func TestAccTextChannelPermissions_UnknownBitRoundTrip(t *testing.T) {
	m := newMockDiscord(t)
	const entry = "permissions.role_overwrites.300"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{ // 1<<47 has no key; keys and bits forms are interchangeable
				Config: permsCfg(`permissions = { role_overwrites = { "300" = { allow = ["unknown_47", "view_channel"] } } }`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(permsChannel, entry+".allow_bits", "140737488356352"),
					checkOverwrites(m, permsChannel, permissionOverwrite{ID: "300", Type: 0, Allow: "140737488356352", Deny: "0"}),
				),
			},
			{
				Config:   permsCfg(`permissions = { role_overwrites = { "300" = { allow_bits = "140737488356352" } } }`),
				PlanOnly: true,
			},
			{ // a bit set out of band decodes to unknown_<n> in the plan, and applies back
				PreConfig: seedFromState(m,
					permissionOverwrite{ID: "300", Type: 0, Allow: "0", Deny: "9223372036854775808"},
				),
				Config:             permsCfg(`permissions = { role_overwrites = { "300" = { allow_bits = "1" } } }`),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: permsCfg(`permissions = { role_overwrites = { "300" = { deny = ["unknown_63"] } } }`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(permsChannel, entry+".deny.0", "unknown_63"),
					resource.TestCheckResourceAttr(permsChannel, entry+".deny_bits", "9223372036854775808"),
				),
			},
		},
	})
}

func TestAccTextChannelPermissions_NullLeavesOverwritesAlone(t *testing.T) {
	m := newMockDiscord(t)
	seeded := permissionOverwrite{ID: "300", Type: 0, Allow: "1024", Deny: "0"}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: permsCfg(""),
				Check:  resource.TestCheckNoResourceAttr(permsChannel, "permissions.%"),
			},
			{ // out-of-band overwrites are neither drift nor touched by an unrelated update
				PreConfig: seedFromState(m, seeded),
				Config:    permsCfg("topic = \"changed\""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr(permsChannel, "permissions.%"),
					checkOverwrites(m, permsChannel, seeded),
					func(*terraform.State) error {
						for _, p := range m.channelPatches[m.channelIDByName("private")] {
							if _, ok := p["permission_overwrites"]; ok {
								return fmt.Errorf("PATCH sent permission_overwrites with permissions unmanaged: %v", p)
							}
						}
						return nil
					},
				),
			},
			{ // adopting management replaces them
				Config: permsCfg(`topic = "changed"
  permissions = { user_overwrites = { "400" = { allow = ["manage_messages"] } } }`),
				Check: checkOverwrites(m, permsChannel, permissionOverwrite{ID: "400", Type: 1, Allow: "8192", Deny: "0"}),
			},
			{ // dropping the attribute stops managing without clearing
				Config: permsCfg(`topic = "changed"`),
				Check:  checkOverwrites(m, permsChannel, permissionOverwrite{ID: "400", Type: 1, Allow: "8192", Deny: "0"}),
			},
			{ // import never populates it
				ResourceName:            permsChannel,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateIdFunc:       importIDFunc(permsChannel, "id"),
				ImportStateVerifyIgnore: []string{"sync_perms_with_category"},
			},
		},
	})
}

func TestAccTextChannelPermissions_Validation(t *testing.T) {
	newMockDiscord(t)
	cases := map[string]struct {
		cfg string
		re  string
	}{
		"allow and allow_bits disagree": {
			`permissions = { role_overwrites = { "1" = { allow = ["view_channel"], allow_bits = "2048" } } }`,
			`same permissions`,
		},
		"deny and deny_bits disagree": {
			`permissions = { user_overwrites = { "1" = { allow_bits = "1", deny = ["view_channel"], deny_bits = "2048" } } }`,
			`same permissions`,
		},
		"both sides empty": {
			`permissions = { role_overwrites = { "1" = { allow = [] } } }`,
			`Empty permission overwrite`,
		},
		"zero bits": {
			`permissions = { role_overwrites = { "1" = { allow_bits = "0", deny_bits = "0" } } }`,
			`Empty permission overwrite`,
		},
		"same bit both sides": {
			`permissions = { role_overwrites = { "1" = { allow = ["view_channel"], deny_bits = "1024" } } }`,
			`Conflicting permission overwrite`,
		},
		"unrecognized key": {
			`permissions = { role_overwrites = { "1" = { allow = ["nope"] } } }`,
			`unrecognized`,
		},
		"unknown_n out of range": {
			`permissions = { role_overwrites = { "1" = { allow = ["unknown_64"] } } }`,
			`canonical decimal`,
		},
		"unknown_n not canonical": {
			`permissions = { role_overwrites = { "1" = { allow = ["unknown_047"] } } }`,
			`canonical decimal`,
		},
		"unknown_n for a named bit": {
			`permissions = { role_overwrites = { "1" = { allow = ["unknown_10"] } } }`,
			`named`,
		},
		"non-canonical bits": {
			`permissions = { role_overwrites = { "1" = { allow_bits = "01024" } } }`,
			`Invalid permission bitfield`,
		},
		"bits overflow": {
			`permissions = { role_overwrites = { "1" = { allow_bits = "18446744073709551616" } } }`,
			`Invalid permission bitfield`,
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
				Steps: []resource.TestStep{{
					Config:      permsCfg(c.cfg),
					PlanOnly:    true,
					ExpectError: regexp.MustCompile(c.re),
				}},
			})
		})
	}
}

// A role created in the same apply makes the overwrites map unknown at plan time;
// every validator and modifier must let it through.
func TestAccTextChannelPermissions_UnknownAtPlan(t *testing.T) {
	m := newMockDiscord(t)
	cfg := `
resource "discord_role" "mod" {
  server_id = "999"
  name      = "mod"
}

resource "discord_text_channel" "test" {
  server_id = "999"
  name      = "private"
  permissions = {
    role_overwrites = {
      (discord_role.mod.id) = { allow = ["view_channel"], deny = ["send_messages"] }
    }
  }
}
`
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check: func(s *terraform.State) error {
					roleID := s.RootModule().Resources["discord_role.mod"].Primary.ID
					return resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttr(permsChannel, "permissions.role_overwrites."+roleID+".allow_bits", "1024"),
						checkOverwrites(m, permsChannel, permissionOverwrite{ID: roleID, Type: 0, Allow: "1024", Deny: "2048"}),
					)(s)
				},
			},
			{Config: cfg, PlanOnly: true},
		},
	})
}
