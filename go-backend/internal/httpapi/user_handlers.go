package httpapi

import (
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/ziyue67/tms/go-backend/internal/auth"
	"github.com/ziyue67/tms/go-backend/internal/store"
)

type managedUserRequest struct {
	ID            *int64 `json:"id"`
	Username      string `json:"user"`
	Email         string `json:"email"`
	Password      string `json:"pwd"`
	Flow          *int64 `json:"flow"`
	ForwardLimit  *int   `json:"num"`
	ExpiryTime    *int64 `json:"expTime"`
	FlowResetTime *int64 `json:"flowResetTime"`
	Status        *int   `json:"status"`
}

func (a *API) createUser(w http.ResponseWriter, r *http.Request) {
	request, ok := decodeManagedUser(w, r, false)
	if !ok {
		return
	}
	exists, err := a.store.UsernameExists(r.Context(), request.Username)
	if err != nil {
		writeResponse(w, Error(-2, "用户创建失败"))
		return
	}
	if exists {
		writeResponse(w, Failure("用户名已存在"))
		return
	}
	encoded, err := auth.HashPassword(request.Password)
	if err != nil {
		writeResponse(w, Error(-2, "用户创建失败"))
		return
	}
	input := request.input()
	input.Password = encoded
	if err := a.store.CreateManagedUser(r.Context(), input); err != nil {
		a.logger.Error("create managed user", "error", err)
		writeResponse(w, Failure("用户创建失败"))
		return
	}
	writeResponse(w, OK("用户创建成功"))
}

func (a *API) listUsers(w http.ResponseWriter, r *http.Request) {
	users, err := a.store.Users(r.Context())
	if err != nil {
		a.logger.Error("list users", "error", err)
		writeResponse(w, Error(-2, "查询用户失败"))
		return
	}
	writeResponse(w, OK(users))
}

func (a *API) updateUser(w http.ResponseWriter, r *http.Request) {
	request, ok := decodeManagedUser(w, r, true)
	if !ok {
		return
	}
	existing, err := a.store.UserByID(r.Context(), *request.ID)
	if store.IsNotFound(err) {
		writeResponse(w, Failure("用户不存在"))
		return
	}
	if err != nil {
		writeResponse(w, Failure("用户更新失败"))
		return
	}
	if existing.RoleID == 0 {
		writeResponse(w, Failure("不能修改管理员用户信息"))
		return
	}
	taken, err := a.store.UsernameExistsExcept(r.Context(), request.Username, existing.ID)
	if err != nil {
		writeResponse(w, Failure("用户更新失败"))
		return
	}
	if taken {
		writeResponse(w, Failure("用户名已被其他用户使用"))
		return
	}
	input := request.input()
	if strings.TrimSpace(request.Password) != "" {
		if utf8.RuneCountInString(request.Password) > 72 {
			writeResponse(w, Error(500, "密码长度不能超过72个字符"))
			return
		}
		input.Password, err = auth.HashPassword(request.Password)
		if err != nil {
			writeResponse(w, Failure("用户更新失败"))
			return
		}
	}
	if err := a.store.UpdateManagedUser(r.Context(), existing.ID, input); err != nil {
		a.logger.Error("update managed user", "user_id", existing.ID, "error", err)
		writeResponse(w, Failure("用户更新失败"))
		return
	}
	writeResponse(w, OK("用户更新成功"))
}

func (a *API) deleteUser(w http.ResponseWriter, r *http.Request) {
	id, ok := decodeID(w, r)
	if !ok {
		return
	}
	user, err := a.store.UserByID(r.Context(), id)
	if store.IsNotFound(err) {
		writeResponse(w, Failure("用户不存在"))
		return
	}
	if err != nil {
		writeResponse(w, Failure("用户删除失败"))
		return
	}
	if user.RoleID == 0 {
		writeResponse(w, Failure("不能删除管理员用户"))
		return
	}
	if err := a.deleteUserData(r, id); err != nil {
		writeResponse(w, Failure("删除用户时发生错误："+err.Error()))
		return
	}
	writeResponse(w, OK("用户及关联数据删除成功"))
}

func (a *API) deleteCurrentUser(w http.ResponseWriter, r *http.Request) {
	claims, ok := claimsFrom(r)
	if !ok {
		writeResponse(w, Failure("用户未登录或token无效"))
		return
	}
	id, err := claims.UserID()
	if err != nil {
		writeResponse(w, Failure("用户未登录或token无效"))
		return
	}
	if _, err := a.store.UserByID(r.Context(), id); store.IsNotFound(err) {
		writeResponse(w, Failure("用户不存在"))
		return
	}
	if err := a.deleteUserData(r, id); err != nil {
		writeResponse(w, Failure("注销账户时发生错误："+err.Error()))
		return
	}
	writeResponse(w, OK("用户及关联数据删除成功"))
}

func (a *API) deleteUserData(r *http.Request, userID int64) error {
	commands, err := a.store.UserCleanupCommands(r.Context(), userID)
	if err != nil {
		return err
	}
	if a.nodeHub != nil {
		for _, command := range commands {
			_ = a.nodeHub.SendCommand(r.Context(), command.InNodeID, "DeleteService", map[string]any{"services": []string{command.Name + "_tcp", command.Name + "_udp"}})
			if command.Tunnel {
				_ = a.nodeHub.SendCommand(r.Context(), command.InNodeID, "DeleteChains", map[string]any{"chain": command.Name + "_chains"})
				_ = a.nodeHub.SendCommand(r.Context(), command.OutNodeID, "DeleteService", map[string]any{"services": []string{command.Name + "_tls"}})
			}
		}
	}
	return a.store.DeleteUserCascade(r.Context(), userID)
}

