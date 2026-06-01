#!/usr/bin/env bash
#
# deploy.sh — portable, interactive Docker installer for dns-filter.
#
# Anyone can clone the repo and run `./deploy.sh` to get the full stack running
# under Docker Compose without editing any file by hand. The script:
#   * checks prerequisites (Docker, Compose v2, Linux, daemon access),
#   * detects port 53 conflicts and offers to free systemd-resolved,
#   * asks for every required/optional setting (admin creds, DoH upstream,
#     optional reputation API keys, metrics, retention, …) with sane defaults,
#   * writes a chmod-600 .env that docker-compose.yml consumes via env_file,
#   * builds and starts the stack, then prints how to reach it.
#
# Subcommands:
#   install   (default) first-time interactive setup + build + up
#   update    git pull + rebuild + up (portable replacement for build.sh)
#   status    docker compose ps
#   logs      docker compose logs -f
#   down      stop the stack
#
# Automation / tests: set DF_NONINTERACTIVE=1 and supply answers via DF_*
# variables (DF_ADMIN_PASSWORD, DF_UI_PORT, …). DF_DRY_RUN=1 writes the .env but
# skips Docker and any host changes (systemd-resolved, resolv.conf). DF_ENV_FILE
# overrides the output path (default: <repo>/.env). See deploy_test.sh.

set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
ENV_FILE=${DF_ENV_FILE:-"$SCRIPT_DIR/.env"}
DC="docker compose"
# Guard with `if` (not a bare `&&` list): a bare `cmd1 && cmd2` whose first test
# is false returns non-zero, which under `set -e` would abort the script — that
# happens when running as root, where `id -u -ne 0` is false.
SUDO=""
if [ "$(id -u)" -ne 0 ] && command -v sudo >/dev/null 2>&1; then SUDO="sudo"; fi

# ---- pretty output -----------------------------------------------------------
if [ -t 1 ]; then
  C_RED=$'\033[31m'; C_GRN=$'\033[32m'; C_YEL=$'\033[33m'; C_BLU=$'\033[34m'; C_RST=$'\033[0m'
else
  C_RED=""; C_GRN=""; C_YEL=""; C_BLU=""; C_RST=""
fi
info() { printf '%s==>%s %s\n' "$C_BLU" "$C_RST" "$*"; }
ok()   { printf '%s ok %s %s\n' "$C_GRN" "$C_RST" "$*"; }
warn() { printf '%s!! %s %s\n' "$C_YEL" "$C_RST" "$*" >&2; }
err()  { printf '%sxx %s %s\n' "$C_RED" "$C_RST" "$*" >&2; }
die()  { err "$*"; exit 1; }

# ---- small helpers -----------------------------------------------------------
require_cmd() { command -v "$1" >/dev/null 2>&1 || die "Required command not found: $1"; }
is_port()   { [[ $1 =~ ^[0-9]+$ ]] && [ "$1" -ge 1 ] && [ "$1" -le 65535 ]; }
is_posint() { [[ $1 =~ ^[0-9]+$ ]] && [ "$1" -ge 1 ]; }
lower()     { printf '%s' "$1" | tr '[:upper:]' '[:lower:]'; }

# version_ge A B — true when version A >= version B (dotted numeric).
version_ge() { [ "$(printf '%s\n%s\n' "$2" "$1" | sort -V | head -n1)" = "$2" ]; }

# env_get KEY FILE — print the value of KEY from a generated .env WITHOUT
# sourcing it. Sourcing (`. file`) would execute `$(...)`/backticks and
# word-split spaces in a password or API key (and abort under `set -u`); this
# reads the literal text after the first `=` from the `KEY=value` lines that
# write_env emits, so any character round-trips safely.
env_get() {
  local key=$1 file=$2 line
  line=$(grep -E "^${key}=" "$file" 2>/dev/null | tail -n1) || true
  printf '%s' "${line#*=}"
}

gen_password() {
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -base64 18 | tr -d '/+=' | cut -c1-20
  else
    head -c 18 /dev/urandom | base64 | tr -d '/+=' | cut -c1-20
  fi
}

