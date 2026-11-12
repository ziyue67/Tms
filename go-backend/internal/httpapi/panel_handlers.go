package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/ziyue67/tms/go-backend/internal/store"
)

// 妙妙屋四面板 handlers:规则集 / 模板管理 / 订阅链接。
// 转发管理页复用既有 /forward /tunnel /landing 接口,不在此文件。
//
// 路由约定(见 router.go):
//   admin 组(要管理员):
//     POST /panel/ruleset/list            列表(不带 content)
//     POST /panel/ruleset/get             {id} 详情(带 content)
//     POST /panel/ruleset/create          {name,displayName,source,remoteUrl,refreshMinutes,content}
//     POST /panel/ruleset/update          {id,...}
//     POST /panel/ruleset/delete          {id}
//     POST /panel/ruleset/refresh         {id} 立即抓取远程规则集
//     POST /panel/template/list
//     POST /panel/template/get            {id}
//     POST /panel/template/create
//     POST /panel/template/update
//     POST /panel/template/delete
//     POST /panel/link/list
//     POST /panel/link/create
//     POST /panel/link/update
//     POST /panel/link/reset-token        {id}
//     POST /panel/link/delete
//   公开(客户端拉订阅):
//     GET  /open_api/panel/{token}        按 format 输出 clash / v2ray
//     GET  /open_api/panel-ruleset/{name} 规则集文件下载(名字带随机后缀防猜)

var rulesetNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

// panelRequest 统一的 {id} 请求体。
type panelIDRequest struct {
	ID int64 `json:"id"`
}

func writePanelError(w http.ResponseWriter, message string) {
	writeResponse(w, Failure(message))
}

// ---------- 规则集 ----------

func (a *API) panelRulesetList(w http.ResponseWriter, r *http.Request) {
	list, err := a.store.Rulesets(r.Context())
	if err != nil {
		writePanelError(w, "加载规则集失败: "+err.Error())
		return
	}
	writeResponse(w, OK(list))
}

func (a *API) panelRulesetGet(w http.ResponseWriter, r *http.Request) {
	var req panelIDRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	item, content, err := a.store.RulesetByID(r.Context(), req.ID)
	if err != nil {
		writePanelError(w, "规则集不存在")
		return
	}
	writeResponse(w, OK(map[string]any{"ruleset": item, "content": content}))
}

func validateRulesetInput(input *store.RulesetInput) string {
	input.Name = strings.TrimSpace(input.Name)
	if !rulesetNameRe.MatchString(input.Name) {
		return "名称只能用字母数字._-,以字母数字开头,最长 64"
	}
	if input.Source != "manual" && input.Source != "remote" {
		input.Source = "manual"
	}
	if input.Source == "remote" {
		u, err := url.Parse(strings.TrimSpace(input.RemoteURL))
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return "远程地址必须是有效的 http(s) URL"
		}
		input.RemoteURL = u.String()
		if input.RefreshMinutes < 0 {
			input.RefreshMinutes = 0
		}
		// 最小 5 分钟:几十个规则集都设 1 分钟会把上游打成 DDoS。
		if input.RefreshMinutes > 0 && input.RefreshMinutes < 5 {
			input.RefreshMinutes = 5
		}
	} else {
		input.RemoteURL = ""
		input.RefreshMinutes = 0
	}
	if len(input.Content) > 4<<20 {
		return "规则集内容超过 4MiB 上限"
	}
	return ""
}

func (a *API) panelRulesetCreate(w http.ResponseWriter, r *http.Request) {
	var input store.RulesetInput
	if !decodeJSON(w, r, &input) {
		return
	}
	if msg := validateRulesetInput(&input); msg != "" {
		writePanelError(w, msg)
		return
	}
	item, err := a.store.CreateRuleset(r.Context(), input)
	if err != nil {
		writePanelError(w, "创建规则集失败: "+err.Error())
		return
	}
	writeResponse(w, OK(item))
}

