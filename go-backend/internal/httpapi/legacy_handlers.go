package httpapi

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/ziyue67/tms/go-backend/internal/store"
)

func (a *API) listTunnels(w http.ResponseWriter, r *http.Request) {
	items, err := a.store.QueryMaps(r.Context(), "SELECT t.*, n.name AS in_node_name, o.name AS out_node_name FROM tunnel t LEFT JOIN node n ON n.id=t.in_node_id LEFT JOIN node o ON o.id=t.out_node_id ORDER BY t.id")
	if err != nil {
		writeResponse(w, Error(-2, err.Error()))
		return
	}
	writeResponse(w, OK(items))
}

func (a *API) createTunnel(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if !decodeJSON(w, r, &body) {
		return
	}
	input, err := tunnelValues(body)
	if err != nil {
		writeResponse(w, Failure(err.Error()))
		return
	}
	values := commonCreatedValues(input)
	id, err := a.store.InsertMap(r.Context(), "tunnel", values)
	if err != nil {
		writeResponse(w, Failure("隧道创建失败: "+err.Error()))
		return
	}
	writeResponse(w, OK(map[string]any{"id": id, "message": "隧道创建成功"}))
}

func (a *API) updateTunnel(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if !decodeJSON(w, r, &body) {
		return
	}
	id, err := storeID(body)
	if err != nil {
		writeResponse(w, Failure("隧道ID不能为空"))
		return
	}
	values, err := tunnelValues(body)
	if err != nil {
		writeResponse(w, Failure(err.Error()))
		return
	}
	delete(values, "created_time")
	delete(values, "status")
	values["updated_time"] = time.Now().UnixMilli()
	if err := a.store.UpdateMap(r.Context(), "tunnel", id, values); err != nil {
		writeResponse(w, Failure("隧道更新失败: "+err.Error()))
		return
	}
	writeResponse(w, OK("隧道更新成功"))
}

func (a *API) deleteTunnel(w http.ResponseWriter, r *http.Request) {
	id, err := decodeID(w, r)
	if !err {
		return
	}
	if deleteErr := a.store.DeleteByID(r.Context(), "tunnel", id); deleteErr != nil {
		writeResponse(w, Failure("隧道删除失败: "+deleteErr.Error()))
		return
	}
	writeResponse(w, OK("隧道删除成功"))
}

func (a *API) listForwards(w http.ResponseWriter, r *http.Request) {
	claims, _ := claimsFrom(r)
	userID, _ := claims.UserID()
	query := "SELECT f.*, t.name AS tunnel_name, t.in_ip AS in_ip, t.out_ip AS out_ip FROM forward f LEFT JOIN tunnel t ON t.id=f.tunnel_id"
	args := []any{}
	if claims.RoleID != 0 {
		query += " WHERE f.user_id=?"
		args = append(args, userID)
	}
	query += " ORDER BY f.created_time DESC"
	items, err := a.store.QueryMaps(r.Context(), query, args...)
	if err != nil {
		writeResponse(w, Error(-2, err.Error()))
		return
	}
	writeResponse(w, OK(items))
}

func (a *API) createForward(w http.ResponseWriter, r *http.Request) {
	claims, _ := claimsFrom(r)
	userID, _ := claims.UserID()
	var body map[string]any
	if !decodeJSON(w, r, &body) {
		return
	}
	tunnelID, err := storeIDFrom(body, "tunnelId")
	if err != nil || strings.TrimSpace(toString(body["name"])) == "" || strings.TrimSpace(toString(body["remoteAddr"])) == "" {
		writeResponse(w, Failure("转发参数不完整"))
		return
	}
	now := time.Now().UnixMilli()
	values := map[string]any{"user_id": userID, "user_name": claims.Name, "name": toString(body["name"]), "tunnel_id": tunnelID,
		"in_port": numberOrZero(body["inPort"]), "remote_addr": toString(body["remoteAddr"]), "strategy": defaultStringValue(body["strategy"], "fifo"),
		"interface_name": nullableValue(body["interfaceName"]), "speed_id": nullableValue(body["speedId"]), "exp_time": nullableValue(body["expTime"]),
		"in_flow": int64(0), "out_flow": int64(0), "inx": int64(0), "status": 1, "created_time": now, "updated_time": now}
	id, err := a.store.InsertMap(r.Context(), "forward", values)
	if err != nil {
		writeResponse(w, Failure("转发创建失败: "+err.Error()))
		return
	}
	writeResponse(w, OK(map[string]any{"id": id, "message": "转发创建成功"}))
}

func (a *API) updateForward(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if !decodeJSON(w, r, &body) {
		return
	}
	id, err := storeID(body)
	if err != nil {
		writeResponse(w, Failure("转发ID不能为空"))
		return
	}
	values := map[string]any{"name": toString(body["name"]), "tunnel_id": numberOrZero(body["tunnelId"]), "remote_addr": toString(body["remoteAddr"]),
		"in_port": numberOrZero(body["inPort"]), "strategy": defaultStringValue(body["strategy"], "fifo"), "interface_name": nullableValue(body["interfaceName"]),
		"speed_id": nullableValue(body["speedId"]), "exp_time": nullableValue(body["expTime"]), "updated_time": time.Now().UnixMilli()}
	if err := a.store.UpdateMap(r.Context(), "forward", id, values); err != nil {
		writeResponse(w, Failure("转发更新失败: "+err.Error()))
		return
	}
	writeResponse(w, OK("转发更新成功"))
}

