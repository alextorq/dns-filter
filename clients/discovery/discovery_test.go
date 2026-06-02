package discovery

import (
	"net"
	"testing"
)

func mustCIDR(t *testing.T, cidr string) *net.IPNet {
	t.Helper()
	_, n, err := net.ParseCIDR(cidr)
	if err != nil {
		t.Fatalf("bad cidr %q: %v", cidr, err)
	}
	return n
}

// Regression for the mDNS leak: on host networking the box answers the mDNS
// browse on every docker0/br-* address it owns (172.18.0.1, 172.24.0.1, …) with
// its own hostname. Those entries have Source "mdns" and only an IP — the ARP
// Device-column filter can't see them, so filterDockerDevices drops them by
// Docker subnet while keeping real LAN devices.
func TestFilterDockerDevices(t *testing.T) {
	dockerNets := []*net.IPNet{
		mustCIDR(t, "172.18.0.0/16"),
		mustCIDR(t, "172.24.0.0/16"),
	}
	devices := []Device{
		{IP: "172.18.0.1", Hostname: "raspberry", Source: "mdns"},
		{IP: "172.24.0.1", Hostname: "raspberry", Source: "mdns"},
		{IP: "192.168.88.45", Hostname: "phone", Source: "mdns"},
		{IP: "10.0.0.5", MAC: "aa:bb:cc:dd:ee:ff", Source: "arp-table"},
	}

	got := filterDockerDevices(devices, dockerNets)
	if len(got) != 2 {
		t.Fatalf("expected 2 LAN devices, got %d: %+v", len(got), got)
	}
	for _, d := range got {
		if d.IP == "172.18.0.1" || d.IP == "172.24.0.1" {
			t.Errorf("Docker device leaked: %s (%s)", d.IP, d.Source)
		}
	}
}

// Edge: with no known Docker subnets (no bridges, or interface read failed)
// the filter is a no-op — every device survives, including 172.x ones we can't
// prove are Docker.
func TestFilterDockerDevices_NoNetsIsNoop(t *testing.T) {
	devices := []Device{
		{IP: "172.18.0.1", Source: "mdns"},
		{IP: "192.168.88.45", Source: "mdns"},
	}
	if got := filterDockerDevices(devices, nil); len(got) != 2 {
		t.Fatalf("nil dockerNets should keep all devices, got %d: %+v", len(got), got)
	}
}