# ask VAR "Prompt" "default" — prompt with a default; non-interactive keeps the
# value already in VAR (or the default) and does not read from the terminal.
ask() {
  local __var=$1 __prompt=$2 __def=${3:-}
  local __cur=${!__var:-}
  if [ "${DF_NONINTERACTIVE:-0}" = "1" ]; then
    printf -v "$__var" '%s' "${__cur:-$__def}"
    return
  fi
  local __shown=${__cur:-$__def} __input
  read -r -p "$__prompt [${__shown}]: " __input || true
  printf -v "$__var" '%s' "${__input:-${__cur:-$__def}}"
}

# ask_secret VAR "Prompt" — silent input with confirmation; empty auto-generates.
ask_secret() {
  local __var=$1 __prompt=$2
  if [ "${DF_NONINTERACTIVE:-0}" = "1" ]; then
    printf -v "$__var" '%s' "${!__var:-}"
    return
  fi
  local p1 p2
  while :; do
    read -rs -p "$__prompt (empty = auto-generate): " p1; echo
    if [ -z "$p1" ]; then
      p1=$(gen_password); ok "Generated password: $p1"
      printf -v "$__var" '%s' "$p1"; return
    fi
    read -rs -p "Repeat password: " p2; echo
    [ "$p1" = "$p2" ] && { printf -v "$__var" '%s' "$p1"; return; }
    warn "Passwords do not match, try again."
  done
}

# ask_yes_no "Prompt" default(y/n) — returns 0 for yes. Non-interactive uses default.
ask_yes_no() {
  local prompt=$1 def=${2:-n} ans
  if [ "${DF_NONINTERACTIVE:-0}" = "1" ]; then
    [ "$def" = "y" ]; return
  fi
  local hint="[y/N]"; [ "$def" = "y" ] && hint="[Y/n]"
  read -r -p "$prompt $hint: " ans || true
  ans=${ans:-$def}
  case "$(lower "$ans")" in y|yes) return 0;; *) return 1;; esac
}

# ---- preflight ---------------------------------------------------------------
preflight() {
  # In dry-run we never touch Docker, so don't require it (keeps tests hermetic).
  if [ "${DF_DRY_RUN:-0}" = "1" ]; then DC="docker compose"; return; fi

  [ "$(uname -s)" = "Linux" ] || warn "This stack uses Docker host networking and targets Linux. On macOS/Windows host networking is partial and :53 may not bind."
  require_cmd docker
  # The compose file uses the long-form `env_file: [{path, required}]`, which
  # Docker Compose only understands from v2.24.0. Legacy `docker-compose` (v1)
  # and older v2 plugins fail to PARSE it, so there is no safe fallback — require
  # a recent v2 plugin and fail with a clear message otherwise.
  docker compose version >/dev/null 2>&1 || die "Docker Compose v2 (the 'docker compose' plugin) is required (>= 2.24.0). Install/upgrade Docker Compose."
  DC="docker compose"
  local cv
  cv=$(docker compose version --short 2>/dev/null | sed 's/^v//') || cv=""
  if [ -n "$cv" ] && ! version_ge "$cv" 2.24.0; then
    die "Docker Compose $cv is too old; this stack needs >= 2.24.0 (env_file long-form). Upgrade Docker Compose."
  fi
  docker info >/dev/null 2>&1 || die "Cannot reach the Docker daemon. Is it running, and do you have permission? (try sudo, or add your user to the 'docker' group)"
}

# ---- port 53 / systemd-resolved ---------------------------------------------
# detect_port_53 probes BOTH UDP and TCP — the dns-filter container binds :53 on
# both, so a TCP-only listener (e.g. the resolved TCP stub) must be detected too.
# Uses `sudo -n` so it never blocks on a password prompt (keeps tests hermetic).
detect_port_53() {
  local sudo=""
  if [ -n "$SUDO" ]; then sudo="sudo -n"; fi
  if command -v ss >/dev/null 2>&1; then
    $sudo ss -lutnpH 'sport = :53' 2>/dev/null
  elif command -v lsof >/dev/null 2>&1; then
    $sudo lsof -nP -iTCP:53 -sTCP:LISTEN -iUDP:53 2>/dev/null | awk 'NR>1'
  fi
}

