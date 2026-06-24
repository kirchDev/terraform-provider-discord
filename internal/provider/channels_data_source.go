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

// --- List data source: every channel in a guild. Input is the guild id; output
// is a computed list of channel objects built via types.ListValueFrom. ---

var (
	_ datasource.DataSource              = (*channelsDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*channelsDataSource)(nil)
)

// NewChannelsDataSource returns a new discord_channels data source.
func NewChannelsDataSource() datasource.DataSource {
	return &channelsDataSource{}
}

type channelsDataSource struct {
	client *client.Client
}

// channelListModel is one entry of the channels list in tfsdk form.
type channelListModel struct {
	ID       types.String `tfsdk:"id"`
	Name     types.String `tfsdk:"name"`
	Topic    types.String `tfsdk:"topic"`
	ParentID types.String `tfsdk:"parent_id"`
	Type     types.Int64  `tfsdk:"type"`
	Position types.Int64  `tfsdk:"position"`
	NSFW     types.Bool   `tfsdk:"nsfw"`
}

var channelListAttrTypes = map[string]attr.Type{
	"id":        types.StringType,
	"name":      types.StringType,
	"topic":     types.StringType,
	"parent_id": types.StringType,
	"type":      types.Int64Type,
	"position":  types.Int64Type,
	"nsfw":      types.BoolType,
}

// channelListWire is one channel element as returned by the API. name, topic and
// parent_id are nullable, so they are pointers.
type channelListWire struct {
	ID       string  `json:"id"`
	Name     *string `json:"name"`
	Topic    *string `json:"topic"`
	ParentID *string `json:"parent_id"`
	Type     int64   `json:"type"`
	Position int64   `json:"position"`
	NSFW     bool    `json:"nsfw"`
}

type channelsDataSourceModel struct {
	ServerID types.String `tfsdk:"server_id"`
	Channels types.List   `tfsdk:"channels"`
}

func (d *channelsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_channels"
}

func (d *channelsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Fetches all channels of a Discord guild (server).",
		Attributes: map[string]schema.Attribute{
			"server_id": schema.StringAttribute{MarkdownDescription: "Snowflake ID of the guild.", Required: true},
			"channels": schema.ListNestedAttribute{
				MarkdownDescription: "Channels of the guild.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":        schema.StringAttribute{MarkdownDescription: "Snowflake ID of the channel.", Computed: true},
						"name":      schema.StringAttribute{MarkdownDescription: "Channel name.", Computed: true},
						"topic":     schema.StringAttribute{MarkdownDescription: "Channel topic.", Computed: true},
						"parent_id": schema.StringAttribute{MarkdownDescription: "Snowflake ID of the parent category.", Computed: true},
						"type":      schema.Int64Attribute{MarkdownDescription: "Channel type.", Computed: true},
						"position":  schema.Int64Attribute{MarkdownDescription: "Sorting position of the channel.", Computed: true},
						"nsfw":      schema.BoolAttribute{MarkdownDescription: "Whether the channel is age-restricted.", Computed: true},
					},
				},
			},
		},
	}
}

func (d *channelsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *channelsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data channelsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	raw, err := d.client.List(ctx, "/guilds/"+data.ServerID.ValueString()+"/channels")
	if err != nil {
		resp.Diagnostics.AddError("Unable to read Discord channels", err.Error())
		return
	}

	models := make([]channelListModel, 0, len(raw))
	for _, elem := range raw {
		var w channelListWire
		if err := json.Unmarshal(elem, &w); err != nil {
			resp.Diagnostics.AddError("Unable to decode Discord channel", err.Error())
			return
		}
		models = append(models, channelListModel{
			ID:       types.StringValue(w.ID),
			Name:     types.StringPointerValue(w.Name),
			Topic:    types.StringPointerValue(w.Topic),
			ParentID: types.StringPointerValue(w.ParentID),
			Type:     types.Int64Value(w.Type),
			Position: types.Int64Value(w.Position),
			NSFW:     types.BoolValue(w.NSFW),
		})
	}

	list, diags := types.ListValueFrom(ctx, types.ObjectType{AttrTypes: channelListAttrTypes}, models)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	data.Channels = list

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
