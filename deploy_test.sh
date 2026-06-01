#!/usr/bin/env bash
#
# deploy_test.sh — hermetic tests for deploy.sh.
#
# Runs the installer in DF_DRY_RUN + DF_NONINTERACTIVE mode so it generates a
# .env (to a temp path) without touching Docker, the network, or the host. Covers
# the happy path, validation failures, the env_file round-trip (incl. shell-
# special characters), the no-source read-back safety, and the update path.
# No Docker required; safe to run in CI.

set -uo pipefail

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
DEPLOY="$SCRIPT_DIR/deploy.sh"
pass=0; fail=0

check() { # check "name" 'test-expression'
  if eval "$2"; then
    printf 'ok   - %s\n' "$1"; pass=$((pass+1))
  else
    printf 'FAIL - %s\n' "$1"; fail=$((fail+1))
  fi
}

file_mode() { stat -c '%a' "$1" 2>/dev/null || stat -f '%Lp' "$1"; }

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

# --- 1. Happy path: full config produces a complete, locked-down .env ---------
out="$tmp/happy.env"
DF_NONINTERACTIVE=1 DF_DRY_RUN=1 DF_ENV_FILE="$out" \
  DF_ADMIN_LOGIN=boss DF_ADMIN_PASSWORD=s3cret DF_UI_PORT=9000 DF_MODE=lan \
  DF_RETENTION_DAYS=45 DF_SUGGEST_INSPECT_ENABLED=true DF_VT_KEY=vtkey \
  bash "$DEPLOY" install >"$tmp/happy.out" 2>&1
rc=$?
check "happy path exits 0"                "[ $rc -eq 0 ]"
check "happy .env created"                "[ -f '$out' ]"
check "admin login written"              "grep -q '^DNS_FILTER_ADMIN_LOGIN=boss\$' '$out'"
check "admin password written"           "grep -q '^DNS_FILTER_ADMIN_PASSWORD=s3cret\$' '$out'"
check "ui port written"                  "grep -q '^DNS_FILTER_UI_PORT=9000\$' '$out'"
check "retention written"                "grep -q '^DNS_FILTER_TRAFFIC_RETENTION_DAYS=45\$' '$out'"
check "vt key written"                   "grep -q '^DNS_FILTER_VT_KEY=vtkey\$' '$out'"
check "inspect enabled"                  "grep -q '^DNS_FILTER_SUGGEST_INSPECT_ENABLED=true\$' '$out'"
check "default DoH upstream written"     "grep -q '^DNS_FILTER_DOH_UPSTREAM=https://cloudflare-dns.com/dns-query\$' '$out'"
check ".env is chmod 600"                "[ \"\$(file_mode '$out')\" = 600 ]"
check "dry-run skipped docker"           "grep -q 'dry-run' '$tmp/happy.out'"

# --- 2. Negative: missing admin password is rejected, no .env left behind -----
out2="$tmp/nopass.env"
DF_NONINTERACTIVE=1 DF_DRY_RUN=1 DF_ENV_FILE="$out2" DF_ADMIN_PASSWORD="" \
  bash "$DEPLOY" install >"$tmp/nopass.out" 2>&1
rc=$?
check "missing password exits non-zero"  "[ $rc -ne 0 ]"
check "missing password writes no .env"  "[ ! -f '$out2' ]"
check "missing password error explains"  "grep -qi password '$tmp/nopass.out'"

# --- 3. Negative: out-of-range UI port is rejected ----------------------------
# NOTE: assert the SPECIFIC error, not just the word 'port' — the success path
# also prints a line containing 'port', which would mask a broken validation.
out3="$tmp/badport.env"
DF_NONINTERACTIVE=1 DF_DRY_RUN=1 DF_ENV_FILE="$out3" \
  DF_ADMIN_PASSWORD=x DF_UI_PORT=99999 \
  bash "$DEPLOY" install >"$tmp/badport.out" 2>&1
