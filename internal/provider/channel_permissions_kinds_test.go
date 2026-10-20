package provider

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// Each kind wires the sync conflict validator itself; a dropped ConfigValidators method would otherwise go unnoticed.
func TestAccChannelPermissions_SyncConflictAllKinds(t *testing.T) {
	newMockDiscord(t)
	for _, kind := range []string{"text", "voice", "news", "stage", "forum", "media"} {
		t.Run(kind, func(t *testing.T) {
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
				Steps: []resource.TestStep{{
					Config: `
resource "discord_` + kind + `_channel" "test" {
  server_id                = "999"
  name                     = "private"
  sync_perms_with_category = true
  permissions              = { role_overwrites = { "1" = { allow = ["view_channel"] } } }
}
`,
					PlanOnly:    true,
					ExpectError: regexp.MustCompile(`Conflicting attributes`),
				}},
			})
		})
	}
}

// Unknown allow/allow_bits values (not whole maps) must pass validation and be recomputed on apply.
func TestAccTextChannelPermissions_UnknownElementAtPlan(t *testing.T) {
	m := newMockDiscord(t)
	cfg := `
resource "terraform_data" "key" {
  input = "view_channel"
}

resource "terraform_data" "bits" {
  input = "2048"
}

resource "discord_text_channel" "test" {
  server_id = "999"
  name      = "private"
  permissions = {
    role_overwrites = {
      "300" = { allow = [terraform_data.key.output] }
    }
    user_overwrites = {
      "400" = { deny_bits = terraform_data.bits.output }
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
					return checkOverwrites(m, permsChannel,
						permissionOverwrite{ID: "300", Type: 0, Allow: "1024", Deny: "0"},
						permissionOverwrite{ID: "400", Type: 1, Allow: "0", Deny: "2048"},
					)(s)
				},
			},
			{Config: cfg, PlanOnly: true},
		},
	})
}

// Kinds sharing one create-then-drift shape; text and category have their own deeper tests.
func TestAccChannelPermissions_CreateAndDriftOtherKinds(t *testing.T) {
	for _, kind := range []string{"voice", "news", "stage", "forum", "media"} {
		t.Run(kind, func(t *testing.T) {
			m := newMockDiscord(t)
			rn := "discord_" + kind + "_channel.test"
			cfg := `
resource "discord_` + kind + `_channel" "test" {
  server_id   = "999"
  name        = "private"
  permissions = { role_overwrites = { "300" = { allow = ["view_channel"] } } }
}
`
			want := permissionOverwrite{ID: "300", Type: 0, Allow: "1024", Deny: "0"}
			resource.Test(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
				Steps: []resource.TestStep{
					{
						Config: cfg,
						Check: resource.ComposeAggregateTestCheckFunc(
							resource.TestCheckResourceAttr(rn, "permissions.role_overwrites.300.allow_bits", "1024"),
							checkOverwrites(m, rn, want),
							func(*terraform.State) error {
								if _, ok := m.lastChannelPost()["permission_overwrites"]; !ok {
									return fmt.Errorf("create POST carried no permission_overwrites: %v", m.lastChannelPost())
								}
								return nil
							},
						),
					},
					{
						PreConfig: seedFromState(m, permissionOverwrite{ID: "300", Type: 0, Allow: "1025", Deny: "0"},
							permissionOverwrite{ID: "600", Type: 1, Allow: "0", Deny: "2048"}),
						Config:             cfg,
						PlanOnly:           true,
						ExpectNonEmptyPlan: true,
					},
					{Config: cfg, Check: checkOverwrites(m, rn, want)},
				},
			})
		})
	}
}
