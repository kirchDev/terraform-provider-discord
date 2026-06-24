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
	_ datasource.DataSource              = (*applicationCommandDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*applicationCommandDataSource)(nil)
)

// NewApplicationCommandDataSource returns a new discord_application_command data source.
func NewApplicationCommandDataSource() datasource.DataSource {
	return &applicationCommandDataSource{}
}

type applicationCommandDataSource struct {
	client *client.Client
}

type applicationCommandDataSourceModel struct {
	ApplicationID types.String `tfsdk:"application_id"`
	GuildID       types.String `tfsdk:"guild_id"`
	CommandID     types.String `tfsdk:"command_id"`
	Name          types.String `tfsdk:"name"`
	Description   types.String `tfsdk:"description"`
	Type          types.Int64  `tfsdk:"type"`
}

func (d *applicationCommandDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_application_command"
}

func (d *applicationCommandDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Fetches a single application (slash) command by id. With `guild_id` set the guild-scoped command is read; otherwise the global command is read.",
		Attributes: map[string]schema.Attribute{
			"application_id": schema.StringAttribute{MarkdownDescription: "Snowflake ID of the application that owns the command.", Required: true},
			"guild_id":       schema.StringAttribute{MarkdownDescription: "Snowflake ID of the guild for a guild-scoped command. Leave null for a global command.", Optional: true},
			"command_id":     schema.StringAttribute{MarkdownDescription: "Snowflake ID of the command.", Required: true},
			"name":           schema.StringAttribute{MarkdownDescription: "Command name.", Computed: true},
			"description":    schema.StringAttribute{MarkdownDescription: "Command description.", Computed: true},
			"type":           schema.Int64Attribute{MarkdownDescription: "Command type.", Computed: true},
		},
	}
}

func (d *applicationCommandDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *applicationCommandDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data applicationCommandDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var path string
	if g := data.GuildID; !g.IsNull() && !g.IsUnknown() && g.ValueString() != "" {
		path = "/applications/" + data.ApplicationID.ValueString() + "/guilds/" + g.ValueString() + "/commands/" + data.CommandID.ValueString()
	} else {
		path = "/applications/" + data.ApplicationID.ValueString() + "/commands/" + data.CommandID.ValueString()
	}

	var a struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Type        int64  `json:"type"`
	}
	if err := d.client.Get(ctx, path, &a); err != nil {
		resp.Diagnostics.AddError("Unable to read Discord application command", err.Error())
		return
	}

	data.Name = types.StringValue(a.Name)
	data.Description = types.StringValue(a.Description)
	data.Type = types.Int64Value(a.Type)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
