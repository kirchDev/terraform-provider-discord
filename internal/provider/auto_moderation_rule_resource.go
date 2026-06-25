package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
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

// --- Auto-moderation rule. trigger_metadata is flattened into top-level
// attributes (only the ones relevant to the chosen trigger_type apply); actions
// are a nested list. ---

var (
	_ resource.Resource                = (*autoModerationRuleResource)(nil)
	_ resource.ResourceWithConfigure   = (*autoModerationRuleResource)(nil)
	_ resource.ResourceWithImportState = (*autoModerationRuleResource)(nil)
)

// NewAutoModerationRuleResource returns a new discord_auto_moderation_rule resource.
func NewAutoModerationRuleResource() resource.Resource {
	return &autoModerationRuleResource{}
}

type autoModerationRuleResource struct {
	client *client.Client
}

type automodActionModel struct {
	Type            types.Int64  `tfsdk:"type"`
	ChannelID       types.String `tfsdk:"channel_id"`
	DurationSeconds types.Int64  `tfsdk:"duration_seconds"`
	CustomMessage   types.String `tfsdk:"custom_message"`
}

var automodActionAttrTypes = map[string]attr.Type{
	"type":             types.Int64Type,
	"channel_id":       types.StringType,
	"duration_seconds": types.Int64Type,
	"custom_message":   types.StringType,
}

type automodActionWire struct {
	Type     int64 `json:"type"`
	Metadata *struct {
		ChannelID       *string `json:"channel_id"`
		DurationSeconds *int64  `json:"duration_seconds"`
		CustomMessage   *string `json:"custom_message"`
	} `json:"metadata"`
}

type automodTriggerMetaWire struct {
	KeywordFilter                []string `json:"keyword_filter"`
	RegexPatterns                []string `json:"regex_patterns"`
	Presets                      []int64  `json:"presets"`
	AllowList                    []string `json:"allow_list"`
	MentionTotalLimit            int64    `json:"mention_total_limit"`
	MentionRaidProtectionEnabled *bool    `json:"mention_raid_protection_enabled,omitempty"`
}

type automodRuleWire struct {
	ID              string                  `json:"id"`
	GuildID         string                  `json:"guild_id"`
	Name            string                  `json:"name"`
	EventType       int64                   `json:"event_type"`
	TriggerType     int64                   `json:"trigger_type"`
	Enabled         bool                    `json:"enabled"`
	TriggerMetadata *automodTriggerMetaWire `json:"trigger_metadata"`
	Actions         []automodActionWire     `json:"actions"`
	ExemptRoles     []string                `json:"exempt_roles"`
	ExemptChannels  []string                `json:"exempt_channels"`
}

type autoModerationRuleResourceModel struct {
	ServerID                     types.String `tfsdk:"server_id"`
	ID                           types.String `tfsdk:"id"`
	Name                         types.String `tfsdk:"name"`
	EventType                    types.Int64  `tfsdk:"event_type"`
	TriggerType                  types.Int64  `tfsdk:"trigger_type"`
	Enabled                      types.Bool   `tfsdk:"enabled"`
	KeywordFilter                types.Set    `tfsdk:"keyword_filter"`
	RegexPatterns                types.Set    `tfsdk:"regex_patterns"`
	Presets                      types.Set    `tfsdk:"presets"`
	AllowList                    types.Set    `tfsdk:"allow_list"`
	MentionTotalLimit            types.Int64  `tfsdk:"mention_total_limit"`
	MentionRaidProtectionEnabled types.Bool   `tfsdk:"mention_raid_protection_enabled"`
	Actions                      types.List   `tfsdk:"actions"`
	ExemptRoles                  types.Set    `tfsdk:"exempt_roles"`
	ExemptChannels               types.Set    `tfsdk:"exempt_channels"`
}

func (r *autoModerationRuleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_auto_moderation_rule"
}

