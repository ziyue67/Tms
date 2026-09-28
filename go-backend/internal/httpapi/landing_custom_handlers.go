package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/ziyue67/tms/go-backend/internal/sharelink"
)

func (a *API) createLanding(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if !decodeJSON(w, r, &body) {
		return
	}
	name, link := toString(body["name"]), toString(body["link"])
	if name == "" {
		writeResponse(w, Error(500, "落地名称不能为空"))
		return
	}
	parsed, err := sharelink.Parse(link)
	if err != nil {
		writeResponse(w, Failure(err.Error()))
		return
	}
	outbound, _ := json.Marshal(parsed.Outbound)
	now := time.Now().UnixMilli()
	id, err := a.store.InsertMap(r.Context(), "landing", map[string]any{"name": name, "type": parsed.Protocol, "link": parsed.Raw,
		"outbound_json": string(outbound), "remark": nullableValue(body["remark"]), "status": 1, "created_time": now, "updated_time": now})
	if err != nil {
		writeResponse(w, Failure("落地保存失败"))
		return
	}
	writeResponse(w, OK(map[string]any{"id": id, "name": name, "type": parsed.Protocol, "link": parsed.Raw, "outboundJson": string(outbound), "remark": nullableValue(body["remark"]), "status": 1, "createdTime": now, "updatedTime": now}))
}

func (a *API) listLandings(w http.ResponseWriter, r *http.Request) {
	items, err := a.store.QueryMaps(r.Context(), "SELECT * FROM landing ORDER BY id DESC")
	if err != nil {
		writeResponse(w, Error(-2, err.Error()))
		return
	}
	writeResponse(w, OK(items))
}

func (a *API) renameLanding(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if !decodeJSON(w, r, &body) {
		return
	}
	id, err := storeID(body)
	if err != nil {
		writeResponse(w, Failure("参数不全"))
		return
	}
	name := toString(body["name"])
	if name == "" {
		writeResponse(w, Failure("落地名称不能为空"))
		return
	}
	if err := a.store.UpdateMap(r.Context(), "landing", id, map[string]any{"name": name, "updated_time": time.Now().UnixMilli()}); err != nil {
		writeResponse(w, Failure("落地名称更新失败"))
		return
	}
	writeResponse(w, OK(nil))
}

func (a *API) deleteLanding(w http.ResponseWriter, r *http.Request) {
	id, ok := decodeID(w, r)
	if !ok {
		return
	}
	used, err := a.store.QueryMaps(r.Context(), "SELECT id FROM inbound WHERE landing_id=? LIMIT 1", id)
	if err != nil {
		writeResponse(w, Error(-2, err.Error()))
		return
	}
	if len(used) > 0 {
		writeResponse(w, Failure("这条落地正在被中转协议使用,先清空对应机器的中转再删"))
		return
	}
	if err := a.store.DeleteByID(r.Context(), "landing", id); err != nil {
		writeResponse(w, Failure(err.Error()))
		return
	}
	writeResponse(w, OK(nil))
}

func (a *API) testLanding(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if !decodeJSON(w, r, &body) {
		return
	}
	parsed, err := sharelink.Parse(toString(body["link"]))
	if err != nil {
		writeResponse(w, Failure(err.Error()))
		return
	}
	if parsed.Protocol != "socks5" {
		writeResponse(w, OK(map[string]any{"ok": true, "skipped": true, "type": parsed.Protocol, "msg": parsed.Protocol + " 落地格式已校验(协议落地暂不支持在线测试,可直接保存)"}))
		return
	}
	nodeID, err := storeIDFrom(body, "nodeId")
	if err != nil {
		writeResponse(w, Failure("前置机不存在"))
		return
	}
	if _, err := a.store.NodeByID(r.Context(), nodeID); err != nil {
		writeResponse(w, Failure("前置机不存在"))
		return
	}
	payload := map[string]any{"type": parsed.Outbound["type"], "server": parsed.Outbound["server"], "port": parsed.Outbound["server_port"]}
	if parsed.Outbound["username"] != nil {
		payload["username"] = parsed.Outbound["username"]
	}
	if parsed.Outbound["password"] != nil {
		payload["password"] = parsed.Outbound["password"]
	}
	result := a.nodeHub.SendCommand(r.Context(), nodeID, "TestOutbound", payload)
	if result.Msg != "OK" {
		writeResponse(w, Failure("经前置机测落地不通:"+result.Msg))
		return
	}
	data, _ := result.Data.(map[string]any)
	if data == nil {
		data = map[string]any{}
	}
	data["type"] = parsed.Protocol
	writeResponse(w, OK(data))
}

