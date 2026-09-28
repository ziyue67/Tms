package payment

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ziyue67/tms/go-backend/internal/store"
)

type Repository interface {
	ConfigValue(context.Context, string) (store.SiteConfig, error)
	SubscriptionPlanByID(context.Context, int64) (store.SubscriptionPlan, error)
	CreatePaymentOrder(context.Context, store.PaymentOrder) (store.PaymentOrder, error)
	PaymentOrderByNo(context.Context, string, *int64) (*store.PaymentOrder, error)
	PaymentOrders(context.Context, *int64) ([]store.PaymentOrder, error)
	FailPaymentOrder(context.Context, string, string) error
	RetryPaymentOrder(context.Context, string) (*store.PaymentOrder, error)
	CompletePaymentOrder(context.Context, string, string, string) (*store.PaymentOrder, error)
}

type Checkout struct {
	Type    string            `json:"type"`
	URL     *string           `json:"url"`
	Fields  map[string]string `json:"fields"`
	Message string            `json:"message"`
}

type Callback struct {
	RawBody string
	Values  map[string]string
	Headers http.Header
}

type Service struct {
	repository Repository
	client     *http.Client
}

func New(repository Repository) *Service {
	return &Service{repository: repository, client: &http.Client{Timeout: 20 * time.Second}}
}

func (s *Service) Providers(ctx context.Context) []map[string]string {
	if !s.enabled(ctx, "payment_enabled", true) {
		return []map[string]string{}
	}
	labels := map[string]string{"alipay": "支付宝", "wechat": "微信支付", "easypay": "易支付", "stripe": "Stripe", "manual": "人工支付"}
	result := make([]map[string]string, 0)
	for _, provider := range []string{"alipay", "wechat", "easypay", "stripe", "manual"} {
		fallback := provider == "manual"
		if s.enabled(ctx, "payment_"+provider+"_enabled", fallback) {
			result = append(result, map[string]string{"key": provider, "label": labels[provider]})
		}
	}
	return result
}

func (s *Service) CreateCheckout(ctx context.Context, userID, planID int64, provider string) (result map[string]any, err error) {
	defer recoverRequired(&err)
	if !s.enabled(ctx, "payment_enabled", true) {
		return nil, errors.New("支付系统已关闭")
	}
	plan, err := s.repository.SubscriptionPlanByID(ctx, planID)
	if err != nil || plan.Status != 1 || plan.ForSale != 1 {
		return nil, errors.New("套餐不可购买")
	}
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider == "" {
		provider = "manual"
	}
	valid := map[string]bool{"alipay": true, "wechat": true, "easypay": true, "stripe": true, "manual": true}
	if !valid[provider] {
		return nil, errors.New("不支持的支付方式")
	}
	if !s.enabled(ctx, "payment_"+provider+"_enabled", provider == "manual") {
		return nil, errors.New("该支付方式未启用")
	}
	now := time.Now()
	suffix := make([]byte, 5)
	if _, err := rand.Read(suffix); err != nil {
		return nil, err
	}
	order := store.PaymentOrder{OrderNo: store.NewOrderNumber(now, strings.ToUpper(hex.EncodeToString(suffix))), UserID: userID,
		PlanID: planID, Provider: provider, Amount: plan.Price, Currency: plan.Currency, Status: "pending", CreatedTime: now.UnixMilli(), UpdatedTime: now.UnixMilli()}
	order, err = s.repository.CreatePaymentOrder(ctx, order)
	if err != nil {
		return nil, err
	}
	checkout, err := s.checkout(ctx, order, plan)
	if err != nil {
		_ = s.repository.FailPaymentOrder(ctx, order.OrderNo, err.Error())
		return nil, err
	}
	return map[string]any{"order": order, "checkout": checkout}, nil
}

func (s *Service) Retry(ctx context.Context, orderNo string) (result map[string]any, err error) {
	defer recoverRequired(&err)
	order, err := s.repository.RetryPaymentOrder(ctx, orderNo)
	if err != nil {
		return nil, err
	}
	if order == nil {
		return nil, errors.New("订单不存在")
	}
	plan, err := s.repository.SubscriptionPlanByID(ctx, order.PlanID)
	if err != nil {
		return nil, errors.New("套餐不存在")
	}
	checkout, err := s.checkout(ctx, *order, plan)
	if err != nil {
		_ = s.repository.FailPaymentOrder(ctx, orderNo, err.Error())
		return nil, err
	}
	return map[string]any{"order": order, "checkout": checkout}, nil
}

