package httpapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/ziyue67/tms/go-backend/internal/store"
)

type subscriptionOutputStore struct {
	*fakeStore
	output store.SubscriptionOutput
	err    error
}

func (s *subscriptionOutputStore) SubscriptionByToken(context.Context, string) (store.SubscriptionOutput, error) {
	return s.output, s.err
}

func TestBuildSubscriptionLinksCoversNativeProtocols(t *testing.T) {
	output := store.SubscriptionOutput{Aggregate: true, Entries: []store.SubscriptionEntry{
		{Protocol: "vless", Server: "2001:db8::1", Port: 20001, UUID: "uuid", SNI: "example.com", PublicKey: "pk", ShortID: "sid", NodeName: "入口", Remark: "VLESS"},
		{Protocol: "shadowsocks", Server: "host", Port: 20002, ConfigJSON: `{"method":"2022-blake3-aes-256-gcm","password":"secret"}`, NodeName: "入口", Remark: "SS"},
		{Protocol: "vmess", Server: "host", Port: 20003, UUID: "uuid", NodeName: "入口", Remark: "VMess"},
	}}
	links := buildSubscriptionLinks(output)
	if len(links) != 3 || !strings.Contains(links[0], "@[2001:db8::1]:20001") || !strings.HasPrefix(links[1], "ss://") || !strings.HasPrefix(links[2], "vmess://") {
		t.Fatalf("unexpected links: %#v", links)
	}
	vmess, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(links[2], "vmess://"))
	if err != nil {
		t.Fatalf("decode VMess: %v", err)
	}
	var value map[string]string
	if err := json.Unmarshal(vmess, &value); err != nil || value["id"] != "uuid" {
		t.Fatalf("invalid VMess payload: %s %v", vmess, err)
	}
}

func TestClashDocumentUsesUniqueNames(t *testing.T) {
	output := store.SubscriptionOutput{Entries: []store.SubscriptionEntry{
		{Protocol: "vmess", Server: "a", Port: 1, UUID: "one", Remark: "same"},
		{Protocol: "vmess", Server: "b", Port: 2, UUID: "two", Remark: "same"},
	}}
	proxies := buildClashProxies(output)
	if proxies[0]["name"] != "same" || proxies[1]["name"] != "same 2" {
		t.Fatalf("duplicate names not resolved: %#v", proxies)
	}
}

func TestClashDocumentIncludesCustomNodes(t *testing.T) {
	output := store.SubscriptionOutput{CustomParsed: []map[string]any{
		{"protocol": "vless", "name": "custom", "server": "vless.example.com", "port": float64(443), "uuid": "uuid", "security": "reality", "sni": "example.com", "pbk": "public-key", "sid": "short-id", "fp": "chrome", "type": "ws", "path": "/ws", "host": "cdn.example.com"},
		{"protocol": "shadowsocks", "name": "custom", "server": "ss.example.com", "port": float64(8443), "method": "aes-256-gcm", "password": "secret"},
	}}
	proxies := buildClashProxies(output)
	if len(proxies) != 2 {
		t.Fatalf("custom nodes missing from Clash output: %#v", proxies)
	}
	if proxies[0]["type"] != "vless" || proxies[0]["name"] != "custom" || proxies[0]["tls"] != true {
		t.Fatalf("invalid VLESS custom proxy: %#v", proxies[0])
	}
	if proxies[0]["ws-opts"] == nil || proxies[0]["reality-opts"] == nil {
		t.Fatalf("VLESS transport or Reality options missing: %#v", proxies[0])
	}
	if proxies[1]["type"] != "ss" || proxies[1]["name"] != "custom 2" || proxies[1]["password"] != "secret" {
		t.Fatalf("invalid Shadowsocks custom proxy: %#v", proxies[1])
	}
}

func TestV2RaySubscriptionIncludesCustomNodes(t *testing.T) {
	const customLink = "vless://uuid@custom.example:443?security=tls#custom"
	dataStore := &subscriptionOutputStore{
		fakeStore: &fakeStore{configs: map[string]string{}},
		output:    store.SubscriptionOutput{UserID: 1, Usable: true, CustomLinks: []string{customLink}},
	}
	handler, _ := testRouter(dataStore)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/open_api/sub?token=test", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	decoded, err := base64.StdEncoding.DecodeString(response.Body.String())
	if err != nil {
		t.Fatalf("decode V2Ray subscription: %v", err)
	}
	if string(decoded) != customLink {
		t.Fatalf("custom node missing from V2Ray subscription: %q", decoded)
	}
}

