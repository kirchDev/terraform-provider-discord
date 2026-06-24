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
	_ datasource.DataSource              = (*inviteDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*inviteDataSource)(nil)
)

// NewInviteDataSource returns a new discord_invite data source.
func NewInviteDataSource() datasource.DataSource {
	return &inviteDataSource{}
}

type inviteDataSource struct {
	client *client.Client
}

type inviteDataSourceModel struct {
	Code      types.String `tfsdk:"code"`
	ChannelID types.String `tfsdk:"channel_id"`
	GuildID   types.String `tfsdk:"guild_id"`
	Uses      types.Int64  `tfsdk:"uses"`
	MaxUses   types.Int64  `tfsdk:"max_uses"`
	MaxAge    types.Int64  `tfsdk:"max_age"`
	Temporary types.Bool   `tfsdk:"temporary"`
	URL       types.String `tfsdk:"url"`
}

func (d *inviteDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_invite"
}

func (d *inviteDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Fetches a single Discord invite by its code.",
		Attributes: map[string]schema.Attribute{
			"code":       schema.StringAttribute{MarkdownDescription: "Invite code.", Required: true},
			"channel_id": schema.StringAttribute{MarkdownDescription: "Snowflake ID of the channel the invite points to.", Computed: true},
			"guild_id":   schema.StringAttribute{MarkdownDescription: "Snowflake ID of the guild the invite belongs to.", Computed: true},
			"uses":       schema.Int64Attribute{MarkdownDescription: "Number of times the invite has been used.", Computed: true},
			"max_uses":   schema.Int64Attribute{MarkdownDescription: "Maximum number of uses (0 is unlimited).", Computed: true},
			"max_age":    schema.Int64Attribute{MarkdownDescription: "Duration in seconds after which the invite expires (0 never expires).", Computed: true},
			"temporary":  schema.BoolAttribute{MarkdownDescription: "Whether the invite grants temporary membership.", Computed: true},
			"url":        schema.StringAttribute{MarkdownDescription: "Full invite URL.", Computed: true},
		},
	}
}

func (d *inviteDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *inviteDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data inviteDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var inv struct {
		Code      string `json:"code"`
		Uses      int64  `json:"uses"`
		MaxUses   int64  `json:"max_uses"`
		MaxAge    int64  `json:"max_age"`
		Temporary bool   `json:"temporary"`
		Channel   *struct {
			ID string `json:"id"`
		} `json:"channel"`
		Guild *struct {
			ID string `json:"id"`
		} `json:"guild"`
	}
	if err := d.client.Get(ctx, "/invites/"+data.Code.ValueString()+"?with_counts=true", &inv); err != nil {
		resp.Diagnostics.AddError("Unable to read Discord invite", err.Error())
		return
	}

	if inv.Channel != nil {
		data.ChannelID = types.StringValue(inv.Channel.ID)
	} else {
		data.ChannelID = types.StringNull()
	}
	if inv.Guild != nil {
		data.GuildID = types.StringValue(inv.Guild.ID)
	} else {
		data.GuildID = types.StringNull()
	}
	data.Uses = types.Int64Value(inv.Uses)
	data.MaxUses = types.Int64Value(inv.MaxUses)
	data.MaxAge = types.Int64Value(inv.MaxAge)
	data.Temporary = types.BoolValue(inv.Temporary)
	data.URL = types.StringValue("https://discord.gg/" + data.Code.ValueString())

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
