package verification

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/ziyue67/tms/go-backend/internal/config"
	"github.com/ziyue67/tms/go-backend/internal/store"
)

type Purpose string

const (
	Register      Purpose = "REGISTER"
	PasswordReset Purpose = "PASSWORD_RESET"
)

const consumeScript = `
local value = redis.call('GET', KEYS[1])
if not value then return 0 end
local sep = string.find(value, '|')
local expected = value
local attempts = 0
if sep then expected = string.sub(value, 1, sep - 1); attempts = tonumber(string.sub(value, sep + 1)) or 0 end
if ARGV[1] == expected then redis.call('DEL', KEYS[1]); return 1 end
attempts = attempts + 1
if attempts >= 5 then redis.call('DEL', KEYS[1]) else local ttl = redis.call('PTTL', KEYS[1]); redis.call('SET', KEYS[1], expected .. '|' .. attempts, 'PX', ttl) end
return -1`

const registerTemplate = `<!DOCTYPE html><html><head><meta charset="UTF-8"></head><body><h1>{{app_name}}</h1><h2>邮箱验证码</h2><p>请使用下面的验证码完成注册：</p><p style="font-size:32px;font-weight:bold;letter-spacing:8px">{{code}}</p><p>验证码将在 {{expires_minutes}} 分钟后失效。</p></body></html>`
const resetTemplate = `<!DOCTYPE html><html><head><meta charset="UTF-8"></head><body><h1>{{app_name}}</h1><h2>密码重置请求</h2><p><a href="{{reset_url}}">重置密码</a></p><p>该链接将在 {{expires_minutes}} 分钟后失效且只能使用一次。</p></body></html>`

type ConfigReader interface {
	ConfigValue(context.Context, string) (store.SiteConfig, error)
}

type rateWindow struct {
	started time.Time
	count   int
}

type Service struct {
	redis       *redis.Client
	configs     ConfigReader
	mailer      Mailer
	expiry      time.Duration
	cooldown    time.Duration
	resetURL    string
	mu          sync.Mutex
	emailLimits map[string]rateWindow
	ipLimits    map[string]rateWindow
}

func New(cfg config.Auth, client *redis.Client, configs ConfigReader, mailer Mailer) *Service {
	if mailer == nil {
		mailer = SMTPMailer{}
	}
	return &Service{redis: client, configs: configs, mailer: mailer, expiry: cfg.VerificationExpiry,
		cooldown: cfg.VerificationCooldown, resetURL: strings.TrimRight(cfg.ResetURLBase, "/"),
		emailLimits: make(map[string]rateWindow), ipLimits: make(map[string]rateWindow)}
}

func NormalizeEmail(email string) string { return strings.ToLower(strings.TrimSpace(email)) }

