# XrayTop architecture — local-only / multi-protocol

```
XrayTop terminal view (overview / inbounds / outbounds / protocols / transports / online users)
         |
  Collector ------------- Linux procfs: xray PID, CPU, RSS, cmdline, cwd
         |                           |
         |                      config paths (-c/-config/-confdir)
         |                           |
         |                      JSON/JSONC descriptor reader
         |                           |  tag, proxy protocol, transport method,
         |                           |  security, listen/port (no secrets)
         |                           |
         +-------- Join by exact inbound/outbound TAG --------+
         |                                                    |
  Xray bundled CLI --> StatsService on 127.0.0.1 (gRPC management)     |
         |                  |                                 |
         +----- cumulative counters, non-resetting --------------+
                              |
                   delta + elapsed seconds = rates
                              |
                   grouped inbound counters only
                 /                              \
          proxy protocol                    transport method
```

Note that **API gRPC != inbound gRPC transport**: every type of proxy inbound can expose counters via the same management endpoint.

## Local configuration discovery

1. If any `--config` or `--config-dir` option is passed, use the explicit path(s), never remotely fetch or execute config.
2. Otherwise inspect the lowest PID `xray` process in `/proc` and resolve `-c`/`-config` and `-confdir` against its cwd. Do not inspect unrelated processes.
3. `-confdir` files are loaded in file name order (`*.json`). Entries with the same non-empty tag in later files replace the earlier metadata, following Xray confdir behavior. Empty tags are not reliably joinable to StatsService metrics.
4. Only parse `tag`, `protocol`, `listen`, `port`, and `streamSettings.method/network/security`; secrets under `settings`, TLS private keys etc. are not exposed. The parser is not a replacement for Xray's own config validation.
5. File reads refresh per sampling cycle. A config mismatch after an Xray restart may appear temporarily; file metadata is never interpreted as evidence of a live socket.

## Statistics and aggregation

- `internal/xray/stats.go` parses `inbound>>>TAG>>>traffic>>>uplink/downlink` and equivalent outbound counters.
- `Sampler` derives rates only from monotonic counters and doesn't reset them. First sample has no rate; a decreased counter produces zero rate rather than a spurious spike.
- `JoinEndpoints` retains configured entries even when StatsService is disabled and retains stats-only tags with `Unknown` metadata. It never infers transport from naming conventions or network ports.
- Aggregate by protocol or by transport is a **different grouping of the same inbound counters**. Do not add the group totals together or combine with outbounds.
- Unix Socket metadata identifies the local Xray endpoint and does not reveal the client IP behind nginx. A future independent nginx log collector would be required for correlation.
- Cumulative stats are deliberately not persisted or sampled when XrayTop exits. v0.3 adds an online-user map view; user-level traffic rates remain a future feature.

## Roadmap

- Replace the periodic `xray api statsquery` subprocess with a native, local-only gRPC StatsService client.
- Improve narrow terminal layouts, scrolling and additional port/listener details.
- Implement optional nginx local access-log correlation as separate, clearly labeled observations without falsely equating connections and users.
- Package binaries in tagged GitHub Releases; continue testing amd64 and arm64 via CI.


## Online view

The Xray 26.3.27 StatsService exposes `GetAllOnlineUsers` and `GetStatsOnlineIpList`.
Every ~6 seconds the collector uses their Xray CLI wrappers to show the source
IPs currently tracked by Xray, with the Unix `last_seen` timestamp. Bounded at
64 users, 4 concurrent IP lookups. This requires a per-level statsUserOnline
policy and unique client email labels; the application never changes config.
Online IP data is **not** inferred from accumulated traffic, logs, or inbound
TCP counts. Loopback-origin sources are not tracked by Xray's online map, and
reverse proxies may hide the original IP.


## Recent accepted connections (v0.4 preview)

The new `internal/xray/activity.go` collector reads systemd's local journal (`journalctl -u xray --since '5 minutes ago' -o short-unix -n 500`). Parsing accepts only lines containing `from IP:port accepted ... [inbound >> outbound]` with validated IPv4/IPv6 source addresses; rejected requests are excluded. The collector is bounded and can be disabled using `--journal-unit ''`. It does not parse UUIDs or destinations into the TUI. The observed address may be forwarded by a reverse proxy; no identity or online-state inference is performed.

The `6` Users view and `7` Recent IP view are deliberately separate: Xray's online map supports authenticated user/IP associations only with its applicable `statsUserOnline` policies and client email identifiers. Journal entries can indicate recent accepted traffic but are not sessions or user identities.

## Optional live TCP observations

Linux host procfs `/proc/net/tcp` and `/proc/net/tcp6` provide ESTABLISHED sockets, filtered only by explicit `--ingress-ports`. This network-level view is joined to recent accepted journal records on normalized IP for convenience, not authenticated user identity. Reading is local and does not require an Xray config change. IPv6 procfs entries require little-endian decoding per 32-bit word. Peer socket counts must never be presented as authenticated Xray user sessions or exact per-inbound connections.


## NGINX reverse-proxy port discovery (v0.7)

The default local-only auto-discovery calls `nginx -T` (read-only) every ~60s, parses brace-scoped `server`, `location`, `listen`, `upstream`, `server`, `proxy_pass` and `grpc_pass` directives. Exact backend matches against the current Xray Catalog's Unix-domain socket and loopback listener endpoints yield monitored TCP ingress ports and corresponding Xray inbound tags. A port can map to more than one inbound. Nonmatching web-only server blocks are excluded. Manual `--ingress-ports` overrides auto discovery and `--nginx-auto=false` disables it. Configuration matches are not proof of per-socket authentication or valid source-IP forwarding.
