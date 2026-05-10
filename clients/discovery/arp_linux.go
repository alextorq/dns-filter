//go:build linux

package discovery

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"net"
	"net/netip"
	"os"
	"strings"
	"time"

	"github.com/mdlayher/arp"
)

// procNetARP is the path the kernel exposes the ARP cache at. Override only
// for tests.
const procNetARP = "/proc/net/arp"

// ReadARPTable parses /proc/net/arp. The format is space-separated and stable:
//
//	IP address       HW type     Flags       HW address            Mask     Device
//	192.168.1.10     0x1         0x2         e8:de:27:d2:97:5e     *        eth0
//
// Rows with all-zero MAC are incomplete (kernel knows the IP but never got
// an ARP reply) and skipped. The first line is a header.
func ReadARPTable() ([]ARPEntry, error) {
	f, err := os.Open(procNetARP)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", procNetARP, err)
	}
	defer f.Close()

	var entries []ARPEntry
	scanner := bufio.NewScanner(f)
	first := true
	for scanner.Scan() {
		if first {
			first = false
			continue
		}
		fields := strings.Fields(scanner.Text())
		if len(fields) < 4 {
			continue
		}
		ip := net.ParseIP(fields[0]).To4()
		if ip == nil {
			continue
		}
		mac, err := net.ParseMAC(fields[3])
		if err != nil || isZeroMAC(mac) {
			continue
		}
		entries = append(entries, ARPEntry{IP: ip, MAC: mac, Source: "arp-table"})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read %s: %w", procNetARP, err)
	}
	return entries, nil
}

func isZeroMAC(mac net.HardwareAddr) bool {
	return bytes.Equal(mac, net.HardwareAddr{0, 0, 0, 0, 0, 0})
}

// activeARPScan broadcasts an ARP REQUEST for every host in the subnet and
// reads replies for the duration of ctx. Requires CAP_NET_RAW (or root) on
// Linux; without it arp.Dial returns a permission error and the caller falls
// back to whatever ReadARPTable already produced.
//
// Sends are paced lightly to avoid overrunning the kernel's outbound queue
// on cheap routers; a /24 (~253 IPs) finishes the send phase in well under
// 100ms even with the per-IP sleep, and reads continue until ctx expires.
func activeARPScan(ctx context.Context, subnet *LocalSubnet) ([]ARPEntry, error) {
	iface, err := net.InterfaceByName(subnet.Interface)
	if err != nil {
		return nil, fmt.Errorf("interface %s: %w", subnet.Interface, err)
	}
	client, err := arp.Dial(iface)
	if err != nil {
		return nil, fmt.Errorf("arp dial: %w", err)
	}
	defer client.Close()

	hosts := subnet.EnumerateHosts()

	// Read deadline so the loop below never blocks past ctx; we re-arm it
	// after every read to keep using up to ctx's remaining time.
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(2 * time.Second)
	}

	// Send phase. We don't wait for replies between requests — the kernel
	// queues responses while we keep sending.
	go func() {
		for _, ip := range hosts {
			if ctx.Err() != nil {
				return
			}
			addr, ok := netip.AddrFromSlice(ip.To4())
			if !ok {
				continue
			}
			_ = client.Request(addr.Unmap())
			time.Sleep(time.Millisecond) // pace
		}
	}()

	// Read phase. We dedupe by IP because a host that already had us in its
	// ARP cache will reply almost immediately and a passive read can also
	// have surfaced it.
	seen := make(map[string]struct{})
	var entries []ARPEntry
	for {
		if err := client.SetReadDeadline(deadline); err != nil {
			break
		}
		pkt, _, err := client.Read()
		if err != nil {
			// Deadline reached or socket error — we're done.
			break
		}
		if pkt.Operation != arp.OperationReply {
			continue
		}
		ip := pkt.SenderIP.AsSlice()
		ipKey := pkt.SenderIP.String()
		if _, dup := seen[ipKey]; dup {
			continue
		}
		seen[ipKey] = struct{}{}
		entries = append(entries, ARPEntry{
			IP:     net.IP(ip),
			MAC:    append(net.HardwareAddr(nil), pkt.SenderHardwareAddr...),
			Source: "active-scan",
		})
	}
	return entries, nil
}

// runARPDiscovery reads the kernel ARP cache and runs an active broadcast
// scan in parallel-ish (passive read first since it's instant, then active
// during the remaining ctx budget). Errors from either phase are collected
// into the result rather than returned, so a missing capability still lets
// the passive entries reach the UI.
func runARPDiscovery(ctx context.Context, subnet *LocalSubnet) scanResult {
	var res scanResult

	if passive, err := ReadARPTable(); err != nil {
		res.Errors = append(res.Errors, fmt.Errorf("arp table: %w", err))
	} else {
		res.Entries = append(res.Entries, passive...)
	}

	if active, err := activeARPScan(ctx, subnet); err != nil {
		res.Errors = append(res.Errors, fmt.Errorf("active scan: %w", err))
	} else {
		// Merge with passive: prefer active-scan source for a freshly-seen
		// reply, but don't drop passive entries that didn't appear actively.
		seen := make(map[string]int)
		for i, e := range res.Entries {
			seen[e.IP.String()] = i
		}
		for _, e := range active {
			if idx, ok := seen[e.IP.String()]; ok {
				res.Entries[idx] = e
			} else {
				res.Entries = append(res.Entries, e)
			}
		}
	}
	return res
}
