package nodehub

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/ziyue67/tms/go-backend/internal/auth"
	"github.com/ziyue67/tms/go-backend/internal/cryptoutil"
	"github.com/ziyue67/tms/go-backend/internal/store"
)

type Store interface {
	NodeBySecret(context.Context, string) (store.NodeConnection, error)
	UpdateNodeConnection(context.Context, int64, string, int, int, int) error
	MarkNodeOffline(context.Context, int64) (bool, error)
}

type Result struct {
	Msg  string `json:"msg"`
	Data any    `json:"data,omitempty"`
}

type nodeState struct {
	SingboxRunning    *bool
	SingboxInstalled  *bool
	SingboxInstalling *bool
	SingboxInstallErr string
	SystemInfo        map[string]any
}

type client struct {
	connection *websocket.Conn
	nodeID     int64
	secret     string
	node       bool
	writeMu    sync.Mutex
}

type Hub struct {
	store  Store
	tokens *auth.TokenService
	logger *slog.Logger

	mu      sync.RWMutex
	nodes   map[int64]*client
	admins  map[*client]struct{}
	states  map[int64]nodeState
	pending map[string]chan Result
}

func New(store Store, tokens *auth.TokenService, logger *slog.Logger) *Hub {
	return &Hub{store: store, tokens: tokens, logger: logger, nodes: make(map[int64]*client),
		admins: make(map[*client]struct{}), states: make(map[int64]nodeState), pending: make(map[string]chan Result)}
}

func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	secret := r.URL.Query().Get("secret")
	isNode := r.URL.Query().Get("type") == "1"
	connectionClient := &client{node: isNode, secret: secret}
	var node store.NodeConnection
	if isNode {
		var err error
		node, err = h.store.NodeBySecret(r.Context(), secret)
		if err != nil {
			http.Error(w, "node authentication failed", http.StatusUnauthorized)
			return
		}
		connectionClient.nodeID = node.ID
	} else {
		claims, err := h.tokens.Validate(secret)
		if err != nil {
			http.Error(w, "authentication failed", http.StatusUnauthorized)
			return
		}
		connectionClient.nodeID, err = claims.UserID()
		if err != nil {
			http.Error(w, "authentication failed", http.StatusUnauthorized)
			return
		}
	}

	connection, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		h.logger.Warn("websocket accept failed", "error", err)
		return
	}
	connection.SetReadLimit(4 << 20)
	connectionClient.connection = connection
	defer connection.Close(websocket.StatusNormalClosure, "")

	if isNode {
		h.connectNode(r, connectionClient)
		defer h.disconnectNode(connectionClient)
	} else {
		h.connectAdmin(connectionClient)
		defer h.disconnectAdmin(connectionClient)
	}
	h.readLoop(r.Context(), connectionClient)
}

func (h *Hub) connectNode(r *http.Request, current *client) {
	h.mu.Lock()
	previous := h.nodes[current.nodeID]
	h.nodes[current.nodeID] = current
	h.mu.Unlock()
	if previous != nil && previous != current {
		_ = previous.connection.Close(websocket.StatusNormalClosure, "replaced by a new node connection")
	}
	version := r.URL.Query().Get("version")
	httpPort, _ := strconv.Atoi(r.URL.Query().Get("http"))
	tlsPort, _ := strconv.Atoi(r.URL.Query().Get("tls"))
	socksPort, _ := strconv.Atoi(r.URL.Query().Get("socks"))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := h.store.UpdateNodeConnection(ctx, current.nodeID, version, httpPort, tlsPort, socksPort); err != nil {
		h.logger.Error("update connected node", "node_id", current.nodeID, "error", err)
	}
	h.broadcast(map[string]any{"id": strconv.FormatInt(current.nodeID, 10), "type": "status", "data": 1})
	_ = h.write(current, []byte(`{"type":"call"}`), true)
	h.logger.Info("node connected", "node_id", current.nodeID, "version", version)
}

func (h *Hub) connectAdmin(current *client) {
	h.mu.Lock()
	h.admins[current] = struct{}{}
	nodes := make([]*client, 0, len(h.nodes))
	for _, node := range h.nodes {
		nodes = append(nodes, node)
	}
	h.mu.Unlock()
	for _, node := range nodes {
		_ = h.write(node, []byte(`{"type":"call"}`), true)
	}
}

