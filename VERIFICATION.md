# Verification — v0.2.0

## Android 16 / KernelSU transparent capture

Development binaries were hot-updated on the existing boot. Platform TrustManager reported 152 anchors including Magicap, compared with 151 after `cert untrust`; enabling trust restored 152. Existing platform anchors were retained. Selected app namespaces initially omitted the CA under root-manager mount isolation; a UID-scoped watcher repairs these namespaces while capture runs.

Taobao 10.69.0: the initial decrypted sample contained 93 sessions, including 61 HTTPS requests. Standard TLS traffic included `acs.m.taobao.com` and homepage resources. A final cold launch captured 61 sessions including 49 HTTPS requests, with automatic namespace repair and no manual per-process injection. Repeated app enable preserved the daemon PID and sessions; stopping removed the owned NAT chain and restarting restored capture. Only selected UIDs receive OUTPUT redirects; system proxy settings remain unchanged.

WeChat 8.0.79: baseline UID-scoped outgoing capture recorded 768 packets and 73 `/mmtls/` requests. During interception, 67 sessions were observed: 15 HTTPS requests (13 successful responses: 12 avatar resources and one map request; two canceled requests), plus 52 HTTP `/mmtls/` requests whose bodies remain encrypted binary frames. This verifies ordinary HTTPS interception, not message plaintext recovery. Temporary WeChat routes were removed and WeChat relaunched normally; Taobao capture was restored. Raw traffic remains in ignored private local storage.

Local tests cover real transparent HTTP, TLS HTTP/1.1 and negotiated HTTP/2 with trusted upstream certificates. Existing proxy/MCP tests and idempotent capture enable regression remain included. The release uses device hot update; v0.2.0 full reboot acceptance is pending.

## Historical v0.1.2 verification

## Post-reboot acceptance and v0.1.2 repair

The user rebooted the Android 16 / KernelSU device with v0.1.1 installed. A changed kernel boot identifier and `sys.boot_completed=1` confirmed the new boot. The module metadata, CLI symlink and file hashes matched the local build; capture and MCP remained disabled. The system certificate overlay matched the device CA, and the CA private key retained mode 0600.

Expanded testing found that MCP `rules_set` incorrectly required `block` for mock and header-rewrite rules. REST accepted the same rules. Marked `block` optional in the shared rule type, preserving its false default, and added local MCP regression cases for mock, rewrite and block rules. v0.1.2 was built locally and hot-updated while services were stopped; this repair used the existing device boot and CA identity.

All 14 final device checks passed:

1. Post-reboot default state.
2. Installed native binary and WebUI file hashes.
3. System CA overlay and private-key permissions.
4. Real HTTP forwarding and request-body preservation.
5. Persistent rules, request/response header rewriting, blocking and mocking.
6. Default GET and explicit POST replay.
7. Full forwarding of a large response and the 1 MiB capture limit.
8. Real external HTTPS decryption and plaintext capture with explicit client CA trust.
9. Rejection of a temporary self-signed upstream certificate, confirmed by the x509 diagnostic. The proxy closes this rejected HTTPS connection.
10. Redesigned WebUI, CA download and runtime version.
11. HAR content and version metadata.
12. Custom-port MCP, calls to all seven tools, optional rule fields and idempotent service startup.
13. Bearer authentication and Origin/Host validation.
14. Actual tcpdump capture producing a nonempty PCAP.

An initial transport interruption removed adb port mappings while the device boot identifier stayed unchanged. Mappings were restored for the final run. After acceptance, original config/rules, disabled service state and CA identity were verified, and test port mappings and PCAP were cleaned up.

## Earlier v0.1.1 Android 16 / KernelSU checks

The previously installed v0.1.0 module completed a real reboot and was visible at `/data/adb/modules/magicap`. For v0.1.1, WebUI/scripts were synchronized through `kam dev --sync-only --hot`; built binaries and module metadata were updated in place while the daemon was stopped. Device and local arm64 binary SHA-256 matched. No additional reboot was performed for the hot update.

Eleven installed-module acceptance checks passed:

