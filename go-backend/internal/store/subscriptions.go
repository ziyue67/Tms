package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ziyue67/tms/go-backend/internal/database"
)

type SubscriptionPlan struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	Description   *string `json:"description"`
	Price         float64 `json:"price"`
	Currency      string  `json:"currency"`
	ValidityValue int     `json:"validityValue"`
	ValidityUnit  string  `json:"validityUnit"`
	TrafficBytes  int64   `json:"trafficBytes"`
	ResetDay      int     `json:"resetDay"`
	ResetQuota    int     `json:"resetQuota"`
	MaxForwards   int     `json:"maxForwards"`
	ForSale       int     `json:"forSale"`
	Redeemable    int     `json:"redeemable"`
	SortOrder     int     `json:"sortOrder"`
	Status        int     `json:"status"`
	CreatedTime   int64   `json:"createdTime"`
	UpdatedTime   int64   `json:"updatedTime"`
	id            int64
}

type PlanInput struct {
	Name          string
	Description   *string
	Price         float64
	Currency      string
	ValidityValue int
	ValidityUnit  string
	TrafficBytes  int64
	ResetDay      int
	ResetQuota    int
	MaxForwards   int
	ForSale       int
	Redeemable    int
	SortOrder     int
	Status        int
}

type UserSubscription struct {
	ID                string  `json:"id"`
	UserID            string  `json:"userId"`
	PlanID            string  `json:"planId"`
	StartsAt          int64   `json:"startsAt"`
	ExpiresAt         int64   `json:"expiresAt"`
	TrafficLimitBytes int64   `json:"trafficLimitBytes"`
	TrafficUsedBytes  int64   `json:"trafficUsedBytes"`
	NextResetAt       *int64  `json:"nextResetAt"`
	MaxForwards       int     `json:"maxForwards"`
	UsedForwards      int     `json:"usedForwards"`
	Status            int     `json:"status"`
	CreatedTime       int64   `json:"createdTime"`
	UpdatedTime       int64   `json:"updatedTime"`
	PlanName          *string `json:"planName"`
	PlanDescription   *string `json:"planDescription"`
	PlanValidityValue *int    `json:"planValidityValue"`
	PlanValidityUnit  *string `json:"planValidityUnit"`
	id                int64
	userID            int64
	planID            int64
}

type RedeemCode struct {
	ID          string  `json:"id"`
	PlanID      string  `json:"planId"`
	CodeHash    string  `json:"codeHash"`
	CodeValue   *string `json:"codeValue"`
	CodePreview string  `json:"codePreview"`
	BatchID     *string `json:"batchId"`
	Status      int     `json:"status"`
	UsedBy      *int64  `json:"usedBy"`
	UsedTime    *int64  `json:"usedTime"`
	ExpiresAt   *int64  `json:"expiresAt"`
	Remark      *string `json:"remark"`
	CreatedTime int64   `json:"createdTime"`
}

type QuotaLog struct {
	ID             int64           `json:"id"`
	UserID         int64           `json:"userId"`
	SubscriptionID *int64          `json:"subscriptionId"`
	EventType      string          `json:"eventType"`
	Amount         int64           `json:"amount"`
	Metadata       json.RawMessage `json:"metadata"`
	CreatedTime    int64           `json:"createdTime"`
}

func (s *Store) SubscriptionPlans(ctx context.Context, publicOnly bool) ([]SubscriptionPlan, error) {
	query := "SELECT " + s.columns("id", "name", "description", "price", "currency", "validity_value", "validity_unit", "traffic_bytes", "reset_day", "reset_quota", "max_forwards", "for_sale", "redeemable", "sort_order", "status", "created_time", "updated_time") + " FROM " + s.quote("subscription_plan")
	if publicOnly {
		query += " WHERE (" + s.quote("status") + " IS NULL OR " + s.quote("status") + " = 1) AND (" + s.quote("for_sale") + " IS NULL OR " + s.quote("for_sale") + " = 1)"
	}
	query += " ORDER BY " + s.quote("sort_order") + ", " + s.quote("id")
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]SubscriptionPlan, 0)
	for rows.Next() {
		var plan SubscriptionPlan
		if err := scanPlan(rows, &plan); err != nil {
			return nil, err
		}
		result = append(result, plan)
	}
	return result, rows.Err()
}

