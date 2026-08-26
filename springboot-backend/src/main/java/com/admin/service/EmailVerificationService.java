package com.admin.service;

import org.springframework.beans.factory.annotation.Value;
import org.springframework.beans.factory.ObjectProvider;
import com.admin.entity.ViteConfig;
import com.baomidou.mybatisplus.core.conditions.query.QueryWrapper;
import org.springframework.mail.javamail.JavaMailSenderImpl;
import org.springframework.mail.javamail.MimeMessageHelper;
import org.springframework.stereotype.Service;
import org.springframework.jdbc.core.JdbcTemplate;
import org.springframework.transaction.annotation.Transactional;
import org.springframework.data.redis.core.StringRedisTemplate;
import org.springframework.data.redis.core.script.DefaultRedisScript;

import java.security.SecureRandom;
import java.security.MessageDigest;
import java.util.Base64;
import java.time.Instant;
import java.util.Map;
import java.util.concurrent.ConcurrentHashMap;

/** 邮箱验证码服务。验证码只保存 BCrypt 摘要，并且只能成功消费一次。 */
@Service
public class EmailVerificationService {
    private static final DefaultRedisScript<Long> CONSUME_SCRIPT = new DefaultRedisScript<>(
            "local value = redis.call('GET', KEYS[1]) " +
            "if not value then return 0 end " +
            "local sep = string.find(value, '|') " +
            "local expected = value " +
            "local attempts = 0 " +
            "if sep then expected = string.sub(value, 1, sep - 1); attempts = tonumber(string.sub(value, sep + 1)) or 0 end " +
            "if ARGV[1] == expected then redis.call('DEL', KEYS[1]); return 1 end " +
            "attempts = attempts + 1 " +
            "if attempts >= 5 then redis.call('DEL', KEYS[1]) else local ttl = redis.call('PTTL', KEYS[1]); redis.call('SET', KEYS[1], expected .. '|' .. attempts, 'PX', ttl) end " +
            "return -1", Long.class);
    private final ViteConfigService configs;
    private final JdbcTemplate jdbc;
    private final org.springframework.security.crypto.bcrypt.BCryptPasswordEncoder encoder =
            new org.springframework.security.crypto.bcrypt.BCryptPasswordEncoder();
    private final Map<String, RateWindow> emailSendWindows = new ConcurrentHashMap<>();
    private final Map<String, RateWindow> ipSendWindows = new ConcurrentHashMap<>();
    private final SecureRandom random = new SecureRandom();
    private final long expireSeconds;
    private final long cooldownSeconds;
    private final StringRedisTemplate redis;
    private final String resetUrlBase;

    public EmailVerificationService(ViteConfigService configs,
                                    JdbcTemplate jdbc,
                                    ObjectProvider<StringRedisTemplate> redisProvider,
                                    @Value("${tms.auth.verification-expire-seconds:600}") long expireSeconds,
                                    @Value("${tms.auth.verification-cooldown-seconds:60}") long cooldownSeconds,
                                    @Value("${tms.auth.reset-url-base:http://localhost:6366}") String resetUrlBase) {
        this.configs = configs;
        this.jdbc = jdbc;
        this.redis = redisProvider.getIfAvailable();
        this.expireSeconds = Math.max(60, expireSeconds);
        this.cooldownSeconds = Math.max(10, cooldownSeconds);
        this.resetUrlBase = resetUrlBase == null || resetUrlBase.isBlank() ? "http://localhost:6366" : resetUrlBase.replaceAll("/$", "");
    }

    public void send(String email, String clientIp) {
        send(email, clientIp, Purpose.REGISTER);
    }

    public void send(String email, String clientIp, Purpose purpose) {
        String normalized = normalize(email);
        checkSendRate(normalized, clientIp, purpose);
        sendCode(normalized, purpose);
    }

