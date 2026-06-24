package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/kirchDev/terraform-provider-discord/internal/client"
)

// --- Forum channel (#300): a GUILD_FORUM channel with managed tags, a default
// reaction, sort order and layout. Threads (forum posts) are managed by the
// discord_thread resource. Tags are managed declaratively by value; Discord owns
// their ids, so a tag's id may change if the tag set is reshuffled. ---

var (
	_ resource.Resource                = (*forumChannelResource)(nil)
	_ resource.ResourceWithConfigure   = (*forumChannelResource)(nil)
	_ resource.ResourceWithImportState = (*forumChannelResource)(nil)
)

// NewForumChannelResource returns a new discord_forum_channel resource.
func NewForumChannelResource() resource.Resource {
	return &forumChannelResource{}
}

type forumChannelResource struct {
	client *client.Client
}

// forumTagModel is one entry of available_tags in tfsdk form.
type forumTagModel struct {
	ID        types.String `tfsdk:"id"`
	Name      types.String `tfsdk:"name"`
	Moderated types.Bool   `tfsdk:"moderated"`
	EmojiID   types.String `tfsdk:"emoji_id"`
	EmojiName types.String `tfsdk:"emoji_name"`
}

var forumTagAttrTypes = map[string]attr.Type{
	"id":         types.StringType,
	"name":       types.StringType,
	"moderated":  types.BoolType,
	"emoji_id":   types.StringType,
	"emoji_name": types.StringType,
}

// forum wire types.
type forumTagWire struct {
	ID        string  `json:"id,omitempty"`
	Name      string  `json:"name"`
	Moderated bool    `json:"moderated"`
	EmojiID   *string `json:"emoji_id"`
	EmojiName *string `json:"emoji_name"`
}

type forumDefaultReactionWire struct {
	EmojiID   *string `json:"emoji_id"`
	EmojiName *string `json:"emoji_name"`
}

type forumChannelWire struct {
	ID                            string                    `json:"id"`
	GuildID                       *string                   `json:"guild_id"`
	Name                          *string                   `json:"name"`
	Topic                         *string                   `json:"topic"`
	NSFW                          bool                      `json:"nsfw"`
	Position                      int64                     `json:"position"`
	ParentID                      *string                   `json:"parent_id"`
	DefaultSortOrder              *int64                    `json:"default_sort_order"`
	DefaultForumLayout            int64                     `json:"default_forum_layout"`
	DefaultReactionEmoji          *forumDefaultReactionWire `json:"default_reaction_emoji"`
	AvailableTags                 []forumTagWire            `json:"available_tags"`
	DefaultThreadRateLimitPerUser int64                     `json:"default_thread_rate_limit_per_user"`
}

type forumChannelResourceModel struct {
	ServerID                      types.String `tfsdk:"server_id"`
	ID                            types.String `tfsdk:"id"`
	Name                          types.String `tfsdk:"name"`
	Category                      types.String `tfsdk:"category"`
	Topic                         types.String `tfsdk:"topic"`
	NSFW                          types.Bool   `tfsdk:"nsfw"`
	Position                      types.Int64  `tfsdk:"position"`
	DefaultSortOrder              types.Int64  `tfsdk:"default_sort_order"`
	DefaultForumLayout            types.Int64  `tfsdk:"default_forum_layout"`
	DefaultReactionEmojiID        types.String `tfsdk:"default_reaction_emoji_id"`
	DefaultReactionEmojiNme       types.String `tfsdk:"default_reaction_emoji_name"`
	AvailableTags                 types.List   `tfsdk:"available_tags"`
	DefaultThreadRateLimitPerUser types.Int64  `tfsdk:"default_thread_rate_limit_per_user"`
	SyncPermsWithCategory         types.Bool   `tfsdk:"sync_perms_with_category"`
}

func (r *forumChannelResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_forum_channel"
}