func (s *Store) SubscriptionPlanByID(ctx context.Context, id int64) (SubscriptionPlan, error) {
	query := "SELECT " + s.columns("id", "name", "description", "price", "currency", "validity_value", "validity_unit", "traffic_bytes", "reset_day", "reset_quota", "max_forwards", "for_sale", "redeemable", "sort_order", "status", "created_time", "updated_time") + " FROM " + s.quote("subscription_plan") + " WHERE " + s.quote("id") + " = ?"
	var plan SubscriptionPlan
	err := scanPlan(s.db.QueryRowContext(ctx, s.bind(query), id), &plan)
	return plan, err
}

func scanPlan(row scanner, plan *SubscriptionPlan) error {
	var description sql.NullString
	var id int64
	err := row.Scan(&id, &plan.Name, &description, &plan.Price, &plan.Currency, &plan.ValidityValue, &plan.ValidityUnit,
		&plan.TrafficBytes, &plan.ResetDay, &plan.ResetQuota, &plan.MaxForwards, &plan.ForSale, &plan.Redeemable,
		&plan.SortOrder, &plan.Status, &plan.CreatedTime, &plan.UpdatedTime)
	plan.id, plan.ID, plan.Description = id, strconv.FormatInt(id, 10), nullString(description)
	return err
}

func (s *Store) CreateSubscriptionPlan(ctx context.Context, input PlanInput) (SubscriptionPlan, error) {
	now := time.Now().UnixMilli()
	query := "INSERT INTO " + s.quote("subscription_plan") + " (" + s.columns("name", "description", "price", "currency", "validity_value", "validity_unit", "traffic_bytes", "reset_day", "reset_quota", "max_forwards", "for_sale", "redeemable", "sort_order", "status", "created_time", "updated_time") + ") VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)"
	args := []any{input.Name, nullableStringPointer(input.Description), input.Price, input.Currency, input.ValidityValue, input.ValidityUnit,
		input.TrafficBytes, input.ResetDay, input.ResetQuota, input.MaxForwards, input.ForSale, input.Redeemable, input.SortOrder, input.Status, now, now}
	var id int64
	if s.dialect == database.PostgreSQL {
		if err := s.db.QueryRowContext(ctx, s.bind(query+" RETURNING "+s.quote("id")), args...).Scan(&id); err != nil {
			return SubscriptionPlan{}, err
		}
	} else {
		result, err := s.db.ExecContext(ctx, query, args...)
		if err != nil {
			return SubscriptionPlan{}, err
		}
		id, err = result.LastInsertId()
		if err != nil {
			return SubscriptionPlan{}, err
		}
	}
	return s.SubscriptionPlanByID(ctx, id)
}

func (s *Store) UpdateSubscriptionPlan(ctx context.Context, id int64, input PlanInput) (SubscriptionPlan, error) {
	query := "UPDATE " + s.quote("subscription_plan") + " SET " + s.quote("name") + "=?, " + s.quote("description") + "=?, " + s.quote("price") + "=?, " + s.quote("currency") + "=?, " + s.quote("validity_value") + "=?, " + s.quote("validity_unit") + "=?, " + s.quote("traffic_bytes") + "=?, " + s.quote("reset_day") + "=?, " + s.quote("reset_quota") + "=?, " + s.quote("max_forwards") + "=?, " + s.quote("for_sale") + "=?, " + s.quote("redeemable") + "=?, " + s.quote("sort_order") + "=?, " + s.quote("status") + "=?, " + s.quote("updated_time") + "=? WHERE " + s.quote("id") + "=?"
	result, err := s.db.ExecContext(ctx, s.bind(query), input.Name, nullableStringPointer(input.Description), input.Price, input.Currency,
		input.ValidityValue, input.ValidityUnit, input.TrafficBytes, input.ResetDay, input.ResetQuota, input.MaxForwards,
		input.ForSale, input.Redeemable, input.SortOrder, input.Status, time.Now().UnixMilli(), id)
	if err != nil {
		return SubscriptionPlan{}, err
	}
	rows, err := result.RowsAffected()
	if err != nil || rows != 1 {
		return SubscriptionPlan{}, errors.New("更新套餐失败")
	}
	return s.SubscriptionPlanByID(ctx, id)
}

