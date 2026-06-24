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
	_ datasource.DataSource              = (*emojiDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*emojiDataSource)(nil)
)

// NewEmojiDataSource returns a new discord_emoji data source.
func NewEmojiDataSource() datasource.DataSource {
	return &emojiDataSource{}
}

type emojiDataSource struct {
	client *client.Client
}

type emojiDataSourceModel struct {
	ServerID  types.String `tfsdk:"server_id"`
	EmojiID   types.String `tfsdk:"emoji_id"`
	Name      types.String `tfsdk:"name"`
	Animated  types.Bool   `tfsdk:"animated"`
	Available types.Bool   `tfsdk:"available"`
	Managed   types.Bool   `tfsdk:"managed"`
	Roles     types.Set    `tfsdk:"roles"`
}

func (d *emojiDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_emoji"
}

func (d *emojiDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Fetches a single custom emoji from a Discord guild by id.",
		Attributes: map[string]schema.Attribute{
			"server_id": schema.StringAttribute{MarkdownDescription: "Snowflake ID of the guild the emoji belongs to.", Required: true},
			"emoji_id":  schema.StringAttribute{MarkdownDescription: "Snowflake ID of the emoji.", Required: true},
			"name":      schema.StringAttribute{MarkdownDescription: "Emoji name.", Computed: true},
			"animated":  schema.BoolAttribute{MarkdownDescription: "Whether the emoji is animated.", Computed: true},
			"available": schema.BoolAttribute{MarkdownDescription: "Whether the emoji is available for use.", Computed: true},
			"managed":   schema.BoolAttribute{MarkdownDescription: "Whether the emoji is managed by an integration.", Computed: true},
			"roles": schema.SetAttribute{
				MarkdownDescription: "Snowflake IDs of the roles allowed to use the emoji.",
				Computed:            true,
				ElementType:         types.StringType,
			},
		},
	}
}

func (d *emojiDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *emojiDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data emojiDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var a struct {
		Name      string   `json:"name"`
		Animated  bool     `json:"animated"`
		Available bool     `json:"available"`
		Managed   bool     `json:"managed"`
		Roles     []string `json:"roles"`
	}
	if err := d.client.Get(ctx, "/guilds/"+data.ServerID.ValueString()+"/emojis/"+data.EmojiID.ValueString(), &a); err != nil {
		resp.Diagnostics.AddError("Unable to read Discord emoji", err.Error())
		return
	}

	data.Name = types.StringValue(a.Name)
	data.Animated = types.BoolValue(a.Animated)
	data.Available = types.BoolValue(a.Available)
	data.Managed = types.BoolValue(a.Managed)

	roles, diags := types.SetValueFrom(ctx, types.StringType, a.Roles)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	data.Roles = roles

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
