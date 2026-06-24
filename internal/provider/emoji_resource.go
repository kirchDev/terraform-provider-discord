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

// --- Manages a custom guild emoji. image_data_uri is a write-only base64 data
// URI the API never returns, so it isn't refreshed in Read. Emojis live under
// /guilds/{server_id}/emojis. ---

var (
	_ resource.Resource                = (*emojiResource)(nil)
	_ resource.ResourceWithConfigure   = (*emojiResource)(nil)
	_ resource.ResourceWithImportState = (*emojiResource)(nil)
)

// NewEmojiResource returns a new discord_emoji resource.
func NewEmojiResource() resource.Resource {
	return &emojiResource{}
}

type emojiResource struct {
	client *client.Client
}

type emojiResourceModel struct {
	ServerID     types.String `tfsdk:"server_id"`
	ID           types.String `tfsdk:"id"`
	Name         types.String `tfsdk:"name"`
	ImageDataURI types.String `tfsdk:"image_data_uri"`
	Roles        types.Set    `tfsdk:"roles"`
	Animated     types.Bool   `tfsdk:"animated"`
	Available    types.Bool   `tfsdk:"available"`
}

// emojiAttributes mirrors a Discord emoji object.
type emojiAttributes struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Animated  bool     `json:"animated"`
	Available bool     `json:"available"`
	Roles     []string `json:"roles"`
}

func (r *emojiResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_emoji"
}

func (r *emojiResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a custom emoji within a Discord guild.",
		Attributes: map[string]schema.Attribute{
			"server_id": schema.StringAttribute{
				MarkdownDescription: "Snowflake ID of the guild.",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "Snowflake ID of the emoji.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Emoji name.",
				Required:            true,
			},
			"image_data_uri": schema.StringAttribute{
				MarkdownDescription: "Base64 image data URI for the emoji. Write-only: applied at creation and not refreshed from the API.",
				Optional:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"roles": schema.SetAttribute{
				MarkdownDescription: "Snowflake IDs of the roles allowed to use this emoji.",
				ElementType:         types.StringType,
				Optional:            true,
			},
			"animated": schema.BoolAttribute{
				MarkdownDescription: "Whether the emoji is animated.",
				Computed:            true,
			},
			"available": schema.BoolAttribute{
				MarkdownDescription: "Whether the emoji is available for use (may be false if the guild lost boosts).",
				Computed:            true,
			},
		},
	}
}

func (r *emojiResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *emojiResource) emojiBase(m *emojiResourceModel) string {
	return "/guilds/" + m.ServerID.ValueString() + "/emojis"
}

func (r *emojiResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan emojiResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := map[string]any{"name": plan.Name.ValueString()}
	if v := plan.ImageDataURI; !v.IsNull() && !v.IsUnknown() {
		body["image"] = v.ValueString()
	}
	if !plan.Roles.IsNull() && !plan.Roles.IsUnknown() {
		var roles []string
		resp.Diagnostics.Append(plan.Roles.ElementsAs(ctx, &roles, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		body["roles"] = roles
	}

	var created emojiAttributes
	if err := r.client.Write(ctx, "POST", r.emojiBase(&plan), body, &created); err != nil {
		resp.Diagnostics.AddError("Unable to create Discord emoji", err.Error())
		return
	}
	plan.ID = types.StringValue(created.ID)

	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Unable to read emoji after create", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *emojiResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state emojiResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.readInto(ctx, &state); err != nil {
		if notFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read Discord emoji", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *emojiResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan emojiResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := map[string]any{"name": plan.Name.ValueString()}
	if !plan.Roles.IsNull() && !plan.Roles.IsUnknown() {
		var roles []string
		resp.Diagnostics.Append(plan.Roles.ElementsAs(ctx, &roles, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		body["roles"] = roles
	}
	if err := r.client.Write(ctx, "PATCH", r.emojiBase(&plan)+"/"+plan.ID.ValueString(), body, nil); err != nil {
		resp.Diagnostics.AddError("Unable to update Discord emoji", err.Error())
		return
	}

	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Unable to read emoji after update", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *emojiResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state emojiResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.Delete(ctx, r.emojiBase(&state)+"/"+state.ID.ValueString()); err != nil && !notFound(err) {
		resp.Diagnostics.AddError("Unable to delete Discord emoji", err.Error())
	}
}

// ImportState accepts "server_id/emoji_id".
func (r *emojiResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, "/")
	if len(parts) != 2 {
		resp.Diagnostics.AddError("Invalid import ID", "Expected \"server_id/emoji_id\".")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("server_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[1])...)
}

// readInto GETs the emoji by id and refreshes its fields. image_data_uri is
// write-only and left as-is.
func (r *emojiResource) readInto(ctx context.Context, m *emojiResourceModel) error {
	var a emojiAttributes
	if err := r.client.Get(ctx, r.emojiBase(m)+"/"+m.ID.ValueString(), &a); err != nil {
		return err
	}
	m.Name = types.StringValue(a.Name)
	m.Animated = types.BoolValue(a.Animated)
	m.Available = types.BoolValue(a.Available)
	set, d := types.SetValueFrom(ctx, types.StringType, a.Roles)
	if d.HasError() {
		return fmt.Errorf("building emoji role set: %v", d.Errors())
	}
	m.Roles = set
	return nil
}
