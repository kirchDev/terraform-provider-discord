package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/kirchDev/terraform-provider-discord/internal/client"
)

// --- Manage-not-create exemplar. A bot cannot create or delete a guild, so this
// resource *adopts* an existing one: Create PATCHes the live guild to match the
// config (it never POSTs), Delete is a no-op that only drops the guild from state
// (the guild itself is left untouched). Import-first: bring the live guild under
// management with `tofu import`. Pair with `prevent_destroy` in config. ---

var (
	_ resource.Resource                = (*managedServerResource)(nil)
	_ resource.ResourceWithConfigure   = (*managedServerResource)(nil)
	_ resource.ResourceWithImportState = (*managedServerResource)(nil)
)

// NewManagedServerResource returns a new discord_managed_server resource.
func NewManagedServerResource() resource.Resource {
	return &managedServerResource{}
}

type managedServerResource struct {
	client *client.Client
}

// guildAttributes mirrors the fields of a Discord guild object this resource maps.
type guildAttributes struct {
	Name                        string  `json:"name"`
	Description                 *string `json:"description"`
	Region                      *string `json:"region"`
	OwnerID                     string  `json:"owner_id"`
	AfkChannelID                *string `json:"afk_channel_id"`
	AfkTimeout                  int64   `json:"afk_timeout"`
	DefaultMessageNotifications int64   `json:"default_message_notifications"`
	ExplicitContentFilter       int64   `json:"explicit_content_filter"`
	VerificationLevel           int64   `json:"verification_level"`
	Icon                        *string `json:"icon"`
	Splash                      *string `json:"splash"`
	SystemChannelID             *string `json:"system_channel_id"`
	RulesChannelID              *string `json:"rules_channel_id"`
	PublicUpdatesChannelID      *string `json:"public_updates_channel_id"`
	SafetyAlertsChannelID       *string `json:"safety_alerts_channel_id"`
	SystemChannelFlags          int64   `json:"system_channel_flags"`
	PreferredLocale             string  `json:"preferred_locale"`
	PremiumProgressBarEnabled   bool    `json:"premium_progress_bar_enabled"`
}

type managedServerResourceModel struct {
	ServerID                    types.String `tfsdk:"server_id"`
	ID                          types.String `tfsdk:"id"`
	Name                        types.String `tfsdk:"name"`
	Description                 types.String `tfsdk:"description"`
	Region                      types.String `tfsdk:"region"`
	OwnerID                     types.String `tfsdk:"owner_id"`
	AfkChannelID                types.String `tfsdk:"afk_channel_id"`
	AfkTimeout                  types.Int64  `tfsdk:"afk_timeout"`
	DefaultMessageNotifications types.Int64  `tfsdk:"default_message_notifications"`
	ExplicitContentFilter       types.Int64  `tfsdk:"explicit_content_filter"`
	VerificationLevel           types.Int64  `tfsdk:"verification_level"`
	IconDataURI                 types.String `tfsdk:"icon_data_uri"`
	IconHash                    types.String `tfsdk:"icon_hash"`
	SplashDataURI               types.String `tfsdk:"splash_data_uri"`
	SplashHash                  types.String `tfsdk:"splash_hash"`
	SystemChannelID             types.String `tfsdk:"system_channel_id"`
	RulesChannelID              types.String `tfsdk:"rules_channel_id"`
	PublicUpdatesChannelID      types.String `tfsdk:"public_updates_channel_id"`
	SafetyAlertsChannelID       types.String `tfsdk:"safety_alerts_channel_id"`
	SystemChannelFlags          types.Int64  `tfsdk:"system_channel_flags"`
	PreferredLocale             types.String `tfsdk:"preferred_locale"`
	PremiumProgressBarEnabled   types.Bool   `tfsdk:"premium_progress_bar_enabled"`
}

func (r *managedServerResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_managed_server"
}

