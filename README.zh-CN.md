# XrayTop

[English](README.md)

**XrayTop** 是一个用于 **Linux 本机 Xray-core** 的只读终端监控程序，支持按**代理协议**和**传输方式**独立查看统计信息。适合同时运行 VLESS + gRPC、VLESS + WebSocket、VLESS + XHTTP、VMess、Trojan 等入站的服务器；可识别 NGINX 反代后连接 Xray 的 Unix Socket 监听路径。

> **v0.11.2-dev 开发预览版。** 已完成多协议配置识别与关联、终端界面、进程 CPU/RSS、按入站/出站标签统计、协议和传输汇总。暂未实现原生 gRPC 客户端或每条 TCP 会话的精细流量归属。**无需修改现有代理入站协议**。

## 工作原理

- **代理协议**：`inbounds[].protocol` / `outbounds[].protocol`，例如 VLESS、Trojan、VMess；
- **传输方式**：读取新版 `streamSettings.method` 或旧版 `streamSettings.network`，例如 WebSocket、gRPC、XHTTP、RAW、HTTPUpgrade、mKCP；同时解析 `streamSettings.security`（TLS/REALITY）；
- **监听端点**：读取 `listen` 和 `port`，包括 `unix:/dev/shm/....socket`，仅用于展示配置，**不证明端口正在监听**；
- **实时流量**：通过本机 Xray 的 StatsService，按**精确的 Inbound/Outbound Tag** 匹配配置元数据。每 2 秒采样并计算速率。统计 API 使用 gRPC 只是管理接口协议，**不要求实际代理流量走 gRPC**。

匹配不到的标签仍会展示为 `Unknown`，绝不根据名称（例如 `grpc-01`）臆测协议。配置存在但没有统计计数器时显示 `-`，不当作 0 B。聚合总流量只计算入站统计，避免入站与出站重复计算。协议汇总和传输汇总是同一流量的两种分类，不能彼此相加。


## 一键安装与升级（推荐）

**适用：** Linux + systemd（Debian/Ubuntu 等）、AMD64/ARM64，机器上已经运行 Xray 和 NGINX。需要 root/sudo、curl 和 sha256sum。安装器**不修改、不重启 Xray 或 NGINX**，也不会自动启用 Xray StatsService。

推荐从固定版本 Release 下载脚本，查看内容后安装：

```bash
curl -fL --retry 3 -o install-xraytop.sh \
  https://github.com/Accelerator6666/xraytop/releases/download/v0.11.2-dev/install.sh
less install-xraytop.sh
sudo bash install-xraytop.sh
```

如果已审核并信任脚本，也可使用真正的单行命令（直接执行网络脚本不如上述做法安全）：

```bash
curl -fsSL https://github.com/Accelerator6666/xraytop/releases/download/v0.11.2-dev/install.sh | sudo bash
```

脚本会完成整个流程：

1. 检查系统架构、Xray 二进制与配置路径；从**同一个版本**下载二进制、systemd 单元和 `SHA256SUMS`，验证两份文件 SHA256 及程序版本，校验失败时不覆盖旧版本。
2. 备份旧程序、service 文件至 `/var/lib/xraytop/backups/`；如服务已经运行，只暂停/重启 `xraytop-collector`，**不触碰代理进程**。
3. 创建权限 `0700` 的 `/var/lib/xraytop`；不覆盖现有 `ip-history.json`，仅在没有历史时导入旧的 `/root/.local/state/xraytop/ip-history.json`。
4. 安装 `/usr/local/bin/xraytop`、配置后台服务及私有目录；根据 NGINX access.log 所属组设置只读组权限（通常 `adm`），保持 `ProtectHome=true`、`ProtectSystem=strict` 和空的 capabilities。
5. 运行 `systemctl enable` 启用开机启动、启动采集并检查新鲜快照；失败时尝试回滚程序与 service 文件。

**安装、升级后的验收：**

```bash
xraytop --version
systemctl is-enabled xraytop-collector
systemctl status xraytop-collector --no-pager
journalctl -u xraytop-collector -n 30 --no-pager
xraytop --data-source daemon --once --once-view 9

jq '{
  nginx_ports: ((.snapshot.Nginx.Ports // []) | length),
  nginx_routes: ((.snapshot.Nginx.Routes // []) | length),
  nginx_error: .snapshot.NginxError,
  tcp_connections: .snapshot.Live.Connections,
  tcp_error: .snapshot.LiveError,
  nginx_http_matches: .snapshot.Access.Matched,
  access_error: .snapshot.AccessError,
  xray_accepted: .snapshot.Recent.Events
}' /var/lib/xraytop/snapshot.json
```

你这台 VPS 已验证 NGINX 为 **15 个入口端口、30 条代理路由**；其他机器可能不同。HTTP 请求数为 0 时请先检查 `access_error`。TCP 连接数及 Xray accepted 事件数都不是在线用户数。

**升级及自定义配置：** 以后再次执行同一脚本即可升级固定版本；选择其他已发布标签时设置 `XRAYTOP_VERSION`。对于不同 Xray 安装路径：

```bash
sudo env XRAYTOP_VERSION=v0.11.2-dev \
  XRAYTOP_CONFIG=/usr/local/etc/xray/config.json \
  XRAYTOP_XRAY_BIN=/usr/local/bin/xray \
  bash install-xraytop.sh
```

还支持 `XRAYTOP_API`、`XRAYTOP_XRAY_UNIT`、`XRAYTOP_NGINX_CONFIG`、`XRAYTOP_ACCESS_LOG`，使用 `bash install-xraytop.sh --help` 查看。自定义路径必须是无空格的绝对路径。

**旧版权限清理：** 若此前使用过 `10-cert-access.conf` / `20-nginx-log.conf` 解决 `nginx -T` 沙箱问题，安装器会提醒但不会擅自删除管理员覆盖文件；待新版本采集正常后再手动清理，保留日志组读取权限。要停止采集，运行 `systemctl disable --now xraytop-collector`；卸载前请先备份 `/var/lib/xraytop/ip-history.json`。

启动交互界面只需输入 `xraytop`，按 `6` 看来源 IP、`8` 看 HTTP、`9` 看历史；按 `q` 退出 TUI 后 systemd **仍继续采集**。

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
