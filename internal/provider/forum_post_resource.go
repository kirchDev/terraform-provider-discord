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

// --- A post (thread + starting message) in a forum or media channel. This is
// the one resource that manages message *content* — by design, for a fixed,
// declarative post (e.g. a pinned info/guidelines post), not a chat stream. The
// forum post's starting message shares the thread's id. ---

const forumThreadFlagPinned = 1 << 1 // PINNED thread flag

var (
	_ resource.Resource                = (*forumPostResource)(nil)
	_ resource.ResourceWithConfigure   = (*forumPostResource)(nil)
	_ resource.ResourceWithImportState = (*forumPostResource)(nil)
)

// NewForumPostResource returns a new discord_forum_post resource.
func NewForumPostResource() resource.Resource {
	return &forumPostResource{}
}

type forumPostResource struct {
	client *client.Client
}

type forumThreadWire struct {
	ID               string   `json:"id"`
	ParentID         *string  `json:"parent_id"`
	Name             *string  `json:"name"`
	RateLimitPerUser int64    `json:"rate_limit_per_user"`
	Flags            int64    `json:"flags"`
	AppliedTags      []string `json:"applied_tags"`
	ThreadMetadata   *struct {
		Archived            bool  `json:"archived"`
		Locked              bool  `json:"locked"`
		AutoArchiveDuration int64 `json:"auto_archive_duration"`
	} `json:"thread_metadata"`
}

type forumPostResourceModel struct {
	ChannelID           types.String `tfsdk:"channel_id"`
	ID                  types.String `tfsdk:"id"`
	Name                types.String `tfsdk:"name"`
	Content             types.String `tfsdk:"content"`
	Tags                types.Set    `tfsdk:"tags"`
	AutoArchiveDuration types.Int64  `tfsdk:"auto_archive_duration"`
	RateLimitPerUser    types.Int64  `tfsdk:"rate_limit_per_user"`
	Pinned              types.Bool   `tfsdk:"pinned"`
	Archived            types.Bool   `tfsdk:"archived"`
	Locked              types.Bool   `tfsdk:"locked"`
}

func (r *forumPostResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_forum_post"
}

func (r *forumPostResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a post (a thread with a starting message) in a forum or media channel — for a " +
			"fixed, declarative post such as a pinned info or guidelines thread. This is the only resource that manages " +
			"message content; the bot needs `Read Message History` to refresh the content.",
		Attributes: map[string]schema.Attribute{
			"channel_id": schema.StringAttribute{
				MarkdownDescription: "Snowflake ID of the forum or media channel.",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "Snowflake ID of the post (its thread, and starting message).",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name":    schema.StringAttribute{MarkdownDescription: "Post title (the thread name).", Required: true},
			"content": schema.StringAttribute{MarkdownDescription: "Body of the starting message.", Required: true},
			"tags": schema.SetAttribute{
				MarkdownDescription: "Snowflake IDs of the forum tags applied to the post.",
				ElementType:         types.StringType,
				Optional:            true,
				Computed:            true,
			},
			"auto_archive_duration": schema.Int64Attribute{
				MarkdownDescription: "Minutes of inactivity before the post is archived (60, 1440, 4320 or 10080).",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
				Validators:          []validator.Int64{int64OneOf(60, 1440, 4320, 10080)},
			},
			"rate_limit_per_user": schema.Int64Attribute{
				MarkdownDescription: "Slowmode in seconds within the post.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
			"pinned":   schema.BoolAttribute{MarkdownDescription: "Whether the post is pinned in the forum.", Optional: true, Computed: true, Default: booldefault.StaticBool(false)},
			"archived": schema.BoolAttribute{MarkdownDescription: "Whether the post is archived.", Optional: true, Computed: true, Default: booldefault.StaticBool(false)},
			"locked":   schema.BoolAttribute{MarkdownDescription: "Whether the post is locked.", Optional: true, Computed: true, Default: booldefault.StaticBool(false)},
		},
	}
}

func (r *forumPostResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *forumPostResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan forumPostResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := map[string]any{
		"name":    plan.Name.ValueString(),
		"message": map[string]any{"content": plan.Content.ValueString()},
	}
	if v := plan.AutoArchiveDuration; !v.IsNull() && !v.IsUnknown() {
		body["auto_archive_duration"] = v.ValueInt64()
	}
	if v := plan.RateLimitPerUser; !v.IsNull() && !v.IsUnknown() {
		body["rate_limit_per_user"] = v.ValueInt64()
	}
	if tags, ok, err := strSet(ctx, plan.Tags); err != nil {
		resp.Diagnostics.AddError("Invalid tags", err.Error())
		return
	} else if ok {
		body["applied_tags"] = tags
	}

	var created forumThreadWire
	if err := r.client.Write(ctx, "POST", channelPath(plan.ChannelID.ValueString())+"/threads", body, &created); err != nil {
		resp.Diagnostics.AddError("Unable to create Discord forum post", err.Error())
		return
	}
	plan.ID = types.StringValue(created.ID)

	// Pin / archive / lock are not settable on create — apply them via a thread edit.
	if plan.Pinned.ValueBool() || plan.Archived.ValueBool() || plan.Locked.ValueBool() {
		if err := r.editThread(ctx, &plan); err != nil {
			resp.Diagnostics.AddError("Unable to set forum post flags", err.Error())
			return
		}
	}

	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Unable to read forum post after create", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *forumPostResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state forumPostResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.readInto(ctx, &state); err != nil {
		if notFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read Discord forum post", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *forumPostResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan forumPostResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.editThread(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Unable to update Discord forum post", err.Error())
		return
	}
	// Content lives on the starting message (shares the thread id).
	if v := plan.Content; !v.IsNull() && !v.IsUnknown() {
		msgPath := channelPath(plan.ID.ValueString()) + "/messages/" + plan.ID.ValueString()
		if err := r.client.Write(ctx, "PATCH", msgPath, map[string]any{"content": v.ValueString()}, nil); err != nil {
			resp.Diagnostics.AddError("Unable to update forum post content", err.Error())
			return
		}
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Unable to read forum post after update", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *forumPostResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state forumPostResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.Delete(ctx, channelPath(state.ID.ValueString())); err != nil && !notFound(err) {
		resp.Diagnostics.AddError("Unable to delete Discord forum post", err.Error())
	}
}

