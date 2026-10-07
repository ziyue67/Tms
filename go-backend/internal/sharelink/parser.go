package sharelink

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

type Parsed struct {
	Protocol string
	Name     string
	Raw      string
	Values   map[string]any
	Outbound map[string]any
}

func Parse(input string) (Parsed, error) {
	raw := canonical(input)
	if raw == "" {
		return Parsed{}, errors.New("分享链接不能为空")
	}
	lower := strings.ToLower(raw)
	switch {
	case strings.HasPrefix(lower, "vmess://"):
		return parseVMess(raw)
	case strings.HasPrefix(lower, "ss://"):
		return parseShadowsocks(raw)
	case strings.HasPrefix(lower, "socks5://"), strings.HasPrefix(lower, "socks://"), strings.HasPrefix(lower, "socks4://"):
		return parseSocks(raw)
	case strings.HasPrefix(lower, "vless://"):
		return parseURLProtocol(raw, "vless")
	case strings.HasPrefix(lower, "trojan://"):
		return parseURLProtocol(raw, "trojan")
	case strings.HasPrefix(lower, "hysteria2://"), strings.HasPrefix(lower, "hy2://"):
		return parseURLProtocol(raw, "hysteria2")
	case strings.HasPrefix(lower, "tuic://"):
		return parseURLProtocol(raw, "tuic")
	case strings.HasPrefix(lower, "anytls://"):
		return parseURLProtocol(raw, "anytls")
	default:
		return parseBareSocks(raw)
	}
}

func parseSocks(raw string) (Parsed, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || u.Port() == "" {
		return Parsed{}, errors.New("服务器地址或端口不正确")
	}
	port, err := validPort(u.Port())
	if err != nil {
		return Parsed{}, err
	}
	values := map[string]any{"server": u.Hostname(), "port": port}
	outbound := map[string]any{"type": "socks", "server": u.Hostname(), "server_port": port, "version": "5"}
	if u.User != nil {
		values["username"] = u.User.Username()
		password, _ := u.User.Password()
		values["password"] = password
		outbound["username"], outbound["password"] = u.User.Username(), password
	}
	return Parsed{Protocol: "socks5", Name: fragment(u), Raw: raw, Values: values, Outbound: outbound}, nil
}

func parseBareSocks(raw string) (Parsed, error) {
	value := strings.SplitN(raw, "#", 2)[0]
	var username, password, host, port string
	if at := strings.LastIndex(value, "@"); at >= 0 {
		credentials := strings.SplitN(value[:at], ":", 2)
		username = credentials[0]
		if len(credentials) == 2 {
			password = credentials[1]
		}
		var err error
		host, port, err = splitHostPort(value[at+1:])
		if err != nil {
			return Parsed{}, err
		}
	} else {
		parts := strings.SplitN(value, ":", 4)
		if len(parts) < 2 {
			return Parsed{}, errors.New("落地链接不认识。住宅 socks 用 IP:端口 或 IP:端口:账号:密码")
		}
		host, port = parts[0], parts[1]
		if len(parts) >= 3 {
			username = parts[2]
		}
		if len(parts) == 4 {
			password = parts[3]
		}
	}
	parsedPort, err := validPort(port)
	if err != nil {
		return Parsed{}, errors.New("落地链接的端口不对。住宅 socks 用 IP:端口:账号:密码")
	}
	values := map[string]any{"server": host, "port": parsedPort, "username": username, "password": password}
	outbound := map[string]any{"type": "socks", "server": host, "server_port": parsedPort, "version": "5"}
	if username != "" {
		outbound["username"], outbound["password"] = username, password
	}
	return Parsed{Protocol: "socks5", Raw: raw, Values: values, Outbound: outbound}, nil
}

func parseShadowsocks(raw string) (Parsed, error) {
	body, name := stripFragment(strings.TrimPrefix(raw, raw[:strings.Index(raw, "://")+3]))
	if query := strings.Index(body, "?"); query >= 0 {
		body = body[:query]
	}
	var credential, address string
	if at := strings.LastIndex(body, "@"); at >= 0 {
		credential, address = body[:at], body[at+1:]
		if decoded, err := decodeBase64(credential); err == nil {
			credential = decoded
		} else if decoded, err := url.PathUnescape(credential); err == nil {
			credential = decoded
		}
	} else {
		decoded, err := decodeBase64(body)
		if err != nil {
			return Parsed{}, errors.New("ss 链接无法解码")
		}
		at := strings.LastIndex(decoded, "@")
		if at < 0 {
			return Parsed{}, errors.New("Shadowsocks 分享链接缺少服务器地址")
		}
		credential, address = decoded[:at], decoded[at+1:]
	}
	credentials := strings.SplitN(credential, ":", 2)
	if len(credentials) != 2 {
		return Parsed{}, errors.New("Shadowsocks 分享链接凭证不正确")
	}
	host, portText, err := splitHostPort(address)
	if err != nil {
		return Parsed{}, err
	}
	port, err := validPort(portText)
	if err != nil {
		return Parsed{}, err
	}
	values := map[string]any{"server": host, "port": port, "method": credentials[0], "password": credentials[1]}
	outbound := map[string]any{"type": "shadowsocks", "server": host, "server_port": port, "method": credentials[0], "password": credentials[1]}
	return Parsed{Protocol: "shadowsocks", Name: name, Raw: raw, Values: values, Outbound: outbound}, nil
}

