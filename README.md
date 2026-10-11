# XrayTop

[简体中文](README.zh-CN.md)

**XrayTop** is a Linux-only, read-only terminal monitor for a **local Xray-core process**, with btop-inspired terminal views and protocol-aware inbound/outbound statistics. It supports mixed VLESS, VMess, Trojan, etc. configurations over WebSocket, gRPC, XHTTP, RAW/TCP, HTTPUpgrade, mKCP and more, including Unix-domain socket listeners behind nginx.

> **v0.11.3-dev preview:** multi-protocol config discovery, process CPU/RSS, tagged inbound/outbound traffic, protocol and transport traffic aggregation, and terminal charts are implemented. Native gRPC API access and authenticated per-user session tracking are not implemented; optional NGINX access-log correlation and IP-first evidence tiers are available, but are not proof of authenticated sessions.

## How monitoring works

XrayTop reads **proxy protocol**, **transport method**, **security layer**, **listener** and **tag** from local Xray JSON/JSONC configuration. It supports both current `streamSettings.method` and legacy `streamSettings.network` and reads multiple `*.json` files from Xray `-confdir`. A tag maps onto the corresponding StatsService counter without guessing any transport from the tag text. Counters returned without matching configuration are labeled `Unknown`. Configured endpoints with no counter are shown as `-`, not as zero traffic.

The StatsService API happens to use gRPC for **management**, independent of how clients connect to the Xray server: WebSocket, XHTTP, RAW, gRPC or any other transport can be monitored with this same API. Protocol and transport groupings are two *alternative* summaries of **inbound** counters. Do not add them or inbound and outbound totals together.


## One-command install and upgrade (recommended)

**Requirements:** Linux with systemd (Debian/Ubuntu etc.), AMD64/ARM64, root/sudo, curl, sha256sum, and running Xray/NGINX deployments. By default the installer **does not modify or restart Xray or NGINX**. Opt-in StatsService setup is available with `XRAYTOP_SETUP_STATS=1` and may briefly restart Xray.

Download the pinned-release installer, inspect it, and run:

```bash
curl -fL --retry 3 -o install-xraytop.sh \
  https://github.com/Accelerator6666/xraytop/releases/download/v0.11.3-dev/install.sh
less install-xraytop.sh
sudo bash install-xraytop.sh
```

After auditing the script, an unattended one-liner is also possible (directly executing remote scripts is less secure):

```bash
curl -fsSL https://github.com/Accelerator6666/xraytop/releases/download/v0.11.3-dev/install.sh | sudo bash
```

The installer checks the platform and Xray/NGINX paths, downloads the executable, systemd unit and SHA256SUMS from the **same release**, verifies the binary/unit SHA256 and executable version, backs up the previous installation under `/var/lib/xraytop/backups/`, preserves/imports IP history, installs the hardened `xraytop-collector` systemd service with NGINX access-log group read permission, enables it, verifies a fresh snapshot, and attempts rollback on failure. Only the XrayTop collector is stopped/restarted.

**Optional StatsService configuration:** The installer checks whether the API responds. It warns without changing Xray in default mode. To authorize modification, backup and restart of the Xray service, download and review the installer then run:

```bash
sudo env XRAYTOP_SETUP_STATS=1 bash install-xraytop.sh
```

This operation requires `jq`, a single active JSON Xray configuration, and `127.0.0.1:PORT` StatsService. It refuses existing API definitions and ambiguous configurations. The release-pinned helper is SHA256-verified. A `.json` temporary configuration is validated with `xray run -test -format json` before Xray is restarted; failures trigger rollback attempts. A restart may interrupt proxy clients. Test recovery and keep a separate SSH management connection.

**Verify:**

```bash
xraytop --version
systemctl is-active xraytop-collector
systemctl status xraytop-collector --no-pager
journalctl -u xraytop-collector -n 30 --no-pager
xraytop --data-source daemon --once --once-view 9
xraytop       # interactive: 6=source IP, 8=HTTP, 9=history
```

Re-run the script for subsequent upgrades (default pinned `v0.11.3-dev`). Customize via `sudo env XRAYTOP_VERSION=... XRAYTOP_CONFIG=/path/to/config.json bash install-xraytop.sh`. Run `bash install-xraytop.sh --help` for `XRAYTOP_XRAY_BIN`, `XRAYTOP_API`, `XRAYTOP_XRAY_UNIT`, `XRAYTOP_NGINX_CONFIG` and `XRAYTOP_ACCESS_LOG`. Custom file paths must be absolute with no spaces.

Historical data is in `/var/lib/xraytop/ip-history.json`: **back it up before uninstalling**. Legacy `10-cert-access.conf` / `20-nginx-log.conf` security overrides are not silently removed; review them after successful installation. Stop collection with `sudo systemctl disable --now xraytop-collector`. Closing the interactive TUI does not stop the systemd collector.

