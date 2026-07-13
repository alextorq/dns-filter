package discovery

import (
	"context"
	"net"
	"sync"
	"time"
)

// Device is the merged view of one host on the LAN as the UI sees it.
// Fields may be empty when a particular technique didn't surface them
// (e.g. mDNS contributes Hostname but not MAC; ARP contributes MAC but not
// Hostname; OUI lookup may have no entry for an obscure NIC).
type Device struct {
	IP                string `json:"ip"`
	MAC               string `json:"mac"`
	Hostname          string `json:"hostname"`
	Vendor            string `json:"vendor"`
	Source            string `json:"source"`
	AlreadyRegistered bool   `json:"already_registered"`
}

// Result is what the web layer returns to the frontend. Errors is best-effort
// partial-failure context: the operator sees "ARP scan failed: permission
// denied" but mDNS-only entries still come through.
type Result struct {
	Devices []Device `json:"devices"`
	Total   int      `json:"total"`
	Errors  []string `json:"errors,omitempty"`
}

// DiscoverOptions tunes a single sweep. FilterDocker drops devices that live on
// the host's Docker bridges from the final result — applied once, after every
// source (ARP, mDNS, active scan) has been collected and merged, by matching
// each device's IP against the host's real bridge subnets. Surfaced in the UI
// as the "Filter Docker networks" checkbox; the HTTP handler defaults it to true.
// Callers should set it explicitly.
type DiscoverOptions struct {
	FilterDocker bool
}

// ScannerDeps are the platform operations used by one LAN scanner instance.
// Keeping them explicit lets consumers test discovery without reading host
// interfaces, opening raw ARP sockets or starting multicast DNS browses.
type ScannerDeps struct {
	Timeout          time.Duration
	FindSubnet       func() (*LocalSubnet, error)
	DiscoverARP      func(context.Context, *LocalSubnet) ([]ARPEntry, []error)
	DiscoverMDNS     func(context.Context) ([]MDNSHost, []error)
	DockerBridgeNets func() []*net.IPNet
	LookupVendor     func(string) string
}

// Scanner owns an immutable set of platform adapters. It is safe to reuse for
// concurrent UI requests because every sweep keeps its state in local values.
type Scanner struct {
	timeout          time.Duration
	findSubnet       func() (*LocalSubnet, error)
	discoverARP      func(context.Context, *LocalSubnet) ([]ARPEntry, []error)
	discoverMDNS     func(context.Context) ([]MDNSHost, []error)
	dockerBridgeNets func() []*net.IPNet
	lookupVendor     func(string) string
}

// NewScanner constructs an instance from explicit platform dependencies.
func NewScanner(deps ScannerDeps) *Scanner {
	if deps.Timeout <= 0 {
		panic("clients/discovery: scanner timeout must be positive")
	}
	if deps.FindSubnet == nil {
		panic("clients/discovery: subnet finder is required")
	}
	if deps.DiscoverARP == nil {
		panic("clients/discovery: ARP discoverer is required")
	}
	if deps.DiscoverMDNS == nil {
		panic("clients/discovery: mDNS discoverer is required")
	}
	if deps.DockerBridgeNets == nil {
		panic("clients/discovery: Docker network provider is required")
	}
	if deps.LookupVendor == nil {
		panic("clients/discovery: vendor lookup is required")
	}
	return &Scanner{
		timeout:          deps.Timeout,
		findSubnet:       deps.FindSubnet,
		discoverARP:      deps.DiscoverARP,
		discoverMDNS:     deps.DiscoverMDNS,
		dockerBridgeNets: deps.DockerBridgeNets,
		lookupVendor:     deps.LookupVendor,
	}
}

// NewDefaultScanner assembles the production scanner from package-owned
// platform adapters. The factory performs no I/O; host access begins only when
// Discover is called.
func NewDefaultScanner() *Scanner {
	return NewScanner(ScannerDeps{
		Timeout:    5 * time.Second,
		FindSubnet: FindLocalSubnet,
		DiscoverARP: func(ctx context.Context, subnet *LocalSubnet) ([]ARPEntry, []error) {
			res := runARPDiscovery(ctx, subnet)
			return res.Entries, res.Errors
		},
		DiscoverMDNS: func(ctx context.Context) ([]MDNSHost, []error) {
			entries, errs := runMDNSDiscovery(ctx)
			return toMDNSHosts(entries), errs
		},
		DockerBridgeNets: dockerBridgeNets,
		LookupVendor:     LookupVendor,
	})
}

