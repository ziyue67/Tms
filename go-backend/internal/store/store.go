package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ziyue67/tms/go-backend/internal/database"
)

type User struct {
	ID                            int64          `json:"id"`
	CreatedTime                   int64          `json:"createdTime"`
	UpdatedTime                   sql.NullInt64  `json:"updatedTime"`
	Status                        int            `json:"status"`
	Username                      string         `json:"user"`
	Email                         sql.NullString `json:"-"`
	Password                      string         `json:"-"`
	RoleID                        int            `json:"roleId"`
	ExpiryTime                    int64          `json:"expTime"`
	Flow                          int64          `json:"flow"`
	InboundFlow                   int64          `json:"inFlow"`
	OutboundFlow                  int64          `json:"outFlow"`
	ForwardLimit                  int            `json:"num"`
	FlowResetTime                 int64          `json:"flowResetTime"`
	AllSubToken                   sql.NullString `json:"allSubToken"`
	SubscriptionPlanID            *int64         `json:"subscriptionPlanId,omitempty"`
	SubscriptionPlanName          *string        `json:"subscriptionPlanName,omitempty"`
	SubscriptionTrafficLimitBytes *int64         `json:"subscriptionTrafficLimitBytes,omitempty"`
	SubscriptionTrafficUsedBytes  *int64         `json:"subscriptionTrafficUsedBytes,omitempty"`
	SubscriptionExpiresAt         *int64         `json:"subscriptionExpiresAt,omitempty"`
	SubscriptionMaxForwards       *int           `json:"subscriptionMaxForwards,omitempty"`
}

func (u User) MarshalJSON() ([]byte, error) {
	type userJSON struct {
		ID                            int64   `json:"id"`
		CreatedTime                   int64   `json:"createdTime"`
		UpdatedTime                   *int64  `json:"updatedTime"`
		Status                        int     `json:"status"`
		Username                      string  `json:"user"`
		Email                         *string `json:"email"`
		RoleID                        int     `json:"roleId"`
		ExpiryTime                    int64   `json:"expTime"`
		Flow                          int64   `json:"flow"`
		InboundFlow                   int64   `json:"inFlow"`
		OutboundFlow                  int64   `json:"outFlow"`
		ForwardLimit                  int     `json:"num"`
		FlowResetTime                 int64   `json:"flowResetTime"`
		AllSubToken                   *string `json:"allSubToken"`
		SubscriptionPlanID            *int64  `json:"subscriptionPlanId,omitempty"`
		SubscriptionPlanName          *string `json:"subscriptionPlanName,omitempty"`
		SubscriptionTrafficLimitBytes *int64  `json:"subscriptionTrafficLimitBytes,omitempty"`
		SubscriptionTrafficUsedBytes  *int64  `json:"subscriptionTrafficUsedBytes,omitempty"`
		SubscriptionExpiresAt         *int64  `json:"subscriptionExpiresAt,omitempty"`
		SubscriptionMaxForwards       *int    `json:"subscriptionMaxForwards,omitempty"`
	}
	var updated *int64
	if u.UpdatedTime.Valid {
		updated = &u.UpdatedTime.Int64
	}
	var email *string
	if u.Email.Valid {
		email = &u.Email.String
	}
	var token *string
	if u.AllSubToken.Valid {
		token = &u.AllSubToken.String
	}
	return json.Marshal(userJSON{ID: u.ID, CreatedTime: u.CreatedTime, UpdatedTime: updated, Status: u.Status, Username: u.Username,
		Email: email, RoleID: u.RoleID, ExpiryTime: u.ExpiryTime, Flow: u.Flow, InboundFlow: u.InboundFlow,
		OutboundFlow: u.OutboundFlow, ForwardLimit: u.ForwardLimit, FlowResetTime: u.FlowResetTime, AllSubToken: token,
		SubscriptionPlanID: u.SubscriptionPlanID, SubscriptionPlanName: u.SubscriptionPlanName,
		SubscriptionTrafficLimitBytes: u.SubscriptionTrafficLimitBytes, SubscriptionTrafficUsedBytes: u.SubscriptionTrafficUsedBytes,
		SubscriptionExpiresAt: u.SubscriptionExpiresAt, SubscriptionMaxForwards: u.SubscriptionMaxForwards})
}

type NewUser struct {
	Username string
	Email    string
	Password string
}

type NodeConnection struct {
	ID      int64
	Secret  string
	Status  int
	Version sql.NullString
	HTTP    int
	TLS     int
	SOCKS   int
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
	query := "SELECT " + s.columns("id", "created_time", "updated_time", "status", "user", "email", "pwd", "role_id", "exp_time", "flow", "in_flow", "out_flow", "num", "flow_reset_time", "all_sub_token") +
		" FROM " + s.quote("tms_user") + " WHERE " + userColumn + " = ?"
	args := []any{login}
	if strings.Contains(login, "@") {
		query += " OR LOWER(" + emailColumn + ") = ?"
		args = append(args, strings.ToLower(login))
	}
	query += " LIMIT 1"
	query = s.bind(query)

	var user User
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&user.ID, &user.CreatedTime, &user.UpdatedTime, &user.Status,
		&user.Username, &user.Email, &user.Password, &user.RoleID, &user.ExpiryTime, &user.Flow,
		&user.InboundFlow, &user.OutboundFlow, &user.ForwardLimit, &user.FlowResetTime, &user.AllSubToken)
	if err != nil {
		return User{}, err
	}
	return user, nil
}