func (s *Service) CompleteTest(ctx context.Context, orderNo string) (*store.PaymentOrder, error) {
	if !s.enabled(ctx, "payment_test_mode", false) {
		return nil, errors.New("测试支付未启用")
	}
	return s.repository.CompletePaymentOrder(ctx, orderNo, "test-"+orderNo, `{"provider":"test"}`)
}

func (s *Service) Callback(ctx context.Context, provider string, callback Callback) (result *store.PaymentOrder, err error) {
	defer recoverRequired(&err)
	provider = strings.ToLower(provider)
	orderNo, tradeNo, err := s.verifyCallback(ctx, provider, callback)
	if err != nil {
		return nil, err
	}
	order, err := s.repository.PaymentOrderByNo(ctx, orderNo, nil)
	if err != nil || order == nil {
		return nil, errors.New("订单不存在")
	}
	if order.Provider != provider {
		return nil, errors.New("支付渠道不匹配")
	}
	if err := s.verifyOrderValues(ctx, *order, provider, callback); err != nil {
		return nil, err
	}
	payload := callback.RawBody
	if payload == "" {
		encoded, _ := json.Marshal(callback.Values)
		payload = string(encoded)
	}
	return s.repository.CompletePaymentOrder(ctx, orderNo, tradeNo, payload)
}

func (s *Service) checkout(ctx context.Context, order store.PaymentOrder, plan store.SubscriptionPlan) (Checkout, error) {
	switch order.Provider {
	case "manual":
		return Checkout{Type: "manual", Fields: map[string]string{}, Message: "请联系管理员确认人工付款"}, nil
	case "easypay":
		fields := map[string]string{"pid": s.required(ctx, "payment_easypay_pid"), "type": s.config(ctx, "payment_easypay_type", "alipay"),
			"out_trade_no": order.OrderNo, "notify_url": s.required(ctx, "payment_easypay_notify_url"), "name": plan.Name,
			"money": fmt.Sprintf("%.2f", order.Amount), "sign_type": "MD5"}
		if value := s.config(ctx, "payment_easypay_return_url", ""); value != "" {
			fields["return_url"] = value
		}
		fields["sign"] = md5Hex(orderedQuery(fields, false) + s.required(ctx, "payment_easypay_key"))
		endpoint := s.config(ctx, "payment_easypay_gateway", "https://your-easypay.example/api.php")
		return Checkout{Type: "form", URL: &endpoint, Fields: fields, Message: "将跳转至易支付完成付款"}, nil
	case "alipay":
		biz, _ := json.Marshal(map[string]any{"out_trade_no": order.OrderNo, "product_code": "FAST_INSTANT_TRADE_PAY", "total_amount": fmt.Sprintf("%.2f", order.Amount), "subject": truncate(plan.Name, 120)})
		fields := map[string]string{"app_id": s.required(ctx, "payment_alipay_app_id"), "method": "alipay.trade.page.pay", "format": "JSON", "charset": "utf-8", "sign_type": "RSA2", "timestamp": time.Now().Format("2006-01-02 15:04:05"), "version": "1.0", "notify_url": s.required(ctx, "payment_alipay_notify_url"), "biz_content": string(biz)}
		if value := s.config(ctx, "payment_alipay_return_url", ""); value != "" {
			fields["return_url"] = value
		}
		signature, err := signRSA(orderedQuery(fields, false), s.required(ctx, "payment_alipay_private_key"))
		if err != nil {
			return Checkout{}, err
		}
		fields["sign"] = signature
		endpoint := s.config(ctx, "payment_alipay_gateway", "https://openapi.alipay.com/gateway.do")
		return Checkout{Type: "form", URL: &endpoint, Fields: fields, Message: "将跳转至支付宝完成付款"}, nil
	case "stripe":
		return s.stripeCheckout(ctx, order, plan)
	case "wechat":
		return s.wechatCheckout(ctx, order, plan)
	}
	return Checkout{}, errors.New("不支持的支付方式")
}

