package com.admin.service.payment;

import com.admin.entity.PaymentOrder;
import com.admin.entity.SubscriptionPlan;
import com.alibaba.fastjson.JSON;
import java.math.BigDecimal;
import java.util.LinkedHashMap;
import java.util.Map;
import org.springframework.stereotype.Component;

/** Alipay page-pay adapter using the official RSA2 request and callback format. */
@Component
public class AlipayPaymentProvider implements PaymentProviderAdapter {
    private final PaymentConfig config;
    public AlipayPaymentProvider(PaymentConfig config) { this.config = config; }
    @Override public String key() { return "alipay"; }

    @Override
    public PaymentCheckout createCheckout(PaymentOrder order, SubscriptionPlan plan) {
        String appId = required("payment_alipay_app_id");
        String privateKey = required("payment_alipay_private_key");
        Map<String, String> fields = new LinkedHashMap<>();
        fields.put("app_id", appId);
        fields.put("method", "alipay.trade.page.pay");
        fields.put("format", "JSON");
        fields.put("charset", "utf-8");
        fields.put("sign_type", "RSA2");
        fields.put("timestamp", java.time.format.DateTimeFormatter.ofPattern("yyyy-MM-dd HH:mm:ss").format(java.time.LocalDateTime.now()));
        fields.put("version", "1.0");
        fields.put("notify_url", required("payment_alipay_notify_url"));
        String returnUrl = config.get("payment_alipay_return_url");
        if (!returnUrl.isBlank()) fields.put("return_url", returnUrl);
        Map<String, Object> biz = new LinkedHashMap<>();
        biz.put("out_trade_no", order.getOrderNo());
        biz.put("product_code", "FAST_INSTANT_TRADE_PAY");
        biz.put("total_amount", order.getAmount().setScale(2).toPlainString());
        biz.put("subject", safeSubject(plan.getName()));
        fields.put("biz_content", JSON.toJSONString(biz));
        fields.put("sign", PaymentCrypto.signRsa(PaymentCrypto.orderedQuery(fields, false), privateKey));
        return PaymentCheckout.form(config.get("payment_alipay_gateway", "https://openapi.alipay.com/gateway.do"), fields, "将跳转至支付宝完成付款");
    }

    @Override public String orderNo(PaymentCallback callback) { return callback.value("out_trade_no"); }
    @Override public String tradeNo(PaymentCallback callback) { return callback.value("trade_no"); }

    @Override
    public void verify(PaymentOrder order, PaymentCallback callback) {
        Map<String, String> values = callback.getValues();
        String status = values.get("trade_status");
        if (!"TRADE_SUCCESS".equals(status) && !"TRADE_FINISHED".equals(status)) throw new IllegalArgumentException("支付宝交易未成功");
        String signature = values.get("sign");
        if (signature == null || !PaymentCrypto.verifyRsa(PaymentCrypto.orderedQuery(values, false), signature, required("payment_alipay_public_key"))) throw new IllegalArgumentException("支付宝回调签名校验失败");
        String configuredAppId = config.get("payment_alipay_app_id");
        if (!configuredAppId.isBlank() && !configuredAppId.equals(values.get("app_id"))) throw new IllegalArgumentException("支付宝应用不匹配");
        verifyAmount(order, values.get("total_amount"));
    }

    private void verifyAmount(PaymentOrder order, String amount) {
        try {
            if (amount == null || order.getAmount().compareTo(new BigDecimal(amount)) != 0) throw new IllegalArgumentException("支付宝回调金额不匹配");
        } catch (NumberFormatException error) { throw new IllegalArgumentException("支付宝回调金额无效"); }
    }
    private String required(String name) { String value = config.get(name); if (value.isBlank()) throw new IllegalArgumentException("请先配置 " + name); return value; }
    private String safeSubject(String name) {
        String subject = name == null || name.isBlank() ? "TMS 套餐" : name;
        return subject.substring(0, Math.min(120, subject.length()));
    }
}
