package provider

import (
	"fmt"
	"math"
	"sort"
)

// --- Position arithmetic shared by discord_role_order and discord_channel_order.
//
// Both resources own the order of a *subset* of a guild's roles or channels, so
// what they guarantee is the relative order of the items they were given — never
// an absolute one, since integration-managed roles cannot be moved by anyone and
// no caller can list them. A dense block of positions covering only the listed ids
// breaks that: it lands on slots that unlisted siblings already occupy, Discord
// resolves the collision by renormalising, and the order it hands back afterwards
// is not the one that was asked for. orderPositions draws slots that no unlisted
// sibling holds instead, so nothing has to be renormalised — and where there are
// not enough such slots it says so rather than climbing over a sibling to find
// one, which is how the same write comes back as Discord's bare 50013. ---

// noCeiling lets the top-up climb as far as it needs. Channel positions carry no
// such limit: they are compared within a category, and no channel is one the bot
// is forbidden to write past.
const noCeiling = int64(math.MaxInt64)

// orderRoomError says the listed items cannot be given one distinct position each
// within the range they may occupy. It carries the numbers rather than a sentence
// so a resource can name the sibling that closes the range in its own vocabulary.
type orderRoomError struct {
	Need    int   // positions the listed items need
	Free    int   // positions available to them
	Ceiling int64 // the first position they may not use
}

func (e *orderRoomError) Error() string {
	return fmt.Sprintf("they need %d distinct positions and only %d are free below position %d",
		e.Need, e.Free, e.Ceiling)
}

// orderPositions builds the [{id, position}] body Discord's modify-positions
// endpoints take, mapping ids onto slots that put them in exactly the given order.
//
// The slots are the ones the listed items already occupy — so the set keeps its
// place among its siblings — deduplicated, and topped up from the next free
// positions where that leaves too few (freshly created items all share one). A
// position held by an unlisted sibling is never used.
//
// The top-up stops below ceiling, so a shortfall is never made up by climbing over
// a sibling the caller does not manage; where that leaves too few positions the
// result is an *orderRoomError rather than a write the API will reject. Pass
// noCeiling where climbing is harmless.
//
// With descending set the first id takes the highest slot (roles, where a higher
// position sits higher in the hierarchy); otherwise it takes the lowest (channels,
// which read top to bottom). start is the lowest position an item may hold — 1 for
// roles, whose position 0 belongs to the immovable @everyone.
func orderPositions(ids []string, current map[string]int64, taken map[int64]bool, start, ceiling int64, descending bool) ([]map[string]any, error) {
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
	for pos := start; len(slots) < len(ids) && pos < ceiling; pos++ {
		if !taken[pos] && !claimed[pos] {
			claimed[pos] = true
			slots = append(slots, pos)
		}
	}
	if len(slots) < len(ids) {
		return nil, &orderRoomError{Need: len(ids), Free: len(slots), Ceiling: ceiling}
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
	return body, nil
}

// crossingCeiling is the first position the listed items may not take: the lowest
// one an unlisted sibling holds above every slot they already occupy. Growing the
// set into free space is fine, climbing over that sibling is not — it would move
// the set past something the resource does not manage, and where that sibling is
// the app's own role (always unlisted, since it is integration-managed) Discord
// refuses the whole request with a bare 50013. noCeiling when nothing sits above.
func crossingCeiling(ids []string, current map[string]int64, taken map[int64]bool) int64 {
	highest := int64(math.MinInt64)
	for _, id := range ids {
		if pos, ok := current[id]; ok && pos > highest {
			highest = pos
		}
	}
	ceiling := noCeiling
	for pos := range taken {
		if pos > highest && pos < ceiling {
			ceiling = pos
		}
	}
	return ceiling
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
