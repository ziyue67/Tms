package com.admin.service.payment;

import com.admin.entity.PaymentOrder;
import com.admin.entity.SubscriptionPlan;
import org.junit.jupiter.api.Test;

import java.math.BigDecimal;
import java.security.KeyPair;
import java.security.KeyPairGenerator;
import java.time.Instant;
import java.util.Base64;
import java.util.LinkedHashMap;
import java.util.Map;
import javax.crypto.Cipher;
import javax.crypto.spec.GCMParameterSpec;
import javax.crypto.spec.SecretKeySpec;

import static org.junit.jupiter.api.Assertions.assertDoesNotThrow;
import static org.junit.jupiter.api.Assertions.assertEquals;

class PaymentProviderAdapterTest {
    @Test
    void alipayCreatesAndVerifiesRsa2Payment() throws Exception {
        KeyPair pair = KeyPairGenerator.getInstance("RSA").generateKeyPair();
        Map<String, String> values = new LinkedHashMap<>();
        values.put("payment_alipay_app_id", "app-1"); values.put("payment_alipay_private_key", privatePem(pair));
        values.put("payment_alipay_public_key", publicPem(pair)); values.put("payment_alipay_notify_url", "https://panel.example/api/v1/payment/alipay/notify");
        AlipayPaymentProvider provider = new AlipayPaymentProvider(config(values));
        PaymentOrder order = order("TMS-A", "CNY", "12.34");
        PaymentCheckout checkout = provider.createCheckout(order, plan("专业套餐"));
        assertEquals("form", checkout.getType()); assertEquals("app-1", checkout.getFields().get("app_id"));
        Map<String, String> callback = new LinkedHashMap<>();
        callback.put("out_trade_no", "TMS-A"); callback.put("trade_no", "20260001"); callback.put("trade_status", "TRADE_SUCCESS");
        callback.put("total_amount", "12.34"); callback.put("app_id", "app-1");
        callback.put("sign", PaymentCrypto.signRsa(PaymentCrypto.orderedQuery(callback, false), privatePem(pair)));
        assertDoesNotThrow(() -> provider.verify(order, new PaymentCallback("", callback, Map.of())));
    }

    @Test
    void easyPayUsesSignedAmountAndOrder() {
        Map<String, String> values = new LinkedHashMap<>();
        values.put("payment_easypay_pid", "1000"); values.put("payment_easypay_key", "merchant-key");
        values.put("payment_easypay_notify_url", "https://panel.example/api/v1/payment/easypay/notify");
        EasyPayPaymentProvider provider = new EasyPayPaymentProvider(config(values));
        PaymentOrder order = order("TMS-B", "CNY", "20.00");
        PaymentCheckout checkout = provider.createCheckout(order, plan("标准套餐"));
        assertEquals("form", checkout.getType());
        Map<String, String> callback = new LinkedHashMap<>();
        callback.put("out_trade_no", "TMS-B"); callback.put("trade_no", "easy-1"); callback.put("trade_status", "TRADE_SUCCESS"); callback.put("money", "20.00");
        callback.put("sign", PaymentCrypto.md5(PaymentCrypto.orderedQuery(callback, false) + "merchant-key"));
        assertDoesNotThrow(() -> provider.verify(order, new PaymentCallback("", callback, Map.of())));
    }

    @Test
    void stripeWebhookRequiresOfficialSignedRawPayload() {
        Map<String, String> values = Map.of("payment_stripe_webhook_secret", "whsec_test");
        StripePaymentProvider provider = new StripePaymentProvider(config(values));
        PaymentOrder order = order("TMS-C", "USD", "10.00");
        String raw = "{\"type\":\"checkout.session.completed\",\"data\":{\"object\":{\"client_reference_id\":\"TMS-C\",\"payment_intent\":\"pi_1\",\"payment_status\":\"paid\",\"amount_total\":1000,\"currency\":\"usd\"}}}";
        String timestamp = String.valueOf(Instant.now().getEpochSecond());
        String signature = PaymentCrypto.hmacSha256Hex("whsec_test", timestamp + "." + raw);
        PaymentCallback callback = new PaymentCallback(raw, Map.of(), Map.of("Stripe-Signature", "t=" + timestamp + ",v1=" + signature));
        assertEquals("TMS-C", provider.orderNo(callback)); assertEquals("pi_1", provider.tradeNo(callback));
        assertDoesNotThrow(() -> provider.verify(order, callback));
    }

