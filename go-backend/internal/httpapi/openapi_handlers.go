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
	output, ok := a.loadSubscription(w, r)
	if !ok {
		return
	}
	links := buildSubscriptionLinks(output)
	if len(links) == 0 {
		http.Error(w, "No available subscription nodes", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(base64.StdEncoding.EncodeToString([]byte(strings.Join(links, "\n")))))
}

func (a *API) openClashSubscription(w http.ResponseWriter, r *http.Request) {
	output, ok := a.loadSubscription(w, r)
	if !ok {
		return
	}
	proxies := buildClashProxies(output)
	if len(proxies) == 0 {
		http.Error(w, "No available subscription nodes", http.StatusNotFound)
		return
	}
	root := map[string]any{"mixed-port": 7890, "allow-lan": false, "mode": "rule", "log-level": "info", "external-controller": "127.0.0.1:9090",
		"dns":     map[string]any{"enable": true, "ipv6": false, "enhanced-mode": "fake-ip", "fake-ip-range": "198.18.0.1/16", "nameserver": []string{"223.5.5.5", "119.29.29.29"}, "fallback": []string{"8.8.8.8", "1.1.1.1"}},
		"proxies": proxies, "rules": []string{"DOMAIN-SUFFIX,local,DIRECT", "IP-CIDR,127.0.0.0/8,DIRECT,no-resolve", "IP-CIDR,192.168.0.0/16,DIRECT,no-resolve", "IP-CIDR,10.0.0.0/8,DIRECT,no-resolve", "IP-CIDR,172.16.0.0/12,DIRECT,no-resolve", "GEOIP,CN,DIRECT", "MATCH,漏网之鱼"}}
	names := make([]string, 0, len(proxies))
	for _, proxy := range proxies {
		names = append(names, fmt.Sprint(proxy["name"]))
	}
	root["proxy-groups"] = []map[string]any{{"name": "节点选择", "type": "select", "proxies": append([]string{"自动选择"}, names...)},
		{"name": "自动选择", "type": "url-test", "proxies": names, "url": "http://www.gstatic.com/generate_204", "interval": 300, "tolerance": 50},
		{"name": "漏网之鱼", "type": "select", "proxies": []string{"节点选择", "DIRECT"}}}
	// JSON is also a valid YAML 1.2 document and is accepted by Mihomo/Clash.
	encoded, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		http.Error(w, "Unable to encode subscription", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/yaml; charset=utf-8")
	_, _ = w.Write(encoded)
}

func (api *API) loadSubscription(response http.ResponseWriter, request *http.Request) (store.SubscriptionOutput, bool) {
	prepareSubscriptionHeaders(response, store.SubscriptionOutput{})
	token := strings.TrimSpace(request.URL.Query().Get("token"))
	if token == "" {
		http.Error(response, "Subscription token is required", http.StatusUnauthorized)
		return store.SubscriptionOutput{}, false
	}
	output, err := api.store.SubscriptionByToken(request.Context(), token)
	if err != nil {
		http.Error(response, "Unable to load subscription", http.StatusInternalServerError)
		return store.SubscriptionOutput{}, false
	}
	prepareSubscriptionHeaders(response, output)
	if output.UserID == 0 {
		http.Error(response, "Invalid subscription token", http.StatusUnauthorized)
		return output, false
	}
	if !output.Usable {
		http.Error(response, "Subscription is disabled, expired, or over quota", http.StatusForbidden)
		return output, false
	}
	return output, true
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
	fragment := url.PathEscape(remark)
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
		return "trojan://" + url.User(entry.Password).String() + "@" + address + "?security=reality&sni=" + url.QueryEscape(entry.SNI) + "&fp=chrome&pbk=" + url.QueryEscape(entry.PublicKey) + "&sid=" + url.QueryEscape(entry.ShortID) + "&type=tcp#" + fragment
	case "hysteria2":
		var hysteriaConfig map[string]any
		_ = json.Unmarshal([]byte(entry.ConfigJSON), &hysteriaConfig)
		query := "?sni=" + url.QueryEscape(entry.SNI) + "&insecure=1"
		if obfs := strings.ToLower(mapString(hysteriaConfig, "obfs")); obfs != "" && obfs != "none" {
			if secret := firstString(mapRawString(hysteriaConfig, "obfs-password"), mapRawString(hysteriaConfig, "obfs_password")); secret != "" {
				query += "&obfs=" + url.QueryEscape(obfs) + "&obfs-password=" + url.QueryEscape(secret)
			}
		}
		if alpn := splitALPN(firstString(mapString(hysteriaConfig, "alpn"), mapString(hysteriaConfig, "alpns"))); len(alpn) > 0 {
			query += "&alpn=" + url.QueryEscape(strings.Join(alpn, ","))
		}
		return "hysteria2://" + url.User(entry.Password).String() + "@" + address + query + "#" + fragment
	case "tuic":
		return "tuic://" + url.UserPassword(entry.UUID, entry.Password).String() + "@" + address + "?congestion_control=bbr&alpn=h3&sni=" + url.QueryEscape(entry.SNI) + "&allow_insecure=1#" + fragment
	case "anytls":
		return "anytls://" + url.User(entry.Password).String() + "@" + address + "?insecure=1&sni=" + url.QueryEscape(entry.SNI) + "#" + fragment
	default:
		return "vless://" + url.User(entry.UUID).String() + "@" + address + "?encryption=none&flow=xtls-rprx-vision&security=reality&sni=" + url.QueryEscape(entry.SNI) + "&fp=chrome&pbk=" + url.QueryEscape(entry.PublicKey) + "&sid=" + url.QueryEscape(entry.ShortID) + "&type=tcp#" + fragment
	}
}

