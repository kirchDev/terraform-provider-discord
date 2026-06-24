package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/kirchDev/terraform-provider-discord/internal/client"
)

// --- Declarative channel ordering. Discord channel `position` values are
// relative and the API renumbers them, so managing them per channel is fragile
// (channels fight each other and bot activity). This resource instead owns the
// order of a *list* of sibling channels and applies it atomically through the
// bulk modify-channel-positions endpoint (PATCH /guilds/{id}/channels). It writes
// only positions, never parent_id — a channel's category stays owned by its own
// `category` attribute. Leave per-channel `position` unset and order here. ---

var (
	_ resource.Resource                = (*channelOrderResource)(nil)
	_ resource.ResourceWithConfigure   = (*channelOrderResource)(nil)
	_ resource.ResourceWithImportState = (*channelOrderResource)(nil)
)

// NewChannelOrderResource returns a new discord_channel_order resource.
func NewChannelOrderResource() resource.Resource {
	return &channelOrderResource{}
}

type channelOrderResource struct {
	client *client.Client
}

type channelOrderResourceModel struct {
	ServerID   types.String `tfsdk:"server_id"`
	ParentID   types.String `tfsdk:"parent_id"`
	ChannelIDs types.List   `tfsdk:"channel_ids"`
}

// channelPos is the slice of a channel object this resource reads.
type channelPos struct {
	ID       string  `json:"id"`
	Position int64   `json:"position"`
	ParentID *string `json:"parent_id"`
}

func (r *channelOrderResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_channel_order"
}

func (r *channelOrderResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Declaratively orders a list of sibling channels via Discord's bulk " +
			"modify-channel-positions endpoint — the robust alternative to per-channel `position`. List the channels " +
			"in the order you want; the resource assigns their relative positions and re-applies them on drift. Use one " +
			"per category (its children) and one without `parent_id` for the top level (categories and uncategorised " +
			"channels). Only the listed channels are touched, so it coexists with bot-managed channels.",
		Attributes: map[string]schema.Attribute{
			"server_id": schema.StringAttribute{
				MarkdownDescription: "Snowflake ID of the guild.",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"parent_id": schema.StringAttribute{
				MarkdownDescription: "Snowflake ID of the category whose children are ordered. Omit to order the top " +
					"level (categories and channels without a category). Used for import discovery.",
				Optional:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"channel_ids": schema.ListAttribute{
				MarkdownDescription: "Snowflake IDs of the channels in the desired order (top to bottom). They should " +
					"be siblings (share the same category, or all be top-level).",
				ElementType: types.StringType,
				Required:    true,
			},
		},
	}
}

func (r *channelOrderResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// apply assigns position = list index to each channel via the bulk endpoint.
func (r *channelOrderResource) apply(ctx context.Context, m *channelOrderResourceModel) error {
	var ids []string
	if d := m.ChannelIDs.ElementsAs(ctx, &ids, false); d.HasError() {
		return fmt.Errorf("reading channel_ids")
	}
	body := make([]map[string]any, len(ids))
	for i, id := range ids {
		body[i] = map[string]any{"id": id, "position": i}
	}
	return r.client.Write(ctx, "PATCH", "/guilds/"+m.ServerID.ValueString()+"/channels", body, nil)
}

func (r *channelOrderResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan channelOrderResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.apply(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Unable to set Discord channel order", err.Error())
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Unable to read channel order after apply", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *channelOrderResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state channelOrderResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.readInto(ctx, &state); err != nil {
		if notFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read Discord channel order", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *channelOrderResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan channelOrderResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.apply(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Unable to update Discord channel order", err.Error())
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Unable to read channel order after update", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete is a no-op: removing the resource stops managing the order; the channels
// keep their current positions.
func (r *channelOrderResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

// ImportState accepts "server_id" (top level) or "server_id/parent_id".
func (r *channelOrderResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, "/", 2)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("server_id"), parts[0])...)
	if len(parts) == 2 && parts[1] != "" {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("parent_id"), parts[1])...)
	}
}

// readInto reorders the channel list to reflect the live positions. On a normal
// refresh it reorders the channels already in state (dropping any that are gone);
// on import (no channel_ids yet) it discovers every channel under parent_id.
func (r *channelOrderResource) readInto(ctx context.Context, m *channelOrderResourceModel) error {
	raws, err := r.client.List(ctx, "/guilds/"+m.ServerID.ValueString()+"/channels")
	if err != nil {
		return err
	}
	all := make([]channelPos, 0, len(raws))
	posByID := map[string]int64{}
	for _, raw := range raws {
		var c channelPos
		if err := json.Unmarshal(raw, &c); err != nil {
			return err
		}
		all = append(all, c)
		posByID[c.ID] = c.Position
	}

	var ordered []string
	var stateIDs []string
	if !m.ChannelIDs.IsNull() && !m.ChannelIDs.IsUnknown() {
		_ = m.ChannelIDs.ElementsAs(ctx, &stateIDs, false)
	}

	if len(stateIDs) == 0 {
		// Import discovery: every channel under the configured parent_id.
		parent := m.ParentID.ValueString()
		wantParent := !m.ParentID.IsNull() && !m.ParentID.IsUnknown() && parent != ""
		matched := make([]channelPos, 0, len(all))
		for _, c := range all {
			has := c.ParentID != nil && *c.ParentID != ""
			if (wantParent && has && *c.ParentID == parent) || (!wantParent && !has) {
				matched = append(matched, c)
			}
		}
		sort.SliceStable(matched, func(i, j int) bool { return matched[i].Position < matched[j].Position })
		for _, c := range matched {
			ordered = append(ordered, c.ID)
		}
	} else {
		// Refresh: reorder the managed channels by their live position; drop any
		// that no longer exist.
		existing := make([]string, 0, len(stateIDs))
		for _, id := range stateIDs {
			if _, ok := posByID[id]; ok {
				existing = append(existing, id)
			}
		}
		sort.SliceStable(existing, func(i, j int) bool { return posByID[existing[i]] < posByID[existing[j]] })
		ordered = existing
	}

	list, d := types.ListValueFrom(ctx, types.StringType, ordered)
	if d.HasError() {
		return fmt.Errorf("building channel_ids state")
	}
	m.ChannelIDs = list
	return nil
}
