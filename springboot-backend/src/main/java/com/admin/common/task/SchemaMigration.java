package com.admin.common.task;

import lombok.extern.slf4j.Slf4j;
import org.springframework.boot.ApplicationArguments;
import org.springframework.boot.ApplicationRunner;
import org.springframework.core.annotation.Order;
import org.springframework.stereotype.Component;

import javax.annotation.Resource;
import javax.sql.DataSource;
import java.sql.Connection;
import java.sql.DatabaseMetaData;
import java.sql.ResultSet;
import java.sql.SQLException;
import java.sql.Statement;
import java.util.Locale;

/** Idempotent migration for old TMS databases and external MySQL/PostgreSQL databases. */
@Slf4j
@Component
@Order(1)
public class SchemaMigration implements ApplicationRunner {
    @Resource
    private DataSource dataSource;

    @Override
    public void run(ApplicationArguments args) throws Exception {
        try (Connection connection = dataSource.getConnection()) {
            Dialect dialect = Dialect.from(connection.getMetaData());
            migrateBaseColumns(connection, dialect);
            migrateConfigColumns(connection, dialect);
            migrateCommerceTables(connection, dialect);
        }
        log.info("TMS database schema migration completed");
    }

    private void migrateBaseColumns(Connection c, Dialect d) throws SQLException {
        addColumn(c, d, "node", "domain", "VARCHAR(255) NULL");
        addColumn(c, d, "node", "cert_mode", "INTEGER NOT NULL DEFAULT 0");
        addColumn(c, d, "node", "cert_path", "VARCHAR(500) NULL");
        addColumn(c, d, "node", "key_path", "VARCHAR(500) NULL");
        addColumn(c, d, "user", "all_sub_token", "VARCHAR(64) NULL");
        addColumn(c, d, "user", "email", "VARCHAR(190) NULL");
        addColumn(c, d, "inbound", "landing_id", "BIGINT NULL");
        createUniqueIndex(c, d, "user", "uk_user_email", "email");
    }

    /** Email HTML templates are substantially larger than the legacy 200-character setting value. */
    private void migrateConfigColumns(Connection c, Dialect d) throws SQLException {
        if (!tableExists(c, "vite_config") || textColumn(c, "vite_config", "value")) return;
        String table = d.q("vite_config");
        if (d.postgres) execute(c, "ALTER TABLE " + table + " ALTER COLUMN " + d.q("value") + " TYPE TEXT");
        else execute(c, "ALTER TABLE " + table + " MODIFY COLUMN " + d.q("value") + " TEXT NOT NULL");
        log.info("Expanded vite_config.value for HTML email templates");
    }

