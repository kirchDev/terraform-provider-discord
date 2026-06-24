package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/kirchDev/terraform-provider-discord/internal/client"
)

// --- Manages just a member's nickname (a single PATCH field on the guild
// member). The member itself isn't created or destroyed here: applying sets the
// nick and destroying clears it back to the user's default. ---

var (
	_ resource.Resource                = (*memberNicknameResource)(nil)
	_ resource.ResourceWithConfigure   = (*memberNicknameResource)(nil)
	_ resource.ResourceWithImportState = (*memberNicknameResource)(nil)
)

// NewMemberNicknameResource returns a new discord_member_nickname resource.
func NewMemberNicknameResource() resource.Resource {
	return &memberNicknameResource{}
}

type memberNicknameResource struct {
	client *client.Client
}

type memberNicknameResourceModel struct {
	ServerID types.String `tfsdk:"server_id"`
	UserID   types.String `tfsdk:"user_id"`
	Nick     types.String `tfsdk:"nick"`
}

func (r *memberNicknameResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_member_nickname"
}

func (r *memberNicknameResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the nickname of a guild member. Applying sets the member's nickname; destroying clears it back to the user's default name.",
		Attributes: map[string]schema.Attribute{
			"server_id": schema.StringAttribute{
				MarkdownDescription: "Snowflake ID of the guild.",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"user_id": schema.StringAttribute{
				MarkdownDescription: "Snowflake ID of the member's user.",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"nick": schema.StringAttribute{
				MarkdownDescription: "Nickname to set for the member.",
				Required:            true,
			},
		},
	}
}

func (r *memberNicknameResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// writePath resolves the member-modify endpoint. Changing the bot's OWN nickname
// must go through /members/@me (needs CHANGE_NICKNAME); any other member uses
// /members/{user_id} (needs MANAGE_NICKNAMES). Reads always use /members/{user_id}.
func (r *memberNicknameResource) writePath(ctx context.Context, m *memberNicknameResourceModel) string {
	if botID, err := r.client.BotUserID(ctx); err == nil && botID == m.UserID.ValueString() {
		return "/guilds/" + m.ServerID.ValueString() + "/members/@me"
	}
	return memberPath(m.ServerID.ValueString(), m.UserID.ValueString())
}

// apply PATCHes the member with the desired nick.
func (r *memberNicknameResource) apply(ctx context.Context, m *memberNicknameResourceModel) error {
	return r.client.Write(ctx, "PATCH", r.writePath(ctx, m), map[string]any{"nick": m.Nick.ValueString()}, nil)
}

func (r *memberNicknameResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan memberNicknameResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.apply(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Unable to set Discord member nickname", err.Error())
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Unable to read member after setting nickname", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *memberNicknameResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state memberNicknameResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.readInto(ctx, &state); err != nil {
		if notFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read Discord member nickname", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *memberNicknameResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan memberNicknameResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.apply(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Unable to update Discord member nickname", err.Error())
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Unable to read member after updating nickname", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete clears the member's nickname back to the user's default.
func (r *memberNicknameResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state memberNicknameResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.Write(ctx, "PATCH", r.writePath(ctx, &state), map[string]any{"nick": nil}, nil); err != nil && !notFound(err) {
		resp.Diagnostics.AddError("Unable to clear Discord member nickname", err.Error())
	}
}

// ImportState accepts "server_id/user_id".
func (r *memberNicknameResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, "/")
	if len(parts) != 2 {
		resp.Diagnostics.AddError("Invalid import ID", "Expected \"server_id/user_id\".")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("server_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("user_id"), parts[1])...)
}

// readInto GETs the member and refreshes the nickname from the live object.
func (r *memberNicknameResource) readInto(ctx context.Context, m *memberNicknameResourceModel) error {
	var a memberAttributes
	if err := r.client.Get(ctx, memberPath(m.ServerID.ValueString(), m.UserID.ValueString()), &a); err != nil {
		return err
	}
	m.Nick = types.StringPointerValue(a.Nick)
	return nil
}
