package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/kirchDev/terraform-provider-discord/internal/client"
)

// --- Creates a channel invite. Invites are immutable: Discord has no update
// endpoint, so every input attribute forces replacement and Update is a no-op
// re-Set. The invite is addressed by its short code (/invites/{code}). ---

var (
	_ resource.Resource                = (*inviteResource)(nil)
	_ resource.ResourceWithConfigure   = (*inviteResource)(nil)
	_ resource.ResourceWithImportState = (*inviteResource)(nil)
)

// NewInviteResource returns a new discord_invite resource.
func NewInviteResource() resource.Resource {
	return &inviteResource{}
}

type inviteResource struct {
	client *client.Client
}

type inviteResourceModel struct {
	ChannelID types.String `tfsdk:"channel_id"`
	MaxAge    types.Int64  `tfsdk:"max_age"`
	MaxUses   types.Int64  `tfsdk:"max_uses"`
	Temporary types.Bool   `tfsdk:"temporary"`
	Unique    types.Bool   `tfsdk:"unique"`
	Code      types.String `tfsdk:"code"`
	URL       types.String `tfsdk:"url"`
	RoleIDs   types.Set    `tfsdk:"role_ids"`
}

func (r *inviteResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_invite"
}

func (r *inviteResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Creates an invite to a Discord channel. Invites are immutable: any change forces a new invite to be created.",
		Attributes: map[string]schema.Attribute{
			"channel_id": schema.StringAttribute{
				MarkdownDescription: "Snowflake ID of the channel the invite points to.",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"max_age": schema.Int64Attribute{
				MarkdownDescription: "Duration of the invite in seconds before expiry (0 never expires).",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.RequiresReplace(), int64planmodifier.UseStateForUnknown()},
			},
			"max_uses": schema.Int64Attribute{
				MarkdownDescription: "Maximum number of uses (0 is unlimited).",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.RequiresReplace(), int64planmodifier.UseStateForUnknown()},
			},
			"temporary": schema.BoolAttribute{
				MarkdownDescription: "Whether the invite grants temporary membership.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.RequiresReplace()},
			},
			"unique": schema.BoolAttribute{
				MarkdownDescription: "Whether to always create a new unique invite rather than reusing a similar one.",
				Optional:            true,
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.RequiresReplace()},
			},
			"role_ids": schema.SetAttribute{
				MarkdownDescription: "Snowflake IDs of roles granted automatically to a user who joins through this invite. " +
					"The bot needs **Manage Roles**, and every role must sit **below the app's own highest role** and not be " +
					"managed by an integration — a role that is not is refused before the invite is created, naming it. " +
					"Roles are granted **on join only** (existing members get nothing), and a granted role **stays** after " +
					"the invite expires or is deleted. Refreshed from the invite's `roles` on read, so a change made outside " +
					"Terraform shows as drift; should Discord omit `roles` from the invite, the configured value is kept and " +
					"drift is not detected. Changing the set creates a new invite.",
				ElementType:   types.StringType,
				Optional:      true,
				PlanModifiers: []planmodifier.Set{setplanmodifier.RequiresReplace()},
			},
			"code": schema.StringAttribute{
				MarkdownDescription: "The invite code.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"url": schema.StringAttribute{
				MarkdownDescription: "The full invite URL.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *inviteResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *inviteResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan inviteResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := map[string]any{}
	if v := plan.MaxAge; !v.IsNull() && !v.IsUnknown() {
		body["max_age"] = v.ValueInt64()
	}
	if v := plan.MaxUses; !v.IsNull() && !v.IsUnknown() {
		body["max_uses"] = v.ValueInt64()
	}
	if v := plan.Temporary; !v.IsNull() && !v.IsUnknown() {
		body["temporary"] = v.ValueBool()
	}
	if v := plan.Unique; !v.IsNull() && !v.IsUnknown() {
		body["unique"] = v.ValueBool()
	}
	roleIDs, _, err := strSet(ctx, plan.RoleIDs)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read role_ids", err.Error())
		return
	}
	if len(roleIDs) > 0 {
		if err := r.checkGrantable(ctx, plan.ChannelID.ValueString(), roleIDs); err != nil {
			resp.Diagnostics.AddError("Unable to create Discord invite", err.Error())
			return
		}
		body["role_ids"] = roleIDs
	}

	// The create response includes the invite metadata (max_age/max_uses/
	// temporary); GET /invites/{code} does not, so capture them here.
	var inv struct {
		Code      string `json:"code"`
		MaxAge    int64  `json:"max_age"`
		MaxUses   int64  `json:"max_uses"`
		Temporary bool   `json:"temporary"`
	}
	if err := r.client.Write(ctx, "POST", "/channels/"+plan.ChannelID.ValueString()+"/invites", body, &inv); err != nil {
		resp.Diagnostics.AddError("Unable to create Discord invite", explainInviteError(err, len(roleIDs) > 0).Error())
		return
	}
	plan.Code = types.StringValue(inv.Code)
	plan.MaxAge = types.Int64Value(inv.MaxAge)
	plan.MaxUses = types.Int64Value(inv.MaxUses)
	plan.Temporary = types.BoolValue(inv.Temporary)
	plan.URL = types.StringValue("https://discord.gg/" + inv.Code)

	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Unable to read invite after create", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *inviteResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state inviteResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.readInto(ctx, &state); err != nil {
		if notFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read Discord invite", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update is a no-op re-Set: invites are immutable and every input forces replacement.