func (s *Store) DisableSubscriptionPlan(ctx context.Context, id int64) (SubscriptionPlan, error) {
	query := "UPDATE " + s.quote("subscription_plan") + " SET " + s.quote("status") + "=0, " + s.quote("for_sale") + "=0, " + s.quote("redeemable") + "=0, " + s.quote("updated_time") + "=? WHERE " + s.quote("id") + "=?"
	result, err := s.db.ExecContext(ctx, s.bind(query), time.Now().UnixMilli(), id)
	if err != nil {
		return SubscriptionPlan{}, err
	}
	rows, _ := result.RowsAffected()
	if rows != 1 {
		return SubscriptionPlan{}, sql.ErrNoRows
	}
	return s.SubscriptionPlanByID(ctx, id)
}

func (s *Store) DeleteSubscriptionPlan(ctx context.Context, id int64) (bool, error) {
	result, err := s.db.ExecContext(ctx, s.bind("DELETE FROM "+s.quote("subscription_plan")+" WHERE "+s.quote("id")+"=?"), id)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows == 1, err
}

func (s *Store) RedeemCodes(ctx context.Context, planID *int64, status *int) ([]RedeemCode, error) {
	query := "SELECT " + s.columns("id", "plan_id", "code_hash", "code_value", "code_preview", "batch_id", "status", "used_by", "used_time", "expires_at", "remark", "created_time") + " FROM " + s.quote("redeem_code") + " WHERE 1=1"
	var args []any
	if planID != nil {
		query += " AND " + s.quote("plan_id") + "=?"
		args = append(args, *planID)
	}
	if status != nil {
		query += " AND " + s.quote("status") + "=?"
		args = append(args, *status)
	}
	query += " ORDER BY " + s.quote("id") + " DESC"
	rows, err := s.db.QueryContext(ctx, s.bind(query), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]RedeemCode, 0)
	for rows.Next() {
		var item RedeemCode
		var id, plan int64
		var codeValue, batch, remark sql.NullString
		var usedBy, usedTime, expires sql.NullInt64
		if err := rows.Scan(&id, &plan, &item.CodeHash, &codeValue, &item.CodePreview, &batch, &item.Status, &usedBy, &usedTime, &expires, &remark, &item.CreatedTime); err != nil {
			return nil, err
		}
		item.ID, item.PlanID = strconv.FormatInt(id, 10), strconv.FormatInt(plan, 10)
		item.CodeValue, item.BatchID, item.Remark = nullString(codeValue), nullString(batch), nullString(remark)
		item.UsedBy, item.UsedTime, item.ExpiresAt = nullInt(usedBy), nullInt(usedTime), nullInt(expires)
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) GenerateRedeemCodes(ctx context.Context, planID int64, batch string, count int) ([]string, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	result := make([]string, 0, count)
	query := "INSERT INTO " + s.quote("redeem_code") + " (" + s.columns("plan_id", "code_hash", "code_value", "code_preview", "batch_id", "status", "created_time") + ") VALUES (?, ?, ?, ?, ?, 1, ?)"
	for range count {
		raw, err := randomRedeemCode()
		if err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, s.bind(query), planID, redeemHash(raw), raw, raw[:4]+"****", batch, time.Now().UnixMilli()); err != nil {
			return nil, err
		}
		result = append(result, raw)
	}
	return result, tx.Commit()
}