func (s *Service) stripeCheckout(ctx context.Context, order store.PaymentOrder, plan store.SubscriptionPlan) (Checkout, error) {
	success := appendOrder(s.required(ctx, "payment_stripe_success_url"), order.OrderNo)
	values := url.Values{"mode": {"payment"}, "success_url": {success}, "cancel_url": {s.required(ctx, "payment_stripe_cancel_url")},
		"client_reference_id": {order.OrderNo}, "metadata[orderNo]": {order.OrderNo}, "line_items[0][price_data][currency]": {strings.ToLower(order.Currency)},
		"line_items[0][price_data][unit_amount]": {strconv.FormatInt(int64(order.Amount*100+0.5), 10)}, "line_items[0][price_data][product_data][name]": {plan.Name}, "line_items[0][quantity]": {"1"}}
	request, _ := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.stripe.com/v1/checkout/sessions", strings.NewReader(values.Encode()))
	request.SetBasicAuth(s.required(ctx, "payment_stripe_secret_key"), "")
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	var response struct {
		URL string `json:"url"`
	}
	if err := s.doJSON(request, &response); err != nil {
		return Checkout{}, err
	}
	if response.URL == "" {
		return Checkout{}, errors.New("Stripe 未返回支付链接")
	}
	return Checkout{Type: "redirect", URL: &response.URL, Fields: map[string]string{}, Message: "将跳转至 Stripe Checkout 完成付款"}, nil
}

func (s *Service) wechatCheckout(ctx context.Context, order store.PaymentOrder, plan store.SubscriptionPlan) (Checkout, error) {
	const path = "/v3/pay/transactions/native"
	body, _ := json.Marshal(map[string]any{"appid": s.required(ctx, "payment_wechat_app_id"), "mchid": s.required(ctx, "payment_wechat_mchid"), "description": plan.Name,
		"out_trade_no": order.OrderNo, "notify_url": s.required(ctx, "payment_wechat_notify_url"), "amount": map[string]any{"total": int64(order.Amount*100 + 0.5), "currency": order.Currency}})
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	nonceBytes := make([]byte, 16)
	_, _ = rand.Read(nonceBytes)
	nonce := hex.EncodeToString(nonceBytes)
	signature, err := signRSA("POST\n"+path+"\n"+timestamp+"\n"+nonce+"\n"+string(body)+"\n", s.required(ctx, "payment_wechat_private_key"))
	if err != nil {
		return Checkout{}, err
	}
	authorization := fmt.Sprintf(`WECHATPAY2-SHA256-RSA2048 mchid="%s",nonce_str="%s",timestamp="%s",serial_no="%s",signature="%s"`, s.required(ctx, "payment_wechat_mchid"), nonce, timestamp, s.required(ctx, "payment_wechat_serial_no"), signature)
	request, _ := http.NewRequestWithContext(ctx, http.MethodPost, s.config(ctx, "payment_wechat_gateway", "https://api.mch.weixin.qq.com")+path, bytes.NewReader(body))
	request.Header.Set("Authorization", authorization)
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	var response struct {
		CodeURL string `json:"code_url"`
	}
	if err := s.doJSON(request, &response); err != nil {
		return Checkout{}, err
	}
	if response.CodeURL == "" {
		return Checkout{}, errors.New("微信支付未返回付款码")
	}
	return Checkout{Type: "qr", URL: &response.CodeURL, Fields: map[string]string{}, Message: "请使用微信扫码完成付款"}, nil
}

func (s *Service) verifyCallback(ctx context.Context, provider string, callback Callback) (string, string, error) {
	switch provider {
	case "alipay", "easypay":
		return callback.Values["out_trade_no"], callback.Values["trade_no"], nil
	case "stripe":
		var root struct {
			Data struct {
				Object struct {
					ClientReferenceID string            `json:"client_reference_id"`
					PaymentIntent     string            `json:"payment_intent"`
					Metadata          map[string]string `json:"metadata"`
				} `json:"object"`
			} `json:"data"`
		}
		if json.Unmarshal([]byte(callback.RawBody), &root) != nil {
			return "", "", errors.New("Stripe 回调格式无效")
		}
		orderNo := root.Data.Object.ClientReferenceID
		if orderNo == "" {
			orderNo = root.Data.Object.Metadata["orderNo"]
		}
		return orderNo, root.Data.Object.PaymentIntent, nil
	case "wechat":
		value, err := s.decryptWechat(ctx, callback.RawBody)
		if err != nil {
			return "", "", err
		}
		return stringValue(value, "out_trade_no"), stringValue(value, "transaction_id"), nil
	default:
		return "", "", errors.New("不支持的支付方式")
	}
}

