//go:build linux

package discovery

import (
	"strings"
	"testing"
)

// realProcNetARP mirrors the column layout of /proc/net/arp on a Docker host:
// container neighbours learned on docker0 / br-<hash> interfaces, plus a couple
// of genuine LAN hosts on eth0. Only the eth0 rows should survive — this is the
// regression for the leak where Docker bridges outside the old 172.17–172.23
// prefix list (e.g. 172.24/172.25) showed up in the network-scan UI.
const realProcNetARP = `IP address       HW type     Flags       HW address            Mask     Device
172.17.0.3       0x1         0x2         66:86:f1:2f:db:3d     *        docker0
172.25.0.6       0x1         0x2         12:0f:22:78:2a:06     *        br-78f9ca1f4f18
172.24.0.2       0x1         0x2         4a:65:21:f6:3c:de     *        br-be9d52286b23
192.168.88.1     0x1         0x2         d4:01:c3:b7:00:87     *        eth0
192.168.88.45    0x1         0x2         5a:ca:8d:d8:05:f9     *        eth0
`

func TestParseARPTable_SkipsDockerBridges(t *testing.T) {
	entries, err := parseARPTable(strings.NewReader(realProcNetARP), true) // filterDocker on
	if err != nil {
		t.Fatalf("parseARPTable: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 LAN entries, got %d: %+v", len(entries), entries)
	}
	for _, e := range entries {
		if !strings.HasPrefix(e.IP.String(), "192.168.88.") {
			t.Errorf("Docker neighbour leaked into results: %s (source %s)", e.IP, e.Source)
		}
	}
}

// With filterDocker off the "show Docker networks" path keeps every complete
// row, including the docker0 / br-<hash> neighbours.
func TestParseARPTable_NoFilterKeepsAll(t *testing.T) {
	entries, err := parseARPTable(strings.NewReader(realProcNetARP), false) // filterDocker off
	if err != nil {
		t.Fatalf("parseARPTable: %v", err)
	}
	if len(entries) != 5 {
		t.Fatalf("expected all 5 rows with filterDocker off, got %d: %+v", len(entries), entries)
	}
}

// Negative / edge cases: an incomplete (all-zero MAC) row, a malformed short
// line, and a non-IPv4 address must all be dropped without producing an error.
func TestParseARPTable_DropsIncompleteAndMalformed(t *testing.T) {
	const data = `IP address       HW type     Flags       HW address            Mask     Device
10.0.0.5         0x1         0x0         00:00:00:00:00:00     *        eth0
short line
not_an_ip        0x1         0x2         aa:bb:cc:dd:ee:ff     *        eth0
10.0.0.9         0x1         0x2         aa:bb:cc:dd:ee:ff     *        eth0
`
	entries, err := parseARPTable(strings.NewReader(data), true)
	if err != nil {
		t.Fatalf("parseARPTable: %v", err)
	}
	if len(entries) != 1 || entries[0].IP.String() != "10.0.0.9" {
		t.Fatalf("expected only 10.0.0.9, got %+v", entries)
	}
}
