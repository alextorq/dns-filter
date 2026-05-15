# План чистки архитектуры

Документ дополняет `ARCHITECTURE.md` (описание системы как есть) — здесь
зафиксирован прогресс рефакторинга и ближайшие шаги, чтобы следующий
коммитящий не начинал с нуля.

---

## Архитектура сейчас (после allow-domain DI)

DNS-фильтр — single-binary Go-сервис: DNS на `:53` (UDP+TCP), HTTP API на
`:8080`, опциональные Prometheus-метрики на `:2112`. SQLite через GORM.

**Composition root — `main.go`.** `db.GetConnection()` вызывается ровно
один раз. Дальше каждая фича получает явные зависимости:

```
main.go
├── *gorm.DB ─────┬─ blocked_domain_db.NewRepo(conn) ── blockRepo
│                 ├─ allow_domain_db.NewRepo(conn) ──── allowRepo
│                 ├─ source_db.NewRepo(conn) ────────── sourceRepo
│                 └─ suggest_to_block_db.NewRepo(conn)  suggestRepo
│
├── filter.NewModule(blockRepo, bloom, cache, conf, log)  → filterModule
├── source.NewModule(sourceRepo, blockRepo, log)          → sourceModule
└── suggest_to_block.NewModule(blockRepo, allowRepo,
        sourceRepo, filterModule, suggestRepo, log)       → suggestModule
                            │
                            ├── dns.CreateServer(..., filterModule.CheckExist, ...)
                            └── web.CreateServer(web.Handlers{
                                    Blocked, Filter, Suggest, Source})
```

**Use-case'ы** (`*/business/use-cases/*`) — функции от узких output-портов,
объявленных рядом с потребителем. `*Repo` удовлетворяет всем портам через
structural typing — «accept interfaces, return structs». Тесты use-case'ов
гоняются на фейках без sqlite; репозитории покрыты отдельными тестами на
in-memory `:memory:`-sqlite.

**HTTP-handlers** — структуры с полями-зависимостями
(`*/web.Handlers{Repo, Module, Filter, Log, …}`). `web.CreateServer`
принимает их пакетом и не читает singleton'ов.

