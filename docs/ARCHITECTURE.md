# XrayTop architecture — local-only / multi-protocol

```
XrayTop terminal view (overview / inbounds / outbounds / protocols / transports)
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
- Cumulative stats are deliberately not persisted or sampled when XrayTop exits. No user-level traffic in v0.2.

## Roadmap

- Replace the periodic `xray api statsquery` subprocess with a native, local-only gRPC StatsService client.
- Improve narrow terminal layouts, scrolling and additional port/listener details.
- Implement optional nginx local access-log correlation as separate, clearly labeled observations without falsely equating connections and users.
- Package binaries in tagged GitHub Releases; continue testing amd64 and arm64 via CI.
