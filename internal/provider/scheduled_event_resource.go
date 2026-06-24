package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/kirchDev/terraform-provider-discord/internal/client"
)

// --- Manages a guild scheduled event. Events live under
// /guilds/{server_id}/scheduled-events. For external events the venue is carried
// in the nested entity_metadata.location; stage/voice events use channel_id. ---

var (
	_ resource.Resource                = (*scheduledEventResource)(nil)
	_ resource.ResourceWithConfigure   = (*scheduledEventResource)(nil)
	_ resource.ResourceWithImportState = (*scheduledEventResource)(nil)
)

// NewScheduledEventResource returns a new discord_scheduled_event resource.
func NewScheduledEventResource() resource.Resource {
	return &scheduledEventResource{}
}

type scheduledEventResource struct {
	client *client.Client
}

type scheduledEventResourceModel struct {
	ServerID           types.String `tfsdk:"server_id"`
	ID                 types.String `tfsdk:"id"`
	Name               types.String `tfsdk:"name"`
	Description        types.String `tfsdk:"description"`
	ScheduledStartTime types.String `tfsdk:"scheduled_start_time"`
	ScheduledEndTime   types.String `tfsdk:"scheduled_end_time"`
	PrivacyLevel       types.Int64  `tfsdk:"privacy_level"`
	EntityType         types.Int64  `tfsdk:"entity_type"`
	ChannelID          types.String `tfsdk:"channel_id"`
	Location           types.String `tfsdk:"location"`
	RecurrenceRuleJSON types.String `tfsdk:"recurrence_rule_json"`
	ImageDataURI       types.String `tfsdk:"image_data_uri"`
	ImageHash          types.String `tfsdk:"image_hash"`
	Status             types.Int64  `tfsdk:"status"`
}

func (r *scheduledEventResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_scheduled_event"
}

func (r *scheduledEventResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a scheduled event within a Discord guild.",
		Attributes: map[string]schema.Attribute{
			"server_id": schema.StringAttribute{
				MarkdownDescription: "Snowflake ID of the guild.",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "Snowflake ID of the scheduled event.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Name of the event.",
				Required:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Description of the event.",
				Optional:            true,
			},
			"scheduled_start_time": schema.StringAttribute{
				MarkdownDescription: "ISO 8601 timestamp at which the event starts.",
				Required:            true,
			},
			"scheduled_end_time": schema.StringAttribute{
				MarkdownDescription: "ISO 8601 timestamp at which the event ends (required for external events).",
				Optional:            true,
			},
			"privacy_level": schema.Int64Attribute{
				MarkdownDescription: "Privacy level of the event (2 = guild only).",
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(2),
				Validators:          []validator.Int64{int64OneOf(2)},
			},
			"entity_type": schema.Int64Attribute{
				MarkdownDescription: "Type of the event: 1 (stage instance), 2 (voice), or 3 (external).",
				Required:            true,
				Validators:          []validator.Int64{int64OneOf(1, 2, 3)},
			},
			"channel_id": schema.StringAttribute{
				MarkdownDescription: "Snowflake ID of the channel the event is hosted in (required for stage and voice events).",
				Optional:            true,
			},
			"location": schema.StringAttribute{
				MarkdownDescription: "Location of an external event (maps to `entity_metadata.location`).",
				Optional:            true,
			},
			"recurrence_rule_json": schema.StringAttribute{
				MarkdownDescription: "Recurrence rule for a recurring event, as a raw JSON object (Discord's `recurrence_rule`). Write-only — sent on apply but not refreshed.",
				Optional:            true,
			},
			"image_data_uri": schema.StringAttribute{
				MarkdownDescription: "Cover image as a base64 data URI (e.g. from `discord_local_file`). Write-only; Discord returns a hash in `image_hash`.",
				Optional:            true,
			},
			"image_hash": schema.StringAttribute{MarkdownDescription: "Current cover image hash.", Computed: true},
			"status": schema.Int64Attribute{
				MarkdownDescription: "Status of the event: 1 (scheduled), 2 (active), 3 (completed), 4 (cancelled).",
				Computed:            true,
			},
		},
	}
}

func (r *scheduledEventResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *scheduledEventResource) eventsBase(m *scheduledEventResourceModel) string {
	return "/guilds/" + m.ServerID.ValueString() + "/scheduled-events"
}

