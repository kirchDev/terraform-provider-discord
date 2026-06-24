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

var (
	_ resource.Resource                = (*guildTemplateResource)(nil)
	_ resource.ResourceWithConfigure   = (*guildTemplateResource)(nil)
	_ resource.ResourceWithImportState = (*guildTemplateResource)(nil)
)

// NewGuildTemplateResource returns a new discord_guild_template resource.
func NewGuildTemplateResource() resource.Resource {
	return &guildTemplateResource{}
}

type guildTemplateResource struct {
	client *client.Client
}

// templateWire mirrors a Discord guild template object. The template is
// addressed by its short string code, not a snowflake id.
type templateWire struct {
	Code        string  `json:"code"`
	Name        string  `json:"name"`
	Description *string `json:"description"`
	UsageCount  int64   `json:"usage_count"`
}

type guildTemplateResourceModel struct {
	ServerID    types.String `tfsdk:"server_id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Code        types.String `tfsdk:"code"`
	UsageCount  types.Int64  `tfsdk:"usage_count"`
}

func (r *guildTemplateResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_guild_template"
}

func (r *guildTemplateResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a guild template — a reusable snapshot of a Discord guild's structure.",
		Attributes: map[string]schema.Attribute{
			"server_id": schema.StringAttribute{
				MarkdownDescription: "Snowflake ID of the source guild the template is created from.",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Template name.",
				Required:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Template description.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"code": schema.StringAttribute{
				MarkdownDescription: "Short template code used to share the template.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"usage_count": schema.Int64Attribute{
				MarkdownDescription: "Number of times the template has been used.",
				Computed:            true,
			},
		},
	}
}

func (r *guildTemplateResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *guildTemplateResource) basePath(m *guildTemplateResourceModel) string {
	return "/guilds/" + m.ServerID.ValueString() + "/templates"
}

// body collects the settable template fields from the plan.
func (r *guildTemplateResource) body(m *guildTemplateResourceModel) map[string]any {
	body := map[string]any{"name": m.Name.ValueString()}
	if v := m.Description; !v.IsNull() && !v.IsUnknown() {
		body["description"] = v.ValueString()
	}
	return body
}

func (r *guildTemplateResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan guildTemplateResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var created templateWire
	if err := r.client.Write(ctx, "POST", r.basePath(&plan), r.body(&plan), &created); err != nil {
		resp.Diagnostics.AddError("Unable to create Discord guild template", err.Error())
		return
	}
	plan.Code = types.StringValue(created.Code)

	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Unable to read guild template after create", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *guildTemplateResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state guildTemplateResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.readInto(ctx, &state); err != nil {
		if notFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read Discord guild template", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *guildTemplateResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan guildTemplateResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	itemPath := r.basePath(&plan) + "/" + plan.Code.ValueString()
	if err := r.client.Write(ctx, "PATCH", itemPath, r.body(&plan), nil); err != nil {
		resp.Diagnostics.AddError("Unable to update Discord guild template", err.Error())
		return
	}

	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Unable to read guild template after update", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *guildTemplateResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state guildTemplateResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	itemPath := r.basePath(&state) + "/" + state.Code.ValueString()
	if err := r.client.Delete(ctx, itemPath); err != nil && !notFound(err) {
		resp.Diagnostics.AddError("Unable to delete Discord guild template", err.Error())
	}
}

// ImportState accepts "server_id/code".
func (r *guildTemplateResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, "/")
	if len(parts) != 2 {
		resp.Diagnostics.AddError("Invalid import ID", "Expected \"server_id/code\".")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("server_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("code"), parts[1])...)
}

// readInto finds the template within the guild template collection (Discord has
// no per-template read) and fills the model.
func (r *guildTemplateResource) readInto(ctx context.Context, m *guildTemplateResourceModel) error {
	a, err := findInList(ctx, r.client, r.basePath(m), m.Code.ValueString(), func(a *templateWire) string { return a.Code })
	if err != nil {
		return err
	}
	m.Name = types.StringValue(a.Name)
	m.Description = types.StringPointerValue(a.Description)
	m.UsageCount = types.Int64Value(a.UsageCount)
	return nil
}
