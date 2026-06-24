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

// --- List data source: bans of a guild. Input is the guild id; output is a
// computed list of ban objects built via types.ListValueFrom. The underlying
// endpoint is paginated; this reads a single page of up to 1000 bans and does
// not paginate further. ---

var (
	_ datasource.DataSource              = (*bansDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*bansDataSource)(nil)
)

// NewBansDataSource returns a new discord_bans data source.
func NewBansDataSource() datasource.DataSource {
	return &bansDataSource{}
}

type bansDataSource struct {
	client *client.Client
}

// banListModel is one entry of the bans list in tfsdk form.
type banListModel struct {
	UserID types.String `tfsdk:"user_id"`
	Reason types.String `tfsdk:"reason"`
}

var banListAttrTypes = map[string]attr.Type{
	"user_id": types.StringType,
	"reason":  types.StringType,
}

// banListWire is one ban element as returned by the API. The user id is nested
// under .user.id and the reason is nullable.
type banListWire struct {
	Reason *string `json:"reason"`
	User   struct {
		ID string `json:"id"`
	} `json:"user"`
}

type bansDataSourceModel struct {
	ServerID types.String `tfsdk:"server_id"`
	Bans     types.List   `tfsdk:"bans"`
}

func (d *bansDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_bans"
}

func (d *bansDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Fetches the bans of a Discord guild (server). Returns a single page of up to 1000 bans; " +
			"it does not paginate further.",
		Attributes: map[string]schema.Attribute{
			"server_id": schema.StringAttribute{MarkdownDescription: "Snowflake ID of the guild.", Required: true},
			"bans": schema.ListNestedAttribute{
				MarkdownDescription: "Bans of the guild.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"user_id": schema.StringAttribute{MarkdownDescription: "Snowflake ID of the banned user.", Computed: true},
						"reason":  schema.StringAttribute{MarkdownDescription: "Reason the user was banned.", Computed: true},
					},
				},
			},
		},
	}
}

func (d *bansDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *bansDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data bansDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	raw, err := d.client.List(ctx, "/guilds/"+data.ServerID.ValueString()+"/bans?limit=1000")
	if err != nil {
		resp.Diagnostics.AddError("Unable to read Discord bans", err.Error())
		return
	}

	models := make([]banListModel, 0, len(raw))
	for _, elem := range raw {
		var w banListWire
		if err := json.Unmarshal(elem, &w); err != nil {
			resp.Diagnostics.AddError("Unable to decode Discord ban", err.Error())
			return
		}
		models = append(models, banListModel{
			UserID: types.StringValue(w.User.ID),
			Reason: types.StringPointerValue(w.Reason),
		})
	}

	list, diags := types.ListValueFrom(ctx, types.ObjectType{AttrTypes: banListAttrTypes}, models)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	data.Bans = list

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
