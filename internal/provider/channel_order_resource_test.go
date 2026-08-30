package provider

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

const (
	channelOrderGuildID  = "900000000000000002"
	channelOrderCategory = "900000000000000100"
	channelOrderFirst    = "900000000000000110"
	channelOrderForeign  = "900000000000000120"
	channelOrderSecond   = "900000000000000130"
	channelOrderThird    = "900000000000000140"
	channelOrderOutsider = "900000000000000200"

	channelOrderFreshGuildID  = "900000000000000003"
	channelOrderFreshCategory = "900000000000000300"
	channelOrderFreshA        = "900000000000000310"
	channelOrderFreshB        = "900000000000000320"
	channelOrderFreshC        = "900000000000000330"
	channelOrderFreshForeign  = "900000000000000340"

	channelOrderGoneGuildID  = "900000000000000006"
	channelOrderGoneCategory = "900000000000000600"
	channelOrderGoneForeign  = "900000000000000610"
	channelOrderGone         = "900000000000000620"
)

// TestAccChannelOrderResource_subsetWithForeignChannel is the channel half of the
// same defect: someone creates a channel in a managed category through the Discord
// client, and the next apply writes a dense block of positions over the slot it
// took. The listed channels must come out in the configured order with that
// channel left where it is.
//
// The outsider channel lives at the top level on the same position number as the
// foreign one, which is legal — Discord compares positions within a parent — and
// pins that the resource reads its occupied slots from siblings only.
func TestAccChannelOrderResource_subsetWithForeignChannel(t *testing.T) {
	m := newMockDiscord(t)
	m.seedChannel(channelOrderCategory, channelOrderGuildID, "Managed", "", 0)
	m.seedChannel(channelOrderFirst, channelOrderGuildID, "general", channelOrderCategory, 0)
	m.seedChannel(channelOrderForeign, channelOrderGuildID, "hand-made", channelOrderCategory, 1)
	m.seedChannel(channelOrderSecond, channelOrderGuildID, "random", channelOrderCategory, 2)
	m.seedChannel(channelOrderThird, channelOrderGuildID, "links", channelOrderCategory, 3)
	m.seedChannel(channelOrderOutsider, channelOrderGuildID, "lobby", "", 1)

	const rn = "discord_channel_order.test"
	cfg := fmt.Sprintf(`
resource "discord_channel_order" "test" {
  server_id   = %q
  parent_id   = %q
  channel_ids = [%q, %q, %q]
}
`, channelOrderGuildID, channelOrderCategory, channelOrderThird, channelOrderFirst, channelOrderSecond)

	siblings := only(m.channelPositions,
		channelOrderFirst, channelOrderForeign, channelOrderSecond, channelOrderThird)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{{
			Config: cfg,
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr(rn, "channel_ids.#", "3"),
				resource.TestCheckResourceAttr(rn, "channel_ids.0", channelOrderThird),
				resource.TestCheckResourceAttr(rn, "channel_ids.1", channelOrderFirst),
				resource.TestCheckResourceAttr(rn, "channel_ids.2", channelOrderSecond),
				checkOrder(m.channelPositions, false, channelOrderThird, channelOrderFirst, channelOrderSecond),
				// The listed channels swap among the three slots they already held;
				// the hand-made channel keeps the one between them.
				checkAtSlot(m.channelPositions, channelOrderThird, 0),
				checkAtSlot(m.channelPositions, channelOrderForeign, 1),
				checkAtSlot(m.channelPositions, channelOrderFirst, 2),
				checkAtSlot(m.channelPositions, channelOrderSecond, 3),
				checkAtSlot(m.channelPositions, channelOrderOutsider, 1),
				checkNoSharedSlot(siblings),
			),
		}},
	})
}

// TestAccChannelOrderResource_freshChannelsShareASlot covers the case reusing the
// listed items' own slots cannot answer on its own: channels created in the same
// apply all arrive on one position, so there are fewer distinct slots than
// channels. The order still has to come out right, and the sibling nobody manages
// still has to keep its own slot.
func TestAccChannelOrderResource_freshChannelsShareASlot(t *testing.T) {
	m := newMockDiscord(t)
	m.seedChannel(channelOrderFreshCategory, channelOrderFreshGuildID, "Managed", "", 0)
	m.seedChannel(channelOrderFreshA, channelOrderFreshGuildID, "one", channelOrderFreshCategory, 0)
	m.seedChannel(channelOrderFreshB, channelOrderFreshGuildID, "two", channelOrderFreshCategory, 0)
	m.seedChannel(channelOrderFreshC, channelOrderFreshGuildID, "three", channelOrderFreshCategory, 0)
	m.seedChannel(channelOrderFreshForeign, channelOrderFreshGuildID, "hand-made", channelOrderFreshCategory, 1)

	cfg := fmt.Sprintf(`
resource "discord_channel_order" "fresh" {
  server_id   = %q
  parent_id   = %q
  channel_ids = [%q, %q, %q]
}
`, channelOrderFreshGuildID, channelOrderFreshCategory,
		channelOrderFreshA, channelOrderFreshB, channelOrderFreshC)

	siblings := only(m.channelPositions,
		channelOrderFreshA, channelOrderFreshB, channelOrderFreshC, channelOrderFreshForeign)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{{
			Config: cfg,
			Check: resource.ComposeAggregateTestCheckFunc(
				checkOrder(m.channelPositions, false, channelOrderFreshA, channelOrderFreshB, channelOrderFreshC),
				checkAtSlot(m.channelPositions, channelOrderFreshForeign, 1),
				checkNoSharedSlot(siblings),
			),
		}},
	})
}

// TestAccChannelOrderResource_rejectsUnknownChannel covers the one path where the
// dense block survived: a listed channel deleted out of band resolves to nothing,
// so it contributes neither a slot to reuse nor a sibling to route around, and the
// top-up hands out 0,1,2,… over whatever the category already holds. A channel
// that is not there cannot be ordered, so the resource says which one is missing.
func TestAccChannelOrderResource_rejectsUnknownChannel(t *testing.T) {
	m := newMockDiscord(t)
	m.seedChannel(channelOrderGoneCategory, channelOrderGoneGuildID, "Managed", "", 0)
	m.seedChannel(channelOrderGoneForeign, channelOrderGoneGuildID, "hand-made", channelOrderGoneCategory, 0)

	cfg := fmt.Sprintf(`
resource "discord_channel_order" "test" {
  server_id   = %q
  parent_id   = %q
  channel_ids = [%q]
}
`, channelOrderGoneGuildID, channelOrderGoneCategory, channelOrderGone)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{{
			Config: cfg,
			// Matched on the provider's own wording: the id alone also appears in
			// the "element 0 has vanished" failure this guard replaces.
			ExpectError: regexp.MustCompile(`(?s)no\s+channel\s+` + channelOrderGone),
		}},
	})

	if got := m.channelPositions()[channelOrderGoneForeign]; got != 0 {
		t.Errorf("hand-made channel moved to position %d, want it left on 0", got)
	}
}