    private void migrateCommerceTables(Connection c, Dialect d) throws SQLException {
        String id = d.identity();
        createTable(c, d, "subscription_plan", "(" +
                "id " + id + ", name VARCHAR(100) NOT NULL, description VARCHAR(500), price DECIMAL(12,2) NOT NULL DEFAULT 0, currency VARCHAR(10) NOT NULL DEFAULT 'CNY', " +
                "validity_value INTEGER NOT NULL, validity_unit VARCHAR(10) NOT NULL DEFAULT 'month', traffic_bytes BIGINT NOT NULL DEFAULT 0, reset_day INTEGER NOT NULL DEFAULT 1, reset_quota INTEGER NOT NULL DEFAULT 1, max_forwards INTEGER NOT NULL DEFAULT 0, for_sale INTEGER NOT NULL DEFAULT 1, redeemable INTEGER NOT NULL DEFAULT 1, sort_order INTEGER NOT NULL DEFAULT 0, status INTEGER NOT NULL DEFAULT 1, created_time BIGINT NOT NULL, updated_time BIGINT NOT NULL, PRIMARY KEY (id))");
        addColumn(c, d, "subscription_plan", "description", "VARCHAR(500) NULL");
        addColumn(c, d, "subscription_plan", "price", "DECIMAL(12,2) NOT NULL DEFAULT 0");
        addColumn(c, d, "subscription_plan", "currency", "VARCHAR(10) NOT NULL DEFAULT 'CNY'");
        addColumn(c, d, "subscription_plan", "validity_value", "INTEGER NOT NULL DEFAULT 1");
        addColumn(c, d, "subscription_plan", "validity_unit", "VARCHAR(10) NOT NULL DEFAULT 'month'");
        addColumn(c, d, "subscription_plan", "traffic_bytes", "BIGINT NOT NULL DEFAULT 0");
        addColumn(c, d, "subscription_plan", "reset_day", "INTEGER NOT NULL DEFAULT 1");
        addColumn(c, d, "subscription_plan", "reset_quota", "INTEGER NOT NULL DEFAULT 1");
        addColumn(c, d, "subscription_plan", "max_forwards", "INTEGER NOT NULL DEFAULT 0");
        addColumn(c, d, "subscription_plan", "for_sale", "INTEGER NOT NULL DEFAULT 1");
        addColumn(c, d, "subscription_plan", "redeemable", "INTEGER NOT NULL DEFAULT 1");
        addColumn(c, d, "subscription_plan", "sort_order", "INTEGER NOT NULL DEFAULT 0");
        addColumn(c, d, "subscription_plan", "status", "INTEGER NOT NULL DEFAULT 1");
        // Older installations predate the commerce flags and may contain NULLs.
        // Normalize them once so both MyBatis and the admin UI see deterministic values.
        normalizePlanColumns(c, d);
        createTable(c, d, "user_subscription", "(" +
                "id " + id + ", user_id BIGINT NOT NULL, plan_id BIGINT NOT NULL, starts_at BIGINT NOT NULL, expires_at BIGINT NOT NULL, traffic_limit_bytes BIGINT NOT NULL DEFAULT 0, traffic_used_bytes BIGINT NOT NULL DEFAULT 0, next_reset_at BIGINT NULL, max_forwards INTEGER NOT NULL DEFAULT 0, used_forwards INTEGER NOT NULL DEFAULT 0, status INTEGER NOT NULL DEFAULT 1, created_time BIGINT NOT NULL, updated_time BIGINT NOT NULL, PRIMARY KEY (id), UNIQUE (user_id))");
        createTable(c, d, "redeem_code", "(" +
                "id " + id + ", plan_id BIGINT NOT NULL, code_hash VARCHAR(64) NOT NULL, code_value VARCHAR(64) NULL, code_preview VARCHAR(20) NOT NULL, batch_id VARCHAR(64) NULL, status INTEGER NOT NULL DEFAULT 1, used_by BIGINT NULL, used_time BIGINT NULL, expires_at BIGINT NULL, remark VARCHAR(255) NULL, created_time BIGINT NOT NULL, PRIMARY KEY (id), UNIQUE (code_hash))");
        // Existing records only stored a one-way hash, so their original values cannot be restored.
        addColumn(c, d, "redeem_code", "code_value", "VARCHAR(64) NULL");
        createTable(c, d, "quota_usage_log", "(" +
                "id " + id + ", user_id BIGINT NOT NULL, subscription_id BIGINT NULL, event_type VARCHAR(32) NOT NULL, amount BIGINT NOT NULL DEFAULT 0, metadata " + d.jsonType() + " NULL, created_time BIGINT NOT NULL, PRIMARY KEY (id))");
        createTable(c, d, "payment_order", "(" +
                "id " + id + ", order_no VARCHAR(64) NOT NULL, user_id BIGINT NOT NULL, plan_id BIGINT NOT NULL, provider VARCHAR(20) NOT NULL, amount DECIMAL(12,2) NOT NULL, currency VARCHAR(10) NOT NULL DEFAULT 'CNY', status VARCHAR(20) NOT NULL DEFAULT 'pending', provider_trade_no VARCHAR(128) NULL, callback_payload TEXT NULL, paid_at BIGINT NULL, created_time BIGINT NOT NULL, updated_time BIGINT NOT NULL, PRIMARY KEY (id), UNIQUE (order_no))");
        createTable(c, d, "custom_node", "(" +
                "id " + id + ", name VARCHAR(255) NOT NULL, protocol VARCHAR(32) NOT NULL, raw_link TEXT NOT NULL, parsed_json TEXT NOT NULL, visibility VARCHAR(12) NOT NULL DEFAULT 'global', status INTEGER NOT NULL DEFAULT 1, created_time BIGINT NOT NULL, updated_time BIGINT NOT NULL, PRIMARY KEY (id))");
        addColumn(c, d, "custom_node", "visibility", "VARCHAR(12) NOT NULL DEFAULT 'global'");
        createTable(c, d, "user_custom_node", "(" +
                "id " + id + ", user_id BIGINT NOT NULL, custom_node_id BIGINT NOT NULL, status INTEGER NOT NULL DEFAULT 1, created_time BIGINT NOT NULL, PRIMARY KEY (id), UNIQUE (user_id, custom_node_id))");
        // Temporary verification data is Redis-only. Remove the legacy table from older installations.
        dropTable(c, d, "verification_code");
        // Before visibility was introduced, only nodes with assignment rows were
        // user-scoped. Preserve that behavior for existing installations.
        execute(c, "UPDATE " + d.q("custom_node") + " n SET " + d.q("visibility") + "='users' WHERE " + d.q("visibility") + "='global' AND EXISTS (SELECT 1 FROM " + d.q("user_custom_node") + " a WHERE a.custom_node_id=n.id AND a.status=1)");
    }

