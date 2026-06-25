package provider

import (
	"context"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/kirchDev/terraform-provider-discord/internal/client"
)

// --- Guild onboarding (a singleton). The whole object — enabled, mode,
// default_channel_ids and the prompts → options tree — round-trips through
// GET/PUT /guilds/{server_id}/onboarding. Prompts are modelled as structured
// nested attributes so drift is detected and imports are complete; the PUT
// replaces the object wholesale, so every apply sends all four fields. ---

var (
	_ resource.Resource                = (*serverOnboardingResource)(nil)
	_ resource.ResourceWithConfigure   = (*serverOnboardingResource)(nil)
	_ resource.ResourceWithImportState = (*serverOnboardingResource)(nil)
)

// NewServerOnboardingResource returns a new discord_server_onboarding resource.
func NewServerOnboardingResource() resource.Resource {
	return &serverOnboardingResource{}
}

type serverOnboardingResource struct {
	client *client.Client
}

// --- wire (Discord REST shapes) ---

// onboardingEmojiWire is Discord's partial emoji object as returned on a read
// (`{id, name}`); on a write the fields are flattened to emoji_id / emoji_name.
type onboardingEmojiWire struct {
	ID   *string `json:"id"`
	Name *string `json:"name"`
}

type onboardingOptionWire struct {
	ID          string               `json:"id"`
	Title       string               `json:"title"`
	Description *string              `json:"description"`
	Emoji       *onboardingEmojiWire `json:"emoji"`
	ChannelIDs  []string             `json:"channel_ids"`
	RoleIDs     []string             `json:"role_ids"`
}

type onboardingPromptWire struct {
	ID           string                 `json:"id"`
	Type         int64                  `json:"type"`
	Title        string                 `json:"title"`
	SingleSelect bool                   `json:"single_select"`
	Required     bool                   `json:"required"`
	InOnboarding bool                   `json:"in_onboarding"`
	Options      []onboardingOptionWire `json:"options"`
}

type onboardingWire struct {
	Enabled           bool                   `json:"enabled"`
	Mode              int64                  `json:"mode"`
	DefaultChannelIDs []string               `json:"default_channel_ids"`
	Prompts           []onboardingPromptWire `json:"prompts"`
}

// --- model (tfsdk) ---

type onboardingOptionModel struct {
	ID          types.String `tfsdk:"id"`
	Title       types.String `tfsdk:"title"`
	Description types.String `tfsdk:"description"`
	EmojiID     types.String `tfsdk:"emoji_id"`
	EmojiName   types.String `tfsdk:"emoji_name"`
	ChannelIDs  types.Set    `tfsdk:"channel_ids"`
	RoleIDs     types.Set    `tfsdk:"role_ids"`
}

type onboardingPromptModel struct {
	ID           types.String `tfsdk:"id"`
	Type         types.Int64  `tfsdk:"type"`
	Title        types.String `tfsdk:"title"`
	SingleSelect types.Bool   `tfsdk:"single_select"`
	Required     types.Bool   `tfsdk:"required"`
	InOnboarding types.Bool   `tfsdk:"in_onboarding"`
	Options      types.List   `tfsdk:"options"`
}

var onboardingOptionAttrTypes = map[string]attr.Type{
	"id":          types.StringType,
	"title":       types.StringType,
	"description": types.StringType,
	"emoji_id":    types.StringType,
	"emoji_name":  types.StringType,
	"channel_ids": types.SetType{ElemType: types.StringType},
	"role_ids":    types.SetType{ElemType: types.StringType},
}

var onboardingPromptAttrTypes = map[string]attr.Type{
	"id":            types.StringType,
	"type":          types.Int64Type,
	"title":         types.StringType,
	"single_select": types.BoolType,
	"required":      types.BoolType,
	"in_onboarding": types.BoolType,
	"options":       types.ListType{ElemType: types.ObjectType{AttrTypes: onboardingOptionAttrTypes}},
}

