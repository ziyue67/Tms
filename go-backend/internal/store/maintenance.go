package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

func (s *Store) RunSubscriptionMaintenance(ctx context.Context, now time.Time) error {
	query := "SELECT us." + s.quote("id") + ", us." + s.quote("user_id") + ", us." + s.quote("traffic_used_bytes") + ", us." + s.quote("next_reset_at") + ", p." + s.quote("reset_day") + ", p." + s.quote("reset_quota") + " FROM " + s.quote("user_subscription") + " us LEFT JOIN " + s.quote("subscription_plan") + " p ON p." + s.quote("id") + "=us." + s.quote("plan_id") + " WHERE us." + s.quote("status") + "=1 AND us." + s.quote("next_reset_at") + " IS NOT NULL AND us." + s.quote("next_reset_at") + "<=?"
	rows, err := s.db.QueryContext(ctx, s.bind(query), now.UnixMilli())
	if err != nil {
		return err
	}
	type due struct {
		id, userID, used int64
		next             int64
		day, reset       int
	}
	items := []due{}
	for rows.Next() {
		var item due
		var day, reset sql.NullInt64
		if err := rows.Scan(&item.id, &item.userID, &item.used, &item.next, &day, &reset); err != nil {
			rows.Close()
			return err
		}
		item.day = 1
		if day.Valid {
			item.day = int(day.Int64)
		}
		item.reset = 1
		if reset.Valid {
			item.reset = int(reset.Int64)
		}
		items = append(items, item)
	}
	rows.Close()
	for _, item := range items {
		used := item.used
		event, detail := "traffic_period_checkpoint", "quota_preserved"
		if item.reset == 1 {
			used, event, detail = 0, "traffic_reset", "quota_restored"
		}
		next := calculateNextReset(now, item.day)
		if _, err := s.db.ExecContext(ctx, s.bind("UPDATE "+s.quote("user_subscription")+" SET "+s.quote("traffic_used_bytes")+"=?, "+s.quote("next_reset_at")+"=?, "+s.quote("updated_time")+"=? WHERE "+s.quote("id")+"=?"), used, next, now.UnixMilli(), item.id); err != nil {
			return err
		}
		metadata := fmt.Sprintf(`{"detail":"%s"}`, detail)
		if _, err := s.db.ExecContext(ctx, s.bind("INSERT INTO "+s.quote("quota_usage_log")+" ("+s.columns("user_id", "subscription_id", "event_type", "amount", "metadata", "created_time")+") VALUES (?,?,?,0,?,?)"), item.userID, item.id, event, metadata, now.UnixMilli()); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) RunDailyFlowReset(ctx context.Context, now time.Time) error {
	day := now.Day()
	last := time.Date(now.Year(), now.Month()+1, 0, 0, 0, 0, 0, now.Location()).Day()
	condition := s.quote("flow_reset_time") + "=?"
	args := []any{day}
	if day == last {
		condition = "(" + s.quote("flow_reset_time") + "=? OR " + s.quote("flow_reset_time") + ">?)"
		args = append(args, last)
	}
	for _, table := range []string{"user", "user_tunnel"} {
		query := "UPDATE " + s.quote(table) + " SET " + s.quote("in_flow") + "=0, " + s.quote("out_flow") + "=0 WHERE " + s.quote("flow_reset_time") + "<>0 AND " + condition
		if _, err := s.db.ExecContext(ctx, s.bind(query), args...); err != nil {
			return err
		}
	}
	return nil
}

type ExpiredForward struct{ ID, UserID, NodeID int64 }

func (s *Store) ExpireDueRecords(ctx context.Context, now time.Time) ([]ExpiredForward, error) {
	if _, err := s.db.ExecContext(ctx, s.bind("UPDATE "+s.quote("user")+" SET "+s.quote("status")+"=0 WHERE "+s.quote("status")+"=1 AND "+s.quote("exp_time")+">0 AND "+s.quote("exp_time")+"<=?"), now.UnixMilli()); err != nil {
		return nil, err
	}
	if _, err := s.db.ExecContext(ctx, s.bind("UPDATE "+s.quote("user_tunnel")+" SET "+s.quote("status")+"=0 WHERE "+s.quote("status")+"=1 AND "+s.quote("exp_time")+">0 AND "+s.quote("exp_time")+"<=?"), now.UnixMilli()); err != nil {
		return nil, err
	}
	query := "SELECT f." + s.quote("id") + ",f." + s.quote("user_id") + ",t." + s.quote("in_node_id") + " FROM " + s.quote("forward") + " f JOIN " + s.quote("tunnel") + " t ON t." + s.quote("id") + "=f." + s.quote("tunnel_id") + " WHERE f." + s.quote("status") + "=1 AND f." + s.quote("exp_time") + " IS NOT NULL AND f." + s.quote("exp_time") + ">0 AND f." + s.quote("exp_time") + "<=?"
	rows, err := s.db.QueryContext(ctx, s.bind(query), now.UnixMilli())
	if err != nil {
		return nil, err
	}
	items := []ExpiredForward{}
	for rows.Next() {
		var item ExpiredForward
		if err := rows.Scan(&item.ID, &item.UserID, &item.NodeID); err != nil {
			rows.Close()
			return nil, err
		}
		items = append(items, item)
	}
	rows.Close()
	for _, item := range items {
		if _, err := s.db.ExecContext(ctx, s.bind("UPDATE "+s.quote("forward")+" SET "+s.quote("status")+"=0, "+s.quote("updated_time")+"=? WHERE "+s.quote("id")+"=?"), now.UnixMilli(), item.ID); err != nil {
			return nil, err
		}
	}
	return items, nil
}

func (s *Store) RecordHourlyStatistics(ctx context.Context, now time.Time) error {
	cutoff := now.Add(-48 * time.Hour).UnixMilli()
	if _, err := s.db.ExecContext(ctx, s.bind("DELETE FROM "+s.quote("statistics_flow")+" WHERE "+s.quote("created_time")+"<?"), cutoff); err != nil {
		return err
	}
	users, err := s.QueryMaps(ctx, "SELECT id,in_flow,out_flow FROM "+s.quote("user"))
	if err != nil {
		return err
	}
	for _, user := range users {
		id, _ := asInt64(user["id"])
		inFlow, _ := asInt64(user["inFlow"])
		outFlow, _ := asInt64(user["outFlow"])
		total := inFlow + outFlow
		last := int64(0)
		rows, _ := s.QueryMaps(ctx, "SELECT total_flow FROM "+s.quote("statistics_flow")+" WHERE "+s.quote("user_id")+"=? ORDER BY "+s.quote("id")+" DESC LIMIT 1", id)
		if len(rows) > 0 {
			last, _ = asInt64(rows[0]["totalFlow"])
		}
		increment := total - last
		if increment < 0 {
			increment = total
		}
		if _, err := s.InsertMap(ctx, "statistics_flow", map[string]any{"user_id": id, "flow": increment, "total_flow": total, "time": now.Format("15:00"), "created_time": now.UnixMilli()}); err != nil {
			return err
		}
	}
	return nil
}
