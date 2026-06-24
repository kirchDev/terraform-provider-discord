package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
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