type serverOnboardingResourceModel struct {
	ServerID          types.String `tfsdk:"server_id"`
	Enabled           types.Bool   `tfsdk:"enabled"`
	Mode              types.Int64  `tfsdk:"mode"`
	DefaultChannelIDs types.Set    `tfsdk:"default_channel_ids"`
	Prompts           types.List   `tfsdk:"prompts"`
}

func (r *serverOnboardingResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_server_onboarding"
}

func (r *serverOnboardingResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	snowflakeID := func(noun string) schema.StringAttribute {
		return schema.StringAttribute{
			MarkdownDescription: "Snowflake ID of the " + noun + ", assigned by Discord.",
			Computed:            true,
			PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		}
	}
	optComputedStrSet := func(desc string) schema.SetAttribute {
		return schema.SetAttribute{MarkdownDescription: desc, ElementType: types.StringType, Optional: true, Computed: true}
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the onboarding configuration of a Community Discord guild (a singleton per guild). " +
			"The `prompts` tree, `enabled`, `mode` and `default_channel_ids` are all refreshed from the API so drift is detected.",
		Attributes: map[string]schema.Attribute{
			"server_id": schema.StringAttribute{
				MarkdownDescription: "Snowflake ID of the guild.",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether onboarding is enabled.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
			"mode": schema.Int64Attribute{
				MarkdownDescription: "Onboarding mode (`0` default, `1` advanced).",
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(0),
				Validators:          []validator.Int64{int64OneOf(0, 1)},
			},
			"default_channel_ids": optComputedStrSet("Channel ids members are opted into by default."),
			"prompts": schema.ListNestedAttribute{
				MarkdownDescription: "Ordered onboarding prompts shown to new members.",
				Optional:            true,
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":            snowflakeID("prompt"),
						"type":          schema.Int64Attribute{MarkdownDescription: "Prompt type (`0` multiple choice, `1` dropdown).", Required: true, Validators: []validator.Int64{int64OneOf(0, 1)}},
						"title":         schema.StringAttribute{MarkdownDescription: "Prompt title.", Required: true},
						"single_select": schema.BoolAttribute{MarkdownDescription: "Whether only a single option may be selected.", Optional: true, Computed: true},
						"required":      schema.BoolAttribute{MarkdownDescription: "Whether the prompt must be answered to finish onboarding.", Optional: true, Computed: true},
						"in_onboarding": schema.BoolAttribute{MarkdownDescription: "Whether the prompt appears in the onboarding flow (vs. only in Channels & Roles).", Optional: true, Computed: true},
						"options": schema.ListNestedAttribute{
							MarkdownDescription: "Selectable options for the prompt.",
							Required:            true,
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"id":          snowflakeID("option"),
									"title":       schema.StringAttribute{MarkdownDescription: "Option title.", Required: true},
									"description": schema.StringAttribute{MarkdownDescription: "Option description.", Optional: true},
									"emoji_id":    schema.StringAttribute{MarkdownDescription: "Snowflake ID of a custom emoji shown next to the option.", Optional: true},
									"emoji_name":  schema.StringAttribute{MarkdownDescription: "Unicode emoji (or the name of a custom emoji) shown next to the option.", Optional: true},
									"channel_ids": optComputedStrSet("Channel ids a member is opted into when selecting this option."),
									"role_ids":    optComputedStrSet("Role ids granted when selecting this option."),
								},
							},
						},
					},
				},
			},
		},
	}
}

func (r *serverOnboardingResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *serverOnboardingResource) onboardingPath(m *serverOnboardingResourceModel) string {
	return "/guilds/" + m.ServerID.ValueString() + "/onboarding"
}

// boolOrDefault returns the bool value, or def when it is null/unknown — used for
// the prompt flags, which are Optional+Computed and thus unknown on create.
func boolOrDefault(v types.Bool, def bool) bool {
	if v.IsNull() || v.IsUnknown() {
		return def
	}
	return v.ValueBool()
}

