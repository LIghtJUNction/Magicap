# Changelog

## v0.1.2

- Make `block` optional in MCP rule input, aligning mock and header-rewrite rules with REST behavior and the existing false default.
- Add MCP regression coverage for mock, header rewrite and blocking rules.
- Complete 14 Android 16 / KernelSU acceptance checks after the user's reboot and the repair hot update, including calls to all seven MCP tools, upstream TLS rejection and service-start idempotence.

## v0.1.1

- Redesign all four WebUI pages with Chinese-only interface text, warm paper, ink and vermilion, fewer borders and more whitespace.
- Add responsive navigation, clearer flow details, loading/success feedback, and request validation.
- Fix MCP request replay schema: omitted method now consistently defaults to GET.
- Include the configured MCP port in runtime health, and derive runtime/HAR/MCP versions from kam.toml at build time.
- Verify HTTP/HTTPS, rewriting, replay, HAR, custom-port MCP, authentication and PCAP on an Android 16 KernelSU device; restore disabled state after tests.

## v0.1.0

- Per-device CA generation and system certificate overlay preparation.
- Explicit HTTP/HTTPS interception proxy, bounded sessions and HAR export.
- Persistent block, header rewrite and mock rules; request replay.
- Local WebUI and fixed module-root CLI symlink.
- Default-off, loopback-only Streamable HTTP MCP with configurable port and optional Bearer secret.
- Optional tcpdump PCAP wrapper; Android arm64 and x86_64 packages.
- Local proxy, CA, lifecycle and MCP integration verification; full feature parity and system-trust installation remain unverified.
