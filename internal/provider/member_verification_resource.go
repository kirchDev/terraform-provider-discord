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

// --- Guild membership screening / member verification (Community guilds): the
// "Server Rules" gate where new members must agree before they can chat. A
// singleton on the guild, round-tripped through GET/PATCH
// /guilds/{server_id}/member-verification. It adopts rather than creates —
// Create/Update PATCH the form, Delete disables the gate. `enabled` is write-only
// (Discord does not return it), so it is not refreshed, mirroring the welcome
// screen. The form is a list of fields; the rules live in a `TERMS` field's
// `values`. The PATCH needs the bot's MANAGE_GUILD permission. ---

var (
	_ resource.Resource                = (*memberVerificationResource)(nil)
	_ resource.ResourceWithConfigure   = (*memberVerificationResource)(nil)
	_ resource.ResourceWithImportState = (*memberVerificationResource)(nil)
)

// NewMemberVerificationResource returns a new discord_member_verification resource.
func NewMemberVerificationResource() resource.Resource {
	return &memberVerificationResource{}
}

type memberVerificationResource struct {
	client *client.Client
}

// --- wire (Discord REST shapes) ---

type memberVerificationFieldWire struct {
	FieldType string   `json:"field_type"`
	Label     string   `json:"label"`
	Values    []string `json:"values"`
	Required  bool     `json:"required"`
}

type memberVerificationWire struct {
	Description *string                       `json:"description"`
	FormFields  []memberVerificationFieldWire `json:"form_fields"`
}

// --- model (tfsdk) ---

type memberVerificationFieldModel struct {
	FieldType types.String `tfsdk:"field_type"`
	Label     types.String `tfsdk:"label"`
	Values    types.List   `tfsdk:"values"`
	Required  types.Bool   `tfsdk:"required"`
}

var memberVerificationFieldAttrTypes = map[string]attr.Type{
	"field_type": types.StringType,
	"label":      types.StringType,
	"values":     types.ListType{ElemType: types.StringType},
	"required":   types.BoolType,
}

type memberVerificationResourceModel struct {
	ServerID    types.String `tfsdk:"server_id"`
	Enabled     types.Bool   `tfsdk:"enabled"`
	Description types.String `tfsdk:"description"`
	FormFields  types.List   `tfsdk:"form_fields"`
}

func (r *memberVerificationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_member_verification"
}

func (r *memberVerificationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the membership screening (\"Server Rules\") of a Community Discord guild (a singleton " +
			"per guild) — the gate where new members must agree to the rules before they can chat. The rules are the " +
			"`values` of a `TERMS` form field. The bot needs the **Manage Server** permission, and screening only applies " +
			"to Community guilds.",
		Attributes: map[string]schema.Attribute{
			"server_id": schema.StringAttribute{
				MarkdownDescription: "Snowflake ID of the guild.",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether the membership screening gate is enabled. Write-only — Discord does not " +
					"return it, so it is not refreshed.",
				Optional: true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Server description shown on the screening form.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"form_fields": schema.ListNestedAttribute{
				MarkdownDescription: "Ordered fields of the screening form. The classic rules gate is a single field of " +
					"type `TERMS` whose `values` are the rules.",
				Optional:      true,
				Computed:      true,
				PlanModifiers: []planmodifier.List{listplanmodifier.UseStateForUnknown()},
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"field_type": schema.StringAttribute{
							MarkdownDescription: "Field type (currently only `TERMS` is supported by Discord). Defaults to `TERMS`.",
							Optional:            true,
							Computed:            true,
						},
						"label": schema.StringAttribute{
							MarkdownDescription: "Title of the field (e.g. \"Read and agree to the server rules\").",
							Required:            true,
						},
						"values": schema.ListAttribute{
							MarkdownDescription: "Ordered list of rule strings shown for a `TERMS` field.",
							ElementType:         types.StringType,
							Optional:            true,
							Computed:            true,
						},
						"required": schema.BoolAttribute{
							MarkdownDescription: "Whether the member has to agree to this field. Defaults to `true`.",
							Optional:            true,
							Computed:            true,
						},
					},
				},
			},
		},
	}
}

