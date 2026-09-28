package database

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// Migrate applies additive compatibility changes and is idempotent for existing installs.
func Migrate(ctx context.Context, db *sql.DB, dialect Dialect) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	m := migrator{tx: tx, dialect: dialect}
	for _, column := range []columnMigration{
		{"node", "domain", "VARCHAR(255) NULL"}, {"node", "cert_mode", "INTEGER NOT NULL DEFAULT 0"},
		{"node", "cert_path", "VARCHAR(500) NULL"}, {"node", "key_path", "VARCHAR(500) NULL"},
		{"user", "all_sub_token", "VARCHAR(64) NULL"}, {"user", "email", "VARCHAR(190) NULL"},
		{"inbound", "landing_id", "BIGINT NULL"}, {"subscription_plan", "reset_quota", "INTEGER NOT NULL DEFAULT 1"},
		{"redeem_code", "code_value", "VARCHAR(64) NULL"}, {"custom_node", "visibility", "VARCHAR(12) NOT NULL DEFAULT 'global'"},
	} {
		if err := m.addColumn(ctx, column); err != nil {
			return err
		}
	}
	if err := m.ensureUserView(ctx); err != nil {
		return err
	}
	if err := m.ensureUserEmailIndex(ctx); err != nil {
		return err
	}
	if err := m.expandConfigValue(ctx); err != nil {
		return err
	}
	for name, definition := range commerceTables(dialect) {
		if err := m.createTable(ctx, name, definition); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, "DROP TABLE IF EXISTS "+m.quote("verification_code")); err != nil {
		return err
	}
	return tx.Commit()
}

type columnMigration struct{ table, column, definition string }
type migrator struct {
	tx      *sql.Tx
	dialect Dialect
}

func (m migrator) quote(value string) string {
	if m.dialect == PostgreSQL {
		return `"` + value + `"`
	}
	return "`" + value + "`"
}

func (m migrator) tableExists(ctx context.Context, table string) (bool, error) {
	var count int
	if m.dialect == PostgreSQL {
		err := m.tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=current_schema() AND table_name=$1", table).Scan(&count)
		return count > 0, err
	}
	err := m.tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name=?", table).Scan(&count)
	return count > 0, err
}

func (m migrator) columnExists(ctx context.Context, table, column string) (bool, error) {
	var count int
	if m.dialect == PostgreSQL {
		err := m.tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=current_schema() AND table_name=$1 AND column_name=$2", table, column).Scan(&count)
		return count > 0, err
	}
	err := m.tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name=? AND column_name=?", table, column).Scan(&count)
	return count > 0, err
}

func (m migrator) addColumn(ctx context.Context, migration columnMigration) error {
	exists, err := m.tableExists(ctx, migration.table)
	if err != nil || !exists {
		return err
	}
	exists, err = m.columnExists(ctx, migration.table, migration.column)
	if err != nil || exists {
		return err
	}
	_, err = m.tx.ExecContext(ctx, "ALTER TABLE "+m.quote(migration.table)+" ADD COLUMN "+m.quote(migration.column)+" "+migration.definition)
	return err
}

func (m migrator) createTable(ctx context.Context, name, definition string) error {
	exists, err := m.tableExists(ctx, name)
	if err != nil || exists {
		return err
	}
	_, err = m.tx.ExecContext(ctx, "CREATE TABLE "+m.quote(name)+" "+definition)
	return err
}

func (m migrator) ensureUserView(ctx context.Context) error {
	exists, err := m.tableExists(ctx, "user")
	if err != nil || !exists {
		return err
	}
	_, err = m.tx.ExecContext(ctx, "CREATE OR REPLACE VIEW "+m.quote("tms_user")+" AS SELECT * FROM "+m.quote("user"))
	return err
}

func (m migrator) ensureUserEmailIndex(ctx context.Context) error {
	exists, err := m.tableExists(ctx, "user")
	if err != nil || !exists {
		return err
	}
	if m.dialect == PostgreSQL {
		_, err = m.tx.ExecContext(ctx, `CREATE UNIQUE INDEX IF NOT EXISTS "uk_user_email" ON "user" ("email") WHERE "email" IS NOT NULL`)
		return err
	}
	var count int
	err = m.tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.statistics WHERE table_schema=DATABASE() AND table_name='user' AND index_name='uk_user_email'").Scan(&count)
	if err != nil || count > 0 {
		return err
	}
	_, err = m.tx.ExecContext(ctx, "CREATE UNIQUE INDEX `uk_user_email` ON `user` (`email`)")
	return err
}

func (m migrator) expandConfigValue(ctx context.Context) error {
	exists, err := m.tableExists(ctx, "vite_config")
	if err != nil || !exists {
		return err
	}
	var dataType string
	if m.dialect == PostgreSQL {
		err = m.tx.QueryRowContext(ctx, "SELECT data_type FROM information_schema.columns WHERE table_schema=current_schema() AND table_name='vite_config' AND column_name='value'").Scan(&dataType)
		if err == nil && !strings.Contains(strings.ToLower(dataType), "text") {
			_, err = m.tx.ExecContext(ctx, `ALTER TABLE "vite_config" ALTER COLUMN "value" TYPE TEXT`)
		}
		return err
	}
	err = m.tx.QueryRowContext(ctx, "SELECT data_type FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='vite_config' AND column_name='value'").Scan(&dataType)
	if err == nil && !strings.Contains(strings.ToLower(dataType), "text") {
		_, err = m.tx.ExecContext(ctx, "ALTER TABLE `vite_config` MODIFY COLUMN `value` TEXT NOT NULL")
	}
	return err
}

