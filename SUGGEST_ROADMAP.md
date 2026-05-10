# DNS-filter / suggest-to-block — Roadmap

Документ для продолжения работы из чистого контекста. Описывает реализованные сигналы, оставшиеся задачи (Phase 1 Task 4–5, Phase 2, Phase 3), конвенции и рабочий процесс TDD-ревью-коммит.

## Контекст

Sinkhole DNS server. Модуль `suggest-to-block` периодически анализирует домены, к которым клиенты обращались, но которые не были заблокированы (`AllowDomainEvent`), и оценивает их по набору эвристик. Если суммарный score ≥ `ThresholdToSuggestBlocking` (= 30) — домен попадает в очередь модерации.

Все эвристики живут в пакете `suggest-to-block/business/use-cases/collect/`.

## Текущее состояние

### Реализованные сигналы

| Сигнал | Score | Файл |
|---|---|---|
| Suspicious entropy (Shannon + consonant ratio) | +20 | `shannon.go` |
| Bad keyword tokens | +5 | `bad-words.go` |
| Risky TLD | +5 | `risky-tld.go` (Task 1) |
| Numeric run ≥7 цифр подряд | +5 | `numeric-run.go` (Task 2) |
| Hex/UUID label | +10 | `hex-uuid.go` (Task 3) |
| Brand impersonation (apex похож на бренд, но не равен) | +25 | `brand-impersonation.go` (Task 5) |
| Subdomain of blocked | +20 | `collect.go` |
| Similar to blocked (same depth + DL≥80%) | +15 | `collect.go` |

Все веса (`ItemScore*`) и фразы причин (`Reason*`) — централизованы в `collect.go`. Никаких магических строк/чисел в коде или тестах.

### Сделанные коммиты

```
67a2d13 suggest: add brand-impersonation signal to detect typosquat domains
8b4fefc suggest: add hex-UUID label signal to detect hashes and UUIDs
aabeeca suggest: add numeric-run signal to detect tracker IDs and timestamps
0208acd suggest: add risky-TLD signal and extract reason texts to constants
a7cb4c8 suggest: fix score accumulation, threshold check and slice allocation
```

Самый старый из них — багфиксы (`= → +=`, неработавший порог, `make([]T, len(s))` → `make([]T, 0, len(s))`). Остальные четыре — Phase 1 Task 1–3 и Task 5 (Task 4 пропущен, см. ниже). Ничего не запушено в origin.

## Рабочий процесс

Каждая задача проходит цикл:

1. **Описание задачи** — детально: API, расположение файлов, состав констант, тестовый план. Жду OK от пользователя.
2. **Red phase** — пишу тестовый файл первым. Прогоняю `go test`, подтверждаю что компиляция падает на ссылках на несуществующие символы (это и есть «red»).
3. **Ревью тестов** — проговариваю что покрыто (positive/negative/trap/property), какие подводные камни закрыты. Жду OK.
4. **Green phase** — минимальная реализация, чтобы тесты прошли. Прогон всего пакета `collect` + смежных (`suggest-to-block/db`, `suggest-to-block/web`).
5. **Independent review** — `Agent(subagent_type="general-purpose")` с briefing'ом, в котором весь нужный контекст без conversation history. Просим verdict + numbered findings + severity. Лимит ответа ~250 слов. Явная инструкция «не rubber-stamp».
6. **Триаж findings** — фиксы для реальных проблем; honest push back с обоснованием там, где сабагент ошибся.
7. **Коммит** — короткое сообщение в стиле проекта (без Co-Authored-By trailer).

### Ключевые принципы

- **Тесты ссылаются на константы.** Никогда `strings.Contains(reason, "tld")` — всегда `strings.Contains(reason, ReasonRiskyTLD)`. Меняем формулировку — тест не ломается.
- **Регрессия после каждой задачи.** Прогон всего пакета — предыдущие тесты могут зацепиться новой логикой (например, новый сигнал случайно добавит +5 к домену из старого fixture).
- **Сабагент-ревью обязательно.** Даже когда уверен, что код чистый. На Phase 1 сабагент несколько раз нашёл реальные проблемы (multiple trailing dots, IDN-skip несогласован между двумя сигналами).
- **Push back на ошибки сабагента** — честно. На Task 3 он ошибочно сказал что entropy срабатывает на UUID; пересчитал арифметику, тест уже это проверял косвенно — не стал править.

## Конвенции кода

### Файлы и имена

- **kebab-case**: `risky-tld.go`, `numeric-run.go`, `hex-uuid.go`. Тестовый файл — то же имя + `_test.go`.
- **API сигнала**: пара `HasFooLabel(domain) bool` (публичная) + `looksLikeFoo(label) bool` (приватная). Public декомпозирует FQDN на лейблы и пропускает TLD; private проверяет один лейбл.

