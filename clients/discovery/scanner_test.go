package discovery

import (
	"context"
	"errors"
	"net"
	"reflect"
	"strings"
	"testing"
	"time"
)

func scannerTestSubnet(t *testing.T) *LocalSubnet {
	t.Helper()
	_, cidr, err := net.ParseCIDR("192.168.88.0/24")
	if err != nil {
		t.Fatalf("ParseCIDR: %v", err)
	}
	return &LocalSubnet{Interface: "lan0", SelfIP: net.ParseIP("192.168.88.1"), CIDR: cidr}
}

func TestScanner_UsesInjectedPlatformDependencies(t *testing.T) {
	subnet := scannerTestSubnet(t)
	dockerNet := mustCIDR(t, "172.18.0.0/16")
	arpCalled, mdnsCalled, dockerCalled := false, false, false

	scanner := NewScanner(ScannerDeps{
		Timeout: time.Second,
		FindSubnet: func() (*LocalSubnet, error) {
			return subnet, nil
		},
		DiscoverARP: func(ctx context.Context, got *LocalSubnet) ([]ARPEntry, []error) {
			arpCalled = true
			if got != subnet {
				t.Errorf("ARP subnet = %p, want %p", got, subnet)
			}
			if _, ok := ctx.Deadline(); !ok {
				t.Error("ARP context has no scanner deadline")
			}
			mac, _ := net.ParseMAC("aa:bb:cc:dd:ee:ff")
			return []ARPEntry{{IP: net.ParseIP("192.168.88.10"), MAC: mac, Source: "active-scan"}}, []error{errors.New("passive ARP unavailable")}
		},
		DiscoverMDNS: func(ctx context.Context) ([]MDNSHost, []error) {
			mdnsCalled = true
			if _, ok := ctx.Deadline(); !ok {
				t.Error("mDNS context has no scanner deadline")
			}
			return []MDNSHost{
				{IP: "192.168.88.10", Hostname: "phone"},
				{IP: "172.18.0.1", Hostname: "docker-host"},
			}, []error{errors.New("one mDNS service failed")}
		},
		DockerBridgeNets: func() []*net.IPNet {
			dockerCalled = true
			return []*net.IPNet{dockerNet}
		},
		LookupVendor: func(mac string) string { return "vendor:" + mac },
	})

	res, err := scanner.Discover(context.Background(), DiscoverOptions{FilterDocker: true})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if !arpCalled || !mdnsCalled || !dockerCalled {
		t.Fatalf("dependency calls: arp=%v mdns=%v docker=%v", arpCalled, mdnsCalled, dockerCalled)
	}
	if res.Total != 1 || len(res.Devices) != 1 {
		t.Fatalf("result = %+v, want one LAN device", res)
	}
	device := res.Devices[0]
	if device.IP != "192.168.88.10" || device.Hostname != "phone" || device.Vendor != "vendor:aa:bb:cc:dd:ee:ff" {
		t.Fatalf("merged device = %+v", device)
	}
	if want := []string{"passive ARP unavailable", "one mDNS service failed"}; !reflect.DeepEqual(res.Errors, want) {
		t.Fatalf("errors = %v, want %v", res.Errors, want)
	}
}

func TestScanner_SubnetFailureStillReturnsMDNS(t *testing.T) {
	arpCalled := false
	scanner := NewScanner(ScannerDeps{
		Timeout: time.Second,
		FindSubnet: func() (*LocalSubnet, error) {
			return nil, errors.New("no LAN interface")
		},
		DiscoverARP: func(context.Context, *LocalSubnet) ([]ARPEntry, []error) {
			arpCalled = true
			return nil, nil
		},
		DiscoverMDNS: func(context.Context) ([]MDNSHost, []error) {
			return []MDNSHost{{IP: "192.168.88.20", Hostname: "speaker"}}, nil
		},
		DockerBridgeNets: func() []*net.IPNet { return nil },
		LookupVendor:     func(string) string { return "" },
	})

	res, err := scanner.Discover(context.Background(), DiscoverOptions{})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if arpCalled {
		t.Fatal("ARP must not run without a subnet")
	}
	if res.Total != 1 || res.Devices[0].Hostname != "speaker" {
		t.Fatalf("mDNS partial result = %+v", res)
	}
	if len(res.Errors) != 1 || !strings.Contains(res.Errors[0], "no LAN interface") {
		t.Fatalf("errors = %v", res.Errors)
	}
}

