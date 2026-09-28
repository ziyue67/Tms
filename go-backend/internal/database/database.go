package database

import (
	"context"
	"crypto/tls"
	"database/sql"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/ziyue67/tms/go-backend/internal/config"
)

type Dialect string

const (
	MySQL      Dialect = "mysql"
	PostgreSQL Dialect = "postgres"
)

func Open(ctx context.Context, cfg config.Database) (*sql.DB, Dialect, error) {
	driver, dsn, dialect, err := connectionDetails(cfg)
	if err != nil {
		return nil, "", err
	}

	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, "", fmt.Errorf("open database: %w", err)
	}
	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, "", fmt.Errorf("ping database: %w", err)
	}
	return db, dialect, nil
}

func connectionDetails(cfg config.Database) (string, string, Dialect, error) {
	rawURL := strings.TrimSpace(cfg.URL)
	driverHint := strings.ToLower(cfg.Driver)
	postgres := strings.Contains(driverHint, "postgres") || strings.HasPrefix(rawURL, "jdbc:postgresql:") || strings.HasPrefix(rawURL, "postgres")
	if postgres {
		dsn, err := postgresDSN(cfg)
		return "pgx", dsn, PostgreSQL, err
	}
	dsn, err := mysqlDSN(cfg)
	return "mysql", dsn, MySQL, err
}

func mysqlDSN(cfg config.Database) (string, error) {
	host := cfg.Host
	port := cfg.Port
	databaseName := cfg.Name
	params := map[string]string{}

	if cfg.URL != "" {
		raw := strings.TrimPrefix(cfg.URL, "jdbc:")
		u, err := url.Parse(raw)
		if err != nil || u.Scheme != "mysql" {
			return "", fmt.Errorf("invalid MySQL DB_URL %q", cfg.URL)
		}
		host = u.Hostname()
		if parsedPort, err := strconv.Atoi(u.Port()); err == nil {
			port = parsedPort
		}
		databaseName = strings.TrimPrefix(u.Path, "/")
		for key, values := range u.Query() {
			if len(values) > 0 {
				params[key] = values[len(values)-1]
			}
		}
	}
	if host == "" || databaseName == "" {
		return "", fmt.Errorf("MySQL host and database name are required")
	}

	mysqlCfg := mysql.NewConfig()
	mysqlCfg.User = cfg.User
	mysqlCfg.Passwd = cfg.Password
	mysqlCfg.Net = "tcp"
	mysqlCfg.Addr = fmt.Sprintf("%s:%d", host, port)
	mysqlCfg.DBName = databaseName
	mysqlCfg.ParseTime = true
	mysqlCfg.Collation = "utf8mb4_unicode_ci"
	if location, err := time.LoadLocation("Asia/Shanghai"); err == nil {
		mysqlCfg.Loc = location
	}
	mysqlCfg.Params = map[string]string{"charset": "utf8mb4"}
	if strings.EqualFold(params["useSSL"], "true") {
		const tlsName = "tms"
		_ = mysql.RegisterTLSConfig(tlsName, &tls.Config{MinVersion: tls.VersionTLS12})
		mysqlCfg.TLSConfig = tlsName
	}
	return mysqlCfg.FormatDSN(), nil
}

func postgresDSN(cfg config.Database) (string, error) {
	var u *url.URL
	var err error
	if cfg.URL == "" {
		u = &url.URL{Scheme: "postgresql", Host: fmt.Sprintf("%s:%d", cfg.Host, cfg.Port), Path: "/" + cfg.Name}
	} else {
		raw := strings.TrimPrefix(cfg.URL, "jdbc:")
		u, err = url.Parse(raw)
		if err != nil || (u.Scheme != "postgresql" && u.Scheme != "postgres") {
			return "", fmt.Errorf("invalid PostgreSQL DB_URL %q", cfg.URL)
		}
	}
	if u.Hostname() == "" || strings.TrimPrefix(u.Path, "/") == "" {
		return "", fmt.Errorf("PostgreSQL host and database name are required")
	}
	u.User = url.UserPassword(cfg.User, cfg.Password)
	query := u.Query()
	if query.Get("sslmode") == "" {
		query.Set("sslmode", "disable")
	}
	u.RawQuery = query.Encode()
	return u.String(), nil
}
