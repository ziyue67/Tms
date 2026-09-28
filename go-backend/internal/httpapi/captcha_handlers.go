package httpapi

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (a *API) captchaCheck(w http.ResponseWriter, r *http.Request) {
	enabled, err := a.configEnabledContext(r.Context(), "captcha_enabled", false)
	if err != nil {
		writeResponse(w, Error(-2, "读取验证码配置失败"))
		return
	}
	if enabled {
		writeResponse(w, OK(1))
		return
	}
	writeResponse(w, OK(0))
}

func (a *API) captchaGenerate(w http.ResponseWriter, r *http.Request) {
	if a.redis == nil {
		writeResponse(w, Failure("验证码服务不可用"))
		return
	}
	bytes := make([]byte, 18)
	if _, err := rand.Read(bytes); err != nil {
		writeResponse(w, Error(-2, "验证码生成失败"))
		return
	}
	id := hex.EncodeToString(bytes)
	operands := make([]byte, 2)
	_, _ = rand.Read(operands)
	left := int(operands[0]%20) + 1
	right := int(operands[1]%20) + 1
	answer := strconv.Itoa(left + right)
	if err := a.redis.Set(r.Context(), "tms:captcha:answer:"+id, sha256Text(answer), 2*time.Minute).Err(); err != nil {
		writeResponse(w, Failure("验证码服务不可用"))
		return
	}
	writeResponse(w, OK(map[string]any{"id": id, "question": fmt.Sprintf("%d + %d = ?", left, right), "expiresIn": 120}))
}

func (a *API) captchaVerify(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID        string `json:"id"`
		Answer    string `json:"answer"`
		CaptchaID string `json:"captchaId"`
		TrackData string `json:"trackData"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.ID == "" {
		body.ID = body.CaptchaID
	}
	if body.Answer == "" {
		body.Answer = body.TrackData
	}
	if a.redis == nil || body.ID == "" || strings.TrimSpace(body.Answer) == "" {
		writeResponse(w, Failure("验证码错误或已过期"))
		return
	}
	script := `local v=redis.call('GET',KEYS[1]); if not v then return 0 end; if v~=ARGV[1] then return -1 end; redis.call('DEL',KEYS[1]); redis.call('SET',KEYS[2],'1','EX',120); return 1`
	result, err := a.redis.Eval(r.Context(), script, []string{"tms:captcha:answer:" + body.ID, "tms:captcha:valid:" + body.ID}, sha256Text(strings.TrimSpace(body.Answer))).Int()
	if err != nil || result != 1 {
		writeResponse(w, Failure("验证码错误或已过期"))
		return
	}
	writeResponse(w, OK(map[string]any{"validToken": body.ID}))
}

func sha256Text(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
