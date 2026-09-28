package store

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"time"

	"github.com/ziyue67/tms/go-backend/internal/database"
)

type PaymentOrder struct {
	ID              int64   `json:"id"`
	OrderNo         string  `json:"orderNo"`
	UserID          int64   `json:"userId"`
	PlanID          int64   `json:"planId"`
	Provider        string  `json:"provider"`
	Amount          float64 `json:"amount"`
	Currency        string  `json:"currency"`
	Status          string  `json:"status"`
	ProviderTradeNo *string `json:"providerTradeNo"`
	CallbackPayload *string `json:"callbackPayload"`
	PaidAt          *int64  `json:"paidAt"`
	CreatedTime     int64   `json:"createdTime"`
	UpdatedTime     int64   `json:"updatedTime"`
}

func (s *Store) CreatePaymentOrder(ctx context.Context, order PaymentOrder) (PaymentOrder, error) {
	query := "INSERT INTO " + s.quote("payment_order") + " (" + s.columns("order_no", "user_id", "plan_id", "provider", "amount", "currency", "status", "created_time", "updated_time") + ") VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)"
	args := []any{order.OrderNo, order.UserID, order.PlanID, order.Provider, order.Amount, order.Currency, order.Status, order.CreatedTime, order.UpdatedTime}
	if s.dialect == database.PostgreSQL {
		if err := s.db.QueryRowContext(ctx, s.bind(query+" RETURNING "+s.quote("id")), args...).Scan(&order.ID); err != nil {
			return PaymentOrder{}, err
		}
	} else {
		result, err := s.db.ExecContext(ctx, query, args...)
		if err != nil {
			return PaymentOrder{}, err
		}
		order.ID, err = result.LastInsertId()
		if err != nil {
			return PaymentOrder{}, err
		}
	}
	return order, nil
}

func (s *Store) PaymentOrderByNo(ctx context.Context, orderNo string, userID *int64) (*PaymentOrder, error) {
	query := "SELECT " + s.columns("id", "order_no", "user_id", "plan_id", "provider", "amount", "currency", "status", "provider_trade_no", "callback_payload", "paid_at", "created_time", "updated_time") + " FROM " + s.quote("payment_order") + " WHERE " + s.quote("order_no") + "=?"
	args := []any{orderNo}
	if userID != nil {
		query += " AND " + s.quote("user_id") + "=?"
		args = append(args, *userID)
	}
	query += " LIMIT 1"
	var item PaymentOrder
	var trade, payload sql.NullString
	var paid sql.NullInt64
	err := s.db.QueryRowContext(ctx, s.bind(query), args...).Scan(&item.ID, &item.OrderNo, &item.UserID, &item.PlanID, &item.Provider,
		&item.Amount, &item.Currency, &item.Status, &trade, &payload, &paid, &item.CreatedTime, &item.UpdatedTime)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	item.ProviderTradeNo, item.CallbackPayload, item.PaidAt = nullString(trade), nullString(payload), nullInt(paid)
	return &item, nil
}

func (s *Store) PaymentOrders(ctx context.Context, userID *int64) ([]PaymentOrder, error) {
	query := "SELECT " + s.columns("id", "order_no", "user_id", "plan_id", "provider", "amount", "currency", "status", "provider_trade_no", "callback_payload", "paid_at", "created_time", "updated_time") + " FROM " + s.quote("payment_order")
	args := []any{}
	if userID != nil {
		query += " WHERE " + s.quote("user_id") + "=?"
		args = append(args, *userID)
	}
	query += " ORDER BY " + s.quote("id") + " DESC"
	rows, err := s.db.QueryContext(ctx, s.bind(query), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]PaymentOrder, 0)
	for rows.Next() {
		var item PaymentOrder
		var trade, payload sql.NullString
		var paid sql.NullInt64
		if err := rows.Scan(&item.ID, &item.OrderNo, &item.UserID, &item.PlanID, &item.Provider, &item.Amount,
			&item.Currency, &item.Status, &trade, &payload, &paid, &item.CreatedTime, &item.UpdatedTime); err != nil {
			return nil, err
		}
		item.ProviderTradeNo, item.CallbackPayload, item.PaidAt = nullString(trade), nullString(payload), nullInt(paid)
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) FailPaymentOrder(ctx context.Context, orderNo, detail string) error {
	query := "UPDATE " + s.quote("payment_order") + " SET " + s.quote("status") + "='failed', " + s.quote("callback_payload") + "=?, " + s.quote("updated_time") + "=? WHERE " + s.quote("order_no") + "=?"
	_, err := s.db.ExecContext(ctx, s.bind(query), detail, time.Now().UnixMilli(), orderNo)
	return err
}