func (r *inviteResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan inviteResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *inviteResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state inviteResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.Delete(ctx, "/invites/"+state.Code.ValueString()); err != nil && !notFound(err) {
		resp.Diagnostics.AddError("Unable to delete Discord invite", err.Error())
	}
}

// ImportState accepts the invite code.
func (r *inviteResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("code"), req.ID)...)
}

// checkGrantable refuses, before the write, a role the app could not grant — the
// same rules discord_role_order checks, since Discord answers either with a bare
// 50013 for the whole request. Best effort, like appCeiling: where the channel's
// guild or its roles cannot be read, it answers nil and the invite is sent anyway;
// where the app's own role cannot be found, only the hierarchy half is skipped.
func (r *inviteResource) checkGrantable(ctx context.Context, channelID string, roleIDs []string) error {
	roles := r.channelGuildRoles(ctx, channelID)
	if roles == nil {
		return nil
	}
	appID, appTop := appCeiling(ctx, r.client, roles)
	for _, id := range roleIDs {
		role, ok := roles[id]
		switch {
		case !ok:
			return fmt.Errorf("no role %s in this server — role_ids may only list roles that exist", id)
		case id == appID:
			return fmt.Errorf("role %q (%s) is this app's own highest role, which it cannot grant — remove it from role_ids", role.Name, id)
		case appID != "" && role.Position >= appTop:
			return fmt.Errorf("role %q (%s) does not sit below %q (%s), this app's highest role — Discord only lets the app "+
				"grant roles below it. Move the role below it or remove it from role_ids", role.Name, id, roles[appID].Name, appID)
		case role.Managed:
			return fmt.Errorf("role %q (%s) is managed by an integration and cannot be granted — remove it from role_ids", role.Name, id)
		}
	}
	return nil
}

// channelGuildRoles loads the roles of the guild a channel belongs to, or nil
// where either read fails — the lookup is best effort, so a failure is not an
// error for checkGrantable to return but the absence of anything to check.
func (r *inviteResource) channelGuildRoles(ctx context.Context, channelID string) map[string]rolePos {
	var ch struct {
		GuildID string `json:"guild_id"`
	}
	if err := r.client.Get(ctx, "/channels/"+channelID, &ch); err != nil || ch.GuildID == "" {
		return nil
	}
	roles, err := guildRoles(ctx, r.client, ch.GuildID)
	if err != nil {
		return nil
	}
	return roles
}

// explainInviteError reports Discord's 50013 "Missing Permissions" as the
// permissions creating this invite needs, rather than as a bare API error.
func explainInviteError(err error, withRoles bool) error {
	var apiErr *client.APIError
	if !errors.As(err, &apiErr) || apiErr.Code != 50013 {
		return err
	}
	if withRoles {
		return fmt.Errorf("missing Manage Roles / Create Instant Invite: the bot needs both permissions, and every role in "+
			"role_ids must sit below its own highest role and not be managed by an integration (%w)", err)
	}
	return fmt.Errorf("missing Create Instant Invite: the bot lacks the permission on this channel (%w)", err)
}

// readInto GETs the invite by code to confirm it still exists and refresh the
// channel id, URL and granted roles. It deliberately does NOT touch max_age/max_uses/temporary:
// GET /invites/{code} omits those metadata fields (they're only returned by the
// channel/guild invite listing), and invites are immutable, so the create-time
// values in state are authoritative.
func (r *inviteResource) readInto(ctx context.Context, m *inviteResourceModel) error {
	var inv struct {
		Code    string `json:"code"`
		Channel struct {
			ID string `json:"id"`
		} `json:"channel"`
		// Partial role objects. A pointer, so a response without the field keeps
		// the create-time value in state rather than clearing it.
		Roles *[]struct {
			ID string `json:"id"`
		} `json:"roles"`
	}
	if err := r.client.Get(ctx, "/invites/"+m.Code.ValueString(), &inv); err != nil {
		return err
	}
	m.ChannelID = types.StringValue(inv.Channel.ID)
	m.URL = types.StringValue("https://discord.gg/" + inv.Code)
	if inv.Roles == nil {
		return nil
	}
	ids := make([]string, 0, len(*inv.Roles))
	for _, role := range *inv.Roles {
		ids = append(ids, role.ID)
	}
	// An invite granting nothing answers an empty array; keep an unset role_ids
	// unset instead of planning a diff against an empty set.
	if len(ids) == 0 && (m.RoleIDs.IsNull() || len(m.RoleIDs.Elements()) == 0) {
		return nil
	}
	set, failed := setOfStrings(ctx, ids)
	if failed {
		return fmt.Errorf("converting role_ids")
	}
	m.RoleIDs = set
	return nil
}