func (s *Service) CheckSendRate(email, clientIP string, purpose Purpose) error {
	normalized := NormalizeEmail(email)
	if clientIP == "" {
		clientIP = "unknown"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := enforceRate(s.emailLimits, strings.ToLower(string(purpose))+":"+normalized, 5, "该邮箱发送次数过多，请一小时后再试"); err != nil {
		return err
	}
	return enforceRate(s.ipLimits, strings.ToLower(string(purpose))+":"+clientIP, 20, "该 IP 发送次数过多，请一小时后再试")
}

func enforceRate(windows map[string]rateWindow, key string, maximum int, message string) error {
	now := time.Now()
	window := windows[key]
	if window.started.IsZero() || now.Sub(window.started) >= time.Hour {
		windows[key] = rateWindow{started: now, count: 1}
		return nil
	}
	if window.count >= maximum {
		return errors.New(message)
	}
	window.count++
	windows[key] = window
	return nil
}

func (s *Service) SendRegistration(ctx context.Context, email, clientIP string) error {
	if err := s.CheckSendRate(email, clientIP, Register); err != nil {
		return err
	}
	return s.sendCode(ctx, NormalizeEmail(email), Register)
}

func (s *Service) SendResetAfterRateCheck(ctx context.Context, email string) error {
	return s.sendResetToken(ctx, NormalizeEmail(email))
}

func (s *Service) sendCode(ctx context.Context, email string, purpose Purpose) error {
	if err := s.requireRedis(); err != nil {
		return err
	}
	cooldown := s.durationConfig(ctx, "email_code_cooldown_seconds", s.cooldown, 10*time.Second, time.Hour)
	expiry := s.durationConfig(ctx, "email_code_expire_seconds", s.expiry, time.Minute, 24*time.Hour)
	reserved, err := s.redis.SetNX(ctx, cooldownKey(purpose, email), "1", cooldown).Result()
	if err != nil {
		return redisUnavailable(err)
	}
	if !reserved {
		return errors.New("验证码发送过于频繁，请稍后再试")
	}
	var randomBytes [4]byte
	if _, err := rand.Read(randomBytes[:]); err != nil {
		return err
	}
	randomValue := uint32(randomBytes[0])<<24 | uint32(randomBytes[1])<<16 | uint32(randomBytes[2])<<8 | uint32(randomBytes[3])
	code := fmt.Sprintf("%06d", randomValue%1_000_000)
	if err := s.redis.Set(ctx, redisKey(purpose, email), digest(code), expiry).Err(); err != nil {
		s.cleanup(ctx, purpose, email)
		return redisUnavailable(err)
	}
	subject := s.configOr(ctx, "email_register_subject", "TMS 注册验证码")
	body := render(s.configOr(ctx, "email_register_template", registerTemplate), s.configOr(ctx, "app_name", "TMS"), code, expiry, "")
	if err := s.send(ctx, email, subject, body); err != nil {
		s.cleanup(ctx, purpose, email)
		return err
	}
	s.audit(ctx, email, purpose, "sent", 0)
	return nil
}

func (s *Service) sendResetToken(ctx context.Context, email string) error {
	if err := s.requireRedis(); err != nil {
		return err
	}
	cooldown := s.durationConfig(ctx, "email_code_cooldown_seconds", s.cooldown, 10*time.Second, time.Hour)
	expiry := s.durationConfig(ctx, "email_code_expire_seconds", s.expiry, time.Minute, 24*time.Hour)
	reserved, err := s.redis.SetNX(ctx, cooldownKey(PasswordReset, email), "1", cooldown).Result()
	if err != nil {
		return redisUnavailable(err)
	}
	if !reserved {
		return errors.New("验证码发送过于频繁，请稍后再试")
	}
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return err
	}
	token := base64.RawURLEncoding.EncodeToString(bytes)
	link := s.resetURL + "/reset-password?email=" + url.QueryEscape(email) + "&token=" + url.QueryEscape(token)
	if err := s.redis.Set(ctx, redisKey(PasswordReset, email), digest(token), expiry).Err(); err != nil {
		s.cleanup(ctx, PasswordReset, email)
		return redisUnavailable(err)
	}
	subject := s.configOr(ctx, "email_reset_subject", "TMS 密码重置")
	body := render(s.configOr(ctx, "email_reset_template", resetTemplate), s.configOr(ctx, "app_name", "TMS"), "", expiry, link)
	if err := s.send(ctx, email, subject, body); err != nil {
		s.cleanup(ctx, PasswordReset, email)
		return err
	}
	s.audit(ctx, email, PasswordReset, "sent", 0)
	return nil
}

func (s *Service) Consume(ctx context.Context, email, credential string, purpose Purpose) bool {
	if s.redis == nil || strings.TrimSpace(credential) == "" {
		return false
	}
	result, err := s.redis.Eval(ctx, consumeScript, []string{redisKey(purpose, NormalizeEmail(email))}, digest(strings.TrimSpace(credential))).Int()
	if err != nil {
		s.audit(ctx, email, purpose, "redis_unavailable", 0)
		return false
	}
	event := "rejected"
	attempts := 0
	if result == 1 {
		event = "consumed"
	} else if result == -1 {
		attempts = 1
	}
	s.audit(ctx, email, purpose, event, attempts)
	return result == 1
}

func (s *Service) SendTest(ctx context.Context, email string) error {
	if strings.TrimSpace(email) == "" {
		return errors.New("测试邮箱不能为空")
	}
	return s.send(ctx, strings.TrimSpace(email), "TMS SMTP 测试", "TMS SMTP 配置测试成功。")
}

func (s *Service) Audit(ctx context.Context, limit int64) []string {
	if s.redis == nil {
		return []string{}
	}
	if limit < 1 {
		limit = 1
	}
	if limit > 200 {
		limit = 200
	}
	values, err := s.redis.LRange(ctx, "tms:auth:audit", -limit, -1).Result()
	if err != nil {
		return []string{}
	}
	return values
}

func (s *Service) Health(ctx context.Context) map[string]any {
	result := map[string]any{"redisConfigured": s.redis != nil}
	if s.redis == nil {
		result["redisAvailable"] = false
		return result
	}
	if err := s.redis.Ping(ctx).Err(); err != nil {
		result["redisAvailable"] = false
		result["message"] = "Redis 无法连接，请检查 REDIS_HOST、REDIS_PORT 和 REDIS_PASSWORD 是否与 Redis 服务一致"
		return result
	}
	result["redisAvailable"] = true
	return result
}

