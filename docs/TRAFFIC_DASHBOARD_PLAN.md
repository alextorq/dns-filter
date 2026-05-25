# Per-device DNS traffic dashboard — working spec

> Working document for the `feature/per-device-traffic-dashboard` branch. It is the single
> source of truth for every implementing/reviewing agent. At Step 7 the durable parts are
> folded into `ARCHITECTURE.md` / `README.md` / `CLAUDE.md` and this file is deleted.

## Goal

A per-device analytics view: for each device on the LAN, how many DNS queries it made,
to which domains, split by **blocked vs allowed**, bucketed by **day**. Counts only — no
per-query rows. One unified counter table replaces the two existing event tables.

## Hard rules (apply to EVERY step)

- **TDD, strictly.** Write the tests first and confirm they fail, THEN implement to green.
  Every new behavior needs BOTH a happy-path test AND at least one negative/edge test
  (bad input, empty, conflict, upstream error). A change without negative tests is not done.
  This mirrors the repo rule in `CLAUDE.md`.
- **No performance regression on the DNS hot path.** The reply must never wait on a DB
  write. Recording is async (channel + drop-on-full), aggregated in RAM, flushed in batches.
- **Do not break existing consumers.** suggest-to-block, domain-inspect, and the block-stats
  endpoint keep working at every step. They depend on narrow ports — swap the implementation
  behind the port, do not change the port or the consumer.
- **Migration is staged (dual-write).** Add the new table + write path first (writing
  alongside the old stores), then repoint readers, then delete the old tables LAST. Each step
  is independently green. **No backfill of old data** — old rows are simply dropped at Step 7.
- **Lint only at the very end** (single final pass), not mid-task. Frontend needs Node ≥ 22.
- **Regenerate API contract on any Go API/type change:** `swag init -g main.go -o ./docs
  --parseDependency` then `cd web/front && npm run generate:api`. Commit generated files with
  the change.
- **Agents report back concisely** — files changed, test command + result, any deviation
  from this spec. Do NOT paste full file contents back to the orchestrator.
- Agents do **not** commit; the orchestrator commits each reviewed-green step.

## Architecture decisions (settled — do not relitigate)

### Device identity = MAC, fallback IP
The DNS hot path ALREADY resolves the client to a stable handle once per request:
`dns/server.go:184` → `lookup, identified := s.Identifier.Identify(identifier.Request{RemoteAddr: remoteAddr})`.
`identifier.IPIdentifier.Identify` (`clients/identifier/identifier.go:63`) returns
`Lookup{Kind: "mac", Value: <mac>}` when the arpwatcher cache knows the MAC for the source IP,
else `Lookup{Kind: "ip", Value: <ip>}`. MAC is stable across DHCP IP churn; IP is the courier.
**Reuse this `lookup` — never re-resolve, never add ARP cost on the hot path.**

### One table replaces `block_domain_events` + `allow_domain_events`
```
domain_traffic:
  ID         uint
  ClientKind string     // "mac" | "ip" — how the device was identified
  ClientValue string    // the MAC (preferred) or IP — THE device key
  ClientIP   string     // last IP we saw this device use — informational, for UI only
  Domain     string     // canonical FQDN
  Blocked    bool       // true = NXDOMAIN'd, false = forwarded upstream
  Day        time.Time  // midnight, server local TZ — the day bucket
  Count      int64
  LastSeen   time.Time
  // UNIQUE(ClientKind, ClientValue, Blocked, Domain, Day)  ← additive upsert target
```
This single table serves:
- new per-device dashboard — group by (ClientKind, ClientValue);
- legacy block stats — `SUM(Count) WHERE Blocked=true GROUP BY Domain` (replaces `GetEventsByDomain`/`GetEventsAmount`);
- suggest candidate pool — `DISTINCT Domain WHERE Blocked=false` (replaces `GetAllActiveFilters`);
- domain-inspect — `EXISTS(...)` for allow/block membership.

### Naming/labels are OUT OF SCOPE
The `clients` table is currently empty and unused. We do **not** write to it. The dashboard:
- lists devices from `domain_traffic` itself (works with empty `clients`);
- shows **vendor** computed live via `clients/discovery.LookupVendor(mac)` (pure, local, no DB/network);
- shows **current IP** from `domain_traffic.ClientIP` (helps tell two same-vendor devices apart);
- no rename, no client-row creation. That is a separate future feature.

