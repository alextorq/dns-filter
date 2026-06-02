package discovery

import "testing"

func TestIsDockerBridgeIface(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		// Positive: Docker's default bridge and compose-created bridges
		// ("br-" + exactly 12 lowercase hex).
		{"docker0", true},
		{"br-78f9ca1f4f18", true},
		{"br-de73d3819a89", true},
		{"br-4c8497284baa", true},

		// Negative: real LAN NICs, loopback, container veth peers.
		{"eth0", false},
		{"wlan0", false},
		{"lo", false},
		{"veth1591c54", false},
		{"", false},

		// Negative: non-Docker Linux bridges. br0/bridge0 have no dash; libvirt
		// uses virbr; and crucially OpenWrt/router LAN bridges are named br-lan/
		// br-wan/br-guest — a bare "br-" prefix would wrongly hide a real LAN.
		{"br0", false},
		{"bridge0", false},
		{"virbr0", false},
		{"br-lan", false},
		{"br-wan", false},
		{"br-guest", false},

		// Negative: "br-" but not exactly 12 hex (too short, too long, non-hex,
		// uppercase — Docker uses lowercase hex).
		{"br-78f9ca1f4f1", false},   // 11 hex
		{"br-78f9ca1f4f180", false}, // 13 hex
		{"br-78f9ca1f4f1z", false},  // non-hex char
		{"br-78F9CA1F4F18", false},  // uppercase
		{"br-", false},
	}
	for _, c := range cases {
		if got := isDockerBridgeIface(c.name); got != c.want {
			t.Errorf("isDockerBridgeIface(%q) = %v, want %v", c.name, got, c.want)
		}
	}
}
