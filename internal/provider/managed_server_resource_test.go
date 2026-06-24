package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccManagedServerResource covers the manage-not-create pattern: Create
// adopts the live guild via PATCH (never POSTs), computed fields come back from
// the read, and import is by guild id.
func TestAccManagedServerResource(t *testing.T) {
	newMockDiscord(t)
	const rn = "discord_managed_server.test"

	cfg := func(name string, vlevel int) string {
		return fmt.Sprintf(`
resource "discord_managed_server" "test" {
  server_id          = "999"
  name               = %q
  verification_level = %d
}
`, name, vlevel)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{ // adopt + apply settings
				Config: cfg("My Server", 2),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(rn, "name", "My Server"),
					resource.TestCheckResourceAttr(rn, "verification_level", "2"),
					resource.TestCheckResourceAttr(rn, "id", "999"),
					resource.TestCheckResourceAttrSet(rn, "owner_id"),
				),
			},
			{ // update settings
				Config: cfg("Renamed Server", 1),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(rn, "name", "Renamed Server"),
					resource.TestCheckResourceAttr(rn, "verification_level", "1"),
				),
			},
			{ // import by guild id
				ResourceName:      rn,
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: importIDFunc(rn, "server_id"),
			},
		},
	})
}