func (r *autoModerationRuleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	optComputedStrSet := func(desc string) schema.SetAttribute {
		return schema.SetAttribute{MarkdownDescription: desc, ElementType: types.StringType, Optional: true, Computed: true}
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages an auto-moderation rule in a Discord guild. The `trigger_metadata` fields " +
			"(`keyword_filter`, `regex_patterns`, `presets`, `allow_list`, `mention_total_limit`, " +
			"`mention_raid_protection_enabled`) apply only to the relevant `trigger_type`.",
		Attributes: map[string]schema.Attribute{
			"server_id": schema.StringAttribute{
				MarkdownDescription: "Snowflake ID of the guild.",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "Snowflake ID of the rule.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name":                schema.StringAttribute{MarkdownDescription: "Rule name.", Required: true},
			"event_type":          schema.Int64Attribute{MarkdownDescription: "Event that triggers the rule (`1` message send, `2` member update).", Required: true, Validators: []validator.Int64{int64OneOf(1, 2)}},
			"trigger_type":        schema.Int64Attribute{MarkdownDescription: "Trigger type (`1` keyword, `3` spam, `4` keyword preset, `5` mention spam). Changing it replaces the rule.", Required: true, PlanModifiers: []planmodifier.Int64{int64planmodifier.RequiresReplace()}, Validators: []validator.Int64{int64OneOf(1, 3, 4, 5)}},
			"enabled":             schema.BoolAttribute{MarkdownDescription: "Whether the rule is enabled.", Optional: true, Computed: true, Default: booldefault.StaticBool(false)},
			"keyword_filter":      optComputedStrSet("Substrings to match (trigger type keyword)."),
			"regex_patterns":      optComputedStrSet("Regular expressions to match (trigger type keyword)."),
			"allow_list":          optComputedStrSet("Substrings exempt from the trigger."),
			"presets":             schema.SetAttribute{MarkdownDescription: "Keyword preset ids (`1` profanity, `2` sexual content, `3` slurs).", ElementType: types.Int64Type, Optional: true, Computed: true},
			"mention_total_limit": schema.Int64Attribute{MarkdownDescription: "Max unique mentions per message (trigger type mention spam).", Optional: true, Computed: true},
			"mention_raid_protection_enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether to automatically detect and block mention raids (trigger type `5`, mention spam). " +
					"Because Discord replaces `trigger_metadata` wholesale on every update, this provider always re-sends the " +
					"managed value so the setting is never silently cleared.",
				Optional: true,
				Computed: true,
			},
			"exempt_roles":    optComputedStrSet("Role ids exempt from the rule."),
			"exempt_channels": optComputedStrSet("Channel ids exempt from the rule."),
			"actions": schema.ListNestedAttribute{
				MarkdownDescription: "Actions taken when the rule triggers.",
				Required:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"type":             schema.Int64Attribute{MarkdownDescription: "Action type (`1` block message, `2` send alert, `3` timeout).", Required: true},
						"channel_id":       schema.StringAttribute{MarkdownDescription: "Channel to log alerts to (action type send alert).", Optional: true},
						"duration_seconds": schema.Int64Attribute{MarkdownDescription: "Timeout duration in seconds (action type timeout).", Optional: true},
						"custom_message":   schema.StringAttribute{MarkdownDescription: "Custom message shown when a message is blocked.", Optional: true},
					},
				},
			},
		},
	}
}

func (r *autoModerationRuleResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *autoModerationRuleResource) rulesPath(m *autoModerationRuleResourceModel) string {
	return "/guilds/" + m.ServerID.ValueString() + "/auto-moderation/rules"
}

func (r *autoModerationRuleResource) body(ctx context.Context, m *autoModerationRuleResourceModel) (map[string]any, error) {
	body := map[string]any{
		"name":         m.Name.ValueString(),
		"event_type":   m.EventType.ValueInt64(),
		"trigger_type": m.TriggerType.ValueInt64(),
	}
	if v := m.Enabled; !v.IsNull() && !v.IsUnknown() {
		body["enabled"] = v.ValueBool()
	}

	meta := map[string]any{}
	if s, ok, err := strSet(ctx, m.KeywordFilter); err != nil {
		return nil, err
	} else if ok {
		meta["keyword_filter"] = s
	}
	if s, ok, err := strSet(ctx, m.RegexPatterns); err != nil {
		return nil, err
	} else if ok {
		meta["regex_patterns"] = s
	}
	if s, ok, err := strSet(ctx, m.AllowList); err != nil {
		return nil, err
	} else if ok {
		meta["allow_list"] = s
	}
	if !m.Presets.IsNull() && !m.Presets.IsUnknown() {
		var presets []int64
		if d := m.Presets.ElementsAs(ctx, &presets, false); d.HasError() {
			return nil, fmt.Errorf("reading presets")
		}
		meta["presets"] = presets
	}
	if v := m.MentionTotalLimit; !v.IsNull() && !v.IsUnknown() {
		meta["mention_total_limit"] = v.ValueInt64()
	}
	if v := m.MentionRaidProtectionEnabled; !v.IsNull() && !v.IsUnknown() {
		meta["mention_raid_protection_enabled"] = v.ValueBool()
	}
	if len(meta) > 0 {
		body["trigger_metadata"] = meta
	}

	if s, ok, err := strSet(ctx, m.ExemptRoles); err != nil {
		return nil, err
	} else if ok {
		body["exempt_roles"] = s
	}
	if s, ok, err := strSet(ctx, m.ExemptChannels); err != nil {
		return nil, err
	} else if ok {
		body["exempt_channels"] = s
	}

	var actions []automodActionModel
	if d := m.Actions.ElementsAs(ctx, &actions, false); d.HasError() {
		return nil, fmt.Errorf("reading actions")
	}
	wire := make([]map[string]any, 0, len(actions))
	for _, a := range actions {
		metadata := map[string]any{}
		if !a.ChannelID.IsNull() && !a.ChannelID.IsUnknown() {
			metadata["channel_id"] = a.ChannelID.ValueString()
		}
		if !a.DurationSeconds.IsNull() && !a.DurationSeconds.IsUnknown() {
			metadata["duration_seconds"] = a.DurationSeconds.ValueInt64()
		}
		if !a.CustomMessage.IsNull() && !a.CustomMessage.IsUnknown() {
			metadata["custom_message"] = a.CustomMessage.ValueString()
		}
		entry := map[string]any{"type": a.Type.ValueInt64()}
		if len(metadata) > 0 {
			entry["metadata"] = metadata
		}
		wire = append(wire, entry)
	}
	body["actions"] = wire
	return body, nil
}