func (h *Hub) readLoop(ctx context.Context, current *client) {
	for {
		messageType, payload, err := current.connection.Read(ctx)
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				h.logger.Debug("websocket read ended", "node_id", current.nodeID, "error", err)
			}
			return
		}
		if messageType != websocket.MessageText || len(payload) == 0 {
			continue
		}
		if current.node {
			payload = h.decrypt(payload, current.secret)
		}
		h.handleMessage(current, payload)
	}
}

func (h *Hub) handleMessage(current *client, payload []byte) {
	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		h.logger.Debug("invalid websocket JSON", "node_id", current.nodeID, "error", err)
		return
	}
	if requestID, _ := decoded["requestId"].(string); requestID != "" {
		message, _ := decoded["message"].(string)
		if message == "" {
			message = "无响应消息"
		}
		h.mu.Lock()
		waiting := h.pending[requestID]
		delete(h.pending, requestID)
		h.mu.Unlock()
		if waiting != nil {
			waiting <- Result{Msg: message, Data: decoded["data"]}
		}
	}
	if !current.node {
		return
	}
	if _, hasMemory := decoded["memory_usage"]; hasMemory {
		_ = h.write(current, []byte(`{"type":"call"}`), true)
	}
	h.updateState(current.nodeID, decoded)
	data, err := json.Marshal(decoded)
	if err != nil {
		data = payload
	}
	h.broadcast(map[string]any{"id": strconv.FormatInt(current.nodeID, 10), "type": "info", "data": string(data)})
}

func (h *Hub) updateState(nodeID int64, info map[string]any) {
	h.mu.Lock()
	defer h.mu.Unlock()
	state := h.states[nodeID]
	state.SingboxRunning = optionalBool(info, "singbox_running", state.SingboxRunning)
	state.SingboxInstalled = optionalBool(info, "singbox_installed", state.SingboxInstalled)
	state.SingboxInstalling = optionalBool(info, "singbox_installing", state.SingboxInstalling)
	if value, exists := info["singbox_install_err"]; exists {
		state.SingboxInstallErr, _ = value.(string)
	}
	if _, exists := info["memory_usage"]; exists {
		now := time.Now().UnixMilli()
		upload := nonNegativeInt64(info["bytes_transmitted"])
		download := nonNegativeInt64(info["bytes_received"])
		previousAt := nonNegativeInt64(state.SystemInfo["reported_at"])
		elapsed := now - previousAt
		uploadSpeed, downloadSpeed := float64(0), float64(0)
		if elapsed >= 250 && elapsed <= 60_000 {
			previousUpload := nonNegativeInt64(state.SystemInfo["bytes_transmitted"])
			previousDownload := nonNegativeInt64(state.SystemInfo["bytes_received"])
			if upload >= previousUpload {
				uploadSpeed = float64(upload-previousUpload) * 1000 / float64(elapsed)
			}
			if download >= previousDownload {
				downloadSpeed = float64(download-previousDownload) * 1000 / float64(elapsed)
			}
		}
		state.SystemInfo = map[string]any{"cpu_usage": info["cpu_usage"], "memory_usage": info["memory_usage"],
			"bytes_received": download, "bytes_transmitted": upload, "upload_speed": uploadSpeed,
			"download_speed": downloadSpeed, "reported_at": now, "uptime": info["uptime"]}
		info["upload_speed"], info["download_speed"], info["reported_at"] = uploadSpeed, downloadSpeed, now
	}
	h.states[nodeID] = state
}

func (h *Hub) disconnectNode(current *client) {
	time.AfterFunc(5*time.Second, func() {
		h.mu.Lock()
		if h.nodes[current.nodeID] != current {
			h.mu.Unlock()
			return
		}
		delete(h.nodes, current.nodeID)
		h.mu.Unlock()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		updated, err := h.store.MarkNodeOffline(ctx, current.nodeID)
		if err != nil {
			h.logger.Error("mark node offline", "node_id", current.nodeID, "error", err)
			return
		}
		if updated {
			h.broadcast(map[string]any{"id": strconv.FormatInt(current.nodeID, 10), "type": "status", "data": 0})
		}
	})
}

func (h *Hub) disconnectAdmin(current *client) {
	h.mu.Lock()
	delete(h.admins, current)
	h.mu.Unlock()
}