func (a *API) deleteForward(w http.ResponseWriter, r *http.Request) {
	id, ok := decodeID(w, r)
	if !ok {
		return
	}
	if err := a.store.DeleteByID(r.Context(), "forward", id); err != nil {
		writeResponse(w, Failure("转发删除失败: "+err.Error()))
		return
	}
	writeResponse(w, OK(nil))
}

func (a *API) pauseForward(w http.ResponseWriter, r *http.Request)  { a.setForwardStatus(w, r, 0) }
func (a *API) resumeForward(w http.ResponseWriter, r *http.Request) { a.setForwardStatus(w, r, 1) }
func (a *API) setForwardStatus(w http.ResponseWriter, r *http.Request, status int) {
	id, ok := decodeID(w, r)
	if !ok {
		return
	}
	if err := a.store.UpdateMap(r.Context(), "forward", id, map[string]any{"status": status, "updated_time": time.Now().UnixMilli()}); err != nil {
		writeResponse(w, Failure(err.Error()))
		return
	}
	writeResponse(w, OK(nil))
}

func (a *API) listSpeedLimits(w http.ResponseWriter, r *http.Request) {
	items, err := a.store.QueryMaps(r.Context(), "SELECT * FROM speed_limit ORDER BY id")
	if err != nil {
		writeResponse(w, Error(-2, err.Error()))
		return
	}
	writeResponse(w, OK(items))
}
func (a *API) createSpeedLimit(w http.ResponseWriter, r *http.Request) { a.saveSpeedLimit(w, r, false) }
func (a *API) updateSpeedLimit(w http.ResponseWriter, r *http.Request) { a.saveSpeedLimit(w, r, true) }
func (a *API) saveSpeedLimit(w http.ResponseWriter, r *http.Request, update bool) {
	var body map[string]any
	if !decodeJSON(w, r, &body) {
		return
	}
	values := map[string]any{"name": toString(body["name"]), "speed": numberOrZero(body["speed"]), "tunnel_id": nullableValue(body["tunnelId"]), "tunnel_name": nullableValue(body["tunnelName"]), "mode": numberOrZero(body["mode"]), "total": numberOrZero(body["total"]), "updated_time": time.Now().UnixMilli(), "status": 1}
	if !update {
		values["created_time"] = time.Now().UnixMilli()
		if _, err := a.store.InsertMap(r.Context(), "speed_limit", values); err != nil {
			writeResponse(w, Failure(err.Error()))
			return
		}
	} else {
		id, err := storeID(body)
		if err != nil || a.store.UpdateMap(r.Context(), "speed_limit", id, values) != nil {
			writeResponse(w, Failure("限速规则更新失败"))
			return
		}
	}
	writeResponse(w, OK(nil))
}
func (a *API) deleteSpeedLimit(w http.ResponseWriter, r *http.Request) {
	id, ok := decodeID(w, r)
	if !ok {
		return
	}
	if err := a.store.DeleteByID(r.Context(), "speed_limit", id); err != nil {
		writeResponse(w, Failure(err.Error()))
		return
	}
	writeResponse(w, OK(nil))
}

func tunnelValues(body map[string]any) (map[string]any, error) {
	inNode, err := storeIDFrom(body, "inNodeId")
	if err != nil {
		return nil, fmt.Errorf("入口节点不能为空")
	}
	outNode := numberOrZero(body["outNodeId"])
	if outNode == 0 {
		outNode = inNode
	}
	return map[string]any{"name": toString(body["name"]), "in_node_id": inNode, "out_node_id": outNode, "type": numberOrZero(body["type"]), "flow": numberOrZero(body["flow"]),
		"traffic_ratio": defaultNumber(body["trafficRatio"], 1), "protocol": defaultStringValue(body["protocol"], "tls"), "tcp_listen_addr": defaultStringValue(body["tcpListenAddr"], "0.0.0.0"),
		"udp_listen_addr": defaultStringValue(body["udpListenAddr"], "0.0.0.0"), "interface_name": nullableValue(body["interfaceName"])}, nil
}

func commonCreatedValues(values map[string]any) map[string]any {
	for key, value := range map[string]any{"created_time": time.Now().UnixMilli(), "updated_time": time.Now().UnixMilli(), "status": 1} {
		values[key] = value
	}
	return values
}

func storeID(body map[string]any) (int64, error)                 { return store.AsInt64(body["id"]) }
func storeIDFrom(body map[string]any, key string) (int64, error) { return store.AsInt64(body[key]) }
func toString(value any) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}
func nullableValue(value any) any {
	if value == nil || toString(value) == "" || toString(value) == "<nil>" {
		return nil
	}
	return value
}
func defaultStringValue(value any, fallback string) string {
	if toString(value) == "" {
		return fallback
	}
	return toString(value)
}
func defaultNumber(value any, fallback float64) any {
	if value == nil {
		return fallback
	}
	return value
}
func numberOrZero(value any) int64 { parsed, _ := store.AsInt64(value); return parsed }
