package com.admin.controller;

import com.admin.common.annotation.RequireRole;
import com.admin.common.lang.R;
import com.admin.entity.SubscriptionPlan;
import com.admin.entity.RedeemCode;
import com.admin.entity.UserSubscription;
import com.admin.service.PaymentService;
import com.admin.service.SubscriptionService;
import com.baomidou.mybatisplus.core.conditions.query.QueryWrapper;
import org.springframework.web.bind.annotation.*;
import java.util.Map;
import org.springframework.transaction.annotation.Transactional;

@RestController
@CrossOrigin
@RequestMapping("/api/v1/admin/subscription")
public class AdminSubscriptionController {
    private final SubscriptionService service;
    private final com.admin.mapper.SubscriptionPlanMapper plans;
    private final PaymentService payments;
    public AdminSubscriptionController(SubscriptionService service, com.admin.mapper.SubscriptionPlanMapper plans, PaymentService payments) { this.service=service; this.plans=plans; this.payments=payments; }
    @RequireRole @GetMapping("/plans") public R list() {
        java.util.List<SubscriptionPlan> result = plans.selectList(new QueryWrapper<SubscriptionPlan>().orderByAsc("sort_order", "id"));
        result.forEach(this::defaults);
        return R.ok(result);
    }
    @RequireRole @PostMapping("/plans") public R create(@RequestBody SubscriptionPlan plan) { try { defaults(plan); validate(plan); long now = System.currentTimeMillis(); plan.setCreatedTime(now); plan.setUpdatedTime(now); plans.insert(plan); return R.ok(plan); } catch (IllegalArgumentException e) { return R.err(e.getMessage()); } catch (RuntimeException e) { return R.err("创建套餐失败: " + rootMessage(e)); } }
    @RequireRole @PutMapping("/plans/{id}") public R update(@PathVariable long id, @RequestBody SubscriptionPlan plan) { try { SubscriptionPlan existing = plans.selectById(id); if (existing == null) return R.err("套餐不存在"); mergeMissing(plan, existing); validate(plan); plan.setId(id); plan.setUpdatedTime(System.currentTimeMillis()); plans.updateById(plan); return R.ok(plans.selectById(id)); } catch (IllegalArgumentException e) { return R.err(e.getMessage()); } catch (RuntimeException e) { return R.err("更新套餐失败: " + rootMessage(e)); } }
    @RequireRole @PostMapping("/plans/{id}/disable") public R disable(@PathVariable long id) { SubscriptionPlan p = plans.selectById(id); if (p == null) return R.err("套餐不存在"); p.setStatus(0); p.setForSale(0); p.setRedeemable(0); p.setUpdatedTime(System.currentTimeMillis()); if (plans.updateById(p) != 1) return R.err("停用套餐失败"); return R.ok(plans.selectById(id)); }
    @Transactional
    @RequireRole @DeleteMapping("/plans/{id}") public R delete(@PathVariable long id) { try { SubscriptionPlan p = plans.selectById(id); if (p == null) return R.err("套餐不存在"); if (plans.deleteById(id) != 1) return R.err("删除套餐失败"); return R.ok(); } catch (RuntimeException e) { return R.err("删除套餐失败: " + rootMessage(e)); } }
    @RequireRole @PostMapping("/redeem-codes") public R createCodes(@RequestBody Map<String,Object> body) { try { if (body.get("planId") == null) return R.err("请选择套餐"); long planId=Long.parseLong(body.get("planId").toString()); SubscriptionPlan plan = plans.selectById(planId); if (plan == null || !enabled(plan.getStatus()) || !enabled(plan.getRedeemable())) return R.err("套餐不可兑换（请先在套餐编辑中启用“允许兑换”）"); int count=body.get("count")==null?1:Integer.parseInt(body.get("count").toString()); if (count < 1 || count > 1000) return R.err("生成数量必须为 1 至 1000"); String batch=body.get("batchId")==null?Long.toHexString(System.currentTimeMillis()):body.get("batchId").toString(); java.util.List<String> result=new java.util.ArrayList<>(); for(int i=0;i<count;i++) result.add(service.generateCode(planId,batch,null)); return R.ok(result); } catch (NumberFormatException e) { return R.err("套餐或数量格式错误"); } catch (RuntimeException e) { return R.err("生成兑换码失败: " + rootMessage(e)); } }
    @RequireRole @GetMapping("/redeem-codes") public R codes(@RequestParam(required = false) Long planId, @RequestParam(required = false) Integer status) { return R.ok(service.redeemCodes(planId, status)); }
    @RequireRole @PostMapping("/redeem-codes/{id}/revoke") public R revokeCode(@PathVariable long id) { try { service.revokeCode(id); return R.ok(); } catch (IllegalArgumentException e) { return R.err(e.getMessage()); } }
    @RequireRole @GetMapping("/users/{userId}") public R userSubscription(@PathVariable long userId) { return R.ok(service.latest(userId)); }
    @RequireRole @GetMapping("/users/{userId}/audit") public R audit(@PathVariable long userId) { return R.ok(service.auditLogs(userId)); }
    @RequireRole @PutMapping("/users/{userId}") public R adjustUser(@PathVariable long userId, @RequestBody Map<String,Object> body) { try { return R.ok(service.adjust(userId, body)); } catch (IllegalArgumentException e) { return R.err(e.getMessage()); } }
    @RequireRole @PostMapping("/users/{userId}/reset-quota") public R resetQuota(@PathVariable long userId) { try { return R.ok(service.resetQuota(userId)); } catch (IllegalArgumentException e) { return R.err(e.getMessage()); } }
    @RequireRole @GetMapping("/orders") public R orders() { return R.ok(payments.listOrders()); }
    @RequireRole @PostMapping("/orders/{orderNo}/retry") public R retryOrder(@PathVariable String orderNo) { try { return R.ok(payments.retryCheckout(orderNo)); } catch (IllegalArgumentException e) { return R.err(e.getMessage()); } }
    @RequireRole @PostMapping("/orders/{orderNo}/complete-test") public R completeTestOrder(@PathVariable String orderNo) { try { return R.ok(payments.completeTestOrder(orderNo)); } catch (IllegalArgumentException e) { return R.err(e.getMessage()); } }

