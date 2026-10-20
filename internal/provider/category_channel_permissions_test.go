package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

const catPermsChannel = "discord_category_channel.test"

func catPermsCfg(permissions string) string {
	return fmt.Sprintf(`
resource "discord_category_channel" "test" {
  server_id = "999"
  name      = "private"
  %s
}
`, permissions)
}

func TestAccCategoryChannelPermissions_CreateUpdate(t *testing.T) {
	m := newMockDiscord(t)
	const (
		role = "permissions.role_overwrites.300"
		user = "permissions.user_overwrites.400"
	)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{ // create: overwrites travel in the POST body, never a follow-up write
				Config: catPermsCfg(`permissions = {
    role_overwrites = {
      "300" = { allow = ["view_channel"], deny = ["send_messages"] }
      "200" = { deny_bits = "1024" }
    }
    user_overwrites = { "400" = { allow = ["manage_messages"] } }
  }`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(catPermsChannel, role+".allow.#", "1"),
					resource.TestCheckResourceAttr(catPermsChannel, role+".allow_bits", "1024"),
					resource.TestCheckResourceAttr(catPermsChannel, role+".deny_bits", "2048"),
					resource.TestCheckResourceAttr(catPermsChannel, "permissions.role_overwrites.200.deny.#", "1"),
					resource.TestCheckResourceAttr(catPermsChannel, "permissions.role_overwrites.200.deny.0", "view_channel"),
					resource.TestCheckResourceAttr(catPermsChannel, "permissions.role_overwrites.200.allow_bits", "0"),
					resource.TestCheckResourceAttr(catPermsChannel, user+".allow_bits", "8192"),
					checkOverwrites(m, catPermsChannel,
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
				Config: catPermsCfg(`permissions = {
    role_overwrites = {
      "300" = { allow = ["view_channel"], deny = ["send_messages"] }
      "200" = { deny_bits = "1024" }
    }
    user_overwrites = { "400" = { allow = ["manage_messages"] } }
  }`),
				PlanOnly: true,
			},
			{ // update: change one, remove one, add one, switch a form
				Config: catPermsCfg(`permissions = {
    role_overwrites = {
      "300" = { allow_bits = "3072" }
      "500" = { deny = ["connect"] }
    }
  }`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(catPermsChannel, role+".allow.#", "2"),
					resource.TestCheckResourceAttr(catPermsChannel, role+".deny.#", "0"),
					resource.TestCheckResourceAttr(catPermsChannel, "permissions.user_overwrites.%", "0"),
					resource.TestCheckNoResourceAttr(catPermsChannel, "permissions.role_overwrites.200.allow_bits"),
					checkOverwrites(m, catPermsChannel,
						permissionOverwrite{ID: "300", Type: 0, Allow: "3072", Deny: "0"},
						permissionOverwrite{ID: "500", Type: 0, Allow: "0", Deny: "1048576"},
					),
				),
			},
			{ // permissions = {} declares "no overwrites" and clears them all
				Config: catPermsCfg(`permissions = {}`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(catPermsChannel, "permissions.role_overwrites.%", "0"),
					checkOverwrites(m, catPermsChannel),
				),
			},
		},
	})
}

func TestAccCategoryChannelPermissions_Drift(t *testing.T) {
	m := newMockDiscord(t)
	cfg := catPermsCfg(`permissions = { role_overwrites = { "300" = { allow = ["view_channel"] } } }`)

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
				Check: checkOverwrites(m, catPermsChannel,
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

func TestAccCategoryChannelPermissions_UnknownBitRoundTrip(t *testing.T) {
	m := newMockDiscord(t)
	const entry = "permissions.role_overwrites.300"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{ // 1<<47 has no key; keys and bits forms are interchangeable
				Config: catPermsCfg(`permissions = { role_overwrites = { "300" = { allow = ["unknown_47", "view_channel"] } } }`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(catPermsChannel, entry+".allow_bits", "140737488356352"),
					checkOverwrites(m, catPermsChannel, permissionOverwrite{ID: "300", Type: 0, Allow: "140737488356352", Deny: "0"}),
				),
			},
			{
				Config:   catPermsCfg(`permissions = { role_overwrites = { "300" = { allow_bits = "140737488356352" } } }`),
				PlanOnly: true,
			},
			{ // a bit set out of band decodes to unknown_<n> in the plan, and applies back
				PreConfig: seedFromState(m,
					permissionOverwrite{ID: "300", Type: 0, Allow: "0", Deny: "9223372036854775808"},
				),
				Config:             catPermsCfg(`permissions = { role_overwrites = { "300" = { allow_bits = "1" } } }`),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: catPermsCfg(`permissions = { role_overwrites = { "300" = { deny = ["unknown_63"] } } }`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(catPermsChannel, entry+".deny.0", "unknown_63"),
					resource.TestCheckResourceAttr(catPermsChannel, entry+".deny_bits", "9223372036854775808"),
				),
			},
		},
	})
}

func TestAccCategoryChannelPermissions_NullLeavesOverwritesAlone(t *testing.T) {
	m := newMockDiscord(t)
	seeded := permissionOverwrite{ID: "300", Type: 0, Allow: "1024", Deny: "0"}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: catPermsCfg(""),
				Check:  resource.TestCheckNoResourceAttr(catPermsChannel, "permissions.%"),
			},
			{ // out-of-band overwrites are neither drift nor touched by an unrelated update
				PreConfig: seedFromState(m, seeded),
				Config:    catPermsCfg("position = 3"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr(catPermsChannel, "permissions.%"),
					checkOverwrites(m, catPermsChannel, seeded),
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
				Config: catPermsCfg(`position = 3
  permissions = { user_overwrites = { "400" = { allow = ["manage_messages"] } } }`),
				Check: checkOverwrites(m, catPermsChannel, permissionOverwrite{ID: "400", Type: 1, Allow: "8192", Deny: "0"}),
			},
			{ // dropping the attribute stops managing without clearing
				Config: catPermsCfg(`position = 3`),
				Check:  checkOverwrites(m, catPermsChannel, permissionOverwrite{ID: "400", Type: 1, Allow: "8192", Deny: "0"}),
			},
			{ // import never populates it
				ResourceName:      catPermsChannel,
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: importIDFunc(catPermsChannel, "id"),
			},
		},
	})
}
