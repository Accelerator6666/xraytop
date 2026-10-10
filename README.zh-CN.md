# XrayTop

[English](README.md)

**XrayTop** 是一个用于 **Linux 本机 Xray-core** 的只读终端监控程序，支持按**代理协议**和**传输方式**独立查看统计信息。适合同时运行 VLESS + gRPC、VLESS + WebSocket、VLESS + XHTTP、VMess、Trojan 等入站的服务器；可识别 NGINX 反代后连接 Xray 的 Unix Socket 监听路径。

> **v0.8.0-dev 开发预览版。** 已完成多协议配置识别与关联、终端界面、进程 CPU/RSS、按入站/出站标签统计、协议和传输汇总。暂未实现原生 gRPC 客户端或每条 TCP 会话的精细流量归属。**无需修改现有代理入站协议**。

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

**键盘操作：** `1` 总览、`2` 入站、`3` 出站、`4` 代理协议汇总、`5` 传输方式汇总、`6` 客户端来源 IP 汇总；`Tab` 循环、`r` 刷新、`q` 退出。汇总视图会显示各分类的入站数量、拥有统计计数器的数量、上下行速率与累计流量。

## 启用实时流量监控

本工具本身不会打开 Xray API、不会写入 Xray 配置。StatsService 必须由运行中的 Xray 启用，并限制在本机回环接口。应将下列设置**合并**到已有 Xray 配置，保留原有代理入站、出站、路由、NGINX 和 Unix Socket：

- `api.services` 包含 `StatsService`，并确保有本地 API 入站以及指向 `api` 出站的路由；
- `stats: {}` 和 `policy.system.statsInboundUplink/statsInboundDownlink/statsOutboundUplink/statsOutboundDownlink: true`；
- IP 优先模式不要求客户 `email` 或 `statsUserOnline`，它只从本机日志获取 IP 的最近接受记录。

验证：

```bash
xray api statsquery -s 127.0.0.1:10085
./xraytop --api 127.0.0.1:10085 --config /usr/local/etc/xray/config.json
```

Xray 26.3.27 在零流量计数时可能省略 `value` 字段；XrayTop 会将此类计数器记为 **0 B**，不会误判为 API 故障。

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


## IP 优先客户端监控（按键 `6` 和 `7`）

**无需配置 `email`，无需启用 `statsUserOnline`，无需改动现有 Xray / NGINX 配置。** 默认按键 `6` 是 **Clients / 来源 IP 汇总**：把相同来源 IP 在不同入站（如 WebSocket 和 gRPC）的 **accepted 日志记录**合并，显示 IP、所用传输类型、最近 5 分钟观察到的接受记录次数与最后活动时间。按 `7` 查看按 **IP + Inbound Tag** 拆分的详细列表。两页都来自本机 systemd journal，不读取用户 UUID。

```bash
# 按 IP 聚合，在终端直接输出（无需 Go 环境）
./xraytop --once --once-view 6 --config /usr/local/etc/xray/config.json

# 按 IP + 入口展开的列表
./xraytop --once --once-view 7 --config /usr/local/etc/xray/config.json

# 交互式监控，按 6 或 7
./xraytop --config /usr/local/etc/xray/config.json --api 127.0.0.1:10085

# Xray systemd 单元不是 xray 时
./xraytop --journal-unit xray@one.service
```

采集器默认每约 10 秒读取 **最近 5 分钟、最多 500 条** `journalctl -u xray` 日志，并仅解析带源 IP 和有效入站标签的 `accepted` 记录；拒绝、鉴权失败的连接不作为成功客户端。IP 只是**分组标识**，不一定对应唯一用户：NAT/共享公网 IP、移动网络换 IP、NGINX 转发和用户可控代理头都可能影响结果；请仅在可信 NGINX 配置下相信透传的源 IP。**accepted 日志不证明连接仍然在线，次数不是活跃 TCP 数量，日志也不包含逐 IP 精确流量。** 页面故意标明「最近连接」，不将其冒充在线状态。

Xray StatsService 仍继续提供按入站/出站汇总的实时流量。在线用户 RPC (`statsgetallonlineusers`/`statsonlineiplist`) 因你的安装没有 email 标识，在 IP 模式下不再轮询；它们与真实的源 IP 日志记录不同。

如未显示来源 IP，请检查：

```bash
journalctl -u xray --since '5 minutes ago' --no-pager | grep ' accepted ' | tail -10
```

**只需更新 XrayTop 可执行文件，不要为了 IP 监控修改 Xray 用户 ID、API 或重启服务。**

