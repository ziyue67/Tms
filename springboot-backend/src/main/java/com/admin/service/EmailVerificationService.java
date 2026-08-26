package com.admin.service;

import org.springframework.beans.factory.annotation.Value;
import org.springframework.beans.factory.ObjectProvider;
import com.admin.entity.ViteConfig;
import com.baomidou.mybatisplus.core.conditions.query.QueryWrapper;
import org.springframework.mail.javamail.JavaMailSenderImpl;
import org.springframework.mail.javamail.MimeMessageHelper;
import org.springframework.stereotype.Service;
import org.springframework.data.redis.core.StringRedisTemplate;
import org.springframework.data.redis.core.script.DefaultRedisScript;

import java.security.SecureRandom;
import java.security.MessageDigest;
import java.util.Base64;
import java.util.Collections;
import java.util.List;
import java.time.Instant;
import java.util.Map;
import java.util.concurrent.ConcurrentHashMap;

/** 邮箱验证码服务。临时凭据只保存 Redis 摘要，并且只能成功消费一次。 */
@Service
public class EmailVerificationService {
    private static final String REGISTER_TEMPLATE = """
            <!DOCTYPE html><html><head><meta charset="UTF-8"></head><body style="margin:0;padding:20px;background:#f5f5f5;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Arial,sans-serif"><div style="max-width:600px;margin:auto;background:#fff;border-radius:8px;overflow:hidden;box-shadow:0 2px 8px rgba(0,0,0,.1)"><div style="padding:30px;text-align:center;background:#667eea;color:#fff"><h1 style="margin:0;font-size:24px">{{app_name}}</h1></div><div style="padding:40px 30px;text-align:center"><h2>邮箱验证码</h2><p>请使用下面的验证码完成注册：</p><div style="margin:24px 0;padding:16px;background:#f3f4f6;border-radius:6px;font-size:32px;font-weight:700;letter-spacing:8px">{{code}}</div><p>验证码将在 <strong>{{expires_minutes}} 分钟</strong> 后失效。</p><p>如果不是你本人操作，请忽略此邮件。</p></div><div style="padding:20px;text-align:center;background:#f8f9fa;color:#999;font-size:12px">这是系统自动发送的邮件，请勿直接回复。</div></div></body></html>
            """;
    private static final String RESET_TEMPLATE = """
            <!DOCTYPE html><html><head><meta charset="UTF-8"></head><body style="margin:0;padding:20px;background:#f5f5f5;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Arial,sans-serif"><div style="max-width:600px;margin:auto;background:#fff;border-radius:8px;overflow:hidden;box-shadow:0 2px 8px rgba(0,0,0,.1)"><div style="padding:30px;text-align:center;background:#667eea;color:#fff"><h1 style="margin:0;font-size:24px">{{app_name}}</h1></div><div style="padding:40px 30px;text-align:center"><h2>密码重置请求</h2><p>我们收到你的密码重置请求。请点击按钮设置新密码：</p><p style="margin:28px 0"><a href="{{reset_url}}" style="display:inline-block;padding:12px 24px;background:#667eea;color:#fff;text-decoration:none;border-radius:5px">重置密码</a></p><p>该链接将在 <strong>{{expires_minutes}} 分钟</strong> 后失效且只能使用一次。</p><p>如果不是你本人操作，请忽略此邮件。</p></div><div style="padding:20px;text-align:center;background:#f8f9fa;color:#999;font-size:12px">这是系统自动发送的邮件，请勿直接回复。</div></div></body></html>
            """;
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
    private final Map<String, RateWindow> emailSendWindows = new ConcurrentHashMap<>();
    private final Map<String, RateWindow> ipSendWindows = new ConcurrentHashMap<>();
    private final SecureRandom random = new SecureRandom();
    private final long expireSeconds;
    private final long cooldownSeconds;
    private final StringRedisTemplate redis;
    private final String resetUrlBase;

