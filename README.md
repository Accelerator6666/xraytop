# XrayTop

[简体中文](README.zh-CN.md)

**XrayTop** is a Linux-only, read-only terminal monitor for a **local Xray-core process**, with btop-inspired terminal views and protocol-aware inbound/outbound statistics. It supports mixed VLESS, VMess, Trojan, etc. configurations over WebSocket, gRPC, XHTTP, RAW/TCP, HTTPUpgrade, mKCP and more, including Unix-domain socket listeners behind nginx.

> **v0.9.0-dev preview:** multi-protocol config discovery, process CPU/RSS, tagged inbound/outbound traffic, protocol and transport traffic aggregation, and terminal charts are implemented. Native gRPC client, nginx access-log correlation and per-user sessions are not implemented yet.

## How monitoring works

XrayTop reads **proxy protocol**, **transport method**, **security layer**, **listener** and **tag** from local Xray JSON/JSONC configuration. It supports both current `streamSettings.method` and legacy `streamSettings.network` and reads multiple `*.json` files from Xray `-confdir`. A tag maps onto the corresponding StatsService counter without guessing any transport from the tag text. Counters returned without matching configuration are labeled `Unknown`. Configured endpoints with no counter are shown as `-`, not as zero traffic.

The StatsService API happens to use gRPC for **management**, independent of how clients connect to the Xray server: WebSocket, XHTTP, RAW, gRPC or any other transport can be monitored with this same API. Protocol and transport groupings are two *alternative* summaries of **inbound** counters. Do not add them or inbound and outbound totals together.

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
