package provider

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/kirchDev/terraform-provider-discord/internal/client"
)

// --- List data source: every custom emoji in a guild. Input is the guild id;
// output is a computed list of emoji objects built via types.ListValueFrom. ---

var (
	_ datasource.DataSource              = (*emojisDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*emojisDataSource)(nil)
)

// NewEmojisDataSource returns a new discord_emojis data source.
func NewEmojisDataSource() datasource.DataSource {
	return &emojisDataSource{}
}

type emojisDataSource struct {
	client *client.Client
}

// emojiListModel is one entry of the emojis list in tfsdk form.
type emojiListModel struct {
	ID        types.String `tfsdk:"id"`
	Name      types.String `tfsdk:"name"`
	Animated  types.Bool   `tfsdk:"animated"`
	Available types.Bool   `tfsdk:"available"`
	Managed   types.Bool   `tfsdk:"managed"`
}

var emojiListAttrTypes = map[string]attr.Type{
	"id":        types.StringType,
	"name":      types.StringType,
	"animated":  types.BoolType,
	"available": types.BoolType,
	"managed":   types.BoolType,
}

// emojiListWire is one emoji element as returned by the API.
type emojiListWire struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Animated  bool   `json:"animated"`
	Available bool   `json:"available"`
	Managed   bool   `json:"managed"`
}

type emojisDataSourceModel struct {
	ServerID types.String `tfsdk:"server_id"`
	Emojis   types.List   `tfsdk:"emojis"`
}

func (d *emojisDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_emojis"
}

func (d *emojisDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Fetches all custom emojis of a Discord guild (server).",
		Attributes: map[string]schema.Attribute{
			"server_id": schema.StringAttribute{MarkdownDescription: "Snowflake ID of the guild.", Required: true},
			"emojis": schema.ListNestedAttribute{
				MarkdownDescription: "Custom emojis of the guild.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":        schema.StringAttribute{MarkdownDescription: "Snowflake ID of the emoji.", Computed: true},
						"name":      schema.StringAttribute{MarkdownDescription: "Emoji name.", Computed: true},
						"animated":  schema.BoolAttribute{MarkdownDescription: "Whether the emoji is animated.", Computed: true},
						"available": schema.BoolAttribute{MarkdownDescription: "Whether the emoji is available for use.", Computed: true},
						"managed":   schema.BoolAttribute{MarkdownDescription: "Whether the emoji is managed by an integration.", Computed: true},
					},
				},
			},
		},
	}
}

func (d *emojisDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected Data Source Configure Type", fmt.Sprintf("Expected *client.Client, got: %T.", req.ProviderData))
		return
	}
	d.client = c
}

func (d *emojisDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data emojisDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	raw, err := d.client.List(ctx, "/guilds/"+data.ServerID.ValueString()+"/emojis")
	if err != nil {
		resp.Diagnostics.AddError("Unable to read Discord emojis", err.Error())
		return
	}

	models := make([]emojiListModel, 0, len(raw))
	for _, elem := range raw {
		var w emojiListWire
		if err := json.Unmarshal(elem, &w); err != nil {
			resp.Diagnostics.AddError("Unable to decode Discord emoji", err.Error())
			return
		}
		models = append(models, emojiListModel{
			ID:        types.StringValue(w.ID),
			Name:      types.StringValue(w.Name),
			Animated:  types.BoolValue(w.Animated),
			Available: types.BoolValue(w.Available),
			Managed:   types.BoolValue(w.Managed),
		})
	}

	list, diags := types.ListValueFrom(ctx, types.ObjectType{AttrTypes: emojiListAttrTypes}, models)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	data.Emojis = list

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