    public EmailVerificationService(ViteConfigService configs,
                                    ObjectProvider<StringRedisTemplate> redisProvider,
                                    @Value("${tms.auth.verification-expire-seconds:600}") long expireSeconds,
                                    @Value("${tms.auth.verification-cooldown-seconds:60}") long cooldownSeconds,
                                    @Value("${tms.auth.reset-url-base:http://localhost:6366}") String resetUrlBase) {
        this.configs = configs;
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
        requireRedis();
        long cooldown = secondsConfig("email_code_cooldown_seconds", cooldownSeconds, 10, 3600);
        long expiry = secondsConfig("email_code_expire_seconds", expireSeconds, 60, 86400);
        String key = redisKey(purpose, normalized);
        if (!reserveCooldown(purpose, normalized, cooldown)) throw new IllegalArgumentException("验证码发送过于频繁，请稍后再试");
        try {
            String code = String.format("%06d", random.nextInt(1_000_000));
            redisPut(key, sha256(code), expiry);
            JavaMailSenderImpl mailSender = createSender();
            String subject = configOr("email_register_subject", "TMS 注册验证码");
            String content = configOr("email_register_template", REGISTER_TEMPLATE);
            sendMessage(mailSender, normalized, render(subject, code, expiry, ""), render(content, code, expiry, ""));
            audit(normalized, purpose, "sent", 0);
        } catch (RuntimeException e) {
            redisDelete(key);
            redisDelete(cooldownKey(purpose, normalized));
            throw e;
        }
    }

    /** sub2api-compatible password reset: the email contains a one-time URL token. */
    private void sendResetToken(String normalized) {
        requireRedis();
        long cooldown = secondsConfig("email_code_cooldown_seconds", cooldownSeconds, 10, 3600);
        long expiry = secondsConfig("email_code_expire_seconds", expireSeconds, 60, 86400);
        if (!reserveCooldown(Purpose.PASSWORD_RESET, normalized, cooldown)) throw new IllegalArgumentException("验证码发送过于频繁，请稍后再试");
        byte[] bytes = new byte[32];
        random.nextBytes(bytes);
        String token = Base64.getUrlEncoder().withoutPadding().encodeToString(bytes);
        String link = resetUrlBase + "/reset-password?email=" + java.net.URLEncoder.encode(normalized, java.nio.charset.StandardCharsets.UTF_8) + "&token=" + java.net.URLEncoder.encode(token, java.nio.charset.StandardCharsets.UTF_8);
        try {
            redisPut(redisKey(Purpose.PASSWORD_RESET, normalized), sha256(token), expiry);
            JavaMailSenderImpl mailSender = createSender();
            String subject = configOr("email_reset_subject", "TMS 密码重置");
            String content = configOr("email_reset_template", RESET_TEMPLATE);
            sendMessage(mailSender, normalized, render(subject, "", expiry, link), render(content, "", expiry, link));
            audit(normalized, Purpose.PASSWORD_RESET, "sent", 0);
        } catch (RuntimeException e) {
            redisDelete(redisKey(Purpose.PASSWORD_RESET, normalized));
            redisDelete(cooldownKey(Purpose.PASSWORD_RESET, normalized));
            throw e;
        }
    }

    public void sendTest(String email) {
        if (email == null || email.isBlank()) throw new IllegalArgumentException("测试邮箱不能为空");
        JavaMailSenderImpl mailSender = createSender();
        sendMessage(mailSender, email.trim(), "TMS SMTP 测试", "TMS SMTP 配置测试成功。");
    }

    /** Returns redacted Redis-only authentication audit entries for administrators. */
    public List<String> audit(int limit) {
        if (redis == null) return Collections.emptyList();
        try {
            int safeLimit = Math.max(1, Math.min(200, limit));
            List<String> entries = redis.opsForList().range("tms:auth:audit", -safeLimit, -1);
            return entries == null ? Collections.emptyList() : entries;
        } catch (Exception ignored) {
            return Collections.emptyList();
        }
    }

