package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/kirchDev/terraform-provider-discord/internal/client"
)

var (
	_ datasource.DataSource              = (*channelDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*channelDataSource)(nil)
)

// NewChannelDataSource returns a new discord_channel data source.
func NewChannelDataSource() datasource.DataSource {
	return &channelDataSource{}
}

type channelDataSource struct {
	client *client.Client
}

type channelDataSourceModel struct {
	ChannelID types.String `tfsdk:"channel_id"`
	ServerID  types.String `tfsdk:"server_id"`
	Name      types.String `tfsdk:"name"`
	Type      types.Int64  `tfsdk:"type"`
	Topic     types.String `tfsdk:"topic"`
	NSFW      types.Bool   `tfsdk:"nsfw"`
	Position  types.Int64  `tfsdk:"position"`
	Category  types.String `tfsdk:"category"`
}

func (d *channelDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_channel"
}

func (d *channelDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Fetches a single Discord channel by id.",
		Attributes: map[string]schema.Attribute{
			"channel_id": schema.StringAttribute{MarkdownDescription: "Snowflake ID of the channel.", Required: true},
			"server_id":  schema.StringAttribute{MarkdownDescription: "Snowflake ID of the guild the channel belongs to.", Computed: true},
			"name":       schema.StringAttribute{MarkdownDescription: "Channel name.", Computed: true},
			"type":       schema.Int64Attribute{MarkdownDescription: "Channel type id.", Computed: true},
			"topic":      schema.StringAttribute{MarkdownDescription: "Channel topic.", Computed: true},
			"nsfw":       schema.BoolAttribute{MarkdownDescription: "Whether the channel is marked NSFW.", Computed: true},
			"position":   schema.Int64Attribute{MarkdownDescription: "Position of the channel.", Computed: true},
			"category":   schema.StringAttribute{MarkdownDescription: "Snowflake ID of the parent category, if any.", Computed: true},
		},
	}
}

func (d *channelDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *channelDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data channelDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	a, err := readChannel(ctx, d.client, data.ChannelID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read Discord channel", err.Error())
		return
	}

	data.ServerID = types.StringPointerValue(a.GuildID)
	data.Name = types.StringPointerValue(a.Name)
	data.Type = types.Int64Value(int64(a.Type))
	data.Topic = types.StringPointerValue(a.Topic)
	data.NSFW = types.BoolValue(a.NSFW)
	data.Position = types.Int64Value(a.Position)
	data.Category = types.StringPointerValue(a.ParentID)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
