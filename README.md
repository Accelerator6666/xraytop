# XrayTop

[简体中文](README.zh-CN.md)

**XrayTop** is a Linux-only, read-only terminal monitor for a **local Xray-core process**, with btop-inspired terminal views and protocol-aware inbound/outbound statistics. It supports mixed VLESS, VMess, Trojan, etc. configurations over WebSocket, gRPC, XHTTP, RAW/TCP, HTTPUpgrade, mKCP and more, including Unix-domain socket listeners behind nginx.

> **v0.12.3-dev preview:** multi-protocol config discovery, process CPU/RSS, tagged inbound/outbound traffic, protocol and transport traffic aggregation, and terminal charts are implemented. Native gRPC API access and authenticated per-user session tracking are not implemented; optional NGINX access-log correlation and IP-first evidence tiers are available, but are not proof of authenticated sessions.

## How monitoring works

XrayTop reads **proxy protocol**, **transport method**, **security layer**, **listener** and **tag** from local Xray JSON/JSONC configuration. It supports both current `streamSettings.method` and legacy `streamSettings.network` and reads multiple `*.json` files from Xray `-confdir`. A tag maps onto the corresponding StatsService counter without guessing any transport from the tag text. Counters returned without matching configuration are labeled `Unknown`. Configured endpoints with no counter are shown as `-`, not as zero traffic.

The StatsService API happens to use gRPC for **management**, independent of how clients connect to the Xray server: WebSocket, XHTTP, RAW, gRPC or any other transport can be monitored with this same API. Protocol and transport groupings are two *alternative* summaries of **inbound** counters. Do not add them or inbound and outbound totals together.


## Installation, upgrades, and StatsService setup (v0.12.3-dev)

### 1. Prerequisites

Use a Linux host with systemd (e.g. Debian/Ubuntu), AMD64 or ARM64, with Xray and NGINX already installed and running. You need root/sudo, `curl`, and `sha256sum`. The defaults are `/usr/local/etc/xray/config.json`, `/usr/local/bin/xray`, and `/etc/nginx/nginx.conf`; override paths with environment variables if necessary.

**Default installation does not modify or restart Xray or NGINX.** The installer tests whether StatsService responds, but a missing API generates only a warning. Setting `XRAYTOP_SETUP_STATS=1` explicitly authorizes Xray config changes and a restart, which may interrupt existing proxy connections. Use a separate, reliable SSH management connection.

### 2. First-time installation (recommended)