    /** Applies the rate limit even when the address is not registered (anti-abuse and anti-enumeration). */
    public void checkSendRate(String email, String clientIp, Purpose purpose) {
        String normalized = normalize(email);
        String purposeKey = purpose.name().toLowerCase(java.util.Locale.ROOT);
        enforceSendRate(emailSendWindows, purposeKey + ":" + normalized, 5, "该邮箱发送次数过多，请一小时后再试");
        enforceSendRate(ipSendWindows, purposeKey + ":" + (clientIp == null ? "unknown" : clientIp), 20, "该 IP 发送次数过多，请一小时后再试");
    }

    public void sendAfterRateCheck(String email, Purpose purpose) {
        String normalized = normalize(email);
        if (purpose == Purpose.PASSWORD_RESET) sendResetToken(normalized);
        else sendCode(normalized, purpose);
    }

    private void sendCode(String normalized, Purpose purpose) {
        long cooldown = secondsConfig("email_code_cooldown_seconds", cooldownSeconds, 10, 3600);
        long expiry = secondsConfig("email_code_expire_seconds", expireSeconds, 60, 86400);
        Entry old = loadPending(normalized, purpose);
        long now = Instant.now().getEpochSecond();
        if (old != null && old.sentAt + cooldown > now) {
            throw new IllegalArgumentException("验证码发送过于频繁，请稍后再试");
        }
        String code = String.format("%06d", random.nextInt(1_000_000));
        JavaMailSenderImpl mailSender = createSender();
        String subject = configOr("email_register_subject", "TMS 注册验证码");
        String content = configOr("email_register_template", "你的 TMS 注册验证码是 {{code}}，有效期 {{expires_minutes}} 分钟。请勿将验证码泄露给他人。");
        sendMessage(mailSender, normalized, render(subject, code, expiry, ""), render(content, code, expiry, ""));
        // Persist only the BCrypt digest. The plaintext code is never written to the database or logs.
        jdbc.update("DELETE FROM verification_code WHERE email = ? AND purpose = ?", normalized, purpose.name());
        jdbc.update("INSERT INTO verification_code (email, purpose, code_hash, expires_at, sent_at, attempts, consumed_at) VALUES (?, ?, ?, ?, ?, 0, NULL)",
                normalized, purpose.name(), encoder.encode(code), now + expiry, now);
        redisPut(redisKey(purpose, normalized), sha256(code), expiry);
    }

    /** sub2api-compatible password reset: the email contains a one-time URL token. */
    private void sendResetToken(String normalized) {
        long cooldown = secondsConfig("email_code_cooldown_seconds", cooldownSeconds, 10, 3600);
        long expiry = secondsConfig("email_code_expire_seconds", expireSeconds, 60, 86400);
        Entry old = loadPending(normalized, Purpose.PASSWORD_RESET);
        long now = Instant.now().getEpochSecond();
        if (old != null && old.sentAt + cooldown > now) throw new IllegalArgumentException("验证码发送过于频繁，请稍后再试");
        byte[] bytes = new byte[32];
        random.nextBytes(bytes);
        String token = Base64.getUrlEncoder().withoutPadding().encodeToString(bytes);
        String link = resetUrlBase + "/reset-password?email=" + java.net.URLEncoder.encode(normalized, java.nio.charset.StandardCharsets.UTF_8) + "&token=" + java.net.URLEncoder.encode(token, java.nio.charset.StandardCharsets.UTF_8);
        JavaMailSenderImpl mailSender = createSender();
        String subject = configOr("email_reset_subject", "TMS 密码重置");
        String content = configOr("email_reset_template", "请点击以下链接重置密码（有效期 {{expires_minutes}} 分钟，使用一次后失效）：\\n{{reset_url}}\\n\\n如果不是你本人操作，请忽略此邮件。");
        sendMessage(mailSender, normalized, render(subject, "", expiry, link), render(content, "", expiry, link));
        jdbc.update("DELETE FROM verification_code WHERE email = ? AND purpose = ?", normalized, Purpose.PASSWORD_RESET.name());
        jdbc.update("INSERT INTO verification_code (email, purpose, code_hash, expires_at, sent_at, attempts, consumed_at) VALUES (?, ?, ?, ?, ?, 0, NULL)",
                normalized, Purpose.PASSWORD_RESET.name(), encoder.encode(token), now + expiry, now);
        redisPut(redisKey(Purpose.PASSWORD_RESET, normalized), sha256(token), expiry);
    }

