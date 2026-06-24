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
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/kirchDev/terraform-provider-discord/internal/client"
)

var (
	_ resource.Resource                = (*stageChannelResource)(nil)
	_ resource.ResourceWithConfigure   = (*stageChannelResource)(nil)
	_ resource.ResourceWithImportState = (*stageChannelResource)(nil)
)

// NewStageChannelResource returns a new discord_stage_channel resource.
func NewStageChannelResource() resource.Resource {
	return &stageChannelResource{}
}

type stageChannelResource struct {
	client *client.Client
}

type stageChannelResourceModel struct {
	ServerID              types.String `tfsdk:"server_id"`
	ID                    types.String `tfsdk:"id"`
	Name                  types.String `tfsdk:"name"`
	Category              types.String `tfsdk:"category"`
	Position              types.Int64  `tfsdk:"position"`
	Bitrate               types.Int64  `tfsdk:"bitrate"`
	UserLimit             types.Int64  `tfsdk:"user_limit"`
	RTCRegion             types.String `tfsdk:"rtc_region"`
	SyncPermsWithCategory types.Bool   `tfsdk:"sync_perms_with_category"`
}

func (r *stageChannelResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_stage_channel"
}

func (r *stageChannelResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a stage channel in a Discord guild.",
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
				MarkdownDescription: "Bitrate (in bits) of the stage channel.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
			"user_limit": schema.Int64Attribute{
				MarkdownDescription: "Maximum number of users allowed in the stage channel.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
			"rtc_region": schema.StringAttribute{
				MarkdownDescription: "Voice region id for the channel; null sets it to automatic.",
				Optional:            true,
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

func (r *stageChannelResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *stageChannelResource) body(m *stageChannelResourceModel) map[string]any {
	body := map[string]any{"name": m.Name.ValueString(), "type": channelTypeStage}
	if v := m.Category; !v.IsNull() && !v.IsUnknown() {
		body["parent_id"] = v.ValueString()
	}
	if v := m.Position; !v.IsNull() && !v.IsUnknown() {
		body["position"] = v.ValueInt64()
	}
	if v := m.Bitrate; !v.IsNull() && !v.IsUnknown() {
		body["bitrate"] = v.ValueInt64()
	}
	if v := m.UserLimit; !v.IsNull() && !v.IsUnknown() {
		body["user_limit"] = v.ValueInt64()
	}
	if v := m.RTCRegion; !v.IsNull() && !v.IsUnknown() {
		body["rtc_region"] = v.ValueString()
	}
	return body
}

func (r *stageChannelResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan stageChannelResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var created channelAttributes
	if err := r.client.Write(ctx, "POST", guildChannelsPath(plan.ServerID.ValueString()), r.body(&plan), &created); err != nil {
		resp.Diagnostics.AddError("Unable to create Discord stage channel", err.Error())
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

func (r *stageChannelResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state stageChannelResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.readInto(ctx, &state); err != nil {
		if notFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read Discord stage channel", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *stageChannelResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan stageChannelResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.Write(ctx, "PATCH", channelPath(plan.ID.ValueString()), r.body(&plan), nil); err != nil {
		resp.Diagnostics.AddError("Unable to update Discord stage channel", err.Error())
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

func (r *stageChannelResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state stageChannelResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.Delete(ctx, channelPath(state.ID.ValueString())); err != nil && !notFound(err) {
		resp.Diagnostics.AddError("Unable to delete Discord stage channel", err.Error())
	}
}

// ImportState accepts the channel id; server_id is recovered from the channel.
func (r *stageChannelResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

func (r *stageChannelResource) readInto(ctx context.Context, m *stageChannelResourceModel) error {
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
	m.UserLimit = types.Int64Value(a.UserLimit)
	m.RTCRegion = types.StringPointerValue(a.RTCRegion)
	return nil
}
