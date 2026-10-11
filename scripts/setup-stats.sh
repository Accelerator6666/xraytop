#!/usr/bin/env bash
# Opt-in Xray StatsService setup: backup, validate, restart, verify, rollback.
set -Eeuo pipefail
umask 077

if (( $# != 5 )); then
  echo 'Usage: setup-stats.sh CONFIG XRAY_BIN 127.0.0.1:PORT XRAY_UNIT BACKUP_DIR' >&2
  exit 2
fi
CFG="$1"
XRAY_BIN="$2"
API="$3"
XRAY_UNIT="$4"
BACKUP_DIR="$5"

(( $(id -u) == 0 )) || { echo 'ERROR: root required' >&2; exit 1; }
for cmd in jq mktemp cp mv systemctl; do
  command -v "$cmd" >/dev/null 2>&1 || { echo "ERROR: missing $cmd" >&2; exit 1; }
done
[[ -f "$CFG" && ! -L "$CFG" && -x "$XRAY_BIN" ]] || {
  echo 'ERROR: Xray config/binary missing or config is a symlink' >&2; exit 1;
}
[[ "$XRAY_UNIT" =~ ^[a-zA-Z0-9@._-]+[.]service$ ]] || {
  echo 'ERROR: invalid Xray systemd unit' >&2; exit 1;
}
[[ "$API" =~ ^127[.]0[.]0[.]1:([0-9]{1,5})$ ]] || {
  echo 'ERROR: automatic setup only supports 127.0.0.1:PORT' >&2; exit 1;
}
PORT="${BASH_REMATCH[1]}"
(( 10#$PORT >= 1 && 10#$PORT <= 65535 )) || { echo 'ERROR: invalid API port' >&2; exit 1; }

# An existing reachable API needs no configuration changes.
if "$XRAY_BIN" api statsquery -s "$API" >/dev/null 2>&1; then
  echo "[PASS] StatsService already available at $API; Xray unchanged"
  exit 0
fi
systemctl is-active --quiet "$XRAY_UNIT" || {
  echo 'ERROR: Xray service is not active; refusing to modify config' >&2; exit 1;
}
# Do not guess which configuration a running process consumes.
MAINPID="$(systemctl show "$XRAY_UNIT" --property=MainPID --value)"
[[ "$MAINPID" =~ ^[1-9][0-9]*$ && -r "/proc/$MAINPID/cmdline" ]] || {
  echo 'ERROR: cannot establish running Xray process configuration' >&2; exit 1;
}
if ! tr '\0' '\n' < "/proc/$MAINPID/cmdline" | grep -Fxq -- "$CFG"; then
  echo 'ERROR: running Xray does not reference this config; manual setup required' >&2
  exit 1
fi
# Existing API definitions and tag collisions require manual review, not overwriting.
jq -e '
  (.api == null)
  and ((.inbounds | type) == "array")
  and (.inbounds | length > 0)
  and ((.outbounds | type) == "array")
  and (([.inbounds[]?.tag, .outbounds[]?.tag] | index("xraytop-api")) == null)
  and (([.inbounds[]?.tag] | map(select(. != null and . != "")) | unique | length)
       == ([.inbounds[]?.tag] | map(select(. != null and . != "")) | length))
  and ([.inbounds[] | select((.tag // "") != "") | .tag] as $tags
       | ([.inbounds | to_entries[] | select((.value.tag // "") == "") | "xraytop-inbound-\(.key)"]
          | all(. as $candidate | ($tags | index($candidate)) == null)))
' "$CFG" >/dev/null || {
  echo 'ERROR: unsupported JSON, existing API, or inbound tag collision; manual setup required' >&2
  exit 1
}
if command -v ss >/dev/null 2>&1 &&
   ss -lntH | awk -v port="$PORT" '$4 ~ (":" port "$") {seen=1} END {exit !seen}'; then
  echo "ERROR: local port $PORT already listening; manual investigation required" >&2
  exit 1
fi

mkdir -p -m 0700 "$BACKUP_DIR"
BACKUP="$BACKUP_DIR/xray-config-$(date -u +%Y%m%dT%H%M%SZ)-$$.json"
TMP_CFG="$(mktemp "$(dirname "$CFG")/.xraytop.XXXXXX.json")"
CHANGED=0
cleanup() {
  rc=$?
  trap - EXIT
  rm -f "$TMP_CFG"
  if (( rc != 0 && CHANGED == 1 )); then
    echo '[WARN] StatsService setup failed; attempting Xray configuration rollback' >&2
    if cp -p "$BACKUP" "$CFG" && systemctl restart "$XRAY_UNIT" &&
       systemctl is-active --quiet "$XRAY_UNIT"; then
      echo '[WARN] Original Xray config restored; verify proxy connections' >&2
    else
      echo "[ERROR] Rollback needs attention; backup: $BACKUP" >&2
    fi
  fi
  exit "$rc"
}
trap cleanup EXIT
cp -p "$CFG" "$BACKUP"

jq --arg listen "$API" '
  .api = {tag: "xraytop-api", listen: $listen, services: ["StatsService"]}
  | .stats = (.stats // {})
  | .policy = (.policy // {})
  | .policy.system = ((.policy.system // {}) + {
      statsInboundUplink: true, statsInboundDownlink: true,
      statsOutboundUplink: true, statsOutboundDownlink: true
    })
  | .inbounds |= (to_entries | map(
      if (.value.tag // "") == "" then
        .value.tag = ("xraytop-inbound-" + (.key | tostring))
      else . end
    ) | map(.value))
' "$CFG" > "$TMP_CFG"
# Both extension and explicit format are intentional for Xray 26.x.
"$XRAY_BIN" run -test -format json -config "$TMP_CFG"
chown --reference="$CFG" "$TMP_CFG"
chmod --reference="$CFG" "$TMP_CFG"
mv -f "$TMP_CFG" "$CFG"
CHANGED=1

echo "[INFO] Restarting $XRAY_UNIT to enable local StatsService"
systemctl restart "$XRAY_UNIT"
OK=0
for (( i=0; i<15; i++ )); do
  if systemctl is-active --quiet "$XRAY_UNIT" &&
     "$XRAY_BIN" api statsquery -s "$API" >/dev/null 2>&1; then
    OK=1
    break
  fi
  sleep 1
done
(( OK == 1 )) || { echo '[ERROR] StatsService unavailable after restart' >&2; exit 1; }
CHANGED=0
echo "[PASS] Xray StatsService enabled at $API"
echo "[PASS] Original config backup: $BACKUP"
