package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/kirchDev/terraform-provider-discord/internal/client"
)

// --- Declarative role hierarchy ordering, the role analogue of
// discord_channel_order. Role `position` is relative and renumbered by Discord,
// so managing it per role is fragile. This resource owns the order of a list of
// roles and applies it atomically through the modify-role-positions endpoint
// (PATCH /guilds/{id}/roles). The bot can only move roles below its own highest
// role, and `@everyone` (position 0) cannot be moved — don't list it. ---

var (
	_ resource.Resource                = (*roleOrderResource)(nil)
	_ resource.ResourceWithConfigure   = (*roleOrderResource)(nil)
	_ resource.ResourceWithImportState = (*roleOrderResource)(nil)
)

// NewRoleOrderResource returns a new discord_role_order resource.
func NewRoleOrderResource() resource.Resource {
	return &roleOrderResource{}
}

type roleOrderResource struct {
	client *client.Client
}

type roleOrderResourceModel struct {
	ServerID types.String `tfsdk:"server_id"`
	RoleIDs  types.List   `tfsdk:"role_ids"`
}

// rolePos is the slice of a role object this resource reads. `managed` is read
// because an integration-managed role (Server Booster, a Twitch subscriber tier)
// cannot be moved by anyone — listing one makes Discord reject the whole PATCH.
type rolePos struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Position int64  `json:"position"`
	Managed  bool   `json:"managed"`
	// tags.bot_id is the app a bot role belongs to. Read only so an error in a
	// guild with more than one app role can tell ours from another integration's.
	Tags struct {
		BotID string `json:"bot_id"`
	} `json:"tags"`
}

func (r *roleOrderResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_role_order"
}

func (r *roleOrderResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Declaratively orders roles in the guild hierarchy via Discord's modify-role-positions " +
			"endpoint — the robust alternative to per-role `position`. List the roles from **highest to lowest** " +
			"(top to bottom, as the role list reads); the resource sets their relative positions and re-applies on " +
			"drift. Only the listed roles are touched: they are ordered **relative to one another** in the slots they " +
			"already hold, so roles you do not list keep theirs. An **integration-managed** role (Server Booster, a " +
			"Twitch subscriber tier) cannot be moved by anyone — listing one fails the apply, naming it. Discord " +
			"creates a new role on position 1 without renumbering the hierarchy, so a guild whose positions are " +
			"packed solid has no free slot to place it in; there the resource plans the renumbering Discord performs " +
			"when a role is dragged in its UI, and an unlisted role standing in the way is renumbered **upwards** by " +
			"Discord's own re-sort. Its absolute position changes, its place **relative** to every listed role never " +
			"does — checked before the write and against the hierarchy read back after it. Where even that cannot " +
			"hold — an unlisted role stands between two listed ones the configured order asks to swap sides around " +
			"— the apply fails naming that role and writes nothing. The bot can only reorder roles **below its own " +
			"highest role**, and the `@everyone` role cannot be moved — do not list it. Leave per-role `position` " +
			"unset and order here.",
		Attributes: map[string]schema.Attribute{
			"server_id": schema.StringAttribute{
				MarkdownDescription: "Snowflake ID of the guild.",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"role_ids": schema.ListAttribute{
				MarkdownDescription: "Snowflake IDs of the roles from highest to lowest in the hierarchy.",
				ElementType:         types.StringType,
				Required:            true,
			},
		},
	}
}

func (r *roleOrderResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected Resource Configure Type", fmt.Sprintf("Expected *client.Client, got: %T.", req.ProviderData))
		return
	}
	r.client = c
}