1. CLI enable is idempotent and reports the active capture state.
2. Real HTTP forwarding to a controlled host server preserves the request body.
3. Request and response header rewriting, blocking and mocking work.
4. A real HTTP request can be replayed through the proxy.
5. A response larger than 1 MiB reaches the client intact, while the captured body is bounded and marked truncated.
6. A real request to `https://example.com` succeeds using the device CA as the client's trust root; captured response contains the expected plaintext.
7. The device serves WebUI HTML and the public CA certificate.
8. HAR 1.2 export includes the captured sessions.
9. MCP starts idempotently on custom port 28765, initializes, lists seven tools and successfully calls request replay without a method parameter.
10. Optional Bearer authentication rejects a missing key and accepts the configured key.
11. The real device tcpdump produces a nonempty PCAP containing test traffic.

The first v0.1.0 run exposed an MCP schema inconsistency: method was mandatory in the generated schema despite the implementation defaulting to GET. Fixed via `omitempty`, covered by local MCP integration and retested successfully on the device.

Tests restored original config/rules and the disabled capture/MCP state, removed test forwards/reverse mappings and test PCAP, and confirmed that the public CA identity had not changed. No system proxy or APEX trust-store settings were modified. The system CA overlay matches the generated CA; the APEX store does not contain it. These tests do not establish arbitrary-app trust or full HttpCanary parity.

## WebUI and local checks

Real headless Chromium was driven through its debugging protocol at desktop 1440×1100 and mobile 393×920. Checks covered connection, five fixture sessions, details, all four tabs, rule edit/save, replay, URL filtering and clear. No JavaScript exceptions or horizontal overflow on the four mobile pages. Desktop and mobile screenshots were visually inspected; preview images in docs use synthetic test sessions.

`go test -race ./...`, `go vet ./...`, `python scripts/smoke.py`, JavaScript syntax, Kam validation/check, release build, ZIP integrity and payload checks passed. The existing vendored kamfw still emits style/lint warnings. Runtime version information is injected from kam.toml by the build hook.

## Earlier initial-build record

### v0.1.0 local packaging record

Validation performed locally on 2026-11-13. No automatic CI was configured or triggered.

| Check | Result |
| --- | --- |
| `go test -race ./...` | Passed: verified TLS MITM against a trusted test upstream, request/response header rewrites, block/mock, CA stability, incomplete CA rejection, body preservation/truncation and session limits |
| `go vet ./...` | Passed |
| `python scripts/smoke.py` | Passed: real daemon startup, idempotent enable, custom ports, mock/replay, HAR, static WebUI, Origin rejection, optional authentication, MCP initialize/tools list/call, MCP rules mutation, shutdown |
| OpenSSL CA compatibility | Passed: generated overlay filename exactly equals `openssl x509 -subject_hash_old -noout` plus `.0` |
| `node --check src/magicap/webroot/app.js` | Passed |
| ShellCheck, project runtime scripts | Passed at error severity |
| `kam validate` | Passed |
| `kam check --fail-on-error` | Passed; vendored template framework produces style/lint warnings, not a warning-free result |
| `kam build --release` | Passed: Android arm64 CGO-free PIE and x86_64 NDK-linked binaries |
| ZIP integrity and root layout | Passed; module.prop at root, both ABIs, CLI launcher, lifecycle scripts, webroot and kamfw environment included; no CA private key or runtime state |
| Android device runtime | Not completed: an Android 16 arm64 device was initially visible but disconnected before the smoke test. Test did not create remote files. Optional `scripts/device-smoke.py` is provided but remains unverified |
| Module installation / reboot / Android system CA trust | Not tested; Android 14+ APEX trust-store injection is not implemented |
| PCAP, HTTP/2, streaming, WebSocket | PCAP wrapper requires external tcpdump and has not been device-tested. HTTP/2/streaming not specifically verified; WebSocket frame decoding not implemented |
| WebUI visual/device rendering | JavaScript and served HTML checked; no visual or manager rendering acceptance |

Go 1.27.4 and the latest stable direct dependency versions reported by the Go module proxy were used: GoProxy v1.9.4 and MCP Go SDK v1.8.0. Transitive dependencies were updated with `GOPROXY=https://proxy.golang.org go get -u ./...`; exact versions are in go.mod/go.sum. Initial module version queries through the host's default Go proxy hung/failed; the public module proxy resolved the update.

The package is unsigned. Building a ZIP does not establish full Magisk/KernelSU/APatch lifecycle or device trust compatibility. This is an initial explicit-proxy release, not feature parity with HttpCanary.
