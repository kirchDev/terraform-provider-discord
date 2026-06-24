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
	_ datasource.DataSource              = (*memberDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*memberDataSource)(nil)
)

// NewMemberDataSource returns a new discord_member data source.
func NewMemberDataSource() datasource.DataSource {
	return &memberDataSource{}
}

type memberDataSource struct {
	client *client.Client
}

type memberDataSourceModel struct {
	ServerID   types.String `tfsdk:"server_id"`
	UserID     types.String `tfsdk:"user_id"`
	Nick       types.String `tfsdk:"nick"`
	Roles      types.Set    `tfsdk:"roles"`
	Username   types.String `tfsdk:"username"`
	GlobalName types.String `tfsdk:"global_name"`
}

func (d *memberDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_member"
}

func (d *memberDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Fetches a single Discord guild member by user id.",
		Attributes: map[string]schema.Attribute{
			"server_id": schema.StringAttribute{MarkdownDescription: "Snowflake ID of the guild.", Required: true},
			"user_id":   schema.StringAttribute{MarkdownDescription: "Snowflake ID of the user.", Required: true},
			"nick":      schema.StringAttribute{MarkdownDescription: "Member nickname in the guild.", Computed: true},
			"roles": schema.SetAttribute{
				MarkdownDescription: "Snowflake IDs of the roles assigned to the member.",
				ElementType:         types.StringType,
				Computed:            true,
			},
			"username":    schema.StringAttribute{MarkdownDescription: "Account username.", Computed: true},
			"global_name": schema.StringAttribute{MarkdownDescription: "Account display (global) name.", Computed: true},
		},
	}
}

func (d *memberDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *memberDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data memberDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var a memberAttributes
	if err := d.client.Get(ctx, memberPath(data.ServerID.ValueString(), data.UserID.ValueString()), &a); err != nil {
		resp.Diagnostics.AddError("Unable to read Discord member", err.Error())
		return
	}

	roles, diags := types.SetValueFrom(ctx, types.StringType, a.Roles)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	data.Nick = types.StringPointerValue(a.Nick)
	data.Roles = roles
	data.Username = types.StringValue(a.User.Username)
	data.GlobalName = types.StringPointerValue(a.User.GlobalName)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