func (s *Service) verifyOrderValues(ctx context.Context, order store.PaymentOrder, provider string, callback Callback) error {
	switch provider {
	case "easypay":
		status := callback.Values["trade_status"]
		if !strings.EqualFold(status, "TRADE_SUCCESS") && !strings.EqualFold(status, "SUCCESS") {
			return errors.New("易支付交易未成功")
		}
		expected := md5Hex(orderedQuery(callback.Values, false) + s.required(ctx, "payment_easypay_key"))
		if !constantEqual(expected, callback.Values["sign"]) {
			return errors.New("易支付回调签名校验失败")
		}
		return verifyAmount(order.Amount, callback.Values["money"], "易支付")
	case "alipay":
		status := callback.Values["trade_status"]
		if status != "TRADE_SUCCESS" && status != "TRADE_FINISHED" {
			return errors.New("支付宝交易未成功")
		}
		if !verifyRSA(orderedQuery(callback.Values, false), callback.Values["sign"], s.required(ctx, "payment_alipay_public_key")) {
			return errors.New("支付宝回调签名校验失败")
		}
		if configured := s.config(ctx, "payment_alipay_app_id", ""); configured != "" && configured != callback.Values["app_id"] {
			return errors.New("支付宝应用不匹配")
		}
		return verifyAmount(order.Amount, callback.Values["total_amount"], "支付宝")
	case "stripe":
		return s.verifyStripe(ctx, order, callback)
	case "wechat":
		return s.verifyWechat(ctx, order, callback)
	}
	return errors.New("不支持的支付方式")
}

func (s *Service) verifyStripe(ctx context.Context, order store.PaymentOrder, callback Callback) error {
	header := callback.Headers.Get("Stripe-Signature")
	var timestamp, signature string
	for _, piece := range strings.Split(header, ",") {
		pair := strings.SplitN(piece, "=", 2)
		if len(pair) == 2 && pair[0] == "t" {
			timestamp = pair[1]
		}
		if len(pair) == 2 && pair[0] == "v1" {
			signature = pair[1]
		}
	}
	seconds, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil || signature == "" || abs(time.Now().Unix()-seconds) > 300 {
		return errors.New("Stripe-Signature 格式无效或已过期")
	}
	if !constantEqual(hmacHex(s.required(ctx, "payment_stripe_webhook_secret"), timestamp+"."+callback.RawBody), signature) {
		return errors.New("Stripe 回调签名校验失败")
	}
	var root struct {
		Type string `json:"type"`
		Data struct {
			Object struct {
				PaymentStatus string `json:"payment_status"`
				Currency      string `json:"currency"`
				AmountTotal   int64  `json:"amount_total"`
			} `json:"object"`
		} `json:"data"`
	}
	if json.Unmarshal([]byte(callback.RawBody), &root) != nil || (root.Type != "checkout.session.completed" && root.Type != "checkout.session.async_payment_succeeded") || !strings.EqualFold(root.Data.Object.PaymentStatus, "paid") {
		return errors.New("Stripe 事件未完成付款")
	}
	if root.Data.Object.AmountTotal != int64(order.Amount*100+0.5) {
		return errors.New("Stripe 回调金额不匹配")
	}
	if !strings.EqualFold(root.Data.Object.Currency, order.Currency) {
		return errors.New("Stripe 回调币种不匹配")
	}
	return nil
}

func (s *Service) verifyWechat(ctx context.Context, order store.PaymentOrder, callback Callback) error {
	timestamp, nonce, signature := callback.Headers.Get("Wechatpay-Timestamp"), callback.Headers.Get("Wechatpay-Nonce"), callback.Headers.Get("Wechatpay-Signature")
	seconds, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil || nonce == "" || signature == "" || abs(time.Now().Unix()-seconds) > 300 {
		return errors.New("缺少微信支付回调签名或回调已过期")
	}
	if !verifyRSA(timestamp+"\n"+nonce+"\n"+callback.RawBody+"\n", signature, s.required(ctx, "payment_wechat_platform_certificate")) {
		return errors.New("微信支付回调签名校验失败")
	}
	value, err := s.decryptWechat(ctx, callback.RawBody)
	if err != nil {
		return err
	}
	if stringValue(value, "trade_state") != "SUCCESS" {
		return errors.New("微信支付交易未成功")
	}
	amount, _ := value["amount"].(map[string]any)
	if int64(numberValue(amount, "total")) != int64(order.Amount*100+0.5) {
		return errors.New("微信支付回调金额不匹配")
	}
	if !strings.EqualFold(stringValue(amount, "currency"), order.Currency) {
		return errors.New("微信支付回调币种不匹配")
	}
	if stringValue(value, "appid") != s.required(ctx, "payment_wechat_app_id") {
		return errors.New("微信支付应用不匹配")
	}
	if stringValue(value, "mchid") != s.required(ctx, "payment_wechat_mchid") {
		return errors.New("微信支付商户不匹配")
	}
	return nil
}

