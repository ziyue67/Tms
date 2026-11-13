package httpapi

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ziyue67/tms/go-backend/internal/paneldefaults"
	"github.com/ziyue67/tms/go-backend/internal/store"
)

// 文本模板(YAML)注入:proxies: null 换成真实节点、__PROXY_NODES__ 展开成节点名、
// __PROXY_PROVIDERS__ 删掉、规则集插到 MATCH 之前。
func TestRenderClashTextTemplateInjectsNodesAndRules(t *testing.T) {
	template := strings.Join([]string{
		"mode: rule",
		"proxies: null",
		"proxy-groups:",
		"  - name: 🚀 手动选择",
		"    type: select",
		"    proxies:",
		"      - ♻️ 自动选择",
		"      - __PROXY_PROVIDERS__",
		"      - __PROXY_NODES__",
		"rules:",
		"  - GEOSITE,cn,DIRECT",
		"  - MATCH,🚀 手动选择",
	}, "\n")
	proxies := []map[string]any{
		{"name": "香港 01", "type": "ss", "server": "1.2.3.4", "port": 443},
		{"name": "日本 02", "type": "trojan", "server": "5.6.7.8", "port": 8443},
	}
	names := []string{"香港 01", "日本 02"}

	out := renderClashTextTemplate(template, proxies, names, []string{"DOMAIN-SUFFIX,google.com,🚀 手动选择"})

	if strings.Contains(out, "__PROXY") {
		t.Fatalf("占位符没有替换干净:\n%s", out)
	}
	if strings.Count(out, "香港 01") != 2 || strings.Count(out, "日本 02") != 2 {
		t.Fatalf("节点没有同时出现在 proxies 与策略组里:\n%s", out)
	}
	ruleIndex := strings.Index(out, "DOMAIN-SUFFIX,google.com")
	matchIndex := strings.Index(out, "MATCH,🚀 手动选择")
	if ruleIndex < 0 || matchIndex < 0 || ruleIndex > matchIndex {
		t.Fatalf("规则集行必须插在 MATCH 之前:\n%s", out)
	}
	if !strings.Contains(out, `proxies:`) || strings.Contains(out, "proxies: null") {
		t.Fatalf("顶層 proxies 没被替换:\n%s", out)
	}
}

// 没有 proxies 键的极简模板也要能出配置 —— 直接补一段在末尾。
func TestRenderClashTextTemplateAppendsProxiesWhenMissing(t *testing.T) {
	out := renderClashTextTemplate("mode: rule\nrules:\n  - MATCH,DIRECT\n", []map[string]any{{"name": "A", "type": "ss"}}, []string{"A"}, nil)
	if !strings.Contains(out, "proxies:\n  - {") {
		t.Fatalf("缺少 proxies 段时没有补齐:\n%s", out)
	}
}

// sing-box 模板的数组占位符要展开成真实出站与 tag。
func TestExpandSingboxPlaceholders(t *testing.T) {
	root := map[string]any{
		"outbounds": []any{
			"__PROXY_OUTBOUNDS__",
			map[string]any{"type": "selector", "outbounds": []any{"自动选择", "__PROXY_TAGS__", "direct"}},
		},
	}
	outbounds := []any{map[string]any{"type": "shadowsocks", "tag": "香港 01"}}
	tags := []any{"香港 01"}

	expanded := expandSingboxPlaceholders(root, outbounds, tags)
	encoded, err := json.Marshal(expanded)
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	if strings.Contains(text, "__PROXY") {
		t.Fatalf("占位符没有展开:%s", text)
	}
	for _, want := range []string{"shadowsocks", "香港 01", "selector", "direct"} {
		if !strings.Contains(text, want) {
			t.Fatalf("输出缺少 %q:%s", want, text)
		}
	}
}

// clash 代理 → sing-box 出站:reality / sni / skip-cert-verify 要跟着走。
func TestSingboxOutboundKeepsRealityOptions(t *testing.T) {
	proxy := map[string]any{
		"name": "香港 01", "type": "vless", "server": "1.2.3.4", "port": 443,
		"uuid": "uuid-1", "flow": "xtls-rprx-vision", "servername": "example.com",
		"client-fingerprint": "chrome",
		"reality-opts":       map[string]any{"public-key": "pk", "short-id": "ab"},
	}
	outbound, ok := singboxOutbound(proxy)
	if !ok {
		t.Fatal("vless 应该能转成 sing-box 出站")
	}
	tls, _ := outbound["tls"].(map[string]any)
	if tls["server_name"] != "example.com" {
		t.Fatalf("server_name 丢了:%v", tls)
	}
	reality, _ := tls["reality"].(map[string]any)
	if reality["public_key"] != "pk" || reality["short_id"] != "ab" {
		t.Fatalf("reality 参数丢了:%v", reality)
	}
	if outbound["flow"] != "xtls-rprx-vision" {
		t.Fatalf("flow 丢了:%v", outbound)
	}
}