func (r *memberVerificationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *memberVerificationResource) verificationPath(m *memberVerificationResourceModel) string {
	return "/guilds/" + m.ServerID.ValueString() + "/member-verification"
}

// apply PATCHes the screening form. The endpoint upserts, so the same call both
// creates and updates it.
func (r *memberVerificationResource) apply(ctx context.Context, m *memberVerificationResourceModel) error {
	body := map[string]any{}
	if v := m.Enabled; !v.IsNull() && !v.IsUnknown() {
		body["enabled"] = v.ValueBool()
	}
	if v := m.Description; !v.IsNull() && !v.IsUnknown() {
		body["description"] = v.ValueString()
	}
	if !m.FormFields.IsNull() && !m.FormFields.IsUnknown() {
		var fields []memberVerificationFieldModel
		if d := m.FormFields.ElementsAs(ctx, &fields, false); d.HasError() {
			return fmt.Errorf("reading form_fields")
		}
		wire := make([]map[string]any, 0, len(fields))
		for _, f := range fields {
			fieldType := "TERMS"
			if v := f.FieldType; !v.IsNull() && !v.IsUnknown() && v.ValueString() != "" {
				fieldType = v.ValueString()
			}
			required := true
			if v := f.Required; !v.IsNull() && !v.IsUnknown() {
				required = v.ValueBool()
			}
			values := []string{}
			if !f.Values.IsNull() && !f.Values.IsUnknown() {
				if d := f.Values.ElementsAs(ctx, &values, false); d.HasError() {
					return fmt.Errorf("reading form_field values")
				}
			}
			wire = append(wire, map[string]any{
				"field_type": fieldType,
				"label":      f.Label.ValueString(),
				"values":     values,
				"required":   required,
			})
		}
		body["form_fields"] = wire
	}
	return r.client.Write(ctx, "PATCH", r.verificationPath(m), body, nil)
}

func (r *memberVerificationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan memberVerificationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.apply(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Unable to set Discord membership screening", err.Error())
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Unable to read membership screening after create", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *memberVerificationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state memberVerificationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.readInto(ctx, &state); err != nil {
		if notFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read Discord membership screening", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *memberVerificationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan memberVerificationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.apply(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Unable to update Discord membership screening", err.Error())
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Unable to read membership screening after update", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete disables the screening gate (it cannot be removed entirely). Removing the
// resource without disabling would leave the gate up unmanaged.
func (r *memberVerificationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state memberVerificationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.Write(ctx, "PATCH", r.verificationPath(&state), map[string]any{"enabled": false}, nil); err != nil && !notFound(err) {
		resp.Diagnostics.AddError("Unable to disable Discord membership screening", err.Error())
	}
}

// ImportState accepts the guild id (the singleton key); Read then populates the
// form so an imported screening config plans clean.
func (r *memberVerificationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("server_id"), req.ID)...)
}

// readInto refreshes the description and the form_fields list. `enabled` is
// write-only and intentionally left untouched.
func (r *memberVerificationResource) readInto(ctx context.Context, m *memberVerificationResourceModel) error {
	var a memberVerificationWire
	if err := r.client.Get(ctx, r.verificationPath(m), &a); err != nil {
		return err
	}
	m.Description = nullIfEmpty(a.Description)

	fieldType := types.ObjectType{AttrTypes: memberVerificationFieldAttrTypes}
	models := make([]memberVerificationFieldModel, 0, len(a.FormFields))
	for _, f := range a.FormFields {
		values, d := types.ListValueFrom(ctx, types.StringType, f.Values)
		if d.HasError() {
			return fmt.Errorf("building form_field values state")
		}
		models = append(models, memberVerificationFieldModel{
			FieldType: types.StringValue(f.FieldType),
			Label:     types.StringValue(f.Label),
			Values:    values,
			Required:  types.BoolValue(f.Required),
		})
	}
	list, d := types.ListValueFrom(ctx, fieldType, models)
	if d.HasError() {
		return fmt.Errorf("building form_fields state: %v", d.Errors())
	}
	m.FormFields = list
	return nil
}
