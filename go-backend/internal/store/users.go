package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

type ManagedUserInput struct {
	Username      string
	Email         string
	Password      string
	Flow          int64
	ForwardLimit  int
	ExpiryTime    int64
	FlowResetTime int64
	Status        int
}

type CleanupCommand struct {
	InNodeID  int64
	OutNodeID int64
	Name      string
	Tunnel    bool
}

type TunnelPermission struct {
	ID             int64   `json:"id"`
	UserID         int64   `json:"userId"`
	TunnelID       int64   `json:"tunnelId"`
	TunnelName     *string `json:"tunnelName"`
	TunnelFlow     *int    `json:"tunnelFlow"`
	Flow           int64   `json:"flow"`
	InboundFlow    int64   `json:"inFlow"`
	OutboundFlow   int64   `json:"outFlow"`
	ForwardLimit   int     `json:"num"`
	FlowResetTime  int64   `json:"flowResetTime"`
	ExpiryTime     int64   `json:"expTime"`
	SpeedID        *int64  `json:"speedId"`
	SpeedLimitName *string `json:"speedLimitName"`
	Speed          *int    `json:"speed"`
}

type UserForward struct {
	ID           int64   `json:"id"`
	Name         string  `json:"name"`
	TunnelID     int64   `json:"tunnelId"`
	TunnelName   *string `json:"tunnelName"`
	InboundIP    *string `json:"inIp"`
	InboundPort  int     `json:"inPort"`
	RemoteAddr   string  `json:"remoteAddr"`
	InboundFlow  int64   `json:"inFlow"`
	OutboundFlow int64   `json:"outFlow"`
	Status       int     `json:"status"`
	CreatedTime  int64   `json:"createdTime"`
}

type FlowStatistic struct {
	ID          *int64 `json:"id"`
	UserID      int64  `json:"userId"`
	Flow        int64  `json:"flow"`
	TotalFlow   int64  `json:"totalFlow"`
	Time        string `json:"time"`
	CreatedTime *int64 `json:"createdTime"`
}

type UserInfo struct {
	ID                            int64   `json:"id"`
	Name                          *string `json:"name"`
	Username                      string  `json:"user"`
	Email                         *string `json:"email"`
	Status                        int     `json:"status"`
	Flow                          int64   `json:"flow"`
	InboundFlow                   int64   `json:"inFlow"`
	OutboundFlow                  int64   `json:"outFlow"`
	ForwardLimit                  int     `json:"num"`
	ExpiryTime                    int64   `json:"expTime"`
	FlowResetTime                 int64   `json:"flowResetTime"`
	CreatedTime                   int64   `json:"createdTime"`
	UpdatedTime                   *int64  `json:"updatedTime"`
	SubscriptionPlanID            *int64  `json:"subscriptionPlanId"`
	SubscriptionPlanName          *string `json:"subscriptionPlanName"`
	SubscriptionPlanDescription   *string `json:"subscriptionPlanDescription"`
	SubscriptionTrafficLimitBytes *int64  `json:"subscriptionTrafficLimitBytes"`
	SubscriptionTrafficUsedBytes  *int64  `json:"subscriptionTrafficUsedBytes"`
	SubscriptionExpiresAt         *int64  `json:"subscriptionExpiresAt"`
	SubscriptionMaxForwards       *int    `json:"subscriptionMaxForwards"`
	SubscriptionValidityValue     *int    `json:"subscriptionValidityValue"`
	SubscriptionValidityUnit      *string `json:"subscriptionValidityUnit"`
}

type UserPackage struct {
	UserInfo          UserInfo           `json:"userInfo"`
	TunnelPermissions []TunnelPermission `json:"tunnelPermissions"`
	Forwards          []UserForward      `json:"forwards"`
	StatisticsFlows   []FlowStatistic    `json:"statisticsFlows"`
}

func (s *Store) UserByID(ctx context.Context, id int64) (User, error) {
	query := "SELECT " + s.columns("id", "created_time", "updated_time", "status", "user", "email", "pwd", "role_id", "exp_time", "flow", "in_flow", "out_flow", "num", "flow_reset_time", "all_sub_token") +
		" FROM " + s.quote("tms_user") + " WHERE " + s.quote("id") + " = ?"
	var user User
	err := s.db.QueryRowContext(ctx, s.bind(query), id).Scan(&user.ID, &user.CreatedTime, &user.UpdatedTime, &user.Status,
		&user.Username, &user.Email, &user.Password, &user.RoleID, &user.ExpiryTime, &user.Flow,
		&user.InboundFlow, &user.OutboundFlow, &user.ForwardLimit, &user.FlowResetTime, &user.AllSubToken)
	return user, err
}