Download the pinned [v0.12.3-dev release](https://github.com/Accelerator6666/xraytop/releases/tag/v0.12.3-dev), review the script, and run:

```bash
curl -fL --retry 3 -o install-xraytop.sh \
  https://github.com/Accelerator6666/xraytop/releases/download/v0.12.3-dev/install.sh

less install-xraytop.sh
sudo bash install-xraytop.sh
```

After reviewing and trusting the script, an unattended one-liner is possible (less secure than reviewing a downloaded script first):

```bash
curl -fsSL https://github.com/Accelerator6666/xraytop/releases/download/v0.12.3-dev/install.sh | sudo bash
```

The installer selects AMD64/ARM64, fetches XrayTop, the systemd unit and SHA256SUMS from the **same release**, verifies binary/unit SHA256 and binary version, installs `/usr/local/bin/xraytop` and `xraytop-collector.service`, and checks for a fresh background snapshot. Existing binaries and unit files are backed up under `/var/lib/xraytop/backups/`; `/var/lib/xraytop/ip-history.json` is preserved. Normal install/upgrade only restarts the Collector.

### 3. Optional automatic repair of a missing StatsService

If the UI says `StatsService: UNAVAILABLE` or the installer prints `[WARN] Xray StatsService unavailable`, test the API:

```bash
sudo /usr/local/bin/xray api statsquery -s 127.0.0.1:10085
```

To **explicitly authorize** repair, use the reviewed `install-xraytop.sh` downloaded above:

```bash
sudo env XRAYTOP_SETUP_STATS=1 bash install-xraytop.sh
```

This mode requires `jq` (on Debian/Ubuntu: `sudo apt-get update && sudo apt-get install -y jq`). Automatic setup only supports **one plain JSON file actually referenced by the running Xray systemd process**, a loopback API at `127.0.0.1:PORT`, and an unambiguous inbound configuration. Existing APIs or conflicting tags are never overwritten. The release-pinned repair helper is SHA256-verified. It backs up the Xray JSON configuration, preserves existing proxy inbounds/outbounds/routes and NGINX/Unix sockets, enables StatsService plus inbound/outbound counters, and assigns `xraytop-inbound-N` to previously untagged inbounds. The `.json` temporary file is validated with `xray run -test -format json` before restarting Xray. It attempts rollback if the API does not become available; **rollback is not a guarantee of service recovery**.

Hosts where StatsService is already reachable need **no repair**. Normal installation does not change Xray, and even opt-in mode skips Xray changes when its API probe succeeds. For `-confdir`, JSONC, multiple instances, existing API setups, or nonstandard launch arguments, configure StatsService manually instead.

### 4. Verify after installation or upgrade

```bash
xraytop --version
sudo systemctl is-enabled xraytop-collector
sudo systemctl is-active xray xraytop-collector
sudo systemctl status xraytop-collector --no-pager
sudo journalctl -u xraytop-collector -n 30 --no-pager

# Verify the local Xray API (when enabled)
sudo /usr/local/bin/xray api statsquery -s 127.0.0.1:10085

# Read the collector history and open the TUI
sudo xraytop --data-source daemon --once --once-view 9
sudo xraytop
```

TUI keys: `1` overview; `6` source IPs; `8` NGINX; `9` history. `StatsService: ONLINE` plus accumulated inbound bytes demonstrates that API counters are readable. `0 B/s` is normal without new proxy traffic or before two samples. HTTP matches, TCP connections, and accepted journal events are **not authenticated online-user counts**.

Check detailed collector fields with:

```bash
sudo jq '{
  nginx_ports: ((.snapshot.Nginx.Ports // []) | length),
  nginx_routes: ((.snapshot.Nginx.Routes // []) | length),
  nginx_error: .snapshot.NginxError,
  tcp_connections: (.snapshot.Live.Connections // 0),
  tcp_error: .snapshot.LiveError,
  nginx_http_matches: (.snapshot.Access.Matched // 0),
  access_error: .snapshot.AccessError,
  xray_accepted: (.snapshot.Recent.Events // 0)
}' /var/lib/xraytop/snapshot.json
```

### 5. Upgrades, custom deployments, and troubleshooting

To upgrade from `v0.11.2-dev` or another older version, download the current release installer again and run it normally; uninstalling first or reconfiguring an existing StatsService is unnecessary. Custom installations can set:

```bash
sudo env XRAYTOP_VERSION=v0.12.3-dev \
  XRAYTOP_CONFIG=/usr/local/etc/xray/config.json \
  XRAYTOP_XRAY_BIN=/usr/local/bin/xray \
  XRAYTOP_API=127.0.0.1:10085 \
  XRAYTOP_XRAY_UNIT=xray.service \
  bash install-xraytop.sh
```

`XRAYTOP_NGINX_CONFIG` and `XRAYTOP_ACCESS_LOG` are also supported. Run `bash install-xraytop.sh --help` for all variables; custom paths must be absolute and contain no spaces.

- **StatsService unavailable:** confirm the API is enabled in Xray, the address matches `XRAYTOP_API`, and `xray api statsquery` succeeds. The default installer will not repair it.
- **Automatic repair refused:** check for an existing API, `-confdir` or JSONC, different Xray process arguments, or a tag collision. Do not override production configs blindly.
- **Collector failure:** inspect `sudo journalctl -u xraytop-collector -n 100 --no-pager`, snapshot freshness, and NGINX access-log permissions.
- **Recovery:** backed-up Xray configs are under `/var/lib/xraytop/backups/<installation>/xray-config-*.json`. Identify and validate the correct backup before any manual restoration/restart.

Back up `/var/lib/xraytop/ip-history.json` before uninstalling. Closing the TUI leaves the Collector running. To stop it:

```bash
sudo systemctl disable --now xraytop-collector
```

Legacy `10-cert-access.conf` / `20-nginx-log.conf` security overrides are not automatically removed; inspect them after verifying collector operation.

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

## v0.12: incremental readers, Doctor, source-IP search

**v0.12.3-dev** adds bounded **in-process incremental** ingestion:
NGINX access.log is followed by inode/offset and new appended lines are parsed
into only IP/inbound/status metadata (no raw URLs or headers retained). The
Xray journal reader follows JSON `__CURSOR` entries instead of rescanning the
last 500 lines on every poll. Cold starts bootstrap the last five minutes.
Rotation, truncation and overflow are flagged as potentially incomplete rather
than silently pretending the totals are exact. Cursors remain in memory only,
so restart gaps/replays remain possible; this is not audit-grade metering.

Read-only health diagnostics:

```bash
xraytop --doctor --config /usr/local/etc/xray/config.json \
  --xray-bin /usr/local/bin/xray --api 127.0.0.1:10085
```

Reports collector health, snapshot freshness, local StatsService, NGINX
routing, file permissions and source errors; FAIL returns nonzero, WARN indicates
degraded optional data. No configuration changes or service restarts occur.

Press `/` on page `6` (source IPs) or `9` (history) to search by source IP
substring; Enter applies, Esc cancels and Ctrl-U clears. Or run:

```bash
xraytop --data-source daemon --once --once-view 6 --ip-search 192.0.2
xraytop --data-source daemon --once --once-view 9 --ip-search 2001:db8
```



## v0.12.1: responsive 80-column SSH history display

The IP history table now uses full columns on terminals >=110 columns, a
compact single-line layout on 66–109 columns (retaining last seen, TCP, peak,
Xray, HTTP and trend), and a two-line layout below 66 columns. Enter shows
the complete IPv6 address and both timestamps. j/k scroll through all rows.

If the SSH client's PTY window size is stale (`stty size` shows
fewer rows/columns than physically visible), resize the terminal or adjust its
terminal profile. **Only when the terminal is actually large enough** you
can override the reported size:

