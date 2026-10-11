# XrayTop

[English](README.md)

**XrayTop** 是一个用于 **Linux 本机 Xray-core** 的只读终端监控程序，支持按**代理协议**和**传输方式**独立查看统计信息。适合同时运行 VLESS + gRPC、VLESS + WebSocket、VLESS + XHTTP、VMess、Trojan 等入站的服务器；可识别 NGINX 反代后连接 Xray 的 Unix Socket 监听路径。

> **v0.12.4-dev 开发预览版。** 已完成多协议配置识别与关联、终端界面、进程 CPU/RSS、按入站/出站标签统计、协议和传输汇总。暂未实现原生 gRPC 客户端或每条 TCP 会话的精细流量归属。**无需修改现有代理入站协议**。

## 工作原理

- **代理协议**：`inbounds[].protocol` / `outbounds[].protocol`，例如 VLESS、Trojan、VMess；
- **传输方式**：读取新版 `streamSettings.method` 或旧版 `streamSettings.network`，例如 WebSocket、gRPC、XHTTP、RAW、HTTPUpgrade、mKCP；同时解析 `streamSettings.security`（TLS/REALITY）；
- **监听端点**：读取 `listen` 和 `port`，包括 `unix:/dev/shm/....socket`，仅用于展示配置，**不证明端口正在监听**；
- **实时流量**：通过本机 Xray 的 StatsService，按**精确的 Inbound/Outbound Tag** 匹配配置元数据。每 2 秒采样并计算速率。统计 API 使用 gRPC 只是管理接口协议，**不要求实际代理流量走 gRPC**。

匹配不到的标签仍会展示为 `Unknown`，绝不根据名称（例如 `grpc-01`）臆测协议。配置存在但没有统计计数器时显示 `-`，不当作 0 B。聚合总流量只计算入站统计，避免入站与出站重复计算。协议汇总和传输汇总是同一流量的两种分类，不能彼此相加。


## 安装、升级与 StatsService 配置（v0.12.4-dev）

### 1. 安装前准备

支持 Debian/Ubuntu 等使用 systemd 的 Linux（x86_64 / ARM64）。目标机器需已安装并运行 Xray 和 NGINX，准备好 root/sudo、`curl`、`sha256sum` 等工具。默认安装器读取 `/usr/local/etc/xray/config.json`、`/usr/local/bin/xray` 和 `/etc/nginx/nginx.conf`；其他路径可以通过环境变量指定。

**默认安装是非侵入式的：不修改或重启 Xray/NGINX。** 安装器会检测 StatsService，但如果 API 缺失，只显示警告，不会擅自修复。需要自动启用 StatsService 时，必须显式设置 `XRAYTOP_SETUP_STATS=1`；此模式可能重启 Xray、短暂中断现有代理连接，请使用独立 SSH 管理连接执行。

### 2. 首次安装（推荐）