### Константы

- **ВСЕ** `ItemScore*` и `Reason*` живут в `collect.go`, в двух соседних `const`-блоках. Никогда не заводим в feature-файле. Контракт «вес и формулировка» собран в одном месте, тесты импортируют оттуда.
- В feature-файле — только тех-константы (например `MinNumericRunLength`, `HexUUIDMinLength`, `MinBrandImpersonationLength`, `RiskyTLDs` map).

### Поведение функций

- **Trailing dots**: `strings.TrimRight(domain, ".")`. **Не** `TrimSuffix` — он снимает только одну точку, а реальные DNS-входы могут иметь несколько.
- **Регистр**: `strings.ToLower` сразу в публичной функции, дальше всё работает с lower-case.
- **Single-label вход**: возвращаем false. Ожидаем FQDN.
- **Punycode skip**: в публичной функции, **не** в private helper'е (для консистентности с `numeric-run.go`). Это нашёл сабагент в ревью Task 3. Применимо к ASCII-сигналам; будущая Task 4 (homograph), наоборот, декодирует `xn--` явно.
- **Length-gate для similarity-сигналов**: при сравнении кандидата со списком «защищаемых» элементов (как `KnownBrands`) — min-length с **обеих** сторон. На 5-6-рунных строках одна замена даёт 80-83% similarity, что превращает любой случайный домен в typosquat соседа. См. `MinBrandImpersonationLength` (нашёл сабагент в ревью Task 5).

### Стиль тестов

- **Table-driven** для основной публичной функции.
- **Минимум один property-style тест**: параметрическая проверка границы (длина, порог) или инвариант (например, «любая подмена символа на не-hex ломает признак»).
- **Trap-кейсы** в таблице — вход, на котором наивная имплементация залажала бы. Каждый с комментарием **почему**: «HasSuffix без точки тут бы лжесработал», «частичный prefix-lookup тут бы прошёл», и т.п.
- **Invariant-тест для list-based сигналов**: если сигнал основан на handcrafted списке (`RiskyTLDs`, `KnownBrands`) — отдельный тест проверяет внутренние свойства списка: нормализацию ключей и (для similarity-сигналов) отсутствие undocumented коллизий между элементами. Опечатка в данных runtime'ом часто не вылавливается — equality просто промахивается, а легитимный элемент начинает совпадать «по similarity» с другим. См. `TestKnownBrands_NoUndocumentedCollisions` + allowlist `knownBrandCollisions`.
- **Интеграция с `CollectSuggest`**, минимум 3 кейса:
  1. Только новый сигнал → не suggest (под порогом).
  2. Сигнал + другие = точная сумма score (через константы), проверка наличия `ReasonX` в `Reason`.
  3. Регрессия на `=` vs `+=`: 3+ сигналов вместе, точная сумма (страховка от классического бага из commit `a7cb4c8`).

## Phase 1 — осталось

### Task 4 — Punycode / homograph

Лейбл, который после декодирования из punycode содержит символы из **более чем одного** алфавитного скрипта (Latin + Cyrillic — классика фишинга).

**Score**: +10 (равно hex-uuid; высокое качество, но soft).

**Reason**: `"label contains a mixed-script homograph"`.

**Примеры поведения**:

| Вход | Декодированное | Ожидание | Почему |
|---|---|---|---|
| `xn--ggle-jum.com` | `gооgle.com` | true | Latin g/l/e + Cyrillic о = phishing |
| `xn--p1ai` | `.рф` | false | TLD-only, мы пропускаем последний лейбл |
| `xn--d1acpjx3f.xn--p1ai` | `яндекс.рф` | false | Только Cyrillic — один скрипт |
| `xn--bcher-kva.com` | `bücher.com` | false | Только Latin (с диакритиком) |
| `example.com` | — | false | Не IDN, без `xn--` префикса |

**Ресёрч (выполнен частично)**

1. ✅ `golang.org/x/net` уже в `go.mod` как **indirect**. Импорт `golang.org/x/net/idna` не требует новой зависимости — просто `go mod tidy` сделает его direct.

2. ❌ **Не выяснено**: в каком виде домены приходят в `AllowDomainEvent.Domain` — punycode (`xn--`) или уже декодированными в Unicode. От этого зависит, нужно ли вообще вызывать `idna.ToUnicode` в эвристике или достаточно сразу проверять Unicode-символы.

   **Точки для проверки в новом чате**:
   - DNS-обработчик в `dns/` — что записывается в QNAME из wire?
   - `allow-domain/business/use-cases/` — какой `domain` передаётся в `CreateAllowDomainEvent`?
   - Самый быстрый способ — добавить `dig` под Cyrillic-доменом через `./create-dns-request.sh` и посмотреть, что попадает в БД.

   Гипотеза: домены хранятся в ACE (`xn--`) форме, потому что DNS wire format использует ACE. Тогда `idna.ToUnicode` нужен.

