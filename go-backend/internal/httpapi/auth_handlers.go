package httpapi

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/mail"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/ziyue67/tms/go-backend/internal/auth"
	"github.com/ziyue67/tms/go-backend/internal/store"
	"github.com/ziyue67/tms/go-backend/internal/verification"
)

type loginRequest struct {
	Username  string `json:"username"`
	Password  string `json:"password"`
	CaptchaID string `json:"captchaId"`
	EmailCode string `json:"emailCode"`
}

type emailRequest struct {
	Email string `json:"email"`
}

type registerRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Code     string `json:"code"`
	Username string `json:"username"`
}

type resetPasswordRequest struct {
	Email       string `json:"email"`
	Code        string `json:"code"`
	Token       string `json:"token"`
	NewPassword string `json:"newPassword"`
	SnakeNew    string `json:"new_password"`
	Password    string `json:"password"`
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

func (a *API) sendRegisterCode(w http.ResponseWriter, r *http.Request) {
	var request emailRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	email, ok := validEmail(request.Email)
	if !ok {
		writeResponse(w, Error(500, "邮箱格式错误"))
		return
	}
	if err := a.ensureRegistrationAllowed(r.Context(), email); err != nil {
		writeResponse(w, Failure(err.Error()))
		return
	}
	emailVerification, err := a.configEnabledContext(r.Context(), "registration_email_verification", true)
	if err != nil {
		a.configReadError(w, err)
		return
	}
	if !emailVerification {
		writeResponse(w, Failure("当前注册未启用邮箱验证，无需发送验证码"))
		return
	}
	if _, err := a.store.UserByEmail(r.Context(), email); err == nil {
		writeResponse(w, Failure("邮箱已注册"))
		return
	} else if !store.IsNotFound(err) {
		a.logger.Error("registration email lookup failed", "error", err)
		writeResponse(w, Error(-2, "注册服务不可用"))
		return
	}
	if a.verification == nil {
		writeResponse(w, Failure("认证服务不可用：Redis 未配置"))
		return
	}
	if err := a.verification.SendRegistration(r.Context(), email, clientIP(r)); err != nil {
		writeResponse(w, Failure(err.Error()))
		return
	}
	writeResponse(w, OK(nil))
}

func (a *API) sendResetCode(w http.ResponseWriter, r *http.Request) {
	var request emailRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	email, ok := validEmail(request.Email)
	if !ok {
		writeResponse(w, Error(500, "邮箱格式错误"))
		return
	}
	if a.verification == nil {
		writeResponse(w, Failure("认证服务不可用：Redis 未配置"))
		return
	}
	if err := a.verification.CheckSendRate(email, clientIP(r), verification.PasswordReset); err != nil {
		writeResponse(w, Failure(err.Error()))
		return
	}
	if _, err := a.store.UserByEmail(r.Context(), email); err == nil {
		if err := a.verification.SendResetAfterRateCheck(r.Context(), email); err != nil {
			writeResponse(w, Failure(err.Error()))
			return
		}
	} else if !store.IsNotFound(err) {
		a.logger.Error("password reset email lookup failed", "error", err)
		writeResponse(w, Error(-2, "认证服务不可用"))
		return
	}
	writeResponse(w, OK(nil))
}

func (a *API) resetPassword(w http.ResponseWriter, r *http.Request) {
	var request resetPasswordRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	email, ok := validEmail(request.Email)
	if !ok {
		writeResponse(w, Error(500, "邮箱格式错误"))
		return
	}
	newPassword := request.NewPassword
	if newPassword == "" {
		newPassword = request.SnakeNew
	}
	if newPassword == "" {
		newPassword = request.Password
	}
	if size := utf8.RuneCountInString(newPassword); size < 6 || size > 72 {
		writeResponse(w, Error(500, "密码长度必须为 6 至 72 个字符"))
		return
	}
	credential := strings.TrimSpace(request.Token)
	if credential == "" {
		credential = strings.TrimSpace(request.Code)
	}
	user, err := a.store.UserByEmail(r.Context(), email)
	if err != nil || a.verification == nil || !a.verification.Consume(r.Context(), email, credential, verification.PasswordReset) {
		writeResponse(w, Failure("验证码错误或已过期"))
		return
	}
	encoded, err := auth.HashPassword(newPassword)
	if err != nil {
		writeResponse(w, Error(-2, "密码重置失败"))
		return
	}
	if err := a.store.UpdatePassword(r.Context(), user.ID, encoded); err != nil {
		a.logger.Error("password reset update failed", "user_id", user.ID, "error", err)
		writeResponse(w, Error(-2, "密码重置失败"))
		return
	}
	writeResponse(w, OK(nil))
}

func (a *API) register(w http.ResponseWriter, r *http.Request) {
	var request registerRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	email, ok := validEmail(request.Email)
	if !ok {
		writeResponse(w, Error(500, "邮箱格式错误"))
		return
	}
	if size := utf8.RuneCountInString(request.Password); size < 6 || size > 72 {
		writeResponse(w, Error(500, "密码长度必须为 6 至 72 个字符"))
		return
	}
	if err := a.ensureRegistrationAllowed(r.Context(), email); err != nil {
		writeResponse(w, Failure(err.Error()))
		return
	}
	if _, err := a.store.UserByEmail(r.Context(), email); err == nil {
		writeResponse(w, Failure("邮箱已注册"))
		return
	} else if !store.IsNotFound(err) {
		writeResponse(w, Error(-2, "注册服务不可用"))
		return
	}
	username := strings.TrimSpace(request.Username)
	if username == "" {
		username = email[:strings.LastIndex(email, "@")]
	}
	exists, err := a.store.UsernameExists(r.Context(), username)
	if err != nil {
		writeResponse(w, Error(-2, "注册服务不可用"))
		return
	}
	if exists {
		writeResponse(w, Failure("用户名已存在"))
		return
	}
	requiresCode, err := a.configEnabledContext(r.Context(), "registration_email_verification", true)
	if err != nil {
		a.configReadError(w, err)
		return
	}
	if requiresCode && (a.verification == nil || !a.verification.Consume(r.Context(), email, request.Code, verification.Register)) {
		writeResponse(w, Failure("验证码错误或已过期"))
		return
	}
	encoded, err := auth.HashPassword(request.Password)
	if err != nil {
		writeResponse(w, Error(-2, "注册失败"))
		return
	}
	user, err := a.store.CreateUser(r.Context(), store.NewUser{Username: username, Email: email, Password: encoded})
	if err != nil {
		a.logger.Error("create registered user failed", "error", err)
		writeResponse(w, Failure("注册失败"))
		return
	}
	token, err := a.tokens.Generate(user.ID, user.Username, user.RoleID)
	if err != nil {
		writeResponse(w, Error(-2, "注册失败"))
		return
	}
	writeResponse(w, OK(map[string]any{"token": token, "name": user.Username, "role_id": user.RoleID}))
}

func (a *API) testEmail(w http.ResponseWriter, r *http.Request) {
	var request emailRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	if a.verification == nil {
		writeResponse(w, Failure("认证服务不可用：Redis 未配置"))
		return
	}
	if err := a.verification.SendTest(r.Context(), request.Email); err != nil {
		writeResponse(w, Failure(err.Error()))
		return
	}
	writeResponse(w, OK(nil))
}

func (a *API) emailAudit(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.ParseInt(r.URL.Query().Get("limit"), 10, 64)
	if limit == 0 {
		limit = 50
	}
	writeResponse(w, OK(a.verification.Audit(r.Context(), limit)))
}

func (a *API) emailHealth(w http.ResponseWriter, r *http.Request) {
	if a.verification == nil {
		writeResponse(w, OK(map[string]any{"redisConfigured": false, "redisAvailable": false}))
		return
	}
	writeResponse(w, OK(a.verification.Health(r.Context())))
}

func (a *API) ensureRegistrationAllowed(ctx context.Context, email string) error {
	enabled, err := a.configEnabledContext(ctx, "registration_enabled", true)
	if err != nil {
		return err
	}
	if !enabled {
		return errors.New("管理员已关闭新用户注册")
	}
	domain := email[strings.LastIndex(email, "@")+1:]
	whitelist, err := a.configValue(ctx, "registration_email_whitelist", "")
	if err != nil || strings.TrimSpace(whitelist) == "" {
		return err
	}
	for _, raw := range strings.FieldsFunc(strings.ToLower(whitelist), func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r' || r == ' ' || r == '\t'
	}) {
		rule := strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(raw), "@"), "*.")
		if rule != "" && (domain == rule || strings.HasSuffix(domain, "."+rule)) {
			return nil
		}
	}
	limited, err := a.configEnabledContext(ctx, "registration_non_whitelist_domain_limit", false)
	if err != nil {
		return err
	}
	if !limited {
		return errors.New("该邮箱域名不在注册白名单内")
	}
	count, err := a.store.CountUsersByEmailDomain(ctx, domain)
	if err != nil {
		return err
	}
	if count > 0 {
		return errors.New("该非白名单邮箱域名已注册过账号")
	}
	return nil
}

func (a *API) configEnabledContext(ctx context.Context, name string, fallback bool) (bool, error) {
	value, err := a.configValue(ctx, name, strconv.FormatBool(fallback))
	if err != nil {
		return false, err
	}
	return strings.EqualFold(strings.TrimSpace(value), "true"), nil
}

func (a *API) configValue(ctx context.Context, name, fallback string) (string, error) {
	item, err := a.store.ConfigValue(ctx, name)
	if store.IsNotFound(err) {
		return fallback, nil
	}
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(item.Value) == "" {
		return fallback, nil
	}
	return strings.TrimSpace(item.Value), nil
}

func validEmail(raw string) (string, bool) {
	normalized := verification.NormalizeEmail(raw)
	parsed, err := mail.ParseAddress(normalized)
	return normalized, err == nil && parsed.Address == normalized && strings.Contains(normalized, "@")
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

func (a *API) configEnabled(r *http.Request, name string, fallback bool) (bool, error) {
	return a.configEnabledContext(r.Context(), name, fallback)
}

func (a *API) configReadError(w http.ResponseWriter, err error) {
	a.logger.Error("read configuration", "error", err)
	writeResponse(w, Error(-2, "读取配置失败"))
}
