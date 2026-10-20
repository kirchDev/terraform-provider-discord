package provider

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Shared exhaustive `permissions` attribute for guild channel resources and data sources.

const (
	overwriteTypeRole = 0
	overwriteTypeUser = 1
	unknownKeyPrefix  = "unknown_"
	maxPermissionBit  = 63
)

var (
	permissionShiftByKey = func() map[string]uint {
		m := make(map[string]uint, len(permissionFlags))
		for _, f := range permissionFlags {
			m[f.Key] = f.Shift
		}
		return m
	}()
	permissionKeyByShift = func() map[uint]string {
		m := make(map[uint]string, len(permissionFlags))
		for _, f := range permissionFlags {
			m[f.Shift] = f.Key
		}
		return m
	}()

	overwriteEntryType = types.ObjectType{AttrTypes: map[string]attr.Type{
		"allow":      types.SetType{ElemType: types.StringType},
		"allow_bits": types.StringType,
		"deny":       types.SetType{ElemType: types.StringType},
		"deny_bits":  types.StringType,
	}}
	overwriteMapType = types.MapType{ElemType: overwriteEntryType}

	// channelPermissionsObjectType is the type of the `permissions` attribute.
	channelPermissionsObjectType = types.ObjectType{AttrTypes: map[string]attr.Type{
		"role_overwrites": overwriteMapType,
		"user_overwrites": overwriteMapType,
	}}
)

// keyShift resolves a permission key (named or unknown_<n>) to its bit shift.
func keyShift(key string) (uint, error) {
	if shift, ok := permissionShiftByKey[key]; ok {
		return shift, nil
	}
	n, ok := strings.CutPrefix(key, unknownKeyPrefix)
	if !ok {
		return 0, fmt.Errorf("unrecognized permission key %q", key)
	}
	shift, err := strconv.ParseUint(n, 10, 8)
	if err != nil || shift > maxPermissionBit || strconv.FormatUint(shift, 10) != n {
		return 0, fmt.Errorf("%q: the number must be a canonical decimal from 0 to %d", key, maxPermissionBit)
	}
	if named, ok := permissionKeyByShift[uint(shift)]; ok {
		return 0, fmt.Errorf("%q is a named permission, use %q", key, named)
	}
	return uint(shift), nil
}

// keysToBits ORs the bits of the given keys.
func keysToBits(keys []string) (uint64, error) {
	var bits uint64
	for _, k := range keys {
		shift, err := keyShift(k)
		if err != nil {
			return 0, err
		}
		bits |= 1 << shift
	}
	return bits, nil
}

// bitsToKeys decodes a bitfield to one key per set bit, in bit order.
func bitsToKeys(bits uint64) []string {
	var keys []string
	for shift := uint(0); shift <= maxPermissionBit; shift++ {
		if bits&(1<<shift) == 0 {
			continue
		}
		if k, ok := permissionKeyByShift[shift]; ok {
			keys = append(keys, k)
		} else {
			keys = append(keys, unknownKeyPrefix+strconv.FormatUint(uint64(shift), 10))
		}
	}
	return keys
}

// parseBits parses a canonical unsigned decimal bitfield; "" reads as 0 since
// Discord omits empty fields.
func parseBits(s string) (uint64, error) {
	if s == "" {
		return 0, nil
	}
	v, err := strconv.ParseUint(s, 10, 64)
	if err != nil || strconv.FormatUint(v, 10) != s {
		return 0, fmt.Errorf("%q is not a canonical unsigned 64-bit decimal", s)
	}
	return v, nil
}

func keySet(bits uint64) types.Set {
	keys := bitsToKeys(bits)
	elems := make([]attr.Value, len(keys))
	for i, k := range keys {
		elems[i] = types.StringValue(k)
	}
	return types.SetValueMust(types.StringType, elems)
}

// keysMask resolves a key set to its mask; known is false while the set is
// unknown or holds an unknown, null or invalid key.
func keysMask(keys types.Set) (mask uint64, known bool) {
	if keys.IsUnknown() {
		return 0, false
	}
	var strs []string
	for _, e := range keys.Elements() {
		s, ok := e.(types.String)
		if !ok || s.IsUnknown() || s.IsNull() {
			return 0, false
		}
		strs = append(strs, s.ValueString())
	}
	m, err := keysToBits(strs)
	return m, err == nil
}