func (r *forumChannelResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a forum channel (`GUILD_FORUM`) in a Discord guild, including its tags, default " +
			"reaction, sort order and layout.",
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
			"name":     schema.StringAttribute{MarkdownDescription: "Channel name.", Required: true},
			"category": schema.StringAttribute{MarkdownDescription: "Snowflake ID of the parent category.", Optional: true},
			"topic":    schema.StringAttribute{MarkdownDescription: "Forum guidelines (the channel topic).", Optional: true},
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
			"default_sort_order": schema.Int64Attribute{
				MarkdownDescription: "Default sort order of posts (`0` latest activity, `1` creation date).",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
				Validators:          []validator.Int64{int64OneOf(0, 1)},
			},
			"default_forum_layout": schema.Int64Attribute{
				MarkdownDescription: "Default layout (`0` not set, `1` list, `2` gallery).",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
				Validators:          []validator.Int64{int64OneOf(0, 1, 2)},
			},
			"default_reaction_emoji_id":   schema.StringAttribute{MarkdownDescription: "Snowflake ID of the default reaction emoji (custom emoji).", Optional: true},
			"default_reaction_emoji_name": schema.StringAttribute{MarkdownDescription: "Unicode emoji used as the default reaction.", Optional: true},
			"available_tags": schema.ListNestedAttribute{
				MarkdownDescription: "Tags that can be applied to posts in the forum. Managed by value; Discord assigns the ids.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.List{listplanmodifier.UseStateForUnknown()},
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":         schema.StringAttribute{MarkdownDescription: "Snowflake ID of the tag (assigned by Discord).", Computed: true},
						"name":       schema.StringAttribute{MarkdownDescription: "Tag name.", Required: true},
						"moderated":  schema.BoolAttribute{MarkdownDescription: "Whether only moderators can apply the tag.", Optional: true, Computed: true, Default: booldefault.StaticBool(false)},
						"emoji_id":   schema.StringAttribute{MarkdownDescription: "Snowflake ID of the tag's custom emoji.", Optional: true},
						"emoji_name": schema.StringAttribute{MarkdownDescription: "Unicode emoji for the tag.", Optional: true},
					},
				},
			},
			"default_thread_rate_limit_per_user": schema.Int64Attribute{
				MarkdownDescription: "Default slowmode (seconds) for new posts in the forum.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
			"sync_perms_with_category": schema.BoolAttribute{
				MarkdownDescription: "When true, the channel's permission overwrites are synced to its parent category on create/update.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
		},
	}
}

