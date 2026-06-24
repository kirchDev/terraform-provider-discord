package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/kirchDev/terraform-provider-discord/internal/client"
)

// --- API-read data-source exemplar. Fetches a single Discord guild by id. New
// data sources follow this shape: an *Attributes struct, a *DataSourceModel,
// Configure (grabs *client.Client), and a Read that GETs and maps. ---

var (
	_ datasource.DataSource              = (*serverDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*serverDataSource)(nil)
)

// NewServerDataSource returns a new discord_server data source.
func NewServerDataSource() datasource.DataSource {
	return &serverDataSource{}
}

type serverDataSource struct {
	client *client.Client
}

type serverDataSourceModel struct {
	ServerID                  types.String `tfsdk:"server_id"`
	Name                      types.String `tfsdk:"name"`
	Description               types.String `tfsdk:"description"`
	OwnerID                   types.String `tfsdk:"owner_id"`
	AfkChannelID              types.String `tfsdk:"afk_channel_id"`
	AfkTimeout                types.Int64  `tfsdk:"afk_timeout"`
	VerificationLevel         types.Int64  `tfsdk:"verification_level"`
	IconHash                  types.String `tfsdk:"icon_hash"`
	SplashHash                types.String `tfsdk:"splash_hash"`
	SystemChannelID           types.String `tfsdk:"system_channel_id"`
	RulesChannelID            types.String `tfsdk:"rules_channel_id"`
	PreferredLocale           types.String `tfsdk:"preferred_locale"`
	PremiumProgressBarEnabled types.Bool   `tfsdk:"premium_progress_bar_enabled"`
}

func (d *serverDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_server"
}

func (d *serverDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Fetches a single Discord guild (server) by id.",
		Attributes: map[string]schema.Attribute{
			"server_id":                    schema.StringAttribute{MarkdownDescription: "Snowflake ID of the guild.", Required: true},
			"name":                         schema.StringAttribute{MarkdownDescription: "Guild name.", Computed: true},
			"description":                  schema.StringAttribute{MarkdownDescription: "Guild description.", Computed: true},
			"owner_id":                     schema.StringAttribute{MarkdownDescription: "Snowflake ID of the owner.", Computed: true},
			"afk_channel_id":               schema.StringAttribute{MarkdownDescription: "Snowflake ID of the AFK channel.", Computed: true},
			"afk_timeout":                  schema.Int64Attribute{MarkdownDescription: "AFK timeout in seconds.", Computed: true},
			"verification_level":           schema.Int64Attribute{MarkdownDescription: "Verification level.", Computed: true},
			"icon_hash":                    schema.StringAttribute{MarkdownDescription: "Guild icon hash.", Computed: true},
			"splash_hash":                  schema.StringAttribute{MarkdownDescription: "Invite splash hash.", Computed: true},
			"system_channel_id":            schema.StringAttribute{MarkdownDescription: "Snowflake ID of the system channel.", Computed: true},
			"rules_channel_id":             schema.StringAttribute{MarkdownDescription: "Snowflake ID of the rules channel.", Computed: true},
			"preferred_locale":             schema.StringAttribute{MarkdownDescription: "Preferred locale.", Computed: true},
			"premium_progress_bar_enabled": schema.BoolAttribute{MarkdownDescription: "Whether the boost progress bar is enabled.", Computed: true},
		},
	}
}

func (d *serverDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *serverDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data serverDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var a guildAttributes
	if err := d.client.Get(ctx, "/guilds/"+data.ServerID.ValueString(), &a); err != nil {
		resp.Diagnostics.AddError("Unable to read Discord guild", err.Error())
		return
	}

	data.Name = types.StringValue(a.Name)
	data.Description = types.StringPointerValue(a.Description)
	data.OwnerID = types.StringValue(a.OwnerID)
	data.AfkChannelID = types.StringPointerValue(a.AfkChannelID)
	data.AfkTimeout = types.Int64Value(a.AfkTimeout)
	data.VerificationLevel = types.Int64Value(a.VerificationLevel)
	data.IconHash = types.StringPointerValue(a.Icon)
	data.SplashHash = types.StringPointerValue(a.Splash)
	data.SystemChannelID = types.StringPointerValue(a.SystemChannelID)
	data.RulesChannelID = types.StringPointerValue(a.RulesChannelID)
	data.PreferredLocale = types.StringValue(a.PreferredLocale)
	data.PremiumProgressBarEnabled = types.BoolValue(a.PremiumProgressBarEnabled)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
