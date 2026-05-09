# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

### Backend (Go 1.24)
- Run dev server with hot reload: `air` (config in `.air.toml`; rebuilds to `./tmp/main`, ignores `web/front`)
- Run directly: `go run main.go`
- Build: `go build -o ./tmp/main .`
- All tests (matches CI): `go test -v -race -coverprofile=coverage.out -covermode=atomic ./...`
- Single package: `go test -v -race ./dns/...`
- Single test: `go test -v -run TestName ./dns`
- The backend uses CGO (sqlite3); a C toolchain must be available locally and Docker builds use `golang:1.24-alpine` with `gcc musl-dev`.

### Frontend (Nuxt 4 + Vue 3, in `web/front/`)
- Install: `cd web/front && npm install`
- Dev server: `npm run dev`
- Build: `npm run build`

### Docker
- Full stack (backend + frontend + Prometheus + Grafana): `docker compose up --build`
- Production-style rebuild from a tag: `./build.sh <tag>` (note: this script hardcodes `/home/balamut/projects/dns-filter` and is intended for the deploy host, not local use)

### Smoke-test the DNS server
- `./create-dns-request.sh` runs `dig` against `192.168.88.88` for several record types — adjust the IP for your environment.

## Architecture

This is a sinkhole DNS server with a management UI. The backend is a single Go binary that serves DNS on `:53` (UDP+TCP) and an HTTP management API on `:8080`; optional Prometheus metrics on `:2112`. Persistence is SQLite via GORM. The deeper architectural notes (in Russian) live in `ARCHITECTURE.md` — read it for the full domain model and component diagrams; the points below are what you need to navigate the code.

### Entry point and startup ordering (`main.go`)
Order matters and is non-obvious:
1. `migrate.Migrate()` — schema migrations
2. `source.Sync()` — pulls block lists from external sources (Steven Black, EasyList) into the DB. **Panics on failure**, so the process won't start without network access on first run.
3. `filter.UpdateFilterFromDb()` — populates the in-memory bloom filter from the DB. Must run before the DNS server accepts traffic.
4. `clients.UpdateClients()` — loads the IP-exclusion list into memory.
5. Background goroutines: `blocked_domain.ClearOldEvent`, `allow_domain.ClearOldEvent`, `suggest_to_block.StartCollectSuggest` (12h cron).
6. `web.CreateServer()` returns immediately because `r.Run(":8080")` is launched inside a goroutine — only `s.Serve()` blocks.

### DNS request path (`dns/`)
Per-query flow, matching the diagrams in `ARCHITECTURE.md`:
1. Lookup client IP in `clients` exclusion map → if excluded, skip filtering.
2. `filter.CheckExist(domain)` — three-tier check designed to keep the hot path off the DB:
   - **Bloom filter** (`filter/filter/`): O(1), 10M elements, 0.1% false-positive rate, in-memory.
   - **LRU cache** (`filter/cache/`, capacity 1500): only consulted on a bloom hit, caches the DB verdict.
   - **SQLite** (`blocked-domain/`): authoritative check, only reached on bloom hit + cache miss.
3. Blocked → respond NXDOMAIN. Allowed → check `dns-cache` (LRU 1500), else forward to upstream **DoH** (default Cloudflare `https://cloudflare-dns.com/dns-query`, with bootstrap IPs to avoid chicken-and-egg DNS resolution of the DoH host).
4. Block/allow events are emitted asynchronously via channel-backed workers (`*EventStore` types in `blocked-domain` / `allow-domain`) — never block the DNS reply on a DB write.

### Cross-cutting conventions
- **Singletons via `sync.Once`** for the bloom filter, logger, DNS cache, and `config.GetConfig()`. Don't construct second instances; use the getters.
- **Channel-based async logger** (`logger/`) with pluggable handlers (console, Loki). Logging never blocks the DNS path.
- **`config.GetConfig().Enabled`** is a runtime flag toggled via `POST /api/filter/change-status`; it gates filtering globally, so the DNS path always re-reads it.
- Module package layout repeats `<feature>/{db,business,web}` (see `blocked-domain/`, `allow-domain/`, `clients/`, `suggest-to-block/`, `source/`) — `web/` packages are mounted by `web/server.go`, which is the single registry of HTTP routes.

### Configuration
All config comes from env vars (loaded from `.env` if present). The full list lives in `config/config.go`; the most load-bearing ones:
- `DNS_FILTER_DOH_UPSTREAM` (default `https://cloudflare-dns.com/dns-query`) — also accepts a legacy `DNS_FILTER_UPSTREAM` if it starts with `http(s)://`.
- `DNS_FILTER_DOH_BOOTSTRAP_IPS` (default `1.1.1.1,1.0.0.1`) — IPs for the DoH host so the resolver can bootstrap without system DNS.
- `DNS_FILTER_DBPATH` (default `./filter.sqlite`; in `.env` it's `./data/filter.sqlite` to match the Docker volume mount).
- `DNS_FILTER_METRIC_ENABLE`, `DNS_FILTER_METRIC_PORT`, `DNS_FILTER_LOG_LEVEL`.