// apply orders the listed roles among themselves, first-listed highest. Higher
// Discord position = higher in the hierarchy, hence the descending assignment.
//
// It prefers to write only slots the listed roles already hold, so roles this
// resource does not manage — integration-managed ones, the app-owned bot roles —
// stay where they are and Discord has no collision to renormalise. Where a
// hierarchy leaves no free slot for a freshly created role it falls back to a
// planned renumbering that lets Discord's own re-sort make room, which relaxes an
// unlisted role's absolute position but never its relative one (see
// order_common.go).
func (r *roleOrderResource) apply(ctx context.Context, m *roleOrderResourceModel) error {
	var ids []string
	if d := m.RoleIDs.ElementsAs(ctx, &ids, false); d.HasError() {
		return fmt.Errorf("reading role_ids")
	}
	guildID := m.ServerID.ValueString()
	roles, err := r.roles(ctx, guildID)
	if err != nil {
		return err
	}
	for _, id := range ids {
		role, ok := roles[id]
		if !ok {
			// A role that is not there holds no slot to reuse and occupies none to
			// route around, so it would silently become a shortfall for the
			// position arithmetic to invent a slot for.
			return fmt.Errorf("no role %s in this server — role_ids may only list roles that exist", id)
		}
		// A managed role cannot be moved whatever the bot's own place in the
		// hierarchy; Discord answers the whole PATCH with a bare 50013.
		if role.Managed {
			return fmt.Errorf("role %q (%s) is managed by an integration and cannot be reordered — remove it from role_ids", role.Name, id)
		}
	}
	positions := make(map[string]int64, len(roles))
	for id, role := range roles {
		positions[id] = role.Position
	}
	listed := listedSet(ids)
	taken := occupiedPositions(positions, listed)
	// @everyone sits at position 0 and cannot be moved, so real roles start at 1.
	// The ceiling keeps the set from climbing over a role it does not manage: the
	// app's own role is integration-managed, so it is never listed, and a write at
	// or above it comes back as the bare 50013 this resource exists to replace.
	body, err := orderPositions(ids, positions, taken, 1, crossingCeiling(ids, positions, taken), true)
	renumbered := false
	if err != nil {
		var room *orderRoomError
		if !errors.As(err, &room) {
			return err
		}
		// No free position below the ceiling: the dense hierarchy a freshly created
		// role lands in, where Discord put it on position 1 beside an existing role
		// without renumbering anything. Rather than refuse — the shortfall is
		// structural, so re-running never clears it — plan the renumbering Discord
		// performs when a role is dragged in its UI: write the listed roles across
		// the whole range they occupy, positions unlisted siblings stand on
		// included, and let Discord's re-sort bump those siblings up. Their absolute
		// positions move; where they stand relative to the listed roles does not.
		body, err = planRenumber(ids, positions, listed, 1, true)
		if err != nil {
			return r.explainOrder(ctx, guildID, roles, err, room)
		}
		renumbered = true
	}
	if err := r.client.Write(ctx, "PATCH", "/guilds/"+guildID+"/roles", body, nil); err != nil {
		return err
	}
	if !renumbered {
		return nil
	}
	// The renumbering was planned against a model of Discord's re-sort, so what
	// Discord actually did is read back and checked rather than assumed.
	live, err := r.roles(ctx, guildID)
	if err != nil {
		return fmt.Errorf("reading the hierarchy back after reordering: %w", err)
	}
	after := make(map[string]int64, len(live))
	for id, role := range live {
		after[id] = role.Position
	}
	if err := verifyOrder(ids, positions, after, listed, true); err != nil {
		return r.explainOrder(ctx, guildID, live, err, nil)
	}
	return nil
}

// explainOrder turns the position arithmetic's typed errors into a sentence in
// this resource's own vocabulary, naming the roles a reader can see in the role
// list. room, when set, is the shortfall the renumbering was reached for.
func (r *roleOrderResource) explainOrder(ctx context.Context, guildID string, roles map[string]rolePos, err error, room *orderRoomError) error {
	var cross *orderCrossError
	if errors.As(err, &cross) {
		return fmt.Errorf("cannot order the listed roles without moving %s past them — %s. "+
			"Reorder role_ids so the listed roles stay on the same side of it, or list the roles in between as well",
			r.describeRole(ctx, guildID, roles, cross.ID),
			"an unlisted role may be renumbered, but never moved past a role you listed")
	}
	var mismatch *orderMismatchError
	if errors.As(err, &mismatch) {
		return fmt.Errorf("the order that was written was not applied by Discord: %s did not end up below %s — "+
			"re-run the apply to read the hierarchy back and try again",
			r.describeRole(ctx, guildID, roles, mismatch.ID), r.describeRole(ctx, guildID, roles, mismatch.Above))
	}
	if room != nil {
		return fmt.Errorf("cannot order the listed roles without moving %s, which is not listed: %s — %w",
			r.describeRole(ctx, guildID, roles, roleIDAt(roles, room.Ceiling)), room, err)
	}
	return err
}

