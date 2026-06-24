package provider

import (
	"context"
	"encoding/json"
	"fmt"

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

// --- Guild onboarding (a singleton). The onboarding prompts are deeply nested
// (prompts → options → channel/role ids + emoji), so they are passed through as
// raw JSON in `prompts_json` (write-only; not refreshed). The scalar settings
// (enabled, mode, default_channel_ids) are managed normally. ---

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

type onboardingWire struct {
	Enabled           bool     `json:"enabled"`
	Mode              int64    `json:"mode"`
	DefaultChannelIDs []string `json:"default_channel_ids"`
}

type serverOnboardingResourceModel struct {
	ServerID          types.String `tfsdk:"server_id"`
	Enabled           types.Bool   `tfsdk:"enabled"`
	Mode              types.Int64  `tfsdk:"mode"`
	DefaultChannelIDs types.Set    `tfsdk:"default_channel_ids"`
	PromptsJSON       types.String `tfsdk:"prompts_json"`
}

func (r *serverOnboardingResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_server_onboarding"
}

func (r *serverOnboardingResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the onboarding configuration of a Community Discord guild (a singleton per guild).",
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
			"default_channel_ids": schema.SetAttribute{
				MarkdownDescription: "Channel ids members are opted into by default.",
				ElementType:         types.StringType,
				Optional:            true,
				Computed:            true,
			},
			"prompts_json": schema.StringAttribute{
				MarkdownDescription: "Onboarding prompts as a raw JSON array (Discord's `prompts` field). Write-only — it is sent on apply but not refreshed into state, so it does not detect drift.",
				Optional:            true,
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
	if v := m.PromptsJSON; !v.IsNull() && !v.IsUnknown() {
		var prompts any
		if err := json.Unmarshal([]byte(v.ValueString()), &prompts); err != nil {
			return fmt.Errorf("prompts_json is not valid JSON: %w", err)
		}
		body["prompts"] = prompts
	} else {
		body["prompts"] = []any{}
	}
	return r.client.Write(ctx, "PUT", r.onboardingPath(m), body, nil)
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

// ImportState accepts the guild id.
func (r *serverOnboardingResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("server_id"), req.ID)...)
}

// readInto refreshes the scalar settings (prompts_json is write-only).
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
	return nil
}
