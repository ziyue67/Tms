package httpapi

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/ziyue67/tms/go-backend/internal/auth"
	"github.com/ziyue67/tms/go-backend/internal/store"
)

func (a *API) openSubscription(w http.ResponseWriter, r *http.Request) {
	output, err := a.store.SubscriptionByToken(r.Context(), r.URL.Query().Get("token"))
	if err != nil {
		http.Error(w, "", http.StatusInternalServerError)
		return
	}
	prepareSubscriptionHeaders(w, output)
	links := buildSubscriptionLinks(output)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(base64.StdEncoding.EncodeToString([]byte(strings.Join(links, "\n")))))
}

func (a *API) openClashSubscription(w http.ResponseWriter, r *http.Request) {
	output, err := a.store.SubscriptionByToken(r.Context(), r.URL.Query().Get("token"))
	if err != nil {
		http.Error(w, "", http.StatusInternalServerError)
		return
	}
	prepareSubscriptionHeaders(w, output)
	proxies := buildClashProxies(output)
	root := map[string]any{"mixed-port": 7890, "allow-lan": false, "mode": "rule", "log-level": "info", "external-controller": "127.0.0.1:9090",
		"dns":     map[string]any{"enable": true, "ipv6": false, "enhanced-mode": "fake-ip", "fake-ip-range": "198.18.0.1/16", "nameserver": []string{"223.5.5.5", "119.29.29.29"}, "fallback": []string{"8.8.8.8", "1.1.1.1"}},
		"proxies": proxies, "rules": []string{"DOMAIN-SUFFIX,local,DIRECT", "IP-CIDR,127.0.0.0/8,DIRECT,no-resolve", "IP-CIDR,192.168.0.0/16,DIRECT,no-resolve", "IP-CIDR,10.0.0.0/8,DIRECT,no-resolve", "IP-CIDR,172.16.0.0/12,DIRECT,no-resolve", "GEOIP,CN,DIRECT", "MATCH,漏网之鱼"}}
	if len(proxies) == 0 {
		root["proxy-groups"] = []any{}
	} else {
		names := make([]string, 0, len(proxies))
		for _, proxy := range proxies {
			names = append(names, fmt.Sprint(proxy["name"]))
		}
		root["proxy-groups"] = []map[string]any{{"name": "节点选择", "type": "select", "proxies": append([]string{"自动选择"}, names...)},
			{"name": "自动选择", "type": "url-test", "proxies": names, "url": "http://www.gstatic.com/generate_204", "interval": 300, "tolerance": 50},
			{"name": "漏网之鱼", "type": "select", "proxies": []string{"节点选择", "DIRECT"}}}
	}
	// JSON is also a valid YAML 1.2 document and is accepted by Mihomo/Clash.
	encoded, _ := json.MarshalIndent(root, "", "  ")
	w.Header().Set("Content-Type", "text/yaml; charset=utf-8")
	_, _ = w.Write(encoded)
}

func (a *API) openSubscriptionStore(w http.ResponseWriter, r *http.Request) {
	username, password := r.URL.Query().Get("user"), r.URL.Query().Get("pwd")
	if username == "" {
		writeResponse(w, Failure("用户不能为空"))
		return
	}
	if password == "" {
		writeResponse(w, Failure("密码不能为空"))
		return
	}
	var tunnelID *int64
	if tunnel := r.URL.Query().Get("tunnel"); tunnel != "" && tunnel != "-1" {
		parsed, err := strconv.ParseInt(tunnel, 10, 64)
		if err != nil {
			writeResponse(w, Failure("隧道不存在"))
			return
		}
		tunnelID = &parsed
	}
	user, permission, err := a.store.SubscriptionStoreHeader(r.Context(), username, tunnelID)
	if err != nil {
		writeResponse(w, Failure("鉴权失败"))
		return
	}
	valid, upgrade := auth.VerifyPassword(user.Password, password)
	if !valid {
		writeResponse(w, Failure("鉴权失败"))
		return
	}
	if upgrade {
		if encoded, hashErr := auth.HashPassword(password); hashErr == nil {
			_ = a.store.UpdatePassword(r.Context(), user.ID, encoded)
		}
	}
	upload, download, total, expire := user.OutboundFlow, user.InboundFlow, user.Flow*1024*1024*1024, user.ExpiryTime/1000
	if permission != nil {
		upload, download, total, expire = permission.OutboundFlow, permission.InboundFlow, permission.Flow*1024*1024*1024, permission.ExpiryTime/1000
	}
	header := fmt.Sprintf("upload=%d; download=%d; total=%d; expire=%d", download, upload, total, expire)
	w.Header().Set("subscription-userinfo", header)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(header))
}

func buildSubscriptionLinks(output store.SubscriptionOutput) []string {
	links := make([]string, 0, len(output.Entries)+len(output.CustomLinks))
	used := map[string]int{}
	for _, entry := range output.Entries {
		remark := entry.Remark
		if remark == "" {
			remark = protocolName(entry.Protocol)
		}
		if output.Aggregate {
			prefix := entry.NodeName
			if entry.LandingName != "" {
				prefix += "→" + entry.LandingName
			}
			remark = prefix + " " + remark
		}
		remark = uniqueName(remark, used)
		links = append(links, clientLink(entry, remark))
	}
	links = append(links, output.CustomLinks...)
	return links
}

