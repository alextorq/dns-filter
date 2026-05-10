package discovery

import (
	_ "embed"
	"strconv"
	"strings"
	"sync"
)

//go:embed oui.txt
var ouiData []byte

// LocallyAdministeredVendor is returned for MACs whose U/L bit is set —
// libvirt/QEMU NICs (typically fe:54:…), randomized client MACs (Android
// privacy MAC, iOS private addresses), and ad-hoc software interfaces. No
// vendor registry will ever contain these prefixes; surfacing the label
// in the UI is more useful than leaving the column blank.
const LocallyAdministeredVendor = "Locally administered"

var (
	ouiMap     map[string]string
	ouiMapOnce sync.Once
)

// LookupVendor returns the vendor display name for a MAC address, or "" if
// the OUI prefix is not in the embedded IEEE list. For locally-administered
// MACs (U/L bit set in the first octet) it returns LocallyAdministeredVendor
// instead of falling through, since those prefixes are not registry-bound.
// The lookup is case-insensitive on both colons and hex digits, and tolerates
// dash separators.
func LookupVendor(mac string) string {
	ouiMapOnce.Do(loadOUI)
	prefix := normalizePrefix(mac)
	if prefix == "" {
		return ""
	}
	if v, ok := ouiMap[prefix]; ok {
		return v
	}
	if isLocallyAdministered(prefix) {
		return LocallyAdministeredVendor
	}
	return ""
}

// isLocallyAdministered reports whether the MAC's U/L bit (bit 1 of the first
// octet) is set. Input must be the canonical "XX:XX:XX" form produced by
// normalizePrefix; non-conforming input returns false.
func isLocallyAdministered(prefix string) bool {
	if len(prefix) < 2 {
		return false
	}
	b, err := strconv.ParseUint(prefix[:2], 16, 8)
	if err != nil {
		return false
	}
	return b&0x02 != 0
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
