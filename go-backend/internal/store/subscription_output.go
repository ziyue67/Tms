package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

type SubscriptionEntry struct {
	Protocol    string
	Server      string
	Port        int
	UUID        string
	Password    string
	SNI         string
	PublicKey   string
	ShortID     string
	ConfigJSON  string
	Remark      string
	NodeName    string
	LandingName string
}

type SubscriptionOutput struct {
	UserID       int64
	RoleID       int
	Aggregate    bool
	Usable       bool
	Upload       int64
	Download     int64
	Total        int64
	Expires      int64
	Entries      []SubscriptionEntry
	CustomLinks  []string
	CustomParsed []map[string]any
}

func (s *Store) SubscriptionByToken(ctx context.Context, token string) (SubscriptionOutput, error) {
	if token == "" {
		return SubscriptionOutput{}, nil
	}
	output := SubscriptionOutput{Usable: true, Entries: []SubscriptionEntry{}, CustomLinks: []string{}, CustomParsed: []map[string]any{}}
	var user User
	query := "SELECT " + s.columns("id", "created_time", "updated_time", "status", "user", "email", "pwd", "role_id", "exp_time", "flow", "in_flow", "out_flow", "num", "flow_reset_time", "all_sub_token") + " FROM " + s.quote("tms_user") + " WHERE " + s.quote("all_sub_token") + "=? LIMIT 1"
	err := s.db.QueryRowContext(ctx, s.bind(query), token).Scan(&user.ID, &user.CreatedTime, &user.UpdatedTime, &user.Status,
		&user.Username, &user.Email, &user.Password, &user.RoleID, &user.ExpiryTime, &user.Flow, &user.InboundFlow,
		&user.OutboundFlow, &user.ForwardLimit, &user.FlowResetTime, &user.AllSubToken)
	if err == nil {
		output.Aggregate = true
	} else if !errors.Is(err, sql.ErrNoRows) {
		return output, err
	} else {
		lookup := "SELECT u." + s.quote("id") + ", u." + s.quote("created_time") + ", u." + s.quote("updated_time") + ", u." + s.quote("status") + ", u." + s.quote("user") + ", u." + s.quote("email") + ", u." + s.quote("pwd") + ", u." + s.quote("role_id") + ", u." + s.quote("exp_time") + ", u." + s.quote("flow") + ", u." + s.quote("in_flow") + ", u." + s.quote("out_flow") + ", u." + s.quote("num") + ", u." + s.quote("flow_reset_time") + ", u." + s.quote("all_sub_token") + " FROM " + s.quote("tms_user") + " u JOIN " + s.quote("inbound_user") + " iu ON iu." + s.quote("user_id") + "=u." + s.quote("id") + " WHERE iu." + s.quote("sub_token") + "=? LIMIT 1"
		err = s.db.QueryRowContext(ctx, s.bind(lookup), token).Scan(&user.ID, &user.CreatedTime, &user.UpdatedTime, &user.Status,
			&user.Username, &user.Email, &user.Password, &user.RoleID, &user.ExpiryTime, &user.Flow, &user.InboundFlow,
			&user.OutboundFlow, &user.ForwardLimit, &user.FlowResetTime, &user.AllSubToken)
		if errors.Is(err, sql.ErrNoRows) {
			return output, nil
		}
		if err != nil {
			return output, err
		}
	}
	output.UserID, output.RoleID = user.ID, user.RoleID
	output.Upload, output.Download = max(int64(0), user.OutboundFlow), max(int64(0), user.InboundFlow)
	output.Total = max(int64(0), user.Flow) * 1024 * 1024 * 1024
	if user.ExpiryTime > 0 {
		output.Expires = user.ExpiryTime / 1000
	}
	subscription, err := s.CurrentSubscription(ctx, user.ID, false)
	if err != nil {
		return output, err
	}
	if subscription != nil {
		output.Upload, output.Download, output.Total = 0, max(int64(0), subscription.TrafficUsedBytes), max(int64(0), subscription.TrafficLimitBytes)
		if subscription.ExpiresAt > 0 {
			output.Expires = subscription.ExpiresAt / 1000
		}
		output.Usable = subscription.Status == 1 && (subscription.ExpiresAt <= 0 || subscription.ExpiresAt > time.Now().UnixMilli()) &&
			(subscription.TrafficLimitBytes <= 0 || subscription.TrafficUsedBytes < subscription.TrafficLimitBytes)
	}
	if user.Status != 1 || (user.ExpiryTime > 0 && user.ExpiryTime <= time.Now().UnixMilli()) {
		output.Usable = false
	}
	if !output.Usable {
		return output, nil
	}
	entries, err := s.subscriptionEntries(ctx, user.ID, token, output.Aggregate)
	if err != nil {
		return output, err
	}
	output.Entries = entries
	if output.Aggregate && (user.RoleID == 0 || subscription != nil) {
		output.CustomLinks, output.CustomParsed, err = s.customSubscriptionEntries(ctx, user.ID)
	}
	return output, err
}

