package httpapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ziyue67/tms/go-backend/internal/store"
)

type subscriptionOutputStore struct {
	*fakeStore
	output store.SubscriptionOutput
}

func (s *subscriptionOutputStore) SubscriptionByToken(context.Context, string) (store.SubscriptionOutput, error) {
	return s.output, nil
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
		output:    store.SubscriptionOutput{CustomLinks: []string{customLink}},
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
