package provider

import (
	"fmt"
	"reflect"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestBitsToKeys(t *testing.T) {
	cases := []struct {
		bits uint64
		want []string
	}{
		{0, nil},
		{1 << 10, []string{"view_channel"}},
		{1<<10 | 1<<11, []string{"view_channel", "send_messages"}},
		{0x6, []string{"kick_members", "ban_members"}},
		{1 << 52, []string{"bypass_slowmode"}},
		{1 << 47, []string{"unknown_47"}},
		{1 << 63, []string{"unknown_63"}},
		{1<<53 | 1<<10, []string{"view_channel", "unknown_53"}},
	}
	for _, c := range cases {
		if got := bitsToKeys(c.bits); !reflect.DeepEqual(got, c.want) {
			t.Errorf("bitsToKeys(%d) = %v, want %v", c.bits, got, c.want)
		}
	}
}

func TestKeysToBitsRoundTrip(t *testing.T) {
	for _, bits := range []uint64{0, 1, 0x6, 1 << 47, 1 << 63, 1<<63 | 1<<52 | 1<<47 | 1, ^uint64(0)} {
		got, err := keysToBits(bitsToKeys(bits))
		if err != nil || got != bits {
			t.Errorf("round trip of %d = %d, %v", bits, got, err)
		}
	}
}

func TestKeyShiftValidation(t *testing.T) {
	good := map[string]uint{"view_channel": 10, "unknown_47": 47, "unknown_63": 63}
	for k, want := range good {
		if got, err := keyShift(k); err != nil || got != want {
			t.Errorf("keyShift(%q) = %d, %v; want %d", k, got, err, want)
		}
	}
	for _, k := range []string{"", "nope", "unknown_", "unknown_64", "unknown_047", "unknown_+47", "unknown_-1", "unknown_ 47", "unknown_0", "unknown_10", "UNKNOWN_47", "unknown_1.0", "unknown_256"} {
		if _, err := keyShift(k); err == nil {
			t.Errorf("keyShift(%q) accepted an invalid key", k)
		}
	}
}

func TestParseBits(t *testing.T) {
	for _, s := range []string{"0", "1", "1024", "18446744073709551615"} {
		v, err := parseBits(s)
		if err != nil || strconv.FormatUint(v, 10) != s {
			t.Errorf("parseBits(%q) = %d, %v", s, v, err)
		}
	}
	for _, s := range []string{"01", "+1", "-1", " 1", "1 ", "0x10", "18446744073709551616", "1.0", "abc"} {
		if _, err := parseBits(s); err == nil {
			t.Errorf("parseBits(%q) accepted a non-canonical value", s)
		}
	}
}

func TestSideMaskUnknownSafe(t *testing.T) {
	nullSet := types.SetNull(types.StringType)
	if _, known := sideMask(types.SetUnknown(types.StringType), types.StringNull()); known {
		t.Error("unknown set reported known")
	}
	if _, known := sideMask(nullSet, types.StringUnknown()); known {
		t.Error("unknown bits reported known")
	}
	withUnknown := types.SetValueMust(types.StringType, []attr.Value{types.StringUnknown()})
	if _, known := sideMask(withUnknown, types.StringNull()); known {
		t.Error("set with an unknown element reported known")
	}
	if m, known := sideMask(nullSet, types.StringNull()); !known || m != 0 {
		t.Errorf("both null = %d, %v; want known empty", m, known)
	}
}

func TestSideMaskBothForms(t *testing.T) {
	keys := types.SetValueMust(types.StringType, []attr.Value{types.StringValue("view_channel")})
	if m, known := sideMask(keys, types.StringValue("1024")); !known || m != 1024 {
		t.Errorf("equivalent forms = %d, %v; want 1024, known", m, known)
	}
	if _, known := sideMask(keys, types.StringValue("2048")); known {
		t.Error("disagreeing forms reported known")
	}
	if _, known := sideMask(keys, types.StringUnknown()); known {
		t.Error("unknown bits alongside keys reported known")
	}
}

func TestPermissionsObjectRoundTrip(t *testing.T) {
	live := []permissionOverwrite{
		{ID: "20", Type: 0, Allow: "1024", Deny: "2048"},
		{ID: "10", Type: 0, Allow: "0", Deny: "140737488355328"}, // 1<<47, unnamed
		{ID: "5", Type: 1, Allow: "8192", Deny: "0"},
	}
	obj, err := permissionsFromOverwrites(live)
	if err != nil {
		t.Fatal(err)
	}
	got, err := overwritesFromPermissions(obj)
	if err != nil {
		t.Fatal(err)
	}
	want := []permissionOverwrite{live[1], live[0], live[2]} // roles by id, then users
	if !reflect.DeepEqual(got, want) {
		t.Errorf("round trip = %+v, want %+v", got, want)
	}
}

func TestPermissionsFromNoOverwritesIsEmptyNotNull(t *testing.T) {
	obj, err := permissionsFromOverwrites(nil)
	if err != nil {
		t.Fatal(err)
	}
	if obj.IsNull() {
		t.Fatal("object is null")
	}
	for _, name := range []string{"role_overwrites", "user_overwrites"} {
		m := obj.Attributes()[name].(types.Map)
		if m.IsNull() || len(m.Elements()) != 0 {
			t.Errorf("%s = %v, want empty non-null map", name, m)
		}
	}
}

func TestApplyLivePermissionsLeavesNullAlone(t *testing.T) {
	perms := types.ObjectNull(channelPermissionsObjectType.AttrTypes)
	if err := applyLivePermissions(&perms, []permissionOverwrite{{ID: "1", Allow: "1", Deny: "0"}}); err != nil {
		t.Fatal(err)
	}
	if !perms.IsNull() {
		t.Error("null permissions were populated")
	}
}

const permsChannel = "discord_text_channel.test"

func permsCfg(permissions string) string {
	return fmt.Sprintf(`
resource "discord_text_channel" "test" {
  server_id = "999"
  name      = "private"
  %s
}
`, permissions)
}

// checkOverwrites asserts the mock's live overwrites for the channel in state.
func checkOverwrites(m *mockDiscord, rn string, want ...permissionOverwrite) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[rn]
		if !ok {
			return fmt.Errorf("resource %s not found in state", rn)
		}
		got := m.channelOverwrites(rs.Primary.ID)
		if len(got) == 0 && len(want) == 0 {
			return nil
		}
		if !reflect.DeepEqual(got, want) {
			return fmt.Errorf("live overwrites = %+v, want %+v", got, want)
		}
		return nil
	}
}

// seedFromState replaces the live overwrites of the test channel out of band.
func seedFromState(m *mockDiscord, overwrites ...permissionOverwrite) func() {
	return func() {
		m.seedOverwrites(m.channelIDByName("private"), overwrites...)
	}
}
