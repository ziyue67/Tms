package database

import (
	"strings"
	"testing"

	"github.com/ziyue67/tms/go-backend/internal/config"
)

func TestMySQLJDBCURL(t *testing.T) {
	driver, dsn, dialect, err := connectionDetails(config.Database{
		Driver:   "com.mysql.cj.jdbc.Driver",
		URL:      "jdbc:mysql://database.example:3307/gost?useUnicode=true&useSSL=false&serverTimezone=Asia/Shanghai",
		User:     "tms user",
		Password: "p@ss:word",
	})
	if err != nil {
		t.Fatalf("connection details: %v", err)
	}
	if driver != "mysql" || dialect != MySQL {
		t.Fatalf("unexpected driver or dialect: %s %s", driver, dialect)
	}
	if !strings.Contains(dsn, "tcp(database.example:3307)/gost") {
		t.Fatalf("JDBC host was not converted: %s", dsn)
	}
}

func TestPostgresJDBCURL(t *testing.T) {
	driver, dsn, dialect, err := connectionDetails(config.Database{
		Driver:   "org.postgresql.Driver",
		URL:      "jdbc:postgresql://database.example:5432/gost?sslmode=require",
		User:     "gost",
		Password: "secret",
	})
	if err != nil {
		t.Fatalf("connection details: %v", err)
	}
	if driver != "pgx" || dialect != PostgreSQL {
		t.Fatalf("unexpected driver or dialect: %s %s", driver, dialect)
	}
	if !strings.HasPrefix(dsn, "postgresql://gost:secret@database.example:5432/gost") || !strings.Contains(dsn, "sslmode=require") {
		t.Fatalf("JDBC URL was not converted: %s", dsn)
	}
}