func (a *API) panelRulesetUpdate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		panelIDRequest
		store.RulesetInput
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if msg := validateRulesetInput(&req.RulesetInput); msg != "" {
		writePanelError(w, msg)
		return
	}
	item, err := a.store.UpdateRuleset(r.Context(), req.ID, req.RulesetInput)
	if err != nil {
		writePanelError(w, "更新规则集失败: "+err.Error())
		return
	}
	writeResponse(w, OK(item))
}

func (a *API) panelRulesetDelete(w http.ResponseWriter, r *http.Request) {
	var req panelIDRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := a.store.DeleteRuleset(r.Context(), req.ID); err != nil {
		writePanelError(w, "删除规则集失败: "+err.Error())
		return
	}
	writeResponse(w, OK(true))
}

// fetchRulesetRemote 抓取远程规则集。失败时不清空已有内容 —— 上次抓到的还能继续服务。
func fetchRulesetRemote(ctx context.Context, target string) (string, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("上游返回 %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20+1))
	if err != nil {
		return "", err
	}
	if len(body) > 4<<20 {
		return "", fmt.Errorf("内容超过 4MiB 上限")
	}
	return string(body), nil
}

func (a *API) panelRulesetRefresh(w http.ResponseWriter, r *http.Request) {
	var req panelIDRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	item, _, err := a.store.RulesetByID(r.Context(), req.ID)
	if err != nil {
		writePanelError(w, "规则集不存在")
		return
	}
	if item.Source != "remote" || item.RemoteURL == "" {
		writePanelError(w, "仅远程来源的规则集可刷新")
		return
	}
	content, fetchErr := fetchRulesetRemote(r.Context(), item.RemoteURL)
	if err := a.store.MarkRulesetFetched(r.Context(), req.ID, content, errString(fetchErr)); err != nil {
		writePanelError(w, "保存抓取结果失败: "+err.Error())
		return
	}
	if fetchErr != nil {
		writePanelError(w, "抓取失败: "+fetchErr.Error())
		return
	}
	writeResponse(w, OK(true))
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	s := err.Error()
	if len(s) > 500 {
		s = s[:500]
	}
	return s
}

// ---------- 订阅模板 ----------

func validateTemplateInput(input *store.TemplateInput) string {
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || len(input.Name) > 128 {
		return "模板名不能为空且最长 128"
	}
	input.Version = strings.ToLower(strings.TrimSpace(input.Version))
	if input.Version != "v2" && input.Version != "v3" {
		input.Version = "v3"
	}
	if len(input.Content) > 4<<20 {
		return "模板内容超过 4MiB 上限"
	}
	if input.Version == "v3" && strings.TrimSpace(input.Content) != "" {
		// v3 是 JSON;只做能解析校验,不约束 schema —— 面板允许自由编辑。
		var probe map[string]any
		if err := json.Unmarshal([]byte(input.Content), &probe); err != nil {
			return "v3 模板内容必须是合法 JSON: " + err.Error()
		}
	}
	input.Remark = strings.TrimSpace(input.Remark)
	if len(input.Remark) > 255 {
		input.Remark = input.Remark[:255]
	}
	return ""
}

func (a *API) panelTemplateList(w http.ResponseWriter, r *http.Request) {
	list, err := a.store.SubscribeTemplates(r.Context())
	if err != nil {
		writePanelError(w, "加载模板失败: "+err.Error())
		return
	}
	writeResponse(w, OK(list))
}

func (a *API) panelTemplateGet(w http.ResponseWriter, r *http.Request) {
	var req panelIDRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	item, err := a.store.SubscribeTemplateByID(r.Context(), req.ID)
	if err != nil {
		writePanelError(w, "模板不存在")
		return
	}
	writeResponse(w, OK(item))
}

func (a *API) panelTemplateCreate(w http.ResponseWriter, r *http.Request) {
	var input store.TemplateInput
	if !decodeJSON(w, r, &input) {
		return
	}
	if msg := validateTemplateInput(&input); msg != "" {
		writePanelError(w, msg)
		return
	}
	item, err := a.store.CreateSubscribeTemplate(r.Context(), input)
	if err != nil {
		writePanelError(w, "创建模板失败: "+err.Error())
		return
	}
	writeResponse(w, OK(item))
}

func (a *API) panelTemplateUpdate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		panelIDRequest
		store.TemplateInput
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if msg := validateTemplateInput(&req.TemplateInput); msg != "" {
		writePanelError(w, msg)
		return
	}
	item, err := a.store.UpdateSubscribeTemplate(r.Context(), req.ID, req.TemplateInput)
	if err != nil {
		writePanelError(w, "更新模板失败: "+err.Error())
		return
	}
	writeResponse(w, OK(item))
}

