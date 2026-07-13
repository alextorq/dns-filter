# План чистки архитектуры

Документ дополняет `ARCHITECTURE.md` (описание системы как есть) — здесь
зафиксирован прогресс рефакторинга и ближайшие шаги, чтобы следующий
коммитящий не начинал с нуля.

---

## Архитектура сейчас (после core DI и unified traffic)

DNS-фильтр — single-binary Go-сервис: DNS на `:53` (UDP+TCP), HTTP API на
`:8080`, опциональные Prometheus-метрики на `:2112`. SQLite через GORM.

**Composition root — `main.go`.** Он загружает конфиг, создаёт logger и
Prometheus registry, конструирует component metrics bundles и один раз вызывает
`db.Open(OpenDeps{Path, Log, Metrics, DBName})`. Дальше каждая фича получает
явные зависимости:

```
main.go
├── *gorm.DB ─────┬─ blocked_domain_db.NewRepo(conn) ── blockRepo
│                 ├─ traffic_db.NewRepo(conn) ───────── trafficRepo
│                 ├─ source_db.NewRepo(conn) ────────── sourceRepo
│                 ├─ suggest_to_block_db.NewRepo(conn)  suggestRepo
│                 └─ settings_db.NewRepo(conn) ───────── settingsRepo
│
├── filter.NewModule(blockRepo, bloom, cache, conf, log)  → filterModule
├── source.NewModule(sourceRepo, blockRepo, sourceLoaders, log)
│                                                        → sourceModule
└── suggest_to_block.NewModule(blockRepo, trafficAllowAdapter,
        sourceRepo, filterModule, suggestRepo, log)       → suggestModule
                            │
                            ├── dns.NewServer(ServerDeps{Filter: filterModule.CheckExist, ...})
                            └── web.NewServer(":8080", web.Handlers{
                                    Blocked, Filter, Suggest, Source})
```

**Use-case'ы** (`*/business/use-cases/*`) — функции от узких output-портов,
объявленных рядом с потребителем. `*Repo` удовлетворяет всем портам через
structural typing — «accept interfaces, return structs». Тесты use-case'ов
гоняются на фейках без sqlite; репозитории покрыты отдельными тестами на
in-memory `:memory:`-sqlite.

**HTTP-handlers** — структуры с полями-зависимостями
(`*/web.Handlers{Repo, Module, Filter, Log, …}`). `web.NewServer` принимает
их пакетом, не читает singleton'ов и не открывает listener; server lifecycle
принадлежит `main`.

