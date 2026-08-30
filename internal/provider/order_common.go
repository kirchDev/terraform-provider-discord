package provider

import "sort"

// --- Position arithmetic shared by discord_role_order and discord_channel_order.
//
// Both resources own the order of a *subset* of a guild's roles or channels, so
// what they guarantee is the relative order of the items they were given — never
// an absolute one, since integration-managed roles cannot be moved by anyone and
// no caller can list them. A dense block of positions covering only the listed ids
// breaks that: it lands on slots that unlisted siblings already occupy, Discord
// resolves the collision by renormalising, and the order it hands back afterwards
// is not the one that was asked for. orderPositions draws slots that no unlisted
// sibling holds instead, so nothing has to be renormalised. ---

// orderPositions builds the [{id, position}] body Discord's modify-positions
// endpoints take, mapping ids onto slots that put them in exactly the given order.
//
// The slots are the ones the listed items already occupy — so the set keeps its
// place among its siblings — deduplicated, and topped up from the next free
// positions where that leaves too few (freshly created items all share one). A
// position held by an unlisted sibling is never used.
//
// With descending set the first id takes the highest slot (roles, where a higher
// position sits higher in the hierarchy); otherwise it takes the lowest (channels,
// which read top to bottom). start is the lowest position an item may hold — 1 for
// roles, whose position 0 belongs to the immovable @everyone.
func orderPositions(ids []string, current map[string]int64, taken map[int64]bool, start int64, descending bool) []map[string]any {
	slots := make([]int64, 0, len(ids))
	claimed := map[int64]bool{}
	for _, id := range ids {
		pos, ok := current[id]
		if !ok || pos < start || taken[pos] || claimed[pos] {
			continue
		}
		claimed[pos] = true
		slots = append(slots, pos)
	}
	for pos := start; len(slots) < len(ids); pos++ {
		if !taken[pos] && !claimed[pos] {
			claimed[pos] = true
			slots = append(slots, pos)
		}
	}
	sort.Slice(slots, func(i, j int) bool { return slots[i] < slots[j] })

	body := make([]map[string]any, len(ids))
	for i, id := range ids {
		slot := slots[i]
		if descending {
			slot = slots[len(ids)-1-i]
		}
		body[i] = map[string]any{"id": id, "position": slot}
	}
	return body
}

// occupiedPositions collects the positions held by everything outside listed, so
// orderPositions can route around them.
func occupiedPositions(positions map[string]int64, listed map[string]bool) map[int64]bool {
	taken := map[int64]bool{}
	for id, pos := range positions {
		if !listed[id] {
			taken[pos] = true
		}
	}
	return taken
}

// listedSet turns the configured id list into a lookup.
func listedSet(ids []string) map[string]bool {
	set := make(map[string]bool, len(ids))
	for _, id := range ids {
		set[id] = true
	}
	return set
}
