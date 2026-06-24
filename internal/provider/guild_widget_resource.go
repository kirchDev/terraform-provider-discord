package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/kirchDev/terraform-provider-discord/internal/client"
)

// --- Manages a guild's widget settings. The widget is a singleton on the guild
// (no own id): it can't be created or deleted, only configured via PATCH, so this
// resource manages-not-creates and Delete just disables the widget. ---

var (
	_ resource.Resource                = (*guildWidgetResource)(nil)
	_ resource.ResourceWithConfigure   = (*guildWidgetResource)(nil)
	_ resource.ResourceWithImportState = (*guildWidgetResource)(nil)
)

// NewGuildWidgetResource returns a new discord_guild_widget resource.
func NewGuildWidgetResource() resource.Resource {
	return &guildWidgetResource{}
}

type guildWidgetResource struct {
	client *client.Client
}

type guildWidgetResourceModel struct {
	ServerID  types.String `tfsdk:"server_id"`
	Enabled   types.Bool   `tfsdk:"enabled"`
	ChannelID types.String `tfsdk:"channel_id"`
}

func (r *guildWidgetResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_guild_widget"
}

func (r *guildWidgetResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the widget settings of a Discord guild. The widget is a singleton on the guild; destroying this resource disables the widget.",
		Attributes: map[string]schema.Attribute{
			"server_id": schema.StringAttribute{
				MarkdownDescription: "Snowflake ID of the guild.",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether the widget is enabled.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
			"channel_id": schema.StringAttribute{
				MarkdownDescription: "Snowflake ID of the channel the widget generates invites for.",
				Optional:            true,
			},
		},
	}
}

func (r *guildWidgetResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// widgetPath is the read/update endpoint for a guild's widget settings.
func widgetPath(serverID string) string {
	return "/guilds/" + serverID + "/widget"
}

// apply PATCHes the widget settings from the plan.
func (r *guildWidgetResource) apply(ctx context.Context, m *guildWidgetResourceModel) error {
	body := map[string]any{}
	if v := m.Enabled; !v.IsNull() && !v.IsUnknown() {
		body["enabled"] = v.ValueBool()
	}
	if v := m.ChannelID; !v.IsNull() && !v.IsUnknown() {
		body["channel_id"] = v.ValueString()
	}
	return r.client.Write(ctx, "PATCH", widgetPath(m.ServerID.ValueString()), body, nil)
}

func (r *guildWidgetResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan guildWidgetResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.apply(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Unable to configure Discord guild widget", err.Error())
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Unable to read guild widget after configure", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *guildWidgetResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state guildWidgetResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.readInto(ctx, &state); err != nil {
		if notFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read Discord guild widget", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *guildWidgetResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan guildWidgetResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.apply(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Unable to update Discord guild widget", err.Error())
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Unable to read guild widget after update", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete disables the widget — it can't be deleted, only turned off.
func (r *guildWidgetResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state guildWidgetResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.Write(ctx, "PATCH", widgetPath(state.ServerID.ValueString()), map[string]any{"enabled": false}, nil); err != nil && !notFound(err) {
		resp.Diagnostics.AddError("Unable to disable Discord guild widget", err.Error())
	}
}

// ImportState accepts the server_id.
func (r *guildWidgetResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("server_id"), req.ID)...)
}

// readInto GETs the widget settings and refreshes the model.
func (r *guildWidgetResource) readInto(ctx context.Context, m *guildWidgetResourceModel) error {
	var w struct {
		Enabled   bool    `json:"enabled"`
		ChannelID *string `json:"channel_id"`
	}
	if err := r.client.Get(ctx, widgetPath(m.ServerID.ValueString()), &w); err != nil {
		return err
	}
	m.Enabled = types.BoolValue(w.Enabled)
	m.ChannelID = types.StringPointerValue(w.ChannelID)
	return nil
}
