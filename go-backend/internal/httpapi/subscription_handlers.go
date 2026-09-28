package httpapi

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/ziyue67/tms/go-backend/internal/store"
)

type planRequest struct {
	Name          *string  `json:"name"`
	Description   *string  `json:"description"`
	Price         *float64 `json:"price"`
	Currency      *string  `json:"currency"`
	ValidityValue *int     `json:"validityValue"`
	ValidityUnit  *string  `json:"validityUnit"`
	TrafficBytes  *int64   `json:"trafficBytes"`
	ResetDay      *int     `json:"resetDay"`
	ResetQuota    *int     `json:"resetQuota"`
	MaxForwards   *int     `json:"maxForwards"`
	ForSale       *int     `json:"forSale"`
	Redeemable    *int     `json:"redeemable"`
	SortOrder     *int     `json:"sortOrder"`
	Status        *int     `json:"status"`
}

func (a *API) publicSubscriptionPlans(w http.ResponseWriter, r *http.Request) {
	plans, err := a.store.SubscriptionPlans(r.Context(), true)
	if err != nil {
		writeResponse(w, Error(-2, err.Error()))
		return
	}
	writeResponse(w, OK(plans))
}

func (a *API) adminSubscriptionPlans(w http.ResponseWriter, r *http.Request) {
	plans, err := a.store.SubscriptionPlans(r.Context(), false)
	if err != nil {
		writeResponse(w, Error(-2, err.Error()))
		return
	}
	writeResponse(w, OK(plans))
}

func (a *API) createSubscriptionPlan(w http.ResponseWriter, r *http.Request) {
	var request planRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	input, err := request.withDefaults().validate()
	if err != nil {
		writeResponse(w, Failure(err.Error()))
		return
	}
	plan, err := a.store.CreateSubscriptionPlan(r.Context(), input)
	if err != nil {
		writeResponse(w, Failure("创建套餐失败: "+err.Error()))
		return
	}
	writeResponse(w, OK(plan))
}

func (a *API) updateSubscriptionPlan(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt64(w, r, "id")
	if !ok {
		return
	}
	existing, err := a.store.SubscriptionPlanByID(r.Context(), id)
	if store.IsNotFound(err) {
		writeResponse(w, Failure("套餐不存在"))
		return
	}
	if err != nil {
		writeResponse(w, Failure("更新套餐失败: "+err.Error()))
		return
	}
	var request planRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	input, err := request.merge(existing).validate()
	if err != nil {
		writeResponse(w, Failure(err.Error()))
		return
	}
	plan, err := a.store.UpdateSubscriptionPlan(r.Context(), id, input)
	if err != nil {
		writeResponse(w, Failure("更新套餐失败: "+err.Error()))
		return
	}
	writeResponse(w, OK(plan))
}

func (a *API) disableSubscriptionPlan(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt64(w, r, "id")
	if !ok {
		return
	}
	plan, err := a.store.DisableSubscriptionPlan(r.Context(), id)
	if store.IsNotFound(err) {
		writeResponse(w, Failure("套餐不存在"))
		return
	}
	if err != nil {
		writeResponse(w, Failure("停用套餐失败: "+err.Error()))
		return
	}
	writeResponse(w, OK(plan))
}

func (a *API) deleteSubscriptionPlan(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt64(w, r, "id")
	if !ok {
		return
	}
	deleted, err := a.store.DeleteSubscriptionPlan(r.Context(), id)
	if err != nil {
		writeResponse(w, Failure("删除套餐失败: "+err.Error()))
		return
	}
	if !deleted {
		writeResponse(w, Failure("套餐不存在"))
		return
	}
	writeResponse(w, OK(nil))
}

func (a *API) createRedeemCodes(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if !decodeJSON(w, r, &body) {
		return
	}
	planID, err := requestInt64(body["planId"])
	if err != nil {
		writeResponse(w, Failure("请选择套餐"))
		return
	}
	plan, err := a.store.SubscriptionPlanByID(r.Context(), planID)
	if err != nil || plan.Status != 1 || plan.Redeemable != 1 {
		writeResponse(w, Failure("套餐不可兑换（请先在套餐编辑中启用“允许兑换”）"))
		return
	}
	count := int64(1)
	if body["count"] != nil {
		count, err = requestInt64(body["count"])
	}
	if err != nil {
		writeResponse(w, Failure("套餐或数量格式错误"))
		return
	}
	if count < 1 || count > 1000 {
		writeResponse(w, Failure("生成数量必须为 1 至 1000"))
		return
	}
	batch := fmt.Sprintf("%x", time.Now().UnixMilli())
	if body["batchId"] != nil {
		batch = fmt.Sprint(body["batchId"])
	}
	codes, err := a.store.GenerateRedeemCodes(r.Context(), planID, batch, int(count))
	if err != nil {
		writeResponse(w, Failure("生成兑换码失败: "+err.Error()))
		return
	}
	writeResponse(w, OK(codes))
}

