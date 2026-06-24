package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/kirchDev/terraform-provider-discord/internal/client"
)

// --- Application (slash) command, global or guild-scoped. Command options are
// recursive, so they are passed through as raw JSON in `options_json`
// (write-only; not refreshed). A null guild_id registers the command globally. ---

var (
	_ resource.Resource                = (*applicationCommandResource)(nil)
	_ resource.ResourceWithConfigure   = (*applicationCommandResource)(nil)
	_ resource.ResourceWithImportState = (*applicationCommandResource)(nil)
)

// NewApplicationCommandResource returns a new discord_application_command resource.
func NewApplicationCommandResource() resource.Resource {
	return &applicationCommandResource{}
}

type applicationCommandResource struct {
	client *client.Client
}

type applicationCommandWire struct {
	ID                       string  `json:"id"`
	Name                     string  `json:"name"`
	Description              string  `json:"description"`
	Type                     int64   `json:"type"`
	DefaultMemberPermissions *string `json:"default_member_permissions"`
	DMPermission             *bool   `json:"dm_permission"`
	NSFW                     bool    `json:"nsfw"`
	Contexts                 []int64 `json:"contexts"`
	IntegrationTypes         []int64 `json:"integration_types"`
}

type applicationCommandResourceModel struct {
	ApplicationID            types.String `tfsdk:"application_id"`
	GuildID                  types.String `tfsdk:"guild_id"`
	ID                       types.String `tfsdk:"id"`
	Name                     types.String `tfsdk:"name"`
	Description              types.String `tfsdk:"description"`
	Type                     types.Int64  `tfsdk:"type"`
	OptionsJSON              types.String `tfsdk:"options_json"`
	DefaultMemberPermissions types.String `tfsdk:"default_member_permissions"`
	DMPermission             types.Bool   `tfsdk:"dm_permission"`
	NSFW                     types.Bool   `tfsdk:"nsfw"`
	NameLocalizations        types.Map    `tfsdk:"name_localizations"`
	DescriptionLocalizations types.Map    `tfsdk:"description_localizations"`
	Contexts                 types.Set    `tfsdk:"contexts"`
	IntegrationTypes         types.Set    `tfsdk:"integration_types"`
}

func (r *applicationCommandResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_application_command"
}

func (r *applicationCommandResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages an application (slash) command. With `guild_id` set the command is registered to that " +
			"guild; without it the command is registered globally.",
		Attributes: map[string]schema.Attribute{
			"application_id": schema.StringAttribute{
				MarkdownDescription: "Snowflake ID of the application (bot).",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"guild_id": schema.StringAttribute{
				MarkdownDescription: "Snowflake ID of the guild to scope the command to. Omit for a global command. Changing it replaces the command.",
				Optional:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "Snowflake ID of the command.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name":        schema.StringAttribute{MarkdownDescription: "Command name (1–32 chars).", Required: true},
			"description": schema.StringAttribute{MarkdownDescription: "Command description (required for CHAT_INPUT commands).", Optional: true, Computed: true},
			"type": schema.Int64Attribute{
				MarkdownDescription: "Command type (`1` chat input, `2` user, `3` message). Changing it replaces the command.",
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(1),
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.RequiresReplace()},
				Validators:          []validator.Int64{int64OneOf(1, 2, 3, 4)},
			},
			"options_json": schema.StringAttribute{
				MarkdownDescription: "Command options as a raw JSON array (Discord's `options` field). Write-only — sent on apply but not refreshed.",
				Optional:            true,
			},
			"default_member_permissions": schema.StringAttribute{
				MarkdownDescription: "Permission bitfield (decimal string) a member needs to use the command. See the `discord_permission` data source.",
				Optional:            true,
			},
			"dm_permission": schema.BoolAttribute{
				MarkdownDescription: "Whether the command is usable in DMs (global commands only).",
				Optional:            true,
				Computed:            true,
			},
			"nsfw": schema.BoolAttribute{
				MarkdownDescription: "Whether the command is age-restricted.",
				Optional:            true,
				Computed:            true,
			},
			"name_localizations": schema.MapAttribute{
				MarkdownDescription: "Localized command names keyed by locale (e.g. `de`, `fr`). Write-only — not refreshed.",
				ElementType:         types.StringType,
				Optional:            true,
			},
			"description_localizations": schema.MapAttribute{
				MarkdownDescription: "Localized command descriptions keyed by locale. Write-only — not refreshed.",
				ElementType:         types.StringType,
				Optional:            true,
			},
			"contexts": schema.SetAttribute{
				MarkdownDescription: "Interaction contexts the command is available in (`0` guild, `1` bot DM, `2` private channel).",
				ElementType:         types.Int64Type,
				Optional:            true,
				Computed:            true,
			},
			"integration_types": schema.SetAttribute{
				MarkdownDescription: "Installation contexts where the command is available (`0` guild install, `1` user install).",
				ElementType:         types.Int64Type,
				Optional:            true,
				Computed:            true,
			},
		},
	}
}

