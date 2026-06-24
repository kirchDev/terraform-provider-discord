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

// --- Manages a thread (type 11) inside a text or announcement channel. The
// parent channel forces replacement; the thread is then addressed globally by id
// (/channels/{id}) for read/update/delete like any channel. Thread-specific state
// (archived/locked/auto_archive_duration/invitable) lives under thread_metadata. ---

var (
	_ resource.Resource                = (*threadResource)(nil)
	_ resource.ResourceWithConfigure   = (*threadResource)(nil)
	_ resource.ResourceWithImportState = (*threadResource)(nil)
)

// NewThreadResource returns a new discord_thread resource.
func NewThreadResource() resource.Resource {
	return &threadResource{}
}

type threadResource struct {
	client *client.Client
}

type threadResourceModel struct {
	ChannelID           types.String `tfsdk:"channel_id"`
	ID                  types.String `tfsdk:"id"`
	Name                types.String `tfsdk:"name"`
	AutoArchiveDuration types.Int64  `tfsdk:"auto_archive_duration"`
	RateLimitPerUser    types.Int64  `tfsdk:"rate_limit_per_user"`
	Archived            types.Bool   `tfsdk:"archived"`
	Locked              types.Bool   `tfsdk:"locked"`
	Invitable           types.Bool   `tfsdk:"invitable"`
}

// threadAttributes mirrors the channel fields the thread resource maps, including
// the nested thread_metadata object.
type threadAttributes struct {
	ID               string  `json:"id"`
	Name             *string `json:"name"`
	RateLimitPerUser int64   `json:"rate_limit_per_user"`
	ParentID         *string `json:"parent_id"`
	ThreadMetadata   struct {
		Archived            bool  `json:"archived"`
		Locked              bool  `json:"locked"`
		AutoArchiveDuration int64 `json:"auto_archive_duration"`
		Invitable           bool  `json:"invitable"`
	} `json:"thread_metadata"`
}

func (r *threadResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_thread"
}

func (r *threadResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a thread within a Discord text or announcement channel.",
		Attributes: map[string]schema.Attribute{
			"channel_id": schema.StringAttribute{
				MarkdownDescription: "Snowflake ID of the parent channel the thread is created in.",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "Snowflake ID of the thread.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Thread name.",
				Required:            true,
			},
			"auto_archive_duration": schema.Int64Attribute{
				MarkdownDescription: "Minutes of inactivity before the thread is archived (60, 1440, 4320, or 10080).",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
				Validators:          []validator.Int64{int64OneOf(60, 1440, 4320, 10080)},
			},
			"rate_limit_per_user": schema.Int64Attribute{
				MarkdownDescription: "Slowmode in seconds (0–21600).",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
			"archived": schema.BoolAttribute{
				MarkdownDescription: "Whether the thread is archived.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
			"locked": schema.BoolAttribute{
				MarkdownDescription: "Whether the thread is locked (only moderators can unarchive it).",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
			"invitable": schema.BoolAttribute{
				MarkdownDescription: "Whether non-moderators can add other members to a private thread.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
		},
	}
}

func (r *threadResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *threadResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan threadResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// type 11 = GUILD_PUBLIC_THREAD.
	body := map[string]any{"name": plan.Name.ValueString(), "type": 11}
	if v := plan.AutoArchiveDuration; !v.IsNull() && !v.IsUnknown() {
		body["auto_archive_duration"] = v.ValueInt64()
	}
	if v := plan.RateLimitPerUser; !v.IsNull() && !v.IsUnknown() {
		body["rate_limit_per_user"] = v.ValueInt64()
	}

	var created threadAttributes
	if err := r.client.Write(ctx, "POST", "/channels/"+plan.ChannelID.ValueString()+"/threads", body, &created); err != nil {
		resp.Diagnostics.AddError("Unable to create Discord thread", err.Error())
		return
	}
	plan.ID = types.StringValue(created.ID)

	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Unable to read thread after create", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *threadResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state threadResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.readInto(ctx, &state); err != nil {
		if notFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read Discord thread", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *threadResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan threadResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := map[string]any{"name": plan.Name.ValueString()}
	if v := plan.AutoArchiveDuration; !v.IsNull() && !v.IsUnknown() {
		body["auto_archive_duration"] = v.ValueInt64()
	}
	if v := plan.RateLimitPerUser; !v.IsNull() && !v.IsUnknown() {
		body["rate_limit_per_user"] = v.ValueInt64()
	}
	if v := plan.Archived; !v.IsNull() && !v.IsUnknown() {
		body["archived"] = v.ValueBool()
	}
	if v := plan.Locked; !v.IsNull() && !v.IsUnknown() {
		body["locked"] = v.ValueBool()
	}
	if v := plan.Invitable; !v.IsNull() && !v.IsUnknown() {
		body["invitable"] = v.ValueBool()
	}
	if err := r.client.Write(ctx, "PATCH", channelPath(plan.ID.ValueString()), body, nil); err != nil {
		resp.Diagnostics.AddError("Unable to update Discord thread", err.Error())
		return
	}

	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Unable to read thread after update", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *threadResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state threadResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.Delete(ctx, channelPath(state.ID.ValueString())); err != nil && !notFound(err) {
		resp.Diagnostics.AddError("Unable to delete Discord thread", err.Error())
	}
}

// ImportState accepts the thread id; channel_id is recovered from the thread.
func (r *threadResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

// readInto GETs the thread by id and refreshes its fields from the channel object
// and its nested thread_metadata.
func (r *threadResource) readInto(ctx context.Context, m *threadResourceModel) error {
	var a threadAttributes
	if err := r.client.Get(ctx, channelPath(m.ID.ValueString()), &a); err != nil {
		return err
	}
	m.ChannelID = types.StringPointerValue(a.ParentID)
	m.Name = types.StringPointerValue(a.Name)
	m.RateLimitPerUser = types.Int64Value(a.RateLimitPerUser)
	m.Archived = types.BoolValue(a.ThreadMetadata.Archived)
	m.Locked = types.BoolValue(a.ThreadMetadata.Locked)
	m.AutoArchiveDuration = types.Int64Value(a.ThreadMetadata.AutoArchiveDuration)
	m.Invitable = types.BoolValue(a.ThreadMetadata.Invitable)
	return nil
}