## Build

Go 1.23+ is required:

```bash
git clone https://github.com/Accelerator6666/xraytop.git
cd xraytop
go build -o xraytop ./cmd/xraytop
./xraytop
```

By default the program detects `-c`, `-config` and `-confdir` from the first local Xray process `/proc/PID/cmdline`, including relative paths against its working directory. Explicit overrides are available for custom setups and missing permissions:

```bash
./xraytop --config /usr/local/etc/xray/config.json
./xraytop --config-dir /usr/local/etc/xray/conf.d
./xraytop --config /etc/xray/01.json --config /etc/xray/02.json
./xraytop --config /etc/xray/config.json --api ''  # metadata/process only
./xraytop --once --config /etc/xray/config.json --api ''  # no TTY needed
./xraytop --interval 1s --api 127.0.0.1:10085
./xraytop --xray-bin /usr/local/bin/xray
```

**Keys:** `1` Overview, `2` Inbounds, `3` Outbounds, `4` Proxy protocols, `5` Transports; `Tab` cycles, `r` refreshes, `q` quits.

## Xray StatsService prerequisite

Enable `StatsService` in Xray's existing API configuration, with an API inbound routed to the `api` outbound on **loopback only**. Enable `stats: {}` and inbound/outbound counters via the `policy.system.statsInboundUplink`, `statsInboundDownlink`, `statsOutboundUplink`, `statsOutboundDownlink` flags. **Merge** with the existing Xray configuration; do not overwrite VLESS, VMess, Trojan inbounds or nginx settings. To verify:

```bash
xray api statsquery -s 127.0.0.1:10085
./xraytop --api 127.0.0.1:10085
```

XrayTop currently invokes the locally installed Xray `api statsquery` CLI for the StatsService call. A future native gRPC client will eliminate the periodic CLI subprocess while retaining the same multi-protocol support.

## Boundaries and safety

- **Local-only**, **read-only**; no port scans, remote host monitoring, config writes or daemon.
- Only tag/protocol/transport/security/listen information is loaded from the local config: UUIDs, user credentials and keys are never displayed. Non-JSON formats aren't parsed.
- Transport and listener fields are **configuration metadata**, not proof of an active connection or a healthy listener. A running process can be using older configuration data.
- nginx → Unix socket → Xray means visible socket peers are not necessarily actual clients; session/IP monitoring is not yet supported.
- Missing counters display as `-`; the initial successful sample cannot yield a transfer rate. On API errors cached totals remain but live rates become unavailable.
- When several Xray processes are running, autodiscovery selects the lowest PID; explicit `--config` is preferred for non-standard deployments.
- StatsService API addresses are restricted to local loopback, and counters are always queried without reset.

## Development

```bash
go test ./...
go vet ./...
```

GitHub Actions runs tests and cross-compiles Linux amd64/arm64. See [architecture](docs/ARCHITECTURE.md). MIT License.

> Compatibility: Xray 26.3.27 may omit `value` for zero-byte counters; XrayTop v0.6.0-dev correctly interprets them as zero.


## IP-first client view (`6`) and IP + inbound detail (`7`)

Client names or email labels are **not required**. XrayTop v0.6.0-dev groups the
source IPs present in **accepted** local Xray journald records. `6` aggregates
by IP across all inbound tags and shows transport metadata; `7` breaks results
down by IP + inbound. The collector reads at most 500 lines from the preceding
five minutes, approximately every ten seconds. It does not query the
email-indexed Xray online map when in IP-first mode.

```bash
./xraytop --once --once-view 6 --config /usr/local/etc/xray/config.json
./xraytop --once --once-view 7 --config /usr/local/etc/xray/config.json
```

**Important:** An accepted journal event is not evidence of a currently open
session; its count is not the number of active connections. NAT can merge
multiple clients under one IP, and one client can change addresses. For nginx
reverse proxy deployments, inspect trusted client-IP forwarding: Xray logs may
record a proxy address or an untrusted claimed address. No accurate per-IP byte
counters can be calculated from standard tagged StatsService totals.

## Optional live TCP observation

Pass `--ingress-ports 443,8964` using **your actual** local NGINX/Xray ingress ports to observe Linux ESTABLISHED TCP peers on those ports. View `6` merges those peers by IP with accepted Xray journal events from the past 5 minutes; view `7` adds an IP-level TCP count. Without the flag there is no live socket enumeration.

**Important:** An ESTABLISHED TCP socket on an ingress port is not proof of an authenticated Xray user/session or an active proxy request. Shared HTTPS, CDN, NAT, multiplexing, and reverse proxies may produce different peer IPs than the Xray access logs. Do not treat the counts as authenticated sessions or per-user bandwidth.