    private void validate(SubscriptionPlan plan) {
        if (plan.getName() == null || plan.getName().trim().isEmpty()) throw new IllegalArgumentException("套餐名称不能为空");
        if (plan.getValidityUnit() == null || !("month".equalsIgnoreCase(plan.getValidityUnit()) || "year".equalsIgnoreCase(plan.getValidityUnit()) || "permanent".equalsIgnoreCase(plan.getValidityUnit()))) throw new IllegalArgumentException("有效期单位必须是 month、year 或 permanent");
        if ("permanent".equalsIgnoreCase(plan.getValidityUnit())) {
            if (plan.getValidityValue() == null || plan.getValidityValue() < 0) throw new IllegalArgumentException("永久套餐有效期数值必须为 0 或更大");
        } else if (plan.getValidityValue() == null || plan.getValidityValue() < 1) throw new IllegalArgumentException("有效期必须大于 0");
        if (plan.getTrafficBytes() != null && plan.getTrafficBytes() < 0) throw new IllegalArgumentException("流量上限不能小于 0");
        if (plan.getResetQuota() != null && plan.getResetQuota() != 0 && plan.getResetQuota() != 1) throw new IllegalArgumentException("流量重置模式参数错误");
        if (plan.getMaxForwards() != null && plan.getMaxForwards() < 0) throw new IllegalArgumentException("转发上限不能小于 0");
        if (plan.getResetDay() != null && (plan.getResetDay() < 0 || plan.getResetDay() > 31)) throw new IllegalArgumentException("流量重置日必须为 0 至 31，0 表示不重置");
    }

    private void defaults(SubscriptionPlan plan) {
        if (plan.getCurrency() == null || plan.getCurrency().isBlank()) plan.setCurrency("CNY");
        if (plan.getValidityValue() == null) plan.setValidityValue(1);
        if (plan.getValidityUnit() == null) plan.setValidityUnit("month");
        if (plan.getTrafficBytes() == null) plan.setTrafficBytes(0L);
        if (plan.getResetDay() == null) plan.setResetDay(1);
        if (plan.getResetQuota() == null) plan.setResetQuota(1);
        if (plan.getMaxForwards() == null) plan.setMaxForwards(0);
        if (plan.getForSale() == null) plan.setForSale(1);
        if (plan.getRedeemable() == null) plan.setRedeemable(1);
        if (plan.getSortOrder() == null) plan.setSortOrder(0);
        if (plan.getStatus() == null) plan.setStatus(1);
        if (plan.getPrice() == null) plan.setPrice(java.math.BigDecimal.ZERO);
    }

    private void mergeMissing(SubscriptionPlan incoming, SubscriptionPlan existing) {
        if (incoming.getName() == null) incoming.setName(existing.getName());
        if (incoming.getDescription() == null) incoming.setDescription(existing.getDescription());
        if (incoming.getPrice() == null) incoming.setPrice(existing.getPrice());
        if (incoming.getCurrency() == null) incoming.setCurrency(existing.getCurrency());
        if (incoming.getValidityValue() == null) incoming.setValidityValue(existing.getValidityValue());
        if (incoming.getValidityUnit() == null) incoming.setValidityUnit(existing.getValidityUnit());
        if (incoming.getTrafficBytes() == null) incoming.setTrafficBytes(existing.getTrafficBytes());
        if (incoming.getResetDay() == null) incoming.setResetDay(existing.getResetDay());
        if (incoming.getResetQuota() == null) incoming.setResetQuota(existing.getResetQuota() == null ? 1 : existing.getResetQuota());
        if (incoming.getMaxForwards() == null) incoming.setMaxForwards(existing.getMaxForwards());
        if (incoming.getForSale() == null) incoming.setForSale(existing.getForSale());
        if (incoming.getRedeemable() == null) incoming.setRedeemable(existing.getRedeemable());
        if (incoming.getSortOrder() == null) incoming.setSortOrder(existing.getSortOrder());
        if (incoming.getStatus() == null) incoming.setStatus(existing.getStatus());
    }

    private String rootMessage(Throwable error) { Throwable t = error; while (t.getCause() != null) t = t.getCause(); return t.getMessage() == null ? t.getClass().getSimpleName() : t.getMessage(); }
    private boolean enabled(Integer value) { return value == null || value == 1; }
}
