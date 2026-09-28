# Go backend migration

The backend migration is complete. The Go service owns the existing API contract,
node WebSocket protocol, database schema, subscriptions, payments, scheduled work,
and production deployment.

Compatibility retained during the rewrite:

- the `code`, `msg`, `ts`, `data` response envelope and existing API paths;
- raw `Authorization` JWTs and the legacy `HmacSHA256` header value;
- legacy MD5 password verification with automatic bcrypt upgrades;
- existing `DB_*`, JDBC `DB_URL`, `REDIS_*`, and `REDIS_URL` settings;
- MySQL 5.7 compatibility and PostgreSQL as the primary database target;
- AES-encrypted node flow/config reports and GOST command names;
- existing native and Clash/Mihomo subscription URLs.

Production uses `ghcr.io/ziyue67/go-backend:latest` with container name
`go-backend`. The module targets Go 1.25.

Validation performed for the completed migration:

- unit, API contract, race, and vet checks;
- static Linux binary and Go 1.25 Docker image builds;
- PostgreSQL 16 schema initialization and idempotent startup migration;
- live PostgreSQL and Redis readiness checks;
- frontend TypeScript and production Vite build;
- all production Compose files and installer shell syntax.
