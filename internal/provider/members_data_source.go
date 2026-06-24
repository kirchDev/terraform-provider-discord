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

// --- List data source: members of a guild. Input is the guild id; output is a
// computed list of member objects built via types.ListValueFrom. This requires
// the GUILD_MEMBERS privileged intent and reads a single page of up to 1000
// members; it does not paginate further. ---

var (
	_ datasource.DataSource              = (*membersDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*membersDataSource)(nil)
)

// NewMembersDataSource returns a new discord_members data source.
func NewMembersDataSource() datasource.DataSource {
	return &membersDataSource{}
}

type membersDataSource struct {
	client *client.Client
}

// memberListModel is one entry of the members list in tfsdk form.
type memberListModel struct {
	UserID types.String `tfsdk:"user_id"`
	Nick   types.String `tfsdk:"nick"`
	Roles  types.Set    `tfsdk:"roles"`
}

var memberListAttrTypes = map[string]attr.Type{
	"user_id": types.StringType,
	"nick":    types.StringType,
	"roles":   types.SetType{ElemType: types.StringType},
}

// memberListWire is one member element as returned by the API. The user id is
// nested under .user.id and the nickname is nullable.
type memberListWire struct {
	Nick  *string  `json:"nick"`
	Roles []string `json:"roles"`
	User  struct {
		ID string `json:"id"`
	} `json:"user"`
}

type membersDataSourceModel struct {
	ServerID types.String `tfsdk:"server_id"`
	Members  types.List   `tfsdk:"members"`
}

func (d *membersDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_members"
}

func (d *membersDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Fetches the members of a Discord guild (server). This requires the `GUILD_MEMBERS` " +
			"privileged intent to be enabled for the bot. Returns a single page of up to 1000 members; it does not " +
			"paginate further.",
		Attributes: map[string]schema.Attribute{
			"server_id": schema.StringAttribute{MarkdownDescription: "Snowflake ID of the guild.", Required: true},
			"members": schema.ListNestedAttribute{
				MarkdownDescription: "Members of the guild.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"user_id": schema.StringAttribute{MarkdownDescription: "Snowflake ID of the member's user.", Computed: true},
						"nick":    schema.StringAttribute{MarkdownDescription: "Guild-specific nickname of the member.", Computed: true},
						"roles": schema.SetAttribute{
							MarkdownDescription: "Snowflake IDs of the roles assigned to the member.",
							ElementType:         types.StringType,
							Computed:            true,
						},
					},
				},
			},
		},
	}
}

func (d *membersDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *membersDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data membersDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	raw, err := d.client.List(ctx, "/guilds/"+data.ServerID.ValueString()+"/members?limit=1000")
	if err != nil {
		resp.Diagnostics.AddError("Unable to read Discord members", err.Error())
		return
	}

	models := make([]memberListModel, 0, len(raw))
	for _, elem := range raw {
		var w memberListWire
		if err := json.Unmarshal(elem, &w); err != nil {
			resp.Diagnostics.AddError("Unable to decode Discord member", err.Error())
			return
		}
		roles, diags := types.SetValueFrom(ctx, types.StringType, w.Roles)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		models = append(models, memberListModel{
			UserID: types.StringValue(w.User.ID),
			Nick:   types.StringPointerValue(w.Nick),
			Roles:  roles,
		})
	}

	list, diags := types.ListValueFrom(ctx, types.ObjectType{AttrTypes: memberListAttrTypes}, models)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	data.Members = list

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
