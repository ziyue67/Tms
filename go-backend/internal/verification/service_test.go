package verification

import (
	"context"
	"database/sql"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/ziyue67/tms/go-backend/internal/config"
	"github.com/ziyue67/tms/go-backend/internal/store"
)

type fakeConfigs map[string]string

func (f fakeConfigs) ConfigValue(_ context.Context, name string) (store.SiteConfig, error) {
	value, ok := f[name]
	if !ok {
		return store.SiteConfig{}, sql.ErrNoRows
	}
	return store.SiteConfig{Name: name, Value: value}, nil
}

type recordingMailer struct {
	messages []Message
}

func (m *recordingMailer) Send(_ SMTPSettings, message Message) error {
	m.messages = append(m.messages, message)
	return nil
}

func newTestService(t *testing.T) (*Service, *miniredis.Miniredis, *recordingMailer) {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	mailer := &recordingMailer{}
	configs := fakeConfigs{
		"smtp_host": "smtp.example.com", "smtp_port": "587", "smtp_username": "sender@example.com",
		"smtp_password": "secret", "smtp_from": "sender@example.com", "app_name": "TMS Test",
	}
	service := New(config.Auth{VerificationExpiry: 10 * time.Minute, VerificationCooldown: time.Minute,
		ResetURLBase: "https://panel.example.com"}, client, configs, mailer)
	t.Cleanup(func() { _ = client.Close() })
	return service, server, mailer
}

func TestRegistrationCodeIsHashedAndSingleUse(t *testing.T) {
	service, server, mailer := newTestService(t)
	ctx := context.Background()
	if err := service.SendRegistration(ctx, "User@Example.com", "127.0.0.1"); err != nil {
		t.Fatalf("send registration: %v", err)
	}
	if len(mailer.messages) != 1 {
		t.Fatalf("expected one email, got %d", len(mailer.messages))
	}
	match := regexp.MustCompile(`[0-9]{6}`).FindString(mailer.messages[0].Body)
	if match == "" {
		t.Fatal("verification code missing from email")
	}
	stored, err := server.Get(redisKey(Register, "user@example.com"))
	if err != nil {
		t.Fatalf("stored code: %v", err)
	}
	if stored == match || stored != digest(match) {
		t.Fatal("Redis must contain only the code digest")
	}
	if !service.Consume(ctx, "user@example.com", match, Register) {
		t.Fatal("valid code was rejected")
	}
	if service.Consume(ctx, "user@example.com", match, Register) {
		t.Fatal("verification code was reusable")
	}
}

func TestResetTokenUsesConfiguredPublicURL(t *testing.T) {
	service, _, mailer := newTestService(t)
	ctx := context.Background()
	if err := service.SendResetAfterRateCheck(ctx, "user@example.com"); err != nil {
		t.Fatalf("send reset: %v", err)
	}
	match := regexp.MustCompile(`https://panel\.example\.com/reset-password\?[^\"]+`).FindString(mailer.messages[0].Body)
	if match == "" {
		t.Fatal("reset URL missing from email")
	}
	parsed, err := url.Parse(strings.ReplaceAll(match, "&amp;", "&"))
	if err != nil {
		t.Fatalf("parse reset URL: %v", err)
	}
	token := parsed.Query().Get("token")
	if token == "" || !service.Consume(ctx, "user@example.com", token, PasswordReset) {
		t.Fatal("reset token was not consumable")
	}
}

func TestRateLimitMatchesLegacyThreshold(t *testing.T) {
	service, _, _ := newTestService(t)
	for attempt := 0; attempt < 5; attempt++ {
		if err := service.CheckSendRate("user@example.com", "127.0.0.1", Register); err != nil {
			t.Fatalf("attempt %d unexpectedly failed: %v", attempt+1, err)
		}
	}
	if err := service.CheckSendRate("user@example.com", "127.0.0.1", Register); err == nil || err.Error() != "该邮箱发送次数过多，请一小时后再试" {
		t.Fatalf("expected email rate limit, got %v", err)
	}
}