func (a *API) panelTemplateDelete(w http.ResponseWriter, r *http.Request) {
	var req panelIDRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := a.store.DeleteSubscribeTemplate(r.Context(), req.ID); err != nil {
		writePanelError(w, "删除模板失败: "+err.Error())
		return
	}
	writeResponse(w, OK(true))
}

// ---------- 订阅链接 ----------

var linkFormats = map[string]bool{"clash": true, "v2ray": true, "singbox": true}

func validateLinkInput(input *store.LinkInput) string {
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || len(input.Name) > 128 {
		return "链接名不能为空且最长 128"
	}
	input.Format = strings.ToLower(strings.TrimSpace(input.Format))
	if !linkFormats[input.Format] {
		input.Format = "clash"
	}
	input.RulesetIDs = store.NormalizeRulesetIDs(input.RulesetIDs)
	return ""
}

func (a *API) panelLinkList(w http.ResponseWriter, r *http.Request) {
	list, err := a.store.SubscribeLinks(r.Context())
	if err != nil {
		writePanelError(w, "加载订阅链接失败: "+err.Error())
		return
	}
	writeResponse(w, OK(list))
}

func (a *API) panelLinkCreate(w http.ResponseWriter, r *http.Request) {
	var input store.LinkInput
	if !decodeJSON(w, r, &input) {
		return
	}
	if msg := validateLinkInput(&input); msg != "" {
		writePanelError(w, msg)
		return
	}
	item, err := a.store.CreateSubscribeLink(r.Context(), input)
	if err != nil {
		writePanelError(w, "创建订阅链接失败: "+err.Error())
		return
	}
	writeResponse(w, OK(item))
}

func (a *API) panelLinkUpdate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		panelIDRequest
		store.LinkInput
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if msg := validateLinkInput(&req.LinkInput); msg != "" {
		writePanelError(w, msg)
		return
	}
	item, err := a.store.UpdateSubscribeLink(r.Context(), req.ID, req.LinkInput)
	if err != nil {
		writePanelError(w, "更新订阅链接失败: "+err.Error())
		return
	}
	writeResponse(w, OK(item))
}

func (a *API) panelLinkResetToken(w http.ResponseWriter, r *http.Request) {
	var req panelIDRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	token, err := a.store.ResetSubscribeLinkToken(r.Context(), req.ID)
	if err != nil {
		writePanelError(w, "重置失败: "+err.Error())
		return
	}
	writeResponse(w, OK(map[string]any{"token": token}))
}

func (a *API) panelLinkDelete(w http.ResponseWriter, r *http.Request) {
	var req panelIDRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := a.store.DeleteSubscribeLink(r.Context(), req.ID); err != nil {
		writePanelError(w, "删除订阅链接失败: "+err.Error())
		return
	}
	writeResponse(w, OK(true))
}

// ---------- 公开下载端点 ----------

// panelSubscriptionEntry 订阅输出里的一个节点(转发链上落地机的入站)。
// 复用 store.SubscriptionOutput —— panel 订阅链接本质是把账号订阅节点按模板/规则集渲染。
func (a *API) loadPanelSubscription(w http.ResponseWriter, r *http.Request) (store.SubscribeLink, store.SubscriptionOutput, bool) {
	token := strings.TrimSpace(chi.URLParam(r, "token"))
	link, err := a.store.SubscribeLinkByToken(r.Context(), token)
	if err != nil {
		http.Error(w, "订阅链接无效或已过期", http.StatusNotFound)
		return link, store.SubscriptionOutput{}, false
	}
	output, err := a.store.SubscriptionByToken(r.Context(), link.Token)
	if err != nil || output.UserID == 0 || !output.Usable {
		http.Error(w, "订阅关联的账号不可用", http.StatusForbidden)
		return link, output, false
	}
	if err := a.store.BumpSubscribeLinkVisits(r.Context(), link.ID); err != nil {
		// 计数失败不影响输出。
		_ = err
	}
	return link, output, true
}

