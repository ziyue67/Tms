package httpapi

import (
	"net/http"
	"strings"

	"github.com/ziyue67/tms/go-backend/internal/auth"
	"github.com/ziyue67/tms/go-backend/internal/store"
)

type loginRequest struct {
	Username  string `json:"username"`
	Password  string `json:"password"`
	CaptchaID string `json:"captchaId"`
	EmailCode string `json:"emailCode"`
}

func (a *API) login(w http.ResponseWriter, r *http.Request) {
	var request loginRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	if strings.TrimSpace(request.Username) == "" {
		writeResponse(w, Error(500, "用户名不能为空"))
		return
	}
	if request.Password == "" {
		writeResponse(w, Error(500, "密码不能为空"))
		return
	}

	if enabled, err := a.configEnabled(r, "captcha_enabled", false); err != nil {
		a.logger.Error("read captcha configuration", "error", err)
		writeResponse(w, Error(-2, "读取验证码配置失败"))
		return
	} else if enabled {
		// Fail closed until the captcha protocol is migrated; bypassing it would weaken login security.
		writeResponse(w, Failure("验证码校验失败"))
		return
	}

	user, err := a.store.UserByLogin(r.Context(), request.Username)
	if err != nil {
		if !store.IsNotFound(err) {
			a.logger.Error("login user lookup failed", "error", err)
		}
		writeResponse(w, Failure("账号或密码错误"))
		return
	}
	valid, needsUpgrade := auth.VerifyPassword(user.Password, request.Password)
	if !valid {
		writeResponse(w, Failure("账号或密码错误"))
		return
	}
	if user.Status == 0 {
		writeResponse(w, Failure("账户停用"))
		return
	}
	if needsUpgrade {
		encoded, hashErr := auth.HashPassword(request.Password)
		if hashErr != nil {
			a.logger.Error("legacy password upgrade failed", "user_id", user.ID, "error", hashErr)
			writeResponse(w, Error(-2, "登录失败"))
			return
		}
		if updateErr := a.store.UpdatePassword(r.Context(), user.ID, encoded); updateErr != nil {
			a.logger.Error("save upgraded password failed", "user_id", user.ID, "error", updateErr)
			writeResponse(w, Error(-2, "登录失败"))
			return
		}
	}

	token, err := a.tokens.Generate(user.ID, user.Username, user.RoleID)
	if err != nil {
		a.logger.Error("generate login token failed", "user_id", user.ID, "error", err)
		writeResponse(w, Error(-2, "登录失败"))
		return
	}
	writeResponse(w, OK(map[string]any{
		"token":                 token,
		"name":                  user.Username,
		"role_id":               user.RoleID,
		"requirePasswordChange": request.Username == "admin_user" || request.Password == "admin_user",
	}))
}

func (a *API) authConfig(w http.ResponseWriter, r *http.Request) {
	captcha, err := a.configEnabled(r, "captcha_enabled", false)
	if err != nil {
		a.configReadError(w, err)
		return
	}
	registration, err := a.configEnabled(r, "registration_enabled", true)
	if err != nil {
		a.configReadError(w, err)
		return
	}
	emailVerification, err := a.configEnabled(r, "registration_email_verification", true)
	if err != nil {
		a.configReadError(w, err)
		return
	}
	writeResponse(w, OK(map[string]any{
		"captchaEnabled":           captcha,
		"registrationEnabled":      registration,
		"emailVerificationEnabled": emailVerification,
	}))
}

func (a *API) configEnabled(r *http.Request, name string, fallback bool) (bool, error) {
	item, err := a.store.ConfigValue(r.Context(), name)
	if store.IsNotFound(err) {
		return fallback, nil
	}
	if err != nil {
		return false, err
	}
	if strings.TrimSpace(item.Value) == "" {
		return fallback, nil
	}
	return strings.EqualFold(strings.TrimSpace(item.Value), "true"), nil
}

func (a *API) configReadError(w http.ResponseWriter, err error) {
	a.logger.Error("read configuration", "error", err)
	writeResponse(w, Error(-2, "读取配置失败"))
}
