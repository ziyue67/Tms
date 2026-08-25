package com.admin.service;

import com.admin.entity.PaymentOrder;
import com.admin.entity.SubscriptionPlan;
import com.admin.mapper.PaymentOrderMapper;
import com.admin.mapper.SubscriptionPlanMapper;
import com.admin.service.payment.PaymentCallback;
import com.admin.service.payment.PaymentCheckout;
import com.admin.service.payment.PaymentConfig;
import com.admin.service.payment.PaymentProviderAdapter;
import com.baomidou.mybatisplus.core.conditions.query.QueryWrapper;
import com.baomidou.mybatisplus.core.conditions.update.UpdateWrapper;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.util.ArrayList;
import java.util.Arrays;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Locale;
import java.util.Map;
import java.util.UUID;
import java.util.stream.Collectors;

@Service
public class PaymentService {
    private final PaymentOrderMapper orders;
    private final SubscriptionPlanMapper plans;
    private final SubscriptionService subscriptions;
    private final PaymentConfig config;
    private final Map<String, PaymentProviderAdapter> adapters;

    public PaymentService(PaymentOrderMapper orders, SubscriptionPlanMapper plans, SubscriptionService subscriptions,
                          PaymentConfig config, List<PaymentProviderAdapter> adapters) {
        this.orders = orders;
        this.plans = plans;
        this.subscriptions = subscriptions;
        this.config = config;
        this.adapters = adapters.stream().collect(Collectors.toMap(PaymentProviderAdapter::key, adapter -> adapter));
    }

    public Map<String, Object> createCheckout(long userId, long planId, String provider) {
        PaymentOrder order = createOrder(userId, planId, provider);
        SubscriptionPlan plan = plans.selectById(planId);
        Map<String, Object> result = new LinkedHashMap<>();
        result.put("order", order);
        if ("manual".equals(order.getProvider())) {
            result.put("checkout", new PaymentCheckout("manual", null, null, "请联系管理员确认人工付款"));
            return result;
        }
        try {
            result.put("checkout", adapter(order.getProvider()).createCheckout(order, plan));
            return result;
        } catch (IllegalArgumentException error) {
            order.setStatus("failed"); order.setCallbackPayload(error.getMessage()); order.setUpdatedTime(System.currentTimeMillis());
            orders.updateById(order);
            throw error;
        }
    }

    public PaymentOrder createOrder(long userId, long planId, String provider) {
        SubscriptionPlan plan = plans.selectById(planId);
        if (plan == null || !Integer.valueOf(1).equals(plan.getStatus()) || !Integer.valueOf(1).equals(plan.getForSale())) throw new IllegalArgumentException("套餐不可购买");
        String channel = provider == null ? "manual" : provider.trim().toLowerCase(Locale.ROOT);
        if (!Arrays.asList("alipay", "wechat", "easypay", "stripe", "manual").contains(channel)) throw new IllegalArgumentException("不支持的支付方式");
        if (!providerEnabled(channel)) throw new IllegalArgumentException("该支付方式未启用");
        long now = System.currentTimeMillis();
        PaymentOrder order = new PaymentOrder();
        order.setOrderNo("TMS" + now + randomSuffix()); order.setUserId(userId); order.setPlanId(planId);
        order.setProvider(channel); order.setAmount(plan.getPrice()); order.setCurrency(plan.getCurrency()); order.setStatus("pending");
        order.setCreatedTime(now); order.setUpdatedTime(now); orders.insert(order);
        return order;
    }

    public Map<String, Object> retryCheckout(String orderNo) {
        PaymentOrder order = find(orderNo);
        if ("paid".equals(order.getStatus())) throw new IllegalArgumentException("已支付订单不能重试");
        if (!"pending".equals(order.getStatus())) {
            order.setStatus("pending"); order.setCallbackPayload(null); order.setUpdatedTime(System.currentTimeMillis()); orders.updateById(order);
        }
        Map<String, Object> result = new LinkedHashMap<>(); result.put("order", order);
        if ("manual".equals(order.getProvider())) {
            result.put("checkout", new PaymentCheckout("manual", null, null, "请联系管理员确认人工付款"));
        } else {
            try { result.put("checkout", adapter(order.getProvider()).createCheckout(order, plans.selectById(order.getPlanId()))); }
            catch (IllegalArgumentException error) { order.setStatus("failed"); order.setCallbackPayload(error.getMessage()); order.setUpdatedTime(System.currentTimeMillis()); orders.updateById(order); throw error; }
        }
        return result;
    }