// sideMask resolves one side (allow or deny) of an entry from whichever of its
// two forms is set. known is false while any input is unknown or invalid, or
// when both forms are set and disagree; both null is the known empty side.
func sideMask(keys types.Set, bits types.String) (mask uint64, known bool) {
	var bitsMask uint64
	if !bits.IsNull() {
		if bits.IsUnknown() {
			return 0, false
		}
		m, err := parseBits(bits.ValueString())
		if err != nil {
			return 0, false
		}
		bitsMask = m
	}
	if keys.IsNull() {
		return bitsMask, true
	}
	keysM, ok := keysMask(keys)
	if !ok || (!bits.IsNull() && keysM != bitsMask) {
		return 0, false
	}
	return keysM, true
}

func overwriteSideAttributes() map[string]schema.Attribute {
	side := func(name string) (schema.SetAttribute, schema.StringAttribute) {
		return schema.SetAttribute{
				MarkdownDescription: "Permission keys (see `discord_permission`), or `unknown_<n>` for a bit the provider has no name for. Either form may be set alone and the other is computed; if both are set they must be equivalent.",
				ElementType:         types.StringType,
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Set{partnerKeysModifier{partner: name + "_bits"}},
				Validators:          []validator.Set{permissionKeysValidator{}},
			}, schema.StringAttribute{
				MarkdownDescription: "The `" + name + "` permissions as a decimal bitfield string. Either form may be set alone and the other is computed; if both are set they must be equivalent.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.String{partnerBitsModifier{partner: name}},
				Validators:          []validator.String{canonicalBitsValidator{}},
			}
	}
	allow, allowBits := side("allow")
	deny, denyBits := side("deny")
	return map[string]schema.Attribute{"allow": allow, "allow_bits": allowBits, "deny": deny, "deny_bits": denyBits}
}

// channelPermissionsAttribute is the optional, exhaustive `permissions` attribute.
func channelPermissionsAttribute() schema.SingleNestedAttribute {
	overwrites := func(what string) schema.MapNestedAttribute {
		return schema.MapNestedAttribute{
			MarkdownDescription: "Overwrites for " + what + ", keyed by snowflake. Unset means none.",
			Optional:            true,
			Computed:            true,
			Default:             mapdefault.StaticValue(types.MapValueMust(overwriteEntryType, map[string]attr.Value{})),
			NestedObject: schema.NestedAttributeObject{
				Attributes: overwriteSideAttributes(),
				Validators: []validator.Object{overwriteEntryValidator{}},
			},
		}
	}
	return schema.SingleNestedAttribute{
		MarkdownDescription: "The channel's permission overwrites, exhaustively: when set, any live overwrite not declared here is removed on apply and reported as drift on plan. Omit to leave overwrites unmanaged. Don't combine with `discord_channel_permission` on the same channel. Unlike `discord_channel_permission`, `allow`/`deny` here are sets of permission keys; use `allow_bits`/`deny_bits` for decimal strings.",
		Optional:            true,
		Attributes: map[string]schema.Attribute{
			"role_overwrites": overwrites("roles"),
			"user_overwrites": overwrites("users"),
		},
	}
}

// partnerKeysModifier derives `allow`/`deny` from the configured `*_bits`.
type partnerKeysModifier struct{ partner string }

func (partnerKeysModifier) Description(context.Context) string {
	return "Derives the key set from the bitfield when only the bitfield is configured."
}
func (m partnerKeysModifier) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (m partnerKeysModifier) PlanModifySet(ctx context.Context, req planmodifier.SetRequest, resp *planmodifier.SetResponse) {
	if !req.ConfigValue.IsNull() {
		resp.PlanValue = req.ConfigValue
		return
	}
	var bits types.String
	if resp.Diagnostics.Append(req.Config.GetAttribute(ctx, req.Path.ParentPath().AtName(m.partner), &bits)...); resp.Diagnostics.HasError() {
		return
	}
	if mask, known := sideMask(types.SetNull(types.StringType), bits); known {
		resp.PlanValue = keySet(mask)
	} else {
		resp.PlanValue = types.SetUnknown(types.StringType)
	}
}