**DNS hot path** — `Module.CheckExist`:
1. `State.Enabled()` (atomic) — глобальный toggle.
2. `State.PausedUntil()` (atomic) — активная пауза.
3. `Bloom.DomainExist` — O(1), 10M элементов, 0.1% FP.
4. `Cache.Get` — LRU 1500 элементов, только при bloom-hit.
5. `Repo.IsActivelyBlocked` — авторитетная проверка с учётом `Active=true`.
   На любую DB-ошибку — fail-open (false), без записи в кэш (#25).

Bloom (`filter/filter`) и verdict LRU (`filter/cache`) теперь создаются
обычными конструкторами в `main.go`; package-level singleton state в них
удалён. `config.Load()` каждый раз строит свежий набор boot-настроек, а mutable
filter state создаётся отдельно через `runtime_state.New(true)`; DB и logger
создаются явными конструкторами в `main`. **Все
зависимости впитываются `*Module` в `main.go`** — фичи их сами не вызывают.
`domain-inspect/checks/local_stats.go`
тоже больше не читает singleton-коннекшен: check создаётся через
`NewLocalStats(blockRepo, trafficRepo)` в composition root.

---

## Что сделано

### Этап 1 (коммит `dda1916`) — пилот DI в `blocked-domain`

- `blocked-domain/db.Repo` — конкретный адаптер хранилища через
  `NewRepo(*gorm.DB)`.
- Use-case'ы переведены на узкие output-порты; `Repo` удовлетворяет им
  через structural typing.
- `blocked-domain/web.Handlers` — struct с зависимостями; методы
  регистрируются в `web/server.go`.
- Тесты use-case'ов на фейках без sqlite; интеграционные тесты `Repo` на
  in-memory `:memory:`-sqlite.
- `db/batch.go::BatchInsertOn` / `BatchUpsertOn` — DI-варианты с явным
  `*gorm.DB`. Старые `BatchInsert`/`BatchUpsert` (singleton-обёртки)
  удалены в этапе 3 вместе с миграцией allow-domain.

### Этап 2 (коммит `530b26a`, **этот PR**) — DI закончен для core

- **`filter.Module`** (`filter/module.go`):
  `NewModule(repo, bloom, cache, conf, log)`. Методы `CheckExist`,
  `UpdateFromDb`, `ChangeStatus`, `Pause/Resume`, `PausedUntil`, `Enabled`.
  Use-case'ы `check-block`, `pause-filter`, `change-filter-dns-records`
  принимают зависимости явно (`Deps` / `*config.Config`). Hot path
  семантически эквивалентен старому: fail-open, порядок «bloom → cache.Clear»,
  атомарный `Enabled`/`PausedUntilUnix` сохранены.
- **`suggest_to_block.Module`** (`suggest-to-block/suggest_to_block.go`):
  `NewModule(blockRepo, allowRepo, sourceGate, filter, suggestRepo, log)`.
  `Collect()` ходит через узкие порты; `Start(ctx)` — 12h ticker.
  `suggest-to-block/db.Repo` — DI-обёртка над `SuggestBlock` CRUD.
  `web.Handlers` — struct с `Repo`, `BlockRepo`, `Filter`, `Log`. Сохранены
  поведенческие якоря: fail-closed на `SourceAutoBlocked`, rebuild bloom
  только при `autoBlocked > 0`, kill-switch.
- **`source.Module`** (`source/sync.go`): `NewModule(repo, blockRepo, log)`.
  `Seed()` идемпотентно сидит каталог источников; `Sync()` загружает и
  раскладывает в blocklist через `BlockRepo`. `source/db.Repo` — DI-обёртка
  с методами `Seed`, `GetAll`, `GetAllActive`, `Amount`, `GetByID`,
  `Update`, `IsActive`. Package-level хелперы и пакет
  `source/business/use-cases/seed/` удалены, тесты перенесены в
  `source/db/repo_test.go`.
- **`allow-domain/db.Repo`** — добавлена тонкая DI-обёртка над
  `GetAllActiveFilters` (потребитель — `suggest_to_block.Module`).
- **`web.CreateServer(web.Handlers{...})`** — принимает все хендлеры
  пакетом и не читает singleton'ов.
- **`main.go`** — composition root: одно соединение открывается явно и все
  `*Repo`, `*Module`, `*Handlers` конструируются здесь и пробрасываются явно.
  Schema migration также получает это соединение явно:
  `migrate.Migrate(conn)` не обращается к process-level DB accessor.

**Удалено:**
- `blocked-domain/blocked_domain.go` (shim) и 4 deprecated package-level
  функции в `blocked-domain/db/db.go` (`GetAllActiveFilters`,
  `IsDomainActivelyBlocked`, `CreateDNSRecordsByDomains`,
  `ChangeRecordStatusBySource`).
- `blocked-domain/db/db_test.go` (тестировал удалённые функции).
- `source/business/use-cases/seed/` пакет целиком.
- Package-level хелперы `source/db/db.go` (`GetAllRecords`,
  `GetAllActiveRecords`, `GetAmountRecords`, `GetRecordByID`,
  `UpdateRecord`, `IsActive`).
- Package-level хелперы `suggest-to-block/db/db.go`
  (`CreateSuggestBlockBatch`, `DeleteSuggestBlock`, `UpdateActiveStatus`,
  `GetAllSuggestBlocks`) — никем не вызывались, тянули singleton.
- `filter/filter_facade.go` → переименован в `filter/module.go`.

### Этап 3 — DI закончен для `allow-domain`

- **`allow-domain/db.Repo`** дополнен `CreateBatch(domains)` и
  `DeleteOlderThan(days)` (через `BatchUpsertOn(r.db, ...)` и `Unscoped`
  hard-delete). `GetAllActiveFilters` уже был там с этапа 2.
- **`AllowDomainEventStore`** переведён на DI:
  `CreateAllowDomainEventStore(repo, log, capacity)`, поля `repo` + `log`
  вместо package-level DB/logger accessors. Тест-сем
  `newWithChannelSize` зеркальный блок-воркеру; полное покрытие веток
  capacity/error/channel-full на фейках без sqlite.
- **`allow_domain_use_cases_clear_events.ClearEvent(repo)`** — узкий порт
  `DeleteOlderThan`, тестируемый шов `clearTask(repo)`.
- **`main.go`** прокидывает `allowRepo` + `chanLogger` в worker и cleanup.

**Удалено:**
- `allow-domain/allow_domain.go` (shim — три обёртки, потерявшие смысл).
- Package-level хелперы `allow-domain/db/db.go`
  (`CreateAllowDomainEvent` — мёртвый, `CreateBatchDomains`,
  `DeleteOlderThan`, `GetAllActiveFilters`). Файл оставлен только под
  тип-токен `AllowDomainEvent` для миграций.
- `db/batch.go::BatchInsert` и `BatchUpsert` — singleton-обёртки больше
  никем не дёргаются.

**Тестовое покрытие, пинённое в этом PR:**
- `TestRepo_CreateBatch` — happy, empty no-op, идемпотентный re-import,
  inserted-rows-are-active.
- `TestRepo_DeleteOlderThan_DeletesOnlyOldRows` +
  `TestRepo_DeleteOlderThan_ClosedConnSurfacesError` — позитив + DB-ошибка.
- `TestRepo_GetAllActiveFilters_FiltersInactive`, `..._Empty`.
- `TestEventStore_FlushesOnCapacity`, `..._LogsRepoError`,
  `..._DropsWhenChannelFull` (через `newWithChannelSize`).
- `TestClearTask_DelegatesWithRetentionDays`,
  `TestClearTask_PropagatesError`.

**Тестовое покрытие, пинённое в этом PR:**
- `TestCheckCacheOrDb_DBErrorFailsOpenWithoutCaching` — fail-open контракт
  без записи в кэш.
- `TestCheckBlock_DisabledShortCircuits`, `TestCheckBlock_PauseSuppressesBlocking`,
  `TestCheckBlock_BloomMissSkipsDB`, `TestCheckBlock_DeactivatedDomainNotBlocked`
  — все ветки hot path.
- `TestCollect_AutoBlockSourceQueryFails_FailClosed`,
  `TestCollect_AutoBlockDisabled_FallsThroughToSuggest`,
  `TestCollect_NoAutoBlock_SkipsFilterRebuild`,
  `TestCollect_AutoBlockUpdatesBloomFilter`, `TestCollect_MixedBatch`,
  `TestCollect_Idempotent`, `TestCollect_BlockRepoError_PropagatesAndSkipsRest`
  — все инварианты Collect.
- `TestPauseFilter_RaceWithEnabledToggle_NoTornState` — `-race`-stress
  на параллельный Pause vs внешний flip Enabled.
- `TestAddToBlock_DeactivatesBeforeRefreshingFilter` — порядок
  UpdateActive → UpdateFromDb через `callLog`.
- `TestAddToBlock_FilterRefreshError_Returns500`,
  `TestAddToBlock_UpdateActiveError_Returns500AndSkipsRefresh`,
  `TestChangeActiveStatus_UpdateError_Returns500` — негативные пути HTTP.
- `TestHandlerPauseFilter_InvalidDuration_Returns400`,
  `TestHandlerPauseFilter_FilterDisabled_Returns409` — маппинг
  business-error → HTTP-status в `filter/web` (раньше не пиннился вообще).

### Этап 4 — каждая фича сама регистрирует роуты

- **DI-фичи** получили метод `(h *Handlers) RegisterRoutes(rg *gin.RouterGroup)`:
  `auth/web`, `blocked-domain/web`, `clients/web`, `db/web`, `dns-cache/web`, `filter/web`, `logger/web`,
  `settings/web`, `suggest-to-block/web`, `source/web`, `traffic/web`.
- `domain-inspect/web` позднее переведён с package-level `Register`/`Inspect`
  на `NewHandlers(checks, log)` с приватными зависимостями и единый метод
  `RegisterRoutes`; неполная сборка теперь падает при старте, а не на запросе.
- **`auth/web`** разнесён на два instance-метода: `RegisterPublic(r gin.IRouter)` —
  только `POST /api/auth/login`; `RegisterRoutes(rg)` — `/auth/logout`,
  `/auth/me`. Middleware `RequireAuth()` также использует injected service.
- **`suggest-to-block/web.GetSignalCodes`** конвертирован из package-level
  функции в метод `*Handlers` — фича теперь регистрируется унифицированно.
- **`web/server.go`** ужат до cross-cutting wiring: CORS, public/protected
  split, Swagger, и набор вызовов `RegisterRoutes`. Введена
  внутренняя функция `buildRouter(h Handlers)` (без `r.Run`) — нужна для
  тестов; `CreateServer` теперь = `buildRouter` + go-`r.Run`.

**Регрессионный якорь.**
- `web/server_test.go::TestBuildRouter_RegistersAllExpectedRoutes` —
  snapshot `(method, path)` всех 30 публичных роутов сравнивается с
  `gin.Engine.Routes()`. Любое случайное удаление / переименование падает в
  CI; добавление эндпойнта требует одновременной правки `expectedRoutes`.
- `TestBuildRouter_LoginIsPublic` — `POST /api/auth/login` не возвращает
  401 от RequireAuth (то есть middleware не сработал, login был вызван).
- `TestBuildRouter_ProtectedRoutesRequireAuth` — таблично: каждый
  репрезентативный путь из каждой фичи без cookie → 401.
- `TestBuildRouter_CORSPreflightOnLogin` — OPTIONS на `/api/auth/login`
  возвращает 204 + корректные CORS-заголовки (login — единственный путь
  до auth, его preflight ломать особенно опасно).

**Документация:** `CLAUDE.md` (раздел Cross-cutting conventions),
`ARCHITECTURE.md` (раздел 9 Web API) — описывают self-routing-контракт.

### Этап 5 — runtime state принадлежит composition root

- `suggest_inspect_enabled` больше не хранится в package-level atomic:
  `main` создаёт один `suggest_inspect.EnabledState`, settings Apply пишет в
  него, а gates воркера и suggest-модуля читают тот же экземпляр.
- `traffic_retention_days` переведён с package-level atomic на
  `traffic_prune.RetentionState`. Один экземпляр передаётся одновременно в
  settings Apply и `traffic_prune.Run`; zero value `0` сохраняет защиту от
  удаления данных до `HydrateAll`.
- Для обоих состояний есть тесты независимости экземпляров и wiring-тесты
  `HydrateAll`/runtime Apply. Каждый подпункт прошёл отдельное ревью и полный
  `go test -race ./...`.

### Этап 6 — source sync принимает context

- `context.Context` проброшен через `background/sourcesync.Job` → `source.Module.Sync` →
  use-case `sync.Sync` → EasyList/hosts loaders.
- HTTP-запросы создаются через `http.NewRequestWithContext`; cancellation не
  логируется как сетевая ошибка и не запускает add/prune/refresh.
- Loader-facing parsers возвращают `scanner.Err`, поэтому отменённый или
  оборванный streaming body не считается успешным partial-списком.
- Source write-порт использует context-aware методы репозитория; GORM получает
  `WithContext(ctx)` для insert/delete batches, а prune проверяет cancellation
  между источниками.
- Exponential backoff использует отменяемый timer вместо `time.Sleep`, поэтому
  будущий signal-derived application context не будет ждать до 30 минут.
- Тесты закрепляют отмену обоих HTTP-loader'ов, pre-canceled Sync, cancellation
  во время sync и backoff. Пока `main` передаёт `context.Background()`; реальная
  остановка по сигналу подключается следующим lifecycle-этапом.

### Этап 7 — HTTP server принадлежит composition root

- `web.NewServer(addr, handlers)` только строит `*http.Server` и не открывает
  listener: пакет `web` больше не запускает скрытую горутину.
- `main` явно вызывает `ListenAndServe`; `http.ErrServerClosed` считается
  штатным результатом, остальные bind/runtime errors логируются.
- Возвращённый handle готов для подключения `Shutdown(ctx)` в общем lifecycle.
  Тест закрепляет адрес, тип handler и полный route snapshot без открытия порта.

### Этап 8 — DB size monitor без import-time goroutine

- `db/metric.go` больше не запускает `MonitoringDbSize` из `init()` и не
  резолвит package-level logger/config.
- `main` явно создаёт `DBSizeMonitor`, передавая registry, DB path, logger и
  interval, затем запускает `Run(ctx)` с application context.
- Monitor делает один immediate sample, останавливает ticker по cancellation,
  логирует stat errors и возвращает ошибку регистрации вместо panic.
- Тесты закрепляют pre-cancel, immediate observation, cancellation, stat error,
  invalid interval и duplicate registration.

### Этап 9 — Prometheus server без import-time listener

- `metric` больше не импортирует config/logger и не запускает HTTP listener из
  `init()`.
- `main` явно регистрирует Go/process/logger collectors после создания logger,
  строит `metric.NewServer` только при `MetricEnable` и владеет `*http.Server`.
- Metrics endpoint использует отдельный `http.ServeMux`, не меняет
  `http.DefaultServeMux`; bind/runtime errors идут через общий server reporter.
- Server запускается после регистрации component collectors и готов для
  `Shutdown(ctx)` в общем lifecycle.
- Глобальный registry и component `init()` collectors удалены на этапе 10.5.

### Этап 10.1 — consumer-owned port в `source/web`

- `source/web.Handlers` принимает узкий `SourceRepo`, а не concrete
  `*source/db.Repo`; production repo удовлетворяет порту structural typing.
- Handler-тесты не поднимают SQLite и покрывают list happy/error, invalid JSON,
  ошибки get/update/block/filter и обязательный порядок
  `source update → block rows → filter refresh`.

### Этап 10.2 — узкие порты в `blocked-domain/web`

- Concrete `*blocked-domain/db.Repo` удалён из `Handlers`: чтение списка идёт
  через consumer-owned `RecordsRepo`, create/update — через уже существующие
  порты соответствующих use-case'ов.
- `main` остаётся composition root и передаёт один production adapter в три
  независимых слота без расширения контрактов потребителей.
- DB-free handler-тесты проверяют передачу фильтра и storage error paths;
  SQLite integration-тесты сохраняют проверку успешных create/update сценариев.

### Этап 10.3 — consumer-owned RDAP cache port в inspect adapter

- `suggest-to-block/inspect.Adapter` принимает двухметодный `RDAPCache`
  (`GetRDAP`, `PutRDAP`) вместо concrete `*inspect/db.Repo`.
- Production repo проверяется compile-time assertion и передаётся из `main`
  structural typing без дополнительного adapter layer.
- Cache-aware adapter-тесты переведены с SQLite на in-memory fake и отдельно
  фиксируют registrable key, TTL и запись возраста домена.

### Этап 10.4 — bootstrap DI для DB и logger

- `main` явно создаёт `ChanLogger`, подключает console handler и передаёт его
  в `db.Open(OpenDeps{Path, Log, Metrics, DBName})`; ошибка открытия БД
  возвращается вызывающему, а не завершает процесс из пакета `db` через
  `log.Fatal`. Pool metrics получают явный registerer и уникальный `DBName`;
  дублирующее имя отклоняется при открытии, поэтому `go_sql_*` никогда не
  остаётся привязанным к устаревшему pool.
- Удалены production singleton accessors `logger.GetLogger` и
  `db.GetConnection`: DB-метрики зависят только от узкого `ErrorLogger` порта.
- Это намеренно breaking internal package API: DNS Filter поставляется как
  приложение, а не versioned Go SDK, поэтому deprecated singleton wrappers не
  сохраняются. Внешние потребители, если они существуют, должны перейти на
  явные конструкторы.
- `settings_wiring` зависит от двухметодного `runtimeLogger` порта, поэтому
  его тесты используют fake без process-level logger.
- Тесты закрепляют успешное открытие DB по явному пути, ошибку несуществующей
  директории и применение/валидацию `log_level` через injected logger.

### Этап 10.5 — component metrics через DI

- `main` создаёт единственный `prometheus.Registry`; package-level
  `metric.Registry` удалён.
- DNS, DNS cache, DB query instrumentation и suggest-inspect получили
  `NewMetrics(registerer)` и принимают созданные bundles через конструкторы.
- Feature packages больше не регистрируют collectors из `init()` и не вызывают
  `MustRegister`; ошибка регистрации возвращается composition root.
- DB query bundle один на приложение и разделяет наблюдения по `db_name`, поэтому
  несколько GORM connections используют общий registry без повторной регистрации.
- Тесты закрепляют независимость двух registry, duplicate-registration errors и
  использование injected bundle в cache/DNS/inspect/DB callbacks.

### Этап 10.6 — filter runtime state отделён от config singleton

- `config.Config` содержит только boot-time/default values; атомики `Enabled` и
  `PausedUntilUnix`, process-level `instance` и `sync.Once` удалены.
- `config.Load()` возвращает свежий config при каждом вызове, поэтому в одном
  процессе можно собрать несколько независимых application graphs.
- `main` создаёт один `filter/runtime-state.State`, передаёт его в
  `filter.Module` и `RestoreState`; use-case'ы зависят от consumer-owned runtime
  state ports и больше не импортируют `config`.
- Сохранены toggle/pause contracts: concurrent toggle parity, очистка pause при
  toggle, fail-open hot path и persisted restore до старта DNS.
- Тесты закрепляют независимость config/state экземпляров и весь прежний
  concurrent/race-контракт фильтра.

### Этап 10.7 — process-owned background runner

- Добавлен узкий lifecycle port `background.Job` и adapter `JobFunc`; feature
  workers сохраняют собственные зависимости и cadence, не импортируя runner.
- `main` явно собирает фиксированный набор jobs, условно добавляет DB monitor и
  LAN workers, затем передаёт весь набор в один `background.Runner`.
- Runner валидирует и копирует набор, запускает jobs конкурентно и ждёт их
  завершения на общем context; pre-canceled context не запускает работу.
- Все jobs стартуют после `settingsModule.HydrateAll()`, поэтому immediate ticks
  читают уже применённые runtime settings. Signal-derived context остаётся
  частью отдельного graceful-shutdown этапа.
- Unit-тесты закрепляют validation, concurrent start, ожидание остановки и
  pre-canceled поведение runner.

### Этап 10.8 — source loaders через DI

- Package-level HTTP clients из EasyList/hosts loaders удалены. `main` создаёт
  один timeout-configured `http.Client` и передаёт его в
  `source_sync.NewDefaultLoaders`; source feature владеет стабильным mapping
  source → adapter/endpoint, а composition root — transport policy.
- Use-case получает `LoaderRegistry map[BlockListSource]Loader` и выбирает
  adapter по активному source вместо прямых package-вызовов и switch по URL.
- Registry валидирует обязательные remote sources, отклоняет nil/typed-nil и
  копирует входную map; отсутствие loader во время sync помечает batch
  incomplete и защищает prune от неполного union.
- `source.Module` зависит от consumer-owned `SourceRepo`, а не concrete
  `*source/db.Repo`, и хранит injected registry для всех запусков Sync.
- Loader cancellation tests больше не подменяют globals и безопасно запускаются
  параллельно; module/use-case tests используют fake repo/loaders без HTTP/SQLite.

### Этап 10.9 — полный domain-inspect checks catalog через DI

- `checks.NewDefaultCatalog` получает от `main` только cross-package inputs:
  block/traffic readers, runtime credentials и URLScan key. Фабрика пакета
  владеет inspection-only HTTP client, системным DNS resolver, wall clock,
  production endpoints и сборкой `dns_resolve`, RDAP, crt.sh, VirusTotal,
  URLScan и Safe Browsing checks; `main` не знает внутренний dependency graph.
- Каждый HTTP check получил конструктор с `HTTPDoer` и endpoint; RDAP дополнительно
  принимает `Clock`, а DNS check — узкий `DNSResolver`. Package-level HTTP client
  и mutable endpoint seams удалены. Эти leaf-конструкторы остаются публичными
  для изолированных тестов и альтернативных фабрик.
- `checks.Default` больше не подставляет package-функции: все семь `CheckFunc`
  обязательны и проверяются при сборке каталога.
- `suggest-to-block/inspect.Adapter` получает RDAP check через `ProviderChecks`
  и оборачивает именно injected instance своим registrable-domain cache.
- Тесты используют изолированные clients/endpoints/resolvers и фиксированный
  RDAP clock; проверены независимость экземпляров, missing/typed-nil dependencies
  и обязательность полного каталога.

---

### Этап 10.10 — instance-based LAN discovery

- `clients.Module` объявляет consumer-owned `Discoverer` и получает его через
  `NewModule`; package-level `discovery.Discover` удалён.
- `discovery.Scanner` собирается из явных `ScannerDeps`: timeout, subnet finder,
  ARP/mDNS adapters, Docker network provider и OUI lookup. Экземпляр stateless и
  безопасен для параллельных HTTP-запросов.
- `discovery.NewDefaultScanner()` — package-owned production-фабрика без I/O;
  доступ к interfaces/raw sockets/multicast начинается только в `Discover`.
- Сохранены 5s default budget, более строгий caller deadline, best-effort partial
  errors, единый Docker filter и stable IP ordering.
- Тесты `clients.Module` используют fake `Discoverer`, а scanner tests — только
  fake platform dependencies; проверены error propagation, subnet-degraded mDNS,
  complete wiring, instance independence и DB-аннотация результатов.

---

## Следующий DI этап

**Выделить DB snapshot exporter из HTTP handler.** `db/web.DownloadDb` пока сам
создаёт temp directory, выполняет `VACUUM INTO`, открывает snapshot через GORM,
удаляет secrets и повторно vacuum'ит файл. Следующий DI PR должен оставить в
handler только HTTP orchestration, а filesystem/SQLite export собрать в
injected `SnapshotExporter` с package-owned production-фабрикой.

## Следующий lifecycle-рефакторинг

**Пункт 9 — graceful shutdown (HTTP + DNS + фоновые задачи).** Самый
острый из оставшихся: на SIGTERM текущая реализация обрывает соединения
без дренажа. После этапа 2 все зависимости явные — graceful shutdown
ложится поверх естественно, не требуя дополнительной инфраструктуры.
После этапа 4 у нас уже есть `buildRouter(h Handlers) *gin.Engine`,
отделяющий построение от запуска — это естественная точка, куда
встраивается возврат `*http.Server` для graceful shutdown.

### Что сейчас плохо

- `main` уже владеет `*http.Server` и явно запускает `ListenAndServe`, но пока
  не вызывает `Shutdown(ctx)` и не связывает HTTP runtime error с завершением
  всего приложения.
- `dnsServer.Serve()` блокирует main, но `s.Shutdown()` (есть в
  `dns/server.go:279`) никем не вызывается.
- Source sync pipeline уже принимает context и имеет отменяемый retry, но
  composition root пока передаёт `context.Background()`. До подключения
  signal-derived application context отмена на SIGTERM фактически не сработает.
- `TrafficEventStore.Stop(ctx)` уже останавливает admission, дожидается
  конкурентных senders, дренирует FIFO и делает финальный flush. Метод
  идемпотентен и безопасен для конкурентных вызовов. Осталось вызвать его из
  общего shutdown-блока после остановки DNS.
- Все периодические cleanup-задачи уже принимают context и logger явно через
  `periodic.Run` и собраны в `background.Runner`, но composition root пока
  запускает runner с `backgroundCtx` без cancel. Его нужно заменить
  signal-derived application context.

### Порядок шагов

1. **HTTP handle и явный запуск — готово**
   - `web.NewServer(addr, handlers)` возвращает `*http.Server` без side effects.
   - `main` владеет handle, запускает `ListenAndServe` и логирует неожиданные
     ошибки. В общем lifecycle осталось вызвать `Shutdown(ctx)`.

2. **Сигналы — `signal.NotifyContext` в main**
   ```go
   ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
   defer stop()
   ```
   - Передать `ctx` вместо текущего `backgroundCtx` в
     `backgroundRunner.Run`; runner раздаёт тот же context `arpwatcher`,
     suggest/auth/inspect workers и retention jobs. Сигнатуры задач уже
     context-aware.

3. **DNS — graceful Shutdown**
   - Запустить `dnsServer.Serve()` в горутине (через `errCh chan error`).
   - В main `select { case <-ctx.Done(): dnsServer.Shutdown(); case err := <-errCh: panic(err) }`.
   - `dns.NewServer` уже создаёт `*DnsServer`, который поднимает UDP+TCP; `Shutdown()`
     корректно дренирует TCP, UDP просто перестаёт читать.

4. **Traffic worker — подключить готовый Stop к shutdown**
   - `TrafficEventStore.Stop(ctx)` уже реализован и протестирован. main должен
     вызвать его после `dnsServer.Shutdown()` — это гарантирует, что новые DNS
     verdicts больше не придут до финального flush.

5. **`source.LoadAndParseActiveSources` — context для HTTP — готово**
   - Context проброшен через `source.Module.Sync` и use-case `sync.Sync` в оба
     loader'а; `suggestModule.Collect` эти loader'ы не вызывает.
   - `background/sourcesync.Job` использует отменяемый timer/select и не делает refresh
     или success-log после cancellation.

6. **Главный блок shutdown в main**
   ```go
   <-ctx.Done()
   shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
   defer cancel()
   _ = httpSrv.Shutdown(shutdownCtx)
   _ = dnsServer.Shutdown()
   trafficWorker.Stop(shutdownCtx)
   // Logger shutdown is intentionally separate: ChanLogger needs a
   // drain-safe Stop(ctx) before the application can close it.
   ```

### Тесты

- HTTP: `TestNewServer_GracefulShutdown_DrainsInFlightRequest` —
  стартуем сервер на ephemeral port, открываем HTTP-запрос с медленным
  handler'ом, отправляем SIGTERM, проверяем что запрос дошёл до конца с
  200 OK (а не получил RST).
- DNS: аналог через UDP/TCP — медленный upstream, проверяем что
  in-flight запрос завершается.
- Worker: финальный flush уже закреплён тестом
  `TestStopFlushesBufferedEvents`; дублировать его не нужно, достаточно
  lifecycle-теста порядка «DNS drain → TrafficEventStore.Stop».

---

## Чего опасаться

### Hot path
- **Поведенческая эквивалентность `filter.Module.CheckExist`** —
  обязательное условие. Fail-open на DB-ошибку (без кэширования) пиннится
  тестом `TestCheckCacheOrDb_DBErrorFailsOpenWithoutCaching`. Не сломать.
- **Bloom + LRU cache — атомарный сброс** в `Module.UpdateFromDb`:
  сначала `bloom.UpdateFilter`, потом `cache.Clear`. Любой порядок
  наоборот = залипший verdict в LRU после смены блок-листа (issue #26).
- **`State.Enabled()` и `State.PausedUntil()`** дёргаются на каждый запрос;
  внутри это atomic loads. RuntimeState interface — consumer-owned port, а
  production implementation один раз внедряется из `main`.

### Graceful shutdown — специфичные риски
- **DNS-server.Shutdown() vs in-flight upstream call.** `s.Shutdown()`
  гасит listener, но горутина `handleDNS`, уже зашедшая в
  `GetFromCacheOrCreateRequest`, может ещё 5 секунд ждать DoH-ответ.
  Нужно убедиться, что это окей (worst case — клиент получит ответ после
  shutdown, что нормально) или передать shutdown-ctx в `Exchange`.
- **HTTP-server.Shutdown() и SSE/long-polling.** В текущем API таких
  endpoint'ов нет, но если появятся — `Shutdown` будет ждать их
  бесконечно. `BaseContext`/`ConnContext` могут понадобиться.
- **`signal.NotifyContext` ловит только первый сигнал.** Второй
  Ctrl-C в терминале НЕ убьёт процесс — нужно
  `defer stop()` и/или ручной `os.Exit(1)` после таймаута shutdown.
- **Порядок shutdown.** HTTP первым (он трогает БД) → DNS (он трогает
  filter+cache) → workers (flush буферов) → logger (последним). Иначе
  логи финальной фазы не дойдут до Loki.
- **`TrafficEventStore`** хранит буфер под одной горутиной; реализованный
  `Stop` сигнализирует worker'у и не читает `buf` снаружи, сохраняя single-owner
  инвариант без мьютекса над агрегатами.

### Поведение существующих функций (актуально для любых будущих PR)
- **`Repo.CreateDNSRecordsByDomains`** сохраняет дедуп + batchSize=4000
  (лимит SQLite parameters). Не превышать.
- **`Repo.BatchCreateBlockDomainEvents`** молча игнорирует домены, которых
  нет в `block_lists` — тест-якорь `TestRepo_BatchCreateBlockDomainEvents`.
- **`source_db.Repo.IsActive(SourceAutoBlocked)`** fail-closed в
  `Collect()` — при DB-ошибке `autoBlockEnabled = false`. Якорь:
  `TestCollect_AutoBlockSourceQueryFails_FailClosed`.

### Migrate и type tokens
- `db/migrate/migrate.go` использует `&blocked_domain_db.BlockList{}`,
  `&BlockDomainEvent{}` как **type-tokens** для `AutoMigrate`. Если
  переносить модели — миграция должна продолжать видеть те же типы.
- Legacy-миграция `exclude_clients` → `clients` гейтится через `HasTable`
  + `Count == 0`. One-shot, идемпотентный. **Нельзя сломать** при
  любом рефакторинге `clients/db` — операторы потеряют IP-исключения из
  старой версии. Значение `filtered` записывается явно после `Create`, потому
  что GORM-тег `default:true` иначе заменяет `false` и инвертирует активное
  legacy-исключение.

### Внешние потребители не из scope DI
- `domain-inspect/checks` не читает singleton DB или package-level network
  dependencies: default-фабрика получает block/traffic readers, env-only
  URLScan key и общий runtime-updatable `Credentials`, связанный с settings в
  `main`, а внутренние remote checks получают от неё явные
  HTTP/DNS/endpoint/clock dependencies.

---

## Полный список десяти изначальных пунктов (статус)

| # | Пункт | Статус |
|---|---|---|
| 1 | Схлопнуть «папку-на-каждый use-case» | не начат |
| 2 | Удалить фасадные прослойки | **готово** (`blocked_domain.go`, `filter_facade.go` → `module.go`, `source/sync.go` упрощён) |
| 3 | DI вместо singleton'ов | **готово для** core, bootstrap DB/logger, component metrics, config/filter state, source loaders, background jobs, db/web, auth, clients и LAN discovery, dns-cache, полного domain-inspect checks catalog, bloom и verdict LRU. **Остаток:** DB snapshot exporter и точечные clock/generator seams |
| 4 | Разделить ORM-модель / domain / HTTP DTO | не начат |
| 5 | Каждая фича сама регистрирует роуты | **готово** (этап 4: `RegisterRoutes` в каждом `*/web/routes.go`, `web/server.go` ужат до cross-cutting wiring, snapshot-тест роутов в `web/server_test.go`) |
| 6 | `source.Sync()` не паникует в `main` | не начат |
| 7 | Свести фоновые задачи в один scheduler | не начат |
| 8 | Конвенция именования пакетов | не начат |
| 9 | Graceful shutdown (HTTP + DNS + workers) | **следующий кандидат** |
| 10 | Hot path не читает глобальный config | **готово**: hot path читает injected `RuntimeState`, `config.Load()` не singleton |

В DI-потоке следующий шаг — DB snapshot exporter; lifecycle п.9 можно вести
независимо. Пункты 1, 6, 7 и 8 также независимы и могут
включаться по мере касания соответствующих файлов.