### Day bucketing
`Day` = the query time truncated to midnight in the **server's local timezone**. Truncation
happens where the aggregation key is built (the Step 2 worker). Document the TZ in ARCHITECTURE.md.

### Retention
Configurable via the dynamic-settings mechanism (`registerDynamicSettings` in `settings_wiring.go`),
key `TRAFFIC_RETENTION_DAYS`, default **30**, editable in the UI. A single daily prune over
`domain_traffic` replaces the two old `clear-events` tasks.

## Key existing touchpoints (so you don't have to re-discover them)

- DB connection + pragmas (WAL, synchronous=NORMAL, modernc/glebarez driver): `db/connect.go`.
- Batch helpers (INSERT-OR-IGNORE upsert, batched insert): `db/batch.go` (`BatchUpsertOn`,
  `BatchInsertOn`). **Additive upsert (count = count + excluded.count) does NOT exist yet** —
  add it (gorm `clause.OnConflict{DoUpdates: clause.Assignments(...)}`).
- Migrations registry: `db/migrate/migrate.go`.
- EventStore pattern to mirror (channel inbox, RAM buffer, 20s ticker, capacity flush,
  drop-on-full): `blocked-domain/business/use-cases/block-domain/block-domain.go`.
- DNS request handler + verdict branch: `dns/server.go:173` `handleDNS`; `lookup` at :184;
  `clientIP` at :178-179; blocked branch calls `s.Handlers.Blocked(w,r)`, allowed branch
  `s.Handlers.Allowed(w,r)`.
- Composition root / wiring: `main.go`.
- suggest-to-block ports + Collect: `suggest-to-block/suggest_to_block.go` (`AllowRepo.GetAllActiveFilters`,
  `BlockRepo.GetAllActiveURLs`). Scoring is string-only — no counts/timestamps needed.
- Legacy block stats endpoint: `blocked-domain/web/main-events.go` (uses `GetEventsAmount` + `GetEventsByDomain`).
- domain-inspect allow/block lookup: `domain-inspect/checks/local_stats.go`.
- Vendor (OUI) read-side helper: `clients/discovery/oui.go` `LookupVendor(mac string) string` (pure).
- Feature route self-registration + the route assertion test: each feature's `web/routes.go`;
  `web/server_test.go::expectedRoutes` must be updated when routes change.
- Frontend: Nuxt 4 in `web/front/`; pages in `app/pages/`; generated client in
  `app/api/generated/` (regen via `npm run generate:api`); use Nuxt UI tokens; every page that
  calls an API must render a visible error state on failure (not a permanent skeleton).

## Steps (each: tests-first → implement → agent review → orchestrator verify → commit)

