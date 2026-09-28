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

## Remaining migration order

1. Registration, password reset, email delivery, Redis verification codes, and captcha.
2. User administration and subscription plans, redemption, quota, and payment callbacks.
3. Nodes, tunnels, forwards, speed limits, landing rules, and inbound protocols.
4. Node WebSocket authentication, AES payload compatibility, request correlation, and live status.
5. Flow ingestion, scheduled maintenance, schema migrations, subscription output, and version checks.
6. Full API contract comparison, production Compose switch, and removal of the Java source tree.

The local hybrid Compose file runs the Go service as `go-backend`. Production
image publishing remains on the existing backend until the compatibility suite
passes, so an incomplete migration cannot replace the currently deployed image.
