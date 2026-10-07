# TMS Go backend

This is the TMS backend. It keeps the existing port, environment variables,
database schema, JWT format, response envelope, and public API paths.

The service includes:

- PostgreSQL and MySQL database support with idempotent startup migrations;
- Redis-backed captcha, verification, and rate limiting;
- user, node, tunnel, forward, speed-limit, inbound, landing, and custom-node APIs;
- subscriptions, redeem codes, quota accounting, payments, and automatic protocol provisioning;
- native and Clash-compatible subscription output;
- node WebSocket commands, flow reporting, diagnostics, and scheduled maintenance;
- health checks, structured logging, panic recovery, CORS, and graceful shutdown.

## Subscription endpoints

- `/api/v1/open_api/clash?token=...` returns a full Clash/Mihomo configuration.
- `/api/v1/open_api/sub?token=...` returns a Base64-encoded list of share links.
- Both endpoints send `Cache-Control: no-store` on success and failure. CDN rules
  must also bypass caching for `/api/v1/open_api/*`.
- Missing or invalid tokens return HTTP 401; disabled, expired, or over-quota
  subscriptions return 403; subscriptions without supported nodes return 404;
  storage or encoding failures return 500. These responses are not proxy configs.
- Mobile wrappers generate subscription links from the selected panel address,
  not the local `file://` page. Web deployments with `VITE_API_BASE` use that API
  address, so it must be reachable from the subscriber's device.

For mobile acceptance, retrieve the correct format in the actual client over
both Wi-Fi and cellular, then update it again. Record HTTP reachability and
client parsing separately; Android user-agent requests and a Mihomo config
check do not replace a real-device test. If retrieval fails before receiving
HTTP (for example, connection refused), check device DNS, TLS, and the CDN/origin
path separately from subscription serialization.

Run tests and build through Docker when Go is not installed locally:

```sh
docker build --target build -t tms-go-backend-build .
docker build -t tms-go-backend .
```
