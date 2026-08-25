package com.admin.service;

import com.admin.entity.PaymentOrder;
import com.admin.entity.SubscriptionPlan;
import com.admin.mapper.PaymentOrderMapper;
import com.admin.mapper.SubscriptionPlanMapper;
import com.baomidou.mybatisplus.core.conditions.query.QueryWrapper;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.util.*;
import javax.crypto.Mac;
import javax.crypto.spec.SecretKeySpec;

@Service
public class PaymentService {
    private final PaymentOrderMapper orders;
    private final SubscriptionPlanMapper plans;
    private final SubscriptionService subscriptions;
    private final ViteConfigService configs;
    public PaymentService(PaymentOrderMapper orders, SubscriptionPlanMapper plans, SubscriptionService subscriptions, ViteConfigService configs) { this.orders=orders; this.plans=plans; this.subscriptions=subscriptions; this.configs=configs; }

    public PaymentOrder create(long userId, long planId, String provider) {
        SubscriptionPlan plan = plans.selectById(planId);
        if (plan == null || plan.getStatus() == null || plan.getStatus() != 1 || plan.getForSale() == null || plan.getForSale() != 1) throw new IllegalArgumentException("套餐不可购买");
        String p = provider == null ? "manual" : provider.trim().toLowerCase(Locale.ROOT);
        if (!Set.of("alipay","wechat","easypay","stripe","manual").contains(p)) throw new IllegalArgumentException("不支持的支付方式");
        if (!providerEnabled(p)) throw new IllegalArgumentException("该支付方式未启用");
        PaymentOrder order = new PaymentOrder(); long now=System.currentTimeMillis();
        order.setOrderNo("TMS" + now + randomSuffix()); order.setUserId(userId); order.setPlanId(planId); order.setProvider(p); order.setAmount(plan.getPrice()); order.setCurrency(plan.getCurrency()); order.setStatus("pending"); order.setCreatedTime(now); order.setUpdatedTime(now); orders.insert(order); return order;
    }
    public List<Map<String, String>> enabledProviders() {
        List<Map<String, String>> result = new ArrayList<>();
        for (String provider : Arrays.asList("alipay", "wechat", "easypay", "stripe", "manual")) {
            if (providerEnabled(provider)) { Map<String, String> item = new LinkedHashMap<>(); item.put("key", provider); item.put("label", providerLabel(provider)); result.add(item); }
        }
        return result;
    }
    @Transactional
    public PaymentOrder completeTestOrder(String orderNo) {
        if (!"true".equalsIgnoreCase(config("payment_test_mode", "false"))) throw new IllegalArgumentException("测试支付未启用");
        PaymentOrder order = orders.selectOne(new QueryWrapper<PaymentOrder>().eq("order_no", orderNo).last("limit 1"));
        if (order == null) throw new IllegalArgumentException("订单不存在");
        if ("paid".equals(order.getStatus())) return order;
        long now = System.currentTimeMillis();
        com.baomidou.mybatisplus.core.conditions.update.UpdateWrapper<PaymentOrder> paid = new com.baomidou.mybatisplus.core.conditions.update.UpdateWrapper<PaymentOrder>().eq("id", order.getId()).eq("status", "pending").set("status", "paid").set("provider_trade_no", "test-" + order.getOrderNo()).set("paid_at", now).set("updated_time", now);
        if (orders.update(null, paid) != 1) return orders.selectById(order.getId());
        order.setStatus("paid"); order.setPaidAt(now); subscriptions.activate(order.getUserId(), order.getPlanId()); return order;
    }

    @Transactional
    public PaymentOrder callback(String provider, String orderNo, String tradeNo, String signature, String payload) {
        PaymentOrder order=orders.selectOne(new QueryWrapper<PaymentOrder>().eq("order_no", orderNo).last("limit 1"));
        if(order==null) throw new IllegalArgumentException("订单不存在");
        if(!provider.equalsIgnoreCase(order.getProvider())) throw new IllegalArgumentException("支付渠道不匹配");
        if("paid".equals(order.getStatus())) return order;
        if(signature==null || signature.isBlank()) throw new IllegalArgumentException("缺少支付签名");
        String secret = Optional.ofNullable(configs.getOne(new QueryWrapper<com.admin.entity.ViteConfig>().eq("name", "payment_" + provider.toLowerCase(Locale.ROOT) + "_secret"))).map(com.admin.entity.ViteConfig::getValue).orElse("");
        if (secret.isBlank() || !constantTime(signature, hmacSha256(secret, orderNo + ":" + (tradeNo == null ? "" : tradeNo)))) throw new IllegalArgumentException("支付签名校验失败");
        long now = System.currentTimeMillis();
        com.baomidou.mybatisplus.core.conditions.update.UpdateWrapper<PaymentOrder> paid = new com.baomidou.mybatisplus.core.conditions.update.UpdateWrapper<PaymentOrder>()
                .eq("id", order.getId()).eq("status", "pending").set("status", "paid").set("provider_trade_no", tradeNo)
                .set("callback_payload", payload).set("paid_at", now).set("updated_time", now);
        if (orders.update(null, paid) != 1) return orders.selectById(order.getId());
        order.setStatus("paid"); order.setProviderTradeNo(tradeNo); order.setCallbackPayload(payload); order.setPaidAt(now); order.setUpdatedTime(now);
        subscriptions.activate(order.getUserId(), order.getPlanId());
        return order;
    }
    public PaymentOrder get(long userId, String orderNo) { return orders.selectOne(new QueryWrapper<PaymentOrder>().eq("user_id",userId).eq("order_no",orderNo).last("limit 1")); }
    public List<PaymentOrder> listOrders() { return orders.selectList(new QueryWrapper<PaymentOrder>().orderByDesc("id")); }
    private boolean providerEnabled(String provider) { return "true".equalsIgnoreCase(config("payment_" + provider + "_enabled", "manual".equals(provider) ? "true" : "false")); }
    private String providerLabel(String provider) { return Map.of("alipay", "支付宝", "wechat", "微信支付", "easypay", "易支付", "stripe", "Stripe", "manual", "人工支付").get(provider); }
    private String config(String name, String fallback) { return Optional.ofNullable(configs.getOne(new QueryWrapper<com.admin.entity.ViteConfig>().eq("name", name))).map(com.admin.entity.ViteConfig::getValue).filter(v -> !v.isBlank()).orElse(fallback); }
    private String randomSuffix(){ return UUID.randomUUID().toString().replace("-","").substring(0,10).toUpperCase(Locale.ROOT); }
    private String hmacSha256(String secret, String value) { try { Mac mac=Mac.getInstance("HmacSHA256"); mac.init(new SecretKeySpec(secret.getBytes(StandardCharsets.UTF_8),"HmacSHA256")); byte[] bytes=mac.doFinal(value.getBytes(StandardCharsets.UTF_8)); StringBuilder b=new StringBuilder(); for(byte x:bytes)b.append(String.format("%02x",x)); return b.toString(); } catch(Exception e){ throw new IllegalStateException(e); } }
    private boolean constantTime(String a,String b){ return MessageDigest.isEqual(a.getBytes(StandardCharsets.UTF_8),b.getBytes(StandardCharsets.UTF_8)); }
}
