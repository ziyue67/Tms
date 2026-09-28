# TMS Go backend

This directory contains the incremental Go replacement for `springboot-backend`.
The replacement keeps the existing port, environment variables, database schema,
JWT format, response envelope, and public API paths.

Implemented in the first migration slice:

- dependency configuration for MySQL, PostgreSQL, and Redis;
- liveness and readiness endpoints;
- legacy-compatible JWT signing and validation;
- account login, including MD5-to-bcrypt password upgrades;
- public site configuration reads;
- authenticated administrator configuration reads and updates;
- graceful shutdown, panic recovery, CORS, and request logging.

Run tests and build through Docker when Go is not installed locally:

```sh
docker build --target build -t tms-go-backend-build .
docker build -t tms-go-backend .
```

The remaining Java service stays in the repository only as a behavior reference
until API parity is complete. It is not included in the Go image.