// partnerBitsModifier derives `allow_bits`/`deny_bits` from the configured keys.
type partnerBitsModifier struct{ partner string }

func (partnerBitsModifier) Description(context.Context) string {
	return "Derives the bitfield from the key set when only the key set is configured."
}
func (m partnerBitsModifier) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (m partnerBitsModifier) PlanModifyString(ctx context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	if !req.ConfigValue.IsNull() {
		resp.PlanValue = req.ConfigValue
		return
	}
	var keys types.Set
	if resp.Diagnostics.Append(req.Config.GetAttribute(ctx, req.Path.ParentPath().AtName(m.partner), &keys)...); resp.Diagnostics.HasError() {
		return
	}
	if mask, known := sideMask(keys, types.StringNull()); known {
		resp.PlanValue = types.StringValue(strconv.FormatUint(mask, 10))
	} else {
		resp.PlanValue = types.StringUnknown()
	}
}

type permissionKeysValidator struct{}

func (permissionKeysValidator) Description(context.Context) string {
	return "Each element must be a known permission key or unknown_<n>."
}
func (v permissionKeysValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (permissionKeysValidator) ValidateSet(_ context.Context, req validator.SetRequest, resp *validator.SetResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	for _, e := range req.ConfigValue.Elements() {
		s, ok := e.(types.String)
		if !ok || s.IsNull() || s.IsUnknown() {
			continue
		}
		if _, err := keyShift(s.ValueString()); err != nil {
			resp.Diagnostics.AddAttributeError(req.Path.AtSetValue(e), "Invalid permission key", err.Error())
		}
	}
}

type canonicalBitsValidator struct{}

func (canonicalBitsValidator) Description(context.Context) string {
	return "Must be an unsigned 64-bit decimal without leading zeros."
}
func (v canonicalBitsValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (canonicalBitsValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if _, err := parseBits(req.ConfigValue.ValueString()); err != nil || req.ConfigValue.ValueString() == "" {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid permission bitfield", fmt.Sprintf("%q is not an unsigned 64-bit decimal without leading zeros or whitespace.", req.ConfigValue.ValueString()))
	}
}

// overwriteEntryValidator checks the cross-attribute rules of one overwrite.
type overwriteEntryValidator struct{}

func (overwriteEntryValidator) Description(context.Context) string {
	return "Both forms of a side agree, a non-empty entry, and no bit both allowed and denied."
}
func (v overwriteEntryValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (overwriteEntryValidator) ValidateObject(_ context.Context, req validator.ObjectRequest, resp *validator.ObjectResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	allow, allowBits, deny, denyBits := entryAttrs(req.ConfigValue)
	for _, side := range []struct {
		name string
		keys types.Set
		bits types.String
	}{{"allow", allow, allowBits}, {"deny", deny, denyBits}} {
		if side.keys.IsNull() || side.bits.IsNull() || side.bits.IsUnknown() {
			continue
		}
		keysM, keysOK := keysMask(side.keys)
		bitsM, err := parseBits(side.bits.ValueString())
		if keysOK && err == nil && keysM != bitsM {
			resp.Diagnostics.AddAttributeError(req.Path.AtName(side.name+"_bits"), "Conflicting attributes",
				fmt.Sprintf("%q and %q must describe the same permissions when both are set.", side.name, side.name+"_bits"))
		}
	}
	if resp.Diagnostics.HasError() {
		return
	}
	allowMask, allowKnown := sideMask(allow, allowBits)
	denyMask, denyKnown := sideMask(deny, denyBits)
	if allowKnown && denyKnown {
		if allowMask == 0 && denyMask == 0 {
			resp.Diagnostics.AddAttributeError(req.Path, "Empty permission overwrite", "An overwrite must allow or deny at least one permission.")
		} else if both := allowMask & denyMask; both != 0 {
			resp.Diagnostics.AddAttributeError(req.Path, "Conflicting permission overwrite",
				fmt.Sprintf("Permissions %s are both allowed and denied.", strings.Join(bitsToKeys(both), ", ")))
		}
	}
}

func entryAttrs(o types.Object) (allow types.Set, allowBits types.String, deny types.Set, denyBits types.String) {
	a := o.Attributes()
	allow, _ = a["allow"].(types.Set)
	allowBits, _ = a["allow_bits"].(types.String)
	deny, _ = a["deny"].(types.Set)
	denyBits, _ = a["deny_bits"].(types.String)
	return
}

// overwritesFromPermissions builds the permission_overwrites array, roles then
// users, each ordered by id so the request is deterministic.
func overwritesFromPermissions(perms types.Object) ([]permissionOverwrite, error) {
	out := []permissionOverwrite{}
	for _, g := range []struct {
		attr string
		kind int
	}{{"role_overwrites", overwriteTypeRole}, {"user_overwrites", overwriteTypeUser}} {
		m, _ := perms.Attributes()[g.attr].(types.Map)
		if m.IsUnknown() {
			return nil, fmt.Errorf("%s is unknown at apply time", g.attr)
		}
		elems := m.Elements()
		ids := make([]string, 0, len(elems))
		for id := range elems {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			entry, _ := elems[id].(types.Object)
			allow, allowBits, deny, denyBits := entryAttrs(entry)
			a, aok := sideMask(allow, allowBits)
			d, dok := sideMask(deny, denyBits)
			if !aok || !dok {
				return nil, fmt.Errorf("overwrite %s %s has an unresolved or invalid permission value", g.attr, id)
			}
			out = append(out, permissionOverwrite{
				ID: id, Type: g.kind,
				Allow: strconv.FormatUint(a, 10), Deny: strconv.FormatUint(d, 10),
			})
		}
	}
	return out, nil
}

// setPermissionOverwrites adds permission_overwrites to a create/update body
// when permissions is managed, reporting whether it failed; null sends nothing.
func setPermissionOverwrites(body map[string]any, perms types.Object, diags *diag.Diagnostics) (failed bool) {
	if perms.IsNull() {
		return false
	}
	if perms.IsUnknown() {
		diags.AddError("Invalid channel permissions", "permissions is unknown at apply time")
		return true
	}
	ow, err := overwritesFromPermissions(perms)
	if err != nil {
		diags.AddError("Invalid channel permissions", err.Error())
		return true
	}
	body["permission_overwrites"] = ow
	return false
}

// permissionsFromOverwrites splits live overwrites by type into the attribute value.
func permissionsFromOverwrites(live []permissionOverwrite) (types.Object, error) {
	groups := map[int]map[string]attr.Value{overwriteTypeRole: {}, overwriteTypeUser: {}}
	for _, o := range live {
		g, ok := groups[o.Type]
		if !ok {
			return types.ObjectNull(channelPermissionsObjectType.AttrTypes), fmt.Errorf("overwrite %s has unsupported type %d", o.ID, o.Type)
		}
		allow, err := parseBits(o.Allow)
		if err != nil {
			return types.ObjectNull(channelPermissionsObjectType.AttrTypes), fmt.Errorf("overwrite %s allow: %w", o.ID, err)
		}
		deny, err := parseBits(o.Deny)
		if err != nil {
			return types.ObjectNull(channelPermissionsObjectType.AttrTypes), fmt.Errorf("overwrite %s deny: %w", o.ID, err)
		}
		g[o.ID] = types.ObjectValueMust(overwriteEntryType.AttrTypes, map[string]attr.Value{
			"allow":      keySet(allow),
			"allow_bits": types.StringValue(strconv.FormatUint(allow, 10)),
			"deny":       keySet(deny),
			"deny_bits":  types.StringValue(strconv.FormatUint(deny, 10)),
		})
	}
	return types.ObjectValueMust(channelPermissionsObjectType.AttrTypes, map[string]attr.Value{
		"role_overwrites": types.MapValueMust(overwriteEntryType, groups[overwriteTypeRole]),
		"user_overwrites": types.MapValueMust(overwriteEntryType, groups[overwriteTypeUser]),
	}), nil
}

// applyLivePermissions replaces a managed permissions value with the live
// overwrites; a null (unmanaged or imported) value stays null.
func applyLivePermissions(perms *types.Object, live []permissionOverwrite) error {
	if perms.IsNull() {
		return nil
	}
	obj, err := permissionsFromOverwrites(live)
	if err != nil {
		return err
	}
	*perms = obj
	return nil
}
