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
	_ datasource.DataSource              = (*webhookDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*webhookDataSource)(nil)
)

// NewWebhookDataSource returns a new discord_webhook data source.
func NewWebhookDataSource() datasource.DataSource {
	return &webhookDataSource{}
}

type webhookDataSource struct {
	client *client.Client
}

type webhookDataSourceModel struct {
	WebhookID  types.String `tfsdk:"webhook_id"`
	Name       types.String `tfsdk:"name"`
	ChannelID  types.String `tfsdk:"channel_id"`
	GuildID    types.String `tfsdk:"guild_id"`
	Type       types.Int64  `tfsdk:"type"`
	AvatarHash types.String `tfsdk:"avatar_hash"`
	URL        types.String `tfsdk:"url"`
}

func (d *webhookDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_webhook"
}

func (d *webhookDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Fetches a single Discord webhook by id.",
		Attributes: map[string]schema.Attribute{
			"webhook_id":  schema.StringAttribute{MarkdownDescription: "Snowflake ID of the webhook.", Required: true},
			"name":        schema.StringAttribute{MarkdownDescription: "Webhook name.", Computed: true},
			"channel_id":  schema.StringAttribute{MarkdownDescription: "Snowflake ID of the channel the webhook posts to.", Computed: true},
			"guild_id":    schema.StringAttribute{MarkdownDescription: "Snowflake ID of the guild the webhook belongs to.", Computed: true},
			"type":        schema.Int64Attribute{MarkdownDescription: "Webhook type.", Computed: true},
			"avatar_hash": schema.StringAttribute{MarkdownDescription: "Webhook avatar hash.", Computed: true},
			"url":         schema.StringAttribute{MarkdownDescription: "Webhook execution URL (only available for incoming webhooks with a token).", Computed: true},
		},
	}
}

func (d *webhookDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *webhookDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data webhookDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var a struct {
		Name      *string `json:"name"`
		ChannelID *string `json:"channel_id"`
		GuildID   *string `json:"guild_id"`
		Type      int64   `json:"type"`
		Avatar    *string `json:"avatar"`
		Token     string  `json:"token"`
	}
	if err := d.client.Get(ctx, "/webhooks/"+data.WebhookID.ValueString(), &a); err != nil {
		resp.Diagnostics.AddError("Unable to read Discord webhook", err.Error())
		return
	}

	data.Name = types.StringPointerValue(a.Name)
	data.ChannelID = types.StringPointerValue(a.ChannelID)
	data.GuildID = types.StringPointerValue(a.GuildID)
	data.Type = types.Int64Value(a.Type)
	data.AvatarHash = types.StringPointerValue(a.Avatar)
	if a.Token != "" {
		data.URL = types.StringValue("https://discord.com/api/webhooks/" + data.WebhookID.ValueString() + "/" + a.Token)
	} else {
		data.URL = types.StringNull()
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
