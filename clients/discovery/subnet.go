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

// dockerBridgePrefixes are the IPv4 prefixes Docker hands out for its default
// and user-defined bridges. We exclude them when picking a scan target so the
// scan doesn't enumerate the in-container bridge network instead of the real
// LAN. If the host's LAN happens to live in the same range (uncommon for home
// setups), an env-var override hook would be the next extension point.
var dockerBridgePrefixes = []string{
	"172.17.",
	"172.18.",
	"172.19.",
	"172.20.",
	"172.21.",
	"172.22.",
	"172.23.",
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
			if !ip4.IsPrivate() || ip4.IsLoopback() || isDockerBridge(ip4) {
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

func isDockerBridge(ip net.IP) bool {
	s := ip.String()
	for _, prefix := range dockerBridgePrefixes {
		if strings.HasPrefix(s, prefix) {
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