free_systemd_resolved() {
  info "Disabling systemd-resolved DNS stub listener…"
  $SUDO mkdir -p /etc/systemd/resolved.conf.d
  printf '[Resolve]\nDNSStubListener=no\n' | $SUDO tee /etc/systemd/resolved.conf.d/dns-filter.conf >/dev/null
  # Back up the current resolv.conf, following symlinks (-L) so we capture the
  # real content even when /etc/resolv.conf is the usual symlink to the stub.
  if [ -e /etc/resolv.conf ]; then
    $SUDO cp -aL /etc/resolv.conf /etc/resolv.conf.dns-filter.bak 2>/dev/null || true
    info "Backed up /etc/resolv.conf -> /etc/resolv.conf.dns-filter.bak"
  fi
  # Repoint the host at a WORKING resolver before disabling the stub — otherwise
  # /etc/resolv.conf would keep pointing at the now-dead 127.0.0.53 and the host
  # could not resolve anything. Prefer systemd's uplink file; if it is absent,
  # write a public fallback so the host always keeps a usable resolver.
  if [ -e /run/systemd/resolve/resolv.conf ]; then
    $SUDO ln -sf /run/systemd/resolve/resolv.conf /etc/resolv.conf
  else
    warn "No /run/systemd/resolve/resolv.conf found; writing a public fallback resolver to /etc/resolv.conf so the host keeps resolving."
    $SUDO rm -f /etc/resolv.conf
    printf 'nameserver 1.1.1.1\nnameserver 1.0.0.1\n' | $SUDO tee /etc/resolv.conf >/dev/null
  fi
  $SUDO systemctl restart systemd-resolved
  ok "systemd-resolved stub disabled; :53 is now free."
}

check_port_53() {
  # Skip entirely in dry-run: this is read-only, but the probe may use sudo and
  # we want tests to stay hermetic and host-independent.
  if [ "${DF_DRY_RUN:-0}" = "1" ]; then info "[dry-run] skipping port 53 check."; return; fi
  local listener; listener=$(detect_port_53 || true)
  if [ -z "$listener" ]; then ok "Port 53 appears free."; return; fi
  warn "Port 53 is already in use:"
  printf '%s\n' "$listener" >&2
  if printf '%s' "$listener" | grep -qi 'systemd-resolve'; then
    # Freeing systemd-resolved rewrites host DNS config — never do it silently
    # in non-interactive mode; require an explicit DF_FREE_PORT_53=1 opt-in.
    if [ "${DF_NONINTERACTIVE:-0}" = "1" ]; then
      if [ "${DF_FREE_PORT_53:-0}" = "1" ]; then
        free_systemd_resolved
      else
        warn "systemd-resolved holds :53. Re-run with DF_FREE_PORT_53=1 to free it automatically, or free it manually — the container will fail to bind until then."
      fi
      return
    fi
    if ask_yes_no "Disable systemd-resolved's stub listener to free :53?" "y"; then
      free_systemd_resolved
    else
      warn "Leaving :53 occupied — the dns-filter container will fail to bind until you free it."
    fi
  else
    warn "Stop whatever is bound to :53 before starting, or the container will fail to bind."
  fi
}