func TestClashIncludesNativeAnyTLS(test *testing.T) {
	output := store.SubscriptionOutput{Entries: []store.SubscriptionEntry{
		{Protocol: "anytls", Server: "anytls.example", Port: 443, Password: "secret", SNI: "example.com"},
	}}
	proxies := buildClashProxies(output)
	if len(proxies) != 1 || proxies[0]["type"] != "anytls" || proxies[0]["password"] != "secret" {
		test.Fatalf("native AnyTLS missing from Clash output: %#v", proxies)
	}
}

func TestSubscriptionRejectsInvalidAndEmptyOutputs(test *testing.T) {
	cases := []struct {
		name   string
		token  string
		output store.SubscriptionOutput
		err    error
		status int
	}{
		{"missing token", "", store.SubscriptionOutput{UserID: 1, Usable: true}, nil, http.StatusUnauthorized},
		{"invalid token", "invalid", store.SubscriptionOutput{Usable: true}, nil, http.StatusUnauthorized},
		{"unusable subscription", "valid", store.SubscriptionOutput{UserID: 1}, nil, http.StatusForbidden},
		{"empty subscription", "valid", store.SubscriptionOutput{UserID: 1, Usable: true}, nil, http.StatusNotFound},
		{"store failure", "valid", store.SubscriptionOutput{}, errors.New("store unavailable"), http.StatusInternalServerError},
	}
	for _, format := range []string{"sub", "clash"} {
		for _, scenario := range cases {
			test.Run(format+"/"+scenario.name, func(test *testing.T) {
				dataStore := &subscriptionOutputStore{fakeStore: &fakeStore{configs: map[string]string{}}, output: scenario.output, err: scenario.err}
				handler, _ := testRouter(dataStore)
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/open_api/"+format+"?token="+scenario.token, nil))
				if response.Code != scenario.status {
					test.Fatalf("got HTTP %d, want %d", response.Code, scenario.status)
				}
				if !strings.Contains(response.Header().Get("Cache-Control"), "no-store") {
					test.Fatal("subscription errors must not be cached")
				}
			})
		}
	}
}

func TestSubscriptionOutputIsConsistentAcrossClientUserAgents(test *testing.T) {
	output := store.SubscriptionOutput{UserID: 1, Usable: true, Entries: []store.SubscriptionEntry{
		{Protocol: "vmess", Server: "example.com", Port: 443, UUID: "uuid", Remark: "香港 C++ 100%"},
		{Protocol: "anytls", Server: "example.com", Port: 443, Password: " secret+with spaces ", SNI: "example.com"},
	}}
	clients := []struct {
		name      string
		userAgent string
	}{
		{"no user agent", ""},
		{"Android FlClash", "FlClash (Android)"},
		{"Android v2rayNG", "v2rayNG (Android)"},
		{"Windows Clash", "Clash-Verge (Windows)"},
	}
	for _, format := range []string{"sub", "clash"} {
		dataStore := &subscriptionOutputStore{fakeStore: &fakeStore{configs: map[string]string{}}, output: output}
		handler, _ := testRouter(dataStore)
		var baseline string
		for _, client := range clients {
			test.Run(format+"/"+client.name, func(test *testing.T) {
				request := httptest.NewRequest(http.MethodGet, "/api/v1/open_api/"+format+"?token=test", nil)
				request.Header.Set("User-Agent", client.userAgent)
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, request)
				if response.Code != http.StatusOK {
					test.Fatalf("got HTTP %d, want 200", response.Code)
				}
				if !strings.Contains(response.Header().Get("Cache-Control"), "no-store") {
					test.Fatal("subscriptions must not be cached")
				}
				if baseline == "" {
					baseline = response.Body.String()
				} else if response.Body.String() != baseline {
					test.Fatal("subscription output changes with client user agent")
				}
			})
		}
	}
}

