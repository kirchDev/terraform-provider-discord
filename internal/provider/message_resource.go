package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/kirchDev/terraform-provider-discord/internal/client"
)

// --- A standalone message in a text/announcement channel, with rich embeds. Like
// discord_forum_post, this manages message *content* declaratively — for a fixed,
// pinned-style message such as a rules embed, not a chat stream. Create POSTs the
// message, Update PATCHes it (a bot can only edit its own messages, so author it
// with the same provider that should own it), Delete removes it. Pair it with an
// aliased provider to author the message as a specific bot. ---

var (
	_ resource.Resource                = (*messageResource)(nil)
	_ resource.ResourceWithConfigure   = (*messageResource)(nil)
	_ resource.ResourceWithImportState = (*messageResource)(nil)
)

// NewMessageResource returns a new discord_message resource.
func NewMessageResource() resource.Resource {
	return &messageResource{}
}

type messageResource struct {
	client *client.Client
}

// --- wire (Discord REST shapes) ---

type embedFieldWire struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline"`
}

type embedFooterWire struct {
	Text    string  `json:"text"`
	IconURL *string `json:"icon_url"`
}

type embedAuthorWire struct {
	Name    string  `json:"name"`
	URL     *string `json:"url"`
	IconURL *string `json:"icon_url"`
}

type embedImageWire struct {
	URL string `json:"url"`
}

type embedWire struct {
	Title       *string          `json:"title"`
	Description *string          `json:"description"`
	URL         *string          `json:"url"`
	Color       *int64           `json:"color"`
	Timestamp   *string          `json:"timestamp"`
	Footer      *embedFooterWire `json:"footer"`
	Author      *embedAuthorWire `json:"author"`
	Image       *embedImageWire  `json:"image"`
	Thumbnail   *embedImageWire  `json:"thumbnail"`
	Fields      []embedFieldWire `json:"fields"`
}

type messageWire struct {
	ID      string      `json:"id"`
	Content string      `json:"content"`
	Embeds  []embedWire `json:"embeds"`
}

// --- model (tfsdk) ---

type embedFieldModel struct {
	Name   types.String `tfsdk:"name"`
	Value  types.String `tfsdk:"value"`
	Inline types.Bool   `tfsdk:"inline"`
}

var embedFieldAttrTypes = map[string]attr.Type{
	"name":   types.StringType,
	"value":  types.StringType,
	"inline": types.BoolType,
}

type embedModel struct {
	Title         types.String `tfsdk:"title"`
	Description   types.String `tfsdk:"description"`
	URL           types.String `tfsdk:"url"`
	Color         types.Int64  `tfsdk:"color"`
	Timestamp     types.String `tfsdk:"timestamp"`
	FooterText    types.String `tfsdk:"footer_text"`
	FooterIconURL types.String `tfsdk:"footer_icon_url"`
	AuthorName    types.String `tfsdk:"author_name"`
	AuthorURL     types.String `tfsdk:"author_url"`
	AuthorIconURL types.String `tfsdk:"author_icon_url"`
	ImageURL      types.String `tfsdk:"image_url"`
	ThumbnailURL  types.String `tfsdk:"thumbnail_url"`
	Fields        types.List   `tfsdk:"fields"`
}

var embedAttrTypes = map[string]attr.Type{
	"title":           types.StringType,
	"description":     types.StringType,
	"url":             types.StringType,
	"color":           types.Int64Type,
	"timestamp":       types.StringType,
	"footer_text":     types.StringType,
	"footer_icon_url": types.StringType,
	"author_name":     types.StringType,
	"author_url":      types.StringType,
	"author_icon_url": types.StringType,
	"image_url":       types.StringType,
	"thumbnail_url":   types.StringType,
	"fields":          types.ListType{ElemType: types.ObjectType{AttrTypes: embedFieldAttrTypes}},
}

type messageResourceModel struct {
	ChannelID types.String `tfsdk:"channel_id"`
	ID        types.String `tfsdk:"id"`
	Content   types.String `tfsdk:"content"`
	Embeds    types.List   `tfsdk:"embeds"`
}

func (r *messageResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_message"
}