// describeRole names a role for an error message, saying why it is one nobody may
// move where that is the point. The app-role lookup is best effort and never on
// the critical path: it only ever words a message, so a failure degrades to the
// plainer name rather than to a failed apply.
func (r *roleOrderResource) describeRole(ctx context.Context, guildID string, roles map[string]rolePos, id string) string {
	if id == "" {
		return "an unlisted role"
	}
	role, ok := roles[id]
	if !ok {
		return fmt.Sprintf("role %s", id)
	}
	switch {
	case id == guildID:
		return fmt.Sprintf("%q (%s), the @everyone role, which sits at position 0 and cannot be moved", role.Name, id)
	case r.isAppRole(ctx, roles, id):
		return fmt.Sprintf("%q (%s), this app's own role, which it cannot move", role.Name, id)
	case role.Managed:
		return fmt.Sprintf("%q (%s), managed by an integration and so movable by nobody", role.Name, id)
	}
	return fmt.Sprintf("%q (%s)", role.Name, id)
}

// isAppRole reports whether a role is the one belonging to the app this provider
// is authenticated as — `tags.bot_id` matching GET /users/@me. Best effort: a
// failed lookup answers false, which only makes the message plainer.
func (r *roleOrderResource) isAppRole(ctx context.Context, roles map[string]rolePos, id string) bool {
	role, ok := roles[id]
	if !ok || role.Tags.BotID == "" {
		return false
	}
	me, err := r.client.BotUserID(ctx)
	return err == nil && me != "" && me == role.Tags.BotID
}

// roleIDAt is the role holding a position, so an error a reader gets points at
// something they can see in the role list. Positions can be shared, so the lowest
// id wins rather than whichever one map iteration reached first.
func roleIDAt(roles map[string]rolePos, position int64) string {
	found := ""
	for id, role := range roles {
		if role.Position != position {
			continue
		}
		if found == "" || snowflakeLess(id, found) {
			found = id
		}
	}
	return found
}

// roles reads the guild's roles keyed by id.
func (r *roleOrderResource) roles(ctx context.Context, guildID string) (map[string]rolePos, error) {
	raws, err := r.client.List(ctx, "/guilds/"+guildID+"/roles")
	if err != nil {
		return nil, err
	}
	out := make(map[string]rolePos, len(raws))
	for _, raw := range raws {
		var role rolePos
		if err := json.Unmarshal(raw, &role); err != nil {
			return nil, err
		}
		out[role.ID] = role
	}
	return out, nil
}

func (r *roleOrderResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan roleOrderResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.apply(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Unable to set Discord role order", err.Error())
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Unable to read role order after apply", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *roleOrderResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state roleOrderResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.readInto(ctx, &state); err != nil {
		if notFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read Discord role order", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *roleOrderResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan roleOrderResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.apply(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Unable to update Discord role order", err.Error())
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Unable to read role order after update", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete is a no-op: removing the resource stops managing the order; the roles
// keep their current positions.
func (r *roleOrderResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

// ImportState accepts the guild id; role_ids is discovered on the following read.
func (r *roleOrderResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("server_id"), req.ID)...)
}

// readInto reorders the role list to reflect the live hierarchy (highest first).
// On a normal refresh it reorders the managed roles (dropping any that are gone);
// on import (no role_ids yet) it discovers every role except @everyone.
func (r *roleOrderResource) readInto(ctx context.Context, m *roleOrderResourceModel) error {
	guildID := m.ServerID.ValueString()
	roles, err := r.roles(ctx, guildID)
	if err != nil {
		return err
	}
	posByID := make(map[string]int64, len(roles))
	for id, role := range roles {
		posByID[id] = role.Position
	}

	var stateIDs []string
	if !m.RoleIDs.IsNull() && !m.RoleIDs.IsUnknown() {
		_ = m.RoleIDs.ElementsAs(ctx, &stateIDs, false)
	}

	var ordered []string
	if len(stateIDs) == 0 {
		// Import discovery: every role except @everyone (whose id == guild id).
		for id := range posByID {
			if id != guildID {
				ordered = append(ordered, id)
			}
		}
	} else {
		// Refresh: keep the managed roles that still exist.
		for _, id := range stateIDs {
			if _, ok := posByID[id]; ok {
				ordered = append(ordered, id)
			}
		}
	}
	// Highest position first (top of the hierarchy).
	sort.SliceStable(ordered, func(i, j int) bool { return posByID[ordered[i]] > posByID[ordered[j]] })

	list, d := types.ListValueFrom(ctx, types.StringType, ordered)
	if d.HasError() {
		return fmt.Errorf("building role_ids state")
	}
	m.RoleIDs = list
	return nil
}