func (s *Store) RevokeRedeemCode(ctx context.Context, id int64) error {
	var status int
	if err := s.db.QueryRowContext(ctx, s.bind("SELECT "+s.quote("status")+" FROM "+s.quote("redeem_code")+" WHERE "+s.quote("id")+"=?"), id).Scan(&status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("兑换码不存在")
		}
		return err
	}
	if status == 0 {
		return errors.New("已使用的兑换码不能作废")
	}
	_, err := s.db.ExecContext(ctx, s.bind("UPDATE "+s.quote("redeem_code")+" SET "+s.quote("status")+"=-1 WHERE "+s.quote("id")+"=?"), id)
	return err
}

func (s *Store) DeleteRedeemCode(ctx context.Context, id int64) error {
	var status int
	if err := s.db.QueryRowContext(ctx, s.bind("SELECT "+s.quote("status")+" FROM "+s.quote("redeem_code")+" WHERE "+s.quote("id")+"=?"), id).Scan(&status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("兑换码不存在")
		}
		return err
	}
	if status == 1 {
		return errors.New("未使用的兑换码请先作废")
	}
	result, err := s.db.ExecContext(ctx, s.bind("DELETE FROM "+s.quote("redeem_code")+" WHERE "+s.quote("id")+"=?"), id)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows != 1 {
		return errors.New("删除兑换码记录失败")
	}
	return nil
}

func (s *Store) CurrentSubscription(ctx context.Context, userID int64, activeOnly bool) (*UserSubscription, error) {
	userSubscriptionColumns := []string{"id", "user_id", "plan_id", "starts_at", "expires_at", "traffic_limit_bytes", "traffic_used_bytes", "next_reset_at", "max_forwards", "used_forwards", "status", "created_time", "updated_time"}
	prefixedColumns := make([]string, len(userSubscriptionColumns))
	for index, column := range userSubscriptionColumns {
		prefixedColumns[index] = "us." + s.quote(column)
	}
	query := "SELECT " + strings.Join(prefixedColumns, ", ") +
		", p." + s.quote("name") + ", p." + s.quote("description") + ", p." + s.quote("validity_value") + ", p." + s.quote("validity_unit") +
		" FROM " + s.quote("user_subscription") + " us LEFT JOIN " + s.quote("subscription_plan") + " p ON p." + s.quote("id") + "=us." + s.quote("plan_id") + " WHERE us." + s.quote("user_id") + "=?"
	if activeOnly {
		query += " AND us." + s.quote("status") + "=1"
	}
	query += " ORDER BY us." + s.quote("id") + " DESC LIMIT 1"
	var item UserSubscription
	var id, user, plan int64
	var nextReset sql.NullInt64
	var name, description, unit sql.NullString
	var validity sql.NullInt64
	err := s.db.QueryRowContext(ctx, s.bind(query), userID).Scan(&id, &user, &plan, &item.StartsAt, &item.ExpiresAt,
		&item.TrafficLimitBytes, &item.TrafficUsedBytes, &nextReset, &item.MaxForwards, &item.UsedForwards,
		&item.Status, &item.CreatedTime, &item.UpdatedTime, &name, &description, &validity, &unit)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	item.id, item.userID, item.planID = id, user, plan
	item.ID, item.UserID, item.PlanID = strconv.FormatInt(id, 10), strconv.FormatInt(user, 10), strconv.FormatInt(plan, 10)
	item.NextResetAt, item.PlanName, item.PlanDescription = nullInt(nextReset), nullString(name), nullString(description)
	item.PlanValidityValue, item.PlanValidityUnit = nullIntAsInt(validity), nullString(unit)
	return &item, nil
}

