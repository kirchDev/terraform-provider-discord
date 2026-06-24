package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/float64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/kirchDev/terraform-provider-discord/internal/client"
)

// --- Manages a guild soundboard sound. The sound file is supplied as a base64
// data URI in the JSON body at creation; sound_data_uri is write-only and not
// refreshed in Read. Sounds live under /guilds/{server_id}/soundboard-sounds. ---

var (
	_ resource.Resource                = (*soundboardSoundResource)(nil)
	_ resource.ResourceWithConfigure   = (*soundboardSoundResource)(nil)
	_ resource.ResourceWithImportState = (*soundboardSoundResource)(nil)
)

// NewSoundboardSoundResource returns a new discord_soundboard_sound resource.
func NewSoundboardSoundResource() resource.Resource {
	return &soundboardSoundResource{}
}

type soundboardSoundResource struct {
	client *client.Client
}

type soundboardSoundResourceModel struct {
	ServerID     types.String  `tfsdk:"server_id"`
	ID           types.String  `tfsdk:"id"`
	Name         types.String  `tfsdk:"name"`
	SoundDataURI types.String  `tfsdk:"sound_data_uri"`
	Volume       types.Float64 `tfsdk:"volume"`
	EmojiID      types.String  `tfsdk:"emoji_id"`
	EmojiName    types.String  `tfsdk:"emoji_name"`
	Available    types.Bool    `tfsdk:"available"`
}

// soundboardSoundAttributes mirrors a Discord soundboard sound object.
type soundboardSoundAttributes struct {
	Name      string  `json:"name"`
	Volume    float64 `json:"volume"`
	EmojiID   *string `json:"emoji_id"`
	EmojiName *string `json:"emoji_name"`
	Available bool    `json:"available"`
}

func (r *soundboardSoundResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_soundboard_sound"
}

func (r *soundboardSoundResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a soundboard sound within a Discord guild.",
		Attributes: map[string]schema.Attribute{
			"server_id": schema.StringAttribute{
				MarkdownDescription: "Snowflake ID of the guild the sound belongs to.",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "Snowflake ID of the soundboard sound.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Sound name.",
				Required:            true,
			},
			"sound_data_uri": schema.StringAttribute{
				MarkdownDescription: "Base64 data URI of the sound file (mp3 or ogg). Write-only: uploaded at creation and not refreshed from the API.",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"volume": schema.Float64Attribute{
				MarkdownDescription: "Playback volume of the sound, between 0 and 1.",
				Optional:            true,
				Computed:            true,
				Default:             float64default.StaticFloat64(1),
			},
			"emoji_id": schema.StringAttribute{
				MarkdownDescription: "Snowflake ID of a custom emoji shown for the sound.",
				Optional:            true,
			},
			"emoji_name": schema.StringAttribute{
				MarkdownDescription: "Unicode emoji shown for the sound.",
				Optional:            true,
			},
			"available": schema.BoolAttribute{
				MarkdownDescription: "Whether the sound is available for use (may be false if the guild lost boosts).",
				Computed:            true,
			},
		},
	}
}

func (r *soundboardSoundResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *soundboardSoundResource) soundsPath(m *soundboardSoundResourceModel) string {
	return "/guilds/" + m.ServerID.ValueString() + "/soundboard-sounds"
}

func (r *soundboardSoundResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan soundboardSoundResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := map[string]any{
		"name":  plan.Name.ValueString(),
		"sound": plan.SoundDataURI.ValueString(),
	}
	if v := plan.Volume; !v.IsNull() && !v.IsUnknown() {
		body["volume"] = v.ValueFloat64()
	}
	if v := plan.EmojiID; !v.IsNull() && !v.IsUnknown() {
		body["emoji_id"] = v.ValueString()
	}
	if v := plan.EmojiName; !v.IsNull() && !v.IsUnknown() {
		body["emoji_name"] = v.ValueString()
	}

	var created struct {
		SoundID string `json:"sound_id"`
	}
	if err := r.client.Write(ctx, "POST", r.soundsPath(&plan), body, &created); err != nil {
		resp.Diagnostics.AddError("Unable to create Discord soundboard sound", err.Error())
		return
	}
	plan.ID = types.StringValue(created.SoundID)

	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Unable to read soundboard sound after create", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *soundboardSoundResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state soundboardSoundResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.readInto(ctx, &state); err != nil {
		if notFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read Discord soundboard sound", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *soundboardSoundResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan soundboardSoundResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := map[string]any{"name": plan.Name.ValueString()}
	if v := plan.Volume; !v.IsNull() && !v.IsUnknown() {
		body["volume"] = v.ValueFloat64()
	}
	if v := plan.EmojiID; !v.IsNull() && !v.IsUnknown() {
		body["emoji_id"] = v.ValueString()
	}
	if v := plan.EmojiName; !v.IsNull() && !v.IsUnknown() {
		body["emoji_name"] = v.ValueString()
	}
	if err := r.client.Write(ctx, "PATCH", r.soundsPath(&plan)+"/"+plan.ID.ValueString(), body, nil); err != nil {
		resp.Diagnostics.AddError("Unable to update Discord soundboard sound", err.Error())
		return
	}

	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Unable to read soundboard sound after update", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *soundboardSoundResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state soundboardSoundResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.Delete(ctx, r.soundsPath(&state)+"/"+state.ID.ValueString()); err != nil && !notFound(err) {
		resp.Diagnostics.AddError("Unable to delete Discord soundboard sound", err.Error())
	}
}

// ImportState accepts "server_id/sound_id".
func (r *soundboardSoundResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, "/")
	if len(parts) != 2 {
		resp.Diagnostics.AddError("Invalid import ID", "Expected \"server_id/sound_id\".")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("server_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[1])...)
}

// readInto GETs the sound by id and refreshes its fields. sound_data_uri is
// write-only and left as-is.
func (r *soundboardSoundResource) readInto(ctx context.Context, m *soundboardSoundResourceModel) error {
	var a soundboardSoundAttributes
	if err := r.client.Get(ctx, r.soundsPath(m)+"/"+m.ID.ValueString(), &a); err != nil {
		return err
	}
	m.Name = types.StringValue(a.Name)
	m.Volume = types.Float64Value(a.Volume)
	m.EmojiID = types.StringPointerValue(a.EmojiID)
	m.EmojiName = types.StringPointerValue(a.EmojiName)
	m.Available = types.BoolValue(a.Available)
	return nil
}
