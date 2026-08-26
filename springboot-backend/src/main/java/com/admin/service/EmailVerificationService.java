package com.admin.service;

import org.springframework.beans.factory.annotation.Value;
import com.admin.entity.ViteConfig;
import com.baomidou.mybatisplus.core.conditions.query.QueryWrapper;
import org.springframework.mail.javamail.JavaMailSenderImpl;
import org.springframework.mail.javamail.MimeMessageHelper;
import org.springframework.stereotype.Service;
import org.springframework.jdbc.core.JdbcTemplate;
import org.springframework.transaction.annotation.Transactional;

import java.security.SecureRandom;
import java.time.Instant;
import java.util.Map;
import java.util.concurrent.ConcurrentHashMap;

/** 邮箱验证码服务。验证码只保存 BCrypt 摘要，并且只能成功消费一次。 */
@Service
public class EmailVerificationService {
    private final ViteConfigService configs;
    private final JdbcTemplate jdbc;
    private final org.springframework.security.crypto.bcrypt.BCryptPasswordEncoder encoder =
            new org.springframework.security.crypto.bcrypt.BCryptPasswordEncoder();
    private final Map<String, RateWindow> emailSendWindows = new ConcurrentHashMap<>();
    private final Map<String, RateWindow> ipSendWindows = new ConcurrentHashMap<>();
    private final SecureRandom random = new SecureRandom();
    private final long expireSeconds;
    private final long cooldownSeconds;

    public EmailVerificationService(ViteConfigService configs,
                                    JdbcTemplate jdbc,
                                    @Value("${tms.auth.verification-expire-seconds:600}") long expireSeconds,
                                    @Value("${tms.auth.verification-cooldown-seconds:60}") long cooldownSeconds) {
        this.configs = configs;
        this.jdbc = jdbc;
        this.expireSeconds = Math.max(60, expireSeconds);
        this.cooldownSeconds = Math.max(10, cooldownSeconds);
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
        sendCode(normalize(email), purpose);
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
        String subject = purpose == Purpose.PASSWORD_RESET ? "TMS 密码重置验证码" : "TMS 注册验证码";
        String action = purpose == Purpose.PASSWORD_RESET ? "重置密码" : "注册账号";
        sendMessage(mailSender, normalized, subject, "你的 TMS " + action + "验证码是 " + code + "，有效期 " + (expiry / 60) + " 分钟。请勿将验证码泄露给他人。");
        // Persist only the BCrypt digest. The plaintext code is never written to the database or logs.
        jdbc.update("DELETE FROM verification_code WHERE email = ? AND purpose = ?", normalized, purpose.name());
        jdbc.update("INSERT INTO verification_code (email, purpose, code_hash, expires_at, sent_at, attempts, consumed_at) VALUES (?, ?, ?, ?, ?, 0, NULL)",
                normalized, purpose.name(), encoder.encode(code), now + expiry, now);
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