func (s *Store) SubscriptionAudit(ctx context.Context, userID int64) ([]QuotaLog, error) {
	query := "SELECT " + s.columns("id", "user_id", "subscription_id", "event_type", "amount", "metadata", "created_time") + " FROM " + s.quote("quota_usage_log") + " WHERE " + s.quote("user_id") + "=? ORDER BY " + s.quote("id") + " DESC LIMIT 100"
	rows, err := s.db.QueryContext(ctx, s.bind(query), userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]QuotaLog, 0)
	for rows.Next() {
		var item QuotaLog
		var subscriptionID sql.NullInt64
		var metadata []byte
		if err := rows.Scan(&item.ID, &item.UserID, &subscriptionID, &item.EventType, &item.Amount, &metadata, &item.CreatedTime); err != nil {
			return nil, err
		}
		item.SubscriptionID = nullInt(subscriptionID)
		if len(metadata) > 0 {
			item.Metadata = json.RawMessage(metadata)
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) AdjustSubscription(ctx context.Context, userID int64, values map[string]any) (*UserSubscription, error) {
	item, err := s.CurrentSubscription(ctx, userID, false)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, errors.New("该用户没有套餐记录，请先分配套餐")
	}
	assignments := []string{}
	args := []any{}
	allowed := map[string]string{"planId": "plan_id", "expiresAt": "expires_at", "trafficLimitBytes": "traffic_limit_bytes", "trafficUsedBytes": "traffic_used_bytes", "nextResetAt": "next_reset_at", "maxForwards": "max_forwards", "status": "status"}
	for inputName, column := range allowed {
		if value, exists := values[inputName]; exists {
			number, err := anyInt64(value)
			if err != nil {
				return nil, errors.New("套餐调整参数格式错误")
			}
			if inputName == "planId" {
				if _, err := s.SubscriptionPlanByID(ctx, number); err != nil {
					return nil, errors.New("套餐不存在")
				}
			}
			assignments, args = append(assignments, s.quote(column)+"=?"), append(args, number)
		}
	}
	assignments, args = append(assignments, s.quote("updated_time")+"=?"), append(args, time.Now().UnixMilli(), item.id)
	query := "UPDATE " + s.quote("user_subscription") + " SET " + strings.Join(assignments, ", ") + " WHERE " + s.quote("id") + "=?"
	if _, err := s.db.ExecContext(ctx, s.bind(query), args...); err != nil {
		return nil, err
	}
	updated, err := s.CurrentSubscription(ctx, userID, false)
	if err == nil {
		err = s.auditSubscription(ctx, updated, "manual_adjust", 0, "admin")
	}
	return updated, err
}

func (s *Store) RemoveSubscription(ctx context.Context, userID int64) error {
	item, err := s.CurrentSubscription(ctx, userID, false)
	if err != nil {
		return err
	}
	if item == nil {
		return errors.New("该用户没有套餐记录")
	}
	result, err := s.db.ExecContext(ctx, s.bind("DELETE FROM "+s.quote("user_subscription")+" WHERE "+s.quote("id")+"=?"), item.id)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows != 1 {
		return errors.New("删除用户套餐失败")
	}
	return nil
}

func (s *Store) ResetSubscriptionQuota(ctx context.Context, userID int64) (*UserSubscription, error) {
	item, err := s.CurrentSubscription(ctx, userID, false)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, errors.New("该用户没有有效套餐")
	}
	plan, _ := s.SubscriptionPlanByID(ctx, item.planID)
	now := time.Now()
	next := calculateNextReset(now, plan.ResetDay)
	query := "UPDATE " + s.quote("user_subscription") + " SET " + s.quote("traffic_used_bytes") + "=0, " + s.quote("next_reset_at") + "=?, " + s.quote("updated_time") + "=? WHERE " + s.quote("id") + "=?"
	if _, err := s.db.ExecContext(ctx, s.bind(query), next, now.UnixMilli(), item.id); err != nil {
		return nil, err
	}
	updated, err := s.CurrentSubscription(ctx, userID, false)
	if err == nil {
		err = s.auditSubscription(ctx, updated, "manual_reset", 0, "admin")
	}
	return updated, err
}

