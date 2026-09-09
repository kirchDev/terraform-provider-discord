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
// Nothing is drawn at or above ceiling — neither a topped-up slot nor one a listed
// item already holds — so a shortfall is never made up by climbing over a sibling
// the caller does not manage, and an item standing above the line is not written
// back to where it stands. Where that leaves too few positions the result is an
// *orderRoomError rather than a write the API will reject. Pass noCeiling where
// climbing is harmless.
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
		if !ok || pos < start || pos >= ceiling || taken[pos] || claimed[pos] {
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

// --- Renumbering: the way out of a hierarchy with no free slot left.
//
// orderPositions above never writes a position an unlisted sibling holds, which is
// why it has to give up when there is no free one below the ceiling. Discord
// creates every new role on position 1 without renumbering anything, so a guild
// whose positions are packed solid below the app's own role can no longer place a
// freshly created role at all — the shortfall is structural and re-running does
// not clear it. The remedy an operator reaches for by hand is to drag the role in
// the Discord UI, which makes Discord renumber and a position appears.
//
// planRenumber does that deliberately instead: it writes the listed items across
// the whole range they occupy, positions unlisted siblings stand on included, and
// lets Discord's own re-sort bump those siblings up. What that relaxes is an
// unlisted sibling's *absolute* position; where it stands relative to every listed
// item is invariant, and is checked here before the write — and by the caller
// against the live read-back afterwards. ---

// orderCrossError says an unlisted item would have to change sides with a listed
// one for the configured order to hold. Nothing the resource may write can move
// it out of the way, so the order simply cannot be had.
type orderCrossError struct {
	ID string // the unlisted item the listed ones would have to cross
}

func (e *orderCrossError) Error() string {
	return fmt.Sprintf("%s would end up on the other side of them", e.ID)
}

// orderMismatchError says two listed items did not come out in the configured
// order. Raised against a hierarchy read back after a write, where it means the
// API did something other than the plan predicted.
type orderMismatchError struct {
	ID    string // the item that should sit below Above
	Above string
}

func (e *orderMismatchError) Error() string {
	return fmt.Sprintf("%s did not end up below %s", e.ID, e.Above)
}

// snowflakeLess compares two Discord ids by age. Ids are decimal snowflakes of
// varying width, so the shorter string is always the smaller — and the older —
// number; equal widths compare lexicographically.
func snowflakeLess(a, b string) bool {
	if len(a) != len(b) {
		return len(a) < len(b)
	}
	return a < b
}

// hierarchyOrder ranks every item lowest first. Two items sharing a position are
// ranked the way Discord itself ranks them: the younger id — the larger snowflake
// — sits lower, which is why a freshly created role appears *below* the role it
// shares position 1 with.
func hierarchyOrder(positions map[string]int64) []string {
	ids := make([]string, 0, len(positions))
	for id := range positions {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		a, b := ids[i], ids[j]
		if positions[a] != positions[b] {
			return positions[a] < positions[b]
		}
		return snowflakeLess(b, a)
	})
	return ids
}

// ranksOf turns a lowest-first ordering into id -> rank.
func ranksOf(order []string) map[string]int {
	out := make(map[string]int, len(order))
	for i, id := range order {
		out[id] = i
	}
	return out
}

// resortAfterWrite replays what the API does to a hierarchy once a
// modify-positions body has been applied: the items named in the body hold
// exactly the positions they were given, and an item that was *not* named but
// stands on one of them is bumped to the next position nothing holds, cascading
// upwards — an item only ever makes room above itself. Positions the body never
// reaches are left alone, so a gap in the hierarchy survives a write below it.
func resortAfterWrite(current, writes map[string]int64) map[string]int64 {
	out := make(map[string]int64, len(current))
	held := map[int64]bool{}
	rest := make([]string, 0, len(current))
	for id := range current {
		if pos, ok := writes[id]; ok {
			out[id] = pos
			held[pos] = true
			continue
		}
		rest = append(rest, id)
	}
	// Lowest first, so a bumped item lands in the gap above it rather than over a
	// sibling that has not been placed yet.
	sort.Slice(rest, func(i, j int) bool {
		a, b := rest[i], rest[j]
		if current[a] != current[b] {
			return current[a] < current[b]
		}
		return snowflakeLess(b, a)
	})
	for _, id := range rest {
		pos := current[id]
		for held[pos] {
			pos++
		}
		held[pos] = true
		out[id] = pos
	}
	return out
}

// crossedSibling names an unlisted item that changed sides with a listed one
// between two rankings, or "" when every one of them kept its place. Ids missing
// from either ranking are skipped — a deleted item crossed nothing.
func crossedSibling(before, after map[string]int, listed map[string]bool) string {
	var unlisted, items []string
	for id := range before {
		if _, ok := after[id]; !ok {
			continue
		}
		if listed[id] {
			items = append(items, id)
		} else {
			unlisted = append(unlisted, id)
		}
	}
	// Sorted so the same hierarchy always names the same item.
	sort.Strings(unlisted)
	sort.Strings(items)
	for _, u := range unlisted {
		for _, l := range items {
			if (before[u] < before[l]) != (after[u] < after[l]) {
				return u
			}
		}
	}
	return ""
}

