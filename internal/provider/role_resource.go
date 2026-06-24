package provider

import (
	"context"
	"fmt"
	"strings"

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

// --- CRUD-resource exemplar. New entities follow this shape: an *Attributes
// struct (the Discord wire object), a *ResourceModel (the tfsdk state), and
// Create/Read/Update/Delete/ImportState + a readInto helper. ---

var (
	_ resource.Resource                = (*roleResource)(nil)
	_ resource.ResourceWithConfigure   = (*roleResource)(nil)
	_ resource.ResourceWithImportState = (*roleResource)(nil)
)

// NewRoleResource returns a new discord_role resource.
func NewRoleResource() resource.Resource {
	return &roleResource{}
}

type roleResource struct {
	client *client.Client
}

// roleAttributes mirrors a Discord role object. Permissions is a decimal string
// bitfield (a 64-bit number would lose precision as a JSON number).
type roleAttributes struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	Color        int64   `json:"color"`
	Hoist        bool    `json:"hoist"`
	Position     int64   `json:"position"`
	Permissions  string  `json:"permissions"`
	Managed      bool    `json:"managed"`
	Mentionable  bool    `json:"mentionable"`
	UnicodeEmoji *string `json:"unicode_emoji"`
	Description  *string `json:"description"`
	Icon         *string `json:"icon"`
}

type roleResourceModel struct {
	ServerID     types.String `tfsdk:"server_id"`
	ID           types.String `tfsdk:"id"`
	Name         types.String `tfsdk:"name"`
	Color        types.Int64  `tfsdk:"color"`
	Hoist        types.Bool   `tfsdk:"hoist"`
	Position     types.Int64  `tfsdk:"position"`
	Permissions  types.String `tfsdk:"permissions"`
	Mentionable  types.Bool   `tfsdk:"mentionable"`
	Managed      types.Bool   `tfsdk:"managed"`
	UnicodeEmoji types.String `tfsdk:"unicode_emoji"`
	Description  types.String `tfsdk:"description"`
	IconDataURI  types.String `tfsdk:"icon_data_uri"`
	IconHash     types.String `tfsdk:"icon_hash"`
}

func (r *roleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_role"
}