func (r *autoModerationRuleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan autoModerationRuleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body, err := r.body(ctx, &plan)
	if err != nil {
		resp.Diagnostics.AddError("Invalid auto-moderation rule configuration", err.Error())
		return
	}
	var created automodRuleWire
	if err := r.client.Write(ctx, "POST", r.rulesPath(&plan), body, &created); err != nil {
		resp.Diagnostics.AddError("Unable to create Discord auto-moderation rule", err.Error())
		return
	}
	plan.ID = types.StringValue(created.ID)
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Unable to read auto-moderation rule after create", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *autoModerationRuleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state autoModerationRuleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.readInto(ctx, &state); err != nil {
		if notFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read Discord auto-moderation rule", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *autoModerationRuleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan autoModerationRuleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body, err := r.body(ctx, &plan)
	if err != nil {
		resp.Diagnostics.AddError("Invalid auto-moderation rule configuration", err.Error())
		return
	}
	// trigger_type is immutable on update; omit it.
	delete(body, "trigger_type")
	if err := r.client.Write(ctx, "PATCH", r.rulesPath(&plan)+"/"+plan.ID.ValueString(), body, nil); err != nil {
		resp.Diagnostics.AddError("Unable to update Discord auto-moderation rule", err.Error())
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Unable to read auto-moderation rule after update", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *autoModerationRuleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state autoModerationRuleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.Delete(ctx, r.rulesPath(&state)+"/"+state.ID.ValueString()); err != nil && !notFound(err) {
		resp.Diagnostics.AddError("Unable to delete Discord auto-moderation rule", err.Error())
	}
}

// ImportState accepts "server_id/rule_id".
func (r *autoModerationRuleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, "/")
	if len(parts) != 2 {
		resp.Diagnostics.AddError("Invalid import ID", "Expected \"server_id/rule_id\".")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("server_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[1])...)
}

func (r *autoModerationRuleResource) readInto(ctx context.Context, m *autoModerationRuleResourceModel) error {
	var a automodRuleWire
	if err := r.client.Get(ctx, r.rulesPath(m)+"/"+m.ID.ValueString(), &a); err != nil {
		return err
	}
	m.ServerID = types.StringValue(a.GuildID)
	m.Name = types.StringValue(a.Name)
	m.EventType = types.Int64Value(a.EventType)
	m.TriggerType = types.Int64Value(a.TriggerType)
	m.Enabled = types.BoolValue(a.Enabled)

	tm := a.TriggerMetadata
	if tm == nil {
		tm = &automodTriggerMetaWire{}
	}
	var d1, d2, d3, d4, d5 bool
	m.KeywordFilter, d1 = setOfStrings(ctx, tm.KeywordFilter)
	m.RegexPatterns, d2 = setOfStrings(ctx, tm.RegexPatterns)
	m.AllowList, d3 = setOfStrings(ctx, tm.AllowList)
	m.ExemptRoles, d4 = setOfStrings(ctx, a.ExemptRoles)
	m.ExemptChannels, d5 = setOfStrings(ctx, a.ExemptChannels)
	if d1 || d2 || d3 || d4 || d5 {
		return fmt.Errorf("building string sets for auto-moderation rule state")
	}
	presets, pd := types.SetValueFrom(ctx, types.Int64Type, tm.Presets)
	if pd.HasError() {
		return fmt.Errorf("building presets state")
	}
	m.Presets = presets
	m.MentionTotalLimit = types.Int64Value(tm.MentionTotalLimit)
	// A nil pointer (field absent for non-mention-spam rules) maps to null, so
	// the value is not echoed back into trigger_metadata where it does not apply.
	m.MentionRaidProtectionEnabled = types.BoolPointerValue(tm.MentionRaidProtectionEnabled)

	actions := make([]automodActionModel, 0, len(a.Actions))
	for _, act := range a.Actions {
		am := automodActionModel{Type: types.Int64Value(act.Type)}
		if act.Metadata != nil {
			am.ChannelID = types.StringPointerValue(act.Metadata.ChannelID)
			if act.Metadata.DurationSeconds != nil {
				am.DurationSeconds = types.Int64Value(*act.Metadata.DurationSeconds)
			} else {
				am.DurationSeconds = types.Int64Null()
			}
			am.CustomMessage = types.StringPointerValue(act.Metadata.CustomMessage)
		} else {
			am.ChannelID = types.StringNull()
			am.DurationSeconds = types.Int64Null()
			am.CustomMessage = types.StringNull()
		}
		actions = append(actions, am)
	}
	list, ld := types.ListValueFrom(ctx, types.ObjectType{AttrTypes: automodActionAttrTypes}, actions)
	if ld.HasError() {
		return fmt.Errorf("building actions state")
	}
	m.Actions = list
	return nil
}
