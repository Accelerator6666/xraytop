# XrayTop

[English](README.md)

XrayTop 是专为 **Linux 本机 Xray-core** 开发的只读终端监控程序，采用类似 btop 的全屏界面。

> 当前版本：**v0.1.0-dev 可运行预览版**。已实现进程 CPU/RSS、实时终端仪表盘、入站/出站累计流量及速率。尚未实现用户会话、NGINX 真实客户端 IP 关联或原生 gRPC 连接。

## 编译

需要 Go 1.23 或更新版本。实时流量使用本机 `xray api statsquery` 读取 Xray StatsService；不需要第三方 Go 模块。

```bash
git clone https://github.com/Accelerator6666/xraytop.git
cd xraytop
go build -o xraytop ./cmd/xraytop
./xraytop --api 127.0.0.1:10085
```

没有启用 Xray API 也能监控进程：

```bash
./xraytop --api ''
./xraytop --once --api ''
./xraytop --interval 1s --api 127.0.0.1:10085
./xraytop --xray-bin /usr/local/bin/xray
```

- `1` 总览，`2` 入站，`3` 出站，`Tab` 切换
- `r` 立即刷新，`q` / `Ctrl-C` 退出
- `--once` 非交互文本模式，适合 SSH 脚本
- 默认每 2 秒采样一次，不会主动清零 Xray 统计计数器

## Xray API 预备工作

需要在**已有配置基础上合并**：`api.services` 包含 `StatsService`、`stats: {}`、`policy.system` 的入站与出站统计开关，以及绑定 `127.0.0.1:10085` 的 API 入站和转发至 `api` 出站的路由。配置示例见 [English README](README.md#enable-xray-statistics)。不要直接覆盖你已经在运行的 VLESS / gRPC / WebSocket 配置。

先测试：

```bash
xray api statsquery -s 127.0.0.1:10085
```

**注意：** 本项目只监控本机；默认只读，不修改 Xray 配置。采用 NGINX → Unix Socket → Xray 架构时，不能把 Linux TCP 连接数当作真实用户会话数。速率以 Xray 入站计数器的采样差值计算，不等于 VPS 网卡总吞吐；第一次读取时只显示累计值。

## 当前开发进度

| 项目 | 状态 |
| --- | --- |
| Linux Xray 进程检测 | 完成 |
| 进程 CPU / RSS | 完成 |
| 终端 TUI 与实时曲线 | 完成（预览版） |
| StatsService 流量与入站/出站标签 | 完成（经 xray CLI） |
| 原生 gRPC 统计客户端 | 计划 |
| 用户、连接、访问日志关联 | 计划 |
| GitHub Actions amd64/arm64 CI | 完成 |

`go test ./...` 运行单元测试；后续规划和架构见 [ARCHITECTURE.md](docs/ARCHITECTURE.md)。许可证：MIT。