// Discover runs the LAN sweep and returns merged results. The default scanner
// budget is short on purpose — discovery is invoked synchronously from a UI
// button click, so a hard cap keeps the user from staring at a spinner if any
// technique hangs. A tighter caller deadline always wins.
func (s *Scanner) Discover(ctx context.Context, opts DiscoverOptions) (*Result, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	res := &Result{}

	subnet, err := s.findSubnet()
	if err != nil {
		// Without a subnet we can still do mDNS (it doesn't care about CIDR),
		// but no ARP scan. Record the error and continue with whatever else
		// works.
		res.Errors = append(res.Errors, err.Error())
	}

	var (
		mu       sync.Mutex
		arpRes   []ARPEntry
		arpErrs  []error
		mdnsRes  []MDNSHost
		mdnsErrs []error
		wg       sync.WaitGroup
	)

	if subnet != nil {
		wg.Go(func() {
			entries, errs := s.discoverARP(ctx, subnet)
			mu.Lock()
			arpRes = entries
			arpErrs = errs
			mu.Unlock()
		})
	}
	wg.Go(func() {
		entries, errs := s.discoverMDNS(ctx)
		mu.Lock()
		mdnsRes = entries
		mdnsErrs = errs
		mu.Unlock()
	})
	wg.Wait()

	for _, e := range arpErrs {
		res.Errors = append(res.Errors, e.Error())
	}
	for _, e := range mdnsErrs {
		res.Errors = append(res.Errors, e.Error())
	}

	res.Devices = merge(arpRes, mdnsRes, s.lookupVendor)
	if opts.FilterDocker {
		// Collect from every source first, then drop Docker in one pass: any
		// device whose IP falls in one of the host's real bridge subnets. One
		// exact check catches both ARP container neighbours and the box's own
		// mDNS self-answers on docker0/br-* (172.18.0.1, 172.24.0.1, …).
		res.Devices = filterDockerDevices(res.Devices, s.dockerBridgeNets())
	}
	res.Total = len(res.Devices)
	return res, nil
}

// filterDockerDevices drops devices whose IP falls inside one of the host's
// Docker bridge subnets (dockerNets). An empty dockerNets is a no-op, so a host
// with no Docker bridges (or a failed interface read) keeps every device.
func filterDockerDevices(devices []Device, dockerNets []*net.IPNet) []Device {
	if len(dockerNets) == 0 {
		return devices
	}
	out := make([]Device, 0, len(devices))
	for _, d := range devices {
		if ip := net.ParseIP(d.IP); ip != nil && ipInNets(ip, dockerNets) {
			continue
		}
		out = append(out, d)
	}
	return out
}

// FilterDockerARP drops ARP entries whose IP falls inside a host Docker bridge
// subnet. The arpwatcher reads the table directly (not via Discover) and wants
// its IP↔MAC cache free of container neighbours, so it filters on every tick.
func FilterDockerARP(entries []ARPEntry) []ARPEntry {
	return filterDockerARP(entries, dockerBridgeNets())
}

func filterDockerARP(entries []ARPEntry, dockerNets []*net.IPNet) []ARPEntry {
	if len(dockerNets) == 0 {
		return entries
	}
	out := make([]ARPEntry, 0, len(entries))
	for _, e := range entries {
		if ipInNets(e.IP, dockerNets) {
			continue
		}
		out = append(out, e)
	}
	return out
}

// merge keys ARP entries and mDNS entries by IP. ARP is the source of truth
// for MAC; mDNS contributes Hostname. We also accept mDNS-only entries (an
// IP that didn't show up in ARP results — happens when active scan is gated
// off or didn't get a reply in time).
func merge(arpEntries []ARPEntry, mdnsEntries []MDNSHost, lookupVendor func(string) string) []Device {
	devices := make(map[string]*Device)

	for _, e := range arpEntries {
		ipStr := e.IP.String()
		dev, ok := devices[ipStr]
		if !ok {
			dev = &Device{IP: ipStr, Source: e.Source}
			devices[ipStr] = dev
		}
		dev.MAC = e.MAC.String()
		dev.Vendor = lookupVendor(dev.MAC)
		// Prefer the more authoritative source label when both surfaced this IP.
		if dev.Source == "arp-table" && e.Source == "active-scan" {
			dev.Source = "active-scan"
		}
	}

	for _, e := range mdnsEntries {
		ip := net.ParseIP(e.IP)
		if ip == nil {
			continue
		}
		ipStr := ip.String()
		dev, ok := devices[ipStr]
		if !ok {
			dev = &Device{IP: ipStr, Source: "mdns"}
			devices[ipStr] = dev
		}
		if dev.Hostname == "" {
			dev.Hostname = e.Hostname
		}
	}

	out := make([]Device, 0, len(devices))
	for _, d := range devices {
		out = append(out, *d)
	}
	// Sort by IP (octet-wise) so the UI table is stable across calls.
	sortByIP(out)
	return out
}

func sortByIP(devices []Device) {
	// In-place insertion sort — len is bounded by /24 ≈ 250, no need to
	// reach for sort.Slice.
	for i := 1; i < len(devices); i++ {
		for j := i; j > 0 && ipLess(devices[j].IP, devices[j-1].IP); j-- {
			devices[j], devices[j-1] = devices[j-1], devices[j]
		}
	}
}

func ipLess(a, b string) bool {
	ipA, ipB := net.ParseIP(a).To4(), net.ParseIP(b).To4()
	if ipA == nil || ipB == nil {
		return a < b
	}
	for i := range 4 {
		if ipA[i] != ipB[i] {
			return ipA[i] < ipB[i]
		}
	}
	return false
}
