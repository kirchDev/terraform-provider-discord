package provider

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/kirchDev/terraform-provider-discord/internal/client"
)

// --- List data source: every role in a guild. Input is the guild id; output is
// a computed list of role objects built via types.ListValueFrom (see
// forum_channel_resource.go for the element-model + attr.Type pattern). ---

var (
	_ datasource.DataSource              = (*rolesDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*rolesDataSource)(nil)
)

// NewRolesDataSource returns a new discord_roles data source.
func NewRolesDataSource() datasource.DataSource {
	return &rolesDataSource{}
}

type rolesDataSource struct {
	client *client.Client
}

// roleListModel is one entry of the roles list in tfsdk form.
type roleListModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Permissions types.String `tfsdk:"permissions"`
	Color       types.Int64  `tfsdk:"color"`
	Position    types.Int64  `tfsdk:"position"`
	Hoist       types.Bool   `tfsdk:"hoist"`
	Mentionable types.Bool   `tfsdk:"mentionable"`
	Managed     types.Bool   `tfsdk:"managed"`
}

var roleListAttrTypes = map[string]attr.Type{
	"id":          types.StringType,
	"name":        types.StringType,
	"permissions": types.StringType,
	"color":       types.Int64Type,
	"position":    types.Int64Type,
	"hoist":       types.BoolType,
	"mentionable": types.BoolType,
	"managed":     types.BoolType,
}

// roleListWire is one role element as returned by the API.
type roleListWire struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Permissions string `json:"permissions"`
	Color       int64  `json:"color"`
	Position    int64  `json:"position"`
	Hoist       bool   `json:"hoist"`
	Mentionable bool   `json:"mentionable"`
	Managed     bool   `json:"managed"`
}

type rolesDataSourceModel struct {
	ServerID types.String `tfsdk:"server_id"`
	Roles    types.List   `tfsdk:"roles"`
}

func (d *rolesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_roles"
}

func (d *rolesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Fetches all roles of a Discord guild (server).",
		Attributes: map[string]schema.Attribute{
			"server_id": schema.StringAttribute{MarkdownDescription: "Snowflake ID of the guild.", Required: true},
			"roles": schema.ListNestedAttribute{
				MarkdownDescription: "Roles of the guild.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":          schema.StringAttribute{MarkdownDescription: "Snowflake ID of the role.", Computed: true},
						"name":        schema.StringAttribute{MarkdownDescription: "Role name.", Computed: true},
						"permissions": schema.StringAttribute{MarkdownDescription: "Permission bit set as a string.", Computed: true},
						"color":       schema.Int64Attribute{MarkdownDescription: "Integer representation of the role color.", Computed: true},
						"position":    schema.Int64Attribute{MarkdownDescription: "Sorting position of the role.", Computed: true},
						"hoist":       schema.BoolAttribute{MarkdownDescription: "Whether the role is displayed separately in the member list.", Computed: true},
						"mentionable": schema.BoolAttribute{MarkdownDescription: "Whether the role can be mentioned.", Computed: true},
						"managed":     schema.BoolAttribute{MarkdownDescription: "Whether the role is managed by an integration.", Computed: true},
					},
				},
			},
		},
	}
}

func (d *rolesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *rolesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data rolesDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	raw, err := d.client.List(ctx, "/guilds/"+data.ServerID.ValueString()+"/roles")
	if err != nil {
		resp.Diagnostics.AddError("Unable to read Discord roles", err.Error())
		return
	}

	models := make([]roleListModel, 0, len(raw))
	for _, elem := range raw {
		var w roleListWire
		if err := json.Unmarshal(elem, &w); err != nil {
			resp.Diagnostics.AddError("Unable to decode Discord role", err.Error())
			return
		}
		models = append(models, roleListModel{
			ID:          types.StringValue(w.ID),
			Name:        types.StringValue(w.Name),
			Permissions: types.StringValue(w.Permissions),
			Color:       types.Int64Value(w.Color),
			Position:    types.Int64Value(w.Position),
			Hoist:       types.BoolValue(w.Hoist),
			Mentionable: types.BoolValue(w.Mentionable),
			Managed:     types.BoolValue(w.Managed),
		})
	}

	list, diags := types.ListValueFrom(ctx, types.ObjectType{AttrTypes: roleListAttrTypes}, models)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	data.Roles = list

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
