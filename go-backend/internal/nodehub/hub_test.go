package nodehub

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/ziyue67/tms/go-backend/internal/auth"
	"github.com/ziyue67/tms/go-backend/internal/cryptoutil"
	"github.com/ziyue67/tms/go-backend/internal/store"
)

type fakeNodeStore struct {
	mu      sync.Mutex
	updates int
}

func (s *fakeNodeStore) NodeBySecret(_ context.Context, secret string) (store.NodeConnection, error) {
	return store.NodeConnection{ID: 7, Secret: secret}, nil
}

func (s *fakeNodeStore) UpdateNodeConnection(context.Context, int64, string, int, int, int) error {
	s.mu.Lock()
	s.updates++
	s.mu.Unlock()
	return nil
}

func (s *fakeNodeStore) MarkNodeOffline(context.Context, int64) (bool, error) { return true, nil }

func TestEncryptedNodeCommandRoundTrip(t *testing.T) {
	dataStore := &fakeNodeStore{}
	hub := New(dataStore, auth.NewTokenService("jwt-secret", time.Hour), slog.New(slog.NewTextHandler(io.Discard, nil)))
	server := httptest.NewServer(hub)
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	address := "ws" + strings.TrimPrefix(server.URL, "http") + "/system-info?type=1&secret=node-secret&version=v1&http=80&tls=443&socks=1080"
	connection, _, err := websocket.Dial(ctx, address, nil)
	if err != nil {
		t.Fatalf("dial node websocket: %v", err)
	}
	defer connection.Close(websocket.StatusNormalClosure, "")

	crypto, err := cryptoutil.NewAES("node-secret")
	if err != nil {
		t.Fatalf("new AES: %v", err)
	}
	if _, _, err := connection.Read(ctx); err != nil { // initial status call
		t.Fatalf("read initial call: %v", err)
	}

	responded := make(chan error, 1)
	go func() {
		_, encryptedCommand, readErr := connection.Read(ctx)
		if readErr != nil {
			responded <- readErr
			return
		}
		var wrapper struct {
			Data string `json:"data"`
		}
		if readErr = json.Unmarshal(encryptedCommand, &wrapper); readErr != nil {
			responded <- readErr
			return
		}
		plain, readErr := crypto.Decrypt(wrapper.Data)
		if readErr != nil {
			responded <- readErr
			return
		}
		var command map[string]any
		if readErr = json.Unmarshal(plain, &command); readErr != nil {
			responded <- readErr
			return
		}
		response, _ := json.Marshal(map[string]any{"requestId": command["requestId"], "type": "TcpPingResponse", "message": "OK", "data": map[string]any{"success": true}})
		ciphertext, readErr := crypto.Encrypt(response)
		if readErr != nil {
			responded <- readErr
			return
		}
		envelope, _ := json.Marshal(map[string]any{"encrypted": true, "data": ciphertext, "timestamp": time.Now().Unix()})
		responded <- connection.Write(ctx, websocket.MessageText, envelope)
	}()

	result := hub.SendCommand(ctx, 7, "TcpPing", map[string]any{"host": "127.0.0.1", "port": 80})
	if err := <-responded; err != nil {
		t.Fatalf("node response: %v", err)
	}
	if result.Msg != "OK" {
		t.Fatalf("unexpected command result: %+v", result)
	}
	data, ok := result.Data.(map[string]any)
	if !ok || data["success"] != true {
		t.Fatalf("response data was lost: %#v", result.Data)
	}
}
