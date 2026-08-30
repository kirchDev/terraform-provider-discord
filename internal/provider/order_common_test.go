package provider

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// Checks shared by the discord_role_order and discord_channel_order tests. They
// assert on the positions the provider actually wrote to the API rather than on
// what it put in state: the defect these resources had was a write landing on
// slots that belong to siblings they do not manage, which state alone cannot show.

// checkAtSlot asserts an item holds exactly the given position.
func checkAtSlot(live func() map[string]int64, id string, want int64) resource.TestCheckFunc {
	return func(*terraform.State) error {
		got, ok := live()[id]
		if !ok {
			return fmt.Errorf("%s is gone", id)
		}
		if got != want {
			return fmt.Errorf("%s holds position %d, want %d", id, got, want)
		}
		return nil
	}
}

// checkNoSharedSlot asserts no two items share a position. A dense block of
// positions covering only the managed ids lands on slots their unlisted siblings
// already occupy; Discord resolves that collision by renormalising, and the order
// it hands back then disagrees with the plan.
func checkNoSharedSlot(live func() map[string]int64) resource.TestCheckFunc {
	return func(*terraform.State) error {
		byPos := map[int64]string{}
		for id, pos := range live() {
			if other, clash := byPos[pos]; clash {
				return fmt.Errorf("%s and %s both hold position %d", other, id, pos)
			}
			byPos[pos] = id
		}
		return nil
	}
}

// checkOrder asserts the given ids hold positions that put them in exactly that
// order — strictly decreasing for roles (a higher position is higher in the
// hierarchy), strictly increasing for channels (which read top to bottom).
func checkOrder(live func() map[string]int64, descending bool, ids ...string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		positions := live()
		for i, id := range ids {
			pos, ok := positions[id]
			if !ok {
				return fmt.Errorf("listed %s is gone", id)
			}
			if i == 0 {
				continue
			}
			prev := positions[ids[i-1]]
			if descending && pos >= prev {
				return fmt.Errorf("%s (position %d) is not below %s (position %d)", id, pos, ids[i-1], prev)
			}
			if !descending && pos <= prev {
				return fmt.Errorf("%s (position %d) is not after %s (position %d)", id, pos, ids[i-1], prev)
			}
		}
		return nil
	}
}

// only narrows a live position map to the given ids. Channel positions are
// compared within a parent, so two channels under different categories may share
// one without clashing — a guild-wide uniqueness check would be wrong.
func only(live func() map[string]int64, ids ...string) func() map[string]int64 {
	return func() map[string]int64 {
		all := live()
		out := make(map[string]int64, len(ids))
		for _, id := range ids {
			if pos, ok := all[id]; ok {
				out[id] = pos
			}
		}
		return out
	}
}
