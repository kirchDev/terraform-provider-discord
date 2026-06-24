package provider

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// --- Compute-only data-source exemplar (no API call). Converts a hex string or
// an RGB triple into the decimal integer Discord uses for role colors. ---

var _ datasource.DataSource = (*colorDataSource)(nil)

// NewColorDataSource returns a new discord_color data source.
func NewColorDataSource() datasource.DataSource {
	return &colorDataSource{}
}

type colorDataSource struct{}

type colorDataSourceModel struct {
	Hex types.String `tfsdk:"hex"`
	RGB types.List   `tfsdk:"rgb"`
	Dec types.Int64  `tfsdk:"dec"`
}

func (d *colorDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_color"
}

func (d *colorDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Converts a color into the decimal RGB integer Discord uses (e.g. for `discord_role.color`). " +
			"Provide exactly one of `hex` or `rgb`.",
		Attributes: map[string]schema.Attribute{
			"hex": schema.StringAttribute{
				MarkdownDescription: "Hex color string, e.g. `#5865F2` or `5865F2`.",
				Optional:            true,
			},
			"rgb": schema.ListAttribute{
				MarkdownDescription: "RGB triple as `[r, g, b]`, each 0–255.",
				ElementType:         types.Int64Type,
				Optional:            true,
			},
			"dec": schema.Int64Attribute{
				MarkdownDescription: "Decimal RGB integer.",
				Computed:            true,
			},
		},
	}
}

func (d *colorDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data colorDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	hexSet := !data.Hex.IsNull() && !data.Hex.IsUnknown()
	rgbSet := !data.RGB.IsNull() && !data.RGB.IsUnknown()
	switch {
	case hexSet == rgbSet:
		resp.Diagnostics.AddError("Invalid color input", "Provide exactly one of `hex` or `rgb`.")
		return
	case hexSet:
		dec, err := parseHexColor(data.Hex.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Invalid hex color", err.Error())
			return
		}
		data.Dec = types.Int64Value(dec)
	default:
		var rgb []int64
		resp.Diagnostics.Append(data.RGB.ElementsAs(ctx, &rgb, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if len(rgb) != 3 {
			resp.Diagnostics.AddError("Invalid rgb color", "`rgb` must have exactly 3 elements.")
			return
		}
		for _, c := range rgb {
			if c < 0 || c > 255 {
				resp.Diagnostics.AddError("Invalid rgb color", "each component must be 0–255.")
				return
			}
		}
		data.Dec = types.Int64Value(rgb[0]<<16 | rgb[1]<<8 | rgb[2])
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func parseHexColor(s string) (int64, error) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(s) != 6 {
		return 0, fmt.Errorf("expected 6 hex digits, got %q", s)
	}
	v, err := strconv.ParseInt(s, 16, 64)
	if err != nil {
		return 0, err
	}
	return v, nil
}