    /** Operational check for administrators. It never exposes the Redis password. */
    public Map<String, Object> health() {
        Map<String, Object> result = new java.util.LinkedHashMap<>();
        result.put("redisConfigured", redis != null);
        if (redis == null) { result.put("redisAvailable", false); return result; }
        try (org.springframework.data.redis.connection.RedisConnection connection = redis.getConnectionFactory().getConnection()) {
            String pong = connection.ping();
            result.put("redisAvailable", "PONG".equalsIgnoreCase(pong));
        } catch (Exception e) {
            result.put("redisAvailable", false);
            result.put("message", "Redis 无法连接，请检查 REDIS_HOST、REDIS_PORT 和 REDIS_PASSWORD 是否与 Redis 服务一致");
        }
        return result;
    }

    public boolean consume(String email, String code) {
        return consume(email, code, Purpose.REGISTER);
    }

    public boolean consume(String email, String code, Purpose purpose) {
        if (code == null || code.trim().isEmpty()) return false;
        String normalized = normalize(email);
        if (redis == null) {
            return false;
        }
        Integer redisResult = consumeRedis(redisKey(purpose, normalized), sha256(code.trim()));
        if (redisResult == null) {
            audit(normalized, purpose, "redis_unavailable", 0);
            return false;
        }
        audit(normalized, purpose, redisResult == 1 ? "consumed" : "rejected", redisResult == -1 ? 1 : 0);
        return redisResult == 1;
    }

    private String redisKey(Purpose purpose, String email) { return "tms:auth:verify:" + purpose.name() + ":" + email; }
    private String cooldownKey(Purpose purpose, String email) { return "tms:auth:cooldown:" + purpose.name() + ":" + email; }

    private boolean reserveCooldown(Purpose purpose, String email, long ttlSeconds) {
        requireRedis();
        try {
            Boolean reserved = redis.opsForValue().setIfAbsent(cooldownKey(purpose, email), "1", java.time.Duration.ofSeconds(Math.max(1, ttlSeconds)));
            return Boolean.TRUE.equals(reserved);
        } catch (Exception e) {
            throw new IllegalArgumentException("认证服务不可用：Redis 无法连接，请检查 REDIS_HOST、REDIS_PORT 和 REDIS_PASSWORD", e);
        }
    }

    private void redisPut(String key, String value, long ttlSeconds) {
        if (redis == null) throw new IllegalArgumentException("认证服务暂不可用，请稍后重试");
        try { redis.opsForValue().set(key, value, java.time.Duration.ofSeconds(Math.max(1, ttlSeconds))); } catch (Exception e) { throw new IllegalArgumentException("认证服务不可用：Redis 无法连接，请检查 REDIS_HOST、REDIS_PORT 和 REDIS_PASSWORD", e); }
    }

    /** Returns null only when Redis is unavailable or the key does not exist. */
    private Integer consumeRedis(String key, String digest) {
        if (redis == null) return null;
        try {
            String existing = redis.opsForValue().get(key);
            if (existing == null) return 0;
            Long result = redis.execute(CONSUME_SCRIPT, java.util.Collections.singletonList(key), digest);
            return result == null ? 0 : result.intValue();
        } catch (Exception ignored) { return null; }
    }

    private void redisDelete(String key) {
        if (redis == null) return;
        try { redis.delete(key); } catch (Exception ignored) { }
    }

    private void requireRedis() {
        if (redis == null) throw new IllegalArgumentException("认证服务不可用：Redis 未配置");
    }

    private void audit(String email, Purpose purpose, String event, int attempts) {
        if (redis == null) return;
        String record = purpose.name() + "|" + event + "|" + sha256(email) + "|" + Instant.now().getEpochSecond() + "|" + attempts;
        try {
            redis.opsForList().rightPush("tms:auth:audit", record);
            redis.opsForList().trim("tms:auth:audit", -1000, -1);
        } catch (Exception ignored) { }
    }

    private String sha256(String value) {
        try {
            byte[] digest = MessageDigest.getInstance("SHA-256").digest(value.getBytes(java.nio.charset.StandardCharsets.UTF_8));
            return java.util.HexFormat.of().formatHex(digest);
        } catch (Exception e) { throw new IllegalStateException("无法生成验证码摘要", e); }
    }

