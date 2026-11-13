# Magicap

Android Root 抓包模块。提供独立 CA、HTTP/HTTPS 解密代理、WebUI、CLI 与 Streamable HTTP MCP。

**v0.1.0 是可运行的初始版本，尚未达到 HttpCanary（小黄鸟）的完整功能覆盖。** 当前采用显式 HTTP 代理；不是 VPN 抓包应用，不自动接管全部应用流量。功能和设备验证范围见下表。

## 功能

| 能力 | 当前实现 |
| --- | --- |
| 抓包证书 | 首次安装在设备生成独立 RSA-3072 CA，私钥权限 0600，不打包共享私钥 |
| 系统 CA | 安装时生成 `system/etc/security/cacerts/<subject_hash_old>.0`，通过模块挂载 |
| HTTP / HTTPS | 显式 HTTP 代理、CONNECT TLS 解密，正常验证上游服务器证书 |
| 会话观察 | URL 筛选、请求/响应头、二进制安全的正文、状态码、耗时 |
| 规则 | URL 子串匹配；阻断、修改请求头/响应头、模拟响应；保存到设备 |
| 请求工具 | 自定义方法、请求头、请求体；从会话载入重放 |
| 导出 | HAR 1.2；响应正文 base64，包含截断标记 |
| PCAP | `cli pcap` 调用设备已有的 tcpdump，支持网卡、BPF 过滤与时长；不内置 tcpdump |
| MCP | 官方 Go SDK，默认关闭，支持自定义端口和可选 Bearer 密钥 |
| 限额 | 最多 500 条会话、32 MiB 捕获正文总量、单方向正文 1 MiB；会话仅在内存中 |
| ABI | Android arm64-v8a、x86_64，最低 API 26 |

不支持：VPN/TUN 全应用抓取、按应用 UID 归因、透明 TLS 转发、证书固定绕过、QUIC/HTTP3 解密、WebSocket 帧分析、实时断点编辑、原始 TCP/UDP 会话解析。长连接和流式响应未专项验证。HTTP/2 能力取决于代理库，本项目尚未进行 HTTP/2 专项验收。

Magisk、KernelSU、APatch 使用模块生命周期；**KernelSU 的系统证书覆盖需要安装挂载元模块**，本模块自身不是元模块。[官方说明](https://kernelsu.org/guide/module.html)。Android 14+ 使用 Conscrypt APEX 的设备可能不会读取 system CA 覆盖，本版不做 APEX/zygote 挂载注入，不承诺系统信任自动生效；可向测试客户端显式导入公钥证书。模块安装、重启后系统证书识别和应用兼容性尚未验收。

## 安装与使用

从 [Releases](https://github.com/LIghtJUNction/Magicap/releases) 下载 `magicap.zip`，通过 Root 管理器安装后重启。

模块根目录：`/data/adb/modules/magicap`。固定入口 `cli -> bin/magicap-cli`，根据 ABI 调用原生 Go 程序。

首次安装时抓包和 MCP 均关闭，无 secret key，所有监听仅绑定 `127.0.0.1`。

```sh
su
cd /data/adb/modules/magicap
./cli capture enable --port 8080
./cli status
./cli mcp enable --port 8765
./cli mcp status --json
./cli mcp disable
./cli capture disable
```

MCP 状态字段固定为 `enabled`、`running`、`pid`、`host`、`port`、`endpoint`、`transport`，端点 `http://127.0.0.1:8765/mcp`，传输为 `streamable-http`。

设备代理端口默认 8080，WebUI 默认 8081，MCP 默认 8765。端口范围 1024–65535，三个端口不得重复。启用状态、端口和规则跨启动保存；修改运行配置会重启进程并清空内存会话。`start` 仅启动配置中的服务，`stop` 停止进程但保留启用意图，`boot` 仅在抓包或 MCP 已启用时启动。

```sh
# 在电脑运行，转发到设备本机地址
adb forward tcp:8080 tcp:8080
adb forward tcp:8081 tcp:8081
adb forward tcp:8765 tcp:8765
```

电脑访问 `http://127.0.0.1:8081`。浏览器或调试客户端的 HTTP/HTTPS 代理设为 `127.0.0.1:8080`，并信任此设备的 CA。设备上的应用必须自行支持代理并配置连接；不会自动修改系统代理。模块包含 `webroot/`，管理器本地页面提供进入观察台的链接；完整会话界面由本机 HTTP 服务提供。

```sh
./cli config --web-port 8081 --proxy-port 8080 --mcp-port 8765
./cli config --secret 'optional-key'
./cli config --secret ''   # 恢复无密钥
./cli cert export         # 仅输出 CA 公钥 PEM，不输出私钥
./cli sessions example.com
./cli har > /sdcard/Download/magicap.har
./cli replay '{"method":"GET","url":"https://example.com"}'
./cli rules '[{"match":"example.com/mock","mock_status":200,"mock_body":"hello"},{"match":"example.com/blocked","block":true}]'
./cli pcap --interface any --seconds 60 --filter 'tcp port 443'
```

配置密钥后，WebUI 在“连接指南”填写密钥；REST API 和 MCP 使用 `Authorization: Bearer <key>`。密钥仅在页面内存中保存。无密钥为用户指定默认值，本机其他应用能否连接取决于设备访问控制。拒绝跨来源 Origin 和非本机 Host。

PCAP 需要 root 和 `PATH` 中可执行的 tcpdump，最多 3600 秒或 100000 包，输出路径由 CLI 返回；BPF 表达式直接传参，不通过 shell。PCAP 文件不会自动删除，请自行管理占用。

## MCP 工具

`sessions_list`、`sessions_clear`、`har_export`、`request_replay`、`rules_list`、`rules_set`、`capture_status`。重放需先开启抓包。会话 JSON 的请求和响应正文字段为 base64。

```sh
kam mcp forward
kam mcp enable
kam mcp status --json
```

`kam.toml` 中 `[dev.mcp].enabled = false`，安装时不会自动启动 MCP；显式 `kam mcp enable` 或 `cli mcp enable` 才会启用。

## 数据与卸载

设备状态位于 `/data/adb/magicap/`：`config.json`、`rules.json`、`certs/ca.pem`、`certs/ca.key`、`daemon.log`、`captures/`。私钥不通过 WebUI/API/MCP 暴露。守护日志每次启动重置。卸载停止服务，管理器移除模块证书覆盖；保留状态目录以免意外丢失 CA 身份和 PCAP。彻底清除时先卸载并确认不再需要数据，再自行删除该目录。重新生成 CA 后需重装/重启并更新客户端信任。

## 本地构建

依赖 Go 1.27.4、Kam 0.6.12+、Android NDK（x86_64 构建需要，设置 `ANDROID_NDK_HOME`）。无需 Node 包管理器或 Rust。

```sh
go test -race ./...
go vet ./...
python scripts/smoke.py
kam validate
kam check --fail-on-error
kam build
unzip -t dist/magicap.zip
```

本地构建同时产出两个 Android ABI，模块 ZIP 为 `dist/magicap.zip`。日常检查在本地运行，不配置自动 CI。具体结果见 [VERIFICATION.md](VERIFICATION.md)。

代理使用 [GoProxy](https://github.com/elazarl/goproxy)，MCP 使用 [官方 Go SDK](https://github.com/modelcontextprotocol/go-sdk)，模块使用 Kam 和随模板提供的 kamfw。运行时默认值和 CA 信任边界参考 [MagicNet](https://github.com/LIghtJUNction/MagicNet)；各依赖许可证见 [THIRD_PARTY.md](THIRD_PARTY.md)。
