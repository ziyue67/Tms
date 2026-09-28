package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"
)

const successfulRunsURL = "https://api.github.com/repos/ziyue67/Tms/actions/runs?branch=main&status=success&per_page=1"

var versionCache struct {
	sync.Mutex
	latest string
	at     time.Time
	failed bool
}

func (a *API) versionInfo(w http.ResponseWriter, r *http.Request) {
	current := a.buildCommit
	if current == "" {
		current = "dev"
	}
	if len(current) > 7 {
		current = current[:7]
	}
	latest, failed := latestSuccessfulCommit(r.Context())
	writeResponse(w, OK(map[string]any{"panelVersion": "1.0.1", "commit": current, "buildTime": a.buildTime,
		"latest": nullableText(latest), "checkFailed": failed, "updateAvailable": latest != "" && current != "dev" && !strings.EqualFold(latest, current)}))
}

func latestSuccessfulCommit(ctx context.Context) (string, bool) {
	versionCache.Lock()
	defer versionCache.Unlock()
	if time.Since(versionCache.at) < 6*time.Hour && !versionCache.at.IsZero() {
		return versionCache.latest, versionCache.failed
	}
	requestCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	request, _ := http.NewRequestWithContext(requestCtx, http.MethodGet, successfulRunsURL, nil)
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("User-Agent", "TMS-Panel")
	response, err := http.DefaultClient.Do(request)
	versionCache.at = time.Now()
	if err != nil {
		versionCache.failed = true
		return versionCache.latest, true
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		versionCache.failed = true
		return versionCache.latest, true
	}
	var body struct {
		Runs []struct {
			HeadSHA string `json:"head_sha"`
		} `json:"workflow_runs"`
	}
	if json.NewDecoder(response.Body).Decode(&body) != nil || len(body.Runs) == 0 || len(body.Runs[0].HeadSHA) < 7 {
		versionCache.failed = true
		return versionCache.latest, true
	}
	versionCache.latest, versionCache.failed = body.Runs[0].HeadSHA[:7], false
	return versionCache.latest, false
}

func nullableText(value string) any {
	if value == "" {
		return nil
	}
	return value
}