## 来源 IP 与当前 TCP 连接（v0.6）

使用 `--ingress-ports` 指定本机实际用于 Xray/NGINX 接入的 TCP 监听端口，才会启用 TCP 快照。**请勿照搬示例端口，先检查你的实际 NGINX 监听配置。**

```bash
./xraytop --config /usr/local/etc/xray/config.json \
  --api 127.0.0.1:10085 --ingress-ports 443,8964
./xraytop --once --once-view 6 --ingress-ports 443,8964
```

- **按键 6**：将 Linux `/proc/net/tcp`、`/proc/net/tcp6` 的当前 `ESTABLISHED` 连接来源 IP 与 Xray 最近 5 分钟的 `accepted` 日志按 IP 合并显示；没有 `--ingress-ports` 时只显示日志数据。
- **按键 7**：仍然按照来源 IP + Inbound 展示最近接受连接的日志，并显示该 IP 在指定入口端口上建立的 TCP 连接数（不是对应入站会话数）。
- 无需客户端 `email`、不启用额外服务或端口，也不修改 Xray 配置。

**限制**：入口的 TCP 连接数量不等于通过 Xray 认证的客户端数量。尤其 NGINX 同时承载网站、HTTP/2、WebSocket 与 gRPC 时，不能可靠地根据一个 TCP socket 推断它属于哪个 VLESS 用户或 Inbound。NAT/CDN/反代还可能导致 NGINX 见到的 IP 与 Xray 日志中的来源不同。按 IP 分组并不是严格的用户身份识别，也不能由此计算每 IP 的准确流量。


## NGINX 入口端口自动发现（v0.7）

默认运行时，XrayTop 每约 60 秒使用本机 `nginx -T` 读取实际生效的
NGINX 配置，根据 `server`/ `listen`/ `proxy_pass`/ `grpc_pass`
以及命名 upstream 的后端地址进行匹配。**只有后端与 Xray 配置中的
Unix Socket 或回环 TCP 入站监听地址准确匹配，才会加入 TCP 采样端口**。
不会猜测端口，也不会更改或重新加载 NGINX。页面 6 提供自动识别状态。

```bash
# 自动发现（默认）：不必再手动输入 15 个端口
./xraytop --config /usr/local/etc/xray/config.json --api 127.0.0.1:10085
# 无需全屏，直接检查第 6 页
./xraytop --once --once-view 6 --config /usr/local/etc/xray/config.json
# 特殊情况手动覆盖自动识别
./xraytop --ingress-ports 1443,2443
# 关闭 NGINX 自动识别
./xraytop --nginx-auto=false
```

注意：若 NGINX 同时承载普通 HTTPS，检测到的 TCP 连接仍可能是网站访客，
并不能直接当作已认证的 Xray 用户；一个端口也可能承载多种入站协议。
自动发现无法读取或无法准确匹配的情况会显示警告，而不会盲目纳入端口。


## NGINX HTTP 访问与源 IP 关联（v0.8，按键 8）

自动发现模式下，XrayTop 每约 10 秒只读取 `/var/log/nginx/access.log` **末尾最多 2 MiB**，识别默认 NGINX combined 日志中的来源 IP、时间、请求方法和状态码；仅统计请求路径与已确认的 Xray WebSocket/gRPC `location` 配置匹配的记录。内部用 URL 路径做定位，但**不会将完整 URL、查询字符串、User-Agent、Referer 或请求头显示或保存到监控结果中**。

```bash
./xraytop --config /usr/local/etc/xray/config.json --api 127.0.0.1:10085
./xraytop --once --once-view 8 --config /usr/local/etc/xray/config.json
# 如不想读取 access.log，可完全禁用该采集器：
./xraytop --nginx-access-log ''
```

第 6 页提供来源 IP 的 Xray accepted、当前 TCP 和 NGINX HTTP 请求次数；第 8 页展示最近 5 分钟按 IP、Inbound、传输类型统计的 HTTP 请求数量、101/200 响应数量。**成功的 HTTP 101/200 并不代表成功的 VLESS 身份验证，只有 Xray accepted 日志反映代理请求通过 Xray 的处理**。默认 combined 日志未记录入口 TCP 端口，若同一路径在不同虚拟主机匹配不同 Xray 入站，工具会跳过无法唯一判断的归类。日志超过 2 MiB 的旧记录可能被跳过，故统计不一定覆盖完整 5 分钟。不能按普通 HTTP 响应字节数计算每 IP 的代理流量。
