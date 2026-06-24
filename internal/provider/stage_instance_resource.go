package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/kirchDev/terraform-provider-discord/internal/client"
)

var (
	_ resource.Resource                = (*stageInstanceResource)(nil)
	_ resource.ResourceWithConfigure   = (*stageInstanceResource)(nil)
	_ resource.ResourceWithImportState = (*stageInstanceResource)(nil)
)

// NewStageInstanceResource returns a new discord_stage_instance resource.
func NewStageInstanceResource() resource.Resource {
	return &stageInstanceResource{}
}

type stageInstanceResource struct {
	client *client.Client
}

// stageInstanceAttributes mirrors a Discord stage instance object. A stage
// instance is a live stage tied to a stage channel and is addressed by the
// stage channel's id (not its own id).
type stageInstanceAttributes struct {
	ID           string `json:"id"`
	Topic        string `json:"topic"`
	PrivacyLevel int64  `json:"privacy_level"`
}

type stageInstanceResourceModel struct {
	ChannelID    types.String `tfsdk:"channel_id"`
	ID           types.String `tfsdk:"id"`
	Topic        types.String `tfsdk:"topic"`
	PrivacyLevel types.Int64  `tfsdk:"privacy_level"`
}

func (r *stageInstanceResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_stage_instance"
}

func (r *stageInstanceResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a live stage instance on a Discord stage channel.",
		Attributes: map[string]schema.Attribute{
			"channel_id": schema.StringAttribute{
				MarkdownDescription: "Snowflake ID of the stage channel the stage instance lives on.",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "Snowflake ID of the stage instance.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"topic": schema.StringAttribute{
				MarkdownDescription: "Topic of the stage instance (the headline shown to listeners).",
				Required:            true,
			},
			"privacy_level": schema.Int64Attribute{
				MarkdownDescription: "Privacy level of the stage instance (`1` public, `2` guild-only). Defaults to `2`.",
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(2),
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *stageInstanceResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *stageInstanceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan stageInstanceResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := map[string]any{
		"channel_id":    plan.ChannelID.ValueString(),
		"topic":         plan.Topic.ValueString(),
		"privacy_level": plan.PrivacyLevel.ValueInt64(),
	}
	var created stageInstanceAttributes
	if err := r.client.Write(ctx, "POST", "/stage-instances", body, &created); err != nil {
		resp.Diagnostics.AddError("Unable to create Discord stage instance", err.Error())
		return
	}
	plan.ID = types.StringValue(created.ID)

	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Unable to read stage instance after create", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *stageInstanceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state stageInstanceResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.readInto(ctx, &state); err != nil {
		if notFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read Discord stage instance", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *stageInstanceResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan stageInstanceResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := map[string]any{
		"topic":         plan.Topic.ValueString(),
		"privacy_level": plan.PrivacyLevel.ValueInt64(),
	}
	if err := r.client.Write(ctx, "PATCH", "/stage-instances/"+plan.ChannelID.ValueString(), body, nil); err != nil {
		resp.Diagnostics.AddError("Unable to update Discord stage instance", err.Error())
		return
	}

	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Unable to read stage instance after update", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *stageInstanceResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state stageInstanceResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.Delete(ctx, "/stage-instances/"+state.ChannelID.ValueString()); err != nil && !notFound(err) {
		resp.Diagnostics.AddError("Unable to delete Discord stage instance", err.Error())
	}
}

// ImportState accepts the stage channel id.
func (r *stageInstanceResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("channel_id"), req.ID)...)
}

// readInto GETs the stage instance by its stage channel id and fills the model.
func (r *stageInstanceResource) readInto(ctx context.Context, m *stageInstanceResourceModel) error {
	var a stageInstanceAttributes
	if err := r.client.Get(ctx, "/stage-instances/"+m.ChannelID.ValueString(), &a); err != nil {
		return err
	}
	m.ID = types.StringValue(a.ID)
	m.Topic = types.StringValue(a.Topic)
	m.PrivacyLevel = types.Int64Value(a.PrivacyLevel)
	return nil
}