func clientLink(entry store.SubscriptionEntry, remark string) string {
	fragment := url.QueryEscape(remark)
	server := entry.Server
	if strings.Contains(server, ":") && !strings.HasPrefix(server, "[") {
		server = "[" + server + "]"
	}
	address := server + ":" + strconv.Itoa(entry.Port)
	switch entry.Protocol {
	case "shadowsocks":
		config := map[string]any{}
		_ = json.Unmarshal([]byte(entry.ConfigJSON), &config)
		method, _ := config["method"].(string)
		password, _ := config["password"].(string)
		credential := base64.RawURLEncoding.EncodeToString([]byte(method + ":" + password))
		return "ss://" + credential + "@" + address + "#" + fragment
	case "vmess":
		value := map[string]string{"v": "2", "ps": remark, "add": entry.Server, "port": strconv.Itoa(entry.Port), "id": entry.UUID, "aid": "0", "scy": "auto", "net": "tcp", "type": "none", "host": "", "path": "", "tls": "", "sni": ""}
		encoded, _ := json.Marshal(value)
		return "vmess://" + base64.StdEncoding.EncodeToString(encoded)
	case "trojan":
		return "trojan://" + url.QueryEscape(entry.Password) + "@" + address + "?security=reality&sni=" + url.QueryEscape(entry.SNI) + "&fp=chrome&pbk=" + url.QueryEscape(entry.PublicKey) + "&sid=" + url.QueryEscape(entry.ShortID) + "&type=tcp#" + fragment
	case "hysteria2":
		return "hysteria2://" + url.QueryEscape(entry.Password) + "@" + address + "?sni=" + url.QueryEscape(entry.SNI) + "&insecure=1#" + fragment
	case "tuic":
		return "tuic://" + url.QueryEscape(entry.UUID) + ":" + url.QueryEscape(entry.Password) + "@" + address + "?congestion_control=bbr&alpn=h3&sni=" + url.QueryEscape(entry.SNI) + "&allow_insecure=1#" + fragment
	case "anytls":
		return "anytls://" + url.QueryEscape(entry.Password) + "@" + address + "?insecure=1&sni=" + url.QueryEscape(entry.SNI) + "#" + fragment
	default:
		return "vless://" + url.QueryEscape(entry.UUID) + "@" + address + "?encryption=none&flow=xtls-rprx-vision&security=reality&sni=" + url.QueryEscape(entry.SNI) + "&fp=chrome&pbk=" + url.QueryEscape(entry.PublicKey) + "&sid=" + url.QueryEscape(entry.ShortID) + "&type=tcp#" + fragment
	}
}

func buildClashProxies(output store.SubscriptionOutput) []map[string]any {
	result := []map[string]any{}
	used := map[string]int{}
	for _, entry := range output.Entries {
		name := entry.Remark
		if name == "" {
			name = protocolName(entry.Protocol)
		}
		if output.Aggregate {
			name = entry.NodeName + map[bool]string{true: "→" + entry.LandingName, false: ""}[entry.LandingName != ""] + " " + name
		}
		name = uniqueName(name, used)
		proxy := map[string]any{"name": name, "server": entry.Server, "port": entry.Port, "udp": true}
		switch entry.Protocol {
		case "vless":
			proxy["type"], proxy["uuid"], proxy["network"], proxy["tls"], proxy["flow"], proxy["servername"], proxy["client-fingerprint"], proxy["reality-opts"] = "vless", entry.UUID, "tcp", true, "xtls-rprx-vision", entry.SNI, "chrome", map[string]any{"public-key": entry.PublicKey, "short-id": entry.ShortID}
		case "trojan":
			proxy["type"], proxy["password"], proxy["sni"], proxy["client-fingerprint"], proxy["reality-opts"] = "trojan", entry.Password, entry.SNI, "chrome", map[string]any{"public-key": entry.PublicKey, "short-id": entry.ShortID}
		case "vmess":
			proxy["type"], proxy["uuid"], proxy["alterId"], proxy["cipher"], proxy["network"] = "vmess", entry.UUID, 0, "auto", "tcp"
		case "shadowsocks":
			var config map[string]any
			_ = json.Unmarshal([]byte(entry.ConfigJSON), &config)
			proxy["type"], proxy["cipher"], proxy["password"] = "ss", config["method"], config["password"]
		case "hysteria2":
			proxy["type"], proxy["password"], proxy["sni"], proxy["skip-cert-verify"] = "hysteria2", entry.Password, entry.SNI, true
		case "tuic":
			proxy["type"], proxy["uuid"], proxy["password"], proxy["sni"], proxy["alpn"], proxy["congestion-controller"], proxy["udp-relay-mode"], proxy["skip-cert-verify"] = "tuic", entry.UUID, entry.Password, entry.SNI, []string{"h3"}, "bbr", "native", true
		default:
			continue
		}
		result = append(result, proxy)
	}
	return result
}

func prepareSubscriptionHeaders(w http.ResponseWriter, output store.SubscriptionOutput) {
	w.Header().Set("subscription-userinfo", fmt.Sprintf("upload=%d; download=%d; total=%d; expire=%d", output.Upload, output.Download, output.Total, output.Expires))
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
}
func protocolName(value string) string {
	names := map[string]string{"shadowsocks": "Shadowsocks", "vmess": "VMess", "trojan": "Trojan", "hysteria2": "Hysteria2", "tuic": "TUIC", "anytls": "AnyTLS"}
	if name := names[value]; name != "" {
		return name
	}
	return "VLESS"
}
func uniqueName(value string, used map[string]int) string {
	used[value]++
	if used[value] == 1 {
		return value
	}
	return fmt.Sprintf("%s %d", value, used[value])
}
