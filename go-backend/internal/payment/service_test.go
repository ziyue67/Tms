package payment

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/ziyue67/tms/go-backend/internal/store"
)

type fakeRepository struct {
	configs map[string]string
	orders  map[string]store.PaymentOrder
	plan    store.SubscriptionPlan
}

func (f *fakeRepository) ConfigValue(_ context.Context, name string) (store.SiteConfig, error) {
	value, ok := f.configs[name]
	if !ok {
		return store.SiteConfig{}, sql.ErrNoRows
	}
	return store.SiteConfig{Name: name, Value: value}, nil
}
func (f *fakeRepository) SubscriptionPlanByID(context.Context, int64) (store.SubscriptionPlan, error) {
	return f.plan, nil
}
func (f *fakeRepository) CreatePaymentOrder(_ context.Context, order store.PaymentOrder) (store.PaymentOrder, error) {
	order.ID = 1
	f.orders[order.OrderNo] = order
	return order, nil
}
func (f *fakeRepository) PaymentOrderByNo(_ context.Context, orderNo string, _ *int64) (*store.PaymentOrder, error) {
	order, ok := f.orders[orderNo]
	if !ok {
		return nil, nil
	}
	return &order, nil
}
func (f *fakeRepository) PaymentOrders(context.Context, *int64) ([]store.PaymentOrder, error) {
	return nil, nil
}
func (f *fakeRepository) FailPaymentOrder(context.Context, string, string) error { return nil }
func (f *fakeRepository) RetryPaymentOrder(_ context.Context, orderNo string) (*store.PaymentOrder, error) {
	order := f.orders[orderNo]
	return &order, nil
}
func (f *fakeRepository) CompletePaymentOrder(_ context.Context, orderNo, tradeNo, payload string) (*store.PaymentOrder, error) {
	order := f.orders[orderNo]
	order.Status = "paid"
	return &order, nil
}

func TestManualCheckoutUsesCompatibilityShape(t *testing.T) {
	repository := &fakeRepository{configs: map[string]string{"payment_enabled": "true", "payment_manual_enabled": "true"}, orders: map[string]store.PaymentOrder{},
		plan: store.SubscriptionPlan{Name: "月卡", Price: 10, Currency: "CNY", Status: 1, ForSale: 1}}
	service := New(repository)
	result, err := service.CreateCheckout(context.Background(), 1, 2, "manual")
	if err != nil {
		t.Fatalf("checkout: %v", err)
	}
	checkout := result["checkout"].(Checkout)
	if checkout.Type != "manual" || checkout.URL != nil || checkout.Message == "" {
		t.Fatalf("unexpected checkout: %+v", checkout)
	}
}

func TestEasyPayRejectsModifiedSignature(t *testing.T) {
	repository := &fakeRepository{configs: map[string]string{"payment_easypay_key": "secret"}, orders: map[string]store.PaymentOrder{}}
	service := New(repository)
	order := store.PaymentOrder{OrderNo: "TMS1", Provider: "easypay", Amount: 10, Currency: "CNY", Status: "pending", CreatedTime: time.Now().UnixMilli()}
	repository.orders[order.OrderNo] = order
	values := map[string]string{"out_trade_no": order.OrderNo, "trade_no": "trade-1", "trade_status": "TRADE_SUCCESS", "money": "10.00", "sign_type": "MD5", "sign": "bad"}
	_, err := service.Callback(context.Background(), "easypay", Callback{Values: values})
	if err == nil || err.Error() != "易支付回调签名校验失败" {
		t.Fatalf("expected signature rejection, got %v", err)
	}
}

func TestRequiredProviderConfigReturnsError(t *testing.T) {
	repository := &fakeRepository{configs: map[string]string{"payment_enabled": "true", "payment_easypay_enabled": "true"}, orders: map[string]store.PaymentOrder{},
		plan: store.SubscriptionPlan{Name: "月卡", Price: 10, Currency: "CNY", Status: 1, ForSale: 1}}
	_, err := New(repository).CreateCheckout(context.Background(), 1, 2, "easypay")
	if err == nil || err.Error() != "请先配置 payment_easypay_pid" {
		t.Fatalf("expected config error, got %v", err)
	}
}