func (r *forumChannelResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// body builds the create/update payload from the plan.
func (r *forumChannelResource) body(ctx context.Context, m *forumChannelResourceModel, diags *[]error) map[string]any {
	body := map[string]any{"name": m.Name.ValueString(), "type": channelTypeForum}
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
	if v := m.DefaultSortOrder; !v.IsNull() && !v.IsUnknown() {
		body["default_sort_order"] = v.ValueInt64()
	}
	if v := m.DefaultForumLayout; !v.IsNull() && !v.IsUnknown() {
		body["default_forum_layout"] = v.ValueInt64()
	}
	if v := m.DefaultThreadRateLimitPerUser; !v.IsNull() && !v.IsUnknown() {
		body["default_thread_rate_limit_per_user"] = v.ValueInt64()
	}
	if (!m.DefaultReactionEmojiID.IsNull() && !m.DefaultReactionEmojiID.IsUnknown()) ||
		(!m.DefaultReactionEmojiNme.IsNull() && !m.DefaultReactionEmojiNme.IsUnknown()) {
		body["default_reaction_emoji"] = map[string]any{
			"emoji_id":   strPtrOrNil(m.DefaultReactionEmojiID),
			"emoji_name": strPtrOrNil(m.DefaultReactionEmojiNme),
		}
	}
	if !m.AvailableTags.IsNull() && !m.AvailableTags.IsUnknown() {
		var tags []forumTagModel
		if d := m.AvailableTags.ElementsAs(ctx, &tags, false); d.HasError() {
			*diags = append(*diags, fmt.Errorf("reading available_tags"))
		} else {
			wire := make([]map[string]any, 0, len(tags))
			for _, t := range tags {
				entry := map[string]any{
					"name":       t.Name.ValueString(),
					"moderated":  t.Moderated.ValueBool(),
					"emoji_id":   strPtrOrNil(t.EmojiID),
					"emoji_name": strPtrOrNil(t.EmojiName),
				}
				if !t.ID.IsNull() && !t.ID.IsUnknown() && t.ID.ValueString() != "" {
					entry["id"] = t.ID.ValueString()
				}
				wire = append(wire, entry)
			}
			body["available_tags"] = wire
		}
	}
	return body
}

func (r *forumChannelResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan forumChannelResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var berr []error
	body := r.body(ctx, &plan, &berr)
	if len(berr) > 0 {
		resp.Diagnostics.AddError("Invalid forum channel configuration", berr[0].Error())
		return
	}
	var created forumChannelWire
	if err := r.client.Write(ctx, "POST", guildChannelsPath(plan.ServerID.ValueString()), body, &created); err != nil {
		resp.Diagnostics.AddError("Unable to create Discord forum channel", err.Error())
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
		resp.Diagnostics.AddError("Unable to read forum channel after create", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *forumChannelResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state forumChannelResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.readInto(ctx, &state); err != nil {
		if notFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read Discord forum channel", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *forumChannelResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan forumChannelResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var berr []error
	body := r.body(ctx, &plan, &berr)
	if len(berr) > 0 {
		resp.Diagnostics.AddError("Invalid forum channel configuration", berr[0].Error())
		return
	}
	if err := r.client.Write(ctx, "PATCH", channelPath(plan.ID.ValueString()), body, nil); err != nil {
		resp.Diagnostics.AddError("Unable to update Discord forum channel", err.Error())
		return
	}
	if plan.SyncPermsWithCategory.ValueBool() && !plan.Category.IsNull() {
		if err := syncPermsWithCategory(ctx, r.client, plan.ID.ValueString(), plan.Category.ValueString()); err != nil {
			resp.Diagnostics.AddError("Unable to sync channel permissions with category", err.Error())
			return
		}
	}

	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Unable to read forum channel after update", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *forumChannelResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state forumChannelResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.Delete(ctx, channelPath(state.ID.ValueString())); err != nil && !notFound(err) {
		resp.Diagnostics.AddError("Unable to delete Discord forum channel", err.Error())
	}
}

// ImportState accepts the channel id.
func (r *forumChannelResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

func (r *forumChannelResource) readInto(ctx context.Context, m *forumChannelResourceModel) error {
	var a forumChannelWire
	if err := r.client.Get(ctx, channelPath(m.ID.ValueString()), &a); err != nil {
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
	if a.DefaultSortOrder != nil {
		m.DefaultSortOrder = types.Int64Value(*a.DefaultSortOrder)
	} else {
		m.DefaultSortOrder = types.Int64Null()
	}
	m.DefaultForumLayout = types.Int64Value(a.DefaultForumLayout)
	m.DefaultThreadRateLimitPerUser = types.Int64Value(a.DefaultThreadRateLimitPerUser)
	if a.DefaultReactionEmoji != nil {
		m.DefaultReactionEmojiID = types.StringPointerValue(a.DefaultReactionEmoji.EmojiID)
		m.DefaultReactionEmojiNme = types.StringPointerValue(a.DefaultReactionEmoji.EmojiName)
	} else {
		m.DefaultReactionEmojiID = types.StringNull()
		m.DefaultReactionEmojiNme = types.StringNull()
	}

	tagModels := make([]forumTagModel, 0, len(a.AvailableTags))
	for _, t := range a.AvailableTags {
		tagModels = append(tagModels, forumTagModel{
			ID:        types.StringValue(t.ID),
			Name:      types.StringValue(t.Name),
			Moderated: types.BoolValue(t.Moderated),
			EmojiID:   types.StringPointerValue(t.EmojiID),
			EmojiName: types.StringPointerValue(t.EmojiName),
		})
	}
	list, d := types.ListValueFrom(ctx, types.ObjectType{AttrTypes: forumTagAttrTypes}, tagModels)
	if d.HasError() {
		return fmt.Errorf("building available_tags state: %v", d.Errors())
	}
	m.AvailableTags = list
	return nil
}
