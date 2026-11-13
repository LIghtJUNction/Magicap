# Verification — v0.1.0

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