// nullIfEmpty maps a nil or empty-string API value to a null attribute, so an
// unset optional (description / emoji) stays null instead of "" and doesn't trip
// the post-apply consistency check.
func nullIfEmpty(p *string) types.String {
	if p == nil || *p == "" {
		return types.StringNull()
	}
	return types.StringValue(*p)
}

// apply PUTs the whole onboarding object (the endpoint replaces it wholesale).
func (r *serverOnboardingResource) apply(ctx context.Context, m *serverOnboardingResourceModel) error {
	body := map[string]any{
		"enabled": m.Enabled.ValueBool(),
		"mode":    m.Mode.ValueInt64(),
	}
	if ids, ok, err := strSet(ctx, m.DefaultChannelIDs); err != nil {
		return err
	} else if ok {
		body["default_channel_ids"] = ids
	} else {
		body["default_channel_ids"] = []string{}
	}
	prompts, err := r.promptsBody(ctx, m)
	if err != nil {
		return err
	}
	body["prompts"] = prompts
	return r.client.Write(ctx, "PUT", r.onboardingPath(m), body, nil)
}

// promptsBody marshals the prompts tree into Discord's write shape. Prompts and
// options without an id (new, server-assigned) get a placeholder id so the PUT
// is accepted; Discord replaces it with a real snowflake, read back in Read.
func (r *serverOnboardingResource) promptsBody(ctx context.Context, m *serverOnboardingResourceModel) ([]map[string]any, error) {
	if m.Prompts.IsNull() || m.Prompts.IsUnknown() {
		return []map[string]any{}, nil
	}
	var prompts []onboardingPromptModel
	if d := m.Prompts.ElementsAs(ctx, &prompts, false); d.HasError() {
		return nil, fmt.Errorf("reading prompts")
	}

	placeholder := 0
	nextID := func(id types.String) string {
		if !id.IsNull() && !id.IsUnknown() && id.ValueString() != "" {
			return id.ValueString()
		}
		placeholder++
		return strconv.Itoa(placeholder)
	}

	out := make([]map[string]any, 0, len(prompts))
	for _, p := range prompts {
		var options []onboardingOptionModel
		if d := p.Options.ElementsAs(ctx, &options, false); d.HasError() {
			return nil, fmt.Errorf("reading prompt options")
		}
		optOut := make([]map[string]any, 0, len(options))
		for _, o := range options {
			entry := map[string]any{
				"id":    nextID(o.ID),
				"title": o.Title.ValueString(),
			}
			if v := o.Description; !v.IsNull() && !v.IsUnknown() {
				entry["description"] = v.ValueString()
			}
			if v := o.EmojiID; !v.IsNull() && !v.IsUnknown() {
				entry["emoji_id"] = v.ValueString()
			}
			if v := o.EmojiName; !v.IsNull() && !v.IsUnknown() {
				entry["emoji_name"] = v.ValueString()
			}
			if ids, ok, err := strSet(ctx, o.ChannelIDs); err != nil {
				return nil, err
			} else if ok {
				entry["channel_ids"] = ids
			} else {
				entry["channel_ids"] = []string{}
			}
			if ids, ok, err := strSet(ctx, o.RoleIDs); err != nil {
				return nil, err
			} else if ok {
				entry["role_ids"] = ids
			} else {
				entry["role_ids"] = []string{}
			}
			optOut = append(optOut, entry)
		}
		out = append(out, map[string]any{
			"id":            nextID(p.ID),
			"type":          p.Type.ValueInt64(),
			"title":         p.Title.ValueString(),
			"single_select": boolOrDefault(p.SingleSelect, false),
			"required":      boolOrDefault(p.Required, false),
			"in_onboarding": boolOrDefault(p.InOnboarding, true),
			"options":       optOut,
		})
	}
	return out, nil
}

