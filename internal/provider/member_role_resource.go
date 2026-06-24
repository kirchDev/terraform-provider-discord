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

// --- Non-authoritative single role assignment. Unlike discord_member_roles
// (which owns a member's whole role set), this manages exactly ONE (member, role)
// pair via the add/remove-role endpoints and leaves every other role — including
// ones granted by bots (level/reaction-role bots) — untouched. Use it when
// something other than Terraform also assigns roles to the same members. ---

var (
	_ resource.Resource                = (*memberRoleResource)(nil)
	_ resource.ResourceWithConfigure   = (*memberRoleResource)(nil)
	_ resource.ResourceWithImportState = (*memberRoleResource)(nil)
)

// NewMemberRoleResource returns a new discord_member_role resource.
func NewMemberRoleResource() resource.Resource {
	return &memberRoleResource{}
}

type memberRoleResource struct {
	client *client.Client
}

type memberRoleResourceModel struct {
	ServerID types.String `tfsdk:"server_id"`
	UserID   types.String `tfsdk:"user_id"`
	RoleID   types.String `tfsdk:"role_id"`
}

func (r *memberRoleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_member_role"
}

func (r *memberRoleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Assigns a single role to a guild member, non-authoritatively. It manages only this one " +
			"`(member, role)` pair and leaves the member's other roles alone — so it coexists with bots that also grant " +
			"roles. For exclusive ownership of a member's entire role set, use `discord_member_roles` instead.",
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
			"role_id": schema.StringAttribute{
				MarkdownDescription: "Snowflake ID of the role to assign.",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
		},
	}
}

func (r *memberRoleResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *memberRoleResource) rolePath(m *memberRoleResourceModel) string {
	return memberPath(m.ServerID.ValueString(), m.UserID.ValueString()) + "/roles/" + m.RoleID.ValueString()
}

func (r *memberRoleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan memberRoleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// Add-member-role takes no body and returns 204.
	if err := r.client.Write(ctx, "PUT", r.rolePath(&plan), nil, nil); err != nil {
		resp.Diagnostics.AddError("Unable to assign Discord member role", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *memberRoleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state memberRoleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	has, err := r.memberHasRole(ctx, &state)
	if err != nil {
		if notFound(err) { // member is gone from the guild
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read Discord member role", err.Error())
		return
	}
	if !has { // member no longer has the role
		resp.State.RemoveResource(ctx)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update never runs: every attribute is RequiresReplace. It exists only to
// satisfy the resource.Resource interface.
func (r *memberRoleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan memberRoleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *memberRoleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state memberRoleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.Delete(ctx, r.rolePath(&state)); err != nil && !notFound(err) {
		resp.Diagnostics.AddError("Unable to remove Discord member role", err.Error())
	}
}

// ImportState accepts "server_id/user_id/role_id".
func (r *memberRoleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, "/")
	if len(parts) != 3 {
		resp.Diagnostics.AddError("Invalid import ID", "Expected \"server_id/user_id/role_id\".")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("server_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("user_id"), parts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("role_id"), parts[2])...)
}

// memberHasRole reports whether the member currently has the role. A 404 on the
// member read is surfaced as an error for the caller to classify via notFound.
func (r *memberRoleResource) memberHasRole(ctx context.Context, m *memberRoleResourceModel) (bool, error) {
	var a memberAttributes
	if err := r.client.Get(ctx, memberPath(m.ServerID.ValueString(), m.UserID.ValueString()), &a); err != nil {
		return false, err
	}
	for _, id := range a.Roles {
		if id == m.RoleID.ValueString() {
			return true, nil
		}
	}
	return false, nil
}