    @Test
    void wechatPayV3VerifiesSignatureAndDecryptsNotification() throws Exception {
        KeyPair pair = KeyPairGenerator.getInstance("RSA").generateKeyPair();
        String key = "0123456789abcdef0123456789abcdef";
        Map<String, String> values = new LinkedHashMap<>();
        values.put("payment_wechat_app_id", "wx-test"); values.put("payment_wechat_mchid", "1900000001");
        values.put("payment_wechat_api_v3_key", key); values.put("payment_wechat_platform_certificate", publicPem(pair));
        WechatPaymentProvider provider = new WechatPaymentProvider(config(values));
        String nonce = "123456789012"; String aad = "transaction";
        String plain = "{\"out_trade_no\":\"TMS-D\",\"transaction_id\":\"wx-1\",\"trade_state\":\"SUCCESS\",\"appid\":\"wx-test\",\"mchid\":\"1900000001\",\"amount\":{\"total\":500,\"currency\":\"CNY\"}}";
        Cipher cipher = Cipher.getInstance("AES/GCM/NoPadding");
        cipher.init(Cipher.ENCRYPT_MODE, new SecretKeySpec(key.getBytes(), "AES"), new GCMParameterSpec(128, nonce.getBytes())); cipher.updateAAD(aad.getBytes());
        String encrypted = Base64.getEncoder().encodeToString(cipher.doFinal(plain.getBytes()));
        String raw = "{\"resource\":{\"nonce\":\"" + nonce + "\",\"associated_data\":\"" + aad + "\",\"ciphertext\":\"" + encrypted + "\"}}";
        String timestamp = String.valueOf(Instant.now().getEpochSecond()); String headerNonce = "header-nonce";
        String signature = PaymentCrypto.signRsa(timestamp + "\n" + headerNonce + "\n" + raw + "\n", privatePem(pair));
        PaymentCallback callback = new PaymentCallback(raw, Map.of(), Map.of("Wechatpay-Timestamp", timestamp, "Wechatpay-Nonce", headerNonce, "Wechatpay-Signature", signature));
        PaymentOrder order = order("TMS-D", "CNY", "5.00");
        assertEquals("TMS-D", provider.orderNo(callback)); assertEquals("wx-1", provider.tradeNo(callback));
        assertDoesNotThrow(() -> provider.verify(order, callback));
    }

    private static PaymentConfig config(Map<String, String> values) {
        return new PaymentConfig(null) {
            @Override public String get(String name) { return values.getOrDefault(name, ""); }
            @Override public String get(String name, String fallback) { return values.getOrDefault(name, fallback); }
        };
    }
    private static PaymentOrder order(String no, String currency, String amount) { PaymentOrder order = new PaymentOrder(); order.setOrderNo(no); order.setAmount(new BigDecimal(amount)); order.setCurrency(currency); return order; }
    private static SubscriptionPlan plan(String name) { SubscriptionPlan plan = new SubscriptionPlan(); plan.setName(name); return plan; }
    private static String privatePem(KeyPair pair) { return "-----BEGIN PRIVATE KEY-----\n" + Base64.getMimeEncoder(64, "\n".getBytes()).encodeToString(pair.getPrivate().getEncoded()) + "\n-----END PRIVATE KEY-----"; }
    private static String publicPem(KeyPair pair) { return "-----BEGIN PUBLIC KEY-----\n" + Base64.getMimeEncoder(64, "\n".getBytes()).encodeToString(pair.getPublic().getEncoded()) + "\n-----END PUBLIC KEY-----"; }
}
