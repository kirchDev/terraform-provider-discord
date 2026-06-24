package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/kirchDev/terraform-provider-discord/internal/client"
)

// --- Reads the bot's own application (Get Current Application). Mainly to obtain
// the application id without hardcoding it, e.g. for discord_application_command. ---

var (
	_ datasource.DataSource              = (*applicationDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*applicationDataSource)(nil)
)

// NewApplicationDataSource returns a new discord_application data source.
func NewApplicationDataSource() datasource.DataSource {
	return &applicationDataSource{}
}

type applicationDataSource struct {
	client *client.Client
}

type applicationDataSourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	BotID       types.String `tfsdk:"bot_id"`
}

func (d *applicationDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_application"
}

func (d *applicationDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads the bot's own Discord application. Use its `id` for `discord_application_command` " +
			"instead of hardcoding the application id.",
		Attributes: map[string]schema.Attribute{
			"id":          schema.StringAttribute{MarkdownDescription: "Snowflake ID of the application.", Computed: true},
			"name":        schema.StringAttribute{MarkdownDescription: "Application name.", Computed: true},
			"description": schema.StringAttribute{MarkdownDescription: "Application description.", Computed: true},
			"bot_id":      schema.StringAttribute{MarkdownDescription: "Snowflake ID of the application's bot user.", Computed: true},
		},
	}
}

func (d *applicationDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *applicationDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data applicationDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var a struct {
		ID          string  `json:"id"`
		Name        string  `json:"name"`
		Description *string `json:"description"`
		Bot         *struct {
			ID string `json:"id"`
		} `json:"bot"`
	}
	if err := d.client.Get(ctx, "/applications/@me", &a); err != nil {
		resp.Diagnostics.AddError("Unable to read Discord application", err.Error())
		return
	}

	data.ID = types.StringValue(a.ID)
	data.Name = types.StringValue(a.Name)
	data.Description = types.StringPointerValue(a.Description)
	if a.Bot != nil {
		data.BotID = types.StringValue(a.Bot.ID)
	} else {
		data.BotID = types.StringNull()
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
