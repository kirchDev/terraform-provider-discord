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

// --- Channel-resource exemplar. The other channel kinds (voice/news/category/
// stage/forum/media) mirror this file, differing only in the channel `type`
// constant and which attributes apply. Channels are addressed globally by id
// (/channels/{id}) for read/update/delete and created under their guild. ---

var (
	_ resource.Resource                = (*textChannelResource)(nil)
	_ resource.ResourceWithConfigure   = (*textChannelResource)(nil)
	_ resource.ResourceWithImportState = (*textChannelResource)(nil)
)

// NewTextChannelResource returns a new discord_text_channel resource.
func NewTextChannelResource() resource.Resource {
	return &textChannelResource{}
}

type textChannelResource struct {
	client *client.Client
}

type textChannelResourceModel struct {
	ServerID              types.String `tfsdk:"server_id"`
	ID                    types.String `tfsdk:"id"`
	Name                  types.String `tfsdk:"name"`
	Category              types.String `tfsdk:"category"`
	Topic                 types.String `tfsdk:"topic"`
	NSFW                  types.Bool   `tfsdk:"nsfw"`
	Position              types.Int64  `tfsdk:"position"`
	RateLimitPerUser      types.Int64  `tfsdk:"rate_limit_per_user"`
	SyncPermsWithCategory types.Bool   `tfsdk:"sync_perms_with_category"`
}

func (r *textChannelResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_text_channel"
}

func (r *textChannelResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a text channel in a Discord guild.",
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
			"topic": schema.StringAttribute{
				MarkdownDescription: "Channel topic.",
				Optional:            true,
			},
			"nsfw": schema.BoolAttribute{
				MarkdownDescription: "Whether the channel is age-restricted.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
			"position": schema.Int64Attribute{
				MarkdownDescription: "Sorting position of the channel.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
			"rate_limit_per_user": schema.Int64Attribute{
				MarkdownDescription: "Slowmode in seconds (0–21600).",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
				Validators:          []validator.Int64{int64Between(0, 21600)},
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

func (r *textChannelResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *textChannelResource) body(m *textChannelResourceModel) map[string]any {
	body := map[string]any{"name": m.Name.ValueString(), "type": channelTypeText}
	if v := m.Category; !v.IsNull() && !v.IsUnknown() {
		body["parent_id"] = v.ValueString()
	}
	if v := m.Topic; !v.IsNull() && !v.IsUnknown() {
		body["topic"] = v.ValueString()
	}
	if v := m.NSFW; !v.IsNull() && !v.IsUnknown() {
		body["nsfw"] = v.ValueBool()
	}
	if v := m.Position; !v.IsNull() && !v.IsUnknown() {
		body["position"] = v.ValueInt64()
	}
	if v := m.RateLimitPerUser; !v.IsNull() && !v.IsUnknown() {
		body["rate_limit_per_user"] = v.ValueInt64()
	}
	return body
}

func (r *textChannelResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan textChannelResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var created channelAttributes
	if err := r.client.Write(ctx, "POST", guildChannelsPath(plan.ServerID.ValueString()), r.body(&plan), &created); err != nil {
		resp.Diagnostics.AddError("Unable to create Discord text channel", err.Error())
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

func (r *textChannelResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state textChannelResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.readInto(ctx, &state); err != nil {
		if notFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read Discord text channel", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *textChannelResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan textChannelResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.Write(ctx, "PATCH", channelPath(plan.ID.ValueString()), r.body(&plan), nil); err != nil {
		resp.Diagnostics.AddError("Unable to update Discord text channel", err.Error())
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

func (r *textChannelResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state textChannelResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.Delete(ctx, channelPath(state.ID.ValueString())); err != nil && !notFound(err) {
		resp.Diagnostics.AddError("Unable to delete Discord text channel", err.Error())
	}
}

// ImportState accepts the channel id; server_id is recovered from the channel.
func (r *textChannelResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

func (r *textChannelResource) readInto(ctx context.Context, m *textChannelResourceModel) error {
	a, err := readChannel(ctx, r.client, m.ID.ValueString())
	if err != nil {
		return err
	}
	if a.GuildID != nil {
		m.ServerID = types.StringValue(*a.GuildID)
	}
	m.Name = types.StringPointerValue(a.Name)
	m.Category = types.StringPointerValue(a.ParentID)
	m.Topic = types.StringPointerValue(a.Topic)
	m.NSFW = types.BoolValue(a.NSFW)
	m.Position = types.Int64Value(a.Position)
	m.RateLimitPerUser = types.Int64Value(a.RateLimitPerUser)
	return nil
}
