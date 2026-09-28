package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/ziyue67/tms/go-backend/internal/cryptoutil"
	"github.com/ziyue67/tms/go-backend/internal/store"
)

func (a *API) flowConfig(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.nodeForFlow(w, r); !ok {
		return
	}
	writeText(w, "ok")
}

func (a *API) flowUpload(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.nodeForFlow(w, r); !ok {
		return
	}
	secret := r.URL.Query().Get("secret")
	payload := make([]byte, 2<<20)
	read, err := r.Body.Read(payload)
	if err != nil && read == 0 {
		writeText(w, "ok")
		return
	}
	payload = payload[:read]
	var wrapper struct {
		Encrypted bool   `json:"encrypted"`
		Data      string `json:"data"`
	}
	if json.Unmarshal(payload, &wrapper) == nil && wrapper.Encrypted {
		if crypto, cryptoErr := cryptoutil.NewAES(secret); cryptoErr == nil {
			if decrypted, decryptErr := crypto.Decrypt(wrapper.Data); decryptErr == nil {
				payload = decrypted
			}
		}
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