func (r *serverOnboardingResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan serverOnboardingResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.apply(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Unable to set Discord onboarding", err.Error())
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Unable to read onboarding after create", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *serverOnboardingResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state serverOnboardingResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.readInto(ctx, &state); err != nil {
		if notFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read Discord onboarding", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *serverOnboardingResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan serverOnboardingResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.apply(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Unable to update Discord onboarding", err.Error())
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Unable to read onboarding after update", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete is a no-op: onboarding cannot be removed, only reconfigured. Removing the
// resource only drops it from state.
func (r *serverOnboardingResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

// ImportState accepts the guild id (the singleton key); Read then fully populates
// the prompts tree so an imported onboarding plans clean.
func (r *serverOnboardingResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("server_id"), req.ID)...)
}

// readInto refreshes the full onboarding object (scalars and the prompts tree).
func (r *serverOnboardingResource) readInto(ctx context.Context, m *serverOnboardingResourceModel) error {
	var a onboardingWire
	if err := r.client.Get(ctx, r.onboardingPath(m), &a); err != nil {
		return err
	}
	m.Enabled = types.BoolValue(a.Enabled)
	m.Mode = types.Int64Value(a.Mode)
	set, hasErr := setOfStrings(ctx, a.DefaultChannelIDs)
	if hasErr {
		return fmt.Errorf("building default_channel_ids state")
	}
	m.DefaultChannelIDs = set

	prompts, err := onboardingPromptsToState(ctx, a.Prompts)
	if err != nil {
		return err
	}
	m.Prompts = prompts
	return nil
}

func onboardingPromptsToState(ctx context.Context, wire []onboardingPromptWire) (types.List, error) {
	promptType := types.ObjectType{AttrTypes: onboardingPromptAttrTypes}
	models := make([]onboardingPromptModel, 0, len(wire))
	for _, p := range wire {
		options, err := onboardingOptionsToState(ctx, p.Options)
		if err != nil {
			return types.ListNull(promptType), err
		}
		models = append(models, onboardingPromptModel{
			ID:           types.StringValue(p.ID),
			Type:         types.Int64Value(p.Type),
			Title:        types.StringValue(p.Title),
			SingleSelect: types.BoolValue(p.SingleSelect),
			Required:     types.BoolValue(p.Required),
			InOnboarding: types.BoolValue(p.InOnboarding),
			Options:      options,
		})
	}
	list, d := types.ListValueFrom(ctx, promptType, models)
	if d.HasError() {
		return types.ListNull(promptType), fmt.Errorf("building prompts state")
	}
	return list, nil
}

func onboardingOptionsToState(ctx context.Context, wire []onboardingOptionWire) (types.List, error) {
	optType := types.ObjectType{AttrTypes: onboardingOptionAttrTypes}
	models := make([]onboardingOptionModel, 0, len(wire))
	for _, o := range wire {
		channelIDs, hadErr := setOfStrings(ctx, o.ChannelIDs)
		if hadErr {
			return types.ListNull(optType), fmt.Errorf("building option channel_ids state")
		}
		roleIDs, hadErr := setOfStrings(ctx, o.RoleIDs)
		if hadErr {
			return types.ListNull(optType), fmt.Errorf("building option role_ids state")
		}
		emojiID, emojiName := types.StringNull(), types.StringNull()
		if o.Emoji != nil {
			emojiID = nullIfEmpty(o.Emoji.ID)
			emojiName = nullIfEmpty(o.Emoji.Name)
		}
		models = append(models, onboardingOptionModel{
			ID:          types.StringValue(o.ID),
			Title:       types.StringValue(o.Title),
			Description: nullIfEmpty(o.Description),
			EmojiID:     emojiID,
			EmojiName:   emojiName,
			ChannelIDs:  channelIDs,
			RoleIDs:     roleIDs,
		})
	}
	list, d := types.ListValueFrom(ctx, optType, models)
	if d.HasError() {
		return types.ListNull(optType), fmt.Errorf("building options state")
	}
	return list, nil
}
