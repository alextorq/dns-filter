# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Working agreements

These rules are mandatory for every change in this repo:

- **Test-driven development.** Write tests before (or alongside) the implementation. Every test suite for a new piece of behavior must include both the happy path (positive case) and at least one failure / edge case (negative case): bad input, upstream error, empty result, unauthorized access — whichever is relevant. A change without negative tests is not done.
- **Regenerate Swagger + frontend client on any API/type change.** When you touch a Go handler, its request/response types, or any struct exposed in the OpenAPI schema, you must run **both** steps in order:
  1. `swag init -g main.go -o ./docs --parseDependency` — regenerates `docs/{docs.go,swagger.json,swagger.yaml}` from the `// @Summary / @Router / ...` annotations.
  2. `cd web/front && npm run generate:api` — regenerates the typed Axios client in `web/front/app/api/generated/` from the freshly produced `docs/swagger.json`.

  Skipping step 2 silently leaves the frontend on a stale contract. Commit the regenerated files together with the change that triggered them.
- **Update documentation when you ship a feature.** A new feature or behavioral change is incomplete until the docs match it. Touch the relevant files in the same PR: `README.md` for user-visible behavior and endpoints, `ARCHITECTURE.md` for new components or changes to the request flow / startup ordering / cross-cutting conventions, and `CLAUDE.md` (this file) when a rule, command, or convention itself changes.

## Commands

### Backend (Go 1.25)
- Run dev server with hot reload: `air` (config in `.air.toml`; rebuilds to `./tmp/main`, ignores `web/front`)
- Run directly: `go run main.go`
- Build: `go build -o ./tmp/main .`
- All tests (matches CI): `go test -v -race -coverprofile=coverage.out -covermode=atomic ./...`
- Single package: `go test -v -race ./dns/...`
- Single test: `go test -v -run TestName ./dns`
- The backend is **pure Go (no CGO)** — SQLite is the `github.com/glebarez/sqlite` GORM driver, backed by `modernc.org/sqlite` (see `db/connect.go`). No C toolchain is needed; `Dockerfile.backend` builds with `CGO_ENABLED=0` on `golang:1.26-alpine` and installs no `gcc`/`musl-dev`.

### Frontend (Nuxt 4 + Vue 3, in `web/front/`)
- **Requires Node ≥ 22.** CI pins Node 22 (`.github/workflows/test.yml`), and the ESLint 10 toolchain (`eslint-flat-config-utils`) calls `Object.groupBy`, which only exists on Node 21+ — `npm run lint` crashes with `TypeError: Object.groupBy is not a function` on Node 20. The Docker frontend build stage uses `node:26-alpine`. If you bump the CI Node version, keep it ≥ 22.
- Install: `cd web/front && npm install`
- Dev server: `npm run dev`
- Build: `npm run build`
- Lint: `npm run lint` (also runs in CI — keep it green; config in `web/front/eslint.config.mjs`)

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
2. `filterModule.UpdateFromDb()` — populates the in-memory bloom filter from whatever the DB **already holds**. Runs before the DNS server accepts traffic, so a restart serves the previous block list immediately. On a genuine first run the DB is empty and nothing is blocked until the background sync (step 5) completes — the deliberate trade-off for a non-blocking start. **Panics on failure** (a failed local DB read means the DB is broken).
3. `clients.Sync()` — loads the IP-exclusion list into memory (local DB read, no network).
4. Background goroutines: `traffic` retention prune (daily — `traffic_prune.Run`, the sole retention task now that the per-table `clear-events` are gone), `suggestModule.Start` (12h cron), `authBusiness.ClearExpiredSessions`, **always launched** `suggest_inspect.Worker.Start` + `StartPrune` (reputation-enrichment of weak suggest candidates — `Worker.RunOnce` is a no-op until the DB-backed `suggest_inspect_enabled` toggle is on, and the VT/SB keys are themselves DB-backed `secret` settings; see `suggest-to-block/inspect/` and ARCHITECTURE.md §11). The inspect goroutines **and the daily `traffic_prune.Run` retention prune** are spawned **after** `settingsModule.HydrateAll()` so the atomics they read (the inspect toggle/keys, and `traffic_retention_days`) already reflect the DB-override on the first tick — the prune's first run is immediate, and pruning with the pre-hydrate seed could hard-delete rows a larger override meant to keep (the atomic is seeded to a `0` "unset" sentinel that `pruneTaskAt` skips, as belt-and-suspenders). **LAN mode only:** `arpwatcher.Run` (IP↔MAC cache) and `hostnames.Collector.Run` (10-min mDNS sweep that learns MAC→hostname into the `host_names` table to enrich device names in the traffic dashboard — see `clients/hostnames/`).
5. `backgroundSync` goroutine — `sourceModule.Sync()` pulls block lists from external sources (Steven Black, EasyList, …) into the DB, then re-runs `filterModule.UpdateFromDb()` to rebuild the bloom and clear the verdict cache. It runs **in the background** so the DNS server starts without waiting on network I/O. It never panics — a panic here would kill an already-serving DNS server; instead a failed sync (typically no network on first boot) is retried with exponential backoff (`syncRetryBaseDelay` → `syncRetryMaxDelay`) until it succeeds.
6. `web.CreateServer()` returns immediately because `r.Run(":8080")` is launched inside a goroutine — only `dnsServer.Serve()` blocks.