func (s *Store) subscriptionEntries(ctx context.Context, userID int64, token string, aggregate bool) ([]SubscriptionEntry, error) {
	query := "SELECT i." + s.quote("protocol") + ", COALESCE(NULLIF(n." + s.quote("domain") + ",''), n." + s.quote("server_ip") + "), f." + s.quote("in_port") + ", COALESCE(iu." + s.quote("uuid") + ",''), COALESCE(iu." + s.quote("password") + ",''), COALESCE(i." + s.quote("sni") + ",''), COALESCE(i." + s.quote("public_key") + ",''), COALESCE(i." + s.quote("short_id") + ",''), COALESCE(i." + s.quote("config_json") + ",''), COALESCE(i." + s.quote("remark") + ",''), n." + s.quote("name") + ", COALESCE(lg." + s.quote("name") + ",''), COALESCE(il." + s.quote("status") + ",1), COALESCE(iu." + s.quote("status") + ",1) FROM " + s.quote("inbound_user") + " iu JOIN " + s.quote("inbound") + " i ON i." + s.quote("id") + "=iu." + s.quote("inbound_id") + " JOIN " + s.quote("node") + " n ON n." + s.quote("id") + "=i." + s.quote("node_id") + " JOIN " + s.quote("forward") + " f ON f." + s.quote("id") + "=iu." + s.quote("gost_forward_id") + " LEFT JOIN " + s.quote("landing") + " lg ON lg." + s.quote("id") + "=i." + s.quote("landing_id") + " LEFT JOIN " + s.quote("inbound_line") + " il ON il." + s.quote("user_id") + "=iu." + s.quote("user_id") + " AND il." + s.quote("node_id") + "=i." + s.quote("node_id") + " AND (il." + s.quote("landing_id") + "=i." + s.quote("landing_id") + " OR (il." + s.quote("landing_id") + " IS NULL AND i." + s.quote("landing_id") + " IS NULL)) WHERE iu." + s.quote("user_id") + "=?"
	args := []any{userID}
	if !aggregate {
		query += " AND iu." + s.quote("sub_token") + "=?"
		args = append(args, token)
	}
	query += " ORDER BY i." + s.quote("node_id") + ", i." + s.quote("landing_id") + ", i." + s.quote("id")
	rows, err := s.db.QueryContext(ctx, s.bind(query), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]SubscriptionEntry, 0)
	for rows.Next() {
		var item SubscriptionEntry
		var lineStatus, userStatus int
		if err := rows.Scan(&item.Protocol, &item.Server, &item.Port, &item.UUID, &item.Password, &item.SNI,
			&item.PublicKey, &item.ShortID, &item.ConfigJSON, &item.Remark, &item.NodeName, &item.LandingName, &lineStatus, &userStatus); err != nil {
			return nil, err
		}
		if lineStatus == 1 && userStatus == 1 {
			result = append(result, item)
		}
	}
	return result, rows.Err()
}

func (s *Store) customSubscriptionEntries(ctx context.Context, userID int64) ([]string, []map[string]any, error) {
	query := "SELECT DISTINCT c." + s.quote("raw_link") + ", c." + s.quote("parsed_json") + ", c." + s.quote("name") + ", c." + s.quote("protocol") + " FROM " + s.quote("custom_node") + " c LEFT JOIN " + s.quote("user_custom_node") + " a ON a." + s.quote("custom_node_id") + "=c." + s.quote("id") + " AND a." + s.quote("user_id") + "=? AND a." + s.quote("status") + "=1 WHERE c." + s.quote("status") + "=1 AND (c." + s.quote("visibility") + " IS NULL OR c." + s.quote("visibility") + " IN ('global','subscribers') OR (c." + s.quote("visibility") + "='users' AND a." + s.quote("id") + " IS NOT NULL)) ORDER BY c." + s.quote("id")
	rows, err := s.db.QueryContext(ctx, s.bind(query), userID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	links := []string{}
	parsed := []map[string]any{}
	for rows.Next() {
		var raw, parsedJSON, name, protocol string
		if err := rows.Scan(&raw, &parsedJSON, &name, &protocol); err != nil {
			return nil, nil, err
		}
		links = append(links, raw)
		value := map[string]any{"name": name, "protocol": protocol}
		_ = json.Unmarshal([]byte(parsedJSON), &value)
		value["name"], value["protocol"] = name, protocol
		parsed = append(parsed, value)
	}
	return links, parsed, rows.Err()
}

func (s *Store) SubscriptionStoreHeader(ctx context.Context, username string, tunnelID *int64) (User, *TunnelPermission, error) {
	user, err := s.UserByLogin(ctx, username)
	if err != nil || tunnelID == nil {
		return user, nil, err
	}
	query := "SELECT ut." + s.quote("id") + ", ut." + s.quote("user_id") + ", ut." + s.quote("tunnel_id") + ", t." + s.quote("name") + ", t." + s.quote("flow") + ", ut." + s.quote("flow") + ", ut." + s.quote("in_flow") + ", ut." + s.quote("out_flow") + ", ut." + s.quote("num") + ", ut." + s.quote("flow_reset_time") + ", ut." + s.quote("exp_time") + ", ut." + s.quote("speed_id") + ", sl." + s.quote("name") + ", sl." + s.quote("speed") + " FROM " + s.quote("user_tunnel") + " ut LEFT JOIN " + s.quote("tunnel") + " t ON t." + s.quote("id") + "=ut." + s.quote("tunnel_id") + " LEFT JOIN " + s.quote("speed_limit") + " sl ON sl." + s.quote("id") + "=ut." + s.quote("speed_id") + " WHERE ut." + s.quote("id") + "=? AND ut." + s.quote("user_id") + "=?"
	var item TunnelPermission
	var tunnelName, speedName sql.NullString
	var tunnelFlow, speed, speedID sql.NullInt64
	err = s.db.QueryRowContext(ctx, s.bind(query), *tunnelID, user.ID).Scan(&item.ID, &item.UserID, &item.TunnelID, &tunnelName, &tunnelFlow, &item.Flow,
		&item.InboundFlow, &item.OutboundFlow, &item.ForwardLimit, &item.FlowResetTime, &item.ExpiryTime, &speedID, &speedName, &speed)
	if err != nil {
		return user, nil, err
	}
	return user, &item, nil
}
