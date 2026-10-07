package sharelink

import (
	"encoding/base64"
	"testing"
)

func TestParseSupportedLinks(t *testing.T) {
	vmessJSON := `{"add":"host","port":"443","id":"uuid","ps":"node","net":"tcp"}`
	cases := []struct{ raw, protocol string }{
		{"127.0.0.1:1080:user:pass", "socks5"},
		{"socks5://user:pass@host:1080", "socks5"},
		{"ss://" + base64.RawURLEncoding.EncodeToString([]byte("aes-256-gcm:secret")) + "@host:443#ss", "shadowsocks"},
		{"vmess://" + base64.StdEncoding.EncodeToString([]byte(vmessJSON)), "vmess"},
		{"vless://uuid@host:443?security=reality&sni=example.com&pbk=pk&sid=sid#vless", "vless"},
		{"trojan://secret@host:443?sni=example.com#trojan", "trojan"},
		{"hysteria2://secret@host:443?sni=example.com#hy2", "hysteria2"},
		{"tuic://uuid:secret@host:443?sni=example.com#tuic", "tuic"},
		{"anytls://secret@host:443?sni=example.com#any", "anytls"},
	}
	for _, test := range cases {
		parsed, err := Parse(test.raw)
		if err != nil {
			t.Errorf("parse %s: %v", test.protocol, err)
			continue
		}
		if parsed.Protocol != test.protocol || parsed.Values["server"] != "host" && test.protocol != "socks5" {
			t.Errorf("unexpected parsed %s: %+v", test.protocol, parsed)
		}
	}
}

func TestURLParametersCannotReplaceEndpoint(test *testing.T) {
	parsed, err := Parse("vless://uuid@example.com:443?server=other.example&port=1#node")
	if err != nil {
		test.Fatal(err)
	}
	if parsed.Values["server"] != "example.com" || parsed.Values["port"] != 443 {
		test.Fatalf("query parameters replaced the endpoint: %#v", parsed.Values)
	}
}

func TestFragmentIsDecodedExactlyOnce(test *testing.T) {
	credential := base64.RawURLEncoding.EncodeToString([]byte("aes-256-gcm:secret"))
	for _, raw := range []string{"vless://uuid@example.com:443#C%2B%2B%20100%25", "ss://" + credential + "@example.com:443#C++%20100%25"} {
		parsed, err := Parse(raw)
		if err != nil {
			test.Fatal(err)
		}
		if parsed.Name != "C++ 100%" {
			test.Fatalf("fragment decoded incorrectly: %q", parsed.Name)
		}
	}
}
