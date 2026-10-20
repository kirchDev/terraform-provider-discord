package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func seedPermsChannels(m *mockDiscord) {
	m.seedChannel("10", "999", "locked", "", 0)
	m.seedOverwrites("10",
		permissionOverwrite{ID: "200", Type: 0, Allow: "1024", Deny: "2048"},
		permissionOverwrite{ID: "400", Type: 1, Allow: "0", Deny: "1024"},
	)
	m.seedChannel("11", "999", "open", "", 1)
}

func TestAccChannelDataSource_Permissions(t *testing.T) {
	m := newMockDiscord(t)
	seedPermsChannels(m)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{{
			Config: `
data "discord_channel" "locked" { channel_id = "10" }
data "discord_channel" "open" { channel_id = "11" }
`,
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("data.discord_channel.locked", "permissions.role_overwrites.200.allow_bits", "1024"),
				resource.TestCheckResourceAttr("data.discord_channel.locked", "permissions.role_overwrites.200.allow.0", "view_channel"),
				resource.TestCheckResourceAttr("data.discord_channel.locked", "permissions.role_overwrites.200.deny.0", "send_messages"),
				resource.TestCheckResourceAttr("data.discord_channel.locked", "permissions.user_overwrites.400.deny_bits", "1024"),
				resource.TestCheckResourceAttr("data.discord_channel.open", "permissions.role_overwrites.%", "0"),
				resource.TestCheckResourceAttr("data.discord_channel.open", "permissions.user_overwrites.%", "0"),
			),
		}},
	})
}

func TestAccChannelsDataSource_Permissions(t *testing.T) {
	m := newMockDiscord(t)
	seedPermsChannels(m)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{{
			Config: `data "discord_channels" "all" { server_id = "999" }`,
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("data.discord_channels.all", "channels.#", "2"),
				// list order is the mock's map order, so match by content
				resource.TestCheckTypeSetElemNestedAttrs("data.discord_channels.all", "channels.*", map[string]string{
					"name": "locked",
					"permissions.role_overwrites.200.allow_bits": "1024",
					"permissions.user_overwrites.400.deny_bits":  "1024",
				}),
				resource.TestCheckTypeSetElemNestedAttrs("data.discord_channels.all", "channels.*", map[string]string{
					"name":                          "open",
					"permissions.role_overwrites.%": "0",
				}),
			),
		}},
	})
}