func (a *API) listRedeemCodes(w http.ResponseWriter, r *http.Request) {
	planID, planOK := optionalQueryInt64(r, "planId")
	status64, statusOK := optionalQueryInt64(r, "status")
	var status *int
	if statusOK {
		value := int(*status64)
		status = &value
	}
	var plan *int64
	if planOK {
		plan = planID
	}
	codes, err := a.store.RedeemCodes(r.Context(), plan, status)
	if err != nil {
		writeResponse(w, Error(-2, err.Error()))
		return
	}
	writeResponse(w, OK(codes))
}

func (a *API) revokeRedeemCode(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt64(w, r, "id")
	if !ok {
		return
	}
	if err := a.store.RevokeRedeemCode(r.Context(), id); err != nil {
		writeResponse(w, Failure(err.Error()))
		return
	}
	writeResponse(w, OK(nil))
}

func (a *API) deleteRedeemCode(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt64(w, r, "id")
	if !ok {
		return
	}
	if err := a.store.DeleteRedeemCode(r.Context(), id); err != nil {
		writeResponse(w, Failure(err.Error()))
		return
	}
	writeResponse(w, OK(nil))
}

func (a *API) currentSubscription(w http.ResponseWriter, r *http.Request) {
	userID, ok := currentUserID(w, r)
	if !ok {
		return
	}
	item, err := a.store.CurrentSubscription(r.Context(), userID, true)
	if err != nil {
		writeResponse(w, Error(-2, err.Error()))
		return
	}
	writeResponse(w, OK(item))
}

func (a *API) adminUserSubscription(w http.ResponseWriter, r *http.Request) {
	userID, ok := pathInt64(w, r, "userId")
	if !ok {
		return
	}
	item, err := a.store.CurrentSubscription(r.Context(), userID, false)
	if err != nil {
		writeResponse(w, Error(-2, err.Error()))
		return
	}
	writeResponse(w, OK(item))
}

func (a *API) subscriptionAudit(w http.ResponseWriter, r *http.Request) {
	userID, ok := pathInt64(w, r, "userId")
	if !ok {
		return
	}
	items, err := a.store.SubscriptionAudit(r.Context(), userID)
	if err != nil {
		writeResponse(w, Error(-2, err.Error()))
		return
	}
	writeResponse(w, OK(items))
}

func (a *API) adjustSubscription(w http.ResponseWriter, r *http.Request) {
	userID, ok := pathInt64(w, r, "userId")
	if !ok {
		return
	}
	values := make(map[string]any)
	if !decodeJSON(w, r, &values) {
		return
	}
	item, err := a.store.AdjustSubscription(r.Context(), userID, values)
	if err != nil {
		writeResponse(w, Failure(err.Error()))
		return
	}
	if err := a.provisionAutoTargetsForUser(r, userID); err != nil {
		a.logger.Error("automatic protocol provisioning failed", "user_id", userID, "error", err)
	}
	writeResponse(w, OK(item))
}

func (a *API) removeSubscription(w http.ResponseWriter, r *http.Request) {
	userID, ok := pathInt64(w, r, "userId")
	if !ok {
		return
	}
	if err := a.store.RemoveSubscription(r.Context(), userID); err != nil {
		writeResponse(w, Failure(err.Error()))
		return
	}
	writeResponse(w, OK(nil))
}

func (a *API) resetSubscriptionQuota(w http.ResponseWriter, r *http.Request) {
	userID, ok := pathInt64(w, r, "userId")
	if !ok {
		return
	}
	item, err := a.store.ResetSubscriptionQuota(r.Context(), userID)
	if err != nil {
		writeResponse(w, Failure(err.Error()))
		return
	}
	writeResponse(w, OK(item))
}

func (a *API) redeemSubscription(w http.ResponseWriter, r *http.Request) {
	userID, ok := currentUserID(w, r)
	if !ok {
		return
	}
	var body map[string]string
	if !decodeJSON(w, r, &body) {
		return
	}
	item, err := a.store.RedeemSubscription(r.Context(), userID, body["code"])
	if err != nil {
		writeResponse(w, Failure(err.Error()))
		return
	}
	if err := a.provisionAutoTargetsForUser(r, userID); err != nil {
		a.logger.Error("automatic protocol provisioning failed", "user_id", userID, "error", err)
	}
	writeResponse(w, OK(item))
}

func (a *API) subscriptionDashboard(w http.ResponseWriter, r *http.Request) {
	userID, ok := currentUserID(w, r)
	if !ok {
		return
	}
	dashboard, err := a.store.SubscriptionDashboard(r.Context(), userID)
	if err != nil {
		writeResponse(w, Error(-2, err.Error()))
		return
	}
	writeResponse(w, OK(dashboard))
}

func (r planRequest) withDefaults() planRequest {
	defaultString(&r.Currency, "CNY")
	defaultString(&r.ValidityUnit, "month")
	defaultInt(&r.ValidityValue, 1)
	defaultInt64(&r.TrafficBytes, 0)
	defaultInt(&r.ResetDay, 1)
	defaultInt(&r.ResetQuota, 1)
	defaultInt(&r.MaxForwards, 0)
	defaultInt(&r.ForSale, 1)
	defaultInt(&r.Redeemable, 1)
	defaultInt(&r.SortOrder, 0)
	defaultInt(&r.Status, 1)
	if r.Price == nil {
		value := float64(0)
		r.Price = &value
	}
	return r
}

