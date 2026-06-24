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

// --- Manages a custom guild sticker. The sticker file is uploaded via
// multipart/form-data at creation (the only multipart resource); file_data_uri is
// write-only and not refreshed in Read since Discord cannot change the file after
// create. Stickers live under /guilds/{server_id}/stickers. ---

var (
	_ resource.Resource                = (*stickerResource)(nil)
	_ resource.ResourceWithConfigure   = (*stickerResource)(nil)
	_ resource.ResourceWithImportState = (*stickerResource)(nil)
)

// NewStickerResource returns a new discord_sticker resource.
func NewStickerResource() resource.Resource {
	return &stickerResource{}
}

type stickerResource struct {
	client *client.Client
}

type stickerResourceModel struct {
	ServerID    types.String `tfsdk:"server_id"`
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Tags        types.String `tfsdk:"tags"`
	FileDataURI types.String `tfsdk:"file_data_uri"`
	FormatType  types.Int64  `tfsdk:"format_type"`
	Available   types.Bool   `tfsdk:"available"`
}

// stickerAttributes mirrors a Discord sticker object.
type stickerAttributes struct {
	Name        string  `json:"name"`
	Description *string `json:"description"`
	Tags        string  `json:"tags"`
	FormatType  int64   `json:"format_type"`
	Available   bool    `json:"available"`
}

func (r *stickerResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_sticker"
}

func (r *stickerResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a custom sticker within a Discord guild.",
		Attributes: map[string]schema.Attribute{
			"server_id": schema.StringAttribute{
				MarkdownDescription: "Snowflake ID of the guild the sticker belongs to.",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "Snowflake ID of the sticker.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Sticker name.",
				Required:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Sticker description (may be empty).",
				Required:            true,
			},
			"tags": schema.StringAttribute{
				MarkdownDescription: "Autocomplete/suggestion tags for the sticker.",
				Required:            true,
			},
			"file_data_uri": schema.StringAttribute{
				MarkdownDescription: "Base64 data URI of the sticker file. Write-only: uploaded at creation and not refreshed from the API. The API cannot change the file after create.",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"format_type": schema.Int64Attribute{
				MarkdownDescription: "Format of the sticker (`1` PNG, `2` APNG, `3` Lottie, `4` GIF).",
				Computed:            true,
			},
			"available": schema.BoolAttribute{
				MarkdownDescription: "Whether the sticker is available for use (may be false if the guild lost boosts).",
				Computed:            true,
			},
		},
	}
}

func (r *stickerResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *stickerResource) stickersPath(m *stickerResourceModel) string {
	return "/guilds/" + m.ServerID.ValueString() + "/stickers"
}

func (r *stickerResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan stickerResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	mime, content, err := decodeDataURI(plan.FileDataURI.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Invalid sticker file data URI", err.Error())
		return
	}
	fileName := "sticker." + extForMime(mime)
	fields := map[string]string{
		"name":        plan.Name.ValueString(),
		"description": plan.Description.ValueString(),
		"tags":        plan.Tags.ValueString(),
	}

	var created struct {
		ID         string `json:"id"`
		FormatType int64  `json:"format_type"`
		Available  bool   `json:"available"`
	}
	if err := r.client.WriteMultipart(ctx, "POST", r.stickersPath(&plan), fields, "file", fileName, mime, content, &created); err != nil {
		resp.Diagnostics.AddError("Unable to create Discord sticker", err.Error())
		return
	}
	plan.ID = types.StringValue(created.ID)

	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Unable to read sticker after create", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *stickerResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state stickerResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.readInto(ctx, &state); err != nil {
		if notFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read Discord sticker", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *stickerResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan stickerResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := map[string]any{
		"name":        plan.Name.ValueString(),
		"description": plan.Description.ValueString(),
		"tags":        plan.Tags.ValueString(),
	}
	if err := r.client.Write(ctx, "PATCH", r.stickersPath(&plan)+"/"+plan.ID.ValueString(), body, nil); err != nil {
		resp.Diagnostics.AddError("Unable to update Discord sticker", err.Error())
		return
	}

	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Unable to read sticker after update", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *stickerResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state stickerResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.Delete(ctx, r.stickersPath(&state)+"/"+state.ID.ValueString()); err != nil && !notFound(err) {
		resp.Diagnostics.AddError("Unable to delete Discord sticker", err.Error())
	}
}

// ImportState accepts "server_id/sticker_id".
func (r *stickerResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, "/")
	if len(parts) != 2 {
		resp.Diagnostics.AddError("Invalid import ID", "Expected \"server_id/sticker_id\".")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("server_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[1])...)
}

// readInto GETs the sticker by id and refreshes its fields. file_data_uri is
// write-only and left as-is.
func (r *stickerResource) readInto(ctx context.Context, m *stickerResourceModel) error {
	var a stickerAttributes
	if err := r.client.Get(ctx, r.stickersPath(m)+"/"+m.ID.ValueString(), &a); err != nil {
		return err
	}
	m.Name = types.StringValue(a.Name)
	m.Description = types.StringPointerValue(a.Description)
	m.Tags = types.StringValue(a.Tags)
	m.FormatType = types.Int64Value(a.FormatType)
	m.Available = types.BoolValue(a.Available)
	return nil
}