func (s *Service) send(ctx context.Context, recipient, subject, body string) error {
	settings, err := s.smtpSettings(ctx)
	if err != nil {
		return err
	}
	if err := s.mailer.Send(settings, Message{To: recipient, Subject: subject, Body: body, HTML: strings.HasPrefix(strings.ToLower(strings.TrimSpace(body)), "<!doctype html")}); err != nil {
		return fmt.Errorf("邮件发送失败：%s", redact(err.Error()))
	}
	return nil
}

func (s *Service) smtpSettings(ctx context.Context) (SMTPSettings, error) {
	host := s.configOr(ctx, "smtp_host", "")
	port, err := strconv.Atoi(s.configOr(ctx, "smtp_port", "587"))
	if err != nil {
		port = 587
	}
	username := s.configOr(ctx, "smtp_username", "")
	settings := SMTPSettings{Host: host, Port: port, Username: username, Password: s.configOr(ctx, "smtp_password", ""),
		From: s.configOr(ctx, "smtp_from", username), FromName: s.configOr(ctx, "smtp_from_name", "")}
	configuredStartTLS, _ := strconv.ParseBool(s.configOr(ctx, "smtp_starttls", strconv.FormatBool(port == 587)))
	configuredSSL, _ := strconv.ParseBool(s.configOr(ctx, "smtp_ssl", strconv.FormatBool(port == 465)))
	settings.SSL = port == 465 || (configuredSSL && port != 587)
	settings.StartTLS = !settings.SSL && (configuredStartTLS || port == 587)
	return settings, validateSMTP(settings)
}

func (s *Service) configOr(ctx context.Context, name, fallback string) string {
	value, err := s.configs.ConfigValue(ctx, name)
	if err != nil || strings.TrimSpace(value.Value) == "" {
		return fallback
	}
	return strings.TrimSpace(value.Value)
}

func (s *Service) durationConfig(ctx context.Context, name string, fallback, minimum, maximum time.Duration) time.Duration {
	seconds, err := strconv.ParseInt(s.configOr(ctx, name, strconv.FormatInt(int64(fallback/time.Second), 10)), 10, 64)
	if err != nil {
		return fallback
	}
	value := time.Duration(seconds) * time.Second
	if value < minimum {
		return minimum
	}
	if value > maximum {
		return maximum
	}
	return value
}

func (s *Service) audit(ctx context.Context, email string, purpose Purpose, event string, attempts int) {
	if s.redis == nil {
		return
	}
	record := fmt.Sprintf("%s|%s|%s|%d|%d", purpose, event, digest(NormalizeEmail(email)), time.Now().Unix(), attempts)
	pipe := s.redis.TxPipeline()
	pipe.RPush(ctx, "tms:auth:audit", record)
	pipe.LTrim(ctx, "tms:auth:audit", -1000, -1)
	_, _ = pipe.Exec(ctx)
}

func (s *Service) cleanup(ctx context.Context, purpose Purpose, email string) {
	if s.redis != nil {
		_ = s.redis.Del(ctx, redisKey(purpose, email), cooldownKey(purpose, email)).Err()
	}
}

func (s *Service) requireRedis() error {
	if s.redis == nil {
		return errors.New("认证服务不可用：Redis 未配置")
	}
	return nil
}

func redisKey(purpose Purpose, email string) string {
	return "tms:auth:verify:" + string(purpose) + ":" + email
}
func cooldownKey(purpose Purpose, email string) string {
	return "tms:auth:cooldown:" + string(purpose) + ":" + email
}
func digest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
func render(template, appName, code string, expiry time.Duration, resetURL string) string {
	return strings.NewReplacer("{{app_name}}", appName, "{{code}}", code, "{{expires_minutes}}",
		strconv.FormatInt(max(1, int64(expiry/time.Minute)), 10), "{{reset_url}}", resetURL).Replace(template)
}
func redisUnavailable(err error) error {
	return fmt.Errorf("认证服务不可用：Redis 无法连接，请检查 REDIS_HOST、REDIS_PORT 和 REDIS_PASSWORD: %w", err)
}

var secretPattern = regexp.MustCompile(`(?i)(password|token|authorization)=[^,\s]+`)

func redact(value string) string { return secretPattern.ReplaceAllString(value, "$1=***") }
