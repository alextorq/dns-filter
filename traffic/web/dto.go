package web

import traffic_db "github.com/alextorq/dns-filter/traffic/db"

// ErrorResponse is a generic error payload returned by handlers.
type ErrorResponse struct {
	Message string `json:"message"`
}

// DeviceDTO is one device row in the dashboard, enriched on the read side with
// its OUI vendor (empty for ip-kind devices and unknown prefixes). client_value
// is the stable device key (MAC when known, else IP); current_ip is the most
// recent IP the device was seen using.
type DeviceDTO struct {
	ClientKind   string `json:"client_kind"`
	ClientValue  string `json:"client_value"`
	CurrentIP    string `json:"current_ip"`
	Vendor       string `json:"vendor"`
	AllowedCount int64  `json:"allowed_count"`
	BlockedCount int64  `json:"blocked_count"`
	LastSeen     string `json:"last_seen"`
}

// DevicesResponse is the device-summary list returned by GET /api/traffic/devices.
type DevicesResponse struct {
	Devices []DeviceDTO `json:"devices"`
}

// DomainCountDTO is a (domain, summed-count) pair used by the per-device and
// top-domains responses.
type DomainCountDTO struct {
	Domain string `json:"domain"`
	Count  int64  `json:"count"`
}

// DeviceDomainsResponse is a page of one device's domains plus the total count
// of distinct domains matching the filter (for pagination).
type DeviceDomainsResponse struct {
	Total int64            `json:"total"`
	List  []DomainCountDTO `json:"list"`
}

// TopDomainsResponse is the top-domains list returned by
// GET /api/traffic/top-domains.
type TopDomainsResponse struct {
	List []DomainCountDTO `json:"list"`
}

// toDomainCountDTOs projects repo DomainTotal rows onto the wire DTO.
func toDomainCountDTOs(rows []traffic_db.DomainTotal) []DomainCountDTO {
	out := make([]DomainCountDTO, len(rows))
	for i, r := range rows {
		out[i] = DomainCountDTO{Domain: r.Domain, Count: r.Count}
	}
	return out
}
