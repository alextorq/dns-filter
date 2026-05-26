```
██████╗ ███╗   ██╗███████╗      ███████╗██╗██╗  ████████╗███████╗██████╗ 
██╔══██╗████╗  ██║██╔════╝      ██╔════╝██║██║  ╚══██╔══╝██╔════╝██╔══██╗
██║  ██║██╔██╗ ██║███████╗█████╗█████╗  ██║██║     ██║   █████╗  ██████╔╝
██║  ██║██║╚██╗██║╚════██║╚════╝██╔══╝  ██║██║     ██║   ██╔══╝  ██╔══██╗
██████╔╝██║ ╚████║███████║      ██║     ██║███████╗██║   ███████╗██║  ██║
╚═════╝ ╚═╝  ╚═══╝╚══════╝      ╚═╝     ╚═╝╚══════╝╚═╝   ╚══════╝╚═╝  ╚═╝
```

[![Tests](https://github.com/alextorq/dns-filter/actions/workflows/test.yml/badge.svg)](https://github.com/alextorq/dns-filter/actions/workflows/test.yml)
[![codecov](https://codecov.io/gh/alextorq/dns-filter/branch/main/graph/badge.svg)](https://codecov.io/gh/alextorq/dns-filter)
[![Go Report Card](https://goreportcard.com/badge/github.com/alextorq/dns-filter)](https://goreportcard.com/report/github.com/alextorq/dns-filter)
[![Go version](https://img.shields.io/github/go-mod/go-version/alextorq/dns-filter)](https://github.com/alextorq/dns-filter/blob/main/go.mod)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Last commit](https://img.shields.io/github/last-commit/alextorq/dns-filter)](https://github.com/alextorq/dns-filter/commits/main)
[![GitHub stars](https://img.shields.io/github/stars/alextorq/dns-filter?style=social)](https://github.com/alextorq/dns-filter/stargazers)

it is simple dns service for block advertisement and malicious
you need seed block domains and set it server like dns in your network

![img](docs/main.png)
![img](docs/domains.png)
![img](docs/grafana-dashboard.png)



```mermaid
sequenceDiagram
autonumber
participant C as Device (Client)
participant DNS as Your DNS Server (Sinkhole)
participant Upstream as Upstream DoH (Cloudflare)
participant Web as Legitimate Website
participant Ad as Ad Server

    Note over C, Ad: Scenario 1: Requesting an allowed (legitimate) domain
    C->>DNS: DNS Query: "What is the IP of example.com?"
    DNS->>DNS: Check domain against Blocklist
    Note right of DNS: Domain NOT found (Allowed)
    DNS->>Upstream: Forward query for example.com
    Upstream-->>DNS: Response: IP address 93.184.216.34
    DNS-->>C: Response: IP address 93.184.216.34
    C->>Web: HTTP/HTTPS request to 93.184.216.34
    Web-->>C: Return content (Page loaded)

    Note over C, Ad: Scenario 2: Requesting an ad or tracking domain
    C->>DNS: DNS Query: "What is the IP of ads.domain.com?"
    DNS->>DNS: Check domain against Blocklist
    Note right of DNS: Domain FOUND in list! (Blocked)
    DNS-->>C: Response: 0.0.0.0 (or NXDOMAIN)
    C-xAd: Connection attempt to 0.0.0.0 fails
    Note left of Ad: Ad or tracker fails to load
```

## Architecture

The DNS path uses a three-tier check designed to keep the hot path off the database — most queries are answered without ever touching SQLite. Every verdict (blocked or allowed) is recorded asynchronously into the unified per-device `domain_traffic` counter — one row per device + domain + verdict + day — so DB writes never block the DNS reply.

```mermaid
flowchart TD
    Q[DNS Query] --> CK{Client in<br/>exclusion list?}
    CK -->|no| BF{Bloom filter<br/>10M elements<br/>0.1% false-positive}
    BF -->|miss| DC{Response cache<br/>LRU 1500}
    CK -->|yes| DC
    BF -->|hit| LRU{Verdict cache<br/>LRU 1500}
    LRU -->|allowed| DC
    LRU -->|blocked| NX[NXDOMAIN]
    LRU -->|miss| DB[(SQLite<br/>blocked_domain)]
    DB -->|found| NX
    DB -->|not found| DC
    DC -->|hit| RESP[DNS response]
    DC -->|miss| DOH[DoH upstream<br/>Cloudflare]
    DOH --> RESP
    NX -. async .-> TR[(domain_traffic<br/>per-device counter)]
    RESP -. async .-> TR
```

## Features

- DNS filtering with block lists
- DNS-over-HTTPS upstream resolver with singleflight coalescing and
  stale-while-revalidate caching (RFC 8767) — keeps answering during short
  Cloudflare/DoH blips and absorbs thundering-herd on TTL expiry
- Web-based management interface (Vue.js frontend)
- RESTful API (Go backend)
- Domain inspection: reputation, registration age, certificate transparency,
  VirusTotal, urlscan.io, and Google Safe Browsing — aggregated into a single
  verdict before adding a domain to the block list (`/inspect` page, `GET
  /api/domain/inspect`)
- Suggest-to-block heuristic collector (runs every 12h) — flags suspicious
  allowed domains for review. High-confidence candidates (score ≥ 60, or any
  subdomain-of-already-blocked) are auto-promoted to the block list with
  `Source = AutoBlocked`; the rest go to `suggest_blocks` for manual
  approval via the UI. Auto-promotion can be turned off by toggling the
  `AutoBlocked` source on the Sources page — disabled candidates fall through
  to `suggest_blocks` instead. See [ARCHITECTURE.md §11](ARCHITECTURE.md) for
  the scoring rules.
- Per-device traffic dashboard (`/traffic` page) — for each device on the LAN,
  how many DNS queries it made, to which domains, split by **blocked vs
  allowed** and bucketed by **day** (local-midnight). Devices are keyed by their
  **MAC** (with the OUI vendor shown), falling back to IP, so a device stays the
  same row across DHCP IP churn. Read-only, backed by the unified
  `domain_traffic` counter (counts only, no per-query rows) via:
  - `GET /api/traffic/devices` — per-device allowed/blocked totals, current IP,
    vendor and last-seen (optional `from`/`to` day range, `YYYY-MM-DD`);
  - `GET /api/traffic/devices/domains` — the domains a single device queried,
    with summed counts (device picked by `kind`+`value` query params; optional
    `blocked` verdict, `from`/`to`, `limit`/`offset`);
  - `GET /api/traffic/top-domains` — highest-traffic domains across all devices
    (optional `blocked`, `limit`).

  How long counters are retained is the `traffic_retention_days` runtime setting
  (env seed `DNS_FILTER_TRAFFIC_RETENTION_DAYS`, default **30**, range 1..3650),
  editable in the UI without a restart; a single daily prune drops rows older
  than the window.
- Block-stats metrics (Prometheus) — the `/api/events/block/*` endpoints now
  aggregate `SUM(count)` from `domain_traffic` (blocked scope); the legacy
  `block_domain_events`/`allow_domain_events` tables were removed.
- Persistent runtime settings from the Settings page (`GET/PUT/DELETE
  /api/settings`) — log level, DoH upstream + bootstrap IPs, the cache
  tuning knobs (SWR on/off, stale grace/TTL, refresh concurrency) and the
  traffic retention window can be changed without a restart and **survive
  one**. The value is stored in the DB and overrides the env default once set;
  `DELETE /api/settings/{key}` reverts a setting to env control. Env vars
  remain the seed/default. The filter on/off + pause state also persists, so a
  deliberately disabled or paused filter stays that way across a restart. See
  [ARCHITECTURE.md §12](ARCHITECTURE.md) for the design and the
  static/dynamic/secret classification of every setting.
- Manual DNS-cache flush from the Settings page (`POST /api/dns-cache/clear`) —
  drops every entry in the in-memory response cache, useful after rotating
  upstream records with a long TTL
- SQLite database for persistent storage
- Dockerized deployment

### Optional API keys for domain inspection

The inspect endpoint runs a fan-out of independent checks. Four of them are
always on (no setup); three are gated by third-party API keys and degrade
gracefully — without a key the check returns `status: skipped` and the
aggregated verdict is computed from the remaining signals. **All three are
free** for personal use (Safe Browsing has a non-commercial restriction).

| Env var                        | Service                                            | Free tier            |
| ------------------------------ | -------------------------------------------------- | -------------------- |
| `DNS_FILTER_VT_KEY`            | [VirusTotal](https://www.virustotal.com)           | 4 req/min, 500/day   |
| `DNS_FILTER_URLSCAN_KEY`       | [urlscan.io](https://urlscan.io)                   | ~1000 searches/day   |
| `DNS_FILTER_SAFE_BROWSING_KEY` | [Google Safe Browsing v4](https://developers.google.com/safe-browsing/v4) | Generous, non-commercial only |

Step-by-step signup, scoring rules per provider, troubleshooting, and
verification instructions live in **[docs/inspect-keys.md](docs/inspect-keys.md)**.

Keys live in `.env` (see `.env.example` for the template). The file is
git-ignored, so secrets don't end up in the repo.

## Getting Started

### Prerequisites
- Go 1.20+
- Node.js & npm (for frontend)
- Docker (optional)

### Backend Setup
1. Install Go dependencies:
   ```sh
   go mod tidy
   ```
2. Run the backend server:
   ```sh
   go run main.go
   ```

### Frontend Setup
1. Navigate to the frontend directory:
   ```sh
   cd web/front
   ```
2. Install dependencies:
   ```sh
   npm install
   ```
3. Start the frontend server:
   ```sh
   npm run dev
   ```

### Docker Deployment
1. Build and start all services:
   ```sh
   docker-compose up --build
   ```

## Monitoring & Logging
- Prometheus metrics endpoint (`:2112/metrics`, toggled by `DNS_FILTER_METRIC_ENABLE`):
  DNS/cache counters, Go runtime + process metrics (goroutines, heap, GC, CPU, FDs),
  and database metrics — per-operation query latency (`db_query_duration_seconds`),
  query errors (`db_query_errors_total`), connection-pool stats (`go_sql_*`) and
  SQLite file size.
- Loki logging integration
- Grafana dashboards in `grafana/dashboards/` (provisioned via `grafana/provisioning/`):
  `dns-filter.json` (DNS & cache) and `runtime-db.json` (container runtime & database health).


## License

MIT

---
*Generated on September 26, 2025*