**Дизайн (предварительный, корректировать после ресёрча)**

Файл `homograph.go`:
```
HasHomographLabel(domain string) bool   // public
looksLikeHomograph(label string) bool   // private
hasMixedScripts(s string) bool          // private
```

`hasMixedScripts` использует `unicode.RangeTable`:
- `unicode.Latin`
- `unicode.Cyrillic`
- `unicode.Greek`

Counter инкрементируется при наличии хотя бы одного rune из каждого. `unicode.Common` (цифры, дефисы, ZWJ) — neutral, не считается. Возврат: count > 1.

Расширение до Han / Arabic / Hebrew — по мере необходимости, отдельной задачей.

**Концерны**

1. **Сложность импла** заметно выше других Tier-1 сигналов (Unicode-таблицы, decode, error handling). Возможно, формально это уже «Phase 1.5».
2. **FP-риск критичен**: легитимные не-Latin домены (`xn--p1ai`, `xn--bcher-kva`) **не должны** триггерить. Эти кейсы — обязательная часть тестового плана.
3. **Malformed punycode**: `idna.ToUnicode` возвращает error — обрабатываем как «не triggered» (return false).

**Альтернативы (если `idna` нельзя использовать)**

- Имплементировать декодер RFC 3492 руками — ~50 строк, стабильно.
- Ограничиться **списком конкретных конфузаблов** (Cyrillic а/е/о/р/с/у/х) — менее общо, но без зависимостей. Проверка простой: ASCII-байт + наличие любого rune из списка → suspicious.

## Phase 2 — структурные изменения

Делать только если Phase 1 показала, что хочется продолжать. Без них Phase 1 уже даёт качественный апгрейд.

### 6. Калибровка через категории с потолками

С ≥ 5 сигналами наивная сумма уже двусчитывает коррелированные (hex-uuid + entropy на UUID-лейбле — два сигнала с одного источника информации). Решение — категории «лексика», «структура», «связь с заблок.», каждая с потолком.

**Делать только если** в продакшене обнаружим заметные FP. Сейчас не приоритет.

### 7. Whitelist как негативный сигнал

Bundle-снимок Tranco top-10k или Cisco Umbrella top-1M. Apex в whitelist → отбрасываем кандидата целиком (или большой минус). Лучшая страховка от FP.

Архитектурно: добавить файл-снимок в репозиторий, читать в map при старте. Обновление — manual через скрипт раз в полгода.

### 8. Markov bigrams вместо Shannon

Замена `IsDomainSuspicious` на bigram log-likelihood под распределением английского. Тренинг — оффлайн на Tranco top-1M, в рантайме 30 строк + таблица 26×26.

Профит: ловит «осмысленно выглядящий, но неанглийский» лейбл (что Shannon не ловит) и отбрасывает «равномерно распределённый, но английский» (false-positive Shannon'а).

## Phase 3 — DNS-специфичные

Уникальные возможности из-за положения резолвера. Серьёзный лифт, но потенциал большой.

### 9. CNAME-cloaking detection (киллер-фича)

`analytics.mysite.com → IN CNAME → trkr.adnxs.com` — URL-блокировщики не видят `adnxs`, наш резолвер видит. Эвристика: пройти CNAME-цепочку до конца, проверить каждый элемент против `BlockList`. Хотя бы один в блоклисте → apex-источник в suggest с **очень высоким** score (прямое доказательство).

Стоимость: затрагивает `dns/`. Резолвер уже разрешает CNAME — нужно сохранять цепочку и пробрасывать в suggest-pipeline.

### 10–12

- **Apex-cardinality**: счётчик уникальных subdomain'ов под одним apex за окно. >50 случайных subdomain'ов → флаг apex.
- **NS-record sharing** с заблокированными: один NS у 50 заблоченных доменов → та же инфраструктура.
- **Fast-flux / TTL**: TTL < 60 + смена IP при каждом разрешении = почти определённо C2/ботнет.

## Что НЕ делать

- Не вводить `ItemScore*` / `Reason*` в feature-файлах — только в `collect.go`.
- Не «улучшать» существующие сигналы попутно — каждое изменение в отдельной задаче, отдельный коммит.
- Не пушить (`git push`) автоматически — пользователь делает это сам.
- Не использовать магические строки/числа в тестах — только константы.
- Не использовать `TrimSuffix(d, ".")` для нормализации trailing dots — только `TrimRight`.