func (a *API) listCustomNodes(w http.ResponseWriter, r *http.Request) {
	items, err := a.store.QueryMaps(r.Context(), "SELECT id,name,protocol,visibility,status,created_time FROM custom_node ORDER BY id DESC")
	if err != nil {
		writeResponse(w, Error(-2, err.Error()))
		return
	}
	for _, item := range items {
		id, _ := requestInt64(item["id"])
		assignments, _ := a.store.QueryMaps(r.Context(), "SELECT user_id FROM user_custom_node WHERE custom_node_id=? AND status=1 ORDER BY id", id)
		userIDs := make([]any, 0, len(assignments))
		for _, assignment := range assignments {
			userIDs = append(userIDs, assignment["userId"])
		}
		item["id"], item["userIds"] = fmt.Sprint(id), userIDs
		if item["visibility"] == nil {
			item["visibility"] = "global"
		}
	}
	writeResponse(w, OK(items))
}

func (a *API) importCustomNodes(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if !decodeJSON(w, r, &body) {
		return
	}
	links := []string{}
	if values, ok := body["links"].([]any); ok {
		for _, value := range values {
			if link := toString(value); link != "" {
				links = append(links, link)
			}
		}
	}
	if len(links) == 0 {
		for _, line := range strings.Split(strings.ReplaceAll(toString(body["link"]), "\r", ""), "\n") {
			if strings.TrimSpace(line) != "" {
				links = append(links, strings.TrimSpace(line))
			}
		}
	}
	if len(links) == 0 {
		writeResponse(w, Failure("请输入协议分享链接"))
		return
	}
	visibility := strings.ToLower(defaultStringValue(body["visibility"], "global"))
	if visibility != "users" && visibility != "subscribers" {
		visibility = "global"
	}
	userIDs := []int64{}
	if values, ok := body["userIds"].([]any); ok {
		for _, value := range values {
			if id, err := requestInt64(value); err == nil {
				userIDs = append(userIDs, id)
			}
		}
	}
	if visibility == "users" && len(userIDs) == 0 {
		writeResponse(w, Failure("按用户订阅时至少选择一个用户"))
		return
	}
	imported, failures := []any{}, []map[string]string{}
	for _, link := range links {
		parsed, err := sharelink.Parse(link)
		if err != nil {
			failures = append(failures, map[string]string{"link": link, "error": err.Error()})
			continue
		}
		name := toString(body["name"])
		if name == "" {
			name = parsed.Name
		}
		if name == "" {
			name = "自定义 " + strings.ToUpper(parsed.Protocol) + " 节点"
		}
		encoded, _ := json.Marshal(parsed.Values)
		now := time.Now().UnixMilli()
		id, err := a.store.InsertMap(r.Context(), "custom_node", map[string]any{"name": name, "protocol": parsed.Protocol, "raw_link": parsed.Raw, "parsed_json": string(encoded), "visibility": visibility, "status": 1, "created_time": now, "updated_time": now})
		if err != nil {
			failures = append(failures, map[string]string{"link": link, "error": err.Error()})
			continue
		}
		for _, userID := range userIDs {
			_, _ = a.store.InsertMap(r.Context(), "user_custom_node", map[string]any{"user_id": userID, "custom_node_id": id, "status": 1, "created_time": now})
		}
		imported = append(imported, map[string]any{"id": fmt.Sprint(id), "name": name, "protocol": parsed.Protocol, "visibility": visibility, "status": 1, "createdTime": now})
	}
	if len(links) == 1 && len(imported) == 1 {
		writeResponse(w, OK(imported[0]))
		return
	}
	writeResponse(w, OK(map[string]any{"imported": imported, "successCount": len(imported), "failureCount": len(failures), "errors": failures}))
}

