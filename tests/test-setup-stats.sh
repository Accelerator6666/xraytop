#!/usr/bin/env bash
set -Eeuo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
TMP="$(mktemp -d)"
PROC_PID=""
cleanup() {
  if [[ -n "$PROC_PID" ]]; then
    kill "$PROC_PID" 2>/dev/null || true
    wait "$PROC_PID" 2>/dev/null || true
  fi
  rm -rf "$TMP"
}
trap cleanup EXIT
mkdir -p "$TMP/bin" "$TMP/state"
export TEST_STATE="$TMP/state"
cat > "$TMP/bin/systemctl" <<'MOCK'
#!/usr/bin/env bash
case "$1" in
  is-active) exit 0 ;;
  show) echo "$TEST_XRAY_PID" ;;
  restart) touch "$TEST_STATE/restarted" ;;
  *) exit 1 ;;
esac
MOCK
cat > "$TMP/bin/ss" <<'MOCK'
#!/usr/bin/env bash
exit 0
MOCK
cat > "$TMP/bin/xray" <<'MOCK'
#!/usr/bin/env bash
case "$1" in
  api)
    [[ "${TEST_API_FAIL:-0}" == 0 ]] &&
      jq -e '.api.listen == "127.0.0.1:10085"' "$TEST_CONFIG" >/dev/null
    ;;
  run)
    [[ "$2" == -test && "$3" == -format && "$4" == json && "$5" == -config ]] &&
      jq -e '.api.listen == "127.0.0.1:10085" and .inbounds[0].tag == "xraytop-inbound-0"' "$6" >/dev/null
    ;;
  *) exit 1 ;;
esac
MOCK
chmod +x "$TMP/bin/"*
export PATH="$TMP/bin:$PATH"
export TEST_CONFIG="$TMP/config.json"
cat > "$TEST_CONFIG" <<'JSON'
{
  "inbounds": [{"protocol":"vless", "listen":"/dev/shm/vless.socket", "settings":{"clients":[{"id":"secret"}]}}],
  "outbounds": [{"tag":"direct", "protocol":"freedom"}],
  "routing":{"rules": [{"type":"field", "outboundTag":"direct", "domain":["example.org"]}]}
}
JSON
cp "$TEST_CONFIG" "$TMP/original.json"
# Mock process includes test config on its command line.
python3 -c 'import time;time.sleep(60)' "$TEST_CONFIG" &
PROC_PID=$!
export TEST_XRAY_PID="$PROC_PID"
HELPER="$ROOT/scripts/setup-stats.sh"

bash "$HELPER" "$TEST_CONFIG" "$TMP/bin/xray" 127.0.0.1:10085 xray.service "$TMP/backups"
jq -e '.api.listen == "127.0.0.1:10085" and .inbounds[0].tag == "xraytop-inbound-0" and .outbounds[0].tag == "direct" and .routing.rules[0].domain[0] == "example.org" and .inbounds[0].settings.clients[0].id == "secret"' "$TEST_CONFIG" >/dev/null
[[ $(find "$TMP/backups" -type f | wc -l) -eq 1 ]]
cp "$TEST_CONFIG" "$TMP/updated.json"
rm -f "$TEST_STATE/restarted"
bash "$HELPER" "$TEST_CONFIG" "$TMP/bin/xray" 127.0.0.1:10085 xray.service "$TMP/backups"
cmp "$TMP/updated.json" "$TEST_CONFIG"
[[ ! -f "$TEST_STATE/restarted" ]] || { echo 'FAIL: repeat setup restarted service' >&2; exit 1; }

# Existing API on a different port must never be overwritten.
jq '.api.listen="127.0.0.1:12345"' "$TMP/updated.json" > "$TEST_CONFIG"
cp "$TEST_CONFIG" "$TMP/before-existing.json"
if bash "$HELPER" "$TEST_CONFIG" "$TMP/bin/xray" 127.0.0.1:10085 xray.service "$TMP/backups"; then
  echo 'FAIL: overwrote an existing API configuration' >&2; exit 1
fi
cmp "$TMP/before-existing.json" "$TEST_CONFIG"

# Missing API + failed post-restart probe must restore the exact original config.
cp "$TMP/original.json" "$TEST_CONFIG"
export TEST_API_FAIL=1
rm -f "$TEST_STATE/restarted"
if bash "$HELPER" "$TEST_CONFIG" "$TMP/bin/xray" 127.0.0.1:10085 xray.service "$TMP/backups"; then
  echo 'FAIL: failed to roll back on API failure' >&2; exit 1
fi
cmp "$TMP/original.json" "$TEST_CONFIG"
[[ -f "$TEST_STATE/restarted" ]]
echo 'PASS: setup-stats success, idempotence, existing API refusal, and rollback'
