package provider

import (
	"context"
	"fmt"
	"math/big"
	"sort"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// --- The named-permission → bitfield helper. The key ergonomics feature: it
// lets config say `deny = ["view_channel", "connect"]` instead of magic decimal
// permission masks. Pure computation — no API call. ---

var (
	_ datasource.DataSource = (*permissionDataSource)(nil)
)

// NewPermissionDataSource returns a new discord_permission data source.
func NewPermissionDataSource() datasource.DataSource {
	return &permissionDataSource{}
}

type permissionDataSource struct{}

type permissionDataSourceModel struct {
	Allow     types.Set    `tfsdk:"allow"`
	Deny      types.Set    `tfsdk:"deny"`
	AllowBits types.String `tfsdk:"allow_bits"`
	DenyBits  types.String `tfsdk:"deny_bits"`
}

func (d *permissionDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_permission"
}

func (d *permissionDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	keys := permissionKeys()
	sorted := append([]string(nil), keys...)
	sort.Strings(sorted)
	keyList := "`" + strings.Join(sorted, "`, `") + "`"

	resp.Schema = schema.Schema{
		MarkdownDescription: "Computes Discord permission bitfields from named permission keys. " +
			"Set `allow` and/or `deny` to lists of permission keys and read back the decimal `allow_bits` / " +
			"`deny_bits` to feed into `discord_role.permissions`, `discord_role_everyone.permissions` or a " +
			"`discord_channel_permission` overwrite — no magic decimal masks.\n\n" +
			"Supported permission keys: " + keyList + ".",
		Attributes: map[string]schema.Attribute{
			"allow": schema.SetAttribute{
				MarkdownDescription: "Permission keys to set in the allow bitfield.",
				ElementType:         types.StringType,
				Optional:            true,
			},
			"deny": schema.SetAttribute{
				MarkdownDescription: "Permission keys to set in the deny bitfield.",
				ElementType:         types.StringType,
				Optional:            true,
			},
			"allow_bits": schema.StringAttribute{
				MarkdownDescription: "Decimal bitfield of the `allow` permission keys. `0` when `allow` is empty.",
				Computed:            true,
			},
			"deny_bits": schema.StringAttribute{
				MarkdownDescription: "Decimal bitfield of the `deny` permission keys. `0` when `deny` is empty.",
				Computed:            true,
			},
		},
	}
}

func (d *permissionDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data permissionDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	data.AllowBits = types.StringValue(bitsForSet(ctx, "allow", data.Allow, resp))
	data.DenyBits = types.StringValue(bitsForSet(ctx, "deny", data.Deny, resp))
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// bitsForSet ORs the bits of every key in the set and returns the decimal field.
// Unknown keys are reported as errors on the named attribute.
func bitsForSet(ctx context.Context, attr string, set types.Set, resp *datasource.ReadResponse) string {
	acc := big.NewInt(0)
	if set.IsNull() || set.IsUnknown() {
		return acc.String()
	}
	var keys []string
	resp.Diagnostics.Append(set.ElementsAs(ctx, &keys, false)...)
	for _, k := range keys {
		bit, ok := permissionBit(strings.ToLower(strings.TrimSpace(k)))
		if !ok {
			resp.Diagnostics.AddError(
				"Unknown permission key",
				fmt.Sprintf("%q is not a known Discord permission key in `%s`.", k, attr),
			)
			continue
		}
		acc.Or(acc, bit)
	}
	return acc.String()
}
