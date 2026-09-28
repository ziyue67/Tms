package httpapi

import (
	"net/http"
	"strings"

	"github.com/ziyue67/tms/go-backend/internal/store"
)

var publicConfigNames = map[string]struct{}{
	"app_name":        {},
	"captcha_enabled": {},
	"captcha_type":    {},
}

func (a *API) publicConfigs(w http.ResponseWriter, r *http.Request) {
	items, err := a.store.Configs(r.Context())
	if err != nil {
		a.configReadError(w, err)
		return
	}
	result := make(map[string]string, len(publicConfigNames))
	for _, item := range items {
		if _, public := publicConfigNames[item.Name]; public {
			result[item.Name] = item.Value
		}
	}
	writeResponse(w, OK(result))
}

func (a *API) publicConfigValue(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Name any `json:"name"`
	}
	if !decodeJSON(w, r, &request) {
		return
	}
	name, ok := request.Name.(string)
	if !ok {
		writeResponse(w, Failure("该配置不允许公开读取"))
		return
	}
	if _, public := publicConfigNames[name]; !public {
		writeResponse(w, Failure("该配置不允许公开读取"))
		return
	}
	item, err := a.store.ConfigValue(r.Context(), name)
	if store.IsNotFound(err) {
		writeResponse(w, Failure("配置不存在"))
		return
	}
	if err != nil {
		a.configReadError(w, err)
		return
	}
	writeResponse(w, OK(item))
}

func (a *API) privateConfigs(w http.ResponseWriter, r *http.Request) {
	items, err := a.store.Configs(r.Context())
	if err != nil {
		a.configReadError(w, err)
		return
	}
	result := make(map[string]string, len(items))
	for _, item := range items {
		result[item.Name] = item.Value
	}
	writeResponse(w, OK(result))
}

func (a *API) updateConfigs(w http.ResponseWriter, r *http.Request) {
	values := make(map[string]string)
	if !decodeJSON(w, r, &values) {
		return
	}
	if len(values) == 0 {
		writeResponse(w, Failure("配置数据不能为空"))
		return
	}
	if err := a.store.UpsertConfigs(r.Context(), values); err != nil {
		a.logger.Error("update configurations", "error", err)
		writeResponse(w, Failure("配置更新失败: "+err.Error()))
		return
	}
	writeResponse(w, OK("配置更新成功"))
}

func (a *API) updateSingleConfig(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	}
	if !decodeJSON(w, r, &request) {
		return
	}
	if strings.TrimSpace(request.Name) == "" {
		writeResponse(w, Failure("配置名称不能为空"))
		return
	}
	if strings.TrimSpace(request.Value) == "" {
		writeResponse(w, Failure("配置值不能为空"))
		return
	}
	if err := a.store.UpsertConfigs(r.Context(), map[string]string{request.Name: request.Value}); err != nil {
		a.logger.Error("update configuration", "name", request.Name, "error", err)
		writeResponse(w, Failure("配置更新失败: "+err.Error()))
		return
	}
	writeResponse(w, OK("配置更新成功"))
}