func (r *messageResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a standalone message (with rich embeds) in a text or announcement channel — for a " +
			"fixed, declarative message such as a pinned rules embed, not a chat stream. The message is authored by the " +
			"provider's bot (use an aliased provider to author it as a specific bot); a bot can only edit its own messages. " +
			"The bot needs `Read Message History` to refresh the content.",
		Attributes: map[string]schema.Attribute{
			"channel_id": schema.StringAttribute{
				MarkdownDescription: "Snowflake ID of the channel the message is posted in.",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "Snowflake ID of the message.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"content": schema.StringAttribute{
				MarkdownDescription: "Plain message body (markdown). Optional when `embeds` are set.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"embeds": schema.ListNestedAttribute{
				MarkdownDescription: "Rich embeds (max 10). Each renders as a card; the classic use is a single embed.",
				Optional:            true,
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"title":           schema.StringAttribute{MarkdownDescription: "Embed title.", Optional: true, Computed: true},
						"description":     schema.StringAttribute{MarkdownDescription: "Embed body (markdown).", Optional: true, Computed: true},
						"url":             schema.StringAttribute{MarkdownDescription: "URL the title links to.", Optional: true, Computed: true},
						"color":           schema.Int64Attribute{MarkdownDescription: "Side-bar color as a decimal integer.", Optional: true, Computed: true},
						"timestamp":       schema.StringAttribute{MarkdownDescription: "ISO 8601 timestamp shown in the footer (e.g. `2026-06-26T14:48:00+02:00`).", Optional: true, Computed: true},
						"footer_text":     schema.StringAttribute{MarkdownDescription: "Footer text.", Optional: true, Computed: true},
						"footer_icon_url": schema.StringAttribute{MarkdownDescription: "Footer icon URL.", Optional: true, Computed: true},
						"author_name":     schema.StringAttribute{MarkdownDescription: "Author name shown above the title.", Optional: true, Computed: true},
						"author_url":      schema.StringAttribute{MarkdownDescription: "URL the author name links to.", Optional: true, Computed: true},
						"author_icon_url": schema.StringAttribute{MarkdownDescription: "Author icon URL.", Optional: true, Computed: true},
						"image_url":       schema.StringAttribute{MarkdownDescription: "Large image URL.", Optional: true, Computed: true},
						"thumbnail_url":   schema.StringAttribute{MarkdownDescription: "Thumbnail image URL.", Optional: true, Computed: true},
						"fields": schema.ListNestedAttribute{
							MarkdownDescription: "Embed fields (name/value pairs, max 25).",
							Optional:            true,
							Computed:            true,
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"name":   schema.StringAttribute{MarkdownDescription: "Field name.", Required: true},
									"value":  schema.StringAttribute{MarkdownDescription: "Field value.", Required: true},
									"inline": schema.BoolAttribute{MarkdownDescription: "Whether the field renders inline.", Optional: true, Computed: true},
								},
							},
						},
					},
				},
			},
		},
	}
}

func (r *messageResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *messageResource) messagesPath(m *messageResourceModel) string {
	return "/channels/" + m.ChannelID.ValueString() + "/messages"
}

// body builds the create/edit payload (content + embeds).
func (r *messageResource) body(ctx context.Context, m *messageResourceModel) (map[string]any, error) {
	body := map[string]any{}
	if v := m.Content; !v.IsNull() && !v.IsUnknown() {
		body["content"] = v.ValueString()
	}
	if !m.Embeds.IsNull() && !m.Embeds.IsUnknown() {
		var embeds []embedModel
		if d := m.Embeds.ElementsAs(ctx, &embeds, false); d.HasError() {
			return nil, fmt.Errorf("reading embeds")
		}
		wire := make([]map[string]any, 0, len(embeds))
		for _, e := range embeds {
			em := map[string]any{}
			putStr(em, "title", e.Title)
			putStr(em, "description", e.Description)
			putStr(em, "url", e.URL)
			putStr(em, "timestamp", e.Timestamp)
			if v := e.Color; !v.IsNull() && !v.IsUnknown() {
				em["color"] = v.ValueInt64()
			}
			if footer := strPtrOrNil(e.FooterText); footer != nil {
				em["footer"] = map[string]any{"text": *footer, "icon_url": strPtrOrNil(e.FooterIconURL)}
			}
			if name := strPtrOrNil(e.AuthorName); name != nil {
				em["author"] = map[string]any{"name": *name, "url": strPtrOrNil(e.AuthorURL), "icon_url": strPtrOrNil(e.AuthorIconURL)}
			}
			if img := strPtrOrNil(e.ImageURL); img != nil {
				em["image"] = map[string]any{"url": *img}
			}
			if thumb := strPtrOrNil(e.ThumbnailURL); thumb != nil {
				em["thumbnail"] = map[string]any{"url": *thumb}
			}
			if !e.Fields.IsNull() && !e.Fields.IsUnknown() {
				var fields []embedFieldModel
				if d := e.Fields.ElementsAs(ctx, &fields, false); d.HasError() {
					return nil, fmt.Errorf("reading embed fields")
				}
				fout := make([]map[string]any, 0, len(fields))
				for _, f := range fields {
					inline := false
					if v := f.Inline; !v.IsNull() && !v.IsUnknown() {
						inline = v.ValueBool()
					}
					fout = append(fout, map[string]any{"name": f.Name.ValueString(), "value": f.Value.ValueString(), "inline": inline})
				}
				em["fields"] = fout
			}
			wire = append(wire, em)
		}
		body["embeds"] = wire
	}
	return body, nil
}

// putStr adds a tfsdk string to a body map when it is set (non-null, non-unknown).
func putStr(body map[string]any, key string, v types.String) {
	if !v.IsNull() && !v.IsUnknown() {
		body[key] = v.ValueString()
	}
}

