package arpwatcher

import (
	"net"
	"testing"

	"github.com/alextorq/dns-filter/clients/discovery"
)

func mac(s string) net.HardwareAddr {
	m, err := net.ParseMAC(s)
	if err != nil {
		panic(err)
	}
	return m
}

func entry(ip, m string) discovery.ARPEntry {
	return discovery.ARPEntry{IP: net.ParseIP(ip).To4(), MAC: mac(m), Source: "test"}
}

// freshCache returns a Cache instance bypassing the package singleton so
// tests don't accidentally share state.
func freshCache() *Cache {
	return &Cache{
		ipToMAC: map[string]string{},
		macToIP: map[string]string{},
	}
}

func TestCache_NewPair(t *testing.T) {
	c := freshCache()
	res := c.Update([]discovery.ARPEntry{entry("192.168.1.10", "aa:bb:cc:dd:ee:01")})
	if res.NewPairs != 1 {
		t.Fatalf("expected 1 new pair, got %d", res.NewPairs)
	}
	if got, ok := c.MAC("192.168.1.10"); !ok || got != "aa:bb:cc:dd:ee:01" {
		t.Fatalf("MAC lookup miss: ok=%v got=%q", ok, got)
	}
	if got, ok := c.IP("aa:bb:cc:dd:ee:01"); !ok || got != "192.168.1.10" {
		t.Fatalf("IP reverse lookup miss: ok=%v got=%q", ok, got)
	}
}

// Idempotent re-feed: same entries twice → no further deltas reported.
func TestCache_RepeatedUpdateIsIdempotent(t *testing.T) {
	c := freshCache()
	es := []discovery.ARPEntry{entry("192.168.1.10", "aa:bb:cc:dd:ee:01")}
	c.Update(es)
	res := c.Update(es)
	if res.NewPairs != 0 || res.ChangedIPs != 0 || res.ChangedMACs != 0 {
		t.Fatalf("expected zero deltas on re-feed, got %+v", res)
	}
}

// DHCP rotation: same MAC moves to a different IP. The old IP→MAC entry
// must be retired so a stale lookup can't return the now-wrong MAC.
func TestCache_IPChangeForKnownMAC(t *testing.T) {
	c := freshCache()
	c.Update([]discovery.ARPEntry{entry("192.168.1.10", "aa:bb:cc:dd:ee:01")})
	res := c.Update([]discovery.ARPEntry{entry("192.168.1.99", "aa:bb:cc:dd:ee:01")})

	if res.ChangedIPs != 1 {
		t.Fatalf("expected ChangedIPs=1, got %+v", res)
	}
	if _, ok := c.MAC("192.168.1.10"); ok {
		t.Fatal("old IP→MAC mapping must be retired after DHCP rotation")
	}
	if got, ok := c.MAC("192.168.1.99"); !ok || got != "aa:bb:cc:dd:ee:01" {
		t.Fatalf("new IP→MAC mapping missing: ok=%v got=%q", ok, got)
	}
	if got, ok := c.IP("aa:bb:cc:dd:ee:01"); !ok || got != "192.168.1.99" {
		t.Fatalf("MAC→IP not updated: ok=%v got=%q", ok, got)
	}
}

// New device on an old IP: the previous MAC binding for that IP is replaced.
// This is the spurious-match scenario the MAC-trumps-IP rule guards against;
// the cache itself just needs to keep its mapping fresh.
func TestCache_MACChangeForKnownIP(t *testing.T) {
	c := freshCache()
	c.Update([]discovery.ARPEntry{entry("192.168.1.10", "aa:bb:cc:dd:ee:01")})
	res := c.Update([]discovery.ARPEntry{entry("192.168.1.10", "aa:bb:cc:dd:ee:99")})

	if res.ChangedMACs != 1 {
		t.Fatalf("expected ChangedMACs=1, got %+v", res)
	}
	if got, ok := c.MAC("192.168.1.10"); !ok || got != "aa:bb:cc:dd:ee:99" {
		t.Fatalf("MAC for IP was not updated: got=%q", got)
	}
	if _, ok := c.IP("aa:bb:cc:dd:ee:01"); ok {
		t.Fatal("old MAC→IP reverse mapping must be retired")
	}
}

func TestCache_StatsTracksKnown(t *testing.T) {
	c := freshCache()
	c.Update([]discovery.ARPEntry{
		entry("192.168.1.10", "aa:bb:cc:dd:ee:01"),
		entry("192.168.1.11", "aa:bb:cc:dd:ee:02"),
	})
	if got := c.Stats().Known; got != 2 {
		t.Fatalf("Known=2 expected, got %d", got)
	}
}

func TestCache_PairsSnapshotIsACopy(t *testing.T) {
	c := freshCache()
	c.Update([]discovery.ARPEntry{entry("192.168.1.10", "aa:bb:cc:dd:ee:01")})
	pairs := c.Pairs()
	pairs["192.168.1.10"] = "tampered"

	if got, _ := c.MAC("192.168.1.10"); got == "tampered" {
		t.Fatal("Pairs() must return a copy, not a live view of internal state")
	}
}
