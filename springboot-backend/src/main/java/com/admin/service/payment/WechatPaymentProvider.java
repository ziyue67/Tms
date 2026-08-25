package com.admin.service.payment;

import com.admin.entity.PaymentOrder;
import com.admin.entity.SubscriptionPlan;
import com.alibaba.fastjson.JSON;
import com.alibaba.fastjson.JSONObject;
import java.io.InputStream;
import java.io.OutputStream;
import java.net.HttpURLConnection;
import java.net.URL;
import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.time.Instant;
import java.util.Base64;
import java.util.LinkedHashMap;
import java.util.Map;
import java.util.UUID;
import javax.crypto.Cipher;
import javax.crypto.spec.GCMParameterSpec;
import javax.crypto.spec.SecretKeySpec;
import org.springframework.stereotype.Component;

/** WeChat Pay v3 Native checkout and signed, AES-GCM encrypted notification handler. */
@Component
public class WechatPaymentProvider implements PaymentProviderAdapter {
    private static final String NATIVE_PATH = "/v3/pay/transactions/native";
    private final PaymentConfig config;
    public WechatPaymentProvider(PaymentConfig config) { this.config = config; }
    @Override public String key() { return "wechat"; }

    @Override
    public PaymentCheckout createCheckout(PaymentOrder order, SubscriptionPlan plan) {
        Map<String, Object> body = new LinkedHashMap<>();
        body.put("appid", required("payment_wechat_app_id"));
        body.put("mchid", required("payment_wechat_mchid"));
        body.put("description", plan.getName() == null ? "TMS 套餐" : plan.getName());
        body.put("out_trade_no", order.getOrderNo());
        body.put("notify_url", required("payment_wechat_notify_url"));
        Map<String, Object> amount = new LinkedHashMap<>();
        amount.put("total", order.getAmount().movePointRight(2).setScale(0).intValueExact());
        amount.put("currency", order.getCurrency()); body.put("amount", amount);
        JSONObject result = JSON.parseObject(request(JSON.toJSONString(body)));
        String codeUrl = result.getString("code_url");
        if (codeUrl == null || codeUrl.isBlank()) throw new IllegalArgumentException("微信支付未返回付款码");
        return PaymentCheckout.qr(codeUrl, "请使用微信扫码完成付款");
    }

    @Override public String orderNo(PaymentCallback callback) { return decrypted(callback).getString("out_trade_no"); }
    @Override public String tradeNo(PaymentCallback callback) { return decrypted(callback).getString("transaction_id"); }

    @Override
    public void verify(PaymentOrder order, PaymentCallback callback) {
        verifyHeader(callback);
        JSONObject value = decrypted(callback);
        if (!"SUCCESS".equals(value.getString("trade_state"))) throw new IllegalArgumentException("微信支付交易未成功");
        JSONObject amount = value.getJSONObject("amount");
        long expected = order.getAmount().movePointRight(2).setScale(0).longValueExact();
        if (amount == null || amount.getLongValue("total") != expected) throw new IllegalArgumentException("微信支付回调金额不匹配");
        if (!order.getCurrency().equalsIgnoreCase(amount.getString("currency"))) throw new IllegalArgumentException("微信支付回调币种不匹配");
        String appId = config.get("payment_wechat_app_id");
        if (!appId.equals(value.getString("appid"))) throw new IllegalArgumentException("微信支付应用不匹配");
        String mchId = config.get("payment_wechat_mchid");
        if (!mchId.equals(value.getString("mchid"))) throw new IllegalArgumentException("微信支付商户不匹配");
    }