func (r *messageResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan messageResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body, err := r.body(ctx, &plan)
	if err != nil {
		resp.Diagnostics.AddError("Invalid message", err.Error())
		return
	}
	var created messageWire
	if err := r.client.Write(ctx, "POST", r.messagesPath(&plan), body, &created); err != nil {
		resp.Diagnostics.AddError("Unable to create Discord message", err.Error())
		return
	}
	plan.ID = types.StringValue(created.ID)
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Unable to read message after create", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *messageResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state messageResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.readInto(ctx, &state); err != nil {
		if notFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read Discord message", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *messageResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan messageResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body, err := r.body(ctx, &plan)
	if err != nil {
		resp.Diagnostics.AddError("Invalid message", err.Error())
		return
	}
	if err := r.client.Write(ctx, "PATCH", r.messagesPath(&plan)+"/"+plan.ID.ValueString(), body, nil); err != nil {
		resp.Diagnostics.AddError("Unable to update Discord message", err.Error())
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Unable to read message after update", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *messageResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state messageResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.Delete(ctx, r.messagesPath(&state)+"/"+state.ID.ValueString()); err != nil && !notFound(err) {
		resp.Diagnostics.AddError("Unable to delete Discord message", err.Error())
	}
}

// ImportState accepts "channel_id/message_id".
func (r *messageResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError("Invalid import ID", "Expected \"channel_id/message_id\".")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("channel_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[1])...)
}

// readInto refreshes content + embeds. Timestamps are reconciled with the prior
// value (Discord reformats them) so an unchanged embed doesn't show drift.
func (r *messageResource) readInto(ctx context.Context, m *messageResourceModel) error {
	var a messageWire
	if err := r.client.Get(ctx, r.messagesPath(m)+"/"+m.ID.ValueString(), &a); err != nil {
		return err
	}
	m.Content = nullIfEmpty(&a.Content)

	// Prior embeds (by index) so timestamps can be kept across the API's reformat.
	var prior []embedModel
	if !m.Embeds.IsNull() && !m.Embeds.IsUnknown() {
		_ = m.Embeds.ElementsAs(ctx, &prior, false)
	}

	embedType := types.ObjectType{AttrTypes: embedAttrTypes}
	models := make([]embedModel, 0, len(a.Embeds))
	for i, ew := range a.Embeds {
		em := embedModel{
			Title:       nullIfEmpty(ew.Title),
			Description: nullIfEmpty(ew.Description),
			URL:         nullIfEmpty(ew.URL),
			Color:       types.Int64Null(),
		}
		if ew.Color != nil {
			em.Color = types.Int64Value(*ew.Color)
		}
		var priorTS types.String
		if i < len(prior) {
			priorTS = prior[i].Timestamp
		}
		em.Timestamp = keepTimestamp(priorTS, derefStr(ew.Timestamp))
		if ew.Footer != nil {
			em.FooterText = nullIfEmpty(&ew.Footer.Text)
			em.FooterIconURL = nullIfEmpty(ew.Footer.IconURL)
		} else {
			em.FooterText, em.FooterIconURL = types.StringNull(), types.StringNull()
		}
		if ew.Author != nil {
			em.AuthorName = nullIfEmpty(&ew.Author.Name)
			em.AuthorURL = nullIfEmpty(ew.Author.URL)
			em.AuthorIconURL = nullIfEmpty(ew.Author.IconURL)
		} else {
			em.AuthorName, em.AuthorURL, em.AuthorIconURL = types.StringNull(), types.StringNull(), types.StringNull()
		}
		em.ImageURL = types.StringNull()
		if ew.Image != nil {
			em.ImageURL = nullIfEmpty(&ew.Image.URL)
		}
		em.ThumbnailURL = types.StringNull()
		if ew.Thumbnail != nil {
			em.ThumbnailURL = nullIfEmpty(&ew.Thumbnail.URL)
		}
		fields, err := embedFieldsToState(ctx, ew.Fields)
		if err != nil {
			return err
		}
		em.Fields = fields
		models = append(models, em)
	}
	list, d := types.ListValueFrom(ctx, embedType, models)
	if d.HasError() {
		return fmt.Errorf("building embeds state: %v", d.Errors())
	}
	m.Embeds = list
	return nil
}

func embedFieldsToState(ctx context.Context, wire []embedFieldWire) (types.List, error) {
	fieldType := types.ObjectType{AttrTypes: embedFieldAttrTypes}
	models := make([]embedFieldModel, 0, len(wire))
	for _, f := range wire {
		models = append(models, embedFieldModel{
			Name:   types.StringValue(f.Name),
			Value:  types.StringValue(f.Value),
			Inline: types.BoolValue(f.Inline),
		})
	}
	list, d := types.ListValueFrom(ctx, fieldType, models)
	if d.HasError() {
		return types.ListNull(fieldType), fmt.Errorf("building embed fields state")
	}
	return list, nil
}

// derefStr returns the pointed-to string, or "" for a nil pointer.
func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
