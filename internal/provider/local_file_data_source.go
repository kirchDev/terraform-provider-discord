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

// --- Compute-only helper: reads any local file into a base64 data URI, the input
// every binary attribute in this provider expects (`*_data_uri`: guild/role icons,
// banners, emoji and soundboard sound, sticker files, …). It detects the mime type
// from the extension; override it with `mime` when needed. The image-only sibling
// is `discord_local_image`. ---

var _ datasource.DataSource = (*localFileDataSource)(nil)

// NewLocalFileDataSource returns a new discord_local_file data source.
func NewLocalFileDataSource() datasource.DataSource {
	return &localFileDataSource{}
}

type localFileDataSource struct{}

type localFileDataSourceModel struct {
	Path    types.String `tfsdk:"path"`
	Mime    types.String `tfsdk:"mime"`
	DataURI types.String `tfsdk:"data_uri"`
}

func (d *localFileDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_local_file"
}

func (d *localFileDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads a local file into a base64 data URI for the provider's `*_data_uri` attributes " +
			"(images, audio, sticker files). The mime type is detected from the file extension unless `mime` is set.",
		Attributes: map[string]schema.Attribute{
			"path": schema.StringAttribute{
				MarkdownDescription: "Path to the local file.",
				Required:            true,
			},
			"mime": schema.StringAttribute{
				MarkdownDescription: "Mime type to use. Defaults to one detected from the file extension.",
				Optional:            true,
				Computed:            true,
			},
			"data_uri": schema.StringAttribute{
				MarkdownDescription: "The file as a `data:<mime>;base64,...` URI.",
				Computed:            true,
			},
		},
	}
}

func (d *localFileDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data localFileDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	raw, err := os.ReadFile(data.Path.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read file", err.Error())
		return
	}

	mime := data.Mime.ValueString()
	if data.Mime.IsNull() || data.Mime.IsUnknown() {
		mime = mimeForFile(data.Path.ValueString())
	}
	data.Mime = types.StringValue(mime)
	data.DataURI = types.StringValue("data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(raw))

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// mimeForFile guesses a mime type from a file extension, covering the asset kinds
// Discord accepts (images, audio, Lottie JSON).
func mimeForFile(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png":
		return "image/png"
	case ".apng":
		return "image/apng"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".mp3":
		return "audio/mpeg"
	case ".ogg":
		return "audio/ogg"
	case ".json":
		return "application/json"
	default:
		return "application/octet-stream"
	}
}