func (h *Hub) SendCommand(ctx context.Context, nodeID int64, commandType string, data any) Result {
	h.mu.RLock()
	node := h.nodes[nodeID]
	h.mu.RUnlock()
	if node == nil {
		return Result{Msg: "节点不在线"}
	}
	requestID, err := randomID()
	if err != nil {
		return Result{Msg: "发送消息失败: " + err.Error()}
	}
	waiting := make(chan Result, 1)
	h.mu.Lock()
	h.pending[requestID] = waiting
	h.mu.Unlock()
	payload, err := json.Marshal(map[string]any{"type": commandType, "data": data, "requestId": requestID})
	if err == nil {
		err = h.write(node, payload, true)
	}
	if err != nil {
		h.removePending(requestID)
		return Result{Msg: "发送消息失败: " + err.Error()}
	}
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	select {
	case result := <-waiting:
		return result
	case <-ctx.Done():
		h.removePending(requestID)
		return Result{Msg: "发送消息失败: " + ctx.Err().Error()}
	case <-timer.C:
		h.removePending(requestID)
		return Result{Msg: "等待响应超时"}
	}
}

func (h *Hub) removePending(requestID string) {
	h.mu.Lock()
	delete(h.pending, requestID)
	h.mu.Unlock()
}

func (h *Hub) write(target *client, payload []byte, encrypt bool) error {
	if encrypt && target.node && target.secret != "" {
		crypto, err := cryptoutil.NewAES(target.secret)
		if err != nil {
			return err
		}
		encrypted, err := crypto.Encrypt(payload)
		if err != nil {
			return err
		}
		payload, err = json.Marshal(map[string]any{"encrypted": true, "data": encrypted, "timestamp": time.Now().UnixMilli()})
		if err != nil {
			return err
		}
	}
	target.writeMu.Lock()
	defer target.writeMu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return target.connection.Write(ctx, websocket.MessageText, payload)
}

func (h *Hub) decrypt(payload []byte, secret string) []byte {
	var wrapper struct {
		Encrypted bool   `json:"encrypted"`
		Data      string `json:"data"`
	}
	if json.Unmarshal(payload, &wrapper) != nil || !wrapper.Encrypted || wrapper.Data == "" {
		return payload
	}
	crypto, err := cryptoutil.NewAES(secret)
	if err != nil {
		return payload
	}
	decrypted, err := crypto.Decrypt(wrapper.Data)
	if err != nil {
		h.logger.Warn("decrypt node message failed", "error", err)
		return payload
	}
	return decrypted
}

func (h *Hub) broadcast(message any) {
	payload, err := json.Marshal(message)
	if err != nil {
		return
	}
	h.mu.RLock()
	admins := make([]*client, 0, len(h.admins))
	for admin := range h.admins {
		admins = append(admins, admin)
	}
	h.mu.RUnlock()
	for _, admin := range admins {
		if err := h.write(admin, payload, false); err != nil {
			h.logger.Debug("broadcast to administrator failed", "error", err)
		}
	}
}

func (h *Hub) DisconnectNode(nodeID int64) {
	h.mu.Lock()
	node := h.nodes[nodeID]
	delete(h.nodes, nodeID)
	h.mu.Unlock()
	if node != nil {
		_ = node.connection.Close(websocket.StatusPolicyViolation, "node removed")
	}
}

func (h *Hub) State(nodeID int64) (map[string]any, bool) {
	h.mu.RLock()
	state, ok := h.states[nodeID]
	h.mu.RUnlock()
	if !ok {
		return nil, false
	}
	result := map[string]any{"singboxRunning": state.SingboxRunning, "singboxInstalled": state.SingboxInstalled,
		"singboxInstalling": state.SingboxInstalling, "singboxInstallErr": state.SingboxInstallErr}
	if state.SystemInfo != nil {
		copyInfo := make(map[string]any, len(state.SystemInfo))
		for key, value := range state.SystemInfo {
			copyInfo[key] = value
		}
		result["systemInfo"] = copyInfo
	}
	return result, true
}

func optionalBool(values map[string]any, key string, fallback *bool) *bool {
	value, exists := values[key]
	if !exists {
		return fallback
	}
	parsed, ok := value.(bool)
	if !ok {
		return fallback
	}
	return &parsed
}

func nonNegativeInt64(value any) int64 {
	var parsed int64
	switch typed := value.(type) {
	case float64:
		parsed = int64(typed)
	case int64:
		parsed = typed
	case json.Number:
		parsed, _ = typed.Int64()
	case string:
		parsed, _ = strconv.ParseInt(typed, 10, 64)
	}
	if parsed < 0 {
		return 0
	}
	return parsed
}

func randomID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	encoded := hex.EncodeToString(bytes)
	return strings.Join([]string{encoded[:8], encoded[8:12], encoded[12:16], encoded[16:20], encoded[20:]}, "-"), nil
}
