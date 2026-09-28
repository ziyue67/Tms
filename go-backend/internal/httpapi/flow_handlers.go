package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ziyue67/tms/go-backend/internal/cryptoutil"
	"github.com/ziyue67/tms/go-backend/internal/store"
)

func (a *API) flowConfig(w http.ResponseWriter, r *http.Request) {
	node, ok := a.nodeForFlow(w, r)
	if !ok {
		return
	}
	payload, err := readFlowPayload(w, r, r.URL.Query().Get("secret"))
	if err == nil && len(payload) > 0 {
		var report gostConfigReport
		if err := json.Unmarshal(payload, &report); err == nil {
			go a.cleanNodeConfig(node.ID, report)
		} else {
			a.logger.Warn("invalid node config report", "node_id", node.ID, "error", err)
		}
	}
	writeText(w, "ok")
}

func (a *API) flowUpload(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.nodeForFlow(w, r); !ok {
		return
	}
	secret := r.URL.Query().Get("secret")
	payload, err := readFlowPayload(w, r, secret)
	if err != nil || len(payload) == 0 {
		writeText(w, "ok")
		return
	}
	var report struct {
		Name     string `json:"n"`
		Upload   int64  `json:"u"`
		Download int64  `json:"d"`
	}
	if json.Unmarshal(payload, &report) == nil && report.Name != "" && report.Name != "web_api" {
		parts := strings.Split(report.Name, "_")
		if len(parts) >= 3 {
			forwardID, forwardErr := strconv.ParseInt(parts[0], 10, 64)
			userID, userErr := strconv.ParseInt(parts[1], 10, 64)
			userTunnelID, tunnelErr := strconv.ParseInt(parts[2], 10, 64)
			if forwardErr == nil && userErr == nil && tunnelErr == nil {
				_ = a.store.RecordTrafficUsage(r.Context(), forwardID, userID, userTunnelID, report.Upload, report.Download)
			}
		}
	}
	writeText(w, "ok")
}

func readFlowPayload(w http.ResponseWriter, r *http.Request, secret string) ([]byte, error) {
	payload, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	var wrapper struct {
		Encrypted bool   `json:"encrypted"`
		Data      string `json:"data"`
	}
	if json.Unmarshal(payload, &wrapper) == nil && wrapper.Encrypted {
		if crypto, cryptoErr := cryptoutil.NewAES(secret); cryptoErr == nil {
			if decrypted, decryptErr := crypto.Decrypt(wrapper.Data); decryptErr == nil {
				return decrypted, nil
			}
		}
	}
	return payload, nil
}

type gostConfigItem struct {
	Name string `json:"name"`
}

type gostConfigReport struct {
	Services []gostConfigItem `json:"services"`
	Chains   []gostConfigItem `json:"chains"`
	Limiters []gostConfigItem `json:"limiters"`
}

func (a *API) cleanNodeConfig(nodeID int64, report gostConfigReport) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	forwardExists := func(name string) (string, bool) {
		parts := strings.Split(name, "_")
		if len(parts) != 4 {
			return "", true
		}
		if _, err := strconv.ParseInt(parts[0], 10, 64); err != nil {
			return "", true
		}
		rows, err := a.store.QueryMaps(ctx, "SELECT id FROM forward WHERE id=? LIMIT 1", parts[0])
		return strings.Join(parts[:3], "_"), err != nil || len(rows) > 0
	}
	for _, service := range report.Services {
		if service.Name == "web_api" {
			continue
		}
		base, exists := forwardExists(service.Name)
		if exists || base == "" {
			continue
		}
		if strings.HasSuffix(service.Name, "_tcp") {
			a.sendCleanupCommand(ctx, nodeID, "DeleteService", map[string]any{"services": []string{base + "_tcp", base + "_udp"}}, service.Name)
		} else if strings.HasSuffix(service.Name, "_tls") {
			a.sendCleanupCommand(ctx, nodeID, "DeleteService", map[string]any{"services": []string{base + "_tls"}}, service.Name)
		}
	}
	for _, chain := range report.Chains {
		base, exists := forwardExists(chain.Name)
		if !exists && base != "" && strings.HasSuffix(chain.Name, "_chains") {
			a.sendCleanupCommand(ctx, nodeID, "DeleteChains", map[string]any{"chain": base + "_chains"}, chain.Name)
		}
	}
	for _, limiter := range report.Limiters {
		limiterID, err := strconv.ParseInt(limiter.Name, 10, 64)
		if err != nil {
			continue
		}
		query, id := "SELECT id FROM speed_limit WHERE id=? LIMIT 1", limiterID
		if limiterID >= 900000000 {
			query, id = "SELECT id FROM tms_user WHERE id=? LIMIT 1", limiterID-900000000
		}
		rows, queryErr := a.store.QueryMaps(ctx, query, id)
		if queryErr == nil && len(rows) == 0 {
			a.sendCleanupCommand(ctx, nodeID, "DeleteLimiters", map[string]any{"limiter": limiter.Name}, limiter.Name)
		}
	}
}

func (a *API) sendCleanupCommand(ctx context.Context, nodeID int64, command string, data any, name string) {
	result := a.nodeHub.SendCommand(ctx, nodeID, command, data)
	if result.Msg != "OK" && !strings.Contains(strings.ToLower(result.Msg), "not found") {
		a.logger.Warn("node config cleanup failed", "node_id", nodeID, "command", command, "name", name, "error", result.Msg)
	}
}

func (a *API) nodeForFlow(w http.ResponseWriter, r *http.Request) (store.NodeConnection, bool) {
	secret := r.URL.Query().Get("secret")
	if secret == "" {
		writeText(w, "ok")
		return store.NodeConnection{}, false
	}
	node, err := a.store.NodeBySecret(r.Context(), secret)
	if err != nil || store.IsNotFound(err) {
		writeText(w, "ok")
		return store.NodeConnection{}, false
	}
	return node, true
}

func writeText(w http.ResponseWriter, value string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(value))
}