func (s *Store) RedeemSubscription(ctx context.Context, userID int64, rawCode string) (*UserSubscription, error) {
	if strings.TrimSpace(rawCode) == "" {
		return nil, errors.New("请输入兑换码")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	query := "SELECT " + s.columns("id", "plan_id", "expires_at") + " FROM " + s.quote("redeem_code") + " WHERE " + s.quote("code_hash") + "=? AND " + s.quote("status") + "=1 LIMIT 1"
	var codeID, planID int64
	var expires sql.NullInt64
	if err := tx.QueryRowContext(ctx, s.bind(query), redeemHash(rawCode)).Scan(&codeID, &planID, &expires); err != nil || (expires.Valid && expires.Int64 > 0 && expires.Int64 < time.Now().UnixMilli()) {
		return nil, errors.New("兑换码无效、已使用或已过期")
	}
	plan, err := s.planByIDTx(ctx, tx, planID)
	if err != nil || plan.Status != 1 || plan.Redeemable != 1 {
		return nil, errors.New("该兑换码对应套餐不可兑换")
	}
	now := time.Now()
	consume := "UPDATE " + s.quote("redeem_code") + " SET " + s.quote("status") + "=0, " + s.quote("used_by") + "=?, " + s.quote("used_time") + "=? WHERE " + s.quote("id") + "=? AND " + s.quote("status") + "=1"
	result, err := tx.ExecContext(ctx, s.bind(consume), userID, now.UnixMilli(), codeID)
	if err != nil {
		return nil, err
	}
	rows, _ := result.RowsAffected()
	if rows != 1 {
		return nil, errors.New("兑换码已被使用")
	}
	item, err := s.activateSubscriptionTx(ctx, tx, userID, plan, now)
	if err != nil {
		return nil, err
	}
	if err := insertAudit(ctx, tx, s, item, "redeem", 0, "codeId="+strconv.FormatInt(codeID, 10)); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.CurrentSubscription(ctx, userID, true)
}

func (s *Store) SubscriptionDashboard(ctx context.Context, userID int64) (map[string]any, error) {
	active, err := s.CurrentSubscription(ctx, userID, true)
	if err != nil {
		return nil, err
	}
	user, _ := s.UserByID(ctx, userID)
	used, limit, forwardLimit := int64(0), int64(0), 0
	if active != nil {
		used, limit, forwardLimit = active.TrafficUsedBytes, active.TrafficLimitBytes, active.MaxForwards
	}
	count, err := countQuery(ctx, s.db, s.bind("SELECT COUNT(*) FROM "+s.quote("forward")+" WHERE "+s.quote("user_id")+"=? AND "+s.quote("status")+"<>-1"), userID)
	if err != nil {
		return nil, err
	}
	cutoff := time.Now().Add(-24 * time.Hour).UnixMilli()
	hourly, err := s.statisticsSince(ctx, userID, cutoff)
	if err != nil {
		return nil, err
	}
	result := map[string]any{"totalTrafficBytes": limit, "usedTrafficBytes": used, "remainingTrafficBytes": max(int64(0), limit-used),
		"forwardCount": count, "forwardLimit": forwardLimit, "last24Hours": hourly, "accountUsedTrafficBytes": user.InboundFlow + user.OutboundFlow}
	if active == nil {
		for _, key := range []string{"expiresAt", "nextResetAt", "planId", "planName", "planDescription", "validityValue", "validityUnit"} {
			result[key] = nil
		}
	} else {
		result["expiresAt"], result["nextResetAt"], result["planId"] = active.ExpiresAt, active.NextResetAt, active.PlanID
		result["planName"], result["planDescription"] = active.PlanName, active.PlanDescription
		result["validityValue"], result["validityUnit"] = active.PlanValidityValue, active.PlanValidityUnit
	}
	return result, nil
}

func (s *Store) statisticsSince(ctx context.Context, userID, cutoff int64) ([]FlowStatistic, error) {
	query := "SELECT " + s.columns("id", "user_id", "flow", "total_flow", "time", "created_time") + " FROM " + s.quote("statistics_flow") + " WHERE " + s.quote("user_id") + "=? AND " + s.quote("created_time") + ">=? ORDER BY " + s.quote("created_time")
	rows, err := s.db.QueryContext(ctx, s.bind(query), userID, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []FlowStatistic{}
	for rows.Next() {
		var item FlowStatistic
		var id, created int64
		if err := rows.Scan(&id, &item.UserID, &item.Flow, &item.TotalFlow, &item.Time, &created); err != nil {
			return nil, err
		}
		item.ID, item.CreatedTime = &id, &created
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) planByIDTx(ctx context.Context, tx *sql.Tx, id int64) (SubscriptionPlan, error) {
	query := "SELECT " + s.columns("id", "name", "description", "price", "currency", "validity_value", "validity_unit", "traffic_bytes", "reset_day", "reset_quota", "max_forwards", "for_sale", "redeemable", "sort_order", "status", "created_time", "updated_time") + " FROM " + s.quote("subscription_plan") + " WHERE " + s.quote("id") + "=?"
	var plan SubscriptionPlan
	err := scanPlan(tx.QueryRowContext(ctx, s.bind(query), id), &plan)
	return plan, err
}

func (s *Store) activateSubscriptionTx(ctx context.Context, tx *sql.Tx, userID int64, plan SubscriptionPlan, now time.Time) (*UserSubscription, error) {
	var oldID sql.NullInt64
	var oldExpiry sql.NullInt64
	var usedForwards sql.NullInt64
	query := "SELECT " + s.columns("id", "expires_at", "used_forwards") + " FROM " + s.quote("user_subscription") + " WHERE " + s.quote("user_id") + "=? ORDER BY " + s.quote("id") + " DESC LIMIT 1"
	err := tx.QueryRowContext(ctx, s.bind(query), userID).Scan(&oldID, &oldExpiry, &usedForwards)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	start := now
	if oldExpiry.Valid && oldExpiry.Int64 > now.UnixMilli() {
		start = time.UnixMilli(oldExpiry.Int64)
	}
	expires := int64(0)
	if !strings.EqualFold(plan.ValidityUnit, "permanent") {
		if strings.EqualFold(plan.ValidityUnit, "year") {
			expires = start.AddDate(plan.ValidityValue, 0, 0).UnixMilli()
		} else {
			expires = start.AddDate(0, plan.ValidityValue, 0).UnixMilli()
		}
	}
	nextReset := calculateNextReset(now, plan.ResetDay)
	if oldID.Valid {
		update := "UPDATE " + s.quote("user_subscription") + " SET " + s.quote("plan_id") + "=?, " + s.quote("starts_at") + "=?, " + s.quote("expires_at") + "=?, " + s.quote("traffic_limit_bytes") + "=?, " + s.quote("traffic_used_bytes") + "=0, " + s.quote("next_reset_at") + "=?, " + s.quote("max_forwards") + "=?, " + s.quote("status") + "=1, " + s.quote("updated_time") + "=? WHERE " + s.quote("id") + "=?"
		_, err = tx.ExecContext(ctx, s.bind(update), plan.id, now.UnixMilli(), expires, plan.TrafficBytes, nextReset, plan.MaxForwards, now.UnixMilli(), oldID.Int64)
	} else {
		insert := "INSERT INTO " + s.quote("user_subscription") + " (" + s.columns("user_id", "plan_id", "starts_at", "expires_at", "traffic_limit_bytes", "traffic_used_bytes", "next_reset_at", "max_forwards", "used_forwards", "status", "created_time", "updated_time") + ") VALUES (?, ?, ?, ?, ?, 0, ?, ?, 0, 1, ?, ?)"
		_, err = tx.ExecContext(ctx, s.bind(insert), userID, plan.id, now.UnixMilli(), expires, plan.TrafficBytes, nextReset, plan.MaxForwards, now.UnixMilli(), now.UnixMilli())
	}
	if err != nil {
		return nil, err
	}
	item := &UserSubscription{userID: userID, planID: plan.id, StartsAt: now.UnixMilli(), ExpiresAt: expires, TrafficLimitBytes: plan.TrafficBytes,
		NextResetAt: &nextReset, MaxForwards: plan.MaxForwards, Status: 1, UpdatedTime: now.UnixMilli(), PlanName: &plan.Name, PlanDescription: plan.Description,
		PlanValidityValue: &plan.ValidityValue, PlanValidityUnit: &plan.ValidityUnit}
	if oldID.Valid {
		item.id = oldID.Int64
	}
	if err := insertAudit(ctx, tx, s, item, "plan_activated", 0, "planId="+strconv.FormatInt(plan.id, 10)); err != nil {
		return nil, err
	}
	return item, nil
}

func (s *Store) auditSubscription(ctx context.Context, item *UserSubscription, event string, amount int64, detail string) error {
	if item == nil {
		return nil
	}
	metadata, _ := json.Marshal(map[string]string{"detail": detail})
	query := "INSERT INTO " + s.quote("quota_usage_log") + " (" + s.columns("user_id", "subscription_id", "event_type", "amount", "metadata", "created_time") + ") VALUES (?, ?, ?, ?, ?, ?)"
	_, err := s.db.ExecContext(ctx, s.bind(query), item.userID, nullableID(item.id), event, amount, string(metadata), time.Now().UnixMilli())
	return err
}

func insertAudit(ctx context.Context, tx *sql.Tx, s *Store, item *UserSubscription, event string, amount int64, detail string) error {
	metadata, _ := json.Marshal(map[string]string{"detail": detail})
	query := "INSERT INTO " + s.quote("quota_usage_log") + " (" + s.columns("user_id", "subscription_id", "event_type", "amount", "metadata", "created_time") + ") VALUES (?, ?, ?, ?, ?, ?)"
	_, err := tx.ExecContext(ctx, s.bind(query), item.userID, nullableID(item.id), event, amount, string(metadata), time.Now().UnixMilli())
	return err
}

func calculateNextReset(from time.Time, day int) int64 {
	if day <= 0 {
		return 0
	}
	if day > 31 {
		day = 31
	}
	year, month, _ := from.Date()
	location := from.Location()
	lastDay := time.Date(year, month+1, 0, 0, 0, 0, 0, location).Day()
	actual := min(day, lastDay)
	candidate := time.Date(year, month, actual, 0, 0, 0, 0, location)
	if !candidate.After(from) {
		month++
		lastDay = time.Date(year, month+1, 0, 0, 0, 0, 0, location).Day()
		candidate = time.Date(year, month, min(day, lastDay), 0, 0, 0, 0, location)
	}
	return candidate.UnixMilli()
}

func randomRedeemCode() (string, error) {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	random := make([]byte, 20)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	var builder strings.Builder
	for index, value := range random {
		if index > 0 && index%5 == 0 {
			builder.WriteByte('-')
		}
		builder.WriteByte(alphabet[int(value)%len(alphabet)])
	}
	return builder.String(), nil
}

func redeemHash(value string) string {
	sum := sha256.Sum256([]byte(strings.ToUpper(strings.TrimSpace(value))))
	return hex.EncodeToString(sum[:])
}

func anyInt64(value any) (int64, error) {
	switch typed := value.(type) {
	case float64:
		return int64(typed), nil
	case json.Number:
		return typed.Int64()
	case string:
		return strconv.ParseInt(typed, 10, 64)
	case int64:
		return typed, nil
	case int:
		return int64(typed), nil
	default:
		return 0, fmt.Errorf("not a number")
	}
}

func nullableStringPointer(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}

func nullableID(value int64) any {
	if value == 0 {
		return nil
	}
	return value
}
