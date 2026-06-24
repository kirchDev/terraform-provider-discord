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
	_ datasource.DataSource              = (*roleDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*roleDataSource)(nil)
)

// NewRoleDataSource returns a new discord_role data source.
func NewRoleDataSource() datasource.DataSource {
	return &roleDataSource{}
}

type roleDataSource struct {
	client *client.Client
}

type roleDataSourceModel struct {
	ServerID    types.String `tfsdk:"server_id"`
	RoleID      types.String `tfsdk:"role_id"`
	Name        types.String `tfsdk:"name"`
	Color       types.Int64  `tfsdk:"color"`
	Hoist       types.Bool   `tfsdk:"hoist"`
	Position    types.Int64  `tfsdk:"position"`
	Permissions types.String `tfsdk:"permissions"`
	Managed     types.Bool   `tfsdk:"managed"`
	Mentionable types.Bool   `tfsdk:"mentionable"`
}

func (d *roleDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_role"
}

func (d *roleDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Fetches a single Discord role within a guild by id.",
		Attributes: map[string]schema.Attribute{
			"server_id":   schema.StringAttribute{MarkdownDescription: "Snowflake ID of the guild.", Required: true},
			"role_id":     schema.StringAttribute{MarkdownDescription: "Snowflake ID of the role.", Required: true},
			"name":        schema.StringAttribute{MarkdownDescription: "Role name.", Computed: true},
			"color":       schema.Int64Attribute{MarkdownDescription: "Decimal RGB color of the role.", Computed: true},
			"hoist":       schema.BoolAttribute{MarkdownDescription: "Whether the role is displayed separately in the member list.", Computed: true},
			"position":    schema.Int64Attribute{MarkdownDescription: "Position of the role in the hierarchy.", Computed: true},
			"permissions": schema.StringAttribute{MarkdownDescription: "Permission bitfield as a decimal string.", Computed: true},
			"managed":     schema.BoolAttribute{MarkdownDescription: "Whether the role is managed by an integration.", Computed: true},
			"mentionable": schema.BoolAttribute{MarkdownDescription: "Whether the role is mentionable.", Computed: true},
		},
	}
}

func (d *roleDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *roleDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data roleDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	a, err := findInList(ctx, d.client, "/guilds/"+data.ServerID.ValueString()+"/roles", data.RoleID.ValueString(), func(a *roleAttributes) string { return a.ID })
	if err != nil {
		if notFound(err) {
			resp.Diagnostics.AddError("Discord role not found", fmt.Sprintf("No role with id %q was found in guild %q.", data.RoleID.ValueString(), data.ServerID.ValueString()))
			return
		}
		resp.Diagnostics.AddError("Unable to read Discord role", err.Error())
		return
	}

	data.Name = types.StringValue(a.Name)
	data.Color = types.Int64Value(a.Color)
	data.Hoist = types.BoolValue(a.Hoist)
	data.Position = types.Int64Value(a.Position)
	data.Permissions = types.StringValue(a.Permissions)
	data.Managed = types.BoolValue(a.Managed)
	data.Mentionable = types.BoolValue(a.Mentionable)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
