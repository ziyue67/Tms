package com.admin.service.payment;

import com.admin.entity.PaymentOrder;
import com.admin.entity.SubscriptionPlan;
import java.math.BigDecimal;
import java.util.LinkedHashMap;
import java.util.Map;
import org.springframework.stereotype.Component;

/** Compatible with the common EasyPay API v1 MD5 signed payment endpoint. */
@Component
public class EasyPayPaymentProvider implements PaymentProviderAdapter {
    private final PaymentConfig config;
    public EasyPayPaymentProvider(PaymentConfig config) { this.config = config; }
    @Override public String key() { return "easypay"; }

    @Override
    public PaymentCheckout createCheckout(PaymentOrder order, SubscriptionPlan plan) {
        Map<String, String> fields = new LinkedHashMap<>();
        fields.put("pid", required("payment_easypay_pid"));
        fields.put("type", config.get("payment_easypay_type", "alipay"));
        fields.put("out_trade_no", order.getOrderNo());
        fields.put("notify_url", required("payment_easypay_notify_url"));
        String returnUrl = config.get("payment_easypay_return_url");
        if (!returnUrl.isBlank()) fields.put("return_url", returnUrl);
        fields.put("name", plan.getName() == null ? "TMS 套餐" : plan.getName());
        fields.put("money", order.getAmount().setScale(2).toPlainString());
        fields.put("sign_type", "MD5");
        fields.put("sign", PaymentCrypto.md5(PaymentCrypto.orderedQuery(fields, false) + required("payment_easypay_key")));
        return PaymentCheckout.form(config.get("payment_easypay_gateway", "https://your-easypay.example/api.php"), fields, "将跳转至易支付完成付款");
    }

    @Override public String orderNo(PaymentCallback callback) { return callback.value("out_trade_no"); }
    @Override public String tradeNo(PaymentCallback callback) { return callback.value("trade_no"); }

    @Override
    public void verify(PaymentOrder order, PaymentCallback callback) {
        Map<String, String> values = callback.getValues();
        String tradeStatus = values.get("trade_status");
        if (!"TRADE_SUCCESS".equalsIgnoreCase(tradeStatus) && !"SUCCESS".equalsIgnoreCase(tradeStatus)) throw new IllegalArgumentException("易支付交易未成功");
        String expected = PaymentCrypto.md5(PaymentCrypto.orderedQuery(values, false) + required("payment_easypay_key"));
        if (!PaymentCrypto.equalsConstantTime(expected, values.get("sign"))) throw new IllegalArgumentException("易支付回调签名校验失败");
        try {
            if (order.getAmount().compareTo(new BigDecimal(values.get("money"))) != 0) throw new IllegalArgumentException("易支付回调金额不匹配");
        } catch (Exception error) { if (error instanceof IllegalArgumentException) throw (IllegalArgumentException) error; throw new IllegalArgumentException("易支付回调金额无效"); }
    }

    private String required(String name) { String value = config.get(name); if (value.isBlank()) throw new IllegalArgumentException("请先配置 " + name); return value; }
}