rc=$?
check "invalid port exits non-zero"      "[ $rc -ne 0 ]"
check "invalid port error explains"      "grep -qi 'invalid ui port' '$tmp/badport.out'"
check "invalid port writes no .env"      "[ ! -f '$out3' ]"

# --- 4. Negative: unknown DNS mode is rejected --------------------------------
out4="$tmp/badmode.env"
DF_NONINTERACTIVE=1 DF_DRY_RUN=1 DF_ENV_FILE="$out4" \
  DF_ADMIN_PASSWORD=x DF_MODE=banana \
  bash "$DEPLOY" install >"$tmp/badmode.out" 2>&1
rc=$?
check "invalid mode exits non-zero"      "[ $rc -ne 0 ]"
check "invalid mode error explains"      "grep -qi 'invalid mode' '$tmp/badmode.out'"

# --- 5. Negative: non-numeric retention is rejected ---------------------------
out5="$tmp/badret.env"
DF_NONINTERACTIVE=1 DF_DRY_RUN=1 DF_ENV_FILE="$out5" \
  DF_ADMIN_PASSWORD=x DF_RETENTION_DAYS=lots \
  bash "$DEPLOY" install >"$tmp/badret.out" 2>&1
rc=$?
check "invalid retention exits non-zero" "[ $rc -ne 0 ]"
check "invalid retention error explains" "grep -qi 'invalid retention' '$tmp/badret.out'"

# --- 6. Negative: bad metrics port rejected only when metrics enabled ---------
out6="$tmp/badmetric.env"
DF_NONINTERACTIVE=1 DF_DRY_RUN=1 DF_ENV_FILE="$out6" \
  DF_ADMIN_PASSWORD=x DF_METRIC_ENABLE=true DF_METRIC_PORT=99999 \
  bash "$DEPLOY" install >"$tmp/badmetric.out" 2>&1
rc=$?
check "invalid metrics port exits non-zero" "[ $rc -ne 0 ]"
check "invalid metrics port error explains" "grep -qi 'invalid metrics port' '$tmp/badmetric.out'"

# --- 7. Metrics positive round-trip -------------------------------------------
out7="$tmp/metric.env"
DF_NONINTERACTIVE=1 DF_DRY_RUN=1 DF_ENV_FILE="$out7" \
  DF_ADMIN_PASSWORD=x DF_METRIC_ENABLE=true DF_METRIC_PORT=2200 \
  bash "$DEPLOY" install >/dev/null 2>&1
check "metrics enable round-trips"       "grep -q '^DNS_FILTER_METRIC_ENABLE=true\$' '$out7'"
check "metrics port round-trips"         "grep -q '^DNS_FILTER_METRIC_PORT=2200\$' '$out7'"

# --- 8. Custom (non-default) values are honored, not ignored -------------------
out8="$tmp/custom.env"
DF_NONINTERACTIVE=1 DF_DRY_RUN=1 DF_ENV_FILE="$out8" \
  DF_ADMIN_PASSWORD=x DF_MODE=public DF_DOH_UPSTREAM=https://dns.google/dns-query \
  DF_LOG_LEVEL=warn \
  bash "$DEPLOY" install >/dev/null 2>&1
check "custom mode honored"              "grep -q '^DNS_FILTER_MODE=public\$' '$out8'"
check "custom DoH upstream honored"      "grep -q '^DNS_FILTER_DOH_UPSTREAM=https://dns.google/dns-query\$' '$out8'"
check "custom log level honored"         "grep -q '^DNS_FILTER_LOG_LEVEL=warn\$' '$out8'"

# --- 9. A VT key alone auto-enables suggest-inspect ---------------------------
out9="$tmp/vtonly.env"
DF_NONINTERACTIVE=1 DF_DRY_RUN=1 DF_ENV_FILE="$out9" \
  DF_ADMIN_PASSWORD=x DF_VT_KEY=abc123 \
  bash "$DEPLOY" install >/dev/null 2>&1