func buildClashProxies(output store.SubscriptionOutput) []map[string]any {
	result := []map[string]any{}
	used := map[string]int{"节点选择": 1, "自动选择": 1, "漏网之鱼": 1, "DIRECT": 1, "REJECT": 1, "GLOBAL": 1}
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
			var config map[string]any
			_ = json.Unmarshal([]byte(entry.ConfigJSON), &config)
			addCustomHysteria2Options(proxy, config)
		case "tuic":
			proxy["type"], proxy["uuid"], proxy["password"], proxy["sni"], proxy["alpn"], proxy["congestion-controller"], proxy["udp-relay-mode"], proxy["skip-cert-verify"] = "tuic", entry.UUID, entry.Password, entry.SNI, []string{"h3"}, "bbr", "native", true
		case "anytls":
			proxy["type"], proxy["password"], proxy["sni"], proxy["skip-cert-verify"] = "anytls", entry.Password, entry.SNI, true
		default:
			continue
		}
		result = append(result, proxy)
	}
	for _, custom := range output.CustomParsed {
		proxy, ok := customClashProxy(custom, used)
		if ok {
			result = append(result, proxy)
		}
	}
	return result
}

func customClashProxy(value map[string]any, used map[string]int) (map[string]any, bool) {
	protocol := strings.ToLower(mapString(value, "protocol"))
	server := mapString(value, "server")
	port := mapInt(value, "port")
	if server == "" || port < 1 || port > 65535 {
		return nil, false
	}
	name := mapString(value, "name")
	if name == "" {
		name = protocolName(protocol)
	}
	proxy := map[string]any{"name": uniqueName(name, used), "server": server, "port": port, "udp": true}
	sni := firstString(mapString(value, "sni"), mapString(value, "peer"), server)
	switch protocol {
	case "vless":
		proxy["type"], proxy["uuid"], proxy["network"] = "vless", mapString(value, "uuid"), firstString(mapString(value, "type"), mapString(value, "net"), "tcp")
		security := strings.ToLower(firstString(mapString(value, "security"), mapString(value, "tls")))
		if security != "" && security != "none" {
			proxy["tls"], proxy["servername"] = true, sni
		}
		if security == "reality" {
			proxy["client-fingerprint"] = firstString(mapString(value, "fp"), "chrome")
			proxy["reality-opts"] = map[string]any{"public-key": mapString(value, "pbk"), "short-id": mapString(value, "sid")}
		}
		// A server provisioned with `flow=xtls-rprx-vision` rejects clients that
		// do not ask for the same flow, so it has to survive the conversion.
		if flow := mapString(value, "flow"); flow != "" && flow != "none" {
			proxy["flow"] = flow
		}
		addCustomClashTransport(proxy, value)
	case "trojan":
		proxy["type"], proxy["password"], proxy["sni"] = "trojan", mapRawString(value, "password"), sni
		if strings.EqualFold(mapString(value, "security"), "reality") {
			proxy["client-fingerprint"] = firstString(mapString(value, "fp"), "chrome")
			proxy["reality-opts"] = map[string]any{"public-key": mapString(value, "pbk"), "short-id": mapString(value, "sid")}
		}
		addCustomClashTransport(proxy, value)
	case "vmess":
		proxy["type"], proxy["uuid"], proxy["alterId"], proxy["cipher"] = "vmess", mapString(value, "uuid"), mapInt(value, "aid"), firstString(mapString(value, "scy"), "auto")
		proxy["network"] = firstString(mapString(value, "net"), mapString(value, "type"), "tcp")
		if tls := strings.ToLower(mapString(value, "tls")); tls != "" && tls != "none" {
			proxy["tls"], proxy["servername"] = true, sni
		}
		addCustomClashTransport(proxy, value)
	case "shadowsocks":
		proxy["type"], proxy["cipher"], proxy["password"] = "ss", mapString(value, "method"), mapRawString(value, "password")
	case "hysteria2":
		proxy["type"], proxy["password"], proxy["sni"], proxy["skip-cert-verify"] = "hysteria2", mapRawString(value, "password"), sni, true
		addCustomHysteria2Options(proxy, value)
	case "tuic":
		proxy["type"], proxy["uuid"], proxy["password"], proxy["sni"] = "tuic", mapString(value, "uuid"), mapRawString(value, "password"), sni
		proxy["alpn"], proxy["congestion-controller"], proxy["udp-relay-mode"], proxy["skip-cert-verify"] = []string{"h3"}, firstString(mapString(value, "congestion_control"), "bbr"), "native", true
	case "anytls":
		proxy["type"], proxy["password"], proxy["sni"], proxy["skip-cert-verify"] = "anytls", mapRawString(value, "password"), sni, true
	default:
		return nil, false
	}
	return proxy, true
}