```bash
xraytop --tui-cols 120 --tui-rows 40
```

The default is auto-detection (0); incorrect override sizes can cause wrapping.


## v0.12.2: terminal compatibility across SSH clients

XrayTop reads PTY geometry (TIOCGWINSZ) and sends a **nonblocking**
xterm-compatible `CSI 18t` query for text-area rows/columns. Supported
terminals may answer `ESC[8;rows;cols t` (without the displayed space);
a validated reply redraws the dashboard, and SIGWINCH triggers a fresh
measurement. Unsupported or silent terminals use the **PTY fallback**.
It is not technically possible to determine the physical size if a client
reports an incorrect PTY size **and** does not support the query.

Press **`z`** in the TUI for terminal settings. `w` cycles saved column
presets (auto / 80 / 100 / 120 / 140 / 160 / 180), `h` cycles row presets
(auto / 24 / 30 / 40 / 45 / 50 / 60), `p` toggles ANSI probing, `a`
toggles ASCII graphics, `r` resets to defaults, `s` saves and `z` / Esc
closes without saving. Preferences are stored per user in
`~/.config/xraytop/terminal.json` (or `$XDG_CONFIG_HOME/xraytop/terminal.json`)
with mode `0600`. This does not affect the root-owned systemd collector.

Precedence: **CLI --tui-cols/--tui-rows > saved size > verified ANSI reply >
PTY**. Users with unsupported clients can save a preferred size once. Use
`--tui-ascii` for terminals whose font cannot render Unicode sparklines.
A `TERM=dumb` session skips CSI size probing.

```bash
xraytop              # press z to configure the current terminal
xraytop --tui-cols 140 --tui-rows 45
xraytop --tui-ascii
stty size            # inspect SSH-reported PTY geometry
```



## v0.12.3: readable history trends and complete keyboard footer

The IP-history table and detail page now render their activity trends with
**ASCII-only symbols by default**, avoiding solid/white rectangles produced by
some SSH client fonts. `.` means missing observation, `_` means sampled zero,
and `: = + # @` represent increasing **relative intensity**, not a connection
count. Numeric TCP/Peak/Xray/HTTP columns retain their original sampled values.

The footer now selects the longest **complete** shortcut layout fitting the
actual usable terminal width; it never clips the `[q] Quit` shortcut or shows
a cut-off `[q]...`. The terminal settings screen has width-aware shortcuts as
well. This is strictly a TUI rendering change; the collector, snapshots,
history storage and Xray/NGINX services are unchanged.