    public void sendTest(String email) {
        if (email == null || email.isBlank()) throw new IllegalArgumentException("测试邮箱不能为空");
        JavaMailSenderImpl mailSender = createSender();
        sendMessage(mailSender, email.trim(), "TMS SMTP 测试", "TMS SMTP 配置测试成功。");
    }

    public boolean consume(String email, String code) {
        return consume(email, code, Purpose.REGISTER);
    }

    @Transactional
    public boolean consume(String email, String code, Purpose purpose) {
        if (code == null || code.trim().isEmpty()) return false;
        String normalized = normalize(email);
        Integer redisResult = consumeRedis(redisKey(purpose, normalized), sha256(code.trim()));
        if (redisResult != null) {
            if (redisResult == 1) markConsumed(normalized, purpose);
            return redisResult == 1;
        }
        Entry entry = loadPending(normalized, purpose);
        long now = Instant.now().getEpochSecond();
        if (entry == null) return false;
        if (entry.expiresAt < now) {
            jdbc.update("DELETE FROM verification_code WHERE id = ?", entry.id);
            return false;
        }
        if (!encoder.matches(code.trim(), entry.hash)) {
            jdbc.update("UPDATE verification_code SET attempts = attempts + 1 WHERE id = ? AND consumed_at IS NULL AND attempts < 5", entry.id);
            jdbc.update("DELETE FROM verification_code WHERE id = ? AND attempts >= 5", entry.id);
            return false;
        }
        return jdbc.update("UPDATE verification_code SET consumed_at = ? WHERE id = ? AND consumed_at IS NULL", Instant.now().getEpochSecond(), entry.id) == 1;
    }

    private void markConsumed(String email, Purpose purpose) {
        jdbc.update("UPDATE verification_code SET consumed_at = ? WHERE email = ? AND purpose = ? AND consumed_at IS NULL", Instant.now().getEpochSecond(), email, purpose.name());
    }

    private String redisKey(Purpose purpose, String email) { return "tms:auth:verify:" + purpose.name() + ":" + email; }

    private void redisPut(String key, String value, long ttlSeconds) {
        if (redis == null) return;
        try { redis.opsForValue().set(key, value, java.time.Duration.ofSeconds(Math.max(1, ttlSeconds))); } catch (Exception ignored) { /* DB fallback remains available. */ }
    }

    private String redisGet(String key) {
        if (redis == null) return null;
        try { return redis.opsForValue().get(key); } catch (Exception ignored) { return null; }
    }

    /** Returns null only when Redis is unavailable or the key does not exist. */
    private Integer consumeRedis(String key, String digest) {
        if (redis == null) return null;
        try {
            String existing = redis.opsForValue().get(key);
            if (existing == null) return null;
            Long result = redis.execute(CONSUME_SCRIPT, java.util.Collections.singletonList(key), digest);
            return result == null ? 0 : result.intValue();
        } catch (Exception ignored) { return null; }
    }

    private long redisTtl(String key) {
        if (redis == null) return 1;
        try { Long ttl = redis.getExpire(key, java.util.concurrent.TimeUnit.SECONDS); return ttl == null || ttl < 1 ? 1 : ttl; } catch (Exception ignored) { return 1; }
    }

    private void redisDelete(String key) {
        if (redis == null) return;
        try { redis.delete(key); } catch (Exception ignored) { }
    }

    private String sha256(String value) {
        try {
            byte[] digest = MessageDigest.getInstance("SHA-256").digest(value.getBytes(java.nio.charset.StandardCharsets.UTF_8));
            return java.util.HexFormat.of().formatHex(digest);
        } catch (Exception e) { throw new IllegalStateException("无法生成验证码摘要", e); }
    }

