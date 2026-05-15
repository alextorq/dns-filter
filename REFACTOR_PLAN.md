# План чистки архитектуры

Документ дополняет `ARCHITECTURE.md` (описание системы как есть) — здесь
зафиксирован прогресс рефакторинга и ближайшие шаги, чтобы следующий
коммитящий не начинал с нуля.

---

## Что сделано

### Этап 1 (коммит `dda1916`) — пилот DI в `blocked-domain`

- `blocked-domain/db.Repo` — конкретный адаптер хранилища через `NewRepo(*gorm.DB)`.
- Use-case'ы — функции, зависящие от узких output-портов; `Repo` удовлетворяет
  их через structural typing.
- `blocked-domain/web.Handlers` — struct с зависимостями; методы регистрируются
  в `web/server.go`.
- Тесты use-case'ов на фейках без sqlite; интеграционные тесты `Repo` на
  in-memory `:memory:`-sqlite.
- `db/batch.go::BatchInsertOn` / `BatchUpsertOn` — DI-варианты с явным `*gorm.DB`.

### Этап 2 (этот коммит) — DI закончен для `filter`, `suggest-to-block`, `source`

- **`filter.Module`** (`filter/module.go`): `NewModule(repo, bloom, cache, conf, log)`.
  Методы `CheckExist`, `UpdateFromDb`, `ChangeStatus`, `Pause/Resume`,
  `PausedUntil`, `Enabled`. Use-case'ы `check-block`, `pause-filter`,
  `change-filter-dns-records` принимают зависимости явно (`Deps` / `*config.Config`),
  больше не зовут singleton'ов. `filter/web.Handlers` — struct с `Module`.
  Тесты `check_block_test.go` — на фейк-репозитории, фейк-bloom, фейк-кэше;
  добавлен явный тест fail-open контракта при DB-ошибке.
- **`suggest_to_block.Module`** (`suggest-to-block/suggest_to_block.go`):
  `NewModule(blockRepo, allowRepo, sourceGate, filter, suggestRepo, log)`.
  `Collect()` ходит через узкие порты; `Start(ctx)` — 12h ticker.
  `suggest-to-block/db.Repo` — DI-обёртка над `SuggestBlock` CRUD.
  `web.Handlers` — struct с `Repo`, `BlockRepo`, `Filter`, `Log`. Тесты —
  через harness c in-memory sqlite, без `os.Chdir`. Сохранены якоря:
  fail-closed на `SourceAutoBlocked`, rebuild bloom только при `autoBlocked>0`,
  kill-switch.
- **`source.Module`** (`source/sync.go`): `NewModule(repo, blockRepo, log)`.
  `Seed()` идемпотентно сидит каталог источников; `Sync()` загружает и
  раскладывает в blocklist через `BlockRepo`. `source/db.Repo` — DI-обёртка,
  package-level хелперы удалены, `seed/` пакет полностью удалён (тесты
  перенесены в `source/db/repo_test.go`). `source/web.Handlers` — struct.
- **`allow-domain/db.Repo`** — добавлена тонкая DI-обёртка над
  `GetAllActiveFilters` (потребитель — `suggest_to_block.Module`). Сам пакет
  `allow-domain` остаётся на singleton'е для `CreateAllowDomainEventStore` и
  `ClearOldEvent` — миграция allow-domain на DI вынесена в следующий PR.

### Удалено вместе с этапом 2

| Файл / функция | Причина |
|---|---|
| `blocked-domain/blocked_domain.go` | shim между фасадом и `Repo`; больше нет потребителей |
| `blocked-domain/db.{GetAllActiveFilters, IsDomainActivelyBlocked, CreateDNSRecordsByDomains, ChangeRecordStatusBySource}` | заменено `*Repo` методами с тем же поведением |
| `blocked-domain/db/db_test.go` | тестировал удалённые package-level helpers; эквивалент уже есть в `repo_test.go` |
| `source/business/use-cases/seed/` | логика переехала в `source/db.Repo.Seed()`, тесты — в `source/db/repo_test.go` |
| `source/db.{GetAllRecords, GetAllActiveRecords, GetAmountRecords, GetRecordByID, UpdateRecord, IsActive}` | заменено `*Repo` методами |

### `main.go` — composition root

- `db.GetConnection()` вызывается ровно в одной точке (`main`).
- Все `*Repo`, `*Module`, `*Handlers` конструируются в `main` и пробрасываются
  явно в `dns.CreateServer` и `web.CreateServer`.
- `web.CreateServer(web.Handlers{...})` принимает все хендлеры пакетом и не
  читает singleton'ов.

---

## Полный список десяти изначальных пунктов (статус)