func (s *Store) UserByEmail(ctx context.Context, email string) (User, error) {
	query := "SELECT " + s.columns("id", "created_time", "updated_time", "status", "user", "email", "pwd", "role_id", "exp_time", "flow", "in_flow", "out_flow", "num", "flow_reset_time", "all_sub_token") +
		" FROM " + s.quote("tms_user") + " WHERE LOWER(" + s.quote("email") + ") = ? LIMIT 1"
	var user User
	err := s.db.QueryRowContext(ctx, s.bind(query), strings.ToLower(email)).Scan(&user.ID, &user.CreatedTime, &user.UpdatedTime, &user.Status,
		&user.Username, &user.Email, &user.Password, &user.RoleID, &user.ExpiryTime, &user.Flow,
		&user.InboundFlow, &user.OutboundFlow, &user.ForwardLimit, &user.FlowResetTime, &user.AllSubToken)
	return user, err
}

func (s *Store) UsernameExists(ctx context.Context, username string) (bool, error) {
	query := "SELECT 1 FROM " + s.quote("tms_user") + " WHERE " + s.quote("user") + " = ? LIMIT 1"
	var one int
	err := s.db.QueryRowContext(ctx, s.bind(query), username).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

func (s *Store) CountUsersByEmailDomain(ctx context.Context, domain string) (int64, error) {
	query := "SELECT COUNT(*) FROM " + s.quote("tms_user") + " WHERE LOWER(" + s.quote("email") + ") LIKE ?"
	var count int64
	err := s.db.QueryRowContext(ctx, s.bind(query), "%@"+strings.ToLower(domain)).Scan(&count)
	return count, err
}

func (s *Store) CreateUser(ctx context.Context, input NewUser) (User, error) {
	now := time.Now().UnixMilli()
	columns := s.columns("user", "email", "pwd", "role_id", "exp_time", "flow", "in_flow", "out_flow", "flow_reset_time", "num", "created_time", "updated_time", "status")
	values := []any{input.Username, input.Email, input.Password, 1, int64(0), int64(0), int64(0), int64(0), int64(1), 0, now, now, 1}
	query := "INSERT INTO " + s.quote("user") + " (" + columns + ") VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)"
	var id int64
	if s.dialect == database.PostgreSQL {
		query += " RETURNING " + s.quote("id")
		if err := s.db.QueryRowContext(ctx, s.bind(query), values...).Scan(&id); err != nil {
			return User{}, err
		}
	} else {
		result, err := s.db.ExecContext(ctx, query, values...)
		if err != nil {
			return User{}, err
		}
		id, err = result.LastInsertId()
		if err != nil {
			return User{}, err
		}
	}
	return User{ID: id, CreatedTime: now, UpdatedTime: sql.NullInt64{Int64: now, Valid: true}, Status: 1,
		Username: input.Username, Email: sql.NullString{String: input.Email, Valid: true}, Password: input.Password,
		RoleID: 1, FlowResetTime: 1}, nil
}

func (s *Store) NodeBySecret(ctx context.Context, secret string) (NodeConnection, error) {
	query := "SELECT " + s.columns("id", "secret", "status", "version", "http", "tls", "socks") +
		" FROM " + s.quote("node") + " WHERE " + s.quote("secret") + " = ? LIMIT 1"
	var node NodeConnection
	err := s.db.QueryRowContext(ctx, s.bind(query), secret).Scan(&node.ID, &node.Secret, &node.Status, &node.Version, &node.HTTP, &node.TLS, &node.SOCKS)
	return node, err
}

func (s *Store) UpdateNodeConnection(ctx context.Context, nodeID int64, version string, httpPort, tlsPort, socksPort int) error {
	query := "UPDATE " + s.quote("node") + " SET " + s.quote("status") + " = 1, " + s.quote("version") + " = ?, " +
		s.quote("http") + " = ?, " + s.quote("tls") + " = ?, " + s.quote("socks") + " = ? WHERE " + s.quote("id") + " = ?"
	_, err := s.db.ExecContext(ctx, s.bind(query), nullableString(version), httpPort, tlsPort, socksPort, nodeID)
	return err
}

func (s *Store) MarkNodeOffline(ctx context.Context, nodeID int64) (bool, error) {
	query := "UPDATE " + s.quote("node") + " SET " + s.quote("status") + " = 0 WHERE " + s.quote("id") + " = ? AND " + s.quote("status") + " <> 0"
	result, err := s.db.ExecContext(ctx, s.bind(query), nodeID)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows > 0, err
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
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