    private Entry loadPending(String email, Purpose purpose) {
        return jdbc.query("SELECT id, code_hash, expires_at, sent_at, attempts FROM verification_code WHERE email = ? AND purpose = ? AND consumed_at IS NULL ORDER BY id DESC",
                rs -> rs.next() ? new Entry(rs.getLong("id"), rs.getString("code_hash"), rs.getLong("expires_at"), rs.getLong("sent_at"), rs.getInt("attempts")) : null,
                email, purpose.name());
    }

    private JavaMailSenderImpl createSender() {
        String host = config("smtp_host");
        if (host.isBlank()) throw new IllegalArgumentException("SMTP 尚未配置");
        JavaMailSenderImpl sender = new JavaMailSenderImpl();
        sender.setHost(host);
        try { sender.setPort(Integer.parseInt(configOr("smtp_port", "587"))); } catch (NumberFormatException ignored) { sender.setPort(587); }
        String username = config("smtp_username");
        String password = config("smtp_password");
        if (!username.isBlank()) sender.setUsername(username);
        if (!password.isBlank()) sender.setPassword(password);
        java.util.Properties props = sender.getJavaMailProperties();
        props.put("mail.smtp.auth", "true");
        props.put("mail.smtp.starttls.enable", configOr("smtp_starttls", "true"));
        props.put("mail.smtp.ssl.enable", configOr("smtp_ssl", "false"));
        return sender;
    }

    private void sendMessage(JavaMailSenderImpl sender, String recipient, String subject, String content) {
        try {
            MimeMessageHelper message = new MimeMessageHelper(sender.createMimeMessage(), false, "UTF-8");
            message.setTo(recipient); message.setSubject(subject); message.setText(content, false);
            String from = config("smtp_from");
            String fromName = config("smtp_from_name");
            if (!from.isBlank()) {
                if (fromName.isBlank()) message.setFrom(from);
                else message.setFrom(from, fromName);
            }
            sender.send(message.getMimeMessage());
        } catch (Exception error) {
            throw new IllegalArgumentException("邮件发送失败，请检查 SMTP 配置", error);
        }
    }

    private String config(String name) { return configOr(name, ""); }
    private String render(String template, String code, long expiry, String resetUrl) {
        String appName = configOr("app_name", "TMS");
        return template.replace("{{code}}", code == null ? "" : code)
                .replace("{{expires_minutes}}", String.valueOf(Math.max(1, expiry / 60)))
                .replace("{{reset_url}}", resetUrl == null ? "" : resetUrl)
                .replace("{{app_name}}", appName);
    }
    private String configOr(String name, String fallback) {
        ViteConfig value = configs.getOne(new QueryWrapper<ViteConfig>().eq("name", name));
        return value == null || value.getValue() == null ? fallback : value.getValue().trim();
    }
    private long secondsConfig(String name, long fallback, long min, long max) {
        try { return Math.max(min, Math.min(max, Long.parseLong(configOr(name, String.valueOf(fallback))))); }
        catch (NumberFormatException ignored) { return fallback; }
    }

    public static String normalize(String email) {
        return email == null ? "" : email.trim().toLowerCase(java.util.Locale.ROOT);
    }

    private void enforceSendRate(Map<String, RateWindow> windows, String key, int max, String message) {
        long now = Instant.now().getEpochSecond();
        while (true) {
            RateWindow current = windows.get(key);
            RateWindow next;
            if (current == null || current.startedAt + 3600 <= now) {
                next = new RateWindow(now, 1);
                if (current == null ? windows.putIfAbsent(key, next) == null : windows.replace(key, current, next)) return;
            } else {
                if (current.count >= max) throw new IllegalArgumentException(message);
                next = new RateWindow(current.startedAt, current.count + 1);
                if (windows.replace(key, current, next)) return;
            }
        }
    }

    private record Entry(long id, String hash, long expiresAt, long sentAt, int attempts) {}
    private record RateWindow(long startedAt, int count) {}

    public enum Purpose { REGISTER, PASSWORD_RESET }
}
