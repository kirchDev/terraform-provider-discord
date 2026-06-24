package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/kirchDev/terraform-provider-discord/internal/client"
)

// --- The guild welcome screen (Community guilds): a singleton on the guild. It
// adopts rather than creates — Create/Update PATCH the welcome screen, Delete
// disables it. `enabled` is write-only (Discord doesn't return it), so it is not
// refreshed. ---

var (
	_ resource.Resource                = (*welcomeScreenResource)(nil)
	_ resource.ResourceWithConfigure   = (*welcomeScreenResource)(nil)
	_ resource.ResourceWithImportState = (*welcomeScreenResource)(nil)
)

// NewWelcomeScreenResource returns a new discord_welcome_screen resource.
func NewWelcomeScreenResource() resource.Resource {
	return &welcomeScreenResource{}
}

type welcomeScreenResource struct {
	client *client.Client
}

type welcomeChannelModel struct {
	ChannelID   types.String `tfsdk:"channel_id"`
	Description types.String `tfsdk:"description"`
	EmojiID     types.String `tfsdk:"emoji_id"`
	EmojiName   types.String `tfsdk:"emoji_name"`
}

var welcomeChannelAttrTypes = map[string]attr.Type{
	"channel_id":  types.StringType,
	"description": types.StringType,
	"emoji_id":    types.StringType,
	"emoji_name":  types.StringType,
}

type welcomeChannelWire struct {
	ChannelID   string  `json:"channel_id"`
	Description string  `json:"description"`
	EmojiID     *string `json:"emoji_id"`
	EmojiName   *string `json:"emoji_name"`
}

type welcomeScreenWire struct {
	Description     *string              `json:"description"`
	WelcomeChannels []welcomeChannelWire `json:"welcome_channels"`
}

type welcomeScreenResourceModel struct {
	ServerID        types.String `tfsdk:"server_id"`
	Enabled         types.Bool   `tfsdk:"enabled"`
	Description     types.String `tfsdk:"description"`
	WelcomeChannels types.List   `tfsdk:"welcome_channels"`
}

func (r *welcomeScreenResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_welcome_screen"
}

func (r *welcomeScreenResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the welcome screen of a Community Discord guild (a singleton per guild).",
		Attributes: map[string]schema.Attribute{
			"server_id": schema.StringAttribute{
				MarkdownDescription: "Snowflake ID of the guild.",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether the welcome screen is shown. Write-only — Discord does not return it, so it is not refreshed.",
				Optional:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Server description shown on the welcome screen.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"welcome_channels": schema.ListNestedAttribute{
				MarkdownDescription: "Channels highlighted on the welcome screen (max 5).",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.List{listplanmodifier.UseStateForUnknown()},
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"channel_id":  schema.StringAttribute{MarkdownDescription: "Snowflake ID of the channel.", Required: true},
						"description": schema.StringAttribute{MarkdownDescription: "Description shown for the channel.", Required: true},
						"emoji_id":    schema.StringAttribute{MarkdownDescription: "Snowflake ID of a custom emoji.", Optional: true},
						"emoji_name":  schema.StringAttribute{MarkdownDescription: "Unicode emoji.", Optional: true},
					},
				},
			},
		},
	}
}

func (r *welcomeScreenResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *welcomeScreenResource) screenPath(m *welcomeScreenResourceModel) string {
	return "/guilds/" + m.ServerID.ValueString() + "/welcome-screen"
}

func (r *welcomeScreenResource) apply(ctx context.Context, m *welcomeScreenResourceModel) error {
	body := map[string]any{}
	if v := m.Enabled; !v.IsNull() && !v.IsUnknown() {
		body["enabled"] = v.ValueBool()
	}
	if v := m.Description; !v.IsNull() && !v.IsUnknown() {
		body["description"] = v.ValueString()
	}
	if !m.WelcomeChannels.IsNull() && !m.WelcomeChannels.IsUnknown() {
		var chans []welcomeChannelModel
		if d := m.WelcomeChannels.ElementsAs(ctx, &chans, false); d.HasError() {
			return fmt.Errorf("reading welcome_channels")
		}
		wire := make([]map[string]any, 0, len(chans))
		for _, c := range chans {
			wire = append(wire, map[string]any{
				"channel_id":  c.ChannelID.ValueString(),
				"description": c.Description.ValueString(),
				"emoji_id":    strPtrOrNil(c.EmojiID),
				"emoji_name":  strPtrOrNil(c.EmojiName),
			})
		}
		body["welcome_channels"] = wire
	}
	return r.client.Write(ctx, "PATCH", r.screenPath(m), body, nil)
}

func (r *welcomeScreenResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan welcomeScreenResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.apply(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Unable to set Discord welcome screen", err.Error())
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Unable to read welcome screen after create", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *welcomeScreenResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state welcomeScreenResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.readInto(ctx, &state); err != nil {
		if notFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read Discord welcome screen", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *welcomeScreenResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan welcomeScreenResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.apply(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Unable to update Discord welcome screen", err.Error())
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Unable to read welcome screen after update", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete disables the welcome screen (it cannot be removed entirely).
func (r *welcomeScreenResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state welcomeScreenResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.Write(ctx, "PATCH", r.screenPath(&state), map[string]any{"enabled": false}, nil); err != nil && !notFound(err) {
		resp.Diagnostics.AddError("Unable to disable Discord welcome screen", err.Error())
	}
}

// ImportState accepts the guild id.
func (r *welcomeScreenResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("server_id"), req.ID)...)
}

func (r *welcomeScreenResource) readInto(ctx context.Context, m *welcomeScreenResourceModel) error {
	var a welcomeScreenWire
	if err := r.client.Get(ctx, r.screenPath(m), &a); err != nil {
		return err
	}
	m.Description = types.StringPointerValue(a.Description)
	models := make([]welcomeChannelModel, 0, len(a.WelcomeChannels))
	for _, c := range a.WelcomeChannels {
		models = append(models, welcomeChannelModel{
			ChannelID:   types.StringValue(c.ChannelID),
			Description: types.StringValue(c.Description),
			EmojiID:     types.StringPointerValue(c.EmojiID),
			EmojiName:   types.StringPointerValue(c.EmojiName),
		})
	}
	list, d := types.ListValueFrom(ctx, types.ObjectType{AttrTypes: welcomeChannelAttrTypes}, models)
	if d.HasError() {
		return fmt.Errorf("building welcome_channels state: %v", d.Errors())
	}
	m.WelcomeChannels = list
	return nil
}