func (r *managedServerResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	optComputedStr := func(desc string) schema.StringAttribute {
		return schema.StringAttribute{
			MarkdownDescription: desc,
			Optional:            true,
			Computed:            true,
			PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		}
	}
	optComputedInt := func(desc string, vs ...validator.Int64) schema.Int64Attribute {
		return schema.Int64Attribute{
			MarkdownDescription: desc,
			Optional:            true,
			Computed:            true,
			PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			Validators:          vs,
		}
	}

	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the settings of an **existing** Discord guild (server). A bot cannot create or " +
			"delete a guild, so this resource adopts a live guild: import it first, then manage its settings. " +
			"Destroying the resource only removes it from state — the guild is left intact. Use `prevent_destroy`.",
		Attributes: map[string]schema.Attribute{
			"server_id": schema.StringAttribute{
				MarkdownDescription: "Snowflake ID of the guild to manage.",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "Guild ID (equal to `server_id`).",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name":                          optComputedStr("Guild name."),
			"description":                   optComputedStr("Guild description (Community guilds)."),
			"region":                        optComputedStr("Voice region id (deprecated by Discord; channel-level now)."),
			"owner_id":                      optComputedStr("Snowflake ID of the guild owner. Changing it transfers ownership (only possible if the bot is the current owner)."),
			"afk_channel_id":                optComputedStr("Snowflake ID of the AFK voice channel."),
			"afk_timeout":                   optComputedInt("AFK timeout in seconds (60, 300, 900, 1800 or 3600).", int64OneOf(60, 300, 900, 1800, 3600)),
			"default_message_notifications": optComputedInt("Default message notification level (`0` = all messages, `1` = only mentions).", int64OneOf(0, 1)),
			"explicit_content_filter":       optComputedInt("Explicit content filter level (`0` disabled, `1` members without roles, `2` all members).", int64OneOf(0, 1, 2)),
			"verification_level":            optComputedInt("Verification level (`0` none … `4` highest).", int64OneOf(0, 1, 2, 3, 4)),
			"icon_data_uri": schema.StringAttribute{
				MarkdownDescription: "Guild icon as a base64 data URI (e.g. from `discord_local_image`). Write-only input; Discord stores it and returns a hash in `icon_hash`. Not refreshed from the API, so it never drifts.",
				Optional:            true,
			},
			"icon_hash": schema.StringAttribute{MarkdownDescription: "Current guild icon hash.", Computed: true},
			"splash_data_uri": schema.StringAttribute{
				MarkdownDescription: "Invite splash image as a base64 data URI. Write-only input; see `icon_data_uri`.",
				Optional:            true,
			},
			"splash_hash":                  schema.StringAttribute{MarkdownDescription: "Current invite splash hash.", Computed: true},
			"system_channel_id":            optComputedStr("Snowflake ID of the system message channel."),
			"rules_channel_id":             optComputedStr("Snowflake ID of the rules channel (Community guilds)."),
			"public_updates_channel_id":    optComputedStr("Snowflake ID of the public updates channel (Community guilds)."),
			"safety_alerts_channel_id":     optComputedStr("Snowflake ID of the safety alerts channel (Community guilds)."),
			"system_channel_flags":         optComputedInt("System channel flags bitfield (e.g. suppress join / boost notifications)."),
			"preferred_locale":             optComputedStr("Preferred locale of a Community guild (e.g. `en-US`)."),
			"premium_progress_bar_enabled": schema.BoolAttribute{MarkdownDescription: "Whether the boost progress bar is shown.", Optional: true, Computed: true, PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()}},
		},
	}
}

