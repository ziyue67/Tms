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

Run tests and build through Docker when Go is not installed locally:

```sh
docker build --target build -t tms-go-backend-build .
docker build -t tms-go-backend .
```
