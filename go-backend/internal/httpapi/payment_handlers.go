package httpapi

import (
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/ziyue67/tms/go-backend/internal/payment"
	"github.com/ziyue67/tms/go-backend/internal/store"
)

func (a *API) paymentProviders(w http.ResponseWriter, r *http.Request) {
	writeResponse(w, OK(a.payments.Providers(r.Context())))
}

func (a *API) createPaymentOrder(w http.ResponseWriter, r *http.Request) {
	userID, ok := currentUserID(w, r)
	if !ok {
		return
	}
	var body map[string]any
	if !decodeJSON(w, r, &body) {
		return
	}
	planID, err := requestInt64(body["planId"])
	if err != nil {
		writeResponse(w, Failure("套餐格式错误"))
		return
	}
	provider := "manual"
	if body["provider"] != nil {
		provider = toString(body["provider"])
	}
	result, err := a.payments.CreateCheckout(r.Context(), userID, planID, provider)
	if err != nil {
		writeResponse(w, Failure(err.Error()))
		return
	}
	writeResponse(w, OK(result))
}

func (a *API) paymentOrder(w http.ResponseWriter, r *http.Request) {
	userID, ok := currentUserID(w, r)
	if !ok {
		return
	}
	item, err := a.store.PaymentOrderByNo(r.Context(), chi.URLParam(r, "orderNo"), &userID)
	if err != nil {
		writeResponse(w, Error(-2, err.Error()))
		return
	}
	writeResponse(w, OK(item))
}

func (a *API) myPaymentOrders(w http.ResponseWriter, r *http.Request) {
	userID, ok := currentUserID(w, r)
	if !ok {
		return
	}
	items, err := a.store.PaymentOrders(r.Context(), &userID)
	if err != nil {
		writeResponse(w, Error(-2, err.Error()))
		return
	}
	writeResponse(w, OK(items))
}

func (a *API) adminPaymentOrders(w http.ResponseWriter, r *http.Request) {
	items, err := a.store.PaymentOrders(r.Context(), nil)
	if err != nil {
		writeResponse(w, Error(-2, err.Error()))
		return
	}
	writeResponse(w, OK(items))
}

func (a *API) retryPaymentOrder(w http.ResponseWriter, r *http.Request) {
	result, err := a.payments.Retry(r.Context(), chi.URLParam(r, "orderNo"))
	if err != nil {
		writeResponse(w, Failure(err.Error()))
		return
	}
	writeResponse(w, OK(result))
}

func (a *API) completeTestPaymentOrder(w http.ResponseWriter, r *http.Request) {
	result, err := a.payments.CompleteTest(r.Context(), chi.URLParam(r, "orderNo"))
	if err != nil {
		writeResponse(w, Failure(err.Error()))
		return
	}
	a.provisionAfterPayment(r, result)
	writeResponse(w, OK(result))
}

func (a *API) paymentWechatCallback(w http.ResponseWriter, r *http.Request) {
	a.paymentRawCallback(w, r, "wechat")
}
func (a *API) paymentStripeCallback(w http.ResponseWriter, r *http.Request) {
	a.paymentRawCallback(w, r, "stripe")
}
func (a *API) paymentRawCallback(w http.ResponseWriter, r *http.Request, provider string) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 2<<20))
	if err != nil {
		writeResponse(w, Failure("支付回调格式错误"))
		return
	}
	result, err := a.payments.Callback(r.Context(), provider, payment.Callback{RawBody: string(body), Headers: r.Header.Clone()})
	if err != nil {
		writeResponse(w, Failure(err.Error()))
		return
	}
	a.provisionAfterPayment(r, result)
	writeResponse(w, OK(result))
}

func (a *API) paymentFormCallback(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeResponse(w, Failure("支付回调格式错误"))
		return
	}
	values := make(map[string]string, len(r.Form))
	for key := range r.Form {
		values[key] = r.Form.Get(key)
	}
	result, err := a.payments.Callback(r.Context(), chi.URLParam(r, "provider"), payment.Callback{Values: values, Headers: r.Header.Clone()})
	if err != nil {
		writeResponse(w, Failure(err.Error()))
		return
	}
	a.provisionAfterPayment(r, result)
	writeResponse(w, OK(result))
}

func (a *API) provisionAfterPayment(r *http.Request, order *store.PaymentOrder) {
	if order == nil {
		return
	}
	if err := a.provisionAutoTargetsForUser(r, order.UserID); err != nil {
		a.logger.Error("automatic protocol provisioning failed", "user_id", order.UserID, "order_no", order.OrderNo, "error", err)
	}
}