func (s *Store) Users(ctx context.Context) ([]User, error) {
	query := "SELECT " + s.columns("id", "created_time", "updated_time", "status", "user", "email", "pwd", "role_id", "exp_time", "flow", "in_flow", "out_flow", "num", "flow_reset_time", "all_sub_token") +
		" FROM " + s.quote("tms_user") + " WHERE " + s.quote("role_id") + " <> 0 ORDER BY " + s.quote("id")
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	users := make([]User, 0)
	for rows.Next() {
		var user User
		if err := rows.Scan(&user.ID, &user.CreatedTime, &user.UpdatedTime, &user.Status, &user.Username, &user.Email,
			&user.Password, &user.RoleID, &user.ExpiryTime, &user.Flow, &user.InboundFlow, &user.OutboundFlow,
			&user.ForwardLimit, &user.FlowResetTime, &user.AllSubToken); err != nil {
			return nil, err
		}
		if err := s.attachSubscription(ctx, &user); err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, rows.Err()
}

func (s *Store) UsernameExistsExcept(ctx context.Context, username string, excludedID int64) (bool, error) {
	query := "SELECT 1 FROM " + s.quote("tms_user") + " WHERE " + s.quote("user") + " = ? AND " + s.quote("id") + " <> ? LIMIT 1"
	var one int
	err := s.db.QueryRowContext(ctx, s.bind(query), username, excludedID).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

func (s *Store) CreateManagedUser(ctx context.Context, input ManagedUserInput) error {
	now := time.Now().UnixMilli()
	query := "INSERT INTO " + s.quote("user") + " (" + s.columns("user", "email", "pwd", "role_id", "exp_time", "flow", "in_flow", "out_flow", "flow_reset_time", "num", "created_time", "updated_time", "status") + ") VALUES (?, ?, ?, 1, ?, ?, 0, 0, ?, ?, ?, ?, ?)"
	_, err := s.db.ExecContext(ctx, s.bind(query), input.Username, nullableString(input.Email), input.Password, input.ExpiryTime,
		input.Flow, input.FlowResetTime, input.ForwardLimit, now, now, input.Status)
	return err
}

func (s *Store) UpdateManagedUser(ctx context.Context, id int64, input ManagedUserInput) error {
	assignments := []string{s.quote("user") + " = ?", s.quote("email") + " = ?", s.quote("flow") + " = ?",
		s.quote("num") + " = ?", s.quote("exp_time") + " = ?", s.quote("flow_reset_time") + " = ?",
		s.quote("status") + " = ?", s.quote("updated_time") + " = ?"}
	args := []any{input.Username, nullableString(input.Email), input.Flow, input.ForwardLimit, input.ExpiryTime, input.FlowResetTime, input.Status, time.Now().UnixMilli()}
	if input.Password != "" {
		assignments = append(assignments, s.quote("pwd")+" = ?")
		args = append(args, input.Password)
	}
	args = append(args, id)
	query := "UPDATE " + s.quote("user") + " SET " + strings.Join(assignments, ", ") + " WHERE " + s.quote("id") + " = ?"
	result, err := s.db.ExecContext(ctx, s.bind(query), args...)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil || rows != 1 {
		return errors.New("用户更新失败")
	}
	return nil
}

func (s *Store) UpdateUsernamePassword(ctx context.Context, id int64, username, password string) error {
	query := "UPDATE " + s.quote("user") + " SET " + s.quote("user") + " = ?, " + s.quote("pwd") + " = ?, " + s.quote("updated_time") + " = ? WHERE " + s.quote("id") + " = ?"
	result, err := s.db.ExecContext(ctx, s.bind(query), username, password, time.Now().UnixMilli(), id)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil || rows != 1 {
		return errors.New("用户更新失败")
	}
	return nil
}

func (s *Store) ResetUserFlow(ctx context.Context, id int64) (bool, error) {
	return s.resetFlow(ctx, "user", id)
}

func (s *Store) ResetUserTunnelFlow(ctx context.Context, id int64) (bool, error) {
	return s.resetFlow(ctx, "user_tunnel", id)
}

func (s *Store) resetFlow(ctx context.Context, table string, id int64) (bool, error) {
	query := "UPDATE " + s.quote(table) + " SET " + s.quote("in_flow") + " = 0, " + s.quote("out_flow") + " = 0 WHERE " + s.quote("id") + " = ?"
	result, err := s.db.ExecContext(ctx, s.bind(query), id)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows == 1, err
}

func (s *Store) UserCleanupCommands(ctx context.Context, userID int64) ([]CleanupCommand, error) {
	query := "SELECT f." + s.quote("id") + ", ut." + s.quote("id") + ", t." + s.quote("type") + ", t." + s.quote("in_node_id") + ", t." + s.quote("out_node_id") +
		" FROM " + s.quote("forward") + " f JOIN " + s.quote("tunnel") + " t ON f." + s.quote("tunnel_id") + " = t." + s.quote("id") +
		" LEFT JOIN " + s.quote("user_tunnel") + " ut ON ut." + s.quote("user_id") + " = f." + s.quote("user_id") + " AND ut." + s.quote("tunnel_id") + " = f." + s.quote("tunnel_id") +
		" WHERE f." + s.quote("user_id") + " = ?"
	rows, err := s.db.QueryContext(ctx, s.bind(query), userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []CleanupCommand
	for rows.Next() {
		var forwardID, inNodeID, outNodeID int64
		var userTunnelID sql.NullInt64
		var tunnelType int
		if err := rows.Scan(&forwardID, &userTunnelID, &tunnelType, &inNodeID, &outNodeID); err != nil {
			return nil, err
		}
		if !userTunnelID.Valid {
			continue
		}
		result = append(result, CleanupCommand{InNodeID: inNodeID, OutNodeID: outNodeID,
			Name: fmt.Sprintf("%d_%d_%d", forwardID, userID, userTunnelID.Int64), Tunnel: tunnelType == 2})
	}
	return result, rows.Err()
}

func (s *Store) DeleteUserCascade(ctx context.Context, userID int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, table := range []string{"forward", "user_tunnel", "inbound_user", "inbound_line", "user_subscription", "quota_usage_log", "payment_order", "user_custom_node", "statistics_flow"} {
		if _, err := tx.ExecContext(ctx, s.bind("DELETE FROM "+s.quote(table)+" WHERE "+s.quote("user_id")+" = ?"), userID); err != nil {
			return err
		}
	}
	result, err := tx.ExecContext(ctx, s.bind("DELETE FROM "+s.quote("user")+" WHERE "+s.quote("id")+" = ?"), userID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil || rows != 1 {
		return errors.New("用户删除失败")
	}
	return tx.Commit()
}

func (s *Store) UserPackage(ctx context.Context, userID int64) (UserPackage, error) {
	user, err := s.UserByID(ctx, userID)
	if err != nil {
		return UserPackage{}, err
	}
	if err := s.attachSubscription(ctx, &user); err != nil {
		return UserPackage{}, err
	}
	permissions, err := s.userTunnelPermissions(ctx, userID)
	if err != nil {
		return UserPackage{}, err
	}
	forwards, err := s.userForwards(ctx, userID)
	if err != nil {
		return UserPackage{}, err
	}
	statistics, err := s.userStatistics(ctx, userID)
	if err != nil {
		return UserPackage{}, err
	}
	info := UserInfo{ID: user.ID, Username: user.Username, Email: nullString(user.Email), Status: user.Status, Flow: user.Flow,
		InboundFlow: user.InboundFlow, OutboundFlow: user.OutboundFlow, ForwardLimit: user.ForwardLimit,
		ExpiryTime: user.ExpiryTime, FlowResetTime: user.FlowResetTime, CreatedTime: user.CreatedTime,
		UpdatedTime: nullInt(user.UpdatedTime), SubscriptionPlanID: user.SubscriptionPlanID,
		SubscriptionPlanName: user.SubscriptionPlanName, SubscriptionTrafficLimitBytes: user.SubscriptionTrafficLimitBytes,
		SubscriptionTrafficUsedBytes: user.SubscriptionTrafficUsedBytes, SubscriptionExpiresAt: user.SubscriptionExpiresAt,
		SubscriptionMaxForwards: user.SubscriptionMaxForwards}
	if user.SubscriptionPlanID != nil {
		query := "SELECT " + s.columns("description", "validity_value", "validity_unit") + " FROM " + s.quote("subscription_plan") + " WHERE " + s.quote("id") + " = ?"
		var description sql.NullString
		var validityValue int
		var validityUnit string
		if err := s.db.QueryRowContext(ctx, s.bind(query), *user.SubscriptionPlanID).Scan(&description, &validityValue, &validityUnit); err == nil {
			info.SubscriptionPlanDescription = nullString(description)
			info.SubscriptionValidityValue = &validityValue
			info.SubscriptionValidityUnit = &validityUnit
		}
	}
	return UserPackage{UserInfo: info, TunnelPermissions: permissions, Forwards: forwards, StatisticsFlows: statistics}, nil
}

func (s *Store) RecordTrafficUsage(ctx context.Context, forwardID, userID, userTunnelID int64, upload, download int64) error {
	if upload < 0 {
		upload = 0
	}
	if download < 0 {
		download = 0
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, s.bind("UPDATE "+s.quote("forward")+" SET "+s.quote("in_flow")+" = "+s.quote("in_flow")+" + ?, "+s.quote("out_flow")+" = "+s.quote("out_flow")+" + ? WHERE "+s.quote("id")+" = ?"), download, upload, forwardID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, s.bind("UPDATE "+s.quote("user")+" SET "+s.quote("in_flow")+" = "+s.quote("in_flow")+" + ?, "+s.quote("out_flow")+" = "+s.quote("out_flow")+" + ? WHERE "+s.quote("id")+" = ?"), download, upload, userID); err != nil {
		return err
	}
	if userTunnelID > 0 {
		if _, err := tx.ExecContext(ctx, s.bind("UPDATE "+s.quote("user_tunnel")+" SET "+s.quote("in_flow")+" = "+s.quote("in_flow")+" + ?, "+s.quote("out_flow")+" = "+s.quote("out_flow")+" + ? WHERE "+s.quote("id")+" = ?"), download, upload, userTunnelID); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, s.bind("UPDATE "+s.quote("user_subscription")+" SET "+s.quote("traffic_used_bytes")+" = "+s.quote("traffic_used_bytes")+" + ?, "+s.quote("updated_time")+" = ? WHERE "+s.quote("user_id")+" = ? AND "+s.quote("status")+" = 1"), upload+download, time.Now().UnixMilli(), userID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) attachSubscription(ctx context.Context, user *User) error {
	query := "SELECT us." + s.quote("plan_id") + ", p." + s.quote("name") + ", us." + s.quote("traffic_limit_bytes") +
		", us." + s.quote("traffic_used_bytes") + ", us." + s.quote("expires_at") + ", us." + s.quote("max_forwards") + ", p." + s.quote("reset_day") +
		" FROM " + s.quote("user_subscription") + " us LEFT JOIN " + s.quote("subscription_plan") + " p ON p." + s.quote("id") + " = us." + s.quote("plan_id") +
		" WHERE us." + s.quote("user_id") + " = ? ORDER BY us." + s.quote("id") + " DESC LIMIT 1"
	var planID, limit, used, expires int64
	var maxForwards, resetDay int
	var planName sql.NullString
	err := s.db.QueryRowContext(ctx, s.bind(query), user.ID).Scan(&planID, &planName, &limit, &used, &expires, &maxForwards, &resetDay)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	user.SubscriptionPlanID, user.SubscriptionPlanName = &planID, nullString(planName)
	user.SubscriptionTrafficLimitBytes, user.SubscriptionTrafficUsedBytes = &limit, &used
	user.SubscriptionExpiresAt, user.SubscriptionMaxForwards = &expires, &maxForwards
	if limit == 0 {
		user.Flow = 99999
	} else {
		user.Flow = max(1, int64(math.Ceil(float64(limit)/1073741824)))
	}
	user.InboundFlow, user.OutboundFlow = used, 0
	if maxForwards == 0 {
		user.ForwardLimit = 99999
	} else {
		user.ForwardLimit = maxForwards
	}
	user.ExpiryTime, user.FlowResetTime = expires, int64(resetDay)
	return nil
}

func (s *Store) userTunnelPermissions(ctx context.Context, userID int64) ([]TunnelPermission, error) {
	query := "SELECT ut." + s.quote("id") + ", ut." + s.quote("user_id") + ", ut." + s.quote("tunnel_id") + ", t." + s.quote("name") +
		", t." + s.quote("flow") + ", ut." + s.quote("flow") + ", ut." + s.quote("in_flow") + ", ut." + s.quote("out_flow") +
		", ut." + s.quote("num") + ", ut." + s.quote("flow_reset_time") + ", ut." + s.quote("exp_time") + ", ut." + s.quote("speed_id") +
		", sl." + s.quote("name") + ", sl." + s.quote("speed") + " FROM " + s.quote("user_tunnel") + " ut LEFT JOIN " + s.quote("tunnel") +
		" t ON ut." + s.quote("tunnel_id") + " = t." + s.quote("id") + " LEFT JOIN " + s.quote("speed_limit") + " sl ON ut." + s.quote("speed_id") +
		" = sl." + s.quote("id") + " WHERE ut." + s.quote("user_id") + " = ? ORDER BY ut." + s.quote("id")
	rows, err := s.db.QueryContext(ctx, s.bind(query), userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]TunnelPermission, 0)
	for rows.Next() {
		var item TunnelPermission
		var tunnelName, speedName sql.NullString
		var tunnelFlow, speed sql.NullInt64
		var speedID sql.NullInt64
		if err := rows.Scan(&item.ID, &item.UserID, &item.TunnelID, &tunnelName, &tunnelFlow, &item.Flow, &item.InboundFlow,
			&item.OutboundFlow, &item.ForwardLimit, &item.FlowResetTime, &item.ExpiryTime, &speedID, &speedName, &speed); err != nil {
			return nil, err
		}
		item.TunnelName, item.SpeedLimitName = nullString(tunnelName), nullString(speedName)
		item.TunnelFlow, item.Speed, item.SpeedID = nullIntAsInt(tunnelFlow), nullIntAsInt(speed), nullInt(speedID)
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) userForwards(ctx context.Context, userID int64) ([]UserForward, error) {
	query := "SELECT f." + s.quote("id") + ", f." + s.quote("name") + ", f." + s.quote("tunnel_id") + ", t." + s.quote("name") +
		", t." + s.quote("in_ip") + ", f." + s.quote("in_port") + ", f." + s.quote("remote_addr") + ", f." + s.quote("in_flow") +
		", f." + s.quote("out_flow") + ", f." + s.quote("status") + ", f." + s.quote("created_time") + " FROM " + s.quote("forward") +
		" f LEFT JOIN " + s.quote("tunnel") + " t ON f." + s.quote("tunnel_id") + " = t." + s.quote("id") +
		" WHERE f." + s.quote("user_id") + " = ? ORDER BY f." + s.quote("created_time") + " DESC"
	rows, err := s.db.QueryContext(ctx, s.bind(query), userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]UserForward, 0)
	for rows.Next() {
		var item UserForward
		var tunnelName, inboundIP sql.NullString
		if err := rows.Scan(&item.ID, &item.Name, &item.TunnelID, &tunnelName, &inboundIP, &item.InboundPort,
			&item.RemoteAddr, &item.InboundFlow, &item.OutboundFlow, &item.Status, &item.CreatedTime); err != nil {
			return nil, err
		}
		item.TunnelName, item.InboundIP = nullString(tunnelName), nullString(inboundIP)
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) userStatistics(ctx context.Context, userID int64) ([]FlowStatistic, error) {
	query := "SELECT " + s.columns("id", "user_id", "flow", "total_flow", "time", "created_time") + " FROM " + s.quote("statistics_flow") +
		" WHERE " + s.quote("user_id") + " = ? ORDER BY " + s.quote("id") + " DESC LIMIT 24"
	rows, err := s.db.QueryContext(ctx, s.bind(query), userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]FlowStatistic, 0, 24)
	for rows.Next() {
		var item FlowStatistic
		var id, created int64
		if err := rows.Scan(&id, &item.UserID, &item.Flow, &item.TotalFlow, &item.Time, &created); err != nil {
			return nil, err
		}
		item.ID, item.CreatedTime = &id, &created
		result = append(result, item)
	}
	startHour := time.Now().Hour()
	if len(result) > 0 {
		if parsed, err := strconv.Atoi(strings.Split(result[len(result)-1].Time, ":")[0]); err == nil {
			startHour = parsed - 1
		}
	}
	for len(result) < 24 {
		if startHour < 0 {
			startHour = 23
		}
		result = append(result, FlowStatistic{UserID: userID, Time: fmt.Sprintf("%02d:00", startHour)})
		startHour--
	}
	return result, rows.Err()
}

func nullIntAsInt(value sql.NullInt64) *int {
	if !value.Valid {
		return nil
	}
	converted := int(value.Int64)
	return &converted
}