func (a *API) userPackage(w http.ResponseWriter, r *http.Request) {
	claims, ok := claimsFrom(r)
	if !ok {
		writeResponse(w, Failure("用户未登录或token无效"))
		return
	}
	id, err := claims.UserID()
	if err != nil {
		writeResponse(w, Failure("用户未登录或token无效"))
		return
	}
	result, err := a.store.UserPackage(r.Context(), id)
	if store.IsNotFound(err) {
		writeResponse(w, Failure("用户不存在"))
		return
	}
	if err != nil {
		a.logger.Error("get user package", "user_id", id, "error", err)
		writeResponse(w, Failure("获取套餐信息失败"))
		return
	}
	writeResponse(w, OK(result))
}

func (a *API) updateCurrentPassword(w http.ResponseWriter, r *http.Request) {
	var request struct {
		NewUsername     string `json:"newUsername"`
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
		ConfirmPassword string `json:"confirmPassword"`
	}
	if !decodeJSON(w, r, &request) {
		return
	}
	if strings.TrimSpace(request.NewUsername) == "" || request.CurrentPassword == "" || request.NewPassword == "" || request.ConfirmPassword == "" {
		writeResponse(w, Error(500, "请求参数不能为空"))
		return
	}
	if request.NewPassword != request.ConfirmPassword {
		writeResponse(w, Failure("新密码和确认密码不匹配"))
		return
	}
	claims, _ := claimsFrom(r)
	id, err := claims.UserID()
	if err != nil {
		writeResponse(w, Failure("用户未登录或token无效"))
		return
	}
	user, err := a.store.UserByID(r.Context(), id)
	if err != nil {
		writeResponse(w, Failure("用户不存在"))
		return
	}
	valid, _ := auth.VerifyPassword(user.Password, request.CurrentPassword)
	if !valid {
		writeResponse(w, Failure("当前密码错误"))
		return
	}
	if user.Username != request.NewUsername {
		taken, err := a.store.UsernameExistsExcept(r.Context(), request.NewUsername, id)
		if err != nil || taken {
			writeResponse(w, Failure("用户名已被其他用户使用"))
			return
		}
	}
	encoded, err := auth.HashPassword(request.NewPassword)
	if err != nil || a.store.UpdateUsernamePassword(r.Context(), id, request.NewUsername, encoded) != nil {
		writeResponse(w, Failure("用户更新失败"))
		return
	}
	writeResponse(w, OK("账号密码修改成功"))
}

func (a *API) resetUserFlow(w http.ResponseWriter, r *http.Request) {
	var request struct {
		ID   *int64 `json:"id"`
		Type *int   `json:"type"`
	}
	if !decodeJSON(w, r, &request) {
		return
	}
	if request.ID == nil {
		writeResponse(w, Error(500, "重置账号id不能为空"))
		return
	}
	if request.Type == nil {
		writeResponse(w, Error(500, "重置类型不能为空"))
		return
	}
	var updated bool
	var err error
	if *request.Type == 1 {
		updated, err = a.store.ResetUserFlow(r.Context(), *request.ID)
		if !updated && err == nil {
			writeResponse(w, Failure("用户不存在"))
			return
		}
	} else {
		updated, err = a.store.ResetUserTunnelFlow(r.Context(), *request.ID)
		if !updated && err == nil {
			writeResponse(w, Failure("隧道不存在"))
			return
		}
	}
	if err != nil {
		writeResponse(w, Error(-2, err.Error()))
		return
	}
	writeResponse(w, OK(nil))
}

func decodeManagedUser(w http.ResponseWriter, r *http.Request, update bool) (managedUserRequest, bool) {
	var request managedUserRequest
	if !decodeJSON(w, r, &request) {
		return request, false
	}
	if update && request.ID == nil {
		writeResponse(w, Error(500, "用户ID不能为空"))
		return request, false
	}
	if strings.TrimSpace(request.Username) == "" {
		writeResponse(w, Error(500, "用户名不能为空"))
		return request, false
	}
	if !update && request.Password == "" {
		writeResponse(w, Error(500, "密码不能为空"))
		return request, false
	}
	if request.Flow == nil || *request.Flow < 0 {
		writeResponse(w, Error(500, "流量不能为空或小于0"))
		return request, false
	}
	if request.ForwardLimit == nil || *request.ForwardLimit < 0 {
		writeResponse(w, Error(500, "转发数量不能为空或小于0"))
		return request, false
	}
	if request.ExpiryTime == nil || request.FlowResetTime == nil {
		writeResponse(w, Error(500, "过期时间和流量重置时间不能为空"))
		return request, false
	}
	if request.Status == nil {
		status := 1
		request.Status = &status
	}
	return request, true
}

func (r managedUserRequest) input() store.ManagedUserInput {
	return store.ManagedUserInput{Username: r.Username, Email: strings.TrimSpace(r.Email), Password: r.Password,
		Flow: *r.Flow, ForwardLimit: *r.ForwardLimit, ExpiryTime: *r.ExpiryTime,
		FlowResetTime: *r.FlowResetTime, Status: *r.Status}
}
