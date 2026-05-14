# План чистки архитектуры

Документ дополняет `ARCHITECTURE.md` (описание системы как есть) — здесь
зафиксирован прогресс рефакторинга и ближайшие шаги, чтобы следующий
коммитящий не начинал с нуля.

---

## Что сделано (коммит `dda1916`)

**Пилот перевода `blocked-domain` на dependency injection.** Реализует пункт 3
из списка ниже («Перестать дёргать `db.GetConnection()` из 22 точек»),
ограничено одним feature-модулем.

- `blocked-domain/db.Repo` — конкретный адаптер хранилища. Создаётся через
  `NewRepo(*gorm.DB)` на composition root.
- Use-case'ы (`create-domain`, `update-dns-record`, `block-domain`,
  `clear-events`) — функции, зависящие от **узких output-портов**
  (интерфейсы рядом с потребителем, ISP). `Repo` удовлетворяет им через
  structural typing — «accept interfaces, return structs».
- `blocked-domain/web.Handlers` — struct с полями-зависимостями (`Repo`,
  `Log`, `RefreshFilter`); хендлеры — методы. Регистрация в `web/server.go`.
- Тесты use-case'ов — на фейках, **без sqlite**. Интеграционные тесты Repo
  — на in-memory `:memory:`-sqlite с `SetMaxOpenConns(1)`.
- `db/batch.go::BatchInsertOn` / `BatchUpsertOn` — DI-варианты с явным
  `*gorm.DB`. Старые `BatchInsert` / `BatchUpsert` помечены `// Deprecated:`.

**Что осталось как временный shim** (удаляется в следующем PR):

| Где | Зачем |
|---|---|
| `blocked-domain/blocked_domain.go::repo()` | внешние потребители фасада продолжают компилироваться |
| `blocked-domain/db/db.go::legacyRepo()` + 4 `Deprecated:` функции (`GetAllActiveFilters`, `IsDomainActivelyBlocked`, `CreateDNSRecordsByDomains`, `ChangeRecordStatusBySource`) | `filter`, `source` всё ещё дёргают package-level API |

Оба shim'а делают один и тот же `NewRepo(GetConnection())` — единственный
путь к БД, без дублирования логики.

---

## Кандидат на следующий рефакторинг

**Продолжение пункта 3 — DI в `filter`, `suggest-to-block`, `source`.**
Завершит переход и позволит удалить оба shim'а целиком.

### Порядок шагов

1. **`filter`**
   - `filter/business/use-cases/check-exist/check-block.go` — функция
     `CheckBlock(repo BlockChecker, domain string) bool`. Узкий порт:
     `interface { IsActivelyBlocked(string) (bool, error) }`.
   - Завести `filter.Module` со связкой `(repo, bloom, cache, config)`.
     Методы: `Module.CheckExist(domain)`, `Module.UpdateFromDb()`.
   - В `main.go`: `filterModule := filter.NewModule(blockRepo); s := dns.CreateServer(..., filterModule.CheckExist, ...)`.
   - Тесты `check-block_test.go` сейчас интеграционные с `os.Chdir`+sqlite —
     переписать на фейк-репозиторий.

2. **`suggest-to-block`**
   - `Collect()` принимает `(blockRepo, allowRepo, filterModule, log)`.
     Сейчас функция глобально дёргает `blocked_domain.GetAllActiveFilters`,
     `filter.UpdateFilterFromDb`, `source_db.IsActive(SourceAutoBlocked)`.
   - `web/suggest.go` — `Handlers struct` с зависимостями.
   - `suggest_test.go::TestMain` чистится от `os.Chdir`.

3. **`source`**
   - `Sync()` принимает `blockRepo`.
   - `web/records.go` — `Handlers struct`.

4. **Удаление shim'ов**
   - `blocked-domain/blocked_domain.go` — целиком.
   - `blocked-domain/db/db.go::legacyRepo()` + 4 `Deprecated:` функции.
   - `db/batch.go::BatchInsert` / `BatchUpsert` — после миграции
     `allow-domain`, `auth`, `suggest-to-block` на `*On` варианты.

5. **`main.go` как полноценный composition root** — все `Repo`, `Module`,
   `Handlers` конструируются здесь и пробрасываются явно. После этого
   шага `db.GetConnection()` остаётся ровно в одной точке.

---

## Чего опасаться

### Hot path
- **`filter.CheckBlock` вызывается на каждый DNS-запрос.** Любая
  регрессия = либо ложные блокировки, либо пропуск реальных. Поведенческая
  эквивалентность — обязательное условие. Текущая семантика fail-open в
  `CheckCacheOrDb` (DB-ошибка ⇒ `false`, не кэшируем) — должна сохраниться;
  тест на это есть, не сломать.