### DNS request path (`dns/`)
Per-query flow, matching the diagrams in `ARCHITECTURE.md`:
1. Lookup client IP in `clients` exclusion map → if excluded, skip filtering.
2. `filter.CheckExist(domain)` — three-tier check designed to keep the hot path off the DB:
   - **Bloom filter** (`filter/filter/`): O(1), 10M elements, 0.1% false-positive rate, in-memory.
   - **LRU cache** (`filter/cache/`, capacity 1500): only consulted on a bloom hit, caches the DB verdict.
   - **SQLite** (`blocked-domain/`): authoritative check, only reached on bloom hit + cache miss.
3. Blocked → respond NXDOMAIN. Allowed → check `dns-cache` (LRU 1500), else forward to upstream **DoH** (default Cloudflare `https://cloudflare-dns.com/dns-query`, with bootstrap IPs to avoid chicken-and-egg DNS resolution of the DoH host).
4. Each verdict (blocked or allowed) is recorded asynchronously into the unified `domain_traffic` counter — one row per device + domain + verdict + day — via the `TrafficRecorder` port (`dns/server.go`) backed by `traffic/business/use-cases/record`'s channel-backed aggregator (drop-on-full, batched additive upsert). Never block the DNS reply on a DB write. On a failed `UpsertBatch` the aggregator **retains** the in-RAM buffer and retries on the next flush instead of dropping the counts (bounded by `capacity` — new keys are shed once full). This replaced the two legacy event tables/stores.

### Cross-cutting conventions
- **Singletons via `sync.Once`** for the bloom filter, logger, DNS cache, and `config.GetConfig()`. Don't construct second instances; use the getters.
- **Channel-based async logger** (`logger/`) with pluggable handlers (console, Loki). Logging never blocks the DNS path.
- **`config.GetConfig().Enabled`** is a runtime flag toggled via `POST /api/filter/change-status`; it gates filtering globally, so the DNS path always re-reads it. Its value (and the pause deadline) now persists across restarts — the filter use-cases write through `filter.PersistHook` to the settings KV table and `filter.RestoreState` reloads it at boot.
- **Persistent runtime settings (`settings/`).** Env-configurable values that should survive a restart and be changeable from the UI are declared as descriptors in `registerDynamicSettings` (`settings_wiring.go`): `{Key, Type, Enum, Default, Validate, Apply}` (e.g. `log_level`, the `cache_*` knobs, `traffic_retention_days`, `suggest_inspect_enabled`, and the `secret`-typed `virustotal_key`/`safebrowsing_key`). The module validates → persists to the `settings` KV table → applies via the descriptor's `Apply` hook; startup `HydrateAll` uses the same `Apply` path. Precedence is **DB override → env default**. Hot-path readers must read from an in-memory atomic that `Apply` writes (never the DB). **`Type: "secret"`** masks the value in `GET /api/settings` to `••••<last 4>` and tells `db/web/download.go` to strip those rows from `/api/config/db/download`; the underlying provider check (e.g. `domain-inspect/checks/virustotal.go`) reads the raw key from the same atomic each request. **To add a dynamic setting:** append one descriptor + add an atomic-backed setter on the relevant sink — no migration, no `web/server.go` edit. Admin password and boot-time-only knobs (DB path, mode, ports) stay env-only; see ARCHITECTURE.md "Три тира настроек".
- Module package layout repeats `<feature>/{db,business,web}` (see `blocked-domain/`, `traffic/`, `clients/`, `suggest-to-block/`, `source/`). The `clients/` tree also holds the LAN-only helpers `discovery/` (on-demand ARP+mDNS scan, OUI vendor lookup), `arpwatcher/` (live IP↔MAC cache), and `hostnames/` (background mDNS→`host_names` collector + its `db` store, joined into the traffic dashboard by MAC). The `traffic/` module owns the unified `domain_traffic` counter; the old `allow-domain/` module and the `blocked-domain` block-domain/clear-events use-cases were removed when the two event tables were dropped. Each feature self-registers its HTTP paths: DI features expose `(h *Handlers) RegisterRoutes(rg *gin.RouterGroup)`; package-level features expose `Register(rg *gin.RouterGroup)`; `auth/web` additionally exposes `RegisterPublic(r gin.IRouter)` for the only pre-auth endpoint (`POST /api/auth/login`). `web/server.go` owns only the cross-cutting wiring — CORS, the public/protected split, Swagger — and calls into each feature's registrar. Adding or renaming an endpoint never requires editing `web/server.go`; update the feature's routes file and adjust `web/server_test.go::expectedRoutes`.

### Configuration
All config comes from env vars (loaded from `.env` if present). The full list lives in `config/config.go`; the most load-bearing ones:
- `DNS_FILTER_DOH_UPSTREAM` (default `https://cloudflare-dns.com/dns-query`) — also accepts a legacy `DNS_FILTER_UPSTREAM` if it starts with `http(s)://`.
- `DNS_FILTER_DOH_BOOTSTRAP_IPS` (default `1.1.1.1,1.0.0.1`) — IPs for the DoH host so the resolver can bootstrap without system DNS.
- `DNS_FILTER_DBPATH` (default `./filter.sqlite`; in `.env` it's `./data/filter.sqlite` to match the Docker volume mount).
- `DNS_FILTER_METRIC_ENABLE`, `DNS_FILTER_METRIC_PORT`, `DNS_FILTER_LOG_LEVEL`.