func parseVMess(raw string) (Parsed, error) {
	body, _ := stripFragment(raw[strings.Index(raw, "://")+3:])
	decoded, err := decodeBase64(body)
	if err != nil {
		return Parsed{}, errors.New("VMess 分享链接不是有效的 Base64 JSON")
	}
	value := map[string]any{}
	if json.Unmarshal([]byte(decoded), &value) != nil {
		return Parsed{}, errors.New("VMess 分享链接不是有效的 Base64 JSON")
	}
	host := text(value["add"])
	port, err := validPort(text(value["port"]))
	if err != nil {
		return Parsed{}, err
	}
	name := text(value["ps"])
	values := map[string]any{"server": host, "port": port, "uuid": text(value["id"]), "name": name,
		"aid": text(value["aid"]), "scy": text(value["scy"]), "net": defaultText(value["net"], "tcp"), "tls": text(value["tls"]),
		"sni": text(value["sni"]), "host": text(value["host"]), "path": text(value["path"])}
	outbound := map[string]any{"type": "vmess", "server": host, "server_port": port, "uuid": text(value["id"]),
		"security": defaultText(value["scy"], "auto"), "alter_id": integer(value["aid"])}
	addTLSAndTransport(outbound, values)
	return Parsed{Protocol: "vmess", Name: name, Raw: raw, Values: values, Outbound: outbound}, nil
}

func parseURLProtocol(raw, protocol string) (Parsed, error) {
	u, err := url.Parse(raw)
	if err != nil || u.User == nil || u.Hostname() == "" || u.Port() == "" {
		return Parsed{}, errors.New("分享链接缺少凭证或服务器地址")
	}
	port, err := validPort(u.Port())
	if err != nil {
		return Parsed{}, err
	}
	credential := u.User.Username()
	password, hasPassword := u.User.Password()
	values := map[string]any{}
	for key, items := range u.Query() {
		if len(items) > 0 {
			values[key] = items[len(items)-1]
		}
	}
	values["server"], values["port"], values["name"] = u.Hostname(), port, fragment(u)
	switch protocol {
	case "vless":
		values["uuid"] = credential
	case "tuic":
		values["uuid"] = credential
		if hasPassword {
			values["password"] = password
		}
	default:
		values["password"] = credential
	}
	outbound := map[string]any{"type": protocol, "server": u.Hostname(), "server_port": port}
	if protocol == "vless" || protocol == "tuic" {
		outbound["uuid"] = values["uuid"]
	} else {
		outbound["password"] = values["password"]
	}
	if protocol == "tuic" {
		outbound["password"] = values["password"]
	}
	addTLSAndTransport(outbound, values)
	return Parsed{Protocol: protocol, Name: fragment(u), Raw: raw, Values: values, Outbound: outbound}, nil
}

func addTLSAndTransport(outbound, values map[string]any) {
	security := strings.ToLower(defaultText(values["security"], text(values["tls"])))
	protocol := text(outbound["type"])
	if security == "tls" || security == "xtls" || security == "reality" || protocol == "trojan" || protocol == "hysteria2" || protocol == "tuic" || protocol == "anytls" {
		sni := first(text(values["sni"]), text(values["peer"]), text(outbound["server"]))
		tls := map[string]any{"enabled": true, "server_name": sni}
		if security == "reality" {
			tls["utls"] = map[string]any{"enabled": true, "fingerprint": first(text(values["fp"]), "chrome")}
			tls["reality"] = map[string]any{"enabled": true, "public_key": text(values["pbk"]), "short_id": text(values["sid"])}
		} else {
			tls["insecure"] = true
		}
		outbound["tls"] = tls
	}
	network := strings.ToLower(first(text(values["type"]), text(values["net"])))
	path := first(text(values["path"]), text(values["serviceName"]))
	switch network {
	case "ws":
		transport := map[string]any{"type": "ws", "path": path}
		if host := text(values["host"]); host != "" {
			transport["headers"] = map[string]string{"Host": host}
		}
		outbound["transport"] = transport
	case "grpc":
		outbound["transport"] = map[string]any{"type": "grpc", "service_name": path}
	}
}

func canonical(value string) string {
	return strings.NewReplacer("&amp;", "&", `\://`, "://", `\@`, "@", `\_`, "_").Replace(strings.TrimSpace(value))
}
func stripFragment(value string) (string, string) {
	if index := strings.Index(value, "#"); index >= 0 {
		name, _ := url.PathUnescape(value[index+1:])
		return value[:index], name
	}
	return value, ""
}
func splitHostPort(value string) (string, string, error) {
	if strings.HasPrefix(value, "[") {
		host, port, err := net.SplitHostPort(value)
		return strings.Trim(host, "[]"), port, err
	}
	index := strings.LastIndex(value, ":")
	if index <= 0 || index == len(value)-1 {
		return "", "", errors.New("服务器地址或端口不正确")
	}
	return value[:index], value[index+1:], nil
}
func decodeBase64(value string) (string, error) {
	value = strings.NewReplacer("-", "+", "_", "/").Replace(strings.TrimSpace(value))
	value += strings.Repeat("=", (4-len(value)%4)%4)
	decoded, err := base64.StdEncoding.DecodeString(value)
	return string(decoded), err
}
func validPort(value string) (int, error) {
	port, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || port < 1 || port > 65535 {
		return 0, errors.New("端口不正确")
	}
	return port, nil
}
func fragment(parsedURL *url.URL) string { return parsedURL.Fragment }
func text(value any) string {
	if value == nil {
		return ""
	}
	return fmt.Sprint(value)
}
func defaultText(value any, fallback string) string {
	if result := text(value); result != "" {
		return result
	}
	return fallback
}
func integer(value any) int { parsed, _ := strconv.Atoi(text(value)); return parsed }
func first(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
