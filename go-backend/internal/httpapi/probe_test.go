package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIsUDPOnlyProtocol(t *testing.T) {
	tests := []struct {
		protocol string
		want     bool
	}{
		{protocol: "hysteria", want: true},
		{protocol: "hysteria2", want: true},
		{protocol: "TUIC", want: true},
		{protocol: " vless ", want: false},
		{protocol: "trojan", want: false},
		{protocol: "vmess", want: false},
		{protocol: "anytls", want: false},
		{protocol: "", want: false},
	}
	for _, test := range tests {
		t.Run(test.protocol, func(t *testing.T) {
			if got := isUDPOnlyProtocol(test.protocol); got != test.want {
				t.Fatalf("isUDPOnlyProtocol(%q)=%v, want %v", test.protocol, got, test.want)
			}
		})
	}
}

func TestDiagnoseForwardSkipsTCPProbeForUDPOnlyProtocol(t *testing.T) {
	dataStore := &fakeStore{
		configs: map[string]string{},
		queryMaps: func(_ context.Context, query string, args ...any) ([]map[string]any, error) {
			switch {
			case strings.Contains(query, "FROM forward"):
				return []map[string]any{{
					"remoteAddr": "127.0.0.1:41003",
					"name":       "node-13-hysteria2",
					"inNodeId":   int64(13),
					"nodeName":   "node-13",
				}}, nil
			case strings.Contains(query, "FROM inbound"):
				if len(args) != 2 || args[0] != int64(13) || args[1] != 41003 {
					t.Fatalf("unexpected inbound lookup args: %#v", args)
				}
				return []map[string]any{{"protocol": "hysteria2"}}, nil
			default:
				return []map[string]any{}, nil
			}
		},
	}
	handler, tokens := testRouter(dataStore)
	token, err := tokens.Generate(1, "admin", 0)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/v1/forward/diagnose", bytes.NewBufferString(`{"forwardId":1}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	var body struct {
		Code int `json:"code"`
		Data struct {
			Results []map[string]any `json:"results"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Code != 0 || len(body.Data.Results) != 1 {
		t.Fatalf("unexpected response: status=%d body=%s", response.Code, response.Body.String())
	}
	result := body.Data.Results[0]
	if result["udpOnly"] != true || result["success"] != true {
		t.Fatalf("expected a skipped UDP-only probe, got %#v", result)
	}
	if message := toString(result["message"]); !strings.Contains(message, "跳过 TCP") {
		t.Fatalf("unexpected message: %q", message)
	}
}