**DNS hot path** — `Module.CheckExist`:
1. `Conf.Enabled.Load()` (atomic) — глобальный toggle.
2. `Conf.PausedUntilUnix.Load()` (atomic) — активная пауза.
3. `Bloom.DomainExist` — O(1), 10M элементов, 0.1% FP.
4. `Cache.Get` — LRU 1500 элементов, только при bloom-hit.
5. `Repo.IsActivelyBlocked` — авторитетная проверка с учётом `Active=true`.
   На любую DB-ошибку — fail-open (false), без записи в кэш (#25).

Singleton'ы остались для bloom (`filter/filter`), LRU
(`filter/cache`), DNS-кэша (`dns-cache`), логгера (`logger`), конфига
(`config`) и `db.GetConnection()`. **Все они впитываются `*Module` в
`main.go`** — фичи их сами не вызывают. На singleton-коннекшене всё ещё
живут `auth/db` и `domain-inspect/checks/local_stats.go` — отдельные
точечные PR.

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
- **`main.go`** — composition root: `db.GetConnection()` вызывается ровно
  один раз. Все `*Repo`, `*Module`, `*Handlers` конструируются здесь и
  пробрасываются явно.

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
  вместо `db.CreateBatchDomains` + `logger.GetLogger()`. Тест-сем
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

---

## Кандидат на следующий рефакторинг

**Пункт 9 — graceful shutdown (HTTP + DNS + фоновые задачи).** Самый
острый из оставшихся: на SIGTERM текущая реализация обрывает соединения
без дренажа. После этапа 2 все зависимости явные — graceful shutdown
ложится поверх естественно, не требуя дополнительной инфраструктуры.

### Что сейчас плохо

- `web.CreateServer` запускает `r.Run(":8080")` в горутине и не
  возвращает `*http.Server`. `Shutdown(ctx)` позвать неоткуда.
- `dnsServer.Serve()` блокирует main, но `s.Shutdown()` (есть в
  `dns/server.go:279`) никем не вызывается.
- `suggestModule.Start(context.Background())` — фоновый ticker. Даже когда
  ctx будет реальный, текущая реализация гасит loop по `ctx.Done()`, но
  in-flight `Collect()` (HTTP-запросы к источникам через `easy-list`,
  upsert в DB) **не прерывается** — `easy_list.LoadFromURL` использует
  свой `http.Get` без context.
- `block_domain_uc.NewBlockDomainEventStore` (и аналогичный allow worker)
  — горутины с буфером, который сбрасывается на ticker'е 20s. На SIGTERM
  буфер теряется (≤ 100 событий на worker).
- `arpwatcher.Run(context.Background(), ...)` — уже принимает ctx, но
  передаётся `Background()` без cancel.

### Порядок шагов

1. **HTTP — `web.CreateServer` возвращает `*http.Server`**
   - Заменить `go r.Run(":8080")` на `srv := &http.Server{Addr: ":8080", Handler: r}`
     и `go srv.ListenAndServe()`. Сигнатура: `func CreateServer(h Handlers) *http.Server`.
   - В main: `httpSrv := web.CreateServer(...)`, дальше при сигнале —
     `httpSrv.Shutdown(ctx)`.

2. **Сигналы — `signal.NotifyContext` в main**
   ```go
   ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
   defer stop()
   ```
   - Передать `ctx` в `arpwatcher.Run`, `suggestModule.Start`,
     `authBusiness.ClearExpiredSessions` (последний сейчас не принимает ctx —
     придётся протащить).

3. **DNS — graceful Shutdown**
   - Запустить `dnsServer.Serve()` в горутине (через `errCh chan error`).
   - В main `select { case <-ctx.Done(): dnsServer.Shutdown(); case err := <-errCh: panic(err) }`.
   - `dns.CreateServer` уже создаёт `*dns.Server` для UDP+TCP; `Shutdown()`
     корректно дренирует TCP, UDP просто перестаёт читать.

4. **Background workers — flush на shutdown**
   - `*BlockDomainEventStore` и `*AllowDomainEventStore` получают метод
     `Stop(ctx)`, который останавливает worker и делает финальный flush
     буфера. main вызывает `Stop` после `dnsServer.Shutdown()` — гарантия,
     что больше событий не придёт.
   - Альтернатива: оставить как есть, принять потерю ≤ 100 событий на
     worker. Это event-логи, не CRUD. Зависит от приоритета — обсудить.

5. **`source.LoadAndParseActiveSources` — context для HTTP**
   - `easy_list.LoadFromURL` и `LoadHostsFromURL` сейчас используют
     `http.Get`. Перевести на `http.NewRequestWithContext(ctx, ...)` →
     `client.Do(req)`. На SIGTERM при первом старте Sync() прервётся
     корректно, не висит на 30-секундном таймауте.
   - `suggestModule.Start(ctx)` — пробросить ctx в `Collect`, оттуда — в
     те же loaders. Сейчас `Collect()` без ctx, нужно расширить сигнатуру.

6. **Главный блок shutdown в main**
   ```go
   <-ctx.Done()
   shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
   defer cancel()
   _ = httpSrv.Shutdown(shutdownCtx)
   _ = dnsServer.Shutdown()
   blockWorker.Stop(shutdownCtx)
   allowWorker.Stop(shutdownCtx)
   logger.GetLogger().Close()  // последним — иначе потеряем логи shutdown
   ```

### Тесты

- HTTP: `TestCreateServer_GracefulShutdown_DrainsInFlightRequest` —
  стартуем сервер на ephemeral port, открываем HTTP-запрос с медленным
  handler'ом, отправляем SIGTERM, проверяем что запрос дошёл до конца с
  200 OK (а не получил RST).
- DNS: аналог через UDP/TCP — медленный upstream, проверяем что
  in-flight запрос завершается.
- Workers: `TestBlockDomainEventStore_StopFlushesBuffer` — наполняем
  буфер ниже capacity, вызываем `Stop(ctx)`, проверяем что репо получил
  все события.

---

## Чего опасаться

### Hot path
- **Поведенческая эквивалентность `filter.Module.CheckExist`** —
  обязательное условие. Fail-open на DB-ошибку (без кэширования) пиннится
  тестом `TestCheckCacheOrDb_DBErrorFailsOpenWithoutCaching`. Не сломать.
- **Bloom + LRU cache — атомарный сброс** в `Module.UpdateFromDb`:
  сначала `bloom.UpdateFilter`, потом `cache.Clear`. Любой порядок
  наоборот = залипший verdict в LRU после смены блок-листа (issue #26).
- **`Conf.Enabled.Load()` и `PausedUntilUnix.Load()`** дёргаются на
  каждый запрос — атомики, дешёвые. Любая прослойка (interface call
  вместо поля) видна на бенчмарке. Если будут менять `Deps.Conf` на
  плагинный интерфейс — мерить через DNS-бенчмарк.

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
- **`block_domain_uc.NewBlockDomainEventStore`** хранит `e.buf` под
  одной горутиной — `Stop` нужно реализовать через канал-сигнал, не
  через мьютекс над buf, иначе race с `start()` loop.

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
  старой версии.

### Внешние потребители не из scope DI
- `domain-inspect/checks/local_stats.go` — дёргает `db.GetConnection()`
  напрямую (не через `blocked-domain` / `allow-domain` Repo). DI там —
  отдельный захват.
- `auth` использует `db.GetConnection()` через `auth/db` package-level
  функции. Также singleton-зависимый.

---

## Полный список десяти изначальных пунктов (статус)

| # | Пункт | Статус |
|---|---|---|
| 1 | Схлопнуть «папку-на-каждый use-case» | не начат |
| 2 | Удалить фасадные прослойки | **готово** (`blocked_domain.go`, `filter_facade.go` → `module.go`, `source/sync.go` упрощён) |
| 3 | DI вместо singleton'ов | **готово для core + allow-domain**. Остатки: `auth`, `domain-inspect`, `dns-cache` — отдельные PR |
| 4 | Разделить ORM-модель / domain / HTTP DTO | не начат |
| 5 | Каждая фича сама регистрирует роуты | не начат (Handlers есть, но routes всё ещё в `web/server.go`) |
| 6 | `source.Sync()` не паникует в `main` | не начат |
| 7 | Свести фоновые задачи в один scheduler | не начат |
| 8 | Конвенция именования пакетов | не начат |
| 9 | Graceful shutdown (HTTP + DNS + workers) | **следующий кандидат** |
| 10 | Hot path не читает глобальный config | частично (через `Deps.Conf` use-case'ы получают конфиг явно, но `*config.Config` всё ещё singleton) |

Логично закрывать в порядке п.9 → п.5 (feature-self-routing, маленький PR
поверх Handlers) → п.4. Пункты 1, 6, 7, 8, 10 — независимы и можно
включать по мере касания соответствующих файлов.