func (r *applicationCommandResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// commandsPath is the create/list endpoint; itemPath appends the command id.
func (r *applicationCommandResource) commandsPath(m *applicationCommandResourceModel) string {
	if !m.GuildID.IsNull() && !m.GuildID.IsUnknown() && m.GuildID.ValueString() != "" {
		return "/applications/" + m.ApplicationID.ValueString() + "/guilds/" + m.GuildID.ValueString() + "/commands"
	}
	return "/applications/" + m.ApplicationID.ValueString() + "/commands"
}

func (r *applicationCommandResource) body(ctx context.Context, m *applicationCommandResourceModel) (map[string]any, error) {
	body := map[string]any{"name": m.Name.ValueString(), "type": m.Type.ValueInt64()}
	if v := m.Description; !v.IsNull() && !v.IsUnknown() {
		body["description"] = v.ValueString()
	}
	if v := m.DefaultMemberPermissions; !v.IsNull() && !v.IsUnknown() {
		body["default_member_permissions"] = v.ValueString()
	}
	if v := m.DMPermission; !v.IsNull() && !v.IsUnknown() {
		body["dm_permission"] = v.ValueBool()
	}
	if v := m.NSFW; !v.IsNull() && !v.IsUnknown() {
		body["nsfw"] = v.ValueBool()
	}
	if v := m.OptionsJSON; !v.IsNull() && !v.IsUnknown() {
		var options any
		if err := json.Unmarshal([]byte(v.ValueString()), &options); err != nil {
			return nil, fmt.Errorf("options_json is not valid JSON: %w", err)
		}
		body["options"] = options
	}
	if v := m.NameLocalizations; !v.IsNull() && !v.IsUnknown() {
		locs := map[string]string{}
		if d := v.ElementsAs(ctx, &locs, false); d.HasError() {
			return nil, fmt.Errorf("reading name_localizations")
		}
		body["name_localizations"] = locs
	}
	if v := m.DescriptionLocalizations; !v.IsNull() && !v.IsUnknown() {
		locs := map[string]string{}
		if d := v.ElementsAs(ctx, &locs, false); d.HasError() {
			return nil, fmt.Errorf("reading description_localizations")
		}
		body["description_localizations"] = locs
	}
	if v := m.Contexts; !v.IsNull() && !v.IsUnknown() {
		var ctxs []int64
		if d := v.ElementsAs(ctx, &ctxs, false); d.HasError() {
			return nil, fmt.Errorf("reading contexts")
		}
		body["contexts"] = ctxs
	}
	if v := m.IntegrationTypes; !v.IsNull() && !v.IsUnknown() {
		var its []int64
		if d := v.ElementsAs(ctx, &its, false); d.HasError() {
			return nil, fmt.Errorf("reading integration_types")
		}
		body["integration_types"] = its
	}
	return body, nil
}

func (r *applicationCommandResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan applicationCommandResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body, err := r.body(ctx, &plan)
	if err != nil {
		resp.Diagnostics.AddError("Invalid application command configuration", err.Error())
		return
	}
	var created applicationCommandWire
	if err := r.client.Write(ctx, "POST", r.commandsPath(&plan), body, &created); err != nil {
		resp.Diagnostics.AddError("Unable to create Discord application command", err.Error())
		return
	}
	plan.ID = types.StringValue(created.ID)
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Unable to read application command after create", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *applicationCommandResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state applicationCommandResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.readInto(ctx, &state); err != nil {
		if notFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read Discord application command", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *applicationCommandResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan applicationCommandResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body, err := r.body(ctx, &plan)
	if err != nil {
		resp.Diagnostics.AddError("Invalid application command configuration", err.Error())
		return
	}
	delete(body, "type") // type is immutable
	if err := r.client.Write(ctx, "PATCH", r.commandsPath(&plan)+"/"+plan.ID.ValueString(), body, nil); err != nil {
		resp.Diagnostics.AddError("Unable to update Discord application command", err.Error())
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Unable to read application command after update", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *applicationCommandResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state applicationCommandResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.Delete(ctx, r.commandsPath(&state)+"/"+state.ID.ValueString()); err != nil && !notFound(err) {
		resp.Diagnostics.AddError("Unable to delete Discord application command", err.Error())
	}
}

// ImportState accepts "application_id/command_id" (global) or
// "application_id/guild_id/command_id" (guild).
func (r *applicationCommandResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, "/")
	switch len(parts) {
	case 2:
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("application_id"), parts[0])...)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[1])...)
	case 3:
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("application_id"), parts[0])...)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("guild_id"), parts[1])...)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[2])...)
	default:
		resp.Diagnostics.AddError("Invalid import ID", "Expected \"application_id/command_id\" or \"application_id/guild_id/command_id\".")
	}
}

// readInto refreshes the scalar fields (options_json is write-only).
func (r *applicationCommandResource) readInto(ctx context.Context, m *applicationCommandResourceModel) error {
	var a applicationCommandWire
	if err := r.client.Get(ctx, r.commandsPath(m)+"/"+m.ID.ValueString(), &a); err != nil {
		return err
	}
	m.Name = types.StringValue(a.Name)
	m.Description = types.StringValue(a.Description)
	m.Type = types.Int64Value(a.Type)
	m.DefaultMemberPermissions = types.StringPointerValue(a.DefaultMemberPermissions)
	if a.DMPermission != nil {
		m.DMPermission = types.BoolValue(*a.DMPermission)
	}
	m.NSFW = types.BoolValue(a.NSFW)
	ctxs, d1 := types.SetValueFrom(ctx, types.Int64Type, a.Contexts)
	if d1.HasError() {
		return fmt.Errorf("building contexts state")
	}
	m.Contexts = ctxs
	its, d2 := types.SetValueFrom(ctx, types.Int64Type, a.IntegrationTypes)
	if d2.HasError() {
		return fmt.Errorf("building integration_types state")
	}
	m.IntegrationTypes = its
	return nil
}