// ImportState accepts the post (thread) id; channel_id is recovered from the read.
func (r *forumPostResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

// editThread PATCHes the post's thread-level properties (name, tags, archive,
// lock, slowmode and the pinned flag).
func (r *forumPostResource) editThread(ctx context.Context, m *forumPostResourceModel) error {
	body := map[string]any{
		"name":     m.Name.ValueString(),
		"archived": m.Archived.ValueBool(),
		"locked":   m.Locked.ValueBool(),
	}
	if v := m.AutoArchiveDuration; !v.IsNull() && !v.IsUnknown() {
		body["auto_archive_duration"] = v.ValueInt64()
	}
	if v := m.RateLimitPerUser; !v.IsNull() && !v.IsUnknown() {
		body["rate_limit_per_user"] = v.ValueInt64()
	}
	if tags, ok, err := strSet(ctx, m.Tags); err != nil {
		return err
	} else if ok {
		body["applied_tags"] = tags
	}
	if m.Pinned.ValueBool() {
		body["flags"] = forumThreadFlagPinned
	} else {
		body["flags"] = 0
	}
	return r.client.Write(ctx, "PATCH", channelPath(m.ID.ValueString()), body, nil)
}

func (r *forumPostResource) readInto(ctx context.Context, m *forumPostResourceModel) error {
	var a forumThreadWire
	if err := r.client.Get(ctx, channelPath(m.ID.ValueString()), &a); err != nil {
		return err
	}
	m.ChannelID = types.StringPointerValue(a.ParentID)
	m.Name = types.StringPointerValue(a.Name)
	m.RateLimitPerUser = types.Int64Value(a.RateLimitPerUser)
	m.Pinned = types.BoolValue(a.Flags&forumThreadFlagPinned != 0)
	if a.ThreadMetadata != nil {
		m.Archived = types.BoolValue(a.ThreadMetadata.Archived)
		m.Locked = types.BoolValue(a.ThreadMetadata.Locked)
		m.AutoArchiveDuration = types.Int64Value(a.ThreadMetadata.AutoArchiveDuration)
	}
	tags, hasErr := setOfStrings(ctx, a.AppliedTags)
	if hasErr {
		return fmt.Errorf("building tags state")
	}
	m.Tags = tags

	// Content lives on the starting message (id == thread id).
	var msg struct {
		Content string `json:"content"`
	}
	if err := r.client.Get(ctx, channelPath(m.ID.ValueString())+"/messages/"+m.ID.ValueString(), &msg); err != nil {
		return err
	}
	m.Content = types.StringValue(msg.Content)
	return nil
}