// 内置的 fake_ip 模板必须能被文本渲染器吃下去:占位符全清、节点落位、规则集插在 MATCH 前。
func TestBuiltinClashTemplateRendersCleanly(t *testing.T) {
	var builtin string
	for _, tpl := range paneldefaults.Templates() {
		if tpl.Name == "fake_ip__v3.yaml" {
			builtin = tpl.Content
		}
	}
	if builtin == "" {
		t.Fatal("内置模板 fake_ip__v3.yaml 缺失")
	}
	proxies := []map[string]any{
		{"name": "香港 01", "type": "ss", "server": "1.2.3.4", "port": 443},
		{"name": "日本 02", "type": "trojan", "server": "5.6.7.8", "port": 8443},
	}
	out := renderClashTextTemplate(builtin, proxies, []string{"香港 01", "日本 02"}, []string{"DOMAIN-SUFFIX,ads.example,REJECT"})

	for _, placeholder := range []string{"__PROXY_NODES__", "__PROXY_PROVIDERS__", "proxies: null"} {
		if strings.Contains(out, placeholder) {
			t.Fatalf("占位符 %s 没清掉", placeholder)
		}
	}
	if !strings.Contains(out, "DOMAIN-SUFFIX,ads.example,REJECT") {
		t.Fatal("规则集没有注入")
	}
	if strings.Index(out, "DOMAIN-SUFFIX,ads.example") > strings.Index(out, "MATCH,") {
		t.Fatal("规则集必须在 MATCH 之前")
	}
	if !strings.Contains(out, "rule-providers:") {
		t.Fatal("模板自带的 rule-providers 段必须原样保留")
	}
}

// 订阅链接带 node_ids 时,输出直接来自节点,不再去查车友 token。
func TestPanelOutputUsesLinkNodes(t *testing.T) {
	fake := &fakeStore{panelNodes: []store.PanelNode{
		{ID: 7001, Name: "香港 01", Protocol: "shadowsocks", RawLink: "ss://abc", Parsed: map[string]any{"server": "1.2.3.4", "port": 443, "method": "aes-256-gcm", "password": "pw"}},
		{ID: 7002, Name: "日本 02", Protocol: "trojan", RawLink: "trojan://xyz", Parsed: map[string]any{"server": "5.6.7.8", "port": 8443, "password": "pw2", "sni": "example.com"}},
	}}
	api := &API{store: fake}
	output, ok := api.panelOutputFor(context.Background(), store.SubscribeLink{ID: 1, Token: "whatever", NodeIDs: "7001,7002"})
	if !ok {
		t.Fatal("带 node_ids 的链接应该能出输出")
	}
	if len(output.CustomParsed) != 2 || len(output.CustomLinks) != 2 {
		t.Fatalf("节点数不对: parsed=%d links=%d", len(output.CustomParsed), len(output.CustomLinks))
	}
	if output.CustomParsed[0]["protocol"] != "shadowsocks" {
		t.Fatalf("protocol 没有补进解析结果: %v", output.CustomParsed[0])
	}
	if output.UserID != 0 {
		t.Fatal("节点模式不应该依赖车友账号")
	}

	proxies := buildClashProxies(output)
	if len(proxies) != 2 {
		t.Fatalf("clash 代理数不对: %d", len(proxies))
	}
	if proxies[0]["type"] != "ss" || proxies[0]["cipher"] != "aes-256-gcm" {
		t.Fatalf("shadowsocks 转换不对: %v", proxies[0])
	}
	if proxies[1]["type"] != "trojan" || proxies[1]["sni"] != "example.com" {
		t.Fatalf("trojan 转换不对: %v", proxies[1])
	}
}

// 没有 node_ids 的老链接不能崩,只是走旧路径(查不到车友就返回不可用)。
func TestPanelOutputFallsBackForLegacyLink(t *testing.T) {
	fake := &fakeStore{}
	api := &API{store: fake}
	if _, ok := api.panelOutputFor(context.Background(), store.SubscribeLink{Token: "no-such-token"}); ok {
		t.Fatal("查不到车友的老链接应该返回不可用")
	}
}

// 前端为避免 64 位精度丢失会把 id 当字符串传,两种写法都要能解。
func TestPanelIDAcceptsStringAndNumber(t *testing.T) {
	for _, body := range []string{`{"id":"12"}`, `{"id":12}`} {
		var req panelIDRequest
		if err := json.Unmarshal([]byte(body), &req); err != nil {
			t.Fatalf("%s 解析失败: %v", body, err)
		}
		if req.value() != 12 {
			t.Fatalf("%s 解析成 %d,应为 12", body, req.value())
		}
	}
	var req panelIDRequest
	if err := json.Unmarshal([]byte(`{"id":"abc"}`), &req); err == nil {
		t.Fatal("非法 id 应该报错")
	}
	if err := json.Unmarshal([]byte(`{}`), &req); err != nil || req.value() != 0 {
		t.Fatal("缺 id 应按 0 处理")
	}
}