func (r planRequest) merge(existing store.SubscriptionPlan) planRequest {
	if r.Name == nil {
		r.Name = &existing.Name
	}
	if r.Description == nil {
		r.Description = existing.Description
	}
	if r.Price == nil {
		r.Price = &existing.Price
	}
	if r.Currency == nil {
		r.Currency = &existing.Currency
	}
	if r.ValidityValue == nil {
		r.ValidityValue = &existing.ValidityValue
	}
	if r.ValidityUnit == nil {
		r.ValidityUnit = &existing.ValidityUnit
	}
	if r.TrafficBytes == nil {
		r.TrafficBytes = &existing.TrafficBytes
	}
	if r.ResetDay == nil {
		r.ResetDay = &existing.ResetDay
	}
	if r.ResetQuota == nil {
		r.ResetQuota = &existing.ResetQuota
	}
	if r.MaxForwards == nil {
		r.MaxForwards = &existing.MaxForwards
	}
	if r.ForSale == nil {
		r.ForSale = &existing.ForSale
	}
	if r.Redeemable == nil {
		r.Redeemable = &existing.Redeemable
	}
	if r.SortOrder == nil {
		r.SortOrder = &existing.SortOrder
	}
	if r.Status == nil {
		r.Status = &existing.Status
	}
	return r
}

func (r planRequest) validate() (store.PlanInput, error) {
	if r.Name == nil || strings.TrimSpace(*r.Name) == "" {
		return store.PlanInput{}, fmt.Errorf("套餐名称不能为空")
	}
	unit := strings.ToLower(*r.ValidityUnit)
	if unit != "month" && unit != "year" && unit != "permanent" {
		return store.PlanInput{}, fmt.Errorf("有效期单位必须是 month、year 或 permanent")
	}
	if unit == "permanent" && *r.ValidityValue < 0 {
		return store.PlanInput{}, fmt.Errorf("永久套餐有效期数值必须为 0 或更大")
	}
	if unit != "permanent" && *r.ValidityValue < 1 {
		return store.PlanInput{}, fmt.Errorf("有效期必须大于 0")
	}
	if *r.TrafficBytes < 0 {
		return store.PlanInput{}, fmt.Errorf("流量上限不能小于 0")
	}
	if *r.ResetQuota != 0 && *r.ResetQuota != 1 {
		return store.PlanInput{}, fmt.Errorf("流量重置模式参数错误")
	}
	if *r.MaxForwards < 0 {
		return store.PlanInput{}, fmt.Errorf("转发上限不能小于 0")
	}
	if *r.ResetDay < 0 || *r.ResetDay > 31 {
		return store.PlanInput{}, fmt.Errorf("流量重置日必须为 0 至 31，0 表示不重置")
	}
	return store.PlanInput{Name: strings.TrimSpace(*r.Name), Description: r.Description, Price: *r.Price, Currency: *r.Currency,
		ValidityValue: *r.ValidityValue, ValidityUnit: unit, TrafficBytes: *r.TrafficBytes, ResetDay: *r.ResetDay,
		ResetQuota: *r.ResetQuota, MaxForwards: *r.MaxForwards, ForSale: *r.ForSale, Redeemable: *r.Redeemable,
		SortOrder: *r.SortOrder, Status: *r.Status}, nil
}

func pathInt64(w http.ResponseWriter, r *http.Request, name string) (int64, bool) {
	value, err := strconv.ParseInt(chi.URLParam(r, name), 10, 64)
	if err != nil {
		writeResponse(w, Error(500, "参数格式错误"))
		return 0, false
	}
	return value, true
}

func optionalQueryInt64(r *http.Request, name string) (*int64, bool) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return nil, false
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return nil, false
	}
	return &value, true
}

func requestInt64(value any) (int64, error) {
	switch typed := value.(type) {
	case float64:
		return int64(typed), nil
	case string:
		return strconv.ParseInt(typed, 10, 64)
	default:
		return 0, fmt.Errorf("invalid number")
	}
}

func currentUserID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	claims, ok := claimsFrom(r)
	if !ok {
		writeResponse(w, Error(401, "未登录或token已过期"))
		return 0, false
	}
	id, err := claims.UserID()
	if err != nil {
		writeResponse(w, Error(401, "无效的token或token已过期"))
		return 0, false
	}
	return id, true
}

func defaultString(value **string, fallback string) {
	if *value == nil || strings.TrimSpace(**value) == "" {
		copy := fallback
		*value = &copy
	}
}
func defaultInt(value **int, fallback int) {
	if *value == nil {
		copy := fallback
		*value = &copy
	}
}
func defaultInt64(value **int64, fallback int64) {
	if *value == nil {
		copy := fallback
		*value = &copy
	}
}