    public List<Map<String, String>> enabledProviders() {
        List<Map<String, String>> result = new ArrayList<>();
        for (String provider : Arrays.asList("alipay", "wechat", "easypay", "stripe", "manual")) {
            if (providerEnabled(provider)) {
                Map<String, String> item = new LinkedHashMap<>(); item.put("key", provider); item.put("label", providerLabel(provider)); result.add(item);
            }
        }
        return result;
    }

    @Transactional
    public PaymentOrder completeTestOrder(String orderNo) {
        if (!"true".equalsIgnoreCase(config.get("payment_test_mode", "false"))) throw new IllegalArgumentException("测试支付未启用");
        PaymentOrder order = find(orderNo);
        if ("paid".equals(order.getStatus())) return order;
        return markPaid(order, "test-" + order.getOrderNo(), "{\"provider\":\"test\"}");
    }

    @Transactional
    public PaymentOrder callback(String provider, PaymentCallback callback) {
        String channel = provider == null ? "" : provider.toLowerCase(Locale.ROOT);
        PaymentProviderAdapter adapter = adapter(channel);
        String orderNo = adapter.orderNo(callback);
        if (orderNo == null || orderNo.isBlank()) throw new IllegalArgumentException("支付回调缺少订单号");
        PaymentOrder order = find(orderNo);
        if (!channel.equals(order.getProvider())) throw new IllegalArgumentException("支付渠道不匹配");
        if ("paid".equals(order.getStatus())) return order;
        adapter.verify(order, callback);
        String tradeNo = adapter.tradeNo(callback);
        if (tradeNo == null || tradeNo.isBlank()) throw new IllegalArgumentException("支付回调缺少交易号");
        return markPaid(order, tradeNo, callback.getRawBody());
    }

    private PaymentOrder markPaid(PaymentOrder order, String tradeNo, String payload) {
        if ("paid".equals(order.getStatus())) return order;
        long now = System.currentTimeMillis();
        UpdateWrapper<PaymentOrder> paid = new UpdateWrapper<PaymentOrder>().eq("id", order.getId()).eq("status", "pending")
                .set("status", "paid").set("provider_trade_no", tradeNo).set("callback_payload", payload)
                .set("paid_at", now).set("updated_time", now);
        if (orders.update(null, paid) != 1) return orders.selectById(order.getId());
        order.setStatus("paid"); order.setProviderTradeNo(tradeNo); order.setCallbackPayload(payload); order.setPaidAt(now); order.setUpdatedTime(now);
        subscriptions.activate(order.getUserId(), order.getPlanId());
        return order;
    }

    public PaymentOrder get(long userId, String orderNo) { return orders.selectOne(new QueryWrapper<PaymentOrder>().eq("user_id", userId).eq("order_no", orderNo).last("limit 1")); }
    public List<PaymentOrder> listOrders() { return orders.selectList(new QueryWrapper<PaymentOrder>().orderByDesc("id")); }
    private PaymentOrder find(String orderNo) { PaymentOrder order = orders.selectOne(new QueryWrapper<PaymentOrder>().eq("order_no", orderNo).last("limit 1")); if (order == null) throw new IllegalArgumentException("订单不存在"); return order; }
    private PaymentProviderAdapter adapter(String provider) { PaymentProviderAdapter adapter = adapters.get(provider); if (adapter == null) throw new IllegalArgumentException("不支持的支付方式"); return adapter; }
    private boolean providerEnabled(String provider) { return "manual".equals(provider) ? "true".equalsIgnoreCase(config.get("payment_manual_enabled", "true")) : config.enabled(provider); }
    private String providerLabel(String provider) { return Map.of("alipay", "支付宝", "wechat", "微信支付", "easypay", "易支付", "stripe", "Stripe", "manual", "人工支付").get(provider); }
    private String randomSuffix() { return UUID.randomUUID().toString().replace("-", "").substring(0, 10).toUpperCase(Locale.ROOT); }
}
