# TMS

TMS is a central management panel for GOST forwarding nodes and user subscriptions. It provides node and protocol management, relay forwarding, traffic quotas, expiry enforcement, plans, redemption codes, account security, and an administrator dashboard.

The upstream project is Apache-2.0. Fork-specific account, subscription, redemption, and commerce additions are available under `LICENSE-MIT`; upstream notices remain in `LICENSE`.

## Features

- Central management for GOST nodes, forwards, relays, speed limits, and protocol inbounds.
- Aggregate user subscriptions for V2Ray-compatible and Clash/Mihomo clients.
- Subscription plans with quota, validity, reset date, sale status, and redemption controls.
- Expiry and quota enforcement for subscription access.
- Bundled MySQL/Redis or external MySQL, PostgreSQL, and Redis.
- Website, SMTP, account-security, and payment-provider configuration.

## Requirements

- Linux with Docker Engine and Docker Compose v2.
- Public IP or domain for the panel and node ports.
- For external services: PostgreSQL 13+ or MySQL 5.7+, plus Redis 6+ reachable from the panel host.

Keep Redis and database ports private, firewalled, or allowlisted.

## Quick Install

Run this on the panel server:

```bash
curl -fsSL https://raw.githubusercontent.com/ziyue67/Tms/main/panel_install.sh -o panel_install.sh
chmod +x panel_install.sh
./panel_install.sh
```

The installer prints the panel address and creates the `tms` command. The initial account is `admin_user` / `admin_user`; change its password immediately.

## External PostgreSQL and Redis

When `DB_URL` or `REDIS_URL` is set, the installer generates a Compose override that disables the corresponding bundled service. It will not pull or start `gost-mysql` or `tms-redis` for externally configured services.

Create a PostgreSQL schema once:

```bash
git clone https://github.com/ziyue67/Tms.git
cd Tms
psql 'postgresql://USER:PASSWORD@DB_HOST:5432/gost' -f springboot-backend/src/main/resources/db/tms-postgres.sql
```

Install with external services:

```bash
export DB_URL='jdbc:postgresql://DB_HOST:5432/gost'
export DB_DRIVER='org.postgresql.Driver'
export DB_USER='gost'
export DB_PASSWORD='replace-with-a-strong-password'
export REDIS_URL='redis://:replace-with-a-strong-password@REDIS_HOST:6379/0'
export JWT_SECRET='replace-with-a-random-32-plus-character-secret'
curl -fsSL https://raw.githubusercontent.com/ziyue67/Tms/main/panel_install.sh -o panel_install.sh
chmod +x panel_install.sh
./panel_install.sh
```

For a manual Compose deployment:

```bash
docker compose -f docker-compose-v4.yml -f docker-compose-external-database.yml -f docker-compose-external-redis.yml --env-file .env up -d
```

The legacy table is named `user`, which is special in PostgreSQL. TMS uses a writable `tms_user` compatibility view while retaining the legacy table and data. New PostgreSQL databases receive the view from `tms-postgres.sql`; existing databases receive it through the backend startup migration. Do not rename the legacy table manually.

## External MySQL

For a new external MySQL database, import `gost.sql` and configure:

```bash
export DB_URL='jdbc:mysql://DB_HOST:3306/gost?useUnicode=true&useSSL=false&characterEncoding=utf8&serverTimezone=Asia/Shanghai'
export DB_DRIVER='com.mysql.cj.jdbc.Driver'
export DB_USER='gost'
export DB_PASSWORD='replace-with-a-strong-password'
```

The backend migration is idempotent and creates missing account, subscription, redemption, inbound, and compatibility objects during startup. Back up a production database before upgrading.

## Redis Check

Redis stores short-lived registration and password-reset credentials. Verify an external instance with:

```bash
REDISCLI_AUTH='your-password' redis-cli -h REDIS_HOST -p 6379 -n 0 ping
```

It must return `PONG`. If it does not, check `REDIS_URL`; when using separate variables, ensure `REDIS_HOST`, `REDIS_PORT`, `REDIS_PASSWORD`, and `REDIS_DATABASE` describe the same instance. Never commit passwords or connection URLs.

## Node Installation

Create a node in the administrator panel and use that node's **Install** command. It contains the panel address and node-specific secret.

```bash
curl -fsSL https://raw.githubusercontent.com/ziyue67/Tms/main/install.sh -o install.sh
chmod +x install.sh
./install.sh -a PANEL_HOST:6365 -s NODE_SECRET
```

If a download returns `Not Found` or HTML, do not run it. Download the current script again through a trusted path and inspect it first.

## Upgrade and Cleanup

Take a backup before upgrading, then use `tms update` and `tms status`. For detailed startup diagnosis, run `docker logs --tail 200 springboot-backend`.

Keep `/opt/tms`: it is the active installation and holds Compose configuration, `.env`, and operational state. After a successful deployment, old build checkouts and deployment logs can be removed:

```bash
rm -rf /opt/tms-build-*
rm -f /opt/tms-deploy-*.sh /opt/tms-deploy-*.log
```

Do not remove `/opt/1panel`, `/opt/gscore-login`, `/opt/komari`, `/opt/containerd`, or `/opt/.1panel_swap` unless their corresponding service is no longer used. Do not delete Docker volumes without confirming their data and backup requirements.

## Security

- Change the default administrator password.
- Use a unique long `JWT_SECRET`, database password, and Redis password.
- Keep SMTP and payment secrets in panel configuration or deployment secrets only.
- Restrict database, Redis, and node ports with firewall rules.
- Take a database backup before upgrades, migrations, or cleanup.

## License

See `LICENSE` for upstream code and `LICENSE-MIT` for fork additions.