func addCustomClashTransport(proxy, value map[string]any) {
	network := strings.ToLower(firstString(mapString(proxy, "network"), mapString(value, "type"), mapString(value, "net")))
	path := firstString(mapString(value, "path"), mapString(value, "serviceName"))
	switch network {
	case "ws":
		proxy["network"] = network
		options := map[string]any{"path": path}
		if host := mapString(value, "host"); host != "" {
			options["headers"] = map[string]string{"Host": host}
		}
		proxy["ws-opts"] = options
	case "grpc":
		proxy["network"] = network
		proxy["grpc-opts"] = map[string]any{"grpc-service-name": path}
	}
}

// addCustomHysteria2Options copies the Hysteria2 transport options that a
// share link may carry. Mihomo rejects a node whose `obfs` is set without a
// matching `obfs-password` (and vice versa), so the two are only emitted
// together; a node that asks for salamander obfuscation but loses the secret
// on the way out shows up as a red/failed node in every Clash client.
func addCustomHysteria2Options(proxy, value map[string]any) {
	obfs := strings.ToLower(mapString(value, "obfs"))
	obfsPassword := firstString(mapRawString(value, "obfs-password"), mapRawString(value, "obfs_password"))
	switch obfs {
	case "", "none":
		// No obfuscation requested: omit both keys.
	case "salamander":
		if obfsPassword != "" {
			proxy["obfs"] = obfs
			proxy["obfs-password"] = obfsPassword
		}
	default:
		// Unknown schemes are forwarded only when complete, so a malformed
		// link degrades to a plain node instead of an unusable one.
		if obfsPassword != "" {
			proxy["obfs"] = obfs
			proxy["obfs-password"] = obfsPassword
		}
	}
	if alpn := splitALPN(firstString(mapString(value, "alpn"), mapString(value, "alpns"))); len(alpn) > 0 {
		proxy["alpn"] = alpn
	}
}

// splitALPN accepts the comma/space separated form used by Hysteria2 share
// links (`alpn=h3`, `alpn=h3,h2`) and returns a de-duplicated list.
func splitALPN(raw string) []string {
	if raw == "" {
		return nil
	}
	seen := map[string]struct{}{}
	result := []string{}
	for _, part := range strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == ' ' }) {
		value := strings.TrimSpace(part)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func mapString(value map[string]any, key string) string {
	return strings.TrimSpace(mapRawString(value, key))
}

func mapRawString(value map[string]any, key string) string {
	if value[key] == nil {
		return ""
	}
	return fmt.Sprint(value[key])
}

func mapInt(value map[string]any, key string) int {
	number, _ := strconv.Atoi(mapString(value, key))
	return number
}

func firstString(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
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
	count := used[value] + 1
	for {
		candidate := value
		if count > 1 {
			candidate = fmt.Sprintf("%s %d", value, count)
		}
		if candidate == value || used[candidate] == 0 {
			used[value] = count
			if candidate != value {
				used[candidate] = 1
			}
			return candidate
		}
		count++
	}
}
