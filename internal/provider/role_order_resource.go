package provider

import (
	"context"
	"encoding/json"
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

// rolePos is the slice of a role object this resource reads.
type rolePos struct {
	ID       string `json:"id"`
	Position int64  `json:"position"`
}

func (r *roleOrderResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_role_order"
}

func (r *roleOrderResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Declaratively orders roles in the guild hierarchy via Discord's modify-role-positions " +
			"endpoint — the robust alternative to per-role `position`. List the roles from **highest to lowest** " +
			"(top to bottom, as the role list reads); the resource sets their relative positions and re-applies on " +
			"drift. Only the listed roles are touched. The bot can only reorder roles **below its own highest role**, " +
			"and the `@everyone` role cannot be moved — do not list it. Leave per-role `position` unset and order here.",
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

// apply gives the first-listed role the highest position and the last the lowest
// (just above @everyone), preserving the top-to-bottom order. Higher Discord
// position = higher in the hierarchy, hence the descending assignment.
func (r *roleOrderResource) apply(ctx context.Context, m *roleOrderResourceModel) error {
	var ids []string
	if d := m.RoleIDs.ElementsAs(ctx, &ids, false); d.HasError() {
		return fmt.Errorf("reading role_ids")
	}
	n := len(ids)
	body := make([]map[string]any, n)
	for i, id := range ids {
		body[i] = map[string]any{"id": id, "position": n - i}
	}
	return r.client.Write(ctx, "PATCH", "/guilds/"+m.ServerID.ValueString()+"/roles", body, nil)
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
	raws, err := r.client.List(ctx, "/guilds/"+guildID+"/roles")
	if err != nil {
		return err
	}
	posByID := map[string]int64{}
	for _, raw := range raws {
		var role rolePos
		if err := json.Unmarshal(raw, &role); err != nil {
			return err
		}
		posByID[role.ID] = role.Position
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
