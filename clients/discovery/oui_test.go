package discovery

import "testing"

func TestLookupVendor(t *testing.T) {
	cases := []struct {
		name string
		mac  string
		want string
	}{
		{"registered apple lowercase", "cc:27:46:11:22:33", "Apple, Inc."},
		{"registered apple uppercase", "CC:27:46:11:22:33", "Apple, Inc."},
		{"registered apple dashes", "cc-27-46-11-22-33", "Apple, Inc."},
		{"locally administered libvirt", "fe:54:e6:00:00:01", LocallyAdministeredVendor},
		{"locally administered random", "82:d1:0f:aa:bb:cc", LocallyAdministeredVendor},
		{"empty input", "", ""},
		{"too short", "ab:cd", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := LookupVendor(tc.mac)
			if got != tc.want {
				t.Fatalf("LookupVendor(%q) = %q, want %q", tc.mac, got, tc.want)
			}
		})
	}
}

func TestIsLocallyAdministered(t *testing.T) {
	cases := []struct {
		prefix string
		want   bool
	}{
		{"00:11:22", false}, // 0x00 — bit 1 clear
		{"CC:27:46", false}, // 0xCC = 1100_1100 — bit 1 clear
		{"FE:54:E6", true},  // 0xFE = 1111_1110 — bit 1 set
		{"82:D1:0F", true},  // 0x82 = 1000_0010 — bit 1 set
		{"02:00:00", true},  // canonical LA example
		{"", false},
		{"X", false},
	}
	for _, tc := range cases {
		if got := isLocallyAdministered(tc.prefix); got != tc.want {
			t.Errorf("isLocallyAdministered(%q) = %v, want %v", tc.prefix, got, tc.want)
		}
	}
}