func TestScanner_UsesEarlierOfScannerAndCallerDeadline(t *testing.T) {
	for _, tc := range []struct {
		name           string
		scannerTimeout time.Duration
		callerTimeout  time.Duration
		wantTimeout    time.Duration
	}{
		{name: "scanner timeout is earlier", scannerTimeout: time.Minute, callerTimeout: time.Hour, wantTimeout: time.Minute},
		{name: "caller deadline is earlier", scannerTimeout: time.Hour, callerTimeout: time.Minute, wantTimeout: time.Minute},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var observedDeadline time.Time
			scanner := NewScanner(ScannerDeps{
				Timeout:          tc.scannerTimeout,
				FindSubnet:       func() (*LocalSubnet, error) { return nil, nil },
				DiscoverARP:      func(context.Context, *LocalSubnet) ([]ARPEntry, []error) { return nil, nil },
				DiscoverMDNS:     func(ctx context.Context) ([]MDNSHost, []error) { observedDeadline, _ = ctx.Deadline(); return nil, nil },
				DockerBridgeNets: func() []*net.IPNet { return nil },
				LookupVendor:     func(string) string { return "" },
			})

			started := time.Now()
			ctx, cancel := context.WithDeadline(context.Background(), started.Add(tc.callerTimeout))
			defer cancel()
			if _, err := scanner.Discover(ctx, DiscoverOptions{}); err != nil {
				t.Fatalf("Discover: %v", err)
			}

			gotTimeout := observedDeadline.Sub(started)
			if delta := gotTimeout - tc.wantTimeout; delta < -time.Second || delta > time.Second {
				t.Fatalf("effective timeout = %v, want approximately %v", gotTimeout, tc.wantTimeout)
			}
		})
	}
}

func TestNewScanner_RejectsIncompleteDependencies(t *testing.T) {
	valid := ScannerDeps{
		Timeout:          time.Second,
		FindSubnet:       func() (*LocalSubnet, error) { return nil, nil },
		DiscoverARP:      func(context.Context, *LocalSubnet) ([]ARPEntry, []error) { return nil, nil },
		DiscoverMDNS:     func(context.Context) ([]MDNSHost, []error) { return nil, nil },
		DockerBridgeNets: func() []*net.IPNet { return nil },
		LookupVendor:     func(string) string { return "" },
	}

	for _, tc := range []struct {
		name string
		edit func(*ScannerDeps)
	}{
		{name: "timeout", edit: func(d *ScannerDeps) { d.Timeout = 0 }},
		{name: "subnet finder", edit: func(d *ScannerDeps) { d.FindSubnet = nil }},
		{name: "ARP discoverer", edit: func(d *ScannerDeps) { d.DiscoverARP = nil }},
		{name: "mDNS discoverer", edit: func(d *ScannerDeps) { d.DiscoverMDNS = nil }},
		{name: "Docker networks", edit: func(d *ScannerDeps) { d.DockerBridgeNets = nil }},
		{name: "vendor lookup", edit: func(d *ScannerDeps) { d.LookupVendor = nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			deps := valid
			tc.edit(&deps)
			defer func() {
				if recover() == nil {
					t.Fatal("expected incomplete scanner wiring to panic")
				}
			}()
			NewScanner(deps)
		})
	}
}

func TestScanner_InstancesUseIndependentDependencies(t *testing.T) {
	base := ScannerDeps{
		Timeout:    time.Second,
		FindSubnet: func() (*LocalSubnet, error) { return scannerTestSubnet(t), nil },
		DiscoverARP: func(context.Context, *LocalSubnet) ([]ARPEntry, []error) {
			mac, _ := net.ParseMAC("aa:bb:cc:dd:ee:ff")
			return []ARPEntry{{IP: net.ParseIP("192.168.88.10"), MAC: mac}}, nil
		},
		DiscoverMDNS:     func(context.Context) ([]MDNSHost, []error) { return nil, nil },
		DockerBridgeNets: func() []*net.IPNet { return nil },
	}
	firstDeps, secondDeps := base, base
	firstDeps.LookupVendor = func(string) string { return "first" }
	secondDeps.LookupVendor = func(string) string { return "second" }

	first, _ := NewScanner(firstDeps).Discover(context.Background(), DiscoverOptions{})
	second, _ := NewScanner(secondDeps).Discover(context.Background(), DiscoverOptions{})
	if first.Devices[0].Vendor != "first" || second.Devices[0].Vendor != "second" {
		t.Fatalf("vendors = %q, %q", first.Devices[0].Vendor, second.Devices[0].Vendor)
	}
}
