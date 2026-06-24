package provider

import (
	"math/big"
	"testing"
)

// TestPermissionBitsAreCorrect spot-checks the named-permission → bit mapping
// against the canonical Discord shifts. The discord_permission helper's whole
// value is that these are right, so they get an explicit guard.
func TestPermissionBitsAreCorrect(t *testing.T) {
	cases := map[string]string{
		"create_instant_invite": "1",                // 1<<0
		"administrator":         "8",                // 1<<3
		"view_channel":          "1024",             // 1<<10
		"send_messages":         "2048",             // 1<<11
		"manage_roles":          "268435456",        // 1<<28
		"moderate_members":      "1099511627776",    // 1<<40
		"send_voice_messages":   "70368744177664",   // 1<<46
		"send_polls":            "562949953421312",  // 1<<49
		"pin_messages":          "2251799813685248", // 1<<51
		"bypass_slowmode":       "4503599627370496", // 1<<52
		"connect":               "1048576",          // 1<<20
	}
	for key, want := range cases {
		bit, ok := permissionBit(key)
		if !ok {
			t.Errorf("permission key %q not found", key)
			continue
		}
		if bit.String() != want {
			t.Errorf("%s = %s, want %s", key, bit.String(), want)
		}
	}
}

// TestPermissionKeysAreUniqueAndKnown ensures every flag resolves and there are
// no duplicate keys (a duplicate would silently shadow a permission).
func TestPermissionKeysAreUniqueAndKnown(t *testing.T) {
	seen := map[string]bool{}
	for _, key := range permissionKeys() {
		if seen[key] {
			t.Errorf("duplicate permission key %q", key)
		}
		seen[key] = true
		if _, ok := permissionBit(key); !ok {
			t.Errorf("permissionBit(%q) returned not-found for a listed key", key)
		}
	}
	if len(seen) != len(permissionFlags) {
		t.Errorf("got %d unique keys, want %d", len(seen), len(permissionFlags))
	}
}

// TestPermissionOr verifies that OR-ing keys produces the combined bitfield, the
// way the helper builds allow_bits / deny_bits.
func TestPermissionOr(t *testing.T) {
	acc := big.NewInt(0)
	for _, key := range []string{"view_channel", "send_messages"} { // 1024 | 2048
		bit, _ := permissionBit(key)
		acc.Or(acc, bit)
	}
	if acc.String() != "3072" {
		t.Errorf("view_channel|send_messages = %s, want 3072", acc.String())
	}
}