func (a *API) assignCustomNode(w http.ResponseWriter, r *http.Request) {
	nodeID, ok := pathInt64(w, r, "nodeId")
	if !ok {
		return
	}
	var body map[string]any
	if !decodeJSON(w, r, &body) {
		return
	}
	userID, err := storeIDFrom(body, "userId")
	if err != nil {
		writeResponse(w, Failure("用户参数错误"))
		return
	}
	existing, err := a.store.QueryMaps(r.Context(), "SELECT id,status FROM user_custom_node WHERE custom_node_id=? AND user_id=? LIMIT 1", nodeID, userID)
	if err != nil {
		writeResponse(w, Error(-2, err.Error()))
		return
	}
	if len(existing) == 0 {
		_, err = a.store.InsertMap(r.Context(), "user_custom_node", map[string]any{"user_id": userID, "custom_node_id": nodeID, "status": 1, "created_time": time.Now().UnixMilli()})
	} else {
		id, _ := requestInt64(existing[0]["id"])
		err = a.store.UpdateMap(r.Context(), "user_custom_node", id, map[string]any{"status": 1})
	}
	if err != nil {
		writeResponse(w, Failure(err.Error()))
		return
	}
	writeResponse(w, OK(nil))
}

func (a *API) unassignCustomNode(w http.ResponseWriter, r *http.Request) {
	nodeID, ok := pathInt64(w, r, "nodeId")
	if !ok {
		return
	}
	userID, ok := pathInt64(w, r, "userId")
	if !ok {
		return
	}
	rows, err := a.store.QueryMaps(r.Context(), "SELECT id FROM user_custom_node WHERE custom_node_id=? AND user_id=?", nodeID, userID)
	if err == nil {
		for _, row := range rows {
			id, _ := requestInt64(row["id"])
			_ = a.store.DeleteByID(r.Context(), "user_custom_node", id)
		}
	}
	writeResponse(w, OK(nil))
}
func (a *API) disableCustomNode(w http.ResponseWriter, r *http.Request) {
	a.setCustomNodeStatus(w, r, 0)
}
func (a *API) enableCustomNode(w http.ResponseWriter, r *http.Request) {
	a.setCustomNodeStatus(w, r, 1)
}
func (a *API) setCustomNodeStatus(w http.ResponseWriter, r *http.Request, status int) {
	id, ok := pathInt64(w, r, "nodeId")
	if !ok {
		return
	}
	if err := a.store.UpdateMap(r.Context(), "custom_node", id, map[string]any{"status": status, "updated_time": time.Now().UnixMilli()}); err != nil {
		writeResponse(w, Failure("自定义节点不存在"))
		return
	}
	rows, _ := a.store.QueryMaps(r.Context(), "SELECT id,name,protocol,visibility,status,created_time,updated_time FROM custom_node WHERE id=?", id)
	if len(rows) == 0 {
		writeResponse(w, Failure("自定义节点不存在"))
		return
	}
	rows[0]["id"] = fmt.Sprint(id)
	writeResponse(w, OK(rows[0]))
}
func (a *API) deleteCustomNode(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt64(w, r, "nodeId")
	if !ok {
		return
	}
	assignments, _ := a.store.QueryMaps(r.Context(), "SELECT id FROM user_custom_node WHERE custom_node_id=?", id)
	for _, row := range assignments {
		assignmentID, _ := requestInt64(row["id"])
		_ = a.store.DeleteByID(r.Context(), "user_custom_node", assignmentID)
	}
	_ = a.store.DeleteByID(r.Context(), "custom_node", id)
	writeResponse(w, OK(nil))
}
