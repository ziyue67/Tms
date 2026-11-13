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
	"github.com/ziyue67/tms/go-backend/internal/panelbootstrap"
	"github.com/ziyue67/tms/go-backend/internal/paneldefaults"
	"github.com/ziyue67/tms/go-backend/internal/sharelink"
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
//
// ID 用 flexInt64 而不是 int64:前端为了避免 64 位 id 在 JS 里丢精度,一律传字符串
// ("id":"12"),直接按 int64 解会报「请求参数格式错误」。两种写法都收。
type panelIDRequest struct {
	ID flexInt64 `json:"id"`
}

// flexInt64 同时接受 JSON 数字与数字字符串。
type flexInt64 int64

func (f *flexInt64) UnmarshalJSON(data []byte) error {
	text := strings.TrimSpace(string(data))
	if text == "" || text == "null" {
		*f = 0
		return nil
	}
	text = strings.Trim(text, `"`)
	if text == "" {
		*f = 0
		return nil
	}
	value, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return fmt.Errorf("id 必须是整数")
	}
	*f = flexInt64(value)
	return nil
}

// value 取回 int64,调用处保持原来的签名。
func (r panelIDRequest) value() int64 { return int64(r.ID) }

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
	item, content, err := a.store.RulesetByID(r.Context(), req.value())
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
	item, err := a.store.UpdateRuleset(r.Context(), req.value(), req.RulesetInput)
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
	if err := a.store.DeleteRuleset(r.Context(), req.value()); err != nil {
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
	item, _, err := a.store.RulesetByID(r.Context(), req.value())
	if err != nil {
		writePanelError(w, "规则集不存在")
		return
	}
	if item.Source != "remote" || item.RemoteURL == "" {
		writePanelError(w, "仅远程来源的规则集可刷新")
		return
	}
	content, fetchErr := fetchRulesetRemote(r.Context(), item.RemoteURL)
	if err := a.store.MarkRulesetFetched(r.Context(), req.value(), content, errString(fetchErr)); err != nil {
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

// templateClients 模板的客户端类型 —— 与前端模板管理页的标签页一一对应。
var templateClients = map[string]bool{"clash": true, "surge": true, "loon": true, "quanx": true, "singbox": true}

func validateTemplateInput(input *store.TemplateInput) string {
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || len(input.Name) > 128 {
		return "模板名不能为空且最长 128"
	}
	input.Client = strings.ToLower(strings.TrimSpace(input.Client))
	if !templateClients[input.Client] {
		input.Client = "clash"
	}
	input.Version = strings.ToLower(strings.TrimSpace(input.Version))
	if input.Version != "v2" && input.Version != "v3" {
		input.Version = "v3"
	}
	if input.Visible != 0 {
		input.Visible = 1
	}
	if len(input.Content) > 4<<20 {
		return "模板内容超过 4MiB 上限"
	}
	if input.Version == "v3" && strings.TrimSpace(input.Content) != "" {
		// v3 是结构化骨架:clash 允许 JSON / YAML 文本,sing-box 必须是 JSON。
		if input.Client == "singbox" {
			var probe map[string]any
			if err := json.Unmarshal([]byte(input.Content), &probe); err != nil {
				return "sing-box 模板必须是合法 JSON: " + err.Error()
			}
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
	item, err := a.store.SubscribeTemplateByID(r.Context(), req.value())
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
	item, err := a.store.UpdateSubscribeTemplate(r.Context(), req.value(), req.TemplateInput)
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
	if err := a.store.DeleteSubscribeTemplate(r.Context(), req.value()); err != nil {
		writePanelError(w, "删除模板失败: "+err.Error())
		return
	}
	writeResponse(w, OK(true))
}

// panelTemplateSetDefault 行内「★」:切换默认模板。默认模板全局只有一个。
func (a *API) panelTemplateSetDefault(w http.ResponseWriter, r *http.Request) {
	var req panelIDRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if _, err := a.store.SubscribeTemplateByID(r.Context(), req.value()); err != nil {
		writePanelError(w, "模板不存在")
		return
	}
	if err := a.store.SetSubscribeTemplateDefault(r.Context(), req.value()); err != nil {
		writePanelError(w, "设置默认模板失败: "+err.Error())
		return
	}
	writeResponse(w, OK(true))
}

// panelTemplateReset 行内「刷新」:把内置模板的内容恢复成出厂版本。
// 只对 builtin=1 的模板开放,自建模板没有出厂内容可回滚。
func (a *API) panelTemplateReset(w http.ResponseWriter, r *http.Request) {
	var req panelIDRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	item, err := a.store.SubscribeTemplateByID(r.Context(), req.value())
	if err != nil {
		writePanelError(w, "模板不存在")
		return
	}
	if item.Builtin != 1 {
		writePanelError(w, "只有内置模板支持恢复出厂内容")
		return
	}
	builtin, ok := paneldefaults.TemplateByName(item.Name)
	if !ok {
		writePanelError(w, "内置模板库里找不到「"+item.Name+"」")
		return
	}
	if err := a.store.ResetSubscribeTemplateContent(r.Context(), item.ID, builtin.Content); err != nil {
		writePanelError(w, "恢复失败: "+err.Error())
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
	// 分组订阅:一条链接里多个「节点组 + 各自规则」,此时 nodeIds/rulesetIds 由分组自己带。
	if strings.TrimSpace(input.GroupsJSON) != "" {
		groups, err := store.ParseGroups(input.GroupsJSON)
		if err != nil {
			return "分组配置格式不正确"
		}
		if len(groups) > 20 {
			return "分组最多 20 个"
		}
		normalized, err := store.MarshalGroups(groups)
		if err != nil {
			return "分组配置格式不正确"
		}
		if normalized == "" {
			return "至少给一个分组挑好节点"
		}
		input.GroupsJSON = normalized
		input.NodeIDs = ""
		input.RulesetIDs = ""
		// shared 由前端显式给:勾「全体用户」时是全局订阅集,否则是带独立地址的分组订阅。
		if input.Shared != 0 {
			input.Shared = 1
		}
		if input.UserID > 0 {
			return "分组订阅不能绑定单个账号"
		}
		if len(input.GroupsJSON) > 64<<10 {
			return "分组配置太大"
		}
		return ""
	}
	if input.UserID > 0 {
		// 归属账号模式:节点由账号自己决定,忽略手挑的节点。
		input.NodeIDs = ""
		input.Shared = 0
	} else {
		input.NodeIDs = store.NormalizeNodeIDs(input.NodeIDs)
		// 既没有归属账号、也没挑节点 = 一套「模板 + 规则」的定义(订阅集),
		// 用户在「我的订阅链接」里套用后才会绑定到自己的节点。
		if input.NodeIDs == "" {
			input.Shared = 1
		} else {
			input.Shared = 0
		}
	}
	if len(input.NodeIDs) > 3800 {
		return "选中的节点太多,分几条订阅链接吧"
	}
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
	if input.UserID > 0 {
		if _, err := a.store.UserByID(r.Context(), input.UserID); err != nil {
			writePanelError(w, "归属账号不存在")
			return
		}
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
	if req.LinkInput.UserID > 0 {
		if _, err := a.store.UserByID(r.Context(), req.LinkInput.UserID); err != nil {
			writePanelError(w, "归属账号不存在")
			return
		}
	}
	item, err := a.store.UpdateSubscribeLink(r.Context(), req.value(), req.LinkInput)
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
	token, err := a.store.ResetSubscribeLinkToken(r.Context(), req.value())
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
	if err := a.store.DeleteSubscribeLink(r.Context(), req.value()); err != nil {
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
	if link.Shared == 1 {
		http.Error(w, "这是订阅集,请到「我的订阅链接」里套用后再拉取", http.StatusForbidden)
		return link, store.SubscriptionOutput{}, false
	}
	output, ok := a.panelOutputFor(r.Context(), link)
	if !ok {
		http.Error(w, "这条订阅链接没有可用节点", http.StatusForbidden)
		return link, output, false
	}
	if err := a.store.BumpSubscribeLinkVisits(r.Context(), link.ID); err != nil {
		// 计数失败不影响输出。
		_ = err
	}
	return link, output, true
}

// panelOutputFor 决定这条订阅链接输出哪些节点。
//
//   - 带了 node_ids(新建订阅时从「节点管理」里挑的):直接用这些 custom_node 渲染,
//     不再依赖某个车友的 token —— 之前面板链接拿自己的 token 去当车友 token 查,
//     查不到就 403,链接等于建了不能用;
//   - 老链接(没有 node_ids):沿用「借用车友 all_sub_token / 线路 sub_token」的老路径。
func (a *API) panelOutputFor(ctx context.Context, link store.SubscribeLink) (store.SubscriptionOutput, bool) {
	// 绑了账号的链接(用户自建订阅):输出这个账号自己的节点。
	if link.UserID > 0 {
		output, err := a.store.SubscriptionByUserID(ctx, link.UserID)
		if err != nil || !output.Usable {
			return output, false
		}
		if len(output.Entries) == 0 && len(output.CustomParsed) == 0 {
			return output, false
		}
		return output, true
	}
	if groups, err := store.ParseGroups(link.GroupsJSON); err == nil && len(groups) > 0 {
		return a.nodesOutput(ctx, store.GroupNodeIDs(groups))
	}
	if ids := parsePanelNodeIDs(link.NodeIDs); len(ids) > 0 {
		nodes, err := a.store.PanelNodesByIDs(ctx, ids)
		if err != nil || len(nodes) == 0 {
			return store.SubscriptionOutput{}, false
		}
		output := store.SubscriptionOutput{
			Usable:       true,
			Entries:      []store.SubscriptionEntry{},
			CustomLinks:  make([]string, 0, len(nodes)),
			CustomParsed: make([]map[string]any, 0, len(nodes)),
		}
		for _, node := range nodes {
			if node.RawLink != "" {
				output.CustomLinks = append(output.CustomLinks, node.RawLink)
			}
			parsed := node.Parsed
			if parsed == nil {
				parsed = map[string]any{}
			}
			if _, ok := parsed["protocol"]; !ok {
				parsed["protocol"] = node.Protocol
			}
			if _, ok := parsed["name"]; !ok || parsed["name"] == "" {
				parsed["name"] = node.Name
			}
			output.CustomParsed = append(output.CustomParsed, parsed)
		}
		return output, true
	}
	output, err := a.store.SubscriptionByToken(ctx, link.Token)
	if err != nil || output.UserID == 0 || !output.Usable {
		return output, false
	}
	return output, true
}

// parsePanelNodeIDs 把 "1,2,3" 解析成 id 列表,非法项直接丢。
func parsePanelNodeIDs(raw string) []int64 {
	ids := []int64{}
	for _, part := range strings.Split(raw, ",") {
		id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
		if err == nil && id > 0 {
			ids = append(ids, id)
		}
	}
	return ids
}

func (a *API) panelSubscription(w http.ResponseWriter, r *http.Request) {
	link, output, ok := a.loadPanelSubscription(w, r)
	if !ok {
		return
	}
	links := buildSubscriptionLinks(output)
	proxies := buildClashProxies(output)
	if link.Format == "v2ray" {
		if len(links) == 0 {
			http.Error(w, "No available subscription nodes", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Subscription-Userinfo", subscriptionUserinfo(output))
		_, _ = w.Write([]byte(strings.Join(links, "\n")))
		return
	}
	if len(proxies) == 0 {
		http.Error(w, "No available subscription nodes", http.StatusNotFound)
		return
	}
	if link.Format == "singbox" {
		body := renderPanelSingboxJSON(r.Context(), a.store, link, proxies)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Subscription-Userinfo", subscriptionUserinfo(output))
		_, _ = w.Write([]byte(body))
		return
	}
	yaml := renderPanelClashYAML(r.Context(), a.store, link, proxies, output.GroupNodes)
	w.Header().Set("Content-Type", "text/yaml; charset=utf-8")
	w.Header().Set("Subscription-Userinfo", subscriptionUserinfo(output))
	_, _ = w.Write([]byte(yaml))
}

// subscriptionUserinfo 流量头,客户端(Shadowrocket 等)拿来显示用量。
func subscriptionUserinfo(output store.SubscriptionOutput) string {
	return fmt.Sprintf("upload=%d; download=%d; total=%d; expire=%d", output.Upload, output.Download, output.Total, output.Expires)
}

// panelTemplateSnapshot 渲染时用的模板快照。
type panelTemplateSnapshot struct {
	Content string
	Client  string
	Version string
}

// panelTemplateFor 取这条订阅链接该用的模板:指定了就用指定的,没指定用默认模板,
// 都没有返回 nil(渲染走内置兜底)。
func panelTemplateFor(ctx context.Context, s panelStore, link store.SubscribeLink) *panelTemplateSnapshot {
	load := func(id int64) *panelTemplateSnapshot {
		if id <= 0 {
			return nil
		}
		item, err := s.SubscribeTemplateByID(ctx, id)
		if err != nil {
			return nil
		}
		return &panelTemplateSnapshot{Content: item.Content, Client: item.Client, Version: item.Version}
	}
	if snapshot := load(link.TemplateID); snapshot != nil {
		return snapshot
	}
	if id, err := s.DefaultTemplateID(ctx); err == nil {
		return load(id)
	}
	return nil
}

// renderPanelClashYAML 用订阅模板渲染 clash 配置。模板缺失/损坏时退回内置默认骨架,
// 订阅永远能出东西 —— 空配置比报错对车友友好。
//
// 支持两种模板写法:
//   - JSON 骨架(结构化):解析后塞 proxies / proxy-groups / rules;
//   - YAML 文本(妙妙屋 rule_templates 风格):文本级替换 proxies: null 与
//     __PROXY_NODES__ / __PROXY_PROVIDERS__ 占位符 —— 不引入 YAML 依赖也能
//     原样保留上游模板里的 DNS、rule-providers 等高级段落。
func renderPanelClashYAML(ctx context.Context, s panelStore, link store.SubscribeLink, proxies []map[string]any, groupNodes map[int64]string) string {
	if groups, err := store.ParseGroups(link.GroupsJSON); err == nil && len(groups) > 0 {
		return renderGroupClashYAML(groups, proxies, groupNodes)
	}
	template := panelTemplateFor(ctx, s, link)
	names := make([]string, 0, len(proxies))
	for _, p := range proxies {
		names = append(names, fmt.Sprint(p["name"]))
	}
	if template == nil || strings.TrimSpace(template.Content) == "" || template.Version != "v3" {
		return defaultPanelClashYAML(proxies, names)
	}
	if template.Client != "" && template.Client != "clash" {
		// 绑了 Surge/sing-box 模板却要 clash 输出:退回默认骨架,别硬套。
		return defaultPanelClashYAML(proxies, names)
	}
	ruleLines := collectRulesetRuleLines(link)
	if json.Valid([]byte(template.Content)) {
		var root map[string]any
		if err := json.Unmarshal([]byte(template.Content), &root); err != nil {
			return defaultPanelClashYAML(proxies, names)
		}
		applyPanelTemplate(root, proxies, names, ruleLines)
		encoded, err := json.MarshalIndent(root, "", "  ")
		if err != nil {
			return defaultPanelClashYAML(proxies, names)
		}
		return string(encoded)
	}
	return renderClashTextTemplate(template.Content, proxies, names, ruleLines)
}

// collectRulesetRuleLines 把订阅链接绑定的规则集展开成一行行 clash 规则。
func collectRulesetRuleLines(link store.SubscribeLink) []string {
	if strings.TrimSpace(link.RulesetIDs) == "" {
		return nil
	}
	lines := []string{}
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
			if line != "" && !strings.HasPrefix(line, "#") && !strings.HasPrefix(line, ";") {
				lines = append(lines, line)
			}
		}
	}
	return lines
}

var (
	yamlProxiesKey = regexp.MustCompile(`^proxies:\s*(\[\]|null)?\s*$`)
	yamlNodeLine   = regexp.MustCompile(`^(\s*)-\s*__PROXY_NODES__\s*$`)
	yamlProvLine   = regexp.MustCompile(`^\s*-\s*__PROXY_PROVIDERS__\s*$`)
)

// renderClashTextTemplate YAML 文本模板的注入渲染。
//
//   - 顶层 `proxies:` 段落整段换成真实节点(没有这个键就在末尾补一段);
//   - 策略组 proxies 里的 `- __PROXY_NODES__` 展开成每个节点名;
//   - `- __PROXY_PROVIDERS__` 直接删掉(面板不提供 proxy-providers);
//   - 规则集行插到第一条 MATCH 之前,兜底规则永远垫底。
func renderClashTextTemplate(template string, proxies []map[string]any, names []string, ruleLines []string) string {
	source := strings.Split(strings.ReplaceAll(template, "\r\n", "\n"), "\n")
	out := make([]string, 0, len(source)+len(names)+len(ruleLines))
	proxiesDone, rulesDone := false, false
	for _, line := range source {
		trimmed := strings.TrimSpace(line)
		if !proxiesDone && yamlProxiesKey.MatchString(line) {
			out = append(out, "proxies:")
			for _, proxy := range proxies {
				out = append(out, "  - "+jsonLine(proxy))
			}
			proxiesDone = true
			continue
		}
		if match := yamlNodeLine.FindStringSubmatch(line); match != nil {
			for _, name := range names {
				out = append(out, match[1]+"- "+jsonLine(name))
			}
			continue
		}
		if yamlProvLine.MatchString(line) {
			continue
		}
		if !rulesDone && len(ruleLines) > 0 && strings.HasPrefix(strings.ToUpper(trimmed), "- MATCH,") {
			indent := line[:len(line)-len(strings.TrimLeft(line, " "))]
			for _, rule := range ruleLines {
				out = append(out, indent+"- "+rule)
			}
			rulesDone = true
		}
		out = append(out, line)
	}
	if !proxiesDone {
		out = append(out, "proxies:")
		for _, proxy := range proxies {
			out = append(out, "  - "+jsonLine(proxy))
		}
	}
	return strings.TrimRight(strings.Join(out, "\n"), "\n") + "\n"
}

// jsonLine 把任意值序列化成一行 JSON —— JSON 是合法 YAML,可以直接当 YAML 节点用。
func jsonLine(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "null"
	}
	return string(encoded)
}

// applyPanelTemplate 把节点与规则集挂进 JSON 骨架:
//   - 模板里没有 proxy-groups 就补默认组;
//   - 规则集行插到 rules 数组里第一条 MATCH 之前。
func applyPanelTemplate(root map[string]any, proxies []map[string]any, names []string, ruleLines []string) {
	root["proxies"] = proxies
	if _, ok := root["proxy-groups"]; !ok {
		root["proxy-groups"] = []map[string]any{
			{"name": "节点选择", "type": "select", "proxies": append([]string{"自动选择"}, names...)},
			{"name": "自动选择", "type": "url-test", "proxies": names, "url": "http://www.gstatic.com/generate_204", "interval": 300, "tolerance": 50},
			{"name": "漏网之鱼", "type": "select", "proxies": []string{"节点选择", "DIRECT"}},
		}
	}
	if len(ruleLines) == 0 {
		return
	}
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

// renderPanelSingboxJSON sing-box 输出。模板优先用订阅链接绑定的 sing-box 模板,
// 没绑就用内置 default__singbox.json —— 选了 sing-box 格式却吐 clash 配置是最坑的,
// 客户端的报错根本看不出问题在哪。
func renderPanelSingboxJSON(ctx context.Context, s panelStore, link store.SubscribeLink, proxies []map[string]any) string {
	template := panelTemplateFor(ctx, s, link)
	content := ""
	if template != nil && template.Client == "singbox" && strings.TrimSpace(template.Content) != "" {
		content = template.Content
	} else if builtin, ok := paneldefaults.TemplateByName("default__singbox.json"); ok {
		content = builtin.Content
	}
	outbounds, tags := singboxOutbounds(proxies)
	var root any
	if err := json.Unmarshal([]byte(content), &root); err != nil {
		root = map[string]any{"outbounds": []any{}}
	}
	root = expandSingboxPlaceholders(root, outbounds, tags)
	encoded, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return content
	}
	return string(encoded)
}

// expandSingboxPlaceholders 递归替换模板里的数组占位符:
// "__PROXY_OUTBOUNDS__" → 全部节点出站;"__PROXY_TAGS__" → 节点 tag 列表。
func expandSingboxPlaceholders(node any, outbounds []any, tags []any) any {
	switch value := node.(type) {
	case []any:
		out := make([]any, 0, len(value))
		for _, item := range value {
			if placeholder, ok := item.(string); ok {
				switch placeholder {
				case "__PROXY_OUTBOUNDS__":
					out = append(out, outbounds...)
					continue
				case "__PROXY_TAGS__":
					out = append(out, tags...)
					continue
				}
			}
			out = append(out, expandSingboxPlaceholders(item, outbounds, tags))
		}
		return out
	case map[string]any:
		for key, item := range value {
			value[key] = expandSingboxPlaceholders(item, outbounds, tags)
		}
		return value
	default:
		return node
	}
}

// singboxOutbounds 把 clash 代理映射转成 sing-box 出站,同时返回 tag 列表给
// selector / urltest 组用。转不了的类型直接跳过(宁可少个节点,也不要吐坏配置)。
func singboxOutbounds(proxies []map[string]any) ([]any, []any) {
	outbounds := make([]any, 0, len(proxies)+1)
	tags := make([]any, 0, len(proxies))
	for _, proxy := range proxies {
		outbound, ok := singboxOutbound(proxy)
		if !ok {
			continue
		}
		outbounds = append(outbounds, outbound)
		tags = append(tags, outbound["tag"])
	}
	if len(outbounds) == 0 {
		outbounds = append(outbounds, map[string]any{"type": "direct", "tag": "direct"})
	}
	return outbounds, tags
}

func singboxOutbound(proxy map[string]any) (map[string]any, bool) {
	base := map[string]any{
		"tag":         fmt.Sprint(proxy["name"]),
		"server":      fmt.Sprint(proxy["server"]),
		"server_port": anyInt(proxy["port"]),
	}
	switch strings.ToLower(fmt.Sprint(proxy["type"])) {
	case "ss":
		base["type"] = "shadowsocks"
		base["method"] = proxy["cipher"]
		base["password"] = proxy["password"]
	case "vmess":
		base["type"] = "vmess"
		base["uuid"] = proxy["uuid"]
		base["security"] = "auto"
		base["alter_id"] = anyInt(proxy["alterId"])
		if tls, _ := proxy["tls"].(bool); tls {
			base["tls"] = singboxTLS(proxy)
		}
	case "vless":
		base["type"] = "vless"
		base["uuid"] = proxy["uuid"]
		if flow, ok := proxy["flow"].(string); ok && flow != "" {
			base["flow"] = flow
		}
		base["tls"] = singboxTLS(proxy)
	case "trojan":
		base["type"] = "trojan"
		base["password"] = proxy["password"]
		base["tls"] = singboxTLS(proxy)
	case "hysteria2":
		base["type"] = "hysteria2"
		base["password"] = proxy["password"]
		base["tls"] = singboxTLS(proxy)
	case "tuic":
		base["type"] = "tuic"
		base["uuid"] = proxy["uuid"]
		base["password"] = proxy["password"]
		base["congestion_control"] = "bbr"
		base["udp_relay_mode"] = "native"
		base["tls"] = singboxTLS(proxy)
	case "anytls":
		base["type"] = "anytls"
		base["password"] = proxy["password"]
		base["tls"] = singboxTLS(proxy)
	default:
		return nil, false
	}
	return base, true
}

func singboxTLS(proxy map[string]any) map[string]any {
	tls := map[string]any{"enabled": true}
	for _, key := range []string{"sni", "servername"} {
		if name, ok := proxy[key].(string); ok && name != "" {
			tls["server_name"] = name
			break
		}
	}
	if insecure, ok := proxy["skip-cert-verify"].(bool); ok && insecure {
		tls["insecure"] = true
	}
	if fingerprint, ok := proxy["client-fingerprint"].(string); ok && fingerprint != "" {
		tls["utls"] = map[string]any{"enabled": true, "fingerprint": fingerprint}
	}
	if reality, ok := proxy["reality-opts"].(map[string]any); ok {
		opts := map[string]any{"enabled": true}
		if key, ok := reality["public-key"].(string); ok && key != "" {
			opts["public_key"] = key
		}
		if short, ok := reality["short-id"].(string); ok && short != "" {
			opts["short_id"] = short
		}
		tls["reality"] = opts
	}
	return tls
}

func anyInt(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case string:
		parsed, _ := strconv.Atoi(typed)
		return parsed
	default:
		return 0
	}
}

// panelSeedDefaults 手动把内置模板 / 内置规则分类写进库。
// 启动时其实已经自动播过一次,这里给「升级前就跑着、库里是空的」的老部署一个补入口,
// 不用非得重启一次(重启也会跑)。
func (a *API) panelSeedDefaults(w http.ResponseWriter, r *http.Request) {
	seeder, ok := a.store.(panelbootstrap.Store)
	if !ok {
		writePanelError(w, "当前存储实现不支持播种内置内容")
		return
	}
	result, err := panelbootstrap.Seed(r.Context(), seeder)
	if err != nil {
		writePanelError(w, "导入内置内容失败: "+err.Error())
		return
	}
	writeResponse(w, OK(map[string]any{
		"templatesAdded": result.TemplatesAdded,
		"rulesetsAdded":  result.RulesetsAdded,
		"defaultFixed":   result.DefaultFixed,
	}))
}

// ---------- 用户自己的订阅链接 ----------

// myUserID 取当前登录账号 id;取不到直接 401。
func (a *API) myUserID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	claims, ok := claimsFrom(r)
	if !ok {
		writeResponse(w, Error(401, "请先登录"))
		return 0, false
	}
	id, err := claims.UserID()
	if err != nil {
		writeResponse(w, Error(401, "登录状态异常"))
		return 0, false
	}
	return id, true
}

// myPanelLinkList 只返回自己的订阅链接。
func (a *API) myPanelLinkList(w http.ResponseWriter, r *http.Request) {
	userID, ok := a.myUserID(w, r)
	if !ok {
		return
	}
	list, err := a.store.SubscribeLinks(r.Context())
	if err != nil {
		writePanelError(w, "加载订阅失败: "+err.Error())
		return
	}
	owned := make([]store.SubscribeLink, 0, len(list))
	for _, item := range list {
		if item.UserID == userID {
			owned = append(owned, item)
		}
	}
	writeResponse(w, OK(owned))
}

// myPanelLinkCreate 用户新建订阅:节点固定取自己的,不接受前端传 nodeIds。
func (a *API) myPanelLinkCreate(w http.ResponseWriter, r *http.Request) {
	userID, ok := a.myUserID(w, r)
	if !ok {
		return
	}
	var input store.LinkInput
	if !decodeJSON(w, r, &input) {
		return
	}
	input.UserID = userID
	input.NodeIDs = ""
	if msg := validateLinkInput(&input); msg != "" {
		writePanelError(w, msg)
		return
	}
	item, err := a.store.CreateSubscribeLink(r.Context(), input)
	if err != nil {
		writePanelError(w, "创建订阅失败: "+err.Error())
		return
	}
	writeResponse(w, OK(item))
}

// myPanelLinkOwned 取自己的链接,不是自己的当不存在。
func (a *API) myPanelLinkOwned(w http.ResponseWriter, r *http.Request, id int64, userID int64) (store.SubscribeLink, bool) {
	item, err := a.store.SubscribeLinkByID(r.Context(), id)
	if err != nil || item.UserID != userID {
		writePanelError(w, "订阅不存在")
		return item, false
	}
	return item, true
}

func (a *API) myPanelLinkUpdate(w http.ResponseWriter, r *http.Request) {
	userID, ok := a.myUserID(w, r)
	if !ok {
		return
	}
	var req struct {
		panelIDRequest
		store.LinkInput
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if _, ok := a.myPanelLinkOwned(w, r, req.value(), userID); !ok {
		return
	}
	req.LinkInput.UserID = userID
	req.LinkInput.NodeIDs = ""
	if msg := validateLinkInput(&req.LinkInput); msg != "" {
		writePanelError(w, msg)
		return
	}
	item, err := a.store.UpdateSubscribeLink(r.Context(), req.value(), req.LinkInput)
	if err != nil {
		writePanelError(w, "更新订阅失败: "+err.Error())
		return
	}
	writeResponse(w, OK(item))
}

func (a *API) myPanelLinkDelete(w http.ResponseWriter, r *http.Request) {
	userID, ok := a.myUserID(w, r)
	if !ok {
		return
	}
	var req panelIDRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if _, owned := a.myPanelLinkOwned(w, r, req.value(), userID); !owned {
		return
	}
	if err := a.store.DeleteSubscribeLink(r.Context(), req.value()); err != nil {
		writePanelError(w, "删除失败: "+err.Error())
		return
	}
	writeResponse(w, OK(true))
}

func (a *API) myPanelLinkResetToken(w http.ResponseWriter, r *http.Request) {
	userID, ok := a.myUserID(w, r)
	if !ok {
		return
	}
	var req panelIDRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if _, owned := a.myPanelLinkOwned(w, r, req.value(), userID); !owned {
		return
	}
	token, err := a.store.ResetSubscribeLinkToken(r.Context(), req.value())
	if err != nil {
		writePanelError(w, "重置失败: "+err.Error())
		return
	}
	writeResponse(w, OK(map[string]any{"token": token}))
}

// myPanelOptions 用户能选的模板(只给 visible=1 的)和规则集。
func (a *API) myPanelOptions(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.myUserID(w, r); !ok {
		return
	}
	templates, err := a.store.SubscribeTemplates(r.Context())
	if err != nil {
		writePanelError(w, "加载模板失败: "+err.Error())
		return
	}
	visible := make([]store.SubscribeTemplate, 0, len(templates))
	for _, item := range templates {
		if item.Visible == 1 {
			visible = append(visible, item)
		}
	}
	rulesets, err := a.store.Rulesets(r.Context())
	if err != nil {
		writePanelError(w, "加载规则集失败: "+err.Error())
		return
	}
	// 管理员建的订阅集(没有归属账号、也没有挑节点的那种)
	links, err := a.store.SubscribeLinks(r.Context())
	if err != nil {
		writePanelError(w, "加载订阅集失败: "+err.Error())
		return
	}
	sets := make([]store.SubscribeLink, 0)
	for _, item := range links {
		if item.Shared == 1 {
			sets = append(sets, item)
		}
	}
	writeResponse(w, OK(map[string]any{"templates": visible, "rulesets": rulesets, "sets": sets}))
}

// nodesOutput 按节点 id 造订阅输出(分组订阅和全局订阅集共用)。
func (a *API) nodesOutput(ctx context.Context, ids []int64) (store.SubscriptionOutput, bool) {
	nodes, err := a.store.PanelNodesByIDs(ctx, ids)
	if err != nil || len(nodes) == 0 {
		return store.SubscriptionOutput{}, false
	}
	output := store.SubscriptionOutput{
		Usable:       true,
		Entries:      []store.SubscriptionEntry{},
		CustomLinks:  make([]string, 0, len(nodes)),
		CustomParsed: make([]map[string]any, 0, len(nodes)),
		GroupNodes:   make(map[int64]string, len(nodes)),
	}
	for _, node := range nodes {
		if node.RawLink != "" {
			output.CustomLinks = append(output.CustomLinks, node.RawLink)
		}
		parsed := node.Parsed
		if parsed == nil {
			parsed = map[string]any{}
		}
		if _, ok := parsed["protocol"]; !ok {
			parsed["protocol"] = node.Protocol
		}
		if _, ok := parsed["name"]; !ok || parsed["name"] == "" {
			parsed["name"] = node.Name
		}
		output.CustomParsed = append(output.CustomParsed, parsed)
		output.GroupNodes[node.ID] = fmt.Sprint(parsed["name"])
	}
	return output, true
}

// firstSharedSet 取管理员配的订阅集(没有账号、没有节点的那个链接)。
// 它是「全局订阅规则」:所有用户原来那条订阅链接的 Clash 输出都会自动套用,
// 用户什么都不用选。
func (a *API) firstSharedSet(ctx context.Context) (store.SubscribeLink, bool) {
	links, err := a.store.SubscribeLinks(ctx)
	if err != nil {
		return store.SubscribeLink{}, false
	}
	for _, item := range links {
		if item.Shared == 1 {
			return item, true
		}
	}
	return store.SubscribeLink{}, false
}

// myPanelLinkUseSet 套用管理员建的订阅集:复制它的模板/规则集,绑定到自己账号。
func (a *API) myPanelLinkUseSet(w http.ResponseWriter, r *http.Request) {
	userID, ok := a.myUserID(w, r)
	if !ok {
		return
	}
	var req struct {
		panelIDRequest
		Name string `json:"name"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	set, err := a.store.SubscribeLinkByID(r.Context(), req.value())
	if err != nil || set.Shared != 1 {
		writePanelError(w, "订阅集不存在")
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = set.Name
	}
	item, err := a.store.CreateSubscribeLink(r.Context(), store.LinkInput{
		Name:       name,
		Format:     set.Format,
		TemplateID: set.TemplateID,
		RulesetIDs: set.RulesetIDs,
		UserID:     userID,
	})
	if err != nil {
		writePanelError(w, "创建订阅失败: "+err.Error())
		return
	}
	writeResponse(w, OK(item))
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

// renderGroupClashYAML 分组订阅的输出:每个分组一个策略组,分组里的规则集行
// 把策略改写成组名,于是「日本1 走广告拦截+谷歌+油管」「日本 走电报+苹果」互不干扰。
//
// 分组模式下不用模板的 proxy-groups/rules(模型对不上),但保留模板里的 DNS 等基础设置
// 意义也不大 —— 直接给一份干净骨架,避免两套规则打架。
func renderGroupClashYAML(groups []store.SubscribeGroup, proxies []map[string]any, groupNodes map[int64]string) string {
	names := make([]string, 0, len(proxies))
	for _, proxy := range proxies {
		names = append(names, fmt.Sprint(proxy["name"]))
	}
	used := map[string]int{"节点选择": 1, "DIRECT": 1, "REJECT": 1, "GLOBAL": 1, "自动选择": 1, "漏网之鱼": 1}
	groupDefs := []any{}
	groupNames := []string{}
	ruleLines := []string{}
	for _, group := range groups {
		members := []string{}
		seen := map[string]struct{}{}
		// 关键词分组:按名字把节点收进来(用户自己的线路也在这里)
		for _, keyword := range strings.Split(group.Match, ",") {
			keyword = strings.ToLower(strings.TrimSpace(keyword))
			if keyword == "" {
				continue
			}
			for _, proxy := range proxies {
				name := fmt.Sprint(proxy["name"])
				if !strings.Contains(strings.ToLower(name), keyword) {
					continue
				}
				if _, exists := seen[name]; exists {
					continue
				}
				seen[name] = struct{}{}
				members = append(members, name)
			}
		}
		for _, rawID := range group.NodeIDs {
			id, err := strconv.ParseInt(strings.TrimSpace(rawID), 10, 64)
			if err != nil {
				continue
			}
			name, ok := groupNodes[id]
			if !ok || name == "" {
				continue
			}
			if _, exists := seen[name]; exists {
				continue
			}
			seen[name] = struct{}{}
			members = append(members, name)
		}
		if len(members) == 0 {
			continue
		}
		groupName := uniqueName(strings.TrimSpace(group.Name), used)
		groupDefs = append(groupDefs, map[string]any{"name": groupName, "type": "select", "proxies": members})
		groupNames = append(groupNames, groupName)
		for _, line := range rulesetLinesByIDs(group.RulesetIDs) {
			if rewritten := rewriteRulePolicy(line, groupName); rewritten != "" {
				ruleLines = append(ruleLines, rewritten)
			}
		}
	}
	if len(groupDefs) == 0 {
		return defaultPanelClashYAML(proxies, names)
	}
	selector := append([]string{}, groupNames...)
	selector = append(selector, "DIRECT")
	groupDefs = append(groupDefs, map[string]any{"name": "节点选择", "type": "select", "proxies": selector})

	rules := []string{
		"DOMAIN-SUFFIX,local,DIRECT",
		"IP-CIDR,127.0.0.0/8,DIRECT,no-resolve",
		"IP-CIDR,192.168.0.0/16,DIRECT,no-resolve",
		"IP-CIDR,10.0.0.0/8,DIRECT,no-resolve",
		"IP-CIDR,172.16.0.0/12,DIRECT,no-resolve",
	}
	rules = append(rules, ruleLines...)
	rules = append(rules, "MATCH,节点选择")

	root := map[string]any{
		"mixed-port": 7890, "allow-lan": false, "mode": "rule", "log-level": "info",
		"dns": map[string]any{
			"enable": true, "ipv6": false, "enhanced-mode": "fake-ip", "fake-ip-range": "198.18.0.1/16",
			"nameserver": []string{"223.5.5.5", "119.29.29.29"},
		},
		"proxies":      proxies,
		"proxy-groups": groupDefs,
		"rules":        rules,
	}
	encoded, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return defaultPanelClashYAML(proxies, names)
	}
	return string(encoded)
}

// rulesetLinesByIDs 把一组规则集 id 展平成规则行。
func rulesetLinesByIDs(ids []string) []string {
	lines := []string{}
	for _, rawID := range ids {
		id, err := strconv.ParseInt(strings.TrimSpace(rawID), 10, 64)
		if err != nil {
			continue
		}
		_, content, err := panelStoreRulesetContent(id)
		if err != nil || content == "" {
			continue
		}
		for _, line := range strings.Split(content, "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
				continue
			}
			lines = append(lines, line)
		}
	}
	return lines
}

// rewriteRulePolicy 把规则行的策略改成 groupName;REJECT / DIRECT 这类内置策略保持原样。
func rewriteRulePolicy(line, groupName string) string {
	parts := strings.Split(line, ",")
	if len(parts) < 3 {
		return ""
	}
	policy := strings.TrimSpace(parts[2])
	switch strings.ToUpper(policy) {
	case "REJECT", "REJECT-DROP", "DIRECT", "PASS", "COMPATIBLE":
		return line
	}
	parts[2] = groupName
	return strings.Join(parts, ",")
}

// syncPanelNodes 把「管理员账号名下的线路」一键同步成公共节点:
// 协议管理 / 中转 的线路是每用户凭证,不能直接当公共节点;但这个面板的使用模型是
// 所有车友公平共用同一批线路,所以拿管理员订阅里的线路生成分享链接写入节点管理,
// 之后它们就和手动导入的节点一样,可以在订阅链接里随便分组。
// 幂等:同名节点会更新链接,不会越点越多。
func (a *API) syncPanelNodes(w http.ResponseWriter, r *http.Request) {
	users, err := a.store.Users(r.Context())
	if err != nil {
		writePanelError(w, "读取用户失败: "+err.Error())
		return
	}
	adminID := int64(1)
	for _, user := range users {
		if user.RoleID == 0 {
			adminID = user.ID
			break
		}
	}
	output, err := a.store.SubscriptionByUserID(r.Context(), adminID)
	if err != nil {
		writePanelError(w, "读取管理员线路失败: "+err.Error())
		return
	}
	links := buildSubscriptionLinks(output)
	if len(links) == 0 {
		writePanelError(w, "管理员账号下没有可同步的线路,先在协议管理/中转里建好并分配")
		return
	}
	existing, err := a.store.QueryMaps(r.Context(), "SELECT id,name,raw_link FROM custom_node")
	if err != nil {
		writePanelError(w, "读取现有节点失败: "+err.Error())
		return
	}
	idByName := map[string]int64{}
	linkByName := map[string]string{}
	for _, row := range existing {
		name := fmt.Sprint(row["name"])
		idByName[name] = toInt64(row["id"])
		linkByName[name] = fmt.Sprint(row["raw_link"])
	}
	now := time.Now().UnixMilli()
	created, updated, failed := 0, 0, 0
	failMsg := ""
	for _, link := range links {
		parsed, err := sharelink.Parse(link)
		if err != nil {
			failed++
			failMsg = err.Error()
			continue
		}
		name := parsed.Name
		if name == "" {
			name = "面板节点 " + strings.ToUpper(parsed.Protocol)
		}
		encoded, _ := json.Marshal(parsed.Values)
		if id, exists := idByName[name]; exists {
			if linkByName[name] == parsed.Raw {
				continue // 内容没变,跳过
			}
			if err := a.store.UpdateMap(r.Context(), "custom_node", id, map[string]any{"raw_link": parsed.Raw, "parsed_json": string(encoded), "updated_time": now}); err != nil {
				failed++
				failMsg = err.Error()
				continue
			}
			updated++
			continue
		}
		if _, err := a.store.InsertMap(r.Context(), "custom_node", map[string]any{"name": name, "protocol": parsed.Protocol, "raw_link": parsed.Raw, "parsed_json": string(encoded), "visibility": "global", "status": 1, "created_time": now, "updated_time": now}); err != nil {
			failed++
			failMsg = err.Error()
			continue
		}
		created++
	}
	result := map[string]any{"created": created, "updated": updated, "total": len(links)}
	if failed > 0 {
		result["failed"] = failed
		result["error"] = failMsg
	}
	writeResponse(w, OK(result))
}

func toInt64(value any) int64 {
	switch v := value.(type) {
	case int64:
		return v
	case int:
		return int64(v)
	case float64:
		return int64(v)
	}
	return 0
}
