package discovery

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/grandcat/zeroconf"
)

// mDNSEntry is a hostname-by-IP fact learned from a multicast browse. We don't
// expose service types or instance names — the goal is just to put a friendly
// name next to an IP discovered via ARP.
type mDNSEntry struct {
	IP       net.IP
	Hostname string
}

// browsedServices is the set of mDNS service types we enumerate. Each one
// gets a relatively short browse window; in parallel they finish within the
// overall ctx deadline. The list is intentionally narrow: these cover the
// vast majority of consumer hardware that announces itself.
var browsedServices = []string{
	"_workstation._tcp", // most Linux distros, macOS
	"_airplay._tcp",     // Apple TV, AirPlay speakers
	"_googlecast._tcp",  // Chromecast, Google Home
	"_homekit._tcp",     // HomeKit accessories
	"_ipp._tcp",         // network printers
	"_smb._tcp",         // file shares (Synology / QNAP / Windows)
	"_raop._tcp",        // legacy AirPlay (older AirPort Express)
}

// runMDNSDiscovery browses each service type in parallel and merges the
// IP→hostname pairs. A timeout shorter than ctx is set per-browse so a slow
// service doesn't starve faster ones; partial results are returned even if
// some browses error out.
//
// Hostnames returned by zeroconf carry the trailing ".local." — we strip it
// for display.
func runMDNSDiscovery(ctx context.Context) ([]mDNSEntry, []error) {
	const perBrowseTimeout = 2 * time.Second
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(perBrowseTimeout)
	}
	if budget := time.Until(deadline); budget < perBrowseTimeout {
		// Caller's overall ctx is tighter than our default — honor it.
		_ = budget
	}

	var (
		mu      sync.Mutex
		entries []mDNSEntry
		errs    []error
		wg      sync.WaitGroup
	)

	for _, service := range browsedServices {
		wg.Go(func() {
			svc := service
			browseCtx, cancel := context.WithDeadline(ctx, deadline)
			defer cancel()

			resolver, err := zeroconf.NewResolver(nil)
			if err != nil {
				mu.Lock()
				errs = append(errs, fmt.Errorf("mdns resolver %s: %w", svc, err))
				mu.Unlock()
				return
			}
			ch := make(chan *zeroconf.ServiceEntry, 32)
			if err := resolver.Browse(browseCtx, svc, "local.", ch); err != nil {
				mu.Lock()
				errs = append(errs, fmt.Errorf("mdns browse %s: %w", svc, err))
				mu.Unlock()
				return
			}
			for entry := range ch {
				host := strings.TrimSuffix(entry.HostName, ".")
				host = strings.TrimSuffix(host, ".local")
				if host == "" {
					host = entry.Instance
				}
				if host == "" {
					continue
				}
				mu.Lock()
				for _, ip := range entry.AddrIPv4 {
					entries = append(entries, mDNSEntry{IP: ip, Hostname: host})
				}
				mu.Unlock()
			}
		})
	}

	wg.Wait()
	return entries, errs
}