## Automatic NGINX ingress discovery (v0.7)

By default XrayTop calls local `nginx -T` and matches configured
`grpc_pass`/`proxy_pass` targets to exact Xray inbound Unix socket or loopback
listen endpoints. It follows named NGINX upstreams and includes **only matching**
listener ports. Results are cached for 60 seconds; no NGINX reload occurs.
NGINX HTTP(S) ports may carry unrelated site traffic, so TCP counts remain
**network-level only**, never authenticated Xray sessions.

```bash
./xraytop --config /usr/local/etc/xray/config.json
./xraytop --once --once-view 6 --config /usr/local/etc/xray/config.json
# Override automatic selection:
./xraytop --ingress-ports 1443,2443
# Disable automatic TCP discovery:
./xraytop --nginx-auto=false
```

When the active configuration cannot be read or proxy backends do not match
configured Xray inbound listeners, the program shows a warning and does not
guess port mappings.


## Read-only NGINX access-log correlation (v0.8)

View `8` reads at most the last 2 MiB of the local NGINX **combined** access log every ~10 seconds; HTTP requests are attributed to an Xray inbound only when a matching NGINX location has an unambiguous backend. It discards raw paths/query strings, referers, user-agents, and credentials. `--nginx-access-log ''` disables this collector. View `6` adds the matched HTTP request count per source IP, separately from Xray accepted events and current TCP sockets. HTTP 101/200 does not prove Xray authentication; common NGINX combined logs don't identify the local listener port, so ambiguous virtual-host routes are dropped. Log volume may limit 5-minute coverage.

## Source-IP evidence tiers and filtering (v0.9)

View `6` includes **NGINX-only IPs** even when neither Xray journald nor TCP snapshots contain them. It ranks sources by evidence tier: `XRAY` accepted journal activity, `HTTP` matched NGINX requests only, `TCP` ingress sockets only. These tiers do not represent logged-in users or an authenticated live-session count. Ports shown are TCP listener ports for that source IP, not verified Xray inbound associations. Press `f` to cycle `all → xray → http → tcp`, or pass `--clients-filter xray` (also usable with `--once --once-view 6`). The HTTP-only tier does not populate Xray-authenticated inbound metadata.


## Local source-IP history (v0.10; key 9)

The monitor samples per-IP TCP ESTABLISHED sockets and the last-5-minute Xray accepted /
NGINX matched HTTP counters **once per minute**. A small atomic JSON file at
`~/.local/state/xraytop/ip-history.json` (or `$XDG_STATE_HOME/xraytop/`) stores
the last **24 hours**, at most **128 IPs**, max **16 MiB**, mode **0600**.
Override with `--history-file PATH`; disable with `--history-file ''`.
No UUIDs, visited sites, full log lines, headers or URL queries are persisted.

Press `9` for history; `j/k` or arrow keys select IP, `Enter` toggles details,
`t` cycles 5/15/30/60-minute trends and `s` cycles sorting
(recent, peak TCP, latest TCP, recent Xray accepted count).
`--once --once-view 9` prints the history overview.

A blank/middle dot means no sample was taken; a zero bar means a sampled zero.
Xray and NGINX counts are rolling 5-minute values at each snapshot, not
per-minute new events. Do not sum them or infer authenticated session lifetime
or a unique person from a public IP. History only accumulates while XrayTop runs.


## v0.11: headless systemd collector and read-only viewer

`--daemon` continuously samples into local atomic 0600 snapshots, protected
by an exclusive file lock; `--data-source auto` (default) follows fresh
daemon snapshots (15s maximum age), without redundant Xray/NGINX polling.
`--data-source daemon` is strictly read-only, `local` forces foreground.
Deploy `packaging/systemd/xraytop-collector.service` with a private systemd
StateDirectory in `/var/lib/xraytop`, storing separate snapshot and history.
This version fixes the history `MM-DD` date-format issue and clips trends to
terminal width. No extra listener or exposed API is added.


## v0.11.1: sandbox-safe, read-only NGINX discovery

Automatic ingress discovery no longer runs `nginx -T`, which may try to
open TLS certificates, NGINX log files and the active pid file during validation.
Instead, XrayTop reads `/etc/nginx/nginx.conf` and recursively expands
literal `include` files (bounded to 256 files and 4 MiB), parsing the tree
read-only and matching exact Xray backends. Override with
`--nginx-config-file PATH`. No NGINX/Xray reload or config modifications.
On-disk files might differ from *currently loaded* NGINX config if there
are unapplied edits, so these are metadata observations, not runtime proof.
An unreadable include returns an explicit error rather than partial mappings.
If NGINX access logs are group-readable by `adm`, systemd can grant the
collector `SupplementaryGroups=adm` instead of broad DAC override capabilities.
