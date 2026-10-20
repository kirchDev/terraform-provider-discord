package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

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