### Step 1 — Table + repo + additive upsert  [no write path yet; old tables untouched]
- New package `traffic/db`:
  - `db.go`: `DomainTraffic` model as specified above, with the composite unique index and
    secondary indexes for read queries: `(client_kind, client_value, day)` and `(day)` (prune)
    and `(blocked, day)`.
  - `repo.go`: `NewRepo(*gorm.DB) *Repo`; `UpsertBatch(rows []DomainTraffic) error` (additive:
    on conflict `count = count + excluded.count`, `last_seen = max(...)`, `client_ip = excluded.client_ip`;
    batch under SQLite's 32766-param limit, ~6 cols → batch 4000; empty input no-op);
    `DeleteOlderThan(cutoff time.Time) error` (Unscoped hard delete WHERE day < cutoff).
  - Add the additive-upsert helper (in `db/batch.go` or local to the repo).
- Register `&DomainTraffic{}` in `db/migrate/migrate.go` (additive only).
- Tests (mirror `blocked-domain/db/repo_test.go` setup against a temp SQLite file with real pragmas):
  happy — insert new; conflict ADDS count + bumps last_seen/client_ip; distinct keys → separate
  rows; DeleteOlderThan prunes old & keeps new; >param-limit batch works. Negative — empty input
  no-op for both methods; bad/zero rows handled.

### Step 2 — Write path (async aggregator) + reuse `lookup`  [dual-write]
- `traffic/business/use-cases/record/` EventStore: inbox channel of `Event{Kind, Value, IP, Domain, Blocked}`,
  in-RAM map keyed by `(Kind, Value, Blocked, Domain, Day)` → accumulates Count + tracks last IP/seen,
  flush on 20s ticker or capacity via `repo.UpsertBatch`, drop-on-full (never block hot path).
  Mirror the existing BlockDomainEventStore structure.
- Inject a narrow `TrafficRecorder` port into `DnsServer` and record inside `handleDNS` using the
  already-resolved `lookup` + `clientIP` + `qname` + the verdict (blocked branch → true, allowed →
  false). Keep the old block/allow stores writing too (dual-write). Fall back to IP identity when
  `!identified`; skip empty domain and loopback/`::1`/the server's own queries.
- Tests: aggregation collapses duplicates into Count; flush by ticker and by capacity; drop-on-full;
  day rollover → new key; Kind/Value taken from lookup (MAC preferred); IP stamped informationally.
  Negative: empty domain ignored, full channel drops without blocking, unidentified client.

### Step 3 — Repoint suggest-to-block + domain-inspect to traffic  [non-critical read adapters]
- `traffic/db` read: `GetAllowedDomains() ([]string, error)` = DISTINCT Domain WHERE Blocked=false;
  `IsAllowed(domain) (bool,error)`, `IsBlockedSeen(domain) (bool,error)` for domain-inspect.
- Provide traffic-backed adapters satisfying suggest's `AllowRepo` and domain-inspect's lookup;
  rewire in `main.go`. Ports unchanged.
- Tests: DISTINCT correctness; suggest still produces candidates from traffic data; domain-inspect
  membership. Negative: empty pool, domain absent.

### Step 4 — Block stats on traffic + dashboard read API  [no client writes]
- `traffic/db` reads: `CountByDomain(blocked bool)`, `TotalCount(blocked bool)`, device summary
  (per (kind,value): allowed/blocked totals, current IP = latest ClientIP by LastSeen, last_seen),
  per-device domains (verdict filter, date range, pagination), top domains.
- Repoint `blocked-domain/web/main-events.go` to read from traffic (`SUM WHERE blocked`) — keep the
  existing JSON response shape so the current stats page is unaffected.
- New `traffic/web/`: `GET /api/traffic/devices`, `GET /api/traffic/devices/:id/domains`,
  `GET /api/traffic/top-domains`. Enrich device rows with vendor via `LookupVendor`. Self-register
  routes; update `web/server_test.go::expectedRoutes`. Swagger annotations.
- Regenerate swagger + frontend client.
- Tests: each handler happy + negative (bad id/ip, empty, 404, unauthorized — protected group).

### Step 5 — Frontend dashboard (Nuxt UI, read-only)
- New page (e.g. `app/pages/traffic/`): device list (key = MAC/IP, vendor, current IP, allowed/blocked
  totals) → device detail (domains with counts, allowed/blocked split, date range). Optional "Scan LAN"
  button reusing existing `POST /api/clients/discover`. No rename.
- Nuxt UI tokens; visible error state on any API failure; uses the regenerated typed client.
- Tests if a vitest setup exists; otherwise rely on final lint + manual verify.

### Step 6 — Retention as a dynamic setting + UI
- Descriptor `TRAFFIC_RETENTION_DAYS` in `registerDynamicSettings` (`settings_wiring.go`): int,
  default 30, Validate > 0, Apply → atomic read by the prune task. Single daily prune goroutine over
  `domain_traffic` (replaces both old clear-events tasks once their tables are gone in Step 7).
- UI control on the settings page (number input), following the existing dynamic-settings UI pattern.
- Tests: validate rejects ≤0; persist+apply; HydrateAll at boot; prune deletes by retention, keeps newer.

### Step 7 — Remove legacy + drop tables + docs
- Delete `BlockDomainEvent` + its repo methods + `BlockDomainEventStore` + block `clear-events`;
  delete `AllowDomainEvent` + allow EventStore + allow `clear-events`; remove dual-write from `handleDNS`.
- Migration: drop `block_domain_events` and `allow_domain_events`. No backfill.
- Update `README.md` (endpoints/behavior), `ARCHITECTURE.md` (new component + day-bucket TZ + identity
  rationale), `CLAUDE.md` (if a rule/command/convention changed). Update `web/server_test.go::expectedRoutes`.
- Final: full `go test -race -coverprofile=coverage.out -covermode=atomic ./...` green + single lint pass.
