package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/kirchDev/terraform-provider-discord/internal/client"
)

// --- Manages a channel webhook. avatar_data_uri is a write-only base64 data URI
// the API never returns, so it isn't refreshed in Read. The webhook is addressed
// globally by id (/webhooks/{id}). ---

var (
	_ resource.Resource                = (*webhookResource)(nil)
	_ resource.ResourceWithConfigure   = (*webhookResource)(nil)
	_ resource.ResourceWithImportState = (*webhookResource)(nil)
)

// NewWebhookResource returns a new discord_webhook resource.
func NewWebhookResource() resource.Resource {
	return &webhookResource{}
}

type webhookResource struct {
	client *client.Client
}

type webhookResourceModel struct {
	ChannelID     types.String `tfsdk:"channel_id"`
	ID            types.String `tfsdk:"id"`
	Name          types.String `tfsdk:"name"`
	AvatarDataURI types.String `tfsdk:"avatar_data_uri"`
	GuildID       types.String `tfsdk:"guild_id"`
	Token         types.String `tfsdk:"token"`
	URL           types.String `tfsdk:"url"`
}

func (r *webhookResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_webhook"
}

func (r *webhookResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a webhook attached to a Discord channel.",
		Attributes: map[string]schema.Attribute{
			"channel_id": schema.StringAttribute{
				MarkdownDescription: "Snowflake ID of the channel the webhook posts to.",
				Required:            true,
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "Snowflake ID of the webhook.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Name of the webhook.",
				Required:            true,
			},
			"avatar_data_uri": schema.StringAttribute{
				MarkdownDescription: "Base64 image data URI for the webhook avatar. Write-only: not refreshed from the API.",
				Optional:            true,
			},
			"guild_id": schema.StringAttribute{
				MarkdownDescription: "Snowflake ID of the guild the webhook's channel belongs to.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"token": schema.StringAttribute{
				MarkdownDescription: "Secret token used to execute the webhook.",
				Computed:            true,
				Sensitive:           true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"url": schema.StringAttribute{
				MarkdownDescription: "The full webhook execution URL.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *webhookResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// webhookAttributes mirrors a Discord webhook object.
type webhookAttributes struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	ChannelID string `json:"channel_id"`
	GuildID   string `json:"guild_id"`
	Token     string `json:"token"`
}

func (r *webhookResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan webhookResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := map[string]any{"name": plan.Name.ValueString()}
	if v := plan.AvatarDataURI; !v.IsNull() && !v.IsUnknown() {
		body["avatar"] = v.ValueString()
	}

	var created webhookAttributes
	if err := r.client.Write(ctx, "POST", "/channels/"+plan.ChannelID.ValueString()+"/webhooks", body, &created); err != nil {
		resp.Diagnostics.AddError("Unable to create Discord webhook", err.Error())
		return
	}
	plan.ID = types.StringValue(created.ID)

	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Unable to read webhook after create", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *webhookResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state webhookResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.readInto(ctx, &state); err != nil {
		if notFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read Discord webhook", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *webhookResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan webhookResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := map[string]any{"name": plan.Name.ValueString(), "channel_id": plan.ChannelID.ValueString()}
	if v := plan.AvatarDataURI; !v.IsNull() && !v.IsUnknown() {
		body["avatar"] = v.ValueString()
	}
	if err := r.client.Write(ctx, "PATCH", "/webhooks/"+plan.ID.ValueString(), body, nil); err != nil {
		resp.Diagnostics.AddError("Unable to update Discord webhook", err.Error())
		return
	}

	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Unable to read webhook after update", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *webhookResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state webhookResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.Delete(ctx, "/webhooks/"+state.ID.ValueString()); err != nil && !notFound(err) {
		resp.Diagnostics.AddError("Unable to delete Discord webhook", err.Error())
	}
}

// ImportState accepts the webhook id.
func (r *webhookResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

// readInto GETs the webhook by id and refreshes its fields. avatar_data_uri is
// write-only and left as-is.
func (r *webhookResource) readInto(ctx context.Context, m *webhookResourceModel) error {
	var a webhookAttributes
	if err := r.client.Get(ctx, "/webhooks/"+m.ID.ValueString(), &a); err != nil {
		return err
	}
	m.ID = types.StringValue(a.ID)
	m.Name = types.StringValue(a.Name)
	m.ChannelID = types.StringValue(a.ChannelID)
	m.GuildID = types.StringValue(a.GuildID)
	m.Token = types.StringValue(a.Token)
	if a.Token != "" {
		m.URL = types.StringValue("https://discord.com/api/webhooks/" + a.ID + "/" + a.Token)
	} else {
		m.URL = types.StringNull()
	}
	return nil
}
