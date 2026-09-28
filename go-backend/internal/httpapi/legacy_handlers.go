package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
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
	inNodeID, _ := requestInt64(values["in_node_id"])
	outNodeID, _ := requestInt64(values["out_node_id"])
	inNode, inErr := a.store.NodeByID(r.Context(), inNodeID)
	outNode, outErr := a.store.NodeByID(r.Context(), outNodeID)
	if inErr != nil || outErr != nil {
		writeResponse(w, Failure("入口或出口节点不存在"))
		return
	}
	values["in_ip"], values["out_ip"] = inNode.IP, outNode.ServerIP
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
	rows, err := a.store.QueryMaps(r.Context(), "SELECT * FROM tunnel WHERE id=?", id)
	if err != nil || len(rows) == 0 {
		writeResponse(w, Failure("隧道不存在"))
		return
	}
	current := rows[0]
	name := defaultStringValue(body["name"], toString(current["name"]))
	duplicates, _ := a.store.QueryMaps(r.Context(), "SELECT id FROM tunnel WHERE name=? AND id<>? LIMIT 1", name, id)
	if len(duplicates) > 0 {
		writeResponse(w, Failure("隧道名称已存在"))
		return
	}
	values := map[string]any{
		"name": name, "flow": valueOr(body, "flow", current["flow"]),
		"traffic_ratio":   valueOr(body, "trafficRatio", current["trafficRatio"]),
		"protocol":        defaultStringValue(body["protocol"], defaultStringValue(current["protocol"], "tls")),
		"tcp_listen_addr": defaultStringValue(body["tcpListenAddr"], defaultStringValue(current["tcpListenAddr"], "0.0.0.0")),
		"udp_listen_addr": defaultStringValue(body["udpListenAddr"], defaultStringValue(current["udpListenAddr"], "0.0.0.0")),
		"interface_name":  valueOr(body, "interfaceName", current["interfaceName"]),
	}
	values["updated_time"] = time.Now().UnixMilli()
	if err := a.store.UpdateMap(r.Context(), "tunnel", id, values); err != nil {
		writeResponse(w, Failure("隧道更新失败: "+err.Error()))
		return
	}
	updatedRows, _ := a.store.QueryMaps(r.Context(), "SELECT * FROM tunnel WHERE id=?", id)
	if len(updatedRows) > 0 && tunnelRuntimeChanged(current, updatedRows[0]) {
		forwards, _ := a.store.QueryMaps(r.Context(), "SELECT * FROM forward WHERE tunnel_id=?", id)
		failures := 0
		for _, forward := range forwards {
			if err := a.reprovisionForward(r, forward, current, updatedRows[0]); err != nil {
				failures++
			}
		}
		if failures > 0 {
			writeResponse(w, Failure(fmt.Sprintf("隧道信息更新成功，但 %d 个转发同步更新失败", failures)))
			return
		}
	}
	writeResponse(w, OK("隧道更新成功"))
}

