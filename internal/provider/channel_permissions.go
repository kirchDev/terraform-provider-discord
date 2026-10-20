package provider

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
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
