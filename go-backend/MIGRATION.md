# Go backend migration

The Go service is being migrated by API contract so the frontend, node agents,
database, and deployment tooling can continue to use the same interfaces.

## Implemented

- process lifecycle, structured logging, panic recovery, CORS, and graceful shutdown;
- MySQL 5.7 and PostgreSQL connections through legacy `DB_*` and JDBC `DB_URL` settings;
- Redis connection through `REDIS_*` or `REDIS_URL`;
- `GET|HEAD /flow/test`, `GET /health/live`, and `GET /health/ready`;
- the legacy `code`, `msg`, `ts`, `data` response envelope;
- raw `Authorization` JWTs with the legacy `HmacSHA256` header value and 90-day expiry;
- `POST /api/v1/user/login` and `POST /api/v1/auth/login`;
- legacy MD5 and bcrypt password verification with automatic bcrypt upgrades;
- `GET /api/v1/auth/config`;
- public and administrator site configuration APIs under `/api/v1/config`;
- administrator role enforcement from JWT `role_id`.
- registration, password reset, Redis verification codes, SMTP delivery, rate limits, and administrator email diagnostics;
- node CRUD, install command generation, node status transitions, live system info, and AES-GCM WebSocket command correlation;
- user administration, account password changes, user deletion cleanup, traffic reset, and package dashboard queries;
- subscription plans, user activation, redeem-code generation/consumption, quota audits, dashboard data, and admin lifecycle APIs.

## Remaining migration order

1. Captcha image generation/track validation.
2. Tunnels, forwards, speed limits, landing rules, and inbound protocol CRUD.
3. Payment providers, signed callbacks, flow ingestion, scheduled access maintenance, and schema migration startup.
4. Subscription text/Clash output, version checks, and remaining OpenAPI endpoints.
5. Full API contract comparison, production Compose switch, and removal of the Java source tree.

The local hybrid Compose file runs the Go service as `go-backend`. Production
image publishing remains on the existing backend until the compatibility suite
passes, so an incomplete migration cannot replace the currently deployed image.
