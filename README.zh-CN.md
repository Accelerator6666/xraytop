# XrayTop

[English](README.md)

**XrayTop** 是一个用于 **Linux 本机 Xray-core** 的只读终端监控程序，支持按**代理协议**和**传输方式**独立查看统计信息。适合同时运行 VLESS + gRPC、VLESS + WebSocket、VLESS + XHTTP、VMess、Trojan 等入站的服务器；可识别 NGINX 反代后连接 Xray 的 Unix Socket 监听路径。

> **v0.2.0-dev 开发预览版。** 已完成多协议配置识别与关联、终端界面、进程 CPU/RSS、按入站/出站标签统计、协议和传输汇总。暂未实现原生 gRPC 客户端或每个用户的会话关联。**无需修改现有代理入站协议**。

## 工作原理

- **代理协议**：`inbounds[].protocol` / `outbounds[].protocol`，例如 VLESS、Trojan、VMess；
- **传输方式**：读取新版 `streamSettings.method` 或旧版 `streamSettings.network`，例如 WebSocket、gRPC、XHTTP、RAW、HTTPUpgrade、mKCP；同时解析 `streamSettings.security`（TLS/REALITY）；
- **监听端点**：读取 `listen` 和 `port`，包括 `unix:/dev/shm/....socket`，仅用于展示配置，**不证明端口正在监听**；
- **实时流量**：通过本机 Xray 的 StatsService，按**精确的 Inbound/Outbound Tag** 匹配配置元数据。每 2 秒采样并计算速率。统计 API 使用 gRPC 只是管理接口协议，**不要求实际代理流量走 gRPC**。

匹配不到的标签仍会展示为 `Unknown`，绝不根据名称（例如 `grpc-01`）臆测协议。配置存在但没有统计计数器时显示 `-`，不当作 0 B。聚合总流量只计算入站统计，避免入站与出站重复计算。协议汇总和传输汇总是同一流量的两种分类，不能彼此相加。

## 编译和运行

需要 Go 1.23+，直接下载仓库编译：

```bash
git clone https://github.com/Accelerator6666/xraytop.git
cd xraytop
go build -o xraytop ./cmd/xraytop
./xraytop
```

默认自动从本机 Xray 进程的命令行（`/proc/<PID>/cmdline`）寻找 `-c` / `-config` / `-confdir` 配置路径。Xray 使用 systemd 启动时一般可以直接识别。如果没有权限读取配置文件、使用环境变量指定配置目录、或 Xray 的启动方式特殊，请手动指定：

```bash
# 单一配置文件，包括带注释的 JSON
./xraytop --config /usr/local/etc/xray/config.json

# Xray -confdir 多文件模式：按文件名顺序加载 *.json；同 tag 后者覆盖前者
./xraytop --config-dir /usr/local/etc/xray/conf.d

# 同时读取多个文件
./xraytop --config /etc/xray/01.json --config /etc/xray/02.json

# 仅显示协议配置与进程指标，不读取 StatsService
./xraytop --config /usr/local/etc/xray/config.json --api ''

# 非交互输出，用于检查识别结果
./xraytop --once --config /usr/local/etc/xray/config.json --api ''
```

**键盘操作：** `1` 总览、`2` 入站、`3` 出站、`4` 代理协议汇总、`5` 传输方式汇总；`Tab` 循环、`r` 刷新、`q` 退出。汇总视图会显示各分类的入站数量、拥有统计计数器的数量、上下行速率与累计流量。

## 启用实时流量监控

本工具本身不会打开 Xray API、不会写入 Xray 配置。StatsService 必须由运行中的 Xray 启用，并限制在本机回环接口。应将下列设置**合并**到已有 Xray 配置，保留原有代理入站、出站、路由、NGINX 和 Unix Socket：

- `api.services` 包含 `StatsService`，并确保有本地 API 入站以及指向 `api` 出站的路由；
- `stats: {}` 和 `policy.system.statsInboundUplink/statsInboundDownlink/statsOutboundUplink/statsOutboundDownlink: true`；
- 若需要用户级统计，还需要相应 `policy.levels` 和客户 `email`（当前版本未在 UI 呈现用户统计）。

验证：

```bash
xray api statsquery -s 127.0.0.1:10085
./xraytop --api 127.0.0.1:10085 --config /usr/local/etc/xray/config.json
```

目前由 Xray 自带的 `xray api statsquery` 子命令调用 StatsService（需要服务器上可执行 `xray`）。`--xray-bin /path/to/xray` 可指定二进制。计划后续升级到**原生 gRPC 客户端**，降低周期性子进程开销；这个 API 仍然兼容所有代理协议。

## 边界条件

- 仅监控**本机**，不开启远程管理，不改变 Xray 配置，不能使用统计数据判断某个监听端口是否正常提供服务；
- 仅从 JSON/JSONC 配置抽取标签、协议、传输、安全层和监听地址，**不会加载或显示 UUID、密码、私钥、用户设置**；当现网配置为非 JSON 格式时请转换为受支持的 JSON 文件；
- 默认仅支持 `127.0.0.1` / `::1` / `localhost` StatsService API；
- NGINX → Unix Socket → Xray 时，API 流量计数可使用，但不能直接把 Xray 看到的 IP 或本地 TCP 连接数解释为最终客户 IP/在线数；
- 配置展示来自本机文件，可能与正在运行的实例状态不一致。使用明确的入站 `tag` 才能可靠匹配 StatsService 数据；
- 多 Xray 进程时自动配置发现只选择最低 PID，不会把不同进程配置错误地组合在一起；
- 第一次成功采样没有速率；API 出错会保留上一次累计值、停止展示实时速率。

## 开发和测试

```bash
go test ./...
go vet ./...
```

CI 自动测试，并交叉编译 Linux amd64/arm64。源代码见 [ARCHITECTURE.md](docs/ARCHITECTURE.md)。MIT License。
