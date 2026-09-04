# TMS

TMS 是一个用于管理 GOST 节点、协议、转发、中转线路和用户订阅的面板。项目包含管理员后台、用户订阅、套餐与兑换码、流量统计、到期控制、SMTP 和支付配置。

## 主要功能

- 管理节点、协议入站、转发、中转线路和限速规则。
- 为用户生成 V2Ray/VLESS、Trojan、VMess、Hysteria2、TUIC、AnyTLS、Shadowsocks 订阅。
- “全部线路”聚合订阅，支持 V2Ray 兼容客户端和 Clash/Mihomo。
- 套餐流量额度、有效期、每月重置、转发数量限制和兑换码。
- 自定义订阅节点：全局聚合（所有用户）、全局聚合（套餐用户）或按用户订阅。
- 套餐用户自动分配已启用的协议和中转线路。
- 管理员和用户流量统计、节点 CPU/内存/流量/运行时间监控。
- 支持内置 MySQL/Redis，也支持外部 MySQL、PostgreSQL 和 Redis。

## 环境要求

- Linux
- Docker Engine
- Docker Compose v2
- 面板服务器需要能访问节点端口。
- 外部数据库：PostgreSQL 13+ 或 MySQL 5.7+。
- 外部 Redis：Redis 6+。

数据库和 Redis 端口应使用防火墙或访问控制限制来源，不建议直接暴露给公网。

## 一键安装

在面板服务器执行：

```bash
curl -fsSL https://raw.githubusercontent.com/ziyue67/Tms/main/panel_install.sh -o panel_install.sh
chmod +x panel_install.sh
./panel_install.sh
```

安装程序会输出面板地址，并创建 `tms` 管理命令。首次登录后请立即修改管理员密码。

## 使用外部 PostgreSQL 和 Redis

设置 `DB_URL` 或 `REDIS_URL` 后，安装程序会生成 Compose 覆盖配置并停用对应的内置服务。配置正确时不会下载或启动 Fork 自带的 MySQL/Redis 容器。

### PostgreSQL 初始化

新数据库可以导入项目提供的结构文件：

```bash
psql 'postgresql://用户名:密码@数据库地址:5432/gost' \
  -f springboot-backend/src/main/resources/db/tms-postgres.sql
```

### 安装配置

```bash
export DB_URL='jdbc:postgresql://数据库地址:5432/gost'
export DB_DRIVER='org.postgresql.Driver'
export DB_USER='gost'
export DB_PASSWORD='请替换为强密码'
export REDIS_URL='redis://:请替换为强密码@Redis地址:6379/0'
export JWT_SECRET='请替换为随机的32位以上密钥'

curl -fsSL https://raw.githubusercontent.com/ziyue67/Tms/main/panel_install.sh -o panel_install.sh
chmod +x panel_install.sh
./panel_install.sh
```

手动使用 Compose 时，根据实际环境选择对应文件，例如：

```bash
docker compose \
  -f docker-compose-v4.yml \
  -f docker-compose-external-postgres.yml \
  -f docker-compose-external-redis.yml \
  --env-file .env up -d
```

项目旧表名为 `user`。PostgreSQL 下 TMS 会创建可写的 `tms_user` 兼容视图，不要手动重命名旧表。后端启动时会自动执行幂等迁移。

### 外部 MySQL

```bash
export DB_URL='jdbc:mysql://数据库地址:3306/gost?useUnicode=true&useSSL=false&characterEncoding=utf8&serverTimezone=Asia/Shanghai'
export DB_DRIVER='com.mysql.cj.jdbc.Driver'
export DB_USER='gost'
export DB_PASSWORD='请替换为强密码'
```

新 MySQL 数据库可导入根目录的 `gost.sql`。升级生产数据库前请先备份。

## Redis 检查

```bash
REDISCLI_AUTH='Redis密码' redis-cli -h Redis地址 -p 6379 -n 0 ping
```

返回 `PONG` 才表示连接正常。使用分开配置时，确认 `REDIS_HOST`、`REDIS_PORT`、`REDIS_PASSWORD`、`REDIS_DATABASE` 指向同一个 Redis 实例。不要把密码或完整连接地址提交到 Git。

## 添加节点

在管理员后台创建节点后，复制该节点的安装命令，在节点服务器执行：

```bash
curl -fsSL https://raw.githubusercontent.com/ziyue67/Tms/main/install.sh -o install.sh
chmod +x install.sh
./install.sh -a 面板地址:6365 -s 节点密钥
```

安装命令会自动放行该节点的 GOST 公网转发端口（TCP 和 UDP）。如果服务器未安装 UFW，脚本会打印可复制的放行命令；云厂商安全组也必须放行相同范围。`41000+` 是仅本机使用的 sing-box 内部端口，不需要对公网开放。

如果下载结果只有 `Not Found` 或 HTML，说明下载地址或 Release 资源不正确，不要直接执行，先检查脚本内容并重新下载。

## 订阅和套餐

1. 在“套餐管理”创建套餐，设置流量、有效期、重置日和是否允许兑换。
2. 在“兑换码”页面生成兑换码，用户兑换后会触发已启用协议和中转线路的自动分配。
3. 在“协议管理”或“中转管理”启用“套餐用户自动分配”，新兑换用户会自动获得对应线路。
4. 在“导入自定义协议节点”选择订阅范围：
   - `全局聚合（套餐用户）`：只向有效套餐用户的聚合订阅提供节点。
   - `全局聚合（所有用户）`：全局范围节点；如需限制访问，建议使用套餐用户范围。
   - `按用户订阅`：只向选定用户提供节点。

自定义外部节点本身不经过 TMS，无法按用户统计流量。需要计费和流量统计时，应选择中转出站并使用系统生成的 TMS 协议线路。

## 升级、状态和日志

```bash
tms status
tms update
```

也可以直接查看容器：

```bash
docker compose ps
docker logs --tail 200 springboot-backend
docker logs --tail 200 vite-frontend
```

升级前请备份数据库和 `.env`。外部 PostgreSQL、Redis 不需要也不应由项目 Compose 重建。

## 清理服务器文件

`/opt/tms` 是当前安装目录，包含 Compose 配置、`.env` 和运行状态，不要删除。

确认部署成功后，可以清理旧构建目录和部署日志：

```bash
rm -rf /opt/tms-build-*
rm -f /opt/tms-deploy-*.sh /opt/tms-deploy-*.log
```

除非确认对应服务已不再使用，否则不要删除 `/opt/1panel`、`/opt/gscore-login`、`/opt/komari`、`/opt/containerd` 或 `/opt/.1panel_swap`。删除 Docker volume 前必须确认其中没有需要保留的数据。

## 安全建议

- 修改默认管理员密码。
- 为 JWT、数据库和 Redis 使用不同的强随机密码。
- SMTP、支付和数据库密钥只放在服务器配置或部署密钥中。
- 用防火墙限制数据库、Redis、面板和节点端口。
- 执行迁移、升级或清理前先做数据库备份。

## 开发

后端目录为 `springboot-backend`，前端目录为 `vite-frontend`。

后端测试：

```bash
cd springboot-backend
mvn test
```

前端构建：

```bash
cd vite-frontend
npm install
npm run build
```

## 许可证

上游代码遵循 `LICENSE` 中的 Apache-2.0 许可；Fork 增加的功能遵循 `LICENSE-MIT`。请同时保留上游版权和许可声明。