func (a *API) panelSubscription(w http.ResponseWriter, r *http.Request) {
	link, output, ok := a.loadPanelSubscription(w, r)
	if !ok {
		return
	}
	links := buildSubscriptionLinks(output)
	proxies := buildClashProxies(output)
	switch link.Format {
	case "v2ray":
		if len(links) == 0 {
			http.Error(w, "No available subscription nodes", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Subscription-Userinfo", subscriptionUserinfo(output))
		_, _ = w.Write([]byte(strings.Join(links, "\n")))
	default: // clash
		if len(proxies) == 0 {
			http.Error(w, "No available subscription nodes", http.StatusNotFound)
			return
		}
		yaml := renderPanelClashYAML(r.Context(), a.store, link, proxies)
		w.Header().Set("Content-Type", "text/yaml; charset=utf-8")
		w.Header().Set("Subscription-Userinfo", subscriptionUserinfo(output))
		_, _ = w.Write([]byte(yaml))
	}
}

// subscriptionUserinfo 流量头,客户端(Shadowrocket 等)拿来显示用量。
func subscriptionUserinfo(output store.SubscriptionOutput) string {
	return fmt.Sprintf("upload=%d; download=%d; total=%d; expire=%d", output.Upload, output.Download, output.Total, output.Expires)
}

// renderPanelClashYAML 用订阅模板渲染 clash 配置。模板缺失/损坏时退回内置默认骨架,
// 订阅永远能出东西 —— 空配置比报错对车友友好。
func renderPanelClashYAML(ctx context.Context, s panelStore, link store.SubscribeLink, proxies []map[string]any) string {
	type tf struct {
		Content string
		Version string
	}
	var template *tf
	if link.TemplateID > 0 {
		if t, err := s.SubscribeTemplateByID(ctx, link.TemplateID); err == nil {
			template = &tf{Content: t.Content, Version: t.Version}
		}
	} else if id, err := s.DefaultTemplateID(ctx); err == nil && id > 0 {
		if t, err := s.SubscribeTemplateByID(ctx, id); err == nil {
			template = &tf{Content: t.Content, Version: t.Version}
		}
	}
	names := make([]string, 0, len(proxies))
	for _, p := range proxies {
		names = append(names, fmt.Sprint(p["name"]))
	}
	if template == nil || strings.TrimSpace(template.Content) == "" || template.Version != "v3" {
		return defaultPanelClashYAML(proxies, names)
	}
	var root map[string]any
	if err := json.Unmarshal([]byte(template.Content), &root); err != nil {
		return defaultPanelClashYAML(proxies, names)
	}
	applyPanelTemplate(root, link, proxies, names)
	encoded, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return defaultPanelClashYAML(proxies, names)
	}
	return string(encoded)
}

// applyPanelTemplate 把节点与规则集挂进模板骨架:
//   - 模板里没有 proxies/proxy-groups 就补默认组;
//   - ruleset_ids 指到的规则集替换模板 rules 数组里的 $RULESET$ 占位符(按顺序)。
func applyPanelTemplate(root map[string]any, link store.SubscribeLink, proxies []map[string]any, names []string) {
	root["proxies"] = proxies
	if _, ok := root["proxy-groups"]; !ok {
		root["proxy-groups"] = []map[string]any{
			{"name": "节点选择", "type": "select", "proxies": append([]string{"自动选择"}, names...)},
			{"name": "自动选择", "type": "url-test", "proxies": names, "url": "http://www.gstatic.com/generate_204", "interval": 300, "tolerance": 50},
			{"name": "漏网之鱼", "type": "select", "proxies": []string{"节点选择", "DIRECT"}},
		}
	}
	if link.RulesetIDs == "" {
		return
	}
	ruleLines := []string{}
	for _, idStr := range strings.Split(link.RulesetIDs, ",") {
		id, err := strconv.ParseInt(strings.TrimSpace(idStr), 10, 64)
		if err != nil {
			continue
		}
		_, content, err := panelStoreRulesetContent(id)
		if err != nil || content == "" {
			continue
		}
		for _, line := range strings.Split(content, "\n") {
			line = strings.TrimSpace(line)
			if line != "" && !strings.HasPrefix(line, "#") {
				ruleLines = append(ruleLines, line)
			}
		}
	}
	if len(ruleLines) == 0 {
		return
	}
	// 规则集行替换 MATCH 兜底行之前的位置:规则集优先,兜底垫底。
	rules, _ := root["rules"].([]any)
	final := []any{}
	matched := false
	for _, rl := range rules {
		str, _ := rl.(string)
		if strings.HasPrefix(strings.ToUpper(str), "MATCH") && !matched {
			final = append(final, anySlice(ruleLines)...)
			matched = true
		}
		final = append(final, str)
	}
	if !matched {
		final = append(final, anySlice(ruleLines)...)
	}
	root["rules"] = final
}

func anySlice(items []string) []any {
	out := make([]any, len(items))
	for i, s := range items {
		out[i] = s
	}
	return out
}

// panelStore 渲染需要的最小存储面,便于测试注入。
type panelStore interface {
	SubscribeTemplateByID(context.Context, int64) (store.SubscribeTemplate, error)
	DefaultTemplateID(context.Context) (int64, error)
}

// panelStoreRulesetContent 由 handler 层注入的规则集读取函数,避免 store 循环依赖。
var panelStoreRulesetContent = func(id int64) (meta any, content string, err error) {
	return nil, "", fmt.Errorf("ruleset loader not wired")
}

// SetPanelRulesetLoader 由 router.go 装配时注入真实读取函数。
func SetPanelRulesetLoader(loader func(id int64) (string, error)) {
	panelStoreRulesetContent = func(id int64) (any, string, error) {
		content, err := loader(id)
		return nil, content, err
	}
}

// defaultPanelClashYAML 内置兜底配置(与 openClashSubscription 同骨架)。
func defaultPanelClashYAML(proxies []map[string]any, names []string) string {
	root := map[string]any{"mixed-port": 7890, "allow-lan": false, "mode": "rule", "log-level": "info",
		"dns":     map[string]any{"enable": true, "ipv6": false, "enhanced-mode": "fake-ip", "fake-ip-range": "198.18.0.1/16", "nameserver": []string{"223.5.5.5", "119.29.29.29"}},
		"proxies": proxies, "rules": []string{"GEOIP,CN,DIRECT", "MATCH,漏网之鱼"}}
	root["proxy-groups"] = []map[string]any{
		{"name": "节点选择", "type": "select", "proxies": append([]string{"自动选择"}, names...)},
		{"name": "自动选择", "type": "url-test", "proxies": names, "url": "http://www.gstatic.com/generate_204", "interval": 300, "tolerance": 50},
		{"name": "漏网之鱼", "type": "select", "proxies": []string{"节点选择", "DIRECT"}},
	}
	encoded, _ := json.MarshalIndent(root, "", "  ")
	return string(encoded)
}

// panelRulesetFile 规则集公开下载:/open_api/panel-ruleset/{name}。
func (a *API) panelRulesetFile(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if !rulesetNameRe.MatchString(name) {
		http.Error(w, "invalid ruleset name", http.StatusNotFound)
		return
	}
	content, updated, err := a.store.RulesetContentByName(r.Context(), name)
	if err != nil {
		http.Error(w, "ruleset not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if updated > 0 {
		w.Header().Set("Last-Modified", time.UnixMilli(updated).UTC().Format(http.TimeFormat))
	}
	_, _ = w.Write([]byte(content))
}
