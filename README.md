# Magicap

Magicap 是面向 Android Root 设备的网络抓包模块，将设备独立证书、HTTP/HTTPS 解密代理、会话观察、请求重放与 MCP 自动化接口整合到同一套工具中。

当前版本为 **v0.2.1**，提供显式代理与按应用 UID 的透明抓包。模块将本机 CA 加入平台信任库，并在抓包期间刷新所选应用的证书挂载。Android 16 / KernelSU 已完成淘宝与微信实机验证，具体范围见 [验证记录](VERIFICATION.md)。

[下载模块](https://github.com/LIghtJUNction/Magicap/releases) · [桌面预览](docs/preview-desktop.png) · [手机预览](docs/preview-mobile.png)

## 功能

| 能力 | 实现方式 |
| --- | --- |
| 设备独立证书 | 首次安装时生成 RSA-3072 CA，私钥以 `0600` 权限保存在设备中 |
| 系统证书挂载 | 生成 `system/etc/security/cacerts/<subject_hash_old>.0`，提供系统目录覆盖，并合并 Conscrypt APEX 平台证书 |
| HTTP/HTTPS 解密 | 支持显式代理、CONNECT 和按应用透明 TLS，校验上游证书 |
| 会话观察 | 按 URL 筛选，查看请求头、响应头、正文、状态码与耗时 |
| 请求规则 | 按 URL 子串匹配，支持阻断、修改请求头和响应头、模拟响应；规则持久化保存 |
| 构造与重放 | 自定义请求方法、地址、请求头和正文，也可从已有会话载入请求 |
| HAR 导出 | 输出 HAR 1.2，响应正文采用 base64 编码，并记录正文截断状态 |
| PCAP 抓取 | 调用设备上的 tcpdump，支持网卡选择、BPF 过滤和抓取时长设置 |
| MCP 自动化 | 使用官方 Go SDK 提供 Streamable HTTP 接口，支持自定义端口与可选 Bearer 密钥 |

会话保存在进程内存中，容量为 **500 条记录、32 MiB 捕获正文总量、单方向正文 1 MiB**。达到容量后按先后顺序淘汰记录；服务重启时清空会话。

模块提供 **arm64-v8a、x86_64** 两种 Android 架构的程序，最低系统版本为 **API 26**。

## 界面

WebUI 以暖白纸面、墨色文字和朱砂点缀呈现中文单语界面，通过留白、字级和少量分隔线组织信息。桌面使用侧栏导航，手机使用横向导航；字体取自设备系统，页面资源随模块提供。

- **会话**：浏览连接记录，展开请求与响应细节，筛选并导出数据。
- **规则**：编辑、读取和保存规则，调整请求经过代理时的处理方式。
- **重放**：构造请求或载入已有会话，查看新的响应结果。
- **连接**：查看启动步骤、代理地址、证书下载入口与密钥设置。

预览图片使用测试会话拍摄：[桌面界面](docs/preview-desktop.png) · [手机界面](docs/preview-mobile.png)。

## 安装与连接

从 [Releases](https://github.com/LIghtJUNction/Magicap/releases) 下载 `magicap.zip`，通过 Magisk、KernelSU 或 APatch 管理器安装，然后重启设备。

KernelSU 的系统证书覆盖需要挂载元模块配合，具体机制见 [KernelSU 模块指南](https://kernelsu.org/guide/module.html)。Magicap 作为普通模块安装。

模块目录为 `/data/adb/modules/magicap`。统一命令入口 `cli -> bin/magicap-cli` 根据设备架构调用对应的 Go 程序。

### 启动服务

首次安装时，抓包与 MCP 均处于关闭状态，密钥配置默认为空。服务绑定设备本机地址 `127.0.0.1`，由用户按需开启。

```sh
su
cd /data/adb/modules/magicap

# 开启抓包并查看状态
./cli capture enable --port 8080
./cli status

# 开启 MCP 并读取机器可解析的状态
./cli mcp enable --port 8765
./cli mcp status --json
```

| 服务 | 默认地址 |
| --- | --- |
| HTTP/HTTPS 代理 | `127.0.0.1:8080` |
| WebUI | `http://127.0.0.1:8081` |
| MCP | `http://127.0.0.1:8765/mcp` |
| 应用透明 TLS | `127.0.0.1:8443`，按应用启用时监听 |

四个监听入口分别使用独立端口，可选范围为 `1024–65535`。启用状态、端口和规则跨启动保存；调整运行配置时，进程重启，会话重新开始记录。

```sh
# 调整端口
./cli config --web-port 8081 --proxy-port 8080 --mcp-port 8765

# 按需关闭服务
./cli mcp disable
./cli capture disable
```

`start` 按当前配置启动进程，`stop` 停止进程并保留启用设置，`boot` 根据抓包或 MCP 的启用设置执行启动。

### 连接电脑或测试客户端

在电脑上执行端口转发：

```sh
adb forward tcp:8080 tcp:8080
adb forward tcp:8081 tcp:8081
adb forward tcp:8765 tcp:8765
```

访问 `http://127.0.0.1:8081` 打开观察台，将浏览器或调试客户端的 HTTP/HTTPS 代理设为 `127.0.0.1:8080`，并导入本机 CA。设备端应用同样通过自身的代理设置接入。

模块的 `webroot/` 提供管理器本地页面与观察台入口，完整会话界面由本机 HTTP 服务呈现。系统代理设置由用户或客户端管理。

### 证书与信任

每台设备在安装时生成自己的 CA，证书公钥可通过 WebUI 下载或通过 CLI 导出：

```sh
# 输出 CA 公钥证书，格式为 PEM
./cli cert export
```

Android 14 及之后版本使用 Conscrypt APEX 证书挂载；挂载保留现有平台根证书，并加入本机 CA。抓包进程会检查所选 UID 的新进程，为独立挂载空间补充证书。平台证书挂载支持手动启用和回退：

```sh
./cli cert trust
./cli cert untrust
```

### 按应用透明抓包

```sh
./cli app enable com.taobao.taobao
./cli app status
./cli app disable com.taobao.taobao
./cli config --transparent-port 8443
```

启用后，应用 UID 的 TCP 80/443 重定向至本机代理，UDP 443 经拒绝触发客户端 TCP 回退。IPv6 NAT 可用时同样重定向；其余设备对所选 UID 的 IPv6 TCP 80/443 连接进行拒绝，由客户端尝试 IPv4。规则集中在 `MAGICAP_OUT` 和 `MAGICAP_QUIC` 链，停用应用或停止服务时清理。

透明 TLS 使用 SNI 确定主机，支持 HTTP/1.1 和 HTTP/2。标准平台证书校验适用本机 CA；证书固定、自带信任库、自定义协议、其他端口和业务层加密需要各自适配。微信实测可读取头像等标准 HTTPS，请求 `/mmtls/…` 的正文保持协议加密形式。淘宝实测已读取首页相关 HTTPS 请求与响应。

首次安装时证书挂载启用，抓包与 MCP 保持关闭。`cert untrust` 同时暂停已配置的应用抓包，`app enable` 重新建立信任与抓包设置。设备实测使用热更新，完整重启验收记录随各版本单独列出。

## CLI 常用操作

```sh
# 筛选会话
./cli sessions example.com

# 导出 HAR
./cli har > /sdcard/Download/magicap.har

# 重放请求
./cli replay '{"method":"GET","url":"https://example.com"}'

# 配置模拟与阻断规则
./cli rules '[{"match":"example.com/mock","mock_status":200,"mock_body":"hello"},{"match":"example.com/blocked","block":true}]'

# 抓取 PCAP
./cli pcap --interface any --seconds 60 --filter 'tcp port 443'
```

PCAP 抓取需要 Root 权限，以及 `PATH` 中可执行的 tcpdump。单次上限为 3600 秒或 100000 个数据包，输出路径由 CLI 返回。BPF 表达式作为命令参数直接传递；生成的 PCAP 由用户按需归档和清理。

### 可选密钥

```sh
# 设置 Bearer 密钥
./cli config --secret 'optional-key'

# 将密钥配置恢复为空
./cli config --secret ''
```

配置密钥后，在 WebUI 的“连接”页填写相同密钥；REST API 与 MCP 使用 `Authorization: Bearer <key>`。页面中的密钥保存在当前页面内存中。

服务校验本机 Host 与同源 Origin。本机其他应用的连接权限由设备访问控制策略决定。

## MCP 自动化

MCP 使用 `streamable-http` 传输，默认端点为 `http://127.0.0.1:8765/mcp`。`cli mcp status --json` 提供固定字段：`enabled`、`running`、`pid`、`host`、`port`、`endpoint`、`transport`。

| 工具 | 用途 |
| --- | --- |
| `sessions_list` | 查询捕获的会话 |
| `sessions_clear` | 清空内存会话 |
| `har_export` | 导出 HAR |
| `request_replay` | 通过已开启的抓包代理重放请求，方法默认为 GET |
| `rules_list` | 读取规则 |
| `rules_set` | 保存并应用规则 |
| `capture_status` | 查询抓包与 MCP 运行配置 |

会话 JSON 的请求正文和响应正文字段采用 base64 编码。

Kam 提供统一的转发与启用入口：

```sh
kam mcp forward
kam mcp enable
kam mcp status --json
```

`kam.toml` 中 `[dev.mcp].enabled = false`，MCP 由 `kam mcp enable` 或 `cli mcp enable` 显式开启。

## 数据与卸载

设备状态集中保存在 `/data/adb/magicap/`：

| 路径 | 内容 |
| --- | --- |
| `config.json` | 服务启用状态、端口与密钥配置 |
| `rules.json` | 持久化规则 |
| `certs/ca.pem` | CA 公钥证书 |
| `certs/ca.key` | 设备本地 CA 私钥，权限为 `0600` |
| `daemon.log` | 当前启动周期的守护进程日志 |
| `captures/` | PCAP 抓取文件 |

证书下载与导出接口提供 CA 公钥，私钥保存在设备状态目录。守护日志在每次启动时重置。

卸载时停止服务，由管理器移除模块证书覆盖；状态目录继续保留，便于保存 CA 身份和抓取文件。彻底清理时，先归档所需材料，再删除状态目录。重新生成 CA 后，重新安装或重启模块，并更新客户端的信任配置。

## 适用范围

当前交付能力集中在显式 HTTP/HTTPS 代理、会话观察、规则处理、重放、HAR 导出与 tcpdump PCAP 抓取。

VPN/TUN 全应用抓取、逐条会话 UID 标记、证书固定适配、QUIC/HTTP3 解密、WebSocket 帧分析、实时断点编辑和原始 TCP/UDP 会话解析属于独立扩展项。透明代理 HTTP/2 已通过本地专项测试；长连接与流式响应的兼容性以对应专项验收为依据。当前版本的功能与验证范围见上文及 [验证记录](VERIFICATION.md)。

## 本地构建

构建环境采用 Go 1.27.4、Kam 0.6.12+ 和 Android NDK。x86_64 构建通过 `ANDROID_NDK_HOME` 指定 NDK 路径；WebUI 使用随模块提供的静态页面资源。

```sh
go test -race ./...
go vet ./...
python scripts/smoke.py
kam validate
kam check --fail-on-error
kam build
unzip -t dist/magicap.zip
```

构建同时生成两个 Android 架构的程序，模块 ZIP 位于 `dist/magicap.zip`。日常验证在本地完成，结果记录于 [VERIFICATION.md](VERIFICATION.md)。

## 项目依赖

代理基于 [GoProxy](https://github.com/elazarl/goproxy)，MCP 基于 [官方 Go SDK](https://github.com/modelcontextprotocol/go-sdk)，模块通过 Kam 与模板自带的 kamfw 构建和运行。默认运行方式与证书信任边界参考 [MagicNet](https://github.com/LIghtJUNction/MagicNet)。依赖版本及许可证见 [THIRD_PARTY.md](THIRD_PARTY.md)。
