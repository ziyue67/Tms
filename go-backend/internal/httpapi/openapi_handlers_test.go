package httpapi

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ziyue67/tms/go-backend/internal/store"
)

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
