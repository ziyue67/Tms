package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ziyue67/tms/go-backend/internal/database"
)

type User struct {
	ID       int64
	Username string
	Email    sql.NullString
	Password string
	RoleID   int
	Status   int
}

type SiteConfig struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Value string `json:"value"`
	Time  int64  `json:"time"`
}

type Store struct {
	db      *sql.DB
	dialect database.Dialect
}

func New(db *sql.DB, dialect database.Dialect) *Store {
	return &Store{db: db, dialect: dialect}
}

func (s *Store) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

func (s *Store) UserByLogin(ctx context.Context, login string) (User, error) {
	userColumn := s.quote("user")
	emailColumn := s.quote("email")
	query := "SELECT " + s.columns("id", "user", "email", "pwd", "role_id", "status") +
		" FROM " + s.quote("tms_user") + " WHERE " + userColumn + " = ?"
	args := []any{login}
	if strings.Contains(login, "@") {
		query += " OR LOWER(" + emailColumn + ") = ?"
		args = append(args, strings.ToLower(login))
	}
	query += " LIMIT 1"
	query = s.bind(query)

	var user User
	err := s.db.QueryRowContext(ctx, query, args...).Scan(
		&user.ID, &user.Username, &user.Email, &user.Password, &user.RoleID, &user.Status,
	)
	if err != nil {
		return User{}, err
	}
	return user, nil
}

func (s *Store) UpdatePassword(ctx context.Context, userID int64, encoded string) error {
	query := "UPDATE " + s.quote("tms_user") + " SET " + s.quote("pwd") + " = ?, " + s.quote("updated_time") + " = ? WHERE " + s.quote("id") + " = ?"
	result, err := s.db.ExecContext(ctx, s.bind(query), encoded, time.Now().UnixMilli(), userID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err == nil && rows != 1 {
		return fmt.Errorf("password update affected %d rows", rows)
	}
	return err
}

func (s *Store) ConfigValue(ctx context.Context, name string) (SiteConfig, error) {
	query := "SELECT " + s.columns("id", "name", "value", "time") + " FROM " + s.quote("vite_config") + " WHERE " + s.quote("name") + " = ? LIMIT 1"
	var item SiteConfig
	err := s.db.QueryRowContext(ctx, s.bind(query), name).Scan(&item.ID, &item.Name, &item.Value, &item.Time)
	if err != nil {
		return SiteConfig{}, err
	}
	return item, nil
}

func (s *Store) Configs(ctx context.Context) ([]SiteConfig, error) {
	query := "SELECT " + s.columns("id", "name", "value", "time") + " FROM " + s.quote("vite_config") + " ORDER BY " + s.quote("id")
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]SiteConfig, 0)
	for rows.Next() {
		var item SiteConfig
		if err := rows.Scan(&item.ID, &item.Name, &item.Value, &item.Time); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) UpsertConfigs(ctx context.Context, values map[string]string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := time.Now().UnixMilli()
	for name, value := range values {
		if strings.TrimSpace(name) == "" {
			continue
		}
		update := "UPDATE " + s.quote("vite_config") + " SET " + s.quote("value") + " = ?, " + s.quote("time") + " = ? WHERE " + s.quote("name") + " = ?"
		result, err := tx.ExecContext(ctx, s.bind(update), value, now, name)
		if err != nil {
			return err
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if rows == 0 {
			insert := "INSERT INTO " + s.quote("vite_config") + " (" + s.columns("name", "value", "time") + ") VALUES (?, ?, ?)"
			if _, err := tx.ExecContext(ctx, s.bind(insert), name, value, now); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

func (s *Store) bind(query string) string {
	if s.dialect != database.PostgreSQL {
		return query
	}
	var out strings.Builder
	index := 1
	for _, char := range query {
		if char == '?' {
			out.WriteString(fmt.Sprintf("$%d", index))
			index++
		} else {
			out.WriteRune(char)
		}
	}
	return out.String()
}

func (s *Store) quote(identifier string) string {
	if s.dialect == database.PostgreSQL {
		return `"` + identifier + `"`
	}
	return "`" + identifier + "`"
}

func (s *Store) columns(names ...string) string {
	quoted := make([]string, len(names))
	for i, name := range names {
		quoted[i] = s.quote(name)
	}
	return strings.Join(quoted, ", ")
}

func IsNotFound(err error) bool {
	return errors.Is(err, sql.ErrNoRows)
}