// body collects the settable event fields from the plan.
func (r *scheduledEventResource) body(m *scheduledEventResourceModel) map[string]any {
	body := map[string]any{
		"name":                 m.Name.ValueString(),
		"privacy_level":        m.PrivacyLevel.ValueInt64(),
		"entity_type":          m.EntityType.ValueInt64(),
		"scheduled_start_time": m.ScheduledStartTime.ValueString(),
	}
	if v := m.Description; !v.IsNull() && !v.IsUnknown() {
		body["description"] = v.ValueString()
	}
	if v := m.ScheduledEndTime; !v.IsNull() && !v.IsUnknown() {
		body["scheduled_end_time"] = v.ValueString()
	}
	if v := m.ChannelID; !v.IsNull() && !v.IsUnknown() {
		body["channel_id"] = v.ValueString()
	}
	if v := m.Location; !v.IsNull() && !v.IsUnknown() {
		body["entity_metadata"] = map[string]any{"location": v.ValueString()}
	}
	if v := m.ImageDataURI; !v.IsNull() && !v.IsUnknown() {
		body["image"] = v.ValueString()
	}
	return body
}

// fullBody is body() plus the optional raw-JSON recurrence rule.
func (r *scheduledEventResource) fullBody(m *scheduledEventResourceModel) (map[string]any, error) {
	body := r.body(m)
	if v := m.RecurrenceRuleJSON; !v.IsNull() && !v.IsUnknown() {
		var rr any
		if err := json.Unmarshal([]byte(v.ValueString()), &rr); err != nil {
			return nil, fmt.Errorf("recurrence_rule_json is not valid JSON: %w", err)
		}
		body["recurrence_rule"] = rr
	}
	return body, nil
}

func (r *scheduledEventResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan scheduledEventResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body, err := r.fullBody(&plan)
	if err != nil {
		resp.Diagnostics.AddError("Invalid scheduled event configuration", err.Error())
		return
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := r.client.Write(ctx, "POST", r.eventsBase(&plan), body, &created); err != nil {
		resp.Diagnostics.AddError("Unable to create Discord scheduled event", err.Error())
		return
	}
	plan.ID = types.StringValue(created.ID)

	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Unable to read scheduled event after create", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *scheduledEventResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state scheduledEventResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.readInto(ctx, &state); err != nil {
		if notFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read Discord scheduled event", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *scheduledEventResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan scheduledEventResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body, err := r.fullBody(&plan)
	if err != nil {
		resp.Diagnostics.AddError("Invalid scheduled event configuration", err.Error())
		return
	}
	if err := r.client.Write(ctx, "PATCH", r.eventsBase(&plan)+"/"+plan.ID.ValueString(), body, nil); err != nil {
		resp.Diagnostics.AddError("Unable to update Discord scheduled event", err.Error())
		return
	}

	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Unable to read scheduled event after update", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *scheduledEventResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state scheduledEventResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.Delete(ctx, r.eventsBase(&state)+"/"+state.ID.ValueString()); err != nil && !notFound(err) {
		resp.Diagnostics.AddError("Unable to delete Discord scheduled event", err.Error())
	}
}

// ImportState accepts "server_id/event_id".
func (r *scheduledEventResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, "/")
	if len(parts) != 2 {
		resp.Diagnostics.AddError("Invalid import ID", "Expected \"server_id/event_id\".")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("server_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[1])...)
}

// readInto GETs the scheduled event by id and refreshes its fields.
func (r *scheduledEventResource) readInto(ctx context.Context, m *scheduledEventResourceModel) error {
	var a struct {
		ID                 string  `json:"id"`
		Name               string  `json:"name"`
		Description        *string `json:"description"`
		ScheduledStartTime string  `json:"scheduled_start_time"`
		ScheduledEndTime   *string `json:"scheduled_end_time"`
		PrivacyLevel       int64   `json:"privacy_level"`
		EntityType         int64   `json:"entity_type"`
		ChannelID          *string `json:"channel_id"`
		Status             int64   `json:"status"`
		Image              *string `json:"image"`
		EntityMetadata     *struct {
			Location *string `json:"location"`
		} `json:"entity_metadata"`
	}
	if err := r.client.Get(ctx, r.eventsBase(m)+"/"+m.ID.ValueString(), &a); err != nil {
		return err
	}
	m.Name = types.StringValue(a.Name)
	m.Description = types.StringPointerValue(a.Description)
	m.ScheduledStartTime = keepTimestamp(m.ScheduledStartTime, a.ScheduledStartTime)
	endVal := ""
	if a.ScheduledEndTime != nil {
		endVal = *a.ScheduledEndTime
	}
	m.ScheduledEndTime = keepTimestamp(m.ScheduledEndTime, endVal)
	m.PrivacyLevel = types.Int64Value(a.PrivacyLevel)
	m.EntityType = types.Int64Value(a.EntityType)
	m.ChannelID = types.StringPointerValue(a.ChannelID)
	m.Status = types.Int64Value(a.Status)
	m.ImageHash = types.StringPointerValue(a.Image)
	if a.EntityMetadata != nil {
		m.Location = types.StringPointerValue(a.EntityMetadata.Location)
	} else {
		m.Location = types.StringNull()
	}
	return nil
}
