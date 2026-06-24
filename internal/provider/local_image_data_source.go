package provider

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// --- Compute-only data-source (no API call). Reads a local image file and emits
// the base64 data URI the icon/avatar/emoji `*_data_uri` attributes expect. ---

var _ datasource.DataSource = (*localImageDataSource)(nil)

// NewLocalImageDataSource returns a new discord_local_image data source.
func NewLocalImageDataSource() datasource.DataSource {
	return &localImageDataSource{}
}

type localImageDataSource struct{}

type localImageDataSourceModel struct {
	Path    types.String `tfsdk:"path"`
	DataURI types.String `tfsdk:"data_uri"`
}

func (d *localImageDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_local_image"
}

func (d *localImageDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads a local image file and produces the base64 data URI the icon/avatar/emoji `*_data_uri` attributes expect.",
		Attributes: map[string]schema.Attribute{
			"path": schema.StringAttribute{
				MarkdownDescription: "Path to a local image file (`.png`, `.jpg`/`.jpeg`, or `.gif`).",
				Required:            true,
			},
			"data_uri": schema.StringAttribute{
				MarkdownDescription: "Base64 data URI of the image, e.g. `data:image/png;base64,...`.",
				Computed:            true,
			},
		},
	}
}

func (d *localImageDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data localImageDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	path := data.Path.ValueString()
	bytes, err := os.ReadFile(path)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read image file", err.Error())
		return
	}

	mime := "image/png"
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png":
		mime = "image/png"
	case ".jpg", ".jpeg":
		mime = "image/jpeg"
	case ".gif":
		mime = "image/gif"
	}

	data.DataURI = types.StringValue("data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(bytes))

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
