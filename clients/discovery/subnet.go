// Package discovery enumerates devices on the local LAN. It combines a
// passive ARP-table read, an active ARP broadcast scan, and an mDNS browse,
// then enriches every result with a vendor name from a MAC OUI lookup.
//
// The whole package only makes sense for the LAN deployment mode — there's
// no LAN to discover from a public DoH endpoint. Web handlers should refuse
// /api/clients/discover when running in public mode rather than calling
// these functions.
package discovery

import (
	"errors"
	"fmt"
	"net"
	"strings"
)

// LocalSubnet describes the IPv4 subnet an active scan should target.
type LocalSubnet struct {
	Interface string
	SelfIP    net.IP
	SelfMAC   net.HardwareAddr
	CIDR      *net.IPNet
}

// ErrNoSubnet means we couldn't find an interface plausible to scan. The
// most common cause is running the dns-filter container on a Docker bridge
// instead of host networking — discovery is documented as requiring host net.
var ErrNoSubnet = errors.New("no suitable LAN interface found (host networking required for discovery)")

// FindLocalSubnet picks the most plausible LAN interface to scan. The rules:
// must be UP, must be IPv4, must be RFC1918 private, must not be a Docker
// bridge, mask must be /20 or narrower (a wider mask would be too many hosts
// to scan in any reasonable time). Most home hosts have exactly one match;
// if multiple, the first in net.Interfaces order wins.
func FindLocalSubnet() (*LocalSubnet, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("list interfaces: %w", err)
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		// Skip Docker-managed bridges (docker0 / br-<hash>). Identifying them by
		// interface name — not by IP range — means a real LAN that happens to
		// sit in 172.16.0.0/12 is still scanned, and Docker networks in any pool
		// (including its 192.168 secondary range) are still skipped.
		if isDockerBridgeIface(iface.Name) {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipnet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			ip4 := ipnet.IP.To4()
			if ip4 == nil {
				continue
			}
			if !ip4.IsPrivate() || ip4.IsLoopback() {
				continue
			}
			if ones, _ := ipnet.Mask.Size(); ones < 20 {
				continue
			}
			return &LocalSubnet{
				Interface: iface.Name,
				SelfIP:    ip4,
				SelfMAC:   iface.HardwareAddr,
				CIDR: &net.IPNet{
					IP:   ip4.Mask(ipnet.Mask),
					Mask: ipnet.Mask,
				},
			}, nil
		}
	}
	return nil, ErrNoSubnet
}

// isDockerBridgeIface reports whether an interface name belongs to a Docker
// bridge. Docker names its default bridge "docker0" and every user-defined
// (compose) network "br-<12 hex>" (the first 12 hex digits of the network ID).
// We match on the interface rather than on a guessed IP range so that:
//   - container neighbours are skipped no matter which subnet Docker assigned
//     them — the default pool is the whole 172.16.0.0/12 and Docker falls back
//     to a 192.168 pool once that fills, so a fixed prefix list misses the tail;
//   - a real LAN interface (eth0/wlan0) is never mistaken for a Docker bridge.
//
// The "br-" check is deliberately exact (prefix + exactly 12 lowercase hex)
// rather than a bare "br-" prefix: non-Docker Linux bridges are commonly named
// "br-lan"/"br-wan"/"br-guest" (OpenWrt, libvirt, netplan), and a loose prefix
// would wrongly exclude such a host's real LAN, making discovery return nothing.
//
// The kernel records the learning interface in the Device column of
// /proc/net/arp, so the passive ARP read (parseARPTable) filters on the same
// predicate. (A Docker network created with an explicit
// com.docker.network.bridge.name gets an operator-chosen interface name that no
// name heuristic can recognise; such neighbours are surfaced like any LAN host,
// and the "show Docker networks" toggle is the escape hatch.)
func isDockerBridgeIface(name string) bool {
	if name == "docker0" {
		return true
	}
	rest, ok := strings.CutPrefix(name, "br-")
	if !ok || len(rest) != 12 {
		return false
	}
	for _, c := range rest {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// dockerBridgeNets returns the IPv4 subnets configured on the host's Docker
// bridge interfaces (docker0 / br-<hash>). It is the IP-based companion to
// isDockerBridgeIface: discovery sources that only surface an IP and not a
// learning interface (mDNS) can't be filtered by the Device-column trick the
// ARP read uses, so we instead check whether their IP falls in a real Docker
// subnet. Best-effort — any error yields nil (filter nothing) rather than
// failing the sweep.
func dockerBridgeNets() []*net.IPNet {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var nets []*net.IPNet
	for _, iface := range ifaces {
		if !isDockerBridgeIface(iface.Name) {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok && ipnet.IP.To4() != nil {
				nets = append(nets, ipnet)
			}
		}
	}
	return nets
}

// ipInNets reports whether ip falls inside any of nets.
func ipInNets(ip net.IP, nets []*net.IPNet) bool {
	ip4 := ip.To4()
	if ip4 == nil {
		return false
	}
	for _, n := range nets {
		if n.Contains(ip4) {
			return true
		}
	}
	return false
}

// EnumerateHosts returns every usable IPv4 host in the subnet, skipping the
// network address, the broadcast address, and the scanner's own IP. For a /24
// that's 253 entries.
func (s *LocalSubnet) EnumerateHosts() []net.IP {
	network := s.CIDR.IP.To4()
	mask := s.CIDR.Mask
	broadcast := make(net.IP, 4)
	for i := range broadcast {
		broadcast[i] = network[i] | ^mask[i]
	}

	var ips []net.IP
	cur := make(net.IP, 4)
	copy(cur, network)
	incIP(cur)
	for !cur.Equal(broadcast) {
		if !cur.Equal(s.SelfIP) {
			next := make(net.IP, 4)
			copy(next, cur)
			ips = append(ips, next)
		}
		incIP(cur)
	}
	return ips
}

func incIP(ip net.IP) {
	for i := len(ip) - 1; i >= 0; i-- {
		ip[i]++
		if ip[i] != 0 {
			return
		}
	}
}