func (r *managedServerResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// buildBody collects the settable guild fields that are set in the plan.
func (r *managedServerResource) buildBody(m *managedServerResourceModel) map[string]any {
	body := map[string]any{}
	if v := m.Name; !v.IsNull() && !v.IsUnknown() {
		body["name"] = v.ValueString()
	}
	if v := m.Description; !v.IsNull() && !v.IsUnknown() {
		body["description"] = v.ValueString()
	}
	if v := m.Region; !v.IsNull() && !v.IsUnknown() {
		body["region"] = v.ValueString()
	}
	if v := m.OwnerID; !v.IsNull() && !v.IsUnknown() {
		body["owner_id"] = v.ValueString()
	}
	if v := m.AfkChannelID; !v.IsNull() && !v.IsUnknown() {
		body["afk_channel_id"] = v.ValueString()
	}
	if v := m.AfkTimeout; !v.IsNull() && !v.IsUnknown() {
		body["afk_timeout"] = v.ValueInt64()
	}
	if v := m.DefaultMessageNotifications; !v.IsNull() && !v.IsUnknown() {
		body["default_message_notifications"] = v.ValueInt64()
	}
	if v := m.ExplicitContentFilter; !v.IsNull() && !v.IsUnknown() {
		body["explicit_content_filter"] = v.ValueInt64()
	}
	if v := m.VerificationLevel; !v.IsNull() && !v.IsUnknown() {
		body["verification_level"] = v.ValueInt64()
	}
	if v := m.IconDataURI; !v.IsNull() && !v.IsUnknown() {
		body["icon"] = v.ValueString()
	}
	if v := m.SplashDataURI; !v.IsNull() && !v.IsUnknown() {
		body["splash"] = v.ValueString()
	}
	if v := m.SystemChannelID; !v.IsNull() && !v.IsUnknown() {
		body["system_channel_id"] = v.ValueString()
	}
	if v := m.RulesChannelID; !v.IsNull() && !v.IsUnknown() {
		body["rules_channel_id"] = v.ValueString()
	}
	if v := m.PublicUpdatesChannelID; !v.IsNull() && !v.IsUnknown() {
		body["public_updates_channel_id"] = v.ValueString()
	}
	if v := m.SafetyAlertsChannelID; !v.IsNull() && !v.IsUnknown() {
		body["safety_alerts_channel_id"] = v.ValueString()
	}
	if v := m.SystemChannelFlags; !v.IsNull() && !v.IsUnknown() {
		body["system_channel_flags"] = v.ValueInt64()
	}
	if v := m.PreferredLocale; !v.IsNull() && !v.IsUnknown() {
		body["preferred_locale"] = v.ValueString()
	}
	if v := m.PremiumProgressBarEnabled; !v.IsNull() && !v.IsUnknown() {
		body["premium_progress_bar_enabled"] = v.ValueBool()
	}
	return body
}

func (r *managedServerResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan managedServerResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Adopt the live guild: apply the configured settings (never POST a new guild).
	if body := r.buildBody(&plan); len(body) > 0 {
		if err := r.client.Write(ctx, "PATCH", "/guilds/"+plan.ServerID.ValueString(), body, nil); err != nil {
			resp.Diagnostics.AddError("Unable to adopt Discord guild", err.Error())
			return
		}
	}
	plan.ID = plan.ServerID

	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Unable to read guild after adopt", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *managedServerResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state managedServerResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.readInto(ctx, &state); err != nil {
		if client.NotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read Discord guild", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *managedServerResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan managedServerResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if body := r.buildBody(&plan); len(body) > 0 {
		if err := r.client.Write(ctx, "PATCH", "/guilds/"+plan.ServerID.ValueString(), body, nil); err != nil {
			resp.Diagnostics.AddError("Unable to update Discord guild", err.Error())
			return
		}
	}
	plan.ID = plan.ServerID

	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Unable to read guild after update", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete is a no-op: a bot must not delete the guild it manages. Removing the
// resource only drops it from state; the guild is left untouched.
func (r *managedServerResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

// ImportState adopts a live guild by its snowflake id.
func (r *managedServerResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("server_id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

// readInto GETs the guild and fills the computed fields. The *_data_uri inputs
// are deliberately not refreshed (Discord only returns hashes), so they never
// drift.
func (r *managedServerResource) readInto(ctx context.Context, m *managedServerResourceModel) error {
	var a guildAttributes
	if err := r.client.Get(ctx, "/guilds/"+m.ServerID.ValueString(), &a); err != nil {
		return err
	}
	m.ID = m.ServerID
	m.Name = types.StringValue(a.Name)
	m.Description = types.StringPointerValue(a.Description)
	m.Region = types.StringPointerValue(a.Region)
	m.OwnerID = types.StringValue(a.OwnerID)
	m.AfkChannelID = types.StringPointerValue(a.AfkChannelID)
	m.AfkTimeout = types.Int64Value(a.AfkTimeout)
	m.DefaultMessageNotifications = types.Int64Value(a.DefaultMessageNotifications)
	m.ExplicitContentFilter = types.Int64Value(a.ExplicitContentFilter)
	m.VerificationLevel = types.Int64Value(a.VerificationLevel)
	m.IconHash = types.StringPointerValue(a.Icon)
	m.SplashHash = types.StringPointerValue(a.Splash)
	m.SystemChannelID = types.StringPointerValue(a.SystemChannelID)
	m.RulesChannelID = types.StringPointerValue(a.RulesChannelID)
	m.PublicUpdatesChannelID = types.StringPointerValue(a.PublicUpdatesChannelID)
	m.SafetyAlertsChannelID = types.StringPointerValue(a.SafetyAlertsChannelID)
	m.SystemChannelFlags = types.Int64Value(a.SystemChannelFlags)
	m.PreferredLocale = types.StringValue(a.PreferredLocale)
	m.PremiumProgressBarEnabled = types.BoolValue(a.PremiumProgressBarEnabled)
	return nil
}
