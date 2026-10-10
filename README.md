# XrayTop

[简体中文](README.zh-CN.md)

**XrayTop** is a Linux-only, read-only, btop-inspired terminal monitor for a **local** Xray-core instance.

> **Status: v0.1.0-dev (working early preview).** Linux process CPU/RSS, terminal dashboard, and inbound/outbound cumulative traffic through Xray's StatsService are implemented. Per-user sessions, remote monitoring, nginx access-log correlation and native gRPC (without a CLI subprocess) are **not** implemented yet.

## Features

- Full-screen keyboard-driven ANSI terminal dashboard; live graphs, traffic and per-tag rankings
- Local Xray process discovery from `/proc`, PID, CPU, RSS, process count
- Non-destructive per-inbound and per-outbound traffic counters; per-inbound rates derived from consecutive samples
- Safe loopback-only Xray API connection, 2-second refresh default, read-only access
- No third-party Go dependencies. The optional StatsService client invokes the installed `xray api statsquery` subcommand once per sample.
- Linux amd64/arm64 CI builds; no root required if procfs is readable

## Install / build

Go 1.23+ is needed. An installed Xray-core executable is required **only for API traffic statistics**.

```bash
git clone https://github.com/Accelerator6666/xraytop.git
cd xraytop
go build -o xraytop ./cmd/xraytop
./xraytop --api 127.0.0.1:10085
```

If API statistics are disabled, the dashboard still shows process statistics:

```bash
./xraytop --api ''
./xraytop --once --api ''  # plain output for cron / pipes
./xraytop --interval 1s --api 127.0.0.1:10085
./xraytop --xray-bin /usr/local/bin/xray
```

Use `1`, `2`, `3` to switch between overview, inbounds and outbounds; `Tab` to cycle, `r` to refresh and `q` or `Ctrl-C` to quit. The interactive mode requires a real terminal.

## Enable Xray statistics

The target Xray instance must have `StatsService` enabled, API inbound routed to the `api` outbound, and relevant `policy.system.statsInboundUplink`, `statsInboundDownlink`, `statsOutboundUplink`, `statsOutboundDownlink` flags enabled. Please **merge** these settings into the existing Xray configuration instead of replacing your configuration. The example below is intentionally incomplete and does not include your VLESS inbounds:

```json
{
  "api": {"tag": "api", "services": ["StatsService"]},
  "stats": {},
  "policy": {"system": {"statsInboundUplink": true, "statsInboundDownlink": true, "statsOutboundUplink": true, "statsOutboundDownlink": true}},
  "inbounds": [{"tag": "api", "listen": "127.0.0.1", "port": 10085, "protocol": "dokodemo-door", "settings": {"address": "127.0.0.1"}}],
  "routing": {"rules": [{"type": "field", "inboundTag": ["api"], "outboundTag": "api"}]}
}
```

Do not expose the API to public networks. Test it with `xray api statsquery -s 127.0.0.1:10085`. Ensure this API routing matches your existing routing and avoid duplicate inbound tags.

## Interpretation and limitations

- Uplink/downlink means Xray's **inbound** bytes in each direction (not overall VPS interface bandwidth).
- The first sample has no instantaneous rate. Counter resets and newly observed tags do not produce artificial spikes. Data is **not persisted** across restarts.
- Inbound plus outbound bytes are not summed to avoid double counting.
- With nginx + gRPC/WebSocket Unix sockets, local socket counts and apparent peer IPs are **not** reliable per-user session counts.
- If `xray` is missing from `PATH` or stats are disabled, the UI displays an error rather than fake traffic metrics. Use `--xray-bin` if needed.

## Development

```bash
go test ./...
go vet ./...
```

Roadmap: native gRPC backend, users and session correlation, nginx logs, release packaging. See [architecture](docs/ARCHITECTURE.md).

MIT License.
