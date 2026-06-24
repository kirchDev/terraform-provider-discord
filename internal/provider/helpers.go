package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/kirchDev/terraform-provider-discord/internal/client"
)

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
