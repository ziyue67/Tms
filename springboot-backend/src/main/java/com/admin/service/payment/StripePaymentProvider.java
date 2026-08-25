package com.admin.service.payment;

import com.admin.entity.PaymentOrder;
import com.admin.entity.SubscriptionPlan;
import com.alibaba.fastjson.JSON;
import com.alibaba.fastjson.JSONObject;
import java.io.InputStream;
import java.io.OutputStream;
import java.math.BigDecimal;
import java.net.HttpURLConnection;
import java.net.URL;
import java.net.URLEncoder;
import java.nio.charset.StandardCharsets;
import java.time.Instant;
import java.util.Base64;
import java.util.LinkedHashMap;
import java.util.Map;
import org.springframework.stereotype.Component;

/** Stripe Checkout and Stripe-Signature webhook implementation. */
@Component
public class StripePaymentProvider implements PaymentProviderAdapter {
    private final PaymentConfig config;
    public StripePaymentProvider(PaymentConfig config) { this.config = config; }
    @Override public String key() { return "stripe"; }

    @Override
    public PaymentCheckout createCheckout(PaymentOrder order, SubscriptionPlan plan) {
        String successUrl = required("payment_stripe_success_url");
        Map<String, String> values = new LinkedHashMap<>();
        values.put("mode", "payment");
        values.put("success_url", appendOrder(successUrl, order.getOrderNo()));
        values.put("cancel_url", required("payment_stripe_cancel_url"));
        values.put("client_reference_id", order.getOrderNo());
        values.put("metadata[orderNo]", order.getOrderNo());
        values.put("line_items[0][price_data][currency]", order.getCurrency().toLowerCase(java.util.Locale.ROOT));
        values.put("line_items[0][price_data][unit_amount]", order.getAmount().movePointRight(2).setScale(0).toPlainString());
        values.put("line_items[0][price_data][product_data][name]", plan.getName() == null ? "TMS Plan" : plan.getName());
        values.put("line_items[0][quantity]", "1");
        JSONObject response = JSON.parseObject(post("https://api.stripe.com/v1/checkout/sessions", values, required("payment_stripe_secret_key")));
        String url = response.getString("url");
        if (url == null || url.isBlank()) throw new IllegalArgumentException("Stripe 未返回支付链接");
        return PaymentCheckout.redirect(url, "将跳转至 Stripe Checkout 完成付款");
    }

    @Override
    public String orderNo(PaymentCallback callback) {
        JSONObject root = JSON.parseObject(callback.getRawBody());
        JSONObject object = root.getJSONObject("data").getJSONObject("object");
        String orderNo = object.getString("client_reference_id");
        if (orderNo == null || orderNo.isBlank()) {
            JSONObject metadata = object.getJSONObject("metadata");
            orderNo = metadata == null ? null : metadata.getString("orderNo");
        }
        return orderNo;
    }

    @Override
    public String tradeNo(PaymentCallback callback) {
        JSONObject root = JSON.parseObject(callback.getRawBody());
        return root.getJSONObject("data").getJSONObject("object").getString("payment_intent");
    }

    @Override
    public void verify(PaymentOrder order, PaymentCallback callback) {
        verifySignature(callback);
        JSONObject root = JSON.parseObject(callback.getRawBody());
        String type = root.getString("type");
        if (!"checkout.session.completed".equals(type) && !"checkout.session.async_payment_succeeded".equals(type)) throw new IllegalArgumentException("Stripe 事件未完成付款");
        JSONObject object = root.getJSONObject("data").getJSONObject("object");
        if (!"paid".equalsIgnoreCase(object.getString("payment_status"))) throw new IllegalArgumentException("Stripe 付款状态不是 paid");
        long expected = order.getAmount().movePointRight(2).setScale(0).longValueExact();
        if (object.getLongValue("amount_total") != expected) throw new IllegalArgumentException("Stripe 回调金额不匹配");
        if (!order.getCurrency().equalsIgnoreCase(object.getString("currency"))) throw new IllegalArgumentException("Stripe 回调币种不匹配");
    }

    private void verifySignature(PaymentCallback callback) {
        String header = callback.header("Stripe-Signature");
        if (header == null || header.isBlank()) throw new IllegalArgumentException("缺少 Stripe-Signature");
        String timestamp = null;
        String signature = null;
        for (String piece : header.split(",")) {
            String[] pair = piece.split("=", 2);
            if (pair.length != 2) continue;
            if ("t".equals(pair[0])) timestamp = pair[1];
            if ("v1".equals(pair[0])) signature = pair[1];
        }
        if (timestamp == null || signature == null) throw new IllegalArgumentException("Stripe-Signature 格式无效");
        long seconds;
        try { seconds = Long.parseLong(timestamp); } catch (NumberFormatException error) { throw new IllegalArgumentException("Stripe 时间戳无效"); }
        if (Math.abs(Instant.now().getEpochSecond() - seconds) > 300) throw new IllegalArgumentException("Stripe 回调已过期");
        String expected = PaymentCrypto.hmacSha256Hex(required("payment_stripe_webhook_secret"), timestamp + "." + callback.getRawBody());
        if (!PaymentCrypto.equalsConstantTime(expected, signature)) throw new IllegalArgumentException("Stripe 回调签名校验失败");
    }

    private String post(String endpoint, Map<String, String> values, String secret) {
        try {
            HttpURLConnection connection = (HttpURLConnection) new URL(endpoint).openConnection();
            connection.setRequestMethod("POST"); connection.setConnectTimeout(10000); connection.setReadTimeout(20000); connection.setDoOutput(true);
            connection.setRequestProperty("Authorization", "Basic " + Base64.getEncoder().encodeToString((secret + ":").getBytes(StandardCharsets.UTF_8)));
            connection.setRequestProperty("Content-Type", "application/x-www-form-urlencoded");
            String payload = form(values);
            try (OutputStream stream = connection.getOutputStream()) { stream.write(payload.getBytes(StandardCharsets.UTF_8)); }
            InputStream stream = connection.getResponseCode() >= 400 ? connection.getErrorStream() : connection.getInputStream();
            String body = new String(stream.readAllBytes(), StandardCharsets.UTF_8);
            if (connection.getResponseCode() >= 400) throw new IllegalArgumentException("Stripe 创建订单失败: " + body);
            return body;
        } catch (IllegalArgumentException error) { throw error; }
        catch (Exception error) { throw new IllegalArgumentException("Stripe 请求失败", error); }
    }

    private String form(Map<String, String> values) { StringBuilder result = new StringBuilder(); for (Map.Entry<String, String> item : values.entrySet()) { if (result.length() > 0) result.append('&'); result.append(URLEncoder.encode(item.getKey(), StandardCharsets.UTF_8)).append('=').append(URLEncoder.encode(item.getValue(), StandardCharsets.UTF_8)); } return result.toString(); }
    private String appendOrder(String url, String orderNo) { return url.contains("?") ? url + "&orderNo=" + URLEncoder.encode(orderNo, StandardCharsets.UTF_8) : url + "?orderNo=" + URLEncoder.encode(orderNo, StandardCharsets.UTF_8); }
    private String required(String name) { String value = config.get(name); if (value.isBlank()) throw new IllegalArgumentException("请先配置 " + name); return value; }
}
