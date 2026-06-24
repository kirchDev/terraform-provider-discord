package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/kirchDev/terraform-provider-discord/internal/client"
)

// --- Bans a user from a guild. A ban has no mutable settings — delete_message_seconds
// and reason are create-only inputs that Discord never returns — so every input
// forces replacement and Update is a no-op re-Set. ---

var (
	_ resource.Resource                = (*guildBanResource)(nil)
	_ resource.ResourceWithConfigure   = (*guildBanResource)(nil)
	_ resource.ResourceWithImportState = (*guildBanResource)(nil)
)

// NewGuildBanResource returns a new discord_guild_ban resource.
func NewGuildBanResource() resource.Resource {
	return &guildBanResource{}
}

type guildBanResource struct {
	client *client.Client
}

type guildBanResourceModel struct {
	ServerID             types.String `tfsdk:"server_id"`
	UserID               types.String `tfsdk:"user_id"`
	DeleteMessageSeconds types.Int64  `tfsdk:"delete_message_seconds"`
	Reason               types.String `tfsdk:"reason"`
}

func (r *guildBanResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_guild_ban"
}

func (r *guildBanResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Bans a user from a Discord guild. Destroying the resource unbans the user.",
		Attributes: map[string]schema.Attribute{
			"server_id": schema.StringAttribute{
				MarkdownDescription: "Snowflake ID of the guild.",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"user_id": schema.StringAttribute{
				MarkdownDescription: "Snowflake ID of the user to ban.",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"delete_message_seconds": schema.Int64Attribute{
				MarkdownDescription: "Number of seconds of the user's recent messages to delete on ban (0–604800). Applied only at creation.",
				Optional:            true,
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.RequiresReplace()},
			},
			"reason": schema.StringAttribute{
				MarkdownDescription: "Audit log reason for the ban. Applied only at creation and not refreshed from the API.",
				Optional:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
		},
	}
}

func (r *guildBanResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// banPath is the create/read/delete endpoint for a single guild ban.
func banPath(serverID, userID string) string {
	return "/guilds/" + serverID + "/bans/" + userID
}

func (r *guildBanResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan guildBanResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := map[string]any{}
	if v := plan.DeleteMessageSeconds; !v.IsNull() && !v.IsUnknown() {
		body["delete_message_seconds"] = v.ValueInt64()
	}
	if err := r.client.Write(ctx, "PUT", banPath(plan.ServerID.ValueString(), plan.UserID.ValueString()), body, nil); err != nil {
		resp.Diagnostics.AddError("Unable to create Discord guild ban", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *guildBanResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state guildBanResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var ban struct {
		User struct {
			ID string `json:"id"`
		} `json:"user"`
	}
	if err := r.client.Get(ctx, banPath(state.ServerID.ValueString(), state.UserID.ValueString()), &ban); err != nil {
		if notFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read Discord guild ban", err.Error())
		return
	}
	// reason and delete_message_seconds are create-only and not returned by the
	// API, so they're left as-is in state.
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update has no mutable fields (every input forces replacement); it just re-Sets state.
func (r *guildBanResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan guildBanResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *guildBanResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state guildBanResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.Delete(ctx, banPath(state.ServerID.ValueString(), state.UserID.ValueString())); err != nil && !notFound(err) {
		resp.Diagnostics.AddError("Unable to delete Discord guild ban", err.Error())
	}
}

// ImportState accepts "server_id/user_id".
func (r *guildBanResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, "/")
	if len(parts) != 2 {
		resp.Diagnostics.AddError("Invalid import ID", "Expected \"server_id/user_id\".")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("server_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("user_id"), parts[1])...)
}