- **Bloom filter и LRU cache — глобальные singleton'ы.** `UpdateFilterFromDb`
  атомарно сбрасывает оба. Если перенос в `Module` нарушит этот порядок —
  получим залипший verdict в LRU после изменения блок-листа (issue #26
  уже был). Сохранить «cache.Clear после bloom.UpdateFilter».
- **`config.Enabled.Load()` и `PausedUntilUnix.Load()` дёргаются на каждый
  запрос.** Это атомики, дешёвые, но в пилотном `Module` их нельзя
  заменить плагинным интерфейсом без замера — лишний indirect-call в hot
  path заметен на бенчмарках. Если трогать — мерить через
  `dns/server_test.go::BenchmarkX`.

### Поведение существующих функций
- **`Repo.CreateDNSRecordsByDomains` сохраняет дедуп + batchSize=4000**
  (лимит SQLite parameters). При переносе других callsite'ов проверять,
  что батч-операции не теряют гард на пустой вход и не превышают лимит.
- **`Repo.BatchCreateBlockDomainEvents` молча игнорирует домены, которых
  нет в `block_lists`.** Это правильное поведение (event без foreign-key
  не нужен), но не очевидно по коду — есть тест-якорь, переносить с ним.
- **`source_db.IsActive(SourceAutoBlocked)` fail-closed** в `Collect()` —
  при DB-ошибке `autoBlockEnabled = false`. При DI не потерять этот
  fallback (иначе ломается kill-switch).

### Migrate и type tokens
- `db/migrate/migrate.go` использует `&blocked_domain_db.BlockList{}`,
  `&BlockDomainEvent{}` как **type-tokens** для `AutoMigrate`. Если
  переносить модели — миграция должна продолжать видеть те же типы.
- Legacy-миграция `exclude_clients` → `clients` гейтится через `HasTable`
  + `Count == 0`. Это **one-shot, идемпотентный** код. При любом
  рефакторинге `clients/db` его нельзя сломать — операторы потеряют
  IP-исключения из старой версии.

### Аллокации в shim
- `repo()` и `legacyRepo()` аллоцируют `Repo{db}` (16 байт стека) на
  каждый вызов. На hot path через `filter.IsDomainActivelyBlocked` это
  значит — N аллокаций в секунду при N DNS-запросах. GC не упадёт, но
  это лишний повод закончить п.3 быстро и не оставлять shim жить долго.

### Тесты, которые сейчас работают через `os.Chdir`+singleton
- `filter/business/use-cases/check-exist/check_block_test.go`
- `suggest-to-block/web/suggest_test.go`
- `suggest-to-block/suggest_to_block_test.go`

Они **рабочие**, но при параллельном запуске тестов через `t.Parallel()`
или `go test -p 2` могут начать конфликтовать (общий `./filter.sqlite` в
текущей директории). При переводе на DI это уходит само.

### Внешние потребители не из scope пилота
- `domain-inspect/checks/local_stats.go` — дёргает `db.GetConnection()`
  напрямую (не через `blocked-domain` фасад), читает таблицы
  `block_lists`/`block_domain_events`. Это **другая песня**: модуль
  inspect не зависит от blocked-domain пакета, и DI там — отдельный
  захват. Помечен как известный остаток.

### HTTP без graceful shutdown
- `web/server.go` запускает gin в горутине без возврата `*http.Server`.
  При останове процесса ответы не дренируются, активные соединения
  обрываются. Это **отдельный пункт (9)**, не блокер для DI, но при
  любом серьёзном захвате `web/server.go` лучше сразу это починить
  заодно.

---

## Полный список десяти изначальных пунктов (статус)

| # | Пункт | Статус |
|---|---|---|
| 1 | Схлопнуть «папку-на-каждый use-case» | не начат |
| 2 | Удалить фасадные прослойки | частично (`blocked_domain.go` — shim, удалится в следующем PR) |
| 3 | DI вместо singleton'ов | **в работе**: blocked-domain ✓, остальное в следующем PR |
| 4 | Разделить ORM-модель / domain / HTTP DTO | не начат |
| 5 | Каждая фича сама регистрирует роуты | не начат |
| 6 | `source.Sync()` не паникует в `main` | не начат |
| 7 | Свести фоновые задачи в один scheduler | не начат |
| 8 | Конвенция именования пакетов | не начат |
| 9 | Graceful shutdown (HTTP) | не начат |
| 10 | Hot path не читает глобальный config | не начат |

Логично закрывать в порядке п.3 (доделать) → п.2 (удалить фасады) → п.5
(роуты регистрируются фичами) → п.9 (graceful shutdown). Пункты 1 и 8 —
косметика, можно вместе. Пункт 4 — самостоятельный, большой захват.
Пункты 6, 7, 10 — точечные.