func (s *Store) RetryPaymentOrder(ctx context.Context, orderNo string) (*PaymentOrder, error) {
	item, err := s.PaymentOrderByNo(ctx, orderNo, nil)
	if err != nil || item == nil {
		return item, err
	}
	if item.Status == "paid" {
		return nil, errors.New("已支付订单不能重试")
	}
	query := "UPDATE " + s.quote("payment_order") + " SET " + s.quote("status") + "='pending', " + s.quote("callback_payload") + "=NULL, " + s.quote("updated_time") + "=? WHERE " + s.quote("id") + "=?"
	if _, err := s.db.ExecContext(ctx, s.bind(query), time.Now().UnixMilli(), item.ID); err != nil {
		return nil, err
	}
	item.Status, item.CallbackPayload = "pending", nil
	return item, nil
}

func (s *Store) CompletePaymentOrder(ctx context.Context, orderNo, tradeNo, payload string) (*PaymentOrder, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	item, err := paymentOrderTx(ctx, tx, s, orderNo)
	if err != nil {
		return nil, err
	}
	if item.Status == "paid" {
		return item, tx.Commit()
	}
	now := time.Now()
	query := "UPDATE " + s.quote("payment_order") + " SET " + s.quote("status") + "='paid', " + s.quote("provider_trade_no") + "=?, " + s.quote("callback_payload") + "=?, " + s.quote("paid_at") + "=?, " + s.quote("updated_time") + "=? WHERE " + s.quote("id") + "=? AND " + s.quote("status") + "='pending'"
	result, err := tx.ExecContext(ctx, s.bind(query), tradeNo, payload, now.UnixMilli(), now.UnixMilli(), item.ID)
	if err != nil {
		return nil, err
	}
	rows, _ := result.RowsAffected()
	if rows != 1 {
		return nil, errors.New("订单状态已变化")
	}
	plan, err := s.planByIDTx(ctx, tx, item.PlanID)
	if err != nil {
		return nil, err
	}
	if _, err := s.activateSubscriptionTx(ctx, tx, item.UserID, plan, now); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	item.Status, item.ProviderTradeNo, item.CallbackPayload = "paid", &tradeNo, &payload
	paidAt := now.UnixMilli()
	item.PaidAt, item.UpdatedTime = &paidAt, paidAt
	return item, nil
}

func paymentOrderTx(ctx context.Context, tx *sql.Tx, s *Store, orderNo string) (*PaymentOrder, error) {
	query := "SELECT " + s.columns("id", "order_no", "user_id", "plan_id", "provider", "amount", "currency", "status", "provider_trade_no", "callback_payload", "paid_at", "created_time", "updated_time") + " FROM " + s.quote("payment_order") + " WHERE " + s.quote("order_no") + "=? LIMIT 1"
	var item PaymentOrder
	var trade, payload sql.NullString
	var paid sql.NullInt64
	err := tx.QueryRowContext(ctx, s.bind(query), orderNo).Scan(&item.ID, &item.OrderNo, &item.UserID, &item.PlanID, &item.Provider,
		&item.Amount, &item.Currency, &item.Status, &trade, &payload, &paid, &item.CreatedTime, &item.UpdatedTime)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errors.New("订单不存在")
	}
	item.ProviderTradeNo, item.CallbackPayload, item.PaidAt = nullString(trade), nullString(payload), nullInt(paid)
	return &item, err
}

func NewOrderNumber(now time.Time, suffix string) string {
	return "TMS" + strconv.FormatInt(now.UnixMilli(), 10) + suffix
}
