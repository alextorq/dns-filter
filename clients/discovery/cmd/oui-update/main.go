// oui-update fetches the IEEE OUI registry CSV and writes it as the
// `XX:XX:XX  Vendor` text file consumed by clients/discovery/oui.go.
//
// Run from the repo root:
//
//	go run ./clients/discovery/cmd/oui-update > clients/discovery/oui.txt
//
// The default source is https://standards-oui.ieee.org/oui/oui.csv. Override
// with -url for testing or to use a mirror. Only MA-L (24-bit) blocks are
// emitted; MA-M / MA-S 28- and 36-bit assignments would need a different
// lookup shape and are skipped — the loader keys on the first 24 bits only.
package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
)

const defaultURL = "https://standards-oui.ieee.org/oui/oui.csv"

func main() {
	url := flag.String("url", defaultURL, "IEEE OUI CSV URL")
	flag.Parse()

	rows, err := fetch(*url)
	if err != nil {
		fmt.Fprintln(os.Stderr, "fetch:", err)
		os.Exit(1)
	}
	if err := write(os.Stdout, rows, *url); err != nil {
		fmt.Fprintln(os.Stderr, "write:", err)
		os.Exit(1)
	}
}

type entry struct {
	prefix string
	vendor string
}

func fetch(url string) ([]entry, error) {
	client := &http.Client{Timeout: 60 * time.Second}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	// IEEE's CDN returns 418 to requests without a recognisable User-Agent.
	req.Header.Set("User-Agent", "dns-filter-oui-update/1.0 (+https://github.com/alextorq/dns-filter)")
	req.Header.Set("Accept", "text/csv,*/*")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %s", resp.Status)
	}
	return parse(resp.Body)
}

func parse(r io.Reader) ([]entry, error) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1
	header, err := cr.Read()
	if err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}
	col := func(name string) int {
		for i, h := range header {
			if strings.EqualFold(strings.TrimSpace(h), name) {
				return i
			}
		}
		return -1
	}
	regCol := col("Registry")
	asnCol := col("Assignment")
	orgCol := col("Organization Name")
	if regCol < 0 || asnCol < 0 || orgCol < 0 {
		return nil, fmt.Errorf("missing expected columns in %v", header)
	}

	var entries []entry
	for {
		row, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("row: %w", err)
		}
		if row[regCol] != "MA-L" {
			continue
		}
		asn := strings.TrimSpace(row[asnCol])
		if len(asn) != 6 {
			continue
		}
		org := strings.TrimSpace(row[orgCol])
		// Some IEEE rows are blank or "Private"; emitting those would just
		// produce noise in the UI without identifying the device.
		if org == "" || strings.EqualFold(org, "Private") {
			continue
		}
		prefix := strings.ToUpper(asn[0:2] + ":" + asn[2:4] + ":" + asn[4:6])
		entries = append(entries, entry{prefix: prefix, vendor: trimVendor(org)})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].prefix < entries[j].prefix })
	return entries, nil
}

// trimVendor collapses internal whitespace runs and strips trailing
// punctuation that occasionally appears in the IEEE Organization Name field
// (extra commas, stray quotes). The vendor string is meant for direct UI
// display, so a tidy single-line value matters more than fidelity to IEEE
// formatting.
func trimVendor(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	s = strings.Trim(s, " \t,;\"'")
	return s
}

func write(w io.Writer, entries []entry, source string) error {
	header := fmt.Sprintf(`# MAC OUI prefix → vendor lookup. Used by discovery to label discovered
# devices. Each line is:
#
#   XX:XX:XX  Vendor display name
#
# This file is generated from the IEEE OUI registry — do not edit by hand.
# Regenerate with:
#
#   go run ./clients/discovery/cmd/oui-update > clients/discovery/oui.txt
#
# Source: %s
# Entries: %d (MA-L only; locally-administered MACs are handled in code)

`, source, len(entries))
	if _, err := io.WriteString(w, header); err != nil {
		return err
	}
	for _, e := range entries {
		if _, err := fmt.Fprintf(w, "%s  %s\n", e.prefix, e.vendor); err != nil {
			return err
		}
	}
	return nil
}
