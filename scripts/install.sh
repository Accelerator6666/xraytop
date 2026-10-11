#!/usr/bin/env bash
# XrayTop Linux installer/upgrader. Never restarts Xray or NGINX.
set -Eeuo pipefail
umask 077
REPO="Accelerator6666/xraytop"
VERSION="${XRAYTOP_VERSION:-v0.12.1-dev}"
BIN="/usr/local/bin/xraytop"
UNIT="/etc/systemd/system/xraytop-collector.service"
SERVICE="xraytop-collector.service"
STATE="/var/lib/xraytop"
CONFIG="${XRAYTOP_CONFIG:-/usr/local/etc/xray/config.json}"
XRAY_BIN="${XRAYTOP_XRAY_BIN:-$(command -v xray || true)}"
API="${XRAYTOP_API:-127.0.0.1:10085}"
NGINX_CONFIG="${XRAYTOP_NGINX_CONFIG:-/etc/nginx/nginx.conf}"
ACCESS_LOG="${XRAYTOP_ACCESS_LOG:-/var/log/nginx/access.log}"
XRAY_UNIT="${XRAYTOP_XRAY_UNIT:-xray.service}"
SETUP_STATS="${XRAYTOP_SETUP_STATS:-0}"

help() {
cat <<'HELP'
XrayTop installer/upgrader (root, Linux/systemd)
  bash install.sh              Install or upgrade
  bash install.sh --help       Show help without changing the system
Environment:
  XRAYTOP_VERSION        Release tag (default: v0.12.1-dev)
  XRAYTOP_CONFIG         Xray JSON/JSONC config (default /usr/local/etc/xray/config.json)
  XRAYTOP_XRAY_BIN       Local Xray executable (default command -v xray)
  XRAYTOP_API            Xray StatsService (default 127.0.0.1:10085)
  XRAYTOP_XRAY_UNIT      Xray systemd unit (default xray.service)
  XRAYTOP_NGINX_CONFIG   NGINX primary config (default /etc/nginx/nginx.conf)
  XRAYTOP_ACCESS_LOG     NGINX combined log (default /var/log/nginx/access.log)
  XRAYTOP_SETUP_STATS    0: diagnose only (default); 1: opt in to backup/modify/restart Xray
Default mode never changes Xray/NGINX. XRAYTOP_SETUP_STATS=1 may restart Xray.
HELP
}
if [[ "${1:-}" == "--help" || "${1:-}" == "-h" ]]; then help; exit 0; fi
if (( $# > 0 )); then help >&2; exit 2; fi
(( $(id -u) == 0 )) || { echo "ERROR: run as root" >&2; exit 1; }
[[ $(uname -s) == Linux ]] && command -v systemctl >/dev/null 2>&1 || {
  echo "ERROR: Linux with systemd required" >&2; exit 1;
}
for cmd in curl sha256sum awk stat install mktemp cp mv date; do
  command -v "$cmd" >/dev/null 2>&1 || { echo "ERROR: missing $cmd" >&2; exit 1; }
done
case "$(uname -m)" in
  x86_64|amd64) ARCH=amd64 ;;
  aarch64|arm64) ARCH=arm64 ;;
  *) echo "ERROR: unsupported CPU architecture" >&2; exit 1 ;;
esac
[[ "$VERSION" =~ ^v[0-9]+[.][0-9]+[.][0-9]+(-[a-zA-Z0-9.]+)?$ ]] || {
  echo "ERROR: invalid version" >&2; exit 2;
}
for p in "$CONFIG" "$XRAY_BIN" "$NGINX_CONFIG" "$ACCESS_LOG"; do
  [[ "$p" =~ ^/[a-zA-Z0-9_./+-]+$ ]] || {
    echo "ERROR: invalid absolute path (spaces unsupported): $p" >&2; exit 2;
  }
