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

// --- Authoritative member → role assignment. This resource OWNS the full role
// set of a member (minus the implicit @everyone): applying it sets exactly the
// listed roles, and destroying it clears them. Two known upstream bugs are
// designed out here: drift is detected by reading the live `roles` array back
// into the set on every Read, and ImportState ("server_id/user_id") hydrates that
// set so an imported resource is immediately consistent. ---

var (
	_ resource.Resource                = (*memberRolesResource)(nil)
	_ resource.ResourceWithConfigure   = (*memberRolesResource)(nil)
	_ resource.ResourceWithImportState = (*memberRolesResource)(nil)
)

// NewMemberRolesResource returns a new discord_member_roles resource.
func NewMemberRolesResource() resource.Resource {
	return &memberRolesResource{}
}

type memberRolesResource struct {
	client *client.Client
}

type memberRolesResourceModel struct {
	ServerID types.String `tfsdk:"server_id"`
	UserID   types.String `tfsdk:"user_id"`
	Roles    types.Set    `tfsdk:"roles"`
}

func (r *memberRolesResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_member_roles"
}

func (r *memberRolesResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Authoritatively manages the set of roles assigned to a guild member. This resource owns " +
			"the member's full role set (excluding the implicit `@everyone` role): it sets exactly the listed roles and " +
			"clears them on destroy. Roles not listed here are **removed** from the member.",
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
			"roles": schema.SetAttribute{
				MarkdownDescription: "Snowflake IDs of the roles the member should have (excluding `@everyone`).",
				ElementType:         types.StringType,
				Required:            true,
			},
		},
	}
}

func (r *memberRolesResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// apply PATCHes the member with the desired authoritative role set.
func (r *memberRolesResource) apply(ctx context.Context, m *memberRolesResourceModel, roles []string) error {
	if roles == nil {
		roles = []string{}
	}
	return r.client.Write(ctx, "PATCH", memberPath(m.ServerID.ValueString(), m.UserID.ValueString()), map[string]any{"roles": roles}, nil)
}

func (r *memberRolesResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan memberRolesResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var roles []string
	resp.Diagnostics.Append(plan.Roles.ElementsAs(ctx, &roles, false)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.apply(ctx, &plan, roles); err != nil {
		resp.Diagnostics.AddError("Unable to set Discord member roles", err.Error())
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Unable to read member after setting roles", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *memberRolesResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state memberRolesResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.readInto(ctx, &state); err != nil {
		if notFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read Discord member roles", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *memberRolesResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan memberRolesResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var roles []string
	resp.Diagnostics.Append(plan.Roles.ElementsAs(ctx, &roles, false)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.apply(ctx, &plan, roles); err != nil {
		resp.Diagnostics.AddError("Unable to update Discord member roles", err.Error())
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Unable to read member after updating roles", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete clears the member's roles (authoritative ownership of the role set).
func (r *memberRolesResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state memberRolesResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.apply(ctx, &state, []string{}); err != nil && !notFound(err) {
		resp.Diagnostics.AddError("Unable to clear Discord member roles", err.Error())
	}
}

// ImportState accepts "server_id/user_id".
func (r *memberRolesResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, "/")
	if len(parts) != 2 {
		resp.Diagnostics.AddError("Invalid import ID", "Expected \"server_id/user_id\".")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("server_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("user_id"), parts[1])...)
}

// readInto GETs the member and replaces the role set with the live `roles` array,
// giving accurate drift detection.
func (r *memberRolesResource) readInto(ctx context.Context, m *memberRolesResourceModel) error {
	var a memberAttributes
	if err := r.client.Get(ctx, memberPath(m.ServerID.ValueString(), m.UserID.ValueString()), &a); err != nil {
		return err
	}
	roles := a.Roles
	if roles == nil {
		roles = []string{}
	}
	set, d := types.SetValueFrom(ctx, types.StringType, roles)
	if d.HasError() {
		return fmt.Errorf("building role set: %v", d.Errors())
	}
	m.Roles = set
	return nil
}