# ---- configuration -----------------------------------------------------------
collect_config() {
  ADMIN_LOGIN=${DF_ADMIN_LOGIN:-admin}
  ask ADMIN_LOGIN "Admin login" "admin"

  ADMIN_PASSWORD=${DF_ADMIN_PASSWORD:-}
  ask_secret ADMIN_PASSWORD "Admin password"
  [ -n "$ADMIN_PASSWORD" ] || die "Admin password is required (set DF_ADMIN_PASSWORD in non-interactive mode)."

  UI_PORT=${DF_UI_PORT:-8090}
  ask UI_PORT "Web UI host port" "8090"
  is_port "$UI_PORT" || die "Invalid UI port: $UI_PORT"

  MODE=${DF_MODE:-lan}
  ask MODE "DNS mode (lan/public)" "lan"
  case "$MODE" in lan|public) ;; *) die "Invalid mode: $MODE (expected lan|public)";; esac

  DOH_UPSTREAM=${DF_DOH_UPSTREAM:-https://cloudflare-dns.com/dns-query}
  ask DOH_UPSTREAM "DoH upstream URL" "https://cloudflare-dns.com/dns-query"

  BOOTSTRAP_IPS=${DF_BOOTSTRAP_IPS:-1.1.1.1,1.0.0.1}
  ask BOOTSTRAP_IPS "DoH bootstrap IPs (comma-separated)" "1.1.1.1,1.0.0.1"

  LOG_LEVEL=${DF_LOG_LEVEL:-info}
  ask LOG_LEVEL "Log level (debug/info/warn/error)" "info"

  RETENTION_DAYS=${DF_RETENTION_DAYS:-30}
  ask RETENTION_DAYS "Traffic retention (days)" "30"
  is_posint "$RETENTION_DAYS" || die "Invalid retention days: $RETENTION_DAYS"

  # HTTPS cookie hardening (only relevant if you put the UI behind TLS).
  COOKIE_SECURE=${DF_COOKIE_SECURE:-false}
  COOKIE_SAMESITE=${DF_COOKIE_SAMESITE:-Lax}
  if [ "${DF_NONINTERACTIVE:-0}" != "1" ]; then
    if ask_yes_no "Will you serve the UI over HTTPS (enable secure cookies)?" "n"; then
      COOKIE_SECURE=true
    fi
  fi

  # Prometheus metrics endpoint.
  METRIC_ENABLE=${DF_METRIC_ENABLE:-false}
  METRIC_PORT=${DF_METRIC_PORT:-2112}
  if [ "${DF_NONINTERACTIVE:-0}" != "1" ]; then
    if ask_yes_no "Enable the Prometheus metrics endpoint?" "n"; then
      METRIC_ENABLE=true
      ask METRIC_PORT "Metrics port" "2112"
    fi
  fi
  if [ "$METRIC_ENABLE" = "true" ]; then
    is_port "$METRIC_PORT" || die "Invalid metrics port: $METRIC_PORT"
  fi

  # Optional reputation enrichment for block suggestions.
  SUGGEST_INSPECT_ENABLED=${DF_SUGGEST_INSPECT_ENABLED:-false}
  VT_KEY=${DF_VT_KEY:-}
  SB_KEY=${DF_SB_KEY:-}
  URLSCAN_KEY=${DF_URLSCAN_KEY:-}
  if [ "${DF_NONINTERACTIVE:-0}" != "1" ]; then
    cat <<'TXT'

Optional — reputation enrichment for block suggestions.
This sends observed domains to third-party services (VirusTotal / Google Safe
Browsing / urlscan.io) to score weak candidates. It is OFF by default and only
runs if enabled AND a VirusTotal or Safe Browsing key is provided.
TXT
    if ask_yes_no "Set up reputation enrichment and enter API keys?" "n"; then
      if ask_yes_no "Do you have a VirusTotal API key?" "n"; then ask VT_KEY "  VirusTotal key" ""; fi
      if ask_yes_no "Do you have a Google Safe Browsing API key?" "n"; then ask SB_KEY "  Safe Browsing key" ""; fi
      # urlscan.io feeds the on-demand /inspect endpoint, NOT the suggest worker
      # (see config/config.go), so it does not enable suggest-inspect.
      if ask_yes_no "Do you have a urlscan.io API key? (used by on-demand /inspect, not the suggest worker)" "n"; then ask URLSCAN_KEY "  urlscan.io key" ""; fi
    fi
  fi
  # Auto-enable suggest-inspect when a VirusTotal or Safe Browsing key is present
  # (the only providers the worker actually uses). This runs in BOTH interactive
  # and non-interactive modes, so `DF_VT_KEY=...` alone turns it on. A urlscan
  # key does NOT flip it. Respect an explicit DF_SUGGEST_INSPECT_ENABLED=true.
  if [ "$SUGGEST_INSPECT_ENABLED" != "true" ] && [ -n "${VT_KEY}${SB_KEY}" ]; then
    SUGGEST_INSPECT_ENABLED=true
  fi
  if [ "$SUGGEST_INSPECT_ENABLED" = "true" ] && [ -z "${VT_KEY}${SB_KEY}" ]; then
    warn "suggest-inspect is enabled but no VirusTotal/Safe Browsing key is set — the worker will no-op until you add one."
  fi
}