func TestNativeHysteria2LinkPreservesObfuscationPassword(test *testing.T) {
	password := " secret+with spaces "
	for _, key := range []string{"obfs-password", "obfs_password"} {
		test.Run(key, func(test *testing.T) {
			config, err := json.Marshal(map[string]any{"obfs": "salamander", key: password})
			if err != nil {
				test.Fatal(err)
			}
			entry := store.SubscriptionEntry{Protocol: "hysteria2", Server: "example.com", Port: 443, Password: "auth", ConfigJSON: string(config)}
			link, err := url.Parse(clientLink(entry, "test"))
			if err != nil {
				test.Fatal(err)
			}
			if link.Query().Get("obfs-password") != password {
				test.Fatal("obfuscation password whitespace discarded")
			}
		})
	}
}

func TestClashNamesAvoidGroupsAndGeneratedNameCollisions(test *testing.T) {
	remarks := []string{"same", "same 2", "same", "节点选择", "自动选择", "漏网之鱼", "DIRECT", "REJECT", "GLOBAL"}
	output := store.SubscriptionOutput{}
	for _, remark := range remarks {
		output.Entries = append(output.Entries, store.SubscriptionEntry{Protocol: "vmess", Server: "example.com", Port: 443, UUID: "uuid", Remark: remark})
	}
	seen := map[string]bool{"节点选择": true, "自动选择": true, "漏网之鱼": true, "DIRECT": true, "REJECT": true, "GLOBAL": true}
	for _, proxy := range buildClashProxies(output) {
		name := proxy["name"].(string)
		if seen[name] {
			test.Fatalf("proxy name collides with a group or another node: %q", name)
		}
		seen[name] = true
	}
}

func TestNativeLinksPreserveCredentialsAndRemarks(test *testing.T) {
	password := " secret+with spaces:@/%?& "
	remark := "香港 C++ 100%"
	for _, protocol := range []string{"vless", "trojan", "hysteria2", "tuic", "anytls"} {
		test.Run(protocol, func(test *testing.T) {
			entry := store.SubscriptionEntry{Protocol: protocol, Server: "2001:db8::1", Port: 443, UUID: "uuid", Password: password}
			parsed, err := url.Parse(clientLink(entry, remark))
			if err != nil {
				test.Fatal(err)
			}
			if parsed.Fragment != remark {
				test.Fatalf("remark changed during URL encoding: %q", parsed.Fragment)
			}
			if protocol == "vless" {
				return
			}
			credential := parsed.User.Username()
			if protocol == "tuic" {
				credential, _ = parsed.User.Password()
			}
			if credential != password {
				test.Fatalf("credential changed during URL encoding: %q", credential)
			}
		})
	}
}

func TestCustomClashPreservesPasswordWhitespace(test *testing.T) {
	password := " secret+with spaces "
	for _, protocol := range []string{"trojan", "hysteria2", "tuic", "anytls", "shadowsocks"} {
		test.Run(protocol, func(test *testing.T) {
			proxy, ok := customClashProxy(map[string]any{"protocol": protocol, "server": "example.com", "port": 443, "uuid": "uuid", "password": password, "method": "aes-256-gcm"}, map[string]int{})
			if !ok || proxy["password"] != password {
				test.Fatalf("password whitespace discarded: %#v", proxy)
			}
		})
	}
}

func TestCustomVMessUsesNetworkForWebSocketOptions(test *testing.T) {
	proxy, ok := customClashProxy(map[string]any{"protocol": "vmess", "server": "example.com", "port": 443, "uuid": "uuid", "net": "ws", "type": "none", "path": "/ws", "host": "cdn.example.com"}, map[string]int{})
	if !ok || proxy["network"] != "ws" || proxy["ws-opts"] == nil {
		test.Fatalf("VMess WebSocket options missing: %#v", proxy)
	}
}

func TestCustomTrojanWebSocketUsesWebSocketNetwork(test *testing.T) {
	proxy, ok := customClashProxy(map[string]any{"protocol": "trojan", "server": "example.com", "port": 443, "password": "secret", "type": "ws", "path": "/ws"}, map[string]int{})
	if !ok || proxy["network"] != "ws" || proxy["ws-opts"] == nil {
		test.Fatalf("Trojan WebSocket network missing: %#v", proxy)
	}
}
