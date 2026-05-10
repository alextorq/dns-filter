package discovery

import (
	_ "embed"
	"strings"
	"sync"
)

//go:embed oui.txt
var ouiData []byte

var (
	ouiMap     map[string]string
	ouiMapOnce sync.Once
)

// LookupVendor returns the vendor display name for a MAC address, or "" if
// the OUI prefix is not in the curated list. The lookup is case-insensitive
// on both colons and hex digits, and tolerates dash separators (the format
// /proc/net/arp returns sometimes uses ':' and the active scan returns hex
// pairs).
//
// The curated list is small on purpose: matching ~80% of consumer devices is
// the goal; comprehensive vendor coverage would balloon the binary by ~3 MB
// (Wireshark's manuf file). Users with rare hardware can rename the client
// in the UI — the Vendor column is informational, not functional.
func LookupVendor(mac string) string {
	ouiMapOnce.Do(loadOUI)
	prefix := normalizePrefix(mac)
	if prefix == "" {
		return ""
	}
	return ouiMap[prefix]
}

func loadOUI() {
	ouiMap = make(map[string]string, 512)
	for line := range strings.SplitSeq(string(ouiData), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Format: "XX:XX:XX  Vendor Name" with whitespace separator.
		fields := strings.SplitN(line, " ", 2)
		if len(fields) != 2 {
			continue
		}
		prefix := normalizePrefix(fields[0])
		vendor := strings.TrimSpace(fields[1])
		if prefix == "" || vendor == "" {
			continue
		}
		ouiMap[prefix] = vendor
	}
}

// normalizePrefix returns the first 6 hex digits of the MAC, uppercased,
// colon-separated as XX:XX:XX. Returns "" if the input is too short or
// malformed.
func normalizePrefix(mac string) string {
	cleaned := strings.Map(func(r rune) rune {
		if r == ':' || r == '-' || r == '.' {
			return -1
		}
		return r
	}, mac)
	if len(cleaned) < 6 {
		return ""
	}
	cleaned = strings.ToUpper(cleaned[:6])
	return cleaned[0:2] + ":" + cleaned[2:4] + ":" + cleaned[4:6]
}