func (s *Service) decryptWechat(ctx context.Context, raw string) (map[string]any, error) {
	var root struct {
		Resource struct{ Nonce, AssociatedData, Ciphertext string } `json:"resource"`
	}
	if json.Unmarshal([]byte(raw), &root) != nil || root.Resource.Ciphertext == "" {
		return nil, errors.New("微信支付回调缺少 resource")
	}
	key := []byte(s.required(ctx, "payment_wechat_api_v3_key"))
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, errors.New("微信支付回调解密失败")
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, errors.New("微信支付回调解密失败")
	}
	ciphertext, err := base64.StdEncoding.DecodeString(root.Resource.Ciphertext)
	if err != nil {
		return nil, errors.New("微信支付回调解密失败")
	}
	plain, err := gcm.Open(nil, []byte(root.Resource.Nonce), ciphertext, []byte(root.Resource.AssociatedData))
	if err != nil {
		return nil, errors.New("微信支付回调解密失败")
	}
	result := map[string]any{}
	if json.Unmarshal(plain, &result) != nil {
		return nil, errors.New("微信支付回调解密失败")
	}
	return result, nil
}

func (s *Service) doJSON(request *http.Request, destination any) error {
	response, err := s.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode >= 400 {
		return fmt.Errorf("支付请求失败: %s", strings.TrimSpace(string(body)))
	}
	return json.Unmarshal(body, destination)
}

func (s *Service) config(ctx context.Context, name, fallback string) string {
	value, err := s.repository.ConfigValue(ctx, name)
	if err != nil || strings.TrimSpace(value.Value) == "" {
		return fallback
	}
	return strings.TrimSpace(value.Value)
}
func (s *Service) required(ctx context.Context, name string) string {
	value := s.config(ctx, name, "")
	if value == "" {
		panic(requiredConfigError{name})
	}
	return value
}
func (s *Service) enabled(ctx context.Context, name string, fallback bool) bool {
	value, err := strconv.ParseBool(s.config(ctx, name, strconv.FormatBool(fallback)))
	return err == nil && value
}

type requiredConfigError struct{ name string }

func (e requiredConfigError) Error() string { return "请先配置 " + e.name }
func recoverRequired(target *error) {
	if recovered := recover(); recovered != nil {
		if configError, ok := recovered.(requiredConfigError); ok {
			*target = configError
			return
		}
		panic(recovered)
	}
}

func appendOrder(raw, orderNo string) string {
	separator := "?"
	if strings.Contains(raw, "?") {
		separator = "&"
	}
	return raw + separator + "orderNo=" + url.QueryEscape(orderNo)
}
func truncate(value string, maximum int) string {
	if value == "" {
		value = "TMS 套餐"
	}
	runes := []rune(value)
	if len(runes) > maximum {
		runes = runes[:maximum]
	}
	return string(runes)
}
func verifyAmount(expected float64, actual, provider string) error {
	parsed, err := strconv.ParseFloat(actual, 64)
	if err != nil {
		return fmt.Errorf("%s回调金额无效", provider)
	}
	if absFloat(expected-parsed) > 0.00001 {
		return fmt.Errorf("%s回调金额不匹配", provider)
	}
	return nil
}
func abs(value int64) int64 {
	if value < 0 {
		return -value
	}
	return value
}
func absFloat(value float64) float64 {
	if value < 0 {
		return -value
	}
	return value
}
func stringValue(values map[string]any, key string) string {
	if values == nil {
		return ""
	}
	value, _ := values[key].(string)
	return value
}
func numberValue(values map[string]any, key string) float64 {
	if values == nil {
		return 0
	}
	value, _ := values[key].(float64)
	return value
}