func commerceTables(dialect Dialect) map[string]string {
	id := "BIGINT AUTO_INCREMENT"
	jsonType := "JSON"
	if dialect == PostgreSQL {
		id = "BIGINT GENERATED BY DEFAULT AS IDENTITY"
		jsonType = "JSONB"
	}
	return map[string]string{
		"subscription_plan":      fmt.Sprintf("(id %s PRIMARY KEY, name VARCHAR(100) NOT NULL, description VARCHAR(500), price DECIMAL(12,2) NOT NULL DEFAULT 0, currency VARCHAR(10) NOT NULL DEFAULT 'CNY', validity_value INTEGER NOT NULL, validity_unit VARCHAR(10) NOT NULL DEFAULT 'month', traffic_bytes BIGINT NOT NULL DEFAULT 0, reset_day INTEGER NOT NULL DEFAULT 1, reset_quota INTEGER NOT NULL DEFAULT 1, max_forwards INTEGER NOT NULL DEFAULT 0, for_sale INTEGER NOT NULL DEFAULT 1, redeemable INTEGER NOT NULL DEFAULT 1, sort_order INTEGER NOT NULL DEFAULT 0, status INTEGER NOT NULL DEFAULT 1, created_time BIGINT NOT NULL, updated_time BIGINT NOT NULL)", id),
		"user_subscription":      fmt.Sprintf("(id %s PRIMARY KEY, user_id BIGINT NOT NULL UNIQUE, plan_id BIGINT NOT NULL, starts_at BIGINT NOT NULL, expires_at BIGINT NOT NULL, traffic_limit_bytes BIGINT NOT NULL DEFAULT 0, traffic_used_bytes BIGINT NOT NULL DEFAULT 0, next_reset_at BIGINT, max_forwards INTEGER NOT NULL DEFAULT 0, used_forwards INTEGER NOT NULL DEFAULT 0, status INTEGER NOT NULL DEFAULT 1, created_time BIGINT NOT NULL, updated_time BIGINT NOT NULL)", id),
		"redeem_code":            fmt.Sprintf("(id %s PRIMARY KEY, plan_id BIGINT NOT NULL, code_hash VARCHAR(64) NOT NULL UNIQUE, code_value VARCHAR(64), code_preview VARCHAR(20) NOT NULL, batch_id VARCHAR(64), status INTEGER NOT NULL DEFAULT 1, used_by BIGINT, used_time BIGINT, expires_at BIGINT, remark VARCHAR(255), created_time BIGINT NOT NULL)", id),
		"quota_usage_log":        fmt.Sprintf("(id %s PRIMARY KEY, user_id BIGINT NOT NULL, subscription_id BIGINT, event_type VARCHAR(32) NOT NULL, amount BIGINT NOT NULL DEFAULT 0, metadata %s, created_time BIGINT NOT NULL)", id, jsonType),
		"payment_order":          fmt.Sprintf("(id %s PRIMARY KEY, order_no VARCHAR(64) NOT NULL UNIQUE, user_id BIGINT NOT NULL, plan_id BIGINT NOT NULL, provider VARCHAR(20) NOT NULL, amount DECIMAL(12,2) NOT NULL, currency VARCHAR(10) NOT NULL DEFAULT 'CNY', status VARCHAR(20) NOT NULL DEFAULT 'pending', provider_trade_no VARCHAR(128), callback_payload TEXT, paid_at BIGINT, created_time BIGINT NOT NULL, updated_time BIGINT NOT NULL)", id),
		"custom_node":            fmt.Sprintf("(id %s PRIMARY KEY, name VARCHAR(255) NOT NULL, protocol VARCHAR(32) NOT NULL, raw_link TEXT NOT NULL, parsed_json TEXT NOT NULL, visibility VARCHAR(12) NOT NULL DEFAULT 'global', status INTEGER NOT NULL DEFAULT 1, created_time BIGINT NOT NULL, updated_time BIGINT NOT NULL)", id),
		"user_custom_node":       fmt.Sprintf("(id %s PRIMARY KEY, user_id BIGINT NOT NULL, custom_node_id BIGINT NOT NULL, status INTEGER NOT NULL DEFAULT 1, created_time BIGINT NOT NULL, UNIQUE(user_id, custom_node_id))", id),
		"inbound_auto_provision": fmt.Sprintf("(id %s PRIMARY KEY, node_id BIGINT NOT NULL, landing_id BIGINT NOT NULL DEFAULT 0, enabled INTEGER NOT NULL DEFAULT 1, created_time BIGINT NOT NULL, updated_time BIGINT NOT NULL, UNIQUE(node_id, landing_id))", id),
	}
}
