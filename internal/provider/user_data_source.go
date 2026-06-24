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
	_ datasource.DataSource              = (*userDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*userDataSource)(nil)
)

// NewUserDataSource returns a new discord_user data source.
func NewUserDataSource() datasource.DataSource {
	return &userDataSource{}
}

type userDataSource struct {
	client *client.Client
}

type userDataSourceModel struct {
	UserID        types.String `tfsdk:"user_id"`
	Username      types.String `tfsdk:"username"`
	GlobalName    types.String `tfsdk:"global_name"`
	Discriminator types.String `tfsdk:"discriminator"`
	Bot           types.Bool   `tfsdk:"bot"`
	AvatarHash    types.String `tfsdk:"avatar_hash"`
}

func (d *userDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_user"
}

func (d *userDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Fetches a single Discord user by id.",
		Attributes: map[string]schema.Attribute{
			"user_id":       schema.StringAttribute{MarkdownDescription: "Snowflake ID of the user.", Required: true},
			"username":      schema.StringAttribute{MarkdownDescription: "Account username.", Computed: true},
			"global_name":   schema.StringAttribute{MarkdownDescription: "Account display (global) name.", Computed: true},
			"discriminator": schema.StringAttribute{MarkdownDescription: "Legacy four-digit discriminator.", Computed: true},
			"bot":           schema.BoolAttribute{MarkdownDescription: "Whether the user is a bot.", Computed: true},
			"avatar_hash":   schema.StringAttribute{MarkdownDescription: "Avatar hash.", Computed: true},
		},
	}
}

func (d *userDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *userDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data userDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var u struct {
		Username      string  `json:"username"`
		GlobalName    *string `json:"global_name"`
		Discriminator string  `json:"discriminator"`
		Bot           bool    `json:"bot"`
		Avatar        *string `json:"avatar"`
	}
	if err := d.client.Get(ctx, "/users/"+data.UserID.ValueString(), &u); err != nil {
		resp.Diagnostics.AddError("Unable to read Discord user", err.Error())
		return
	}

	data.Username = types.StringValue(u.Username)
	data.GlobalName = types.StringPointerValue(u.GlobalName)
	data.Discriminator = types.StringValue(u.Discriminator)
	data.Bot = types.BoolValue(u.Bot)
	data.AvatarHash = types.StringPointerValue(u.Avatar)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
