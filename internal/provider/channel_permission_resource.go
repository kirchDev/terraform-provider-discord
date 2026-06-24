package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/kirchDev/terraform-provider-discord/internal/client"
)

// --- A single permission overwrite on a channel (one per channel × target).
// Targets are addressed by `type` (role|member) + `overwrite_id`. allow/deny are
// decimal string bitfields — feed them from the discord_permission helper. ---

var (
	_ resource.Resource                = (*channelPermissionResource)(nil)
	_ resource.ResourceWithConfigure   = (*channelPermissionResource)(nil)
	_ resource.ResourceWithImportState = (*channelPermissionResource)(nil)
)

// NewChannelPermissionResource returns a new discord_channel_permission resource.
func NewChannelPermissionResource() resource.Resource {
	return &channelPermissionResource{}
}

type channelPermissionResource struct {
	client *client.Client
}

type channelPermissionResourceModel struct {
	ChannelID   types.String `tfsdk:"channel_id"`
	OverwriteID types.String `tfsdk:"overwrite_id"`
	Type        types.String `tfsdk:"type"`
	Allow       types.String `tfsdk:"allow"`
	Deny        types.String `tfsdk:"deny"`
}

func (r *channelPermissionResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_channel_permission"
}

func (r *channelPermissionResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a single permission overwrite on a Discord channel for one role or member.",
		Attributes: map[string]schema.Attribute{
			"channel_id": schema.StringAttribute{
				MarkdownDescription: "Snowflake ID of the channel.",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"overwrite_id": schema.StringAttribute{
				MarkdownDescription: "Snowflake ID of the target role or member.",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"type": schema.StringAttribute{
				MarkdownDescription: "Overwrite target type: `role` or `member`.",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
				Validators:          []validator.String{stringOneOf("role", "member", "user")},
			},
			"allow": schema.StringAttribute{
				MarkdownDescription: "Allowed permission bitfield as a decimal string. See the `discord_permission` data source.",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("0"),
			},
			"deny": schema.StringAttribute{
				MarkdownDescription: "Denied permission bitfield as a decimal string. See the `discord_permission` data source.",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("0"),
			},
		},
	}
}

func (r *channelPermissionResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// overwriteType maps the string type to Discord's integer (0 role, 1 member).
func overwriteType(s string) (int, error) {
	switch strings.ToLower(s) {
	case "role":
		return 0, nil
	case "member", "user":
		return 1, nil
	default:
		return 0, fmt.Errorf("type must be \"role\" or \"member\", got %q", s)
	}
}

func overwriteTypeString(t int) string {
	if t == 0 {
		return "role"
	}
	return "member"
}

func (r *channelPermissionResource) permPath(m *channelPermissionResourceModel) string {
	return channelPath(m.ChannelID.ValueString()) + "/permissions/" + m.OverwriteID.ValueString()
}

func (r *channelPermissionResource) put(ctx context.Context, m *channelPermissionResourceModel) error {
	t, err := overwriteType(m.Type.ValueString())
	if err != nil {
		return err
	}
	body := map[string]any{
		"type":  t,
		"allow": m.Allow.ValueString(),
		"deny":  m.Deny.ValueString(),
	}
	return r.client.Write(ctx, "PUT", r.permPath(m), body, nil)
}

func (r *channelPermissionResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan channelPermissionResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.put(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Unable to create Discord channel permission", err.Error())
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Unable to read channel permission after create", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *channelPermissionResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state channelPermissionResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.readInto(ctx, &state); err != nil {
		if notFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read Discord channel permission", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *channelPermissionResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan channelPermissionResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.put(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Unable to update Discord channel permission", err.Error())
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Unable to read channel permission after update", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *channelPermissionResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state channelPermissionResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.Delete(ctx, r.permPath(&state)); err != nil && !notFound(err) {
		resp.Diagnostics.AddError("Unable to delete Discord channel permission", err.Error())
	}
}

// ImportState accepts "channel_id/overwrite_id".
func (r *channelPermissionResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, "/")
	if len(parts) != 2 {
		resp.Diagnostics.AddError("Invalid import ID", "Expected \"channel_id/overwrite_id\".")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("channel_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("overwrite_id"), parts[1])...)
}

// readInto finds the overwrite within the channel's permission_overwrites.
func (r *channelPermissionResource) readInto(ctx context.Context, m *channelPermissionResourceModel) error {
	ch, err := readChannel(ctx, r.client, m.ChannelID.ValueString())
	if err != nil {
		return err
	}
	for _, ow := range ch.PermissionOverwrites {
		if ow.ID == m.OverwriteID.ValueString() {
			m.Type = types.StringValue(overwriteTypeString(ow.Type))
			m.Allow = types.StringValue(ow.Allow)
			m.Deny = types.StringValue(ow.Deny)
			return nil
		}
	}
	return errNotInCollection
}