done
[[ "$SETUP_STATS" == 0 || "$SETUP_STATS" == 1 ]] || { echo "ERROR: XRAYTOP_SETUP_STATS must be 0 or 1" >&2; exit 2; }
[[ "$API" =~ ^[a-zA-Z0-9.:_-]+$ ]] || { echo "ERROR: invalid API address" >&2; exit 2; }
[[ "$XRAY_UNIT" =~ ^[a-zA-Z0-9@._-]+[.]service$ ]] || { echo "ERROR: invalid Xray unit" >&2; exit 2; }
[[ -x "$XRAY_BIN" && -r "$CONFIG" && -r "$NGINX_CONFIG" ]] || {
  echo "ERROR: Xray binary, Xray config, or NGINX config is unavailable" >&2; exit 1;
}
TMP="$(mktemp -d)"
BACKUP=""
CHANGED=0
SUCCESS=0
HAD_BIN=0
HAD_UNIT=0
WAS_ACTIVE=0
WAS_ENABLED=0
cleanup() {
  rc=$?
  trap - EXIT
  if (( CHANGED && !SUCCESS )); then
    echo "ERROR: install failed; attempting to restore previous collector" >&2
    set +e
    systemctl stop "$SERVICE" >/dev/null 2>&1
    if (( HAD_BIN )); then cp -a "$BACKUP/xraytop" "$BIN"; else rm -f "$BIN"; fi
    if (( HAD_UNIT )); then cp -a "$BACKUP/collector.service" "$UNIT"; else rm -f "$UNIT"; fi
    systemctl daemon-reload
    if (( !WAS_ENABLED )); then systemctl disable "$SERVICE" >/dev/null 2>&1; fi
    if (( WAS_ACTIVE )); then systemctl start "$SERVICE"; fi
  fi
  rm -rf "$TMP"
  exit "$rc"
}
trap cleanup EXIT
BASE="https://github.com/$REPO/releases/download/$VERSION"
fetch() {
  curl -fL --retry 3 --connect-timeout 15 --max-time 180 \
    -o "$TMP/$2" "$BASE/$1"
}
echo "==> Fetching XrayTop $VERSION linux/$ARCH"
fetch "xraytop-linux-$ARCH" xraytop
fetch "xraytop-collector.service" collector.service
fetch "SHA256SUMS" SHA256SUMS
for pair in "xraytop-linux-$ARCH:xraytop" "xraytop-collector.service:collector.service"; do
  name="${pair%%:*}"
  dest="${pair#*:}"
  want="$(awk -v name="$name" '$2 == name {print $1}' "$TMP/SHA256SUMS")"
  [[ "$want" =~ ^[0-9a-fA-F]{64}$ ]] || { echo "ERROR: no checksum for $name" >&2; exit 1; }
  actual="$(sha256sum "$TMP/$dest")"
  actual="${actual%% *}"
  [[ "${actual,,}" == "${want,,}" ]] || { echo "ERROR: SHA256 mismatch for $name" >&2; exit 1; }
done
chmod 0755 "$TMP/xraytop"
[[ "$("$TMP/xraytop" --version)" == "xraytop $VERSION" ]] || {
  echo "ERROR: downloaded executable version mismatch" >&2; exit 1;
}
grep -q '^\[Service\]$' "$TMP/collector.service" || { echo "ERROR: invalid unit" >&2; exit 1; }

# Select group-readable access.log without DAC override or permission changes.
LOG_GROUP=""
if [[ -e "$ACCESS_LOG" ]]; then
  LOG_GROUP="$(stat -c %G "$ACCESS_LOG")"
  if ! [[ "$LOG_GROUP" =~ ^[a-zA-Z_][a-zA-Z0-9_-]*$ ]] ||
     ! command -v getent >/dev/null 2>&1 ||
     ! getent group "$LOG_GROUP" >/dev/null 2>&1; then LOG_GROUP=""; fi
fi
if [[ -z "$LOG_GROUP" ]] && command -v getent >/dev/null 2>&1 &&
   getent group adm >/dev/null 2>&1; then LOG_GROUP=adm; fi

# Build a local unit from the release-matched, checksum-verified template.
while IFS= read -r line || [[ -n "$line" ]]; do
  case "$line" in
    Wants=xray.service) printf 'Wants=%s\n' "$XRAY_UNIT" ;;
    After=xray.service) printf 'After=%s\n' "$XRAY_UNIT" ;;
    '# SupplementaryGroups=adm')
      if [[ -n "$LOG_GROUP" ]]; then printf 'SupplementaryGroups=%s\n' "$LOG_GROUP"; fi ;;
    ExecStart=*)
      printf 'ExecStart=%s --daemon --interval 2s --config %s --api %s --xray-bin %s --nginx-config-file %s --nginx-access-log %s --snapshot-file %s/snapshot.json --history-file %s/ip-history.json\n' \
        "$BIN" "$CONFIG" "$API" "$XRAY_BIN" "$NGINX_CONFIG" "$ACCESS_LOG" "$STATE" "$STATE" ;;
    *) printf '%s\n' "$line" ;;
  esac
done < "$TMP/collector.service" > "$TMP/unit.final"