    private String request(String body) {
        try {
            String timestamp = String.valueOf(Instant.now().getEpochSecond());
            String nonce = UUID.randomUUID().toString().replace("-", "");
            String message = "POST\n" + NATIVE_PATH + "\n" + timestamp + "\n" + nonce + "\n" + body + "\n";
            String signature = PaymentCrypto.signRsa(message, required("payment_wechat_private_key"));
            String authorization = "WECHATPAY2-SHA256-RSA2048 mchid=\"" + required("payment_wechat_mchid") + "\",nonce_str=\"" + nonce + "\",timestamp=\"" + timestamp + "\",serial_no=\"" + required("payment_wechat_serial_no") + "\",signature=\"" + signature + "\"";
            HttpURLConnection connection = (HttpURLConnection) new URL(config.get("payment_wechat_gateway", "https://api.mch.weixin.qq.com") + NATIVE_PATH).openConnection();
            connection.setRequestMethod("POST"); connection.setConnectTimeout(10000); connection.setReadTimeout(20000); connection.setDoOutput(true);
            connection.setRequestProperty("Authorization", authorization); connection.setRequestProperty("Accept", "application/json"); connection.setRequestProperty("Content-Type", "application/json");
            try (OutputStream output = connection.getOutputStream()) { output.write(body.getBytes(StandardCharsets.UTF_8)); }
            InputStream input = connection.getResponseCode() >= 400 ? connection.getErrorStream() : connection.getInputStream();
            String response = new String(input.readAllBytes(), StandardCharsets.UTF_8);
            if (connection.getResponseCode() >= 400) throw new IllegalArgumentException("微信支付创建订单失败: " + response);
            return response;
        } catch (IllegalArgumentException error) { throw error; }
        catch (Exception error) { throw new IllegalArgumentException("微信支付请求失败", error); }
    }

    private void verifyHeader(PaymentCallback callback) {
        String timestamp = callback.header("Wechatpay-Timestamp"); String nonce = callback.header("Wechatpay-Nonce"); String signature = callback.header("Wechatpay-Signature");
        if (timestamp == null || nonce == null || signature == null) throw new IllegalArgumentException("缺少微信支付回调签名");
        try {
            long seconds = Long.parseLong(timestamp);
            if (Math.abs(Instant.now().getEpochSecond() - seconds) > 300) throw new IllegalArgumentException("微信支付回调已过期");
            String content = timestamp + "\n" + nonce + "\n" + callback.getRawBody() + "\n";
            java.security.Signature verifier = java.security.Signature.getInstance("SHA256withRSA");
            verifier.initVerify(platformPublicKey(required("payment_wechat_platform_certificate")));
            verifier.update(content.getBytes(StandardCharsets.UTF_8));
            if (!verifier.verify(Base64.getDecoder().decode(signature))) throw new IllegalArgumentException("微信支付回调签名校验失败");
        } catch (IllegalArgumentException error) { throw error; }
        catch (Exception error) { throw new IllegalArgumentException("微信支付回调签名校验失败", error); }
    }

    private JSONObject decrypted(PaymentCallback callback) {
        try {
            JSONObject resource = JSON.parseObject(callback.getRawBody()).getJSONObject("resource");
            if (resource == null) throw new IllegalArgumentException("微信支付回调缺少 resource");
            Cipher cipher = Cipher.getInstance("AES/GCM/NoPadding");
            cipher.init(Cipher.DECRYPT_MODE, new SecretKeySpec(required("payment_wechat_api_v3_key").getBytes(StandardCharsets.UTF_8), "AES"), new GCMParameterSpec(128, resource.getString("nonce").getBytes(StandardCharsets.UTF_8)));
            cipher.updateAAD(resource.getString("associated_data").getBytes(StandardCharsets.UTF_8));
            return JSON.parseObject(new String(cipher.doFinal(Base64.getDecoder().decode(resource.getString("ciphertext"))), StandardCharsets.UTF_8));
        } catch (IllegalArgumentException error) { throw error; }
        catch (Exception error) { throw new IllegalArgumentException("微信支付回调解密失败", error); }
    }

    private java.security.PublicKey platformPublicKey(String value) throws Exception {
        try { return PaymentCrypto.certificatePublicKey(value); }
        catch (Exception ignored) { return PaymentCrypto.publicKey(value); }
    }

    private String required(String name) { String value = config.get(name); if (value.isBlank()) throw new IllegalArgumentException("请先配置 " + name); return value; }
}
