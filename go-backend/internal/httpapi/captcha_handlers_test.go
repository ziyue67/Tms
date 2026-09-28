package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/ziyue67/tms/go-backend/internal/auth"
	"github.com/ziyue67/tms/go-backend/internal/config"
	"github.com/ziyue67/tms/go-backend/internal/store"
)

func TestCaptchaTokenIsSingleUseAtLogin(t *testing.T) {
	server := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = redisClient.Close() })
	dataStore := &fakeStore{user: store.User{ID: 1, Username: "admin", Password: "3c85cdebade1c51cf64ca9f3c09d182d", RoleID: 0, Status: 1}, configs: map[string]string{"captcha_enabled": "true"}}
	tokens := auth.NewTokenService("test-secret", time.Hour)
	handler := New(Dependencies{Config: config.Config{JWTSecret: "test-secret"}, Store: dataStore, Tokens: tokens, Redis: redisClient, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})

	generated := performJSONRequest(t, handler, http.MethodPost, "/api/v1/captcha/generate", "")
	data := generated["data"].(map[string]any)
	question := data["question"].(string)
	numbers := regexp.MustCompile(`[0-9]+`).FindAllString(question, -1)
	left, _ := strconv.Atoi(numbers[0])
	right, _ := strconv.Atoi(numbers[1])
	verifyBody, _ := json.Marshal(map[string]any{"id": data["id"], "answer": strconv.Itoa(left + right)})
	verified := performJSONRequest(t, handler, http.MethodPost, "/api/v1/captcha/verify", string(verifyBody))
	validToken := verified["data"].(map[string]any)["validToken"].(string)

	loginBody := `{"username":"admin","password":"admin_user","captchaId":"` + validToken + `"}`
	first := performJSONRequest(t, handler, http.MethodPost, "/api/v1/user/login", loginBody)
	if first["code"].(float64) != 0 {
		t.Fatalf("verified login failed: %#v", first)
	}
	second := performJSONRequest(t, handler, http.MethodPost, "/api/v1/user/login", loginBody)
	if second["code"].(float64) == 0 || !strings.Contains(second["msg"].(string), "验证码") {
		t.Fatalf("captcha token was reusable: %#v", second)
	}
}

func performJSONRequest(t *testing.T, handler http.Handler, method, path, body string) map[string]any {
	t.Helper()
	request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	request = request.WithContext(context.Background())
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	result := map[string]any{}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode %s: %v (%s)", path, err, response.Body.String())
	}
	return result
}
