package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/kirchDev/terraform-provider-discord/internal/client"
)

// --- The @everyone base role. It always exists, cannot be created or deleted,
// and its id equals the guild id. Like discord_managed_server it adopts rather
// than creates: Create PATCHes its permissions, Delete is a no-op. ---

var (
	_ resource.Resource                = (*roleEveryoneResource)(nil)
	_ resource.ResourceWithConfigure   = (*roleEveryoneResource)(nil)
	_ resource.ResourceWithImportState = (*roleEveryoneResource)(nil)
)

// NewRoleEveryoneResource returns a new discord_role_everyone resource.
func NewRoleEveryoneResource() resource.Resource {
	return &roleEveryoneResource{}
}

type roleEveryoneResource struct {
	client *client.Client
}

type roleEveryoneResourceModel struct {
	ServerID    types.String `tfsdk:"server_id"`
	ID          types.String `tfsdk:"id"`
	Permissions types.String `tfsdk:"permissions"`
}

func (r *roleEveryoneResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_role_everyone"
}

func (r *roleEveryoneResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the `@everyone` base role of a Discord guild. This role always exists and " +
			"cannot be created or deleted — its id equals the guild id. Destroying the resource only removes it from " +
			"state; the role's permissions are left as they are.",
		Attributes: map[string]schema.Attribute{
			"server_id": schema.StringAttribute{
				MarkdownDescription: "Snowflake ID of the guild. Also the id of the `@everyone` role.",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "Role ID (equal to `server_id`).",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"permissions": schema.StringAttribute{
				MarkdownDescription: "Permission bitfield as a decimal string. See the `discord_permission` data source.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *roleEveryoneResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *roleEveryoneResource) apply(ctx context.Context, m *roleEveryoneResourceModel) error {
	if m.Permissions.IsNull() || m.Permissions.IsUnknown() {
		return nil
	}
	itemPath := "/guilds/" + m.ServerID.ValueString() + "/roles/" + m.ServerID.ValueString()
	return r.client.Write(ctx, "PATCH", itemPath, map[string]any{"permissions": m.Permissions.ValueString()}, nil)
}

func (r *roleEveryoneResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan roleEveryoneResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.apply(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Unable to set @everyone permissions", err.Error())
		return
	}
	plan.ID = plan.ServerID
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Unable to read @everyone role after apply", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *roleEveryoneResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state roleEveryoneResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.readInto(ctx, &state); err != nil {
		if notFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read @everyone role", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *roleEveryoneResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan roleEveryoneResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.apply(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Unable to update @everyone permissions", err.Error())
		return
	}
	plan.ID = plan.ServerID
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Unable to read @everyone role after update", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete is a no-op: the @everyone role cannot be deleted.
func (r *roleEveryoneResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

// ImportState accepts the guild id (which is also the @everyone role id).
func (r *roleEveryoneResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("server_id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

func (r *roleEveryoneResource) readInto(ctx context.Context, m *roleEveryoneResourceModel) error {
	guildID := m.ServerID.ValueString()
	a, err := findInList(ctx, r.client, "/guilds/"+guildID+"/roles", guildID, func(a *roleAttributes) string { return a.ID })
	if err != nil {
		return err
	}
	m.ID = m.ServerID
	m.Permissions = types.StringValue(a.Permissions)
	return nil
}