    private JavaMailSenderImpl createSender() {
        String host = config("smtp_host");
        if (host.isBlank()) throw new IllegalArgumentException("SMTP 尚未配置");
        JavaMailSenderImpl sender = new JavaMailSenderImpl();
        sender.setHost(host);
        int port;
        try { port = Integer.parseInt(configOr("smtp_port", "587")); } catch (NumberFormatException ignored) { port = 587; }
        if (port < 1 || port > 65535) throw new IllegalArgumentException("SMTP 端口无效");
        sender.setPort(port);
        String username = config("smtp_username");
        String password = config("smtp_password");
        if (username.isBlank() || password.isBlank()) throw new IllegalArgumentException("SMTP 用户名和授权码不能为空（QQ 邮箱请填写授权码）");
        if (!username.isBlank()) sender.setUsername(username);
        if (!password.isBlank()) sender.setPassword(password);
        boolean configuredStartTls = Boolean.parseBoolean(configOr("smtp_starttls", port == 587 ? "true" : "false"));
        boolean configuredSsl = Boolean.parseBoolean(configOr("smtp_ssl", port == 465 ? "true" : "false"));
        // QQ and most providers reject the ambiguous combination. Port 465 is implicit TLS;
        // submission ports 587/25 use STARTTLS. Normalize legacy settings accordingly.
        boolean ssl = port == 465 || (configuredSsl && port != 587);
        boolean startTls = !ssl && (configuredStartTls || port == 587);
        if (ssl && startTls) throw new IllegalArgumentException("SMTP SSL 与 STARTTLS 不能同时启用");
        java.util.Properties props = sender.getJavaMailProperties();
        props.put("mail.smtp.auth", "true");
        props.put("mail.smtp.starttls.enable", String.valueOf(startTls));
        props.put("mail.smtp.starttls.required", String.valueOf(startTls));
        props.put("mail.smtp.ssl.enable", String.valueOf(ssl));
        props.put("mail.smtp.ssl.trust", host);
        props.put("mail.smtp.connectiontimeout", "10000");
        props.put("mail.smtp.timeout", "20000");
        props.put("mail.smtp.writetimeout", "20000");
        return sender;
    }

    private void sendMessage(JavaMailSenderImpl sender, String recipient, String subject, String content) {
        try {
            MimeMessageHelper message = new MimeMessageHelper(sender.createMimeMessage(), false, "UTF-8");
            message.setTo(recipient); message.setSubject(subject); message.setText(content, isHtml(content));
            String from = config("smtp_from");
            String fromName = config("smtp_from_name");
            if (from.isBlank()) from = config("smtp_username");
            if (from.isBlank()) throw new IllegalArgumentException("发件人地址不能为空");
            if (!from.isBlank()) {
                if (fromName.isBlank()) message.setFrom(from);
                else message.setFrom(from, fromName);
            }
            sender.send(message.getMimeMessage());
        } catch (Exception error) {
            Throwable root = error;
            while (root.getCause() != null) root = root.getCause();
            String detail = root.getMessage() == null ? root.getClass().getSimpleName() : root.getMessage().replaceAll("(?i)(password|token|authorization)=[^,\\s]+", "$1=***");
            throw new IllegalArgumentException("邮件发送失败：" + detail, error);
        }
    }

    private String config(String name) { return configOr(name, ""); }
    private boolean isHtml(String content) { return content != null && content.trim().toLowerCase(java.util.Locale.ROOT).startsWith("<!doctype html"); }
    private String render(String template, String code, long expiry, String resetUrl) {
        String appName = configOr("app_name", "TMS");
        return template.replace("{{code}}", code == null ? "" : code)
                .replace("{{expires_minutes}}", String.valueOf(Math.max(1, expiry / 60)))
                .replace("{{reset_url}}", resetUrl == null ? "" : resetUrl)
                .replace("{{app_name}}", appName);
    }
    private String configOr(String name, String fallback) {
        ViteConfig value = configs.getOne(new QueryWrapper<ViteConfig>().eq("name", name));
        if (value == null || value.getValue() == null || value.getValue().trim().isEmpty()) return fallback;
        return value.getValue().trim();
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

    private record RateWindow(long startedAt, int count) {}

    public enum Purpose { REGISTER, PASSWORD_RESET }
}
