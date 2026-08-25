package com.admin.service.payment;

import java.net.URLEncoder;
import java.nio.charset.StandardCharsets;
import java.security.KeyFactory;
import java.security.MessageDigest;
import java.security.PrivateKey;
import java.security.PublicKey;
import java.security.Signature;
import java.security.spec.PKCS8EncodedKeySpec;
import java.security.spec.X509EncodedKeySpec;
import java.security.cert.CertificateFactory;
import java.util.ArrayList;
import java.util.Base64;
import java.util.Comparator;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import javax.crypto.Mac;
import javax.crypto.spec.SecretKeySpec;

final class PaymentCrypto {
    private PaymentCrypto() { }

    static String orderedQuery(Map<String, String> values, boolean encoded) {
        List<Map.Entry<String, String>> entries = new ArrayList<>(values.entrySet());
        entries.removeIf(entry -> entry.getValue() == null || entry.getValue().isEmpty() || "sign".equalsIgnoreCase(entry.getKey()) || "sign_type".equalsIgnoreCase(entry.getKey()));
        entries.sort(Comparator.comparing(Map.Entry::getKey));
        StringBuilder result = new StringBuilder();
        for (Map.Entry<String, String> entry : entries) {
            if (result.length() > 0) result.append('&');
            result.append(encoded ? encode(entry.getKey()) : entry.getKey()).append('=')
                    .append(encoded ? encode(entry.getValue()) : entry.getValue());
        }
        return result.toString();
    }

    static String md5(String value) {
        try {
            byte[] bytes = MessageDigest.getInstance("MD5").digest(value.getBytes(StandardCharsets.UTF_8));
            StringBuilder output = new StringBuilder();
            for (byte item : bytes) output.append(String.format("%02x", item));
            return output.toString();
        } catch (Exception error) { throw new IllegalStateException("无法计算 MD5", error); }
    }

    static String hmacSha256(String key, String value) {
        try {
            Mac mac = Mac.getInstance("HmacSHA256");
            mac.init(new SecretKeySpec(key.getBytes(StandardCharsets.UTF_8), "HmacSHA256"));
            return Base64.getEncoder().encodeToString(mac.doFinal(value.getBytes(StandardCharsets.UTF_8)));
        } catch (Exception error) { throw new IllegalStateException("无法计算 HMAC", error); }
    }

    static String hmacSha256Hex(String key, String value) {
        try {
            Mac mac = Mac.getInstance("HmacSHA256");
            mac.init(new SecretKeySpec(key.getBytes(StandardCharsets.UTF_8), "HmacSHA256"));
            byte[] bytes = mac.doFinal(value.getBytes(StandardCharsets.UTF_8));
            StringBuilder output = new StringBuilder();
            for (byte item : bytes) output.append(String.format("%02x", item));
            return output.toString();
        } catch (Exception error) { throw new IllegalStateException("无法计算 HMAC", error); }
    }

    static String signRsa(String content, String pem) { return sign(content, pem, "SHA256withRSA"); }
    static boolean verifyRsa(String content, String signature, String pem) { return verify(content, signature, pem, "SHA256withRSA"); }

    static String sign(String content, String pem, String algorithm) {
        try {
            Signature signer = Signature.getInstance(algorithm);
            signer.initSign(privateKey(pem));
            signer.update(content.getBytes(StandardCharsets.UTF_8));
            return Base64.getEncoder().encodeToString(signer.sign());
        } catch (Exception error) { throw new IllegalArgumentException("支付私钥配置无效", error); }
    }

    static boolean verify(String content, String signature, String pem, String algorithm) {
        try {
            Signature verifier = Signature.getInstance(algorithm);
            verifier.initVerify(publicKey(pem));
            verifier.update(content.getBytes(StandardCharsets.UTF_8));
            return verifier.verify(Base64.getDecoder().decode(signature));
        } catch (Exception error) { throw new IllegalArgumentException("支付回调签名无效", error); }
    }

    static boolean equalsConstantTime(String left, String right) {
        if (left == null || right == null) return false;
        return MessageDigest.isEqual(left.getBytes(StandardCharsets.UTF_8), right.getBytes(StandardCharsets.UTF_8));
    }

    static PrivateKey privateKey(String pem) throws Exception {
        String normalized = pem.replaceAll("-----BEGIN (?:RSA )?PRIVATE KEY-----", "")
                .replaceAll("-----END (?:RSA )?PRIVATE KEY-----", "").replaceAll("\\s", "");
        return KeyFactory.getInstance("RSA").generatePrivate(new PKCS8EncodedKeySpec(Base64.getDecoder().decode(normalized)));
    }

    static PublicKey publicKey(String pem) throws Exception {
        String normalized = pem.replaceAll("-----BEGIN (?:RSA )?PUBLIC KEY-----", "")
                .replaceAll("-----END (?:RSA )?PUBLIC KEY-----", "").replaceAll("\\s", "");
        return KeyFactory.getInstance("RSA").generatePublic(new X509EncodedKeySpec(Base64.getDecoder().decode(normalized)));
    }

    static PublicKey certificatePublicKey(String pem) throws Exception {
        return CertificateFactory.getInstance("X.509").generateCertificate(
                new java.io.ByteArrayInputStream(pem.getBytes(StandardCharsets.UTF_8))).getPublicKey();
    }

    static Map<String, String> copy(Map<String, String> source) { return new LinkedHashMap<>(source); }
    private static String encode(String value) { return URLEncoder.encode(value, StandardCharsets.UTF_8).replace("+", "%20"); }
}
