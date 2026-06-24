package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/kirchDev/terraform-provider-discord/internal/client"
)

// --- Manages an application-owned (bot) emoji. image_data_uri is a write-only
// base64 data URI the API never returns, so it isn't refreshed in Read. roles is
// only settable on create; the modify endpoint accepts name only. Application
// emojis live under /applications/{application_id}/emojis. ---

var (
	_ resource.Resource                = (*applicationEmojiResource)(nil)
	_ resource.ResourceWithConfigure   = (*applicationEmojiResource)(nil)
	_ resource.ResourceWithImportState = (*applicationEmojiResource)(nil)
)

// NewApplicationEmojiResource returns a new discord_application_emoji resource.
func NewApplicationEmojiResource() resource.Resource {
	return &applicationEmojiResource{}
}

type applicationEmojiResource struct {
	client *client.Client
}

type applicationEmojiResourceModel struct {
	ApplicationID types.String `tfsdk:"application_id"`
	ID            types.String `tfsdk:"id"`
	Name          types.String `tfsdk:"name"`
	ImageDataURI  types.String `tfsdk:"image_data_uri"`
	Roles         types.Set    `tfsdk:"roles"`
	Animated      types.Bool   `tfsdk:"animated"`
	Available     types.Bool   `tfsdk:"available"`
	Managed       types.Bool   `tfsdk:"managed"`
}

// applicationEmojiAttributes mirrors a Discord application emoji object.
type applicationEmojiAttributes struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Animated  bool     `json:"animated"`
	Available bool     `json:"available"`
	Managed   bool     `json:"managed"`
	Roles     []string `json:"roles"`
}

func (r *applicationEmojiResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_application_emoji"
}

func (r *applicationEmojiResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages an emoji owned by a Discord application (bot).",
		Attributes: map[string]schema.Attribute{
			"application_id": schema.StringAttribute{
				MarkdownDescription: "Snowflake ID of the application the emoji belongs to.",
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
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"roles": schema.SetAttribute{
				MarkdownDescription: "Snowflake IDs of the roles allowed to use this emoji. Only settable on creation; changing it forces replacement.",
				ElementType:         types.StringType,
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Set{setplanmodifier.RequiresReplace()},
			},
			"animated": schema.BoolAttribute{
				MarkdownDescription: "Whether the emoji is animated.",
				Computed:            true,
			},
			"available": schema.BoolAttribute{
				MarkdownDescription: "Whether the emoji is available for use.",
				Computed:            true,
			},
			"managed": schema.BoolAttribute{
				MarkdownDescription: "Whether the emoji is managed by an integration (read-only).",
				Computed:            true,
			},
		},
	}
}

func (r *applicationEmojiResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *applicationEmojiResource) emojisPath(m *applicationEmojiResourceModel) string {
	return "/applications/" + m.ApplicationID.ValueString() + "/emojis"
}

func (r *applicationEmojiResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan applicationEmojiResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := map[string]any{
		"name":  plan.Name.ValueString(),
		"image": plan.ImageDataURI.ValueString(),
	}
	if !plan.Roles.IsNull() && !plan.Roles.IsUnknown() {
		var roles []string
		resp.Diagnostics.Append(plan.Roles.ElementsAs(ctx, &roles, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		body["roles"] = roles
	}

	var created applicationEmojiAttributes
	if err := r.client.Write(ctx, "POST", r.emojisPath(&plan), body, &created); err != nil {
		resp.Diagnostics.AddError("Unable to create Discord application emoji", err.Error())
		return
	}
	plan.ID = types.StringValue(created.ID)

	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Unable to read application emoji after create", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *applicationEmojiResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state applicationEmojiResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.readInto(ctx, &state); err != nil {
		if notFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read Discord application emoji", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *applicationEmojiResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan applicationEmojiResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := map[string]any{"name": plan.Name.ValueString()}
	if err := r.client.Write(ctx, "PATCH", r.emojisPath(&plan)+"/"+plan.ID.ValueString(), body, nil); err != nil {
		resp.Diagnostics.AddError("Unable to update Discord application emoji", err.Error())
		return
	}

	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Unable to read application emoji after update", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *applicationEmojiResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state applicationEmojiResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.Delete(ctx, r.emojisPath(&state)+"/"+state.ID.ValueString()); err != nil && !notFound(err) {
		resp.Diagnostics.AddError("Unable to delete Discord application emoji", err.Error())
	}
}

// ImportState accepts "application_id/emoji_id".
func (r *applicationEmojiResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, "/")
	if len(parts) != 2 {
		resp.Diagnostics.AddError("Invalid import ID", "Expected \"application_id/emoji_id\".")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("application_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[1])...)
}

// readInto GETs the emoji by id and refreshes its fields. image_data_uri is
// write-only and left as-is.
func (r *applicationEmojiResource) readInto(ctx context.Context, m *applicationEmojiResourceModel) error {
	var a applicationEmojiAttributes
	if err := r.client.Get(ctx, r.emojisPath(m)+"/"+m.ID.ValueString(), &a); err != nil {
		return err
	}
	m.Name = types.StringValue(a.Name)
	m.Animated = types.BoolValue(a.Animated)
	m.Available = types.BoolValue(a.Available)
	m.Managed = types.BoolValue(a.Managed)
	set, d := types.SetValueFrom(ctx, types.StringType, a.Roles)
	if d.HasError() {
		return fmt.Errorf("building application emoji role set: %v", d.Errors())
	}
	m.Roles = set
	return nil
}