| # | Пункт | Статус |
|---|---|---|
| 1 | Схлопнуть «папку-на-каждый use-case» | не начат |
| 2 | Удалить фасадные прослойки | **готово** (`blocked_domain.go`, `filter_facade.go` → `module.go`, `source/sync.go` упрощён) |
| 3 | DI вместо singleton'ов | **готово** для blocked-domain / filter / suggest-to-block / source. Остатки: allow-domain, auth, domain-inspect — отдельные PR |
| 4 | Разделить ORM-модель / domain / HTTP DTO | не начат |
| 5 | Каждая фича сама регистрирует роуты | не начат (Handlers есть, но routes всё ещё в `web/server.go`) |
| 6 | `source.Sync()` не паникует в `main` | не начат |
| 7 | Свести фоновые задачи в один scheduler | не начат |
| 8 | Конвенция именования пакетов | не начат |
| 9 | Graceful shutdown (HTTP) | не начат |
| 10 | Hot path не читает глобальный config | не начат (`config.GetConfig().Enabled.Load()` уходит из use-case'а в `Deps.Conf`, но singleton остался) |

---

## Кандидаты на следующий PR

Самые «дешёвые» и логически связанные пункты после этого этапа:

1. **Пункт 5 — feature-self-routing.** `web/server.go` всё ещё знает о каждом
   per-feature `Handlers`. Дать каждому модулю метод `RegisterRoutes(r gin.IRouter)` —
   `server.go` превратится в декларативный список модулей плюс auth-обёртку.
2. **Пункт 9 — graceful shutdown HTTP.** `web.CreateServer` сейчас запускает
   `r.Run(":8080")` в горутине без возврата `*http.Server`. При SIGTERM
   соединения обрываются. Минимум: возвращать `*http.Server` и в `main`
   ловить сигнал + `Shutdown(ctx)`. Хорошо ложится поверх DI — все зависимости
   уже явные.
3. **Доделать `allow-domain` на DI.** `allow-domain/db.Repo` уже есть, но
   `CreateAllowDomainEventStore`, `ClearOldEvent` и `CreateBatchDomains`
   продолжают читать singleton. Маленький точечный PR.
4. **`db/batch.go::BatchInsert` / `BatchUpsert`** удалить после миграции
   `allow-domain`, `auth` на `*On` варианты.

Логично закрыть в порядке 5 → 9 → 3-allow-domain → 4. Пункт 4 — самостоятельный
большой захват (ORM/HTTP DTO). Пункты 1, 6, 7, 8, 10 — независимы и можно
включать по мере касания соответствующих файлов.

---

## Чего опасаться (применимо к будущим PR)

### Hot path (DNS-запрос)
- **Поведенческая эквивалентность `filter.Module.CheckExist`** — обязательное
  условие. Fail-open на DB-ошибку (без кэширования) пиннится тестом
  `TestCheckCacheOrDb_DBErrorFailsOpenWithoutCaching`. Не сломать.
- **Bloom + LRU cache — атомарный сброс** в `Module.UpdateFromDb`: сначала
  `bloom.UpdateFilter`, потом `cache.Clear`. Любой порядок наоборот = залипший
  verdict в LRU после смены блок-листа (issue #26).
- **`config.Enabled.Load()` и `PausedUntilUnix.Load()`** дёргаются на каждый
  запрос. Это атомики, дешёвые — но любая прослойка (interface call вместо
  поля) видна на бенчмарке. Если будут менять `Deps.Conf` на плагинный
  интерфейс — мерить через DNS-бенчмарк.

### Поведение существующих функций
- **`Repo.CreateDNSRecordsByDomains` сохраняет дедуп + batchSize=4000** (лимит
  SQLite parameters). Не превышать.
- **`Repo.BatchCreateBlockDomainEvents` молча игнорирует домены, которых нет
  в `block_lists`** — тест-якорь `TestRepo_BatchCreateBlockDomainEvents`.
- **`source_db.IsActive(SourceAutoBlocked)` fail-closed** в `Collect()` —
  при DB-ошибке `autoBlockEnabled = false`. Якорь:
  `TestCollect_AutoBlockSourceQueryFails_FailClosed`.

### Migrate и type tokens
- `db/migrate/migrate.go` использует `&blocked_domain_db.BlockList{}`,
  `&BlockDomainEvent{}` как **type-tokens** для `AutoMigrate`. Если переносить
  модели — миграция должна продолжать видеть те же типы.
- Legacy-миграция `exclude_clients` → `clients` гейтится через `HasTable` +
  `Count == 0`. One-shot, идемпотентный.

### Внешние потребители не из scope DI
- `domain-inspect/checks/local_stats.go` — дёргает `db.GetConnection()`
  напрямую (не через `blocked-domain` фасад). Модуль inspect не зависит от
  blocked-domain пакета, и DI там — отдельный захват.

### HTTP без graceful shutdown
- `web/server.go::CreateServer` запускает gin в горутине без возврата
  `*http.Server`. При останове процесса ответы не дренируются. Пункт 9, не
  блокер для DI, но при следующем серьёзном захвате `web/server.go` лучше
  чинить заодно.