从固定版本 [v0.12.4-dev Release](https://github.com/Accelerator6666/xraytop/releases/tag/v0.12.4-dev) 下载安装脚本，先阅读再运行：

```bash
curl -fL --retry 3 -o install-xraytop.sh \
  https://github.com/Accelerator6666/xraytop/releases/download/v0.12.4-dev/install.sh

less install-xraytop.sh
sudo bash install-xraytop.sh
```

已经审核并信任安装脚本时，也可一行安装（直接执行远程脚本的安全性低于先下载审核）：

```bash
curl -fsSL https://github.com/Accelerator6666/xraytop/releases/download/v0.12.4-dev/install.sh | sudo bash
```

安装器会自动识别 AMD64/ARM64，从同一 Release 下载 XrayTop、systemd 服务文件和 SHA256SUMS，验证二进制及服务文件校验和与版本，部署 `/usr/local/bin/xraytop` 和 `xraytop-collector.service`，并检查后台快照是否刷新。原程序与服务文件备份在 `/var/lib/xraytop/backups/`；现有 `/var/lib/xraytop/ip-history.json` 不会被覆盖。日常安装/升级只重启 Collector。

### 3. StatsService 缺失时的自动修复（可选）

如果首页显示 `StatsService: UNAVAILABLE` 或安装报告出现 `[WARN] Xray StatsService unavailable`，先检查当前 API：

```bash
sudo /usr/local/bin/xray api statsquery -s 127.0.0.1:10085
```

若确实没有启用 StatsService，可以**主动授权**安装器修复（请复用上一步下载并审核的 `install-xraytop.sh`）：

```bash
sudo env XRAYTOP_SETUP_STATS=1 bash install-xraytop.sh
```

需要 `jq` 时可先在 Debian/Ubuntu 安装：

```bash
sudo apt-get update
sudo apt-get install -y jq
```

修复程序采用以下保护措施：只接受当前 Xray systemd 进程确实使用的**单文件标准 JSON 配置**，API 地址只允许 `127.0.0.1:PORT`；已有 API、重复/冲突的入站标签、配置路径不明时拒绝自动修改。修复时备份原始 `config.json`，保留现有 VLESS、WebSocket、NGINX/Unix Socket、出站和路由，新增 `StatsService`、系统上下行计数并为缺少 tag 的入站生成 `xraytop-inbound-N` 标签。使用带 `.json` 后缀的临时文件和 `xray run -test -format json` 校验后才重启 Xray；若重启后 API 不可用，会尝试回滚原始配置。**自动回滚不等于业务恢复保证**，操作后仍需检查代理连接。

**已配置 StatsService 的服务器无需修复**：运行普通安装命令即可；即使选择修复模式，若 API 查询已经正常，程序也会跳过 Xray 配置修改及重启。采用 `-confdir`、JSONC 注释、多实例、非标准 API 或非默认启动参数时，请人工合并统计配置，勿强行自动修复。

### 4. 安装、升级后验收

```bash
xraytop --version
sudo systemctl is-enabled xraytop-collector
sudo systemctl is-active xray xraytop-collector
sudo systemctl status xraytop-collector --no-pager
sudo journalctl -u xraytop-collector -n 30 --no-pager

# 本地统计 API（已启用时应可成功查询）
sudo /usr/local/bin/xray api statsquery -s 127.0.0.1:10085

# 只读查看 Collector 的历史快照
sudo xraytop --data-source daemon --once --once-view 9

# 进入终端界面；1=总览，6=来源 IP，8=NGINX，9=历史
sudo xraytop
```

若 `StatsService: ONLINE` 且入站显示累计流量，说明 API 计数器已打通。刚启动时速率 `0 B/s` 很正常：需要有真实代理流量，并等待至少两次采样。NGINX 请求次数、当前 TCP 连接数、Xray accepted 事件数**不等于认证在线用户数**。

进一步检查采集详情：

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

### 5. 从旧版本升级、自定义路径与故障排查

升级 `v0.11.2-dev` 或其他旧版本时，只需再次下载当前 Release 的 `install.sh` 并运行普通安装命令；保留历史记录，不需要先卸载，也不需要重设已有 StatsService。特殊安装路径可通过以下参数指定：

```bash
sudo env XRAYTOP_VERSION=v0.12.4-dev \
  XRAYTOP_CONFIG=/usr/local/etc/xray/config.json \
  XRAYTOP_XRAY_BIN=/usr/local/bin/xray \
  XRAYTOP_API=127.0.0.1:10085 \
  XRAYTOP_XRAY_UNIT=xray.service \
  bash install-xraytop.sh
```

另支持 `XRAYTOP_NGINX_CONFIG`、`XRAYTOP_ACCESS_LOG`；运行 `bash install-xraytop.sh --help` 查看参数。自定义路径必须是无空格的绝对路径。

- **API 显示 UNAVAILABLE**：确认 Xray 配置已启用 StatsService，监听地址与 `XRAYTOP_API` 一致，使用 `xray api statsquery` 测试；默认安装不会自动改变 Xray。
- **安装修复被拒绝**：检查是否已有 API 配置、是否使用 `-confdir` 或 JSONC、实际 systemd 服务名与配置路径；根据提示人工检查，不建议强行覆盖。
- **Collector 异常**：查看 `sudo journalctl -u xraytop-collector -n 100 --no-pager`；检查 `/var/lib/xraytop/snapshot.json` 更新时间及 NGINX 日志可读性。
- **恢复配置**：自动修复的 Xray 原配置备份在 `/var/lib/xraytop/backups/<安装批次>/xray-config-*.json`。确认具体备份文件并先校验后，再人工恢复与重启服务；不要盲目覆盖现网配置。

历史文件位于 `/var/lib/xraytop/ip-history.json`，卸载前请另行备份。退出 TUI 不会停止后台采集；如需停止：

```bash
sudo systemctl disable --now xraytop-collector
```

安装器不会自行删除旧版 `10-cert-access.conf` 或 `20-nginx-log.conf` systemd 权限覆盖配置，需要管理员核对后清理。

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

默认运行时，XrayTop 每约 60 秒**只读解析**本机 `/etc/nginx/nginx.conf` 及递归 `include` 的文件（它们是当前磁盘配置，可能尚未加载），根据 `server`/ `listen`/ `proxy_pass`/ `grpc_pass`
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

## 客户端 IP 证据分层及筛选（v0.9）

第 `6` 页将全部来源 IP 严格分成三类，按 **XRAY → HTTP → TCP** 排列：
- `XRAY`：最近 5 分钟 Xray systemd 日志中有 accepted 记录（代理活动证据；不等于仍在线）。
- `HTTP`：只在匹配 Xray 路由的 NGINX combined 日志中出现（HTTP 请求证据；不代表 VLESS 认证）。
- `TCP`：只有当前入口端口上 `ESTABLISHED` 网络连接（不代表代理客户端）。

这三个状态按最强可用证据互斥划分；**新增了之前遗漏的 NGINX-only 来源 IP**。第 6 页增加当前 TCP 入口端口列，多个端口会合并显示。Xray 日志的 Inbound 与 NGINX-only 的 HTTP 路由不会混为已认证身份。按 `f` 可循环过滤：`all → xray → http → tcp`，也可启动时指定：

```bash
./xraytop --config /usr/local/etc/xray/config.json --clients-filter xray
./xraytop --once --once-view 6 --clients-filter http
```

`xray` 表示有 accepted 代理请求记录；`http` / `tcp` 仅展示没有更高级证据的 IP。不同 IP 可能来自同一用户，共享 IP 也可能代表多位用户。所有统计仍只读本机，不需要 email，也无需修改或重启 Xray/NGINX。


## 来源 IP 历史与趋势（v0.10，按键 9）

默认将观察到的来源 IP（只存 IP + TCP 数量、Xray 最近 5 分钟 accepted 次数、
NGINX 最近 5 分钟匹配请求次数及采样时间）保存在运行用户私有目录：
`~/.local/state/xraytop/ip-history.json`，或 `$XDG_STATE_HOME/xraytop/ip-history.json`。
每分钟采样一次，保存 **最近 24 小时**，最多 **128 个来源 IP**；
单个历史文件最大 16 MiB；原子覆盖写入，权限 **0600**。
不会保存 UUID、目标网址、URL 参数、请求头或日志原文。

`9` 为历史概览，`j/k` 或上下方向键选择 IP，按 `Enter` 展开/关闭详情，
`t` 切换 5/15/30/60 分钟趋势，`s` 依次按最近活动、24h TCP 峰值、
当前 TCP、最近一条采样的 Xray accepted 数排序；按 `q` 退出。

```bash
./xraytop --config /usr/local/etc/xray/config.json
./xraytop --once --once-view 9 --config /usr/local/etc/xray/config.json
# 禁用历史写入
./xraytop --history-file ''
# 指定其他本地历史文件
./xraytop --history-file /root/.local/state/xraytop/my-history.json
```

**计量解释：** 每分钟的 TCP 是采样瞬间的 ESTABLISHED 数；
Xray 与 HTTP 是**当时最近 5 分钟的滚动事件数量**，不可跨分钟简单求和。
`·` 表示没有采样（程序未运行或未能记录），`▁` 表示采样值为零。
首次发现/最后活动指 XrayTop 在此保留期内观察到该 IP 的时间，
不证明该 IP 唯一对应某用户，也不能证明在线 VLESS 会话。
历史只在 XrayTop 运行时积累；关闭后不会在后台独立收集。


## v0.11：systemd 后台采集 / 前台只读查看

v0.11 修复历史日期 `100-10` 的显示问题（现在正常显示 `10-10`），
改善较窄终端的趋势列宽，并加入后台采集模式。

`--daemon` 在无终端模式采样，每 2 秒原子发布一份本地状态快照（权限 0600），使用文件锁避免重复后台采集进程。
`--data-source auto`（默认）在本机 `/var/lib/xraytop/snapshot.json` 不超过 15 秒时只读后台结果，不重复采集。
`--data-source daemon` 严格只读后台，`--data-source local` 强制旧版本地采样。

systemd 文件：`packaging/systemd/xraytop-collector.service`。安装后：

```bash
systemctl daemon-reload
systemctl enable --now xraytop-collector
systemctl status xraytop-collector --no-pager
/usr/local/bin/xraytop --data-source daemon --once --once-view 9
/usr/local/bin/xraytop
```

服务以 root 运行以便读取 Xray systemd journal、NGINX 配置和日志，`StateDirectory=xraytop` 会创建私有 `/var/lib/xraytop` 目录；其中历史和快照分别存储在 `ip-history.json` 与 `snapshot.json`。**不修改或重启 Xray、NGINX。**
服务停止后，过期快照不会被当作实时数据。此前 `/root/.local/state/xraytop/ip-history.json` 不会自动迁移，如需保留，必须先停服务、备份后再人工迁移。历史采样依旧不代表已认证在线会话。


## v0.11.1：修复受限 systemd 服务无法执行 `nginx -T`

之前使用 `nginx -T` 做 NGINX 自动发现，实际会执行配置测试并尝试打开
证书、`error.log` 和 `/run/nginx.pid`，在严格 sandbox 下不可靠。
现在默认只读解析 `/etc/nginx/nginx.conf` 和展开的 `include`，
按后端 Unix Socket/回环地址与 Xray Inbound 精确匹配，**不运行 `nginx -T`**，
不打开 TLS 证书、日志或 PID 文件。自定义主配置文件用：
`--nginx-config-file /path/to/nginx.conf`。

由于只读文件解析的是磁盘内容，若修改 NGINX 配置但尚未 reload，它可能
**不等于当前运行的 NGINX 配置**；建议用单独的管理员命令验证 `nginx -t`
并确保配置已生效。无法读取某一处 include 时明确报告错误，而不产生虚假的
入口路由。此修复无需修改 Xray 或 NGINX 配置，服务继续保留严格隔离。

日志读取是独立的采集步骤；若 access.log 属于 `www-data:adm`、权限
`0640`，服务又清空了 Linux capabilities，可按实际需要只给服务增加
`SupplementaryGroups=adm` 以获得只读日志权限，不要使用 `CAP_DAC_OVERRIDE`。

## v0.12：增量采集、Doctor 自检与来源 IP 搜索

**v0.12.4-dev** 对高流量 VPS 的数据采集作了重要改进：

- **NGINX access.log**：在采集进程中记录文件 inode 和偏移量，仅增量读取新内容；冷启动从最后最多 2 MiB 初始化。仅保留最近 5 分钟的来源 IP／入站／HTTP 状态元数据，不保存 URL、查询参数或原始日志。轮转、截断和处理限额可能导致窗口不完整，界面会明确提示。
- **Xray journal**：使用 `journalctl --output=json` 与 `__CURSOR` 逐批读取新记录，避免每次重复查询最近 500 条；每批有大小和行数上限。
- 游标目前**只在运行内存中保存**。重启后重新初始化最近五分钟，因此不是无遗漏的审计流水。历史中的 Xray/HTTP 仍是采样时最近五分钟的**滚动事件量**，不能跨分钟求和。

**只读 Doctor 诊断**：

```bash
xraytop --doctor --config /usr/local/etc/xray/config.json \
  --xray-bin /usr/local/bin/xray --api 127.0.0.1:10085
```

检测采集服务、快照新鲜度、StatsService、NGINX 反代路由、日志文件权限与采集错误。`FAIL` 表示检查失败并返回非零退出码；`WARN` 提示可选功能受限。该命令**不会修改配置或重启 Xray/NGINX**。

**IP 搜索**：第 `6` 页和第 `9` 页按 `/` 输入 IP 子串；`Enter` 应用、`Esc` 取消、`Ctrl-U` 清空。搜索只影响界面呈现，不会改变历史采集。

```bash
xraytop --data-source daemon --once --once-view 6 --ip-search 192.0.2
xraytop --data-source daemon --once --once-view 9 --ip-search 2001:db8
```



## v0.12.1：修复 SSH 窄屏历史表格显示

第 9 页历史视图现在根据终端实际列宽自适应：
- 110 列及以上：完整 IPv6 地址、首次/末次活动时间、TCP / Peak / Xray / HTTP 计数及趋势；
- 66–109 列（含常见 80 列 SSH）：优先保留最后活动时间、**四项计数**和 TCP 趋势，缩短 IP 列；选中后按 Enter 可查看未截断的完整 IP 和首次活动时间；
- 少于 66 列：两行一条，避免右侧字段被直接截断。上下键或 j/k 可滚动浏览全部历史 IP。

若 SSH 客户端报告的窗口行列数与实际窗口不一致（如画面只占屏幕左上角，大片空白），先在 VPS 执行 `stty size`，并调整 SSH 终端窗口大小或其终端尺寸配置。也可以在**确保真实窗口容量足够**的情况下临时指定：

```bash
xraytop --tui-cols 120 --tui-rows 40
```

默认两项为 0（使用终端上报尺寸）；错误地把覆盖值设得过大可能导致文本自动折行。


## v0.12.2：跨 SSH 客户端的终端适配

直接运行 `xraytop` 时，程序先读取 PTY 当前尺寸（TIOCGWINSZ），再**非阻塞**发出 xterm 兼容的 `CSI 18t` 文本区域尺寸查询。若客户端支持并返回合法的 `ESC[8;行;列t`，则在当前会话自动重绘，窗口大小变化时再次检测。**并非所有 SSH 客户端支持该协议**：无响应或不可信响应会使用 PTY 尺寸，不会阻塞界面；无法通过终端协议或 PTY 获得真实大小时，无法保证自动扩展到物理窗口。

TUI 新增 **`z` 终端设置页**，使用 `w` 切换列数（auto / 80 / 100 / 120 / 140 / 160 / 180）、`h` 切换行数（auto / 24 / 30 / 40 / 45 / 50 / 60）、`p` 开关主动尺寸协商、`a` 开关 ASCII 绘图、`r` 恢复默认、`s` 保存设置、`z` 或 Esc 取消。这些选项保存在当前运行用户的 `~/.config/xraytop/terminal.json`（也遵循 `XDG_CONFIG_HOME`），权限为 `0600`。不同 VPS 用户分别拥有各自的配置，不影响后台采集器。

尺寸选择优先级：**命令行 `--tui-cols`/`--tui-rows` > 用户保存尺寸 > ANSI 终端响应 > PTY 上报值**。如自动检测失败，可仅保存一次尺寸预设，后续无需额外参数；如终端不支持 Unicode 字符，可选 `--tui-ascii` 或在设置页打开 ASCII 模式。

```bash
xraytop
# 在 TUI 按 z 进入终端设置

# 也可以手动覆盖（请确保真实窗口至少容纳这些行列）
xraytop --tui-cols 140 --tui-rows 45
xraytop --tui-ascii

# 检查 SSH 上报值
stty size
```

旧版普通 SSH 用户无需升级 Xray/NGINX，也无需修改 StatsService。只有 TUI 的终端交互和本地用户偏好发生变化。`TERM=dumb` 不发送尺寸查询。



## v0.12.3：历史趋势字符与底部菜单兼容性修复

- 第 9 页的 TCP、XRAY、HTTP 趋势图改为**默认 ASCII 字符**，不再依赖部分 SSH 客户端无法正确呈现的 Unicode 方块字形。趋势中的 `.` 表示**未采样**，`_` 表示**已采样且为零**，`: = + # @` 是从低到高的**相对强度**，并非实际连接数量。列表中的 TCP／Peak／Xray／HTTP 数字继续显示实际采样值。
- 底部快捷键菜单不再被固定长度硬截断，而是根据终端的实际内容宽度，在完整、中等、紧凑和极窄菜单之间选择。优先保留 `[q] Quit` 和 `[z] Settings`，避免原先在 140 列窗口尾部出现 `[q]...`。
- 宽度适配同样作用于 `z` 终端设置页；测试覆盖多档宽度和趋势图中的缺失/零/有值数据。
- 这属于**纯显示修复**：不会改变 Collector、已有历史 JSON、日志采集方式、Xray 或 NGINX。


## v0.12.4：btop 风格来源 IP 趋势仪表

第 9 页采用 btop 风格的**分段色块**显示，同时保留普通 SSH 环境可读的 ASCII 图形。

- **统一 TCP 刻度**：同一页面上所有 IP 使用相同刻度，至少为 16 条连接，峰值较高时提升至 32、64、128……。持续 2 条 TCP 连接不会再显示成整排最高强度。
- **分钟时间轴**：趋势图有边框，由左至右表示过去到现在。按 `t` 切换 5/15/30/60 分钟窗口；空间不足时按时间段取最大观测值。空格表示**没有采样**，`.` 为**采样值为零**，`: = + # @` 为在当前刻度下从低到高的强度。
- **IP 详情**：第 9 页用 `j/k` 选择 IP，按 `Enter` 查看 TCP、XRAY 和 HTTP 三组分段仪表（例如 `2/16`）及各自分钟趋势。TCP 是观测到的网络连接数量；XRAY、HTTP 是**最近五分钟滚动事件数**，不是认证在线人数、累计请求数或精确的每 IP 字节流量。
- **显示兼容**：彩色模式在确定宽度和裁剪之后再绘制 ANSI 背景色块，避免颜色控制符影响列对齐；`--no-color` / ASCII 模式显示 `[||----]`、`[ .:=+#@]` 等纯文本标记。
- **升级安全**：本次只修改 TUI 绘制逻辑，不修改采集器、现有 24 小时 IP 历史 JSON、Xray 和 NGINX 配置。

彩色条的强度相对于明确显示的刻度（如 `TCP 0-16`），**不是 CPU 使用百分比**；缺失采样不能推断为没有连接。

