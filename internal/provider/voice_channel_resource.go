package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/kirchDev/terraform-provider-discord/internal/client"
)

var (
	_ resource.Resource                = (*voiceChannelResource)(nil)
	_ resource.ResourceWithConfigure   = (*voiceChannelResource)(nil)
	_ resource.ResourceWithImportState = (*voiceChannelResource)(nil)
)

// NewVoiceChannelResource returns a new discord_voice_channel resource.
func NewVoiceChannelResource() resource.Resource {
	return &voiceChannelResource{}
}

type voiceChannelResource struct {
	client *client.Client
}

type voiceChannelResourceModel struct {
	ServerID              types.String `tfsdk:"server_id"`
	ID                    types.String `tfsdk:"id"`
	Name                  types.String `tfsdk:"name"`
	Category              types.String `tfsdk:"category"`
	Position              types.Int64  `tfsdk:"position"`
	Bitrate               types.Int64  `tfsdk:"bitrate"`
	VideoQualityMode      types.Int64  `tfsdk:"video_quality_mode"`
	UserLimit             types.Int64  `tfsdk:"user_limit"`
	RTCRegion             types.String `tfsdk:"rtc_region"`
	NSFW                  types.Bool   `tfsdk:"nsfw"`
	SyncPermsWithCategory types.Bool   `tfsdk:"sync_perms_with_category"`
}

func (r *voiceChannelResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_voice_channel"
}

func (r *voiceChannelResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a voice channel in a Discord guild.",
		Attributes: map[string]schema.Attribute{
			"server_id": schema.StringAttribute{
				MarkdownDescription: "Snowflake ID of the guild.",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "Snowflake ID of the channel.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Channel name.",
				Required:            true,
			},
			"category": schema.StringAttribute{
				MarkdownDescription: "Snowflake ID of the parent category channel.",
				Optional:            true,
			},
			"position": schema.Int64Attribute{
				MarkdownDescription: "Sorting position of the channel.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
			"bitrate": schema.Int64Attribute{
				MarkdownDescription: "Bitrate (in bits) of the voice channel.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
			"video_quality_mode": schema.Int64Attribute{
				MarkdownDescription: "Camera video quality mode (`1` auto, `2` full 720p).",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
				Validators:          []validator.Int64{int64OneOf(1, 2)},
			},
			"user_limit": schema.Int64Attribute{
				MarkdownDescription: "Maximum number of users allowed in the voice channel.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
			"rtc_region": schema.StringAttribute{
				MarkdownDescription: "Voice region id for the channel; null sets it to automatic.",
				Optional:            true,
			},
			"nsfw": schema.BoolAttribute{
				MarkdownDescription: "Whether the channel is age-restricted.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
			"sync_perms_with_category": schema.BoolAttribute{
				MarkdownDescription: "When true, the channel's permission overwrites are synced to its parent category on create/update. Conflicts with explicit `discord_channel_permission` overwrites on the same channel.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
		},
	}
}

func (r *voiceChannelResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *voiceChannelResource) body(m *voiceChannelResourceModel) map[string]any {
	body := map[string]any{"name": m.Name.ValueString(), "type": channelTypeVoice}
	if v := m.Category; !v.IsNull() && !v.IsUnknown() {
		body["parent_id"] = v.ValueString()
	}
	if v := m.Position; !v.IsNull() && !v.IsUnknown() {
		body["position"] = v.ValueInt64()
	}
	if v := m.Bitrate; !v.IsNull() && !v.IsUnknown() {
		body["bitrate"] = v.ValueInt64()
	}
	// Never send 0: Discord rejects it (valid: 1 auto, 2 full) and a channel that
	// never set this unmarshals the absent field to 0, which would 400 any update
	// (e.g. a rename) of an imported channel.
	if v := m.VideoQualityMode; !v.IsNull() && !v.IsUnknown() && v.ValueInt64() != 0 {
		body["video_quality_mode"] = v.ValueInt64()
	}
	if v := m.UserLimit; !v.IsNull() && !v.IsUnknown() {
		body["user_limit"] = v.ValueInt64()
	}
	if v := m.RTCRegion; !v.IsNull() && !v.IsUnknown() {
		body["rtc_region"] = v.ValueString()
	}
	if v := m.NSFW; !v.IsNull() && !v.IsUnknown() {
		body["nsfw"] = v.ValueBool()
	}
	return body
}

func (r *voiceChannelResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan voiceChannelResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var created channelAttributes
	if err := r.client.Write(ctx, "POST", guildChannelsPath(plan.ServerID.ValueString()), r.body(&plan), &created); err != nil {
		resp.Diagnostics.AddError("Unable to create Discord voice channel", err.Error())
		return
	}
	plan.ID = types.StringValue(created.ID)

	if plan.SyncPermsWithCategory.ValueBool() && !plan.Category.IsNull() {
		if err := syncPermsWithCategory(ctx, r.client, created.ID, plan.Category.ValueString()); err != nil {
			resp.Diagnostics.AddError("Unable to sync channel permissions with category", err.Error())
			return
		}
	}

	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Unable to read channel after create", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *voiceChannelResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state voiceChannelResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.readInto(ctx, &state); err != nil {
		if notFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read Discord voice channel", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *voiceChannelResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan voiceChannelResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.Write(ctx, "PATCH", channelPath(plan.ID.ValueString()), r.body(&plan), nil); err != nil {
		resp.Diagnostics.AddError("Unable to update Discord voice channel", err.Error())
		return
	}
	if plan.SyncPermsWithCategory.ValueBool() && !plan.Category.IsNull() {
		if err := syncPermsWithCategory(ctx, r.client, plan.ID.ValueString(), plan.Category.ValueString()); err != nil {
			resp.Diagnostics.AddError("Unable to sync channel permissions with category", err.Error())
			return
		}
	}

	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Unable to read channel after update", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *voiceChannelResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state voiceChannelResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.Delete(ctx, channelPath(state.ID.ValueString())); err != nil && !notFound(err) {
		resp.Diagnostics.AddError("Unable to delete Discord voice channel", err.Error())
	}
}

// ImportState accepts the channel id; server_id is recovered from the channel.
func (r *voiceChannelResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

func (r *voiceChannelResource) readInto(ctx context.Context, m *voiceChannelResourceModel) error {
	a, err := readChannel(ctx, r.client, m.ID.ValueString())
	if err != nil {
		return err
	}
	if a.GuildID != nil {
		m.ServerID = types.StringValue(*a.GuildID)
	}
	m.Name = types.StringPointerValue(a.Name)
	m.Category = types.StringPointerValue(a.ParentID)
	m.Position = types.Int64Value(a.Position)
	m.Bitrate = types.Int64Value(a.Bitrate)
	m.VideoQualityMode = types.Int64Value(a.VideoQualityMode)
	m.UserLimit = types.Int64Value(a.UserLimit)
	m.RTCRegion = types.StringPointerValue(a.RTCRegion)
	m.NSFW = types.BoolValue(a.NSFW)
	return nil
}
