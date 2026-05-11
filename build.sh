#!/usr/bin/env bash
# Deploy dns-filter on the host.
#
# Usage: build.sh [GIT_TAG]
#   GIT_TAG задан    → чекаут этого тега
#   GIT_TAG не задан → последний main
#
# Конвенции:
# - .env в репозитории игнорируется (см. .gitignore). Боевые значения и API-ключи
#   живут в /home/balamut/secrets/dns-filter.env. Скрипт копирует их в
#   $PROJECT_DIR/.env перед сборкой, потому что Dockerfile.backend делает
#   COPY .env в образ.
# - Worktree приводится к origin/<ref> через git reset --hard, любые локальные
#   правки в проекте затираются. Все долговременные данные — в docker volume
#   dns-filter-data, не в worktree.
set -euo pipefail

# --- Конфигурация ---
PROJECT_DIR="/home/balamut/projects/dns-filter"
SECRET_ENV="/home/balamut/secrets/dns-filter.env"
LOCK_FILE="/tmp/dns-filter-deploy.lock"
HEALTH_TIMEOUT=90    # сек на старт контейнеров

# --- Вывод ---
if [[ -t 1 ]]; then
    RED=$'\033[0;31m'; GREEN=$'\033[0;32m'; YELLOW=$'\033[1;33m'
    BLUE=$'\033[0;34m'; NC=$'\033[0m'
else
    RED=''; GREEN=''; YELLOW=''; BLUE=''; NC=''
fi
log()  { printf '%s[%s]%s %s\n' "$BLUE"   "$(date +%H:%M:%S)" "$NC" "$*"; }
ok()   { printf '%s[%s] ✅%s %s\n' "$GREEN"  "$(date +%H:%M:%S)" "$NC" "$*"; }
warn() { printf '%s[%s] ⚠️%s  %s\n' "$YELLOW" "$(date +%H:%M:%S)" "$NC" "$*"; }
err()  { printf '%s[%s] ❌%s %s\n' "$RED"    "$(date +%H:%M:%S)" "$NC" "$*" >&2; }

# Защита от параллельного запуска.
exec 9>"$LOCK_FILE"
if ! flock -n 9; then
    err "Другой деплой уже идёт (lock: $LOCK_FILE)."
    exit 1
fi

TARGET_TAG="${1:-}"

# --- Pre-flight checks ---
[[ -d "$PROJECT_DIR/.git" ]] || { err "$PROJECT_DIR не является git-репозиторием"; exit 1; }
[[ -f "$SECRET_ENV" ]]      || { err "Нет $SECRET_ENV — положи туда боевой .env с ключами."; exit 1; }
command -v docker >/dev/null      || { err "docker не найден в PATH"; exit 1; }
docker compose version >/dev/null 2>&1 || { err "docker compose v2 не найден"; exit 1; }

cd "$PROJECT_DIR"

# --- Git: fetch + hard reset на нужный ref ---
log "git fetch --all --tags --prune..."
git fetch --all --tags --prune --quiet

if [[ -n "$TARGET_TAG" ]]; then
    if ! git rev-parse --verify --quiet "refs/tags/$TARGET_TAG" >/dev/null; then
        err "Тег '$TARGET_TAG' не найден."
        warn "Последние 5 тегов:"
        git tag --sort=-creatordate | head -n 5
        exit 1
    fi
    TARGET_REF="refs/tags/$TARGET_TAG"
    log "Чекаут тега $TARGET_TAG..."
else
    TARGET_REF="origin/main"
    log "Тег не указан — берём origin/main."
fi

# reset --hard + clean -fd: гарантированно приводим worktree к целевому ref,
# удаляя любые tracked-правки и untracked-мусор. Игнорируемые файлы (.env, data/)
# не трогаются — это намеренно.
git reset --hard --quiet "$TARGET_REF"
git clean -fd --quiet

CURRENT_REF=$(git describe --tags --always --dirty)
COMMIT_SHA=$(git rev-parse --short HEAD)
ok "Чекаут готов: $CURRENT_REF ($COMMIT_SHA)"

# --- Sanity-check секретного .env по шаблону .env.example ---
TEMPLATE=".env.example"
if [[ -f "$TEMPLATE" ]]; then
    missing=()
    while IFS= read -r key; do
        [[ -n "$key" ]] || continue
        grep -qE "^${key}=" "$SECRET_ENV" || missing+=("$key")
    done < <(grep -E '^[A-Z][A-Z0-9_]*=' "$TEMPLATE" | cut -d= -f1)
    if (( ${#missing[@]} > 0 )); then
        warn "В $SECRET_ENV отсутствуют ключи из $TEMPLATE:"
        printf '  - %s\n' "${missing[@]}"
        warn "Они окажутся пустыми в образе. Допиши их при необходимости."
    else
        ok "Все ключи из $TEMPLATE присутствуют в secret .env."
    fi
else
    warn "$TEMPLATE отсутствует в репо — sanity-check пропущен."
fi

# --- Подмена .env ---
log "Копируем $SECRET_ENV → $PROJECT_DIR/.env"
install -m 600 "$SECRET_ENV" "$PROJECT_DIR/.env"

# --- Сборка ---
log "docker compose build..."
docker compose build

# --- Рестарт ---
log "Останавливаем старые контейнеры..."
docker compose down --remove-orphans
log "Стартуем новые..."
docker compose up -d

# --- Healthcheck ---
log "Ждём готовности (до ${HEALTH_TIMEOUT}s)..."
deadline=$(( $(date +%s) + HEALTH_TIMEOUT ))
http_ok=0; dns_ok=0
while (( $(date +%s) < deadline )); do
    if (( !http_ok )) && timeout 2 bash -c '</dev/tcp/127.0.0.1/8080' >/dev/null 2>&1; then
        http_ok=1; ok "HTTP API :8080 отвечает."
    fi
    if (( !dns_ok )) && timeout 2 bash -c '</dev/tcp/127.0.0.1/53' >/dev/null 2>&1; then
        dns_ok=1; ok "DNS :53/tcp слушает."
    fi
    (( http_ok && dns_ok )) && break
    sleep 1
done

if (( !http_ok || !dns_ok )); then
    err "Healthcheck не прошёл: http_ok=$http_ok dns_ok=$dns_ok"
    warn "Последние 50 строк логов dns-filter:"
    docker compose logs --tail=50 dns-filter || true
    exit 1
fi

# --- Очистка ---
log "docker image prune (dangling)..."
docker image prune -f >/dev/null || true

ok "🚀 Deploy готов. Ref: $CURRENT_REF ($COMMIT_SHA)"