    private void normalizePlanColumns(Connection c, Dialect d) throws SQLException {
        if (!tableExists(c, "subscription_plan")) return;
        String table = d.q("subscription_plan");
        execute(c, "UPDATE " + table + " SET " + d.q("status") + "=1 WHERE " + d.q("status") + " IS NULL");
        execute(c, "UPDATE " + table + " SET " + d.q("for_sale") + "=1 WHERE " + d.q("for_sale") + " IS NULL");
        execute(c, "UPDATE " + table + " SET " + d.q("redeemable") + "=1 WHERE " + d.q("redeemable") + " IS NULL");
        execute(c, "UPDATE " + table + " SET " + d.q("reset_quota") + "=1 WHERE " + d.q("reset_quota") + " IS NULL");
        execute(c, "UPDATE " + table + " SET " + d.q("reset_day") + "=1 WHERE " + d.q("reset_day") + " IS NULL");
        execute(c, "UPDATE " + table + " SET " + d.q("max_forwards") + "=0 WHERE " + d.q("max_forwards") + " IS NULL");
    }

    private void addColumn(Connection c, Dialect d, String table, String column, String definition) throws SQLException {
        if (!tableExists(c, table) || columnExists(c, table, column)) return;
        execute(c, "ALTER TABLE " + d.q(table) + " ADD COLUMN " + d.q(column) + " " + definition);
        log.info("Added missing database column {}.{}", table, column);
    }

    private void createTable(Connection c, Dialect d, String table, String definition) throws SQLException {
        if (tableExists(c, table)) return;
        execute(c, "CREATE TABLE " + d.q(table) + " " + definition);
        log.info("Created missing database table {}", table);
    }

    private void dropTable(Connection c, Dialect d, String table) throws SQLException {
        if (!tableExists(c, table)) return;
        execute(c, "DROP TABLE " + d.q(table));
        log.info("Removed legacy database table {}", table);
    }

    private void createUniqueIndex(Connection c, Dialect d, String table, String index, String column) throws SQLException {
        if (!tableExists(c, table) || indexExists(c, table, index)) return;
        execute(c, "CREATE UNIQUE INDEX " + d.q(index) + " ON " + d.q(table) + " (" + d.q(column) + ")");
    }

    private void execute(Connection c, String sql) throws SQLException {
        try (Statement statement = c.createStatement()) { statement.executeUpdate(sql); }
    }

    private boolean tableExists(Connection c, String table) throws SQLException {
        DatabaseMetaData m = c.getMetaData();
        try (ResultSet rs = m.getTables(c.getCatalog(), c.getSchema(), table, new String[]{"TABLE"})) {
            if (rs.next()) return true;
        }
        try (ResultSet rs = m.getTables(c.getCatalog(), c.getSchema(), table.toUpperCase(Locale.ROOT), new String[]{"TABLE"})) { return rs.next(); }
    }

    private boolean columnExists(Connection c, String table, String column) throws SQLException {
        DatabaseMetaData m = c.getMetaData();
        try (ResultSet rs = m.getColumns(c.getCatalog(), c.getSchema(), table, column)) {
            if (rs.next()) return true;
        }
        try (ResultSet rs = m.getColumns(c.getCatalog(), c.getSchema(), table.toUpperCase(Locale.ROOT), column.toUpperCase(Locale.ROOT))) { return rs.next(); }
    }

    private boolean textColumn(Connection c, String table, String column) throws SQLException {
        DatabaseMetaData m = c.getMetaData();
        try (ResultSet rs = m.getColumns(c.getCatalog(), c.getSchema(), table, column)) {
            if (rs.next()) return isTextType(rs.getString("TYPE_NAME"));
        }
        try (ResultSet rs = m.getColumns(c.getCatalog(), c.getSchema(), table.toUpperCase(Locale.ROOT), column.toUpperCase(Locale.ROOT))) {
            return rs.next() && isTextType(rs.getString("TYPE_NAME"));
        }
    }

    private boolean isTextType(String typeName) {
        String type = typeName == null ? "" : typeName.toLowerCase(Locale.ROOT);
        return type.contains("text") || type.contains("clob") || type.equals("longvarchar");
    }

    private boolean indexExists(Connection c, String table, String index) throws SQLException {
        try (ResultSet rs = c.getMetaData().getIndexInfo(c.getCatalog(), c.getSchema(), table, false, false)) {
            while (rs.next()) {
                if (index.equalsIgnoreCase(rs.getString("INDEX_NAME"))) return true;
            }
            return false;
        }
    }

    private record Dialect(boolean postgres) {
        static Dialect from(DatabaseMetaData metadata) throws SQLException { return new Dialect(metadata.getDatabaseProductName().toLowerCase(Locale.ROOT).contains("postgres")); }
        String q(String value) { return postgres ? "\"" + value + "\"" : "`" + value + "`"; }
        String identity() { return postgres ? "BIGINT GENERATED BY DEFAULT AS IDENTITY" : "BIGINT AUTO_INCREMENT"; }
        String jsonType() { return postgres ? "JSONB" : "JSON"; }
    }
}