write_env() {
  umask 077
  cat > "$ENV_FILE" <<EOF
# Generated by deploy.sh on $(date -u +%Y-%m-%dT%H:%M:%SZ).
# Contains the admin password and any API keys — DO NOT COMMIT (it is gitignored).
# Re-run ./deploy.sh to change these values.

DNS_FILTER_MODE=$MODE
DNS_FILTER_DOH_UPSTREAM=$DOH_UPSTREAM
DNS_FILTER_DOH_BOOTSTRAP_IPS=$BOOTSTRAP_IPS
DNS_FILTER_DBPATH=./data/filter.sqlite

DNS_FILTER_ADMIN_LOGIN=$ADMIN_LOGIN
DNS_FILTER_ADMIN_PASSWORD=$ADMIN_PASSWORD
DNS_FILTER_COOKIE_SECURE=$COOKIE_SECURE
DNS_FILTER_COOKIE_SAMESITE=$COOKIE_SAMESITE

DNS_FILTER_LOG_LEVEL=$LOG_LEVEL
DNS_FILTER_TRAFFIC_RETENTION_DAYS=$RETENTION_DAYS

DNS_FILTER_METRIC_ENABLE=$METRIC_ENABLE
DNS_FILTER_METRIC_PORT=$METRIC_PORT

DNS_FILTER_SUGGEST_INSPECT_ENABLED=$SUGGEST_INSPECT_ENABLED
DNS_FILTER_VT_KEY=$VT_KEY
DNS_FILTER_SAFE_BROWSING_KEY=$SB_KEY
DNS_FILTER_URLSCAN_KEY=$URLSCAN_KEY

# Web UI host port — read by docker-compose.yml interpolation.
DNS_FILTER_UI_PORT=$UI_PORT
EOF
  chmod 600 "$ENV_FILE"
  ok "Wrote $ENV_FILE"
}

# ---- compose lifecycle -------------------------------------------------------
compose_build_up() {
  info "Building images (first build can take a few minutes)…"
  $DC build
  info "Starting the stack…"
  $DC up -d
  # Actively probe readiness instead of just checking the container is listed —
  # a container crash-looping on a :53 bind error still appears in `ps`. Under
  # host networking the backend binds host :8080 and :53 directly.
  info "Waiting for the backend to become ready (up to 90s)…"
  local deadline=$(( SECONDS + 90 )) http_ok=0 dns_ok=0
  while [ "$SECONDS" -lt "$deadline" ]; do
    if [ "$http_ok" -eq 0 ] && timeout 2 bash -c '</dev/tcp/127.0.0.1/8080' >/dev/null 2>&1; then
      http_ok=1; ok "HTTP API :8080 is up."
    fi
    if [ "$dns_ok" -eq 0 ] && timeout 2 bash -c '</dev/tcp/127.0.0.1/53' >/dev/null 2>&1; then
      dns_ok=1; ok "DNS :53/tcp is up."
    fi
    [ "$http_ok" -eq 1 ] && [ "$dns_ok" -eq 1 ] && break
    sleep 1
  done
  if [ "$http_ok" -eq 0 ] || [ "$dns_ok" -eq 0 ]; then
    warn "Backend did not become ready (http_ok=$http_ok dns_ok=$dns_ok). Recent logs:"
    $DC logs --tail=40 dns-filter || true
    warn "If you see a bind error on :53, free the port and re-run: ./deploy.sh update"
  fi
}