func (a *API) deleteTunnel(w http.ResponseWriter, r *http.Request) {
	id, err := decodeID(w, r)
	if !err {
		return
	}
	forwards, queryErr := a.store.QueryMaps(r.Context(), "SELECT id FROM forward WHERE tunnel_id=? LIMIT 1", id)
	if queryErr != nil {
		writeResponse(w, Failure(queryErr.Error()))
		return
	}
	if len(forwards) > 0 {
		writeResponse(w, Failure("该隧道还有转发在使用，请先删除相关转发"))
		return
	}
	permissions, queryErr := a.store.QueryMaps(r.Context(), "SELECT id FROM user_tunnel WHERE tunnel_id=? LIMIT 1", id)
	if queryErr != nil {
		writeResponse(w, Failure(queryErr.Error()))
		return
	}
	if len(permissions) > 0 {
		writeResponse(w, Failure("该隧道还有用户权限关联，请先取消用户权限分配"))
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
	tunnels, _ := a.store.QueryMaps(r.Context(), "SELECT * FROM tunnel WHERE id=?", tunnelID)
	if len(tunnels) == 0 {
		writeResponse(w, Failure("隧道不存在"))
		return
	}
	if numberOrZero(tunnels[0]["status"]) != 1 {
		writeResponse(w, Failure("隧道已禁用，无法创建转发"))
		return
	}
	limiter := body["speedId"]
	if claims.RoleID != 0 {
		permissionLimiter, accessErr := a.forwardAccess(r, userID, tunnelID, 0)
		if accessErr != nil {
			writeResponse(w, Failure(accessErr.Error()))
			return
		}
		if limiter == nil {
			limiter = permissionLimiter
		}
	}
	if numberOrZero(body["inPort"]) == 0 {
		nodeID, _ := requestInt64(tunnels[0]["inNodeId"])
		port, allocateErr := a.allocateForwardPort(r, nodeID)
		if allocateErr != nil {
			writeResponse(w, Failure(allocateErr.Error()))
			return
		}
		body["inPort"] = port
	}
	if numberOrZero(tunnels[0]["type"]) == 2 {
		outNodeID, _ := requestInt64(tunnels[0]["outNodeId"])
		outPort, allocateErr := a.allocateForwardPort(r, outNodeID)
		if allocateErr != nil {
			writeResponse(w, Failure("隧道出口端口已满，无法分配新端口"))
			return
		}
		body["outPort"] = outPort
	}
	now := time.Now().UnixMilli()
	values := map[string]any{"user_id": userID, "user_name": claims.Name, "name": toString(body["name"]), "tunnel_id": tunnelID,
		"in_port": numberOrZero(body["inPort"]), "out_port": nullableValue(body["outPort"]), "remote_addr": toString(body["remoteAddr"]), "strategy": defaultStringValue(body["strategy"], "fifo"),
		"interface_name": nullableValue(body["interfaceName"]), "speed_id": nullableValue(limiter), "exp_time": nullableValue(body["expTime"]),
		"in_flow": int64(0), "out_flow": int64(0), "inx": int64(0), "status": 1, "created_time": now, "updated_time": now}
	id, err := a.store.InsertMap(r.Context(), "forward", values)
	if err != nil {
		writeResponse(w, Failure("转发创建失败: "+err.Error()))
		return
	}
	if len(tunnels) > 0 {
		tunnel := tunnels[0]
		name := fmt.Sprintf("%d_%d_0", id, userID)
		if provisionErr := a.provisionForward(r, name, int(numberOrZero(body["inPort"])), int(numberOrZero(body["outPort"])), toString(body["remoteAddr"]), limiter, tunnel, body["interfaceName"], defaultStringValue(body["strategy"], "fifo")); provisionErr != nil {
			_ = a.store.DeleteByID(r.Context(), "forward", id)
			writeResponse(w, Failure(provisionErr.Error()))
			return
		}
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
	if !a.authorizeForward(w, r, id) {
		return
	}
	oldRows, queryErr := a.store.QueryMaps(r.Context(), "SELECT * FROM forward WHERE id=?", id)
	if queryErr != nil || len(oldRows) == 0 {
		writeResponse(w, Failure("转发不存在"))
		return
	}
	oldForward := oldRows[0]
	oldTunnelID, _ := requestInt64(oldForward["tunnelId"])
	oldTunnels, _ := a.store.QueryMaps(r.Context(), "SELECT * FROM tunnel WHERE id=?", oldTunnelID)
	newTunnelID := numberOrZero(body["tunnelId"])
	if newTunnelID == 0 {
		newTunnelID = oldTunnelID
	}
	newTunnels, _ := a.store.QueryMaps(r.Context(), "SELECT * FROM tunnel WHERE id=? AND status=1", newTunnelID)
	if len(oldTunnels) == 0 || len(newTunnels) == 0 {
		writeResponse(w, Failure("隧道不存在或已禁用"))
		return
	}
	claims, _ := claimsFrom(r)
	if claims.RoleID != 0 {
		ownerID, _ := requestInt64(oldForward["userId"])
		permissionLimiter, accessErr := a.forwardAccess(r, ownerID, newTunnelID, id)
		if accessErr != nil {
			writeResponse(w, Failure(accessErr.Error()))
			return
		}
		if body["speedId"] == nil {
			body["speedId"] = oldForward["speedId"]
			if body["speedId"] == nil {
				body["speedId"] = permissionLimiter
			}
		}
	}
	values := map[string]any{
		"name": defaultStringValue(body["name"], toString(oldForward["name"])), "tunnel_id": newTunnelID,
		"remote_addr": defaultStringValue(body["remoteAddr"], toString(oldForward["remoteAddr"])),
		"in_port":     valueOr(body, "inPort", oldForward["inPort"]), "strategy": defaultStringValue(body["strategy"], defaultStringValue(oldForward["strategy"], "fifo")),
		"interface_name": valueOr(body, "interfaceName", oldForward["interfaceName"]), "speed_id": valueOr(body, "speedId", oldForward["speedId"]),
		"exp_time": valueOr(body, "expTime", oldForward["expTime"]), "updated_time": time.Now().UnixMilli(),
	}
	if numberOrZero(newTunnels[0]["type"]) == 2 {
		outPort := numberOrZero(oldForward["outPort"])
		if outPort == 0 || oldTunnelID != newTunnelID {
			outNodeID, _ := requestInt64(newTunnels[0]["outNodeId"])
			allocated, allocateErr := a.allocateForwardPort(r, outNodeID)
			if allocateErr != nil {
				writeResponse(w, Failure("隧道出口端口已满，无法分配新端口"))
				return
			}
			outPort = int64(allocated)
		}
		values["out_port"] = outPort
	} else {
		values["out_port"] = nil
	}
	updatedForward := cloneMap(oldForward)
	for key, value := range values {
		updatedForward[camelKey(key)] = value
	}
	if err := a.reprovisionForward(r, updatedForward, oldTunnels[0], newTunnels[0]); err != nil {
		writeResponse(w, Failure("转发节点配置更新失败: "+err.Error()))
		return
	}
	if err := a.store.UpdateMap(r.Context(), "forward", id, values); err != nil {
		_ = a.reprovisionForward(r, oldForward, newTunnels[0], oldTunnels[0])
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
	if !a.authorizeForward(w, r, id) {
		return
	}
	if err := a.deleteForwardRecord(r, id); err != nil {
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
	if !a.authorizeForward(w, r, id) {
		return
	}
	claims, _ := claimsFrom(r)
	if status == 1 && claims.RoleID != 0 {
		rows, _ := a.store.QueryMaps(r.Context(), "SELECT user_id,tunnel_id FROM forward WHERE id=?", id)
		if len(rows) == 0 {
			writeResponse(w, Failure("转发不存在"))
			return
		}
		ownerID, _ := requestInt64(rows[0]["userId"])
		tunnelID, _ := requestInt64(rows[0]["tunnelId"])
		if _, accessErr := a.forwardAccess(r, ownerID, tunnelID, id); accessErr != nil {
			writeResponse(w, Failure(accessErr.Error()))
			return
		}
	}
	command := "ResumeService"
	if status == 0 {
		command = "PauseService"
	}
	a.pauseResumeForwardByID(r, id, command)
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
	values := map[string]any{"name": toString(body["name"]), "speed": numberOrZero(body["speed"]), "tunnel_id": nullableValue(body["tunnelId"]), "tunnel_name": toString(body["tunnelName"]), "mode": numberOrZero(body["mode"]), "total": numberOrZero(body["total"]), "updated_time": time.Now().UnixMilli(), "status": 1}
	var tunnel map[string]any
	if tunnelID := numberOrZero(body["tunnelId"]); tunnelID != 0 {
		rows, err := a.store.QueryMaps(r.Context(), "SELECT * FROM tunnel WHERE id=?", tunnelID)
		if err != nil || len(rows) == 0 {
			writeResponse(w, Failure("指定的隧道不存在"))
			return
		}
		tunnel = rows[0]
		if values["tunnel_name"] != "" && toString(tunnel["name"]) != values["tunnel_name"] {
			writeResponse(w, Failure("隧道名称与隧道ID不匹配"))
			return
		}
		values["tunnel_name"] = tunnel["name"]
	}
	if !update {
		values["created_time"] = time.Now().UnixMilli()
		id, err := a.store.InsertMap(r.Context(), "speed_limit", values)
		if err != nil {
			writeResponse(w, Failure(err.Error()))
			return
		}
		if tunnel != nil {
			if err := a.pushLimiter(r, "AddLimiters", id, tunnel, values); err != nil {
				_ = a.store.DeleteByID(r.Context(), "speed_limit", id)
				writeResponse(w, Failure(err.Error()))
				return
			}
		}
	} else {
		id, err := storeID(body)
		if err != nil {
			writeResponse(w, Failure("限速规则不存在"))
			return
		}
		oldRows, _ := a.store.QueryMaps(r.Context(), "SELECT * FROM speed_limit WHERE id=?", id)
		if len(oldRows) == 0 {
			writeResponse(w, Failure("限速规则不存在"))
			return
		}
		if tunnel != nil {
			if err := a.pushLimiter(r, "UpdateLimiters", id, tunnel, values); err != nil {
				writeResponse(w, Failure(err.Error()))
				return
			}
		}
		if a.store.UpdateMap(r.Context(), "speed_limit", id, values) != nil {
			writeResponse(w, Failure("限速规则更新失败"))
			return
		}
		oldTunnelID := numberOrZero(oldRows[0]["tunnelId"])
		if oldTunnelID != 0 && oldTunnelID != numberOrZero(body["tunnelId"]) {
			oldTunnels, _ := a.store.QueryMaps(r.Context(), "SELECT * FROM tunnel WHERE id=?", oldTunnelID)
			if len(oldTunnels) > 0 {
				a.deleteLimiter(r, id, oldTunnels[0])
			}
		}
	}
	writeResponse(w, OK(nil))
}
func (a *API) deleteSpeedLimit(w http.ResponseWriter, r *http.Request) {
	id, ok := decodeID(w, r)
	if !ok {
		return
	}
	usedByPermissions, err := a.store.QueryMaps(r.Context(), "SELECT id FROM user_tunnel WHERE speed_id=? LIMIT 1", id)
	if err != nil {
		writeResponse(w, Failure(err.Error()))
		return
	}
	usedByForwards, err := a.store.QueryMaps(r.Context(), "SELECT id FROM forward WHERE speed_id=? LIMIT 1", id)
	if err != nil {
		writeResponse(w, Failure(err.Error()))
		return
	}
	if len(usedByPermissions) > 0 || len(usedByForwards) > 0 {
		writeResponse(w, Failure("该限速规则还有用户或转发在使用，请先取消分配"))
		return
	}
	rows, _ := a.store.QueryMaps(r.Context(), "SELECT * FROM speed_limit WHERE id=?", id)
	if len(rows) == 0 {
		writeResponse(w, Failure("限速规则不存在"))
		return
	}
	if tunnelID := numberOrZero(rows[0]["tunnelId"]); tunnelID != 0 {
		tunnels, _ := a.store.QueryMaps(r.Context(), "SELECT * FROM tunnel WHERE id=?", tunnelID)
		if len(tunnels) > 0 {
			a.deleteLimiter(r, id, tunnels[0])
		}
	}
	if err := a.store.DeleteByID(r.Context(), "speed_limit", id); err != nil {
		writeResponse(w, Failure(err.Error()))
		return
	}
	writeResponse(w, OK(nil))
}

func (a *API) pushLimiter(r *http.Request, command string, id int64, tunnel, values map[string]any) error {
	nodeID, err := requestInt64(tunnel["inNodeId"])
	if err != nil {
		return errors.New("隧道入口节点不存在")
	}
	data := limiterData(id, values)
	payload := any(data)
	if command == "UpdateLimiters" {
		payload = map[string]any{"limiter": strconv.FormatInt(id, 10), "data": data}
	}
	result := a.nodeHub.SendCommand(r.Context(), nodeID, command, payload)
	if command == "UpdateLimiters" && strings.Contains(strings.ToLower(result.Msg), "not found") {
		result = a.nodeHub.SendCommand(r.Context(), nodeID, "AddLimiters", data)
	}
	if result.Msg != "OK" {
		return errors.New(result.Msg)
	}
	return nil
}

func (a *API) deleteLimiter(r *http.Request, id int64, tunnel map[string]any) {
	nodeID, err := requestInt64(tunnel["inNodeId"])
	if err == nil {
		_ = a.nodeHub.SendCommand(r.Context(), nodeID, "DeleteLimiters", map[string]any{"limiter": strconv.FormatInt(id, 10)})
	}
}

func limiterData(id int64, values map[string]any) map[string]any {
	speed := numberOrZero(values["speed"])
	rate := fmt.Sprintf("%.1fMB %.1fMB", float64(speed), float64(speed))
	mode := int(numberOrZero(values["mode"]))
	limits := []string{"$ " + rate}
	if mode == 1 {
		limits = []string{"$$ " + rate}
	} else if mode == 2 {
		limits = []string{"0.0.0.0/0 " + rate, "::/0 " + rate}
	}
	if total := numberOrZero(values["total"]); total > 0 && mode != 0 {
		limits = append(limits, fmt.Sprintf("$ %.1fMB %.1fMB", float64(total), float64(total)))
	}
	return map[string]any{"name": strconv.FormatInt(id, 10), "limits": limits}
}

func (a *API) assignUserTunnel(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if !decodeJSON(w, r, &body) {
		return
	}
	userID, userErr := storeIDFrom(body, "userId")
	tunnelID, tunnelErr := storeIDFrom(body, "tunnelId")
	if userErr != nil || tunnelErr != nil {
		writeResponse(w, Failure("用户ID和隧道ID不能为空"))
		return
	}
	existing, err := a.store.QueryMaps(r.Context(), "SELECT id FROM user_tunnel WHERE user_id=? AND tunnel_id=? LIMIT 1", userID, tunnelID)
	if err != nil {
		writeResponse(w, Error(-2, err.Error()))
		return
	}
	if len(existing) > 0 {
		writeResponse(w, Failure("该用户已拥有此隧道权限"))
		return
	}
	_, err = a.store.InsertMap(r.Context(), "user_tunnel", map[string]any{"user_id": userID, "tunnel_id": tunnelID, "speed_id": nullableValue(body["speedId"]),
		"num": numberOrZero(body["num"]), "flow": numberOrZero(body["flow"]), "in_flow": int64(0), "out_flow": int64(0),
		"flow_reset_time": numberOrZero(body["flowResetTime"]), "exp_time": numberOrZero(body["expTime"]), "status": 1})
	if err != nil {
		writeResponse(w, Failure("用户隧道权限分配失败"))
		return
	}
	writeResponse(w, OK("用户隧道权限分配成功"))
}

func (a *API) listUserTunnels(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if !decodeJSON(w, r, &body) {
		return
	}
	userID, err := storeIDFrom(body, "userId")
	if err != nil {
		writeResponse(w, Error(500, "用户ID不能为空"))
		return
	}
	items, err := a.store.QueryMaps(r.Context(), "SELECT ut.*,t.name AS tunnel_name,t.flow AS tunnel_flow,t.in_ip,t.out_ip,t.type,t.protocol,sl.name AS speed_limit_name,sl.speed FROM user_tunnel ut LEFT JOIN tunnel t ON t.id=ut.tunnel_id LEFT JOIN speed_limit sl ON sl.id=ut.speed_id WHERE ut.user_id=? ORDER BY ut.id", userID)
	if err != nil {
		writeResponse(w, Error(-2, err.Error()))
		return
	}
	writeResponse(w, OK(items))
}

func (a *API) currentUserTunnels(w http.ResponseWriter, r *http.Request) {
	userID, ok := currentUserID(w, r)
	if !ok {
		return
	}
	items, err := a.store.QueryMaps(r.Context(), "SELECT ut.*,t.name AS tunnel_name,t.flow AS tunnel_flow,t.in_ip,t.out_ip,t.type,t.protocol,sl.name AS speed_limit_name,sl.speed FROM user_tunnel ut LEFT JOIN tunnel t ON t.id=ut.tunnel_id LEFT JOIN speed_limit sl ON sl.id=ut.speed_id WHERE ut.user_id=? AND ut.status=1 ORDER BY ut.id", userID)
	if err != nil {
		writeResponse(w, Error(-2, err.Error()))
		return
	}
	writeResponse(w, OK(items))
}

func (a *API) removeUserTunnel(w http.ResponseWriter, r *http.Request) {
	id, ok := decodeID(w, r)
	if !ok {
		return
	}
	rows, _ := a.store.QueryMaps(r.Context(), "SELECT user_id,tunnel_id FROM user_tunnel WHERE id=?", id)
	if len(rows) == 0 {
		writeResponse(w, Failure("未找到对应的用户隧道权限记录"))
		return
	}
	userID, _ := requestInt64(rows[0]["userId"])
	tunnelID, _ := requestInt64(rows[0]["tunnelId"])
	forwards, _ := a.store.QueryMaps(r.Context(), "SELECT id FROM forward WHERE user_id=? AND tunnel_id=?", userID, tunnelID)
	for _, forward := range forwards {
		forwardID, _ := requestInt64(forward["id"])
		_ = a.deleteForwardRecord(r, forwardID)
	}
	if err := a.store.DeleteByID(r.Context(), "user_tunnel", id); err != nil {
		writeResponse(w, Failure("未找到对应的用户隧道权限记录"))
		return
	}
	writeResponse(w, OK("用户隧道权限删除成功"))
}

func (a *API) updateUserTunnel(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if !decodeJSON(w, r, &body) {
		return
	}
	id, err := storeID(body)
	if err != nil {
		writeResponse(w, Error(500, "用户隧道权限ID不能为空"))
		return
	}
	values := map[string]any{"flow": numberOrZero(body["flow"]), "num": numberOrZero(body["num"]), "flow_reset_time": numberOrZero(body["flowResetTime"]),
		"exp_time": numberOrZero(body["expTime"]), "status": numberOrZero(body["status"]), "speed_id": nullableValue(body["speedId"])}
	if err := a.store.UpdateMap(r.Context(), "user_tunnel", id, values); err != nil {
		writeResponse(w, Failure("用户隧道权限不存在"))
		return
	}
	writeResponse(w, OK("用户隧道权限更新成功"))
}

func (a *API) updateForwardOrder(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if !decodeJSON(w, r, &body) {
		return
	}
	values, ok := body["forwards"].([]any)
	if !ok || len(values) == 0 {
		writeResponse(w, Failure("forwards参数不能为空"))
		return
	}
	claims, _ := claimsFrom(r)
	userID, _ := claims.UserID()
	for _, value := range values {
		item, ok := value.(map[string]any)
		if !ok {
			continue
		}
		id, err := storeID(item)
		if err != nil {
			continue
		}
		if claims.RoleID != 0 {
			owned, _ := a.store.QueryMaps(r.Context(), "SELECT id FROM forward WHERE id=? AND user_id=?", id, userID)
			if len(owned) == 0 {
				writeResponse(w, Failure("只能更新自己的转发排序"))
				return
			}
		}
		_ = a.store.UpdateMap(r.Context(), "forward", id, map[string]any{"inx": numberOrZero(item["inx"]), "updated_time": time.Now().UnixMilli()})
	}
	writeResponse(w, OK("排序更新成功"))
}

func (a *API) diagnoseForward(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if !decodeJSON(w, r, &body) {
		return
	}
	id, err := storeIDFrom(body, "forwardId")
	if err != nil {
		writeResponse(w, Failure("转发不存在"))
		return
	}
	if !a.authorizeForward(w, r, id) {
		return
	}
	rows, _ := a.store.QueryMaps(r.Context(), "SELECT f.remote_addr,f.name,t.in_node_id,n.name AS node_name FROM forward f JOIN tunnel t ON t.id=f.tunnel_id JOIN node n ON n.id=t.in_node_id WHERE f.id=?", id)
	if len(rows) == 0 {
		writeResponse(w, Failure("转发不存在"))
		return
	}
	host, port := splitTarget(toString(rows[0]["remoteAddr"]))
	nodeID, _ := requestInt64(rows[0]["inNodeId"])
	result := a.tcpPing(r, nodeID, toString(rows[0]["nodeName"]), host, port, "入口->目标")
	writeResponse(w, OK(map[string]any{"forwardId": id, "forwardName": rows[0]["name"], "results": []any{result}, "timestamp": time.Now().UnixMilli()}))
}

func (a *API) diagnoseTunnel(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if !decodeJSON(w, r, &body) {
		return
	}
	id, err := storeIDFrom(body, "tunnelId")
	if err != nil {
		writeResponse(w, Failure("隧道不存在"))
		return
	}
	rows, _ := a.store.QueryMaps(r.Context(), "SELECT t.*,n.name AS in_node_name,o.name AS out_node_name,o.server_ip AS out_server_ip FROM tunnel t JOIN node n ON n.id=t.in_node_id JOIN node o ON o.id=t.out_node_id WHERE t.id=?", id)
	if len(rows) == 0 {
		writeResponse(w, Failure("隧道不存在"))
		return
	}
	tunnel := rows[0]
	inNode, _ := requestInt64(tunnel["inNodeId"])
	tunnelType := numberOrZero(tunnel["type"])
	results := []any{}
	if tunnelType == 1 {
		results = append(results, a.tcpPing(r, inNode, toString(tunnel["inNodeName"]), "www.google.com", 443, "入口->外网"))
	} else {
		results = append(results, a.tcpPing(r, inNode, toString(tunnel["inNodeName"]), toString(tunnel["outServerIp"]), 22, "入口->出口"))
		outNode, _ := requestInt64(tunnel["outNodeId"])
		results = append(results, a.tcpPing(r, outNode, toString(tunnel["outNodeName"]), "www.google.com", 443, "出口->外网"))
	}
	writeResponse(w, OK(map[string]any{"tunnelId": id, "tunnelName": tunnel["name"], "tunnelType": map[bool]string{true: "端口转发", false: "隧道转发"}[tunnelType == 1], "results": results, "timestamp": time.Now().UnixMilli()}))
}

func (a *API) tcpPing(r *http.Request, nodeID int64, nodeName, host string, port int, description string) map[string]any {
	response := a.nodeHub.SendCommand(r.Context(), nodeID, "TcpPing", map[string]any{"ip": host, "port": port, "count": 2, "timeout": 3000})
	result := map[string]any{"nodeId": nodeID, "nodeName": nodeName, "targetIp": host, "targetPort": port, "description": description, "timestamp": time.Now().UnixMilli()}
	data, _ := response.Data.(map[string]any)
	if response.Msg == "OK" && data != nil {
		for key, value := range data {
			result[key] = value
		}
		result["message"] = "TCP连接成功"
	} else {
		result["success"], result["message"], result["averageTime"], result["packetLoss"] = false, response.Msg, -1, 100
	}
	return result
}
func splitTarget(value string) (string, int) {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "[") {
		if end := strings.Index(value, "]:"); end > 0 {
			port, _ := strconv.Atoi(value[end+2:])
			return value[1:end], port
		}
	}
	index := strings.LastIndex(value, ":")
	if index < 0 {
		return value, -1
	}
	port, _ := strconv.Atoi(value[index+1:])
	return value[:index], port
}

func buildGostServicesWithTunnel(name string, port int, remote string, limiter any, tunnel map[string]any, interfaceName any, strategy string) []map[string]any {
	if port == 0 {
		port = 20000
	}
	services := []map[string]any{}
	for _, protocol := range []string{"tcp", "udp"} {
		listen := toString(tunnel["tcpListenAddr"])
		if protocol == "udp" {
			listen = toString(tunnel["udpListenAddr"])
		}
		if listen == "" {
			listen = "0.0.0.0"
		}
		handler := map[string]any{"type": protocol}
		if numberOrZero(tunnel["type"]) == 2 {
			handler["chain"] = name + "_chains"
		}
		service := map[string]any{"name": name + "_" + protocol, "addr": fmt.Sprintf("%s:%d", listen, port), "handler": handler, "listener": map[string]any{"type": protocol}}
		if limiter != nil {
			service["limiter"] = fmt.Sprint(limiter)
		}
		if interfaceName != nil {
			service["metadata"] = map[string]any{"interface": interfaceName}
		}
		if numberOrZero(tunnel["type"]) == 1 {
			nodes := []map[string]any{}
			for index, address := range strings.Split(remote, ",") {
				nodes = append(nodes, map[string]any{"name": fmt.Sprintf("node_%d", index+1), "addr": strings.TrimSpace(address)})
			}
			service["forwarder"] = map[string]any{"nodes": nodes, "selector": map[string]any{"strategy": defaultStringValue(strategy, "fifo"), "maxFails": 3, "failTimeout": "10s"}}
		}
		services = append(services, service)
	}
	return services
}

func (a *API) provisionForward(r *http.Request, name string, inPort, outPort int, remote string, limiter any, tunnel map[string]any, interfaceName any, strategy string) error {
	inNodeID, _ := requestInt64(tunnel["inNodeId"])
	if numberOrZero(tunnel["type"]) == 2 {
		outNodeID, _ := requestInt64(tunnel["outNodeId"])
		outIP := toString(tunnel["outIp"])
		if strings.Contains(outIP, ":") && !strings.HasPrefix(outIP, "[") {
			outIP = "[" + outIP + "]"
		}
		protocol := defaultStringValue(tunnel["protocol"], "tls")
		dialer := map[string]any{"type": protocol}
		if protocol == "quic" {
			dialer["metadata"] = map[string]any{"keepAlive": true, "ttl": "10s"}
		}
		node := map[string]any{"name": "node-" + name, "addr": fmt.Sprintf("%s:%d", outIP, outPort), "connector": map[string]any{"type": "relay"}, "dialer": dialer}
		if tunnel["interfaceName"] != nil {
			node["interface"] = tunnel["interfaceName"]
		}
		chain := map[string]any{"name": name + "_chains", "hops": []map[string]any{{"name": "hop-" + name, "nodes": []map[string]any{node}}}}
		if result := a.nodeHub.SendCommand(r.Context(), inNodeID, "AddChains", chain); result.Msg != "OK" {
			return errors.New(result.Msg)
		}
		remoteNodes := []map[string]any{}
		for index, address := range strings.Split(remote, ",") {
			remoteNodes = append(remoteNodes, map[string]any{"name": fmt.Sprintf("node_%d", index+1), "addr": strings.TrimSpace(address)})
		}
		remoteService := map[string]any{"name": name + "_tls", "addr": fmt.Sprintf(":%d", outPort), "handler": map[string]any{"type": "relay"}, "listener": map[string]any{"type": protocol}, "forwarder": map[string]any{"nodes": remoteNodes, "selector": map[string]any{"strategy": "fifo", "maxFails": 3, "failTimeout": "10s"}}}
		if interfaceName != nil {
			remoteService["metadata"] = map[string]any{"interface": interfaceName}
		}
		if result := a.nodeHub.SendCommand(r.Context(), outNodeID, "AddService", []map[string]any{remoteService}); result.Msg != "OK" {
			_ = a.nodeHub.SendCommand(r.Context(), inNodeID, "DeleteChains", map[string]any{"chain": name + "_chains"})
			return errors.New(result.Msg)
		}
	}
	services := buildGostServicesWithTunnel(name, inPort, remote, limiter, tunnel, interfaceName, strategy)
	if result := a.nodeHub.SendCommand(r.Context(), inNodeID, "AddService", services); result.Msg != "OK" {
		_ = a.cleanupForwardCommands(r, name, tunnel)
		return errors.New(result.Msg)
	}
	return nil
}

func (a *API) reprovisionForward(r *http.Request, forward, oldTunnel, newTunnel map[string]any) error {
	id, _ := requestInt64(forward["id"])
	userID, _ := requestInt64(forward["userId"])
	name := fmt.Sprintf("%d_%d_0", id, userID)
	_ = a.cleanupForwardCommands(r, name, oldTunnel)
	err := a.provisionForward(r, name, int(numberOrZero(forward["inPort"])), int(numberOrZero(forward["outPort"])), toString(forward["remoteAddr"]), forward["speedId"], newTunnel, forward["interfaceName"], defaultStringValue(forward["strategy"], "fifo"))
	if err != nil {
		_ = a.provisionForward(r, name, int(numberOrZero(forward["inPort"])), int(numberOrZero(forward["outPort"])), toString(forward["remoteAddr"]), forward["speedId"], oldTunnel, forward["interfaceName"], defaultStringValue(forward["strategy"], "fifo"))
		return err
	}
	if numberOrZero(forward["status"]) == 0 {
		inNodeID, _ := requestInt64(newTunnel["inNodeId"])
		_ = a.nodeHub.SendCommand(r.Context(), inNodeID, "PauseService", map[string]any{"services": []string{name + "_tcp", name + "_udp"}})
		if numberOrZero(newTunnel["type"]) == 2 {
			outNodeID, _ := requestInt64(newTunnel["outNodeId"])
			_ = a.nodeHub.SendCommand(r.Context(), outNodeID, "PauseService", map[string]any{"services": []string{name + "_tls"}})
		}
	}
	return nil
}

func tunnelRuntimeChanged(oldTunnel, newTunnel map[string]any) bool {
	for _, key := range []string{"protocol", "tcpListenAddr", "udpListenAddr", "interfaceName"} {
		if toString(oldTunnel[key]) != toString(newTunnel[key]) {
			return true
		}
	}
	return false
}

func valueOr(values map[string]any, key string, fallback any) any {
	if value, ok := values[key]; ok {
		return nullableValue(value)
	}
	return fallback
}

func (a *API) forwardAccess(r *http.Request, userID, tunnelID, excludeForwardID int64) (any, error) {
	user, err := a.store.UserByID(r.Context(), userID)
	if err != nil || user.Status != 1 {
		return nil, errors.New("用户不存在或已禁用")
	}
	now := time.Now().UnixMilli()
	if user.ExpiryTime > 0 && user.ExpiryTime <= now {
		return nil, errors.New("当前账号已到期")
	}
	const gib = int64(1024 * 1024 * 1024)
	if user.Flow <= 0 || user.InboundFlow+user.OutboundFlow >= user.Flow*gib {
		return nil, errors.New("用户总流量已用完")
	}
	subscriptions, queryErr := a.store.QueryMaps(r.Context(), "SELECT status,expires_at,traffic_limit_bytes,traffic_used_bytes,max_forwards FROM user_subscription WHERE user_id=? ORDER BY id DESC LIMIT 1", userID)
	if queryErr != nil {
		return nil, queryErr
	}
	if len(subscriptions) > 0 {
		subscription := subscriptions[0]
		if numberOrZero(subscription["status"]) != 1 || (numberOrZero(subscription["expiresAt"]) > 0 && numberOrZero(subscription["expiresAt"]) <= now) {
			return nil, errors.New("套餐已到期，无法创建或恢复转发")
		}
		limit := numberOrZero(subscription["trafficLimitBytes"])
		if limit > 0 && numberOrZero(subscription["trafficUsedBytes"]) >= limit {
			return nil, errors.New("套餐流量已用尽，无法创建或恢复转发")
		}
		if maxForwards := numberOrZero(subscription["maxForwards"]); maxForwards > 0 {
			count, err := a.forwardCount(r, userID, 0, excludeForwardID)
			if err != nil {
				return nil, err
			}
			if count >= maxForwards {
				return nil, errors.New("已达到套餐转发数量上限")
			}
		}
	}
	permissions, queryErr := a.store.QueryMaps(r.Context(), "SELECT status,exp_time,flow,in_flow,out_flow,num,speed_id FROM user_tunnel WHERE user_id=? AND tunnel_id=? LIMIT 1", userID, tunnelID)
	if queryErr != nil {
		return nil, queryErr
	}
	if len(permissions) == 0 {
		return nil, errors.New("你没有该隧道权限")
	}
	permission := permissions[0]
	if numberOrZero(permission["status"]) != 1 {
		return nil, errors.New("隧道权限已禁用")
	}
	if expiry := numberOrZero(permission["expTime"]); expiry > 0 && expiry <= now {
		return nil, errors.New("该隧道权限已到期")
	}
	flow := numberOrZero(permission["flow"])
	if flow <= 0 || numberOrZero(permission["inFlow"])+numberOrZero(permission["outFlow"]) >= flow*gib {
		return nil, errors.New("该隧道流量已用完")
	}
	count, err := a.forwardCount(r, userID, tunnelID, excludeForwardID)
	if err != nil {
		return nil, err
	}
	if limit := numberOrZero(permission["num"]); count >= limit {
		return nil, fmt.Errorf("该隧道转发数量已达上限，当前限制：%d个", limit)
	}
	count, err = a.forwardCount(r, userID, 0, excludeForwardID)
	if err != nil {
		return nil, err
	}
	if count >= int64(user.ForwardLimit) {
		return nil, fmt.Errorf("用户总转发数量已达上限，当前限制：%d个", user.ForwardLimit)
	}
	return permission["speedId"], nil
}

func (a *API) forwardCount(r *http.Request, userID, tunnelID, excludeID int64) (int64, error) {
	query := "SELECT COUNT(*) AS count FROM forward WHERE user_id=? AND status<>-1"
	args := []any{userID}
	if tunnelID > 0 {
		query += " AND tunnel_id=?"
		args = append(args, tunnelID)
	}
	if excludeID > 0 {
		query += " AND id<>?"
		args = append(args, excludeID)
	}
	rows, err := a.store.QueryMaps(r.Context(), query, args...)
	if err != nil || len(rows) == 0 {
		return 0, err
	}
	return requestInt64(rows[0]["count"])
}

func camelKey(value string) string {
	parts := strings.Split(value, "_")
	for index := 1; index < len(parts); index++ {
		if parts[index] != "" {
			parts[index] = strings.ToUpper(parts[index][:1]) + parts[index][1:]
		}
	}
	return strings.Join(parts, "")
}

func (a *API) cleanupForwardCommands(r *http.Request, name string, tunnel map[string]any) error {
	inNodeID, _ := requestInt64(tunnel["inNodeId"])
	_ = a.nodeHub.SendCommand(r.Context(), inNodeID, "DeleteService", map[string]any{"services": []string{name + "_tcp", name + "_udp"}})
	if numberOrZero(tunnel["type"]) == 2 {
		outNodeID, _ := requestInt64(tunnel["outNodeId"])
		_ = a.nodeHub.SendCommand(r.Context(), inNodeID, "DeleteChains", map[string]any{"chain": name + "_chains"})
		_ = a.nodeHub.SendCommand(r.Context(), outNodeID, "DeleteService", map[string]any{"services": []string{name + "_tls"}})
	}
	return nil
}

func (a *API) authorizeForward(w http.ResponseWriter, r *http.Request, id int64) bool {
	claims, ok := claimsFrom(r)
	if !ok {
		writeResponse(w, Error(401, "未登录或token已过期"))
		return false
	}
	if claims.RoleID == 0 {
		return true
	}
	userID, err := claims.UserID()
	if err != nil {
		writeResponse(w, Error(401, "无效的token或token已过期"))
		return false
	}
	rows, err := a.store.QueryMaps(r.Context(), "SELECT id FROM forward WHERE id=? AND user_id=?", id, userID)
	if err != nil || len(rows) == 0 {
		writeResponse(w, Error(403, "只能操作自己的转发"))
		return false
	}
	return true
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