check "VT key auto-enables inspect"      "grep -q '^DNS_FILTER_SUGGEST_INSPECT_ENABLED=true\$' '$out9'"

# --- 10. A urlscan key alone does NOT enable suggest-inspect ------------------
out10="$tmp/urlscan.env"
DF_NONINTERACTIVE=1 DF_DRY_RUN=1 DF_ENV_FILE="$out10" \
  DF_ADMIN_PASSWORD=x DF_URLSCAN_KEY=us123 \
  bash "$DEPLOY" install >/dev/null 2>&1
check "urlscan key written"              "grep -q '^DNS_FILTER_URLSCAN_KEY=us123\$' '$out10'"
check "urlscan key does NOT enable inspect" "grep -q '^DNS_FILTER_SUGGEST_INSPECT_ENABLED=false\$' '$out10'"

# --- 11. Secret write is injection-safe (no command exec, value stored literally)
marker="$tmp/PWNED"
inj='a b$(touch '"$marker"')c'
out11="$tmp/inject.env"
DF_NONINTERACTIVE=1 DF_DRY_RUN=1 DF_ENV_FILE="$out11" \
  DF_ADMIN_PASSWORD="$inj" \
  bash "$DEPLOY" install >/dev/null 2>&1
check "write did not execute injected cmd"   "[ ! -f '$marker' ]"
check "password stored literally"            "grep -qF 'DNS_FILTER_ADMIN_PASSWORD=$inj' '$out11'"

# --- 12. env_get unit: returns literal value without executing it -------------
# Extract just the env_get function from deploy.sh and exercise it directly.
gmarker="$tmp/GET_PWNED"
cat > "$tmp/g.env" <<EOF
KEY=a b\$(touch $gmarker)c
EOF
val=$(
  eval "$(sed -n '/^env_get()/,/^}/p' "$DEPLOY")"
  env_get KEY "$tmp/g.env"
)
check "env_get did not execute value"    "[ ! -f '$gmarker' ]"
check "env_get returns literal value"    "[ \"\$val\" = 'a b\$(touch $gmarker)c' ]"

# --- 13. update without a prior .env is rejected ------------------------------
out13="$tmp/missing.env"
DF_NONINTERACTIVE=1 DF_DRY_RUN=1 DF_ENV_FILE="$out13" \
  bash "$DEPLOY" update >"$tmp/upd_missing.out" 2>&1
rc=$?
check "update without .env exits non-zero" "[ $rc -ne 0 ]"
check "update without .env error explains" "grep -qi 'no .env' '$tmp/upd_missing.out'"

# --- 14. update read-back is safe (does not source/execute the .env) ----------
# Reuse the injection .env from test 11; the marker must STILL not exist after
# update reads it back, and update must succeed.
rm -f "$marker"
DF_NONINTERACTIVE=1 DF_DRY_RUN=1 DF_ENV_FILE="$out11" \
  bash "$DEPLOY" update >"$tmp/upd.out" 2>&1
rc=$?
check "update exits 0 on valid .env"     "[ $rc -eq 0 ]"
check "update did not execute .env value" "[ ! -f '$marker' ]"

# --- 15. Unknown subcommand is rejected with usage (isolated env path) --------
DF_DRY_RUN=1 DF_ENV_FILE="$tmp/cmd.env" \
  bash "$DEPLOY" frobnicate >"$tmp/cmd.out" 2>&1
rc=$?
check "unknown command exits non-zero"   "[ $rc -ne 0 ]"
check "unknown command shows usage"      "grep -qi usage '$tmp/cmd.out'"
check "unknown command wrote no .env"    "[ ! -f '$tmp/cmd.env' ]"

echo
printf 'passed=%d failed=%d\n' "$pass" "$fail"
[ "$fail" -eq 0 ]
