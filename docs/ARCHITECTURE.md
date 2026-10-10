# XrayTop architecture (local-only)

```
XrayTop terminal UI -> Collector -> /proc (CPU, RSS, PID)
                             \-> Xray bundled CLI -> gRPC StatsService (loopback only)
```

Runtime polls every 2 seconds by default, and only the most recent sample is rendered. UI events never modify the Xray server.

`internal/xray` decodes the JSON returned by `xray api statsquery` and stores raw counters. The rate sampler compares consecutive successful cumulative samples without using `reset=true`, so it is safe alongside other StatsService clients. When Xray restarts or counters decrease, rates are clamped to zero. Inbound counters alone are summed to avoid double counting inbound/outbound mirrored traffic. Metric direction is from the Xray inbound perspective.

`internal/system` reads procfs for the `xray` process; CPU percent is computed from process ticks relative to kernel ticks and may exceed 100 percent for multithreaded processes. If multiple local Xray processes exist, the lowest PID is displayed for CPU/RSS and the process count is shown.

`internal/tui` renders ANSI terminal frames and manages Linux terminal raw mode. Planned replacement: Bubble Tea v2 once a pinned dependency is integrated and tested. Planned stats client: native gRPC with generated protobuf code to remove periodic CLI subprocesses. Additional roadmap: structured nginx logs, per-user sessions, logs and release packaging.

Not supported: remote host management, configuration edits, node selection, persistent daemon, external API exposure.