func (r *roleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a role within a Discord guild.",
		Attributes: map[string]schema.Attribute{
			"server_id": schema.StringAttribute{
				MarkdownDescription: "Snowflake ID of the guild the role belongs to.",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "Snowflake ID of the role.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Role name.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"color": schema.Int64Attribute{
				MarkdownDescription: "Role color as a decimal RGB integer (`0` leaves the color unset). See the `discord_color` data source.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
				Validators:          []validator.Int64{int64Between(0, 16777215)},
			},
			"hoist": schema.BoolAttribute{
				MarkdownDescription: "Whether the role is shown separately in the member list.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
			"position": schema.Int64Attribute{
				MarkdownDescription: "Position of the role in the hierarchy (higher is more senior). Discord assigns one on create; set this to enforce a position.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
			"permissions": schema.StringAttribute{
				MarkdownDescription: "Permission bitfield as a decimal string. See the `discord_permission` data source.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"mentionable": schema.BoolAttribute{
				MarkdownDescription: "Whether the role can be mentioned by anyone.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
			"unicode_emoji": schema.StringAttribute{
				MarkdownDescription: "Standard unicode emoji shown as the role's icon (needs the `ROLE_ICONS` guild feature).",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Role description.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"icon_data_uri": schema.StringAttribute{
				MarkdownDescription: "Role icon image as a base64 data URI (needs the `ROLE_ICONS` guild feature; mutually exclusive with `unicode_emoji`). Write-only; Discord returns a hash in `icon_hash`.",
				Optional:            true,
			},
			"icon_hash": schema.StringAttribute{MarkdownDescription: "Current role icon hash.", Computed: true},
			"managed": schema.BoolAttribute{
				MarkdownDescription: "Whether the role is managed by an integration (read-only).",
				Computed:            true,
			},
		},
	}
}

func (r *roleResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *roleResource) rolesPath(m *roleResourceModel) string {
	return "/guilds/" + m.ServerID.ValueString() + "/roles"
}

// body collects the settable role fields from the plan (everything but position,
// which has its own endpoint).
func (r *roleResource) body(m *roleResourceModel) map[string]any {
	body := map[string]any{}
	if v := m.Name; !v.IsNull() && !v.IsUnknown() {
		body["name"] = v.ValueString()
	}
	if v := m.Color; !v.IsNull() && !v.IsUnknown() {
		body["color"] = v.ValueInt64()
	}
	if v := m.Hoist; !v.IsNull() && !v.IsUnknown() {
		body["hoist"] = v.ValueBool()
	}
	if v := m.Permissions; !v.IsNull() && !v.IsUnknown() {
		body["permissions"] = v.ValueString()
	}
	if v := m.Mentionable; !v.IsNull() && !v.IsUnknown() {
		body["mentionable"] = v.ValueBool()
	}
	if v := m.UnicodeEmoji; !v.IsNull() && !v.IsUnknown() {
		body["unicode_emoji"] = v.ValueString()
	}
	if v := m.Description; !v.IsNull() && !v.IsUnknown() {
		body["description"] = v.ValueString()
	}
	if v := m.IconDataURI; !v.IsNull() && !v.IsUnknown() {
		body["icon"] = v.ValueString()
	}
	return body
}

func (r *roleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan roleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var created roleAttributes
	if err := r.client.Write(ctx, "POST", r.rolesPath(&plan), r.body(&plan), &created); err != nil {
		resp.Diagnostics.AddError("Unable to create Discord role", err.Error())
		return
	}
	plan.ID = types.StringValue(created.ID)

	if !plan.Position.IsNull() && !plan.Position.IsUnknown() {
		if err := r.setPosition(ctx, &plan); err != nil {
			resp.Diagnostics.AddError("Unable to set Discord role position", err.Error())
			return
		}
	}

	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Unable to read role after create", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *roleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state roleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.readInto(ctx, &state); err != nil {
		if notFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read Discord role", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *roleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan roleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	itemPath := r.rolesPath(&plan) + "/" + plan.ID.ValueString()
	if err := r.client.Write(ctx, "PATCH", itemPath, r.body(&plan), nil); err != nil {
		resp.Diagnostics.AddError("Unable to update Discord role", err.Error())
		return
	}
	if !plan.Position.IsNull() && !plan.Position.IsUnknown() {
		if err := r.setPosition(ctx, &plan); err != nil {
			resp.Diagnostics.AddError("Unable to set Discord role position", err.Error())
			return
		}
	}

	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Unable to read role after update", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *roleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state roleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	itemPath := r.rolesPath(&state) + "/" + state.ID.ValueString()
	if err := r.client.Delete(ctx, itemPath); err != nil && !notFound(err) {
		resp.Diagnostics.AddError("Unable to delete Discord role", err.Error())
	}
}

// ImportState accepts "server_id/role_id".
func (r *roleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, "/")
	if len(parts) != 2 {
		resp.Diagnostics.AddError("Invalid import ID", "Expected \"server_id/role_id\".")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("server_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[1])...)
}

// setPosition applies the desired role position via the modify-role-positions
// endpoint (PATCH on the guild role collection with a [{id, position}] array).
func (r *roleResource) setPosition(ctx context.Context, m *roleResourceModel) error {
	body := []map[string]any{{"id": m.ID.ValueString(), "position": m.Position.ValueInt64()}}
	return r.client.Write(ctx, "PATCH", r.rolesPath(m), body, nil)
}

// readInto finds the role within the guild role collection (Discord has no clean
// per-role read across versions) and fills the model.
func (r *roleResource) readInto(ctx context.Context, m *roleResourceModel) error {
	a, err := findInList(ctx, r.client, r.rolesPath(m), m.ID.ValueString(), func(a *roleAttributes) string { return a.ID })
	if err != nil {
		return err
	}
	m.Name = types.StringValue(a.Name)
	m.Color = types.Int64Value(a.Color)
	m.Hoist = types.BoolValue(a.Hoist)
	m.Position = types.Int64Value(a.Position)
	m.Permissions = types.StringValue(a.Permissions)
	m.Mentionable = types.BoolValue(a.Mentionable)
	m.Managed = types.BoolValue(a.Managed)
	m.UnicodeEmoji = types.StringPointerValue(a.UnicodeEmoji)
	m.Description = types.StringPointerValue(a.Description)
	m.IconHash = types.StringPointerValue(a.Icon)
	return nil
}