// planRenumber builds the [{id, position}] body that puts the listed items in the
// given order by laying the whole hierarchy out again — including the items it may
// not write, which are counted but never sent.
//
// The listed items are permuted among the places they already hold in the
// hierarchy, so every unlisted item keeps its own place by construction; what the
// permutation can still break is which side of an unlisted item a listed one ends
// up on, and that is refused as an *orderCrossError. Positions are then laid out
// from the bottom: an unlisted item keeps its own unless the items below it have
// already claimed it, a listed one takes the next position going up, and gaps
// nothing needs are left alone. start is the lowest position an item may hold.
//
// With descending set the first id takes the highest place (roles); otherwise the
// lowest (channels). The plan is finally replayed through resortAfterWrite: a plan
// whose replay is not the plan is never written.
//
// ceiling is the one line the layout may not cross: no listed item is ever planned
// at or above it. It is deliberately *not* the same bound orderPositions took —
// writing over an unlisted sibling's position and letting the API's re-sort bump it
// is the whole point of renumbering — but a caller whose API refuses a write past
// some absolute line (for roles, the app's own highest role) has to say so, or the
// plan comes back as that API's flat refusal. Pass noCeiling where none exists.
func planRenumber(ids []string, positions map[string]int64, listed map[string]bool, start, ceiling int64, descending bool) ([]map[string]any, error) {
	order := hierarchyOrder(positions)
	before := ranksOf(order)
	var slots []int
	for i, id := range order {
		if listed[id] {
			slots = append(slots, i)
		}
	}
	if len(slots) != len(ids) {
		return nil, fmt.Errorf("the listed items hold %d places in the hierarchy but %d were listed", len(slots), len(ids))
	}

	target := make([]string, len(order))
	copy(target, order)
	for i, rank := range slots {
		id := ids[i]
		if descending {
			id = ids[len(ids)-1-i]
		}
		target[rank] = id
	}
	if crossed := crossedSibling(before, ranksOf(target), listed); crossed != "" {
		return nil, &orderCrossError{ID: crossed}
	}

	plan := make(map[string]int64, len(target))
	next := int64(0)
	for _, id := range target {
		pos := positions[id]
		switch {
		case listed[id]:
			pos = next
			if pos < start {
				pos = start
			}
		case pos < next:
			pos = next
		}
		plan[id] = pos
		next = pos + 1
	}

	// Laid out from the bottom, the plan is already the tightest one there is, so a
	// listed item landing at or above the ceiling means the order does not fit
	// under it at all — every position from start upwards is spoken for. Report the
	// shortfall the caller can name a role for rather than writing a body the API
	// answers flatly.
	for _, id := range ids {
		if plan[id] < ceiling {
			continue
		}
		free := ceiling - start
		if free < 0 {
			free = 0
		}
		return nil, &orderRoomError{Need: len(ids), Free: int(free), Ceiling: ceiling}
	}

	writes := make(map[string]int64, len(ids))
	for _, id := range ids {
		writes[id] = plan[id]
	}
	// A plan whose replay is not the plan is never written. Name an unlisted item
	// where one landed wrong — that is the sibling a reader can act on; a listed
	// one landing wrong means the model of the re-sort is off, not the config.
	off := ""
	replay := resortAfterWrite(positions, writes)
	for _, id := range hierarchyOrder(positions) {
		if replay[id] == plan[id] {
			continue
		}
		if !listed[id] {
			return nil, &orderCrossError{ID: id}
		}
		if off == "" {
			off = id
		}
	}
	if off != "" {
		return nil, fmt.Errorf("renumbering would not put %s where the plan asks for it", off)
	}

	body := make([]map[string]any, len(ids))
	for i, id := range ids {
		body[i] = map[string]any{"id": id, "position": writes[id]}
	}
	return body, nil
}

// verifyOrder checks a hierarchy read back after a write against what was asked
// for: the listed items stand in exactly the configured order, and every unlisted
// item stands where it did relative to each of them. It is the guard on relying on
// a model of the API's renumbering — a mismatch fails the apply naming the item,
// rather than storing an order nobody asked for.
func verifyOrder(ids []string, before, after map[string]int64, listed map[string]bool, descending bool) error {
	rb := ranksOf(hierarchyOrder(before))
	ra := ranksOf(hierarchyOrder(after))
	if crossed := crossedSibling(rb, ra, listed); crossed != "" {
		return &orderCrossError{ID: crossed}
	}
	for i := 1; i < len(ids); i++ {
		above, below := ids[i-1], ids[i]
		if !descending {
			above, below = below, above
		}
		hi, okHi := ra[above]
		lo, okLo := ra[below]
		if !okHi || !okLo || lo >= hi {
			return &orderMismatchError{ID: below, Above: above}
		}
	}
	return nil
}