summary() {
  local ip; ip=$(hostname -I 2>/dev/null | awk '{print $1}') || true
  ip=${ip:-<host-ip>}
  echo
  ok "dns-filter stack is up."
  cat <<TXT
  Web UI:  http://${ip}:${UI_PORT}   (login: ${ADMIN_LOGIN})
  DNS:     point a device's DNS server at ${ip}:53 (UDP+TCP)
  Data:    Docker volume 'dns-filter-data' (SQLite at /app/data/filter.sqlite)

  Manage:
    ./deploy.sh status     # container status
    ./deploy.sh logs       # tail logs
    ./deploy.sh update     # pull latest + rebuild + restart
    ./deploy.sh down       # stop the stack
TXT
}

# ---- subcommands -------------------------------------------------------------
cmd_install() {
  preflight
  local skip_collect=0
  if [ -f "$ENV_FILE" ] && [ "${DF_NONINTERACTIVE:-0}" != "1" ]; then
    warn "$ENV_FILE already exists."
    if ask_yes_no "Reconfigure and overwrite it?" "n"; then
      skip_collect=0
    else
      info "Keeping existing .env; rebuilding with current settings."
      skip_collect=1
    fi
  fi
  if [ "$skip_collect" = "1" ]; then
    # Read back only the two values the summary needs, WITHOUT sourcing .env
    # (see env_get — sourcing would execute/word-split secret values).
    ADMIN_LOGIN=$(env_get DNS_FILTER_ADMIN_LOGIN "$ENV_FILE"); ADMIN_LOGIN=${ADMIN_LOGIN:-admin}
    UI_PORT=$(env_get DNS_FILTER_UI_PORT "$ENV_FILE"); UI_PORT=${UI_PORT:-8090}
  else
    collect_config
    write_env
  fi
  check_port_53
  if [ "${DF_DRY_RUN:-0}" = "1" ]; then ok "[dry-run] skipping docker build/up."; return; fi
  compose_build_up
  summary
}

cmd_update() {
  preflight
  [ -f "$ENV_FILE" ] || die "No .env found — run ./deploy.sh install first."
  if [ "${DF_DRY_RUN:-0}" != "1" ] && command -v git >/dev/null 2>&1 && [ -d "$SCRIPT_DIR/.git" ]; then
    info "Pulling latest changes…"
    git -C "$SCRIPT_DIR" pull --ff-only || warn "git pull failed/skipped — building current checkout."
  fi
  # Read back only the summary values, WITHOUT sourcing .env (see env_get).
  # Compose itself reads .env for ${...} interpolation from the project dir.
  ADMIN_LOGIN=$(env_get DNS_FILTER_ADMIN_LOGIN "$ENV_FILE"); ADMIN_LOGIN=${ADMIN_LOGIN:-admin}
  UI_PORT=$(env_get DNS_FILTER_UI_PORT "$ENV_FILE"); UI_PORT=${UI_PORT:-8090}
  if [ "${DF_DRY_RUN:-0}" = "1" ]; then ok "[dry-run] skipping docker build/up."; return; fi
  compose_build_up
  docker image prune -f >/dev/null 2>&1 || true
  summary
}

usage() {
  cat <<TXT
dns-filter deploy script

Usage: ./deploy.sh [command]

Commands:
  install   (default) interactive first-time setup, then build & start
  update    git pull, rebuild images, restart the stack
  status    show container status
  logs      follow container logs
  down      stop the stack
  help      show this help
TXT
}

main() {
  cd "$SCRIPT_DIR"
  case "${1:-install}" in
    install)        cmd_install ;;
    update)         cmd_update ;;
    status)         preflight; $DC ps ;;
    logs)           preflight; $DC logs -f --tail=100 ;;
    down)           preflight; $DC down ;;
    -h|--help|help) usage ;;
    *)              err "Unknown command: $1"; usage; exit 1 ;;
  esac
}

main "$@"