install -d -m 0700 "$STATE"
BACKUP="$STATE/backups/$(date -u +%Y%m%dT%H%M%SZ)-$$"
install -d -m 0700 "$BACKUP"
if [[ -f "$BIN" ]]; then cp -a "$BIN" "$BACKUP/xraytop"; HAD_BIN=1; fi
if [[ -f "$UNIT" ]]; then cp -a "$UNIT" "$BACKUP/collector.service"; HAD_UNIT=1; fi
if systemctl is-active --quiet "$SERVICE"; then WAS_ACTIVE=1; fi
if [[ -f "$UNIT" ]] && systemctl is-enabled --quiet "$SERVICE" 2>/dev/null; then WAS_ENABLED=1; fi

CHANGED=1
if (( WAS_ACTIVE )); then systemctl stop "$SERVICE"; fi
LEGACY="/root/.local/state/xraytop/ip-history.json"
if [[ ! -e "$STATE/ip-history.json" && -f "$LEGACY" ]]; then
  install -m 0600 "$LEGACY" "$STATE/ip-history.json"
  echo "==> Imported existing IP history without overwriting current history"
fi
install -m 0755 "$TMP/xraytop" "$BIN.new.$$"
mv -f "$BIN.new.$$" "$BIN"
install -m 0644 "$TMP/unit.final" "$UNIT.new.$$"
mv -f "$UNIT.new.$$" "$UNIT"
systemctl daemon-reload
systemctl enable "$SERVICE" >/dev/null
systemctl restart "$SERVICE"
sleep 5
systemctl is-active --quiet "$SERVICE" || { echo "ERROR: collector not active" >&2; exit 1; }
[[ -s "$STATE/snapshot.json" ]] || { echo "ERROR: snapshot absent" >&2; exit 1; }
age=$(( $(date +%s) - $(stat -c %Y "$STATE/snapshot.json") ))
(( age >= 0 && age <= 20 )) || { echo "ERROR: stale snapshot" >&2; exit 1; }
SUCCESS=1
echo "==> Installed $("$BIN" --version); systemd collector running"
echo "==> Backup: $BACKUP"
echo "==> Data directory: $STATE"
echo "==> Use: xraytop  (6=IP, 8=HTTP, 9=history)"
echo "==> Check: systemctl status xraytop-collector --no-pager"
if command -v jq >/dev/null 2>&1; then
  jq '{
    nginx_ports: ((.snapshot.Nginx.Ports // []) | length),
    nginx_routes: ((.snapshot.Nginx.Routes // []) | length),
    nginx_error: .snapshot.NginxError,
    tcp_connections: (.snapshot.Live.Connections // 0),
    nginx_http_matches: (.snapshot.Access.Matched // 0),
    access_error: .snapshot.AccessError,
    xray_accepted: (.snapshot.Recent.Events // 0)
  }' "$STATE/snapshot.json" || true
fi
echo "==> Checking local Xray StatsService at $API"
if "$XRAY_BIN" api statsquery -s "$API" >/dev/null 2>&1; then
  echo "[PASS] StatsService online"
elif [[ "$SETUP_STATS" == 1 ]]; then
  echo "[INFO] Explicit StatsService setup requested. Xray may briefly restart."
  # Fetch from the SAME pinned release; SHA256SUMS must cover the helper.
  fetch "setup-stats.sh" "setup-stats.sh"
  want="$(awk '$2 == "setup-stats.sh" {print $1}' "$TMP/SHA256SUMS")"
  actual="$(sha256sum "$TMP/setup-stats.sh")"
  actual="${actual%% *}"
  [[ "$want" =~ ^[0-9a-fA-F]{64}$ && "${actual,,}" == "${want,,}" ]] || {
    echo "ERROR: setup-stats.sh SHA256 verification failed" >&2; exit 1;
  }
  bash "$TMP/setup-stats.sh" "$CONFIG" "$XRAY_BIN" "$API" "$XRAY_UNIT" "$BACKUP"
  systemctl restart "$SERVICE"
else
  echo "[WARN] Xray StatsService unavailable: $API"
  echo "       To authorize backup/configuration and Xray restart, run installer with XRAYTOP_SETUP_STATS=1"
fi
if [[ -e /etc/systemd/system/xraytop-collector.service.d/10-cert-access.conf ||
      -e /etc/systemd/system/xraytop-collector.service.d/20-nginx-log.conf ]]; then
  echo "NOTICE: legacy 10-cert-access.conf / 20-nginx-log.conf overrides still exist."
  echo "        Review/remove them manually after verifying the updated collector."
fi
