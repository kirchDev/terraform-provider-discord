package provider

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/kirchDev/terraform-provider-discord/internal/client"
)

// decodeDataURI parses a "data:<mime>;base64,<payload>" URI into its mime type and
// decoded bytes — used to turn a `*_data_uri` input into the raw bytes a multipart
// upload (e.g. a sticker file) needs.
func decodeDataURI(s string) (mime string, data []byte, err error) {
	rest, ok := strings.CutPrefix(s, "data:")
	if !ok {
		return "", nil, fmt.Errorf("not a data URI (must start with %q)", "data:")
	}
	meta, payload, ok := strings.Cut(rest, ",")
	if !ok {
		return "", nil, fmt.Errorf("malformed data URI: missing comma")
	}
	if !strings.Contains(meta, "base64") {
		return "", nil, fmt.Errorf("data URI must be base64-encoded")
	}
	mime = strings.TrimSuffix(meta, ";base64")
	data, err = base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return "", nil, fmt.Errorf("decoding data URI payload: %w", err)
	}
	return mime, data, nil
}

// extForMime maps a binary mime type to a filename extension, for multipart file
// parts where Discord infers the asset format from the name.
func extForMime(mime string) string {
	switch mime {
	case "image/png":
		return "png"
	case "image/apng":
		return "apng"
	case "image/gif":
		return "gif"
	case "application/json", "application/json+lottie", "application/lottie+json":
		return "json"
	default:
		return "bin"
	}
}

// keepTimestamp reconciles an RFC3339 timestamp read back from the API with the
// one already in state/config. Discord re-formats timestamps (e.g. sends back
// `...+00:00` for a `...000Z` input), which would otherwise look like drift. If
// the current value denotes the same instant as the API value, the current
// (config) value is kept; otherwise the API value wins (real drift / import).
func keepTimestamp(current types.String, apiVal string) types.String {
	if apiVal == "" {
		return types.StringNull()
	}
	if !current.IsNull() && !current.IsUnknown() {
		a, err1 := time.Parse(time.RFC3339, current.ValueString())
		b, err2 := time.Parse(time.RFC3339, apiVal)
		if err1 == nil && err2 == nil && a.Equal(b) {
			return current
		}
	}
	return types.StringValue(apiVal)
}

// strPtrOrNil returns a *string for a tfsdk string, or nil when the value is null
// or unknown — handy for JSON bodies where an absent field must serialise as null.
func strPtrOrNil(s types.String) *string {
	if s.IsNull() || s.IsUnknown() {
		return nil
	}
	v := s.ValueString()
	return &v
}

// strSet returns the string elements of a tfsdk set and whether it was set
// (non-null, non-unknown) — so a body field is sent only when the user gave it.
func strSet(ctx context.Context, set types.Set) ([]string, bool, error) {
	if set.IsNull() || set.IsUnknown() {
		return nil, false, nil
	}
	var out []string
	if d := set.ElementsAs(ctx, &out, false); d.HasError() {
		return nil, false, fmt.Errorf("reading string set")
	}
	return out, true, nil
}

// setOfStrings builds a tfsdk string set from a slice (nil → empty set). The bool
// reports whether conversion failed.
func setOfStrings(ctx context.Context, in []string) (types.Set, bool) {
	set, d := types.SetValueFrom(ctx, types.StringType, in)
	return set, d.HasError()
}

// errNotInCollection signals that a list-scan read did not find the requested id.
// Read methods treat it like a 404 and drop the resource from state.
var errNotInCollection = errors.New("resource not found in collection")

// notFound reports whether err means the resource is gone upstream — either a
// real API 404 or a missing element in a list-scan read.
func notFound(err error) bool {
	return client.NotFound(err) || errors.Is(err, errNotInCollection)
}

// findInList GETs the collection at path and returns the first element whose
// idOf(...) equals id, or errNotInCollection. Used where Discord has no clean
// single-item read (e.g. a role within a guild, an overwrite within a channel).
func findInList[T any](ctx context.Context, c *client.Client, path, id string, idOf func(*T) string) (*T, error) {
	raws, err := c.List(ctx, path)
	if err != nil {
		return nil, err
	}
	for _, raw := range raws {
		var item T
		if err := json.Unmarshal(raw, &item); err != nil {
			return nil, err
		}
		if idOf(&item) == id {
			return &item, nil
		}
	}
	return nil, errNotInCollection
}
