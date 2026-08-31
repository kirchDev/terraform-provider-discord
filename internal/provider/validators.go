package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// --- Small, dependency-free value validators for the provider's enum and range
// attributes, so a bad value fails at plan time with a clear message instead of
// surfacing as a Discord API 400 during apply. ---

// int64OneOfValidator permits only the listed int64 values.
type int64OneOfValidator struct{ allowed []int64 }

func int64OneOf(allowed ...int64) validator.Int64 { return int64OneOfValidator{allowed} }

func (v int64OneOfValidator) Description(_ context.Context) string {
	return fmt.Sprintf("value must be one of: %v", v.allowed)
}

func (v int64OneOfValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v int64OneOfValidator) ValidateInt64(_ context.Context, req validator.Int64Request, resp *validator.Int64Response) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	val := req.ConfigValue.ValueInt64()
	for _, a := range v.allowed {
		if val == a {
			return
		}
	}
	resp.Diagnostics.AddAttributeError(req.Path, "Invalid value", fmt.Sprintf("must be one of %v, got %d", v.allowed, val))
}

// int64BetweenValidator permits an inclusive [min, max] range.
type int64BetweenValidator struct{ min, max int64 }

func int64Between(min, max int64) validator.Int64 { return int64BetweenValidator{min, max} }

func (v int64BetweenValidator) Description(_ context.Context) string {
	return fmt.Sprintf("value must be between %d and %d", v.min, v.max)
}

func (v int64BetweenValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v int64BetweenValidator) ValidateInt64(_ context.Context, req validator.Int64Request, resp *validator.Int64Response) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if val := req.ConfigValue.ValueInt64(); val < v.min || val > v.max {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid value", fmt.Sprintf("must be between %d and %d, got %d", v.min, v.max, val))
	}
}

// uniqueNestedKeyValidator rejects duplicate `key` values within a nested list.
// The key is an element's identity across applies, so two elements sharing one
// would both match the same prior element and the loser's server-assigned id
// would be re-minted — the corruption the key exists to prevent.
type uniqueNestedKeyValidator struct{ noun string }

func uniqueNestedKey(noun string) validator.List { return uniqueNestedKeyValidator{noun} }

func (v uniqueNestedKeyValidator) Description(_ context.Context) string {
	return fmt.Sprintf("every %s must carry a unique key", v.noun)
}

func (v uniqueNestedKeyValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v uniqueNestedKeyValidator) ValidateList(_ context.Context, req validator.ListRequest, resp *validator.ListResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	elements := req.ConfigValue.Elements()
	seen := make(map[string]int, len(elements))
	for i, el := range elements {
		obj, ok := el.(types.Object)
		if !ok {
			continue
		}
		key, ok := obj.Attributes()["key"].(types.String)
		if !ok || key.IsNull() || key.IsUnknown() {
			continue
		}
		if first, dup := seen[key.ValueString()]; dup {
			resp.Diagnostics.AddAttributeError(
				req.Path.AtListIndex(i).AtName("key"),
				"Duplicate key",
				fmt.Sprintf("%s key %q is already used at index %d. Each key must be unique so the id Discord assigned stays attached to the right %s.",
					v.noun, key.ValueString(), first, v.noun),
			)
			continue
		}
		seen[key.ValueString()] = i
	}
}

// stringOneOfValidator permits only the listed string values (case-insensitive).
type stringOneOfValidator struct{ allowed []string }

func stringOneOf(allowed ...string) validator.String { return stringOneOfValidator{allowed} }

func (v stringOneOfValidator) Description(_ context.Context) string {
	return fmt.Sprintf("value must be one of: %s", strings.Join(v.allowed, ", "))
}

func (v stringOneOfValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v stringOneOfValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	val := strings.ToLower(req.ConfigValue.ValueString())
	for _, a := range v.allowed {
		if val == strings.ToLower(a) {
			return
		}
	}
	resp.Diagnostics.AddAttributeError(req.Path, "Invalid value", fmt.Sprintf("must be one of %s, got %q", strings.Join(v.allowed, ", "), req.ConfigValue.ValueString()))
}
