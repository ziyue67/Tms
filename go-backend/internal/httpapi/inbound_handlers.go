package httpapi

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/ziyue67/tms/go-backend/internal/sharelink"
	"github.com/ziyue67/tms/go-backend/internal/store"
)

var validSNI = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9-]*[A-Za-z0-9])?(\.[A-Za-z0-9]([A-Za-z0-9-]*[A-Za-z0-9])?)+$`)

func (a *API) createInbound(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if !decodeJSON(w, r, &body) {
		return
	}
	item, err := a.createInboundRecord(r, body)
	if err != nil {
		writeResponse(w, Failure(err.Error()))
		return
	}
	if err := a.pushSingbox(r, numberOrZero(body["nodeId"])); err != nil {
		_ = a.store.DeleteByID(r.Context(), "inbound", numberOrZero(item["id"]))
		writeResponse(w, Failure(err.Error()))
		return
	}
	writeResponse(w, OK(item))
}

func (a *API) oneClickInbound(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if !decodeJSON(w, r, &body) {
		return
	}
	nodeID, err := storeIDFrom(body, "nodeId")
	if err != nil {
		writeResponse(w, Failure("节点不存在"))
		return
	}
	created, err := a.createProtocolSet(r, nodeID, nil, toString(body["sni"]))
	if err != nil {
		writeResponse(w, Failure(err.Error()))
		return
	}
	writeResponse(w, OK(created))
}

func (a *API) oneClickRelay(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if !decodeJSON(w, r, &body) {
		return
	}
	nodeID, err := storeIDFrom(body, "nodeId")
	if err != nil {
		writeResponse(w, Failure("前置机不存在"))
		return
	}
	parsed, err := sharelink.Parse(toString(body["link"]))
	if err != nil {
		writeResponse(w, Failure("落地解析失败:"+err.Error()))
		return
	}
	name := toString(body["name"])
	if name == "" {
		name = "落地-" + fmt.Sprint(nodeID)
	}
	outbound, _ := json.Marshal(parsed.Outbound)
	now := time.Now().UnixMilli()
	landingID, err := a.store.InsertMap(r.Context(), "landing", map[string]any{"name": name, "type": parsed.Protocol, "link": parsed.Raw, "outbound_json": string(outbound), "status": 1, "created_time": now, "updated_time": now})
	if err != nil {
		writeResponse(w, Failure("落地解析失败:"+err.Error()))
		return
	}
	created, err := a.createProtocolSet(r, nodeID, &landingID, toString(body["sni"]))
	if err != nil {
		writeResponse(w, Failure(err.Error()))
		return
	}
	writeResponse(w, OK(created))
}

func (a *API) createProtocolSet(r *http.Request, nodeID int64, landingID *int64, sni string) ([]map[string]any, error) {
	created := []map[string]any{}
	for _, protocol := range []string{"vless", "trojan", "vmess", "hysteria2", "tuic", "anytls"} {
		body := map[string]any{"nodeId": nodeID, "protocol": protocol, "sni": sni}
		if landingID != nil {
			body["landingId"] = *landingID
		}
		item, err := a.createInboundRecord(r, body)
		if err != nil {
			return nil, fmt.Errorf("一键添加中断(%s):%w(已成功 %d 个)", protocol, err, len(created))
		}
		created = append(created, item)
	}
	if err := a.pushSingbox(r, nodeID); err != nil {
		return nil, fmt.Errorf("协议已入库,但下发 sing-box 配置失败:%w", err)
	}
	return created, nil
}

func (a *API) createInboundRecord(r *http.Request, body map[string]any) (map[string]any, error) {
	nodeID, err := storeIDFrom(body, "nodeId")
	if err != nil {
		return nil, errors.New("节点不存在")
	}
	if _, err := a.store.NodeByID(r.Context(), nodeID); err != nil {
		return nil, errors.New("节点不存在")
	}
	protocol := strings.ToLower(defaultStringValue(body["protocol"], "shadowsocks"))
	supported := map[string]bool{"shadowsocks": true, "vless": true, "trojan": true, "vmess": true, "hysteria2": true, "tuic": true, "anytls": true}
	if !supported[protocol] {
		return nil, fmt.Errorf("暂不支持的协议:%s", protocol)
	}
	listen := int(numberOrZero(body["listenPort"]))
	if listen == 0 {
		rows, _ := a.store.QueryMaps(r.Context(), "SELECT MAX(listen_port) AS max_port FROM inbound WHERE node_id=?", nodeID)
		listen = 41000
		if len(rows) > 0 {
			if value, _ := requestInt64(rows[0]["maxPort"]); value >= 41000 {
				listen = int(value + 1)
			}
		}
	}
	now := time.Now().UnixMilli()
	values := map[string]any{"node_id": nodeID, "landing_id": nullableValue(body["landingId"]), "tag": fmt.Sprintf("in-%d-%d", nodeID, listen), "protocol": protocol, "listen_port": listen, "remark": nullableValue(body["remark"]), "status": 1, "created_time": now, "updated_time": now}
	sni := sanitizeSNI(toString(body["sni"]))
	switch protocol {
	case "shadowsocks":
		key := make([]byte, 32)
		_, _ = rand.Read(key)
		cfg, _ := json.Marshal(map[string]any{"method": "2022-blake3-aes-256-gcm", "password": base64.StdEncoding.EncodeToString(key)})
		values["security"], values["config_json"] = "none", string(cfg)
	case "vmess":
		values["security"] = "none"
	case "hysteria2", "tuic", "anytls":
		if sni == "" {
			sni = "www.bing.com"
		}
		values["security"], values["sni"] = "tls", sni
	case "vless", "trojan":
		if sni == "" {
			sni = "www.apple.com"
		}
		result := a.nodeHub.SendCommand(r.Context(), nodeID, "GenerateRealityKeypair", map[string]any{})
		if result.Msg != "OK" {
			return nil, fmt.Errorf("生成 Reality 密钥失败:%s", result.Msg)
		}
		data, _ := result.Data.(map[string]any)
		if data == nil {
			return nil, errors.New("Reality 密钥解析失败")
		}
		short := make([]byte, 4)
		_, _ = rand.Read(short)
		values["security"], values["sni"], values["dest"], values["public_key"], values["private_key"], values["short_id"] = "reality", sni, firstNonEmpty(toString(body["dest"]), sni), data["publicKey"], data["privateKey"], hex.EncodeToString(short)
	}
	id, err := a.store.InsertMap(r.Context(), "inbound", values)
	if err != nil {
		return nil, err
	}
	values["id"] = id
	return camelMap(values), nil
}

func (a *API) listInbounds(w http.ResponseWriter, r *http.Request) {
	items, err := a.store.QueryMaps(r.Context(), "SELECT i.*,n.name AS node_name,l.name AS landing_name FROM inbound i LEFT JOIN node n ON n.id=i.node_id LEFT JOIN landing l ON l.id=i.landing_id ORDER BY i.id")
	if err != nil {
		writeResponse(w, Error(-2, err.Error()))
		return
	}
	writeResponse(w, OK(items))
}

func (a *API) deleteInbound(w http.ResponseWriter, r *http.Request) {
	id, ok := decodeID(w, r)
	if !ok {
		return
	}
	if err := a.deleteInboundRecord(r, id); err != nil {
		writeResponse(w, Failure(err.Error()))
		return
	}
	writeResponse(w, OK(nil))
}
func (a *API) deleteInboundRecord(r *http.Request, id int64) error {
	rows, err := a.store.QueryMaps(r.Context(), "SELECT node_id FROM inbound WHERE id=?", id)
	if err != nil || len(rows) == 0 {
		return errors.New("入站不存在")
	}
	nodeID, _ := requestInt64(rows[0]["nodeId"])
	users, _ := a.store.QueryMaps(r.Context(), "SELECT id,gost_forward_id,user_id FROM inbound_user WHERE inbound_id=?", id)
	for _, user := range users {
		if forwardID, err := requestInt64(user["gostForwardId"]); err == nil && forwardID > 0 {
			_ = a.deleteForwardRecord(r, forwardID)
		}
		uid, _ := requestInt64(user["id"])
		_ = a.store.DeleteByID(r.Context(), "inbound_user", uid)
	}
	if err := a.store.DeleteByID(r.Context(), "inbound", id); err != nil {
		return err
	}
	return a.pushSingbox(r, nodeID)
}
func (a *API) deleteInboundsByNode(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if !decodeJSON(w, r, &body) {
		return
	}
	nodeID, _ := storeIDFrom(body, "nodeId")
	query := "SELECT id FROM inbound WHERE node_id=?"
	args := []any{nodeID}
	relay, _ := body["relay"].(bool)
	if relay {
		if body["landingId"] != nil {
			query += " AND landing_id=?"
			args = append(args, numberOrZero(body["landingId"]))
		} else {
			query += " AND landing_id IS NOT NULL"
		}
	} else {
		query += " AND landing_id IS NULL"
	}
	rows, _ := a.store.QueryMaps(r.Context(), query, args...)
	for _, row := range rows {
		id, _ := requestInt64(row["id"])
		_ = a.deleteInboundRecord(r, id)
	}
	writeResponse(w, OK(nil))
}

func (a *API) assignInbound(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if !decodeJSON(w, r, &body) {
		return
	}
	result, err := a.assignOneInbound(r, body)
	if err != nil {
		writeResponse(w, Failure(err.Error()))
		return
	}
	writeResponse(w, OK(result))
}
func (a *API) assignAllInbounds(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if !decodeJSON(w, r, &body) {
		return
	}
	result, err := a.assignInboundGroup(r, body)
	if err != nil {
		writeResponse(w, Failure(err.Error()))
		return
	}
	writeResponse(w, OK(result))
}
func (a *API) assignSelfInbounds(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if !decodeJSON(w, r, &body) {
		return
	}
	claims, _ := claimsFrom(r)
	uid, _ := claims.UserID()
	body["userId"], body["speedId"], body["expTime"], body["flow"] = uid, nil, nil, nil
	result, err := a.assignInboundGroup(r, body)
	if err != nil {
		writeResponse(w, Failure(err.Error()))
		return
	}
	writeResponse(w, OK(result))
}

func (a *API) assignInboundGroup(r *http.Request, body map[string]any) (map[string]any, error) {
	_, err := storeIDFrom(body, "userId")
	if err != nil {
		return nil, errors.New("用户不能为空")
	}
	query := "SELECT id FROM inbound WHERE 1=1"
	args := []any{}
	if body["nodeId"] != nil {
		query += " AND node_id=?"
		args = append(args, numberOrZero(body["nodeId"]))
	}
	relay, _ := body["relay"].(bool)
	if relay {
		query += " AND landing_id=?"
		args = append(args, numberOrZero(body["landingId"]))
	} else if body["nodeId"] != nil {
		query += " AND landing_id IS NULL"
	}
	rows, err := a.store.QueryMaps(r.Context(), query, args...)
	if err != nil || len(rows) == 0 {
		return nil, errors.New("没有可分配的协议")
	}
	body["_limiterCache"] = map[int64]any{}
	assigned, skipped, updated := 0, 0, 0
	var token string
	for _, row := range rows {
		copyBody := cloneMap(body)
		copyBody["inboundId"] = row["id"]
		result, assignErr := a.assignOneInbound(r, copyBody)
		if assignErr != nil {
			skipped++
			continue
		}
		if existing, _ := result["existing"].(bool); existing {
			if changed, _ := result["updated"].(bool); changed {
				updated++
			} else {
				skipped++
			}
		} else {
			assigned++
		}
		token = toString(result["subToken"])
	}
	return map[string]any{"subToken": token, "assigned": assigned, "skipped": skipped, "updated": updated}, nil
}

func (a *API) assignOneInbound(r *http.Request, body map[string]any) (map[string]any, error) {
	inboundID, err := storeIDFrom(body, "inboundId")
	if err != nil {
		return nil, errors.New("入站不存在")
	}
	userID, err := storeIDFrom(body, "userId")
	if err != nil {
		return nil, errors.New("用户不存在")
	}
	inbounds, err := a.store.QueryMaps(r.Context(), "SELECT * FROM inbound WHERE id=?", inboundID)
	if err != nil || len(inbounds) == 0 {
		return nil, errors.New("入站不存在")
	}
	user, err := a.store.UserByID(r.Context(), userID)
	if err != nil {
		return nil, errors.New("用户不存在")
	}
	inbound := inbounds[0]
	nodeID, _ := requestInt64(inbound["nodeId"])
	token, err := a.ensureInboundLine(r, userID, nodeID, inbound["landingId"], body["flow"], body["expTime"])
	if err != nil {
		return nil, err
	}
	limiter, err := a.ensureUserLimiter(r, nodeID, userID, body)
	if err != nil {
		return nil, err
	}
	existing, _ := a.store.QueryMaps(r.Context(), "SELECT id,gost_forward_id,sub_token FROM inbound_user WHERE inbound_id=? AND user_id=? LIMIT 1", inboundID, userID)
	if len(existing) > 0 {
		changed := false
		if forwardID, forwardErr := requestInt64(existing[0]["gostForwardId"]); forwardErr == nil && forwardID > 0 && (limiter != nil || body["expTime"] != nil) {
			forwards, _ := a.store.QueryMaps(r.Context(), "SELECT * FROM forward WHERE id=?", forwardID)
			if len(forwards) > 0 {
				tunnelID, _ := requestInt64(forwards[0]["tunnelId"])
				tunnels, _ := a.store.QueryMaps(r.Context(), "SELECT * FROM tunnel WHERE id=?", tunnelID)
				if len(tunnels) > 0 {
					updatedForward := cloneMap(forwards[0])
					values := map[string]any{"updated_time": time.Now().UnixMilli()}
					if limiter != nil {
						updatedForward["speedId"], values["speed_id"] = limiter, limiter
					}
					if body["expTime"] != nil {
						updatedForward["expTime"], values["exp_time"] = body["expTime"], body["expTime"]
					}
					if err := a.reprovisionForward(r, updatedForward, tunnels[0], tunnels[0]); err != nil {
						return nil, err
					}
					if err := a.store.UpdateMap(r.Context(), "forward", forwardID, values); err != nil {
						return nil, err
					}
					changed = true
				}
			}
		}
		return map[string]any{"inboundUserId": existing[0]["id"], "subToken": token, "existing": true, "updated": changed}, nil
	}
	tunnelID, err := a.ensureInboundTunnel(r, nodeID)
	if err != nil {
		return nil, err
	}
	port, err := a.allocateForwardPort(r, nodeID)
	if err != nil {
		return nil, err
	}
	uuid := randomUUID()
	password := strings.ReplaceAll(randomUUID(), "-", "")
	now := time.Now().UnixMilli()
	forwardID, err := a.store.InsertMap(r.Context(), "forward", map[string]any{"user_id": userID, "user_name": user.Username, "name": fmt.Sprintf("inbound-%d-user-%d", inboundID, userID), "tunnel_id": tunnelID, "in_port": port, "remote_addr": fmt.Sprintf("127.0.0.1:%v", inbound["listenPort"]), "strategy": "fifo", "in_flow": int64(0), "out_flow": int64(0), "inx": 0, "speed_id": limiter, "exp_time": nullableValue(body["expTime"]), "created_time": now, "updated_time": now, "status": 1})
	if err != nil {
		return nil, err
	}
	serviceName := fmt.Sprintf("%d_%d_0", forwardID, userID)
	services := buildGostServices(serviceName, port, fmt.Sprintf("127.0.0.1:%v", inbound["listenPort"]), limiter)
	result := a.nodeHub.SendCommand(r.Context(), nodeID, "AddService", services)
	if result.Msg != "OK" {
		_ = a.store.DeleteByID(r.Context(), "forward", forwardID)
		return nil, errors.New(result.Msg)
	}
	iuID, err := a.store.InsertMap(r.Context(), "inbound_user", map[string]any{"inbound_id": inboundID, "user_id": userID, "uuid": uuid, "password": password, "gost_forward_id": forwardID, "sub_token": token, "status": 1, "created_time": now})
	if err != nil {
		_ = a.deleteForwardRecord(r, forwardID)
		return nil, err
	}
	if err := a.pushSingbox(r, nodeID); err != nil {
		_ = a.store.DeleteByID(r.Context(), "inbound_user", iuID)
		_ = a.deleteForwardRecord(r, forwardID)
		return nil, err
	}
	node, _ := a.store.NodeByID(r.Context(), nodeID)
	server := node.ServerIP
	if node.Domain.Valid && strings.TrimSpace(node.Domain.String) != "" {
		server = strings.TrimSpace(node.Domain.String)
	}
	return map[string]any{"inboundUserId": iuID, "subToken": token, "link": clientLinkFromMaps(inbound, server, port, uuid, password)}, nil
}

func (a *API) ensureUserLimiter(r *http.Request, nodeID, userID int64, body map[string]any) (any, error) {
	ruleID := numberOrZero(body["speedId"])
	if ruleID == 0 {
		return nil, nil
	}
	limiterName := int64(900000000) + userID
	if cache, ok := body["_limiterCache"].(map[int64]any); ok {
		if cached, exists := cache[nodeID]; exists {
			return cached, nil
		}
	}
	rules, err := a.store.QueryMaps(r.Context(), "SELECT * FROM speed_limit WHERE id=?", ruleID)
	if err != nil || len(rules) == 0 {
		return nil, errors.New("限速规则不存在")
	}
	data := limiterData(limiterName, rules[0])
	result := a.nodeHub.SendCommand(r.Context(), nodeID, "AddLimiters", data)
	if strings.Contains(strings.ToLower(result.Msg), "already exists") {
		result = a.nodeHub.SendCommand(r.Context(), nodeID, "UpdateLimiters", map[string]any{"limiter": fmt.Sprint(limiterName), "data": data})
	}
	if result.Msg != "OK" {
		return nil, fmt.Errorf("下发限速器失败:%s", result.Msg)
	}
	if cache, ok := body["_limiterCache"].(map[int64]any); ok {
		cache[nodeID] = limiterName
	}
	return limiterName, nil
}

func (a *API) ensureInboundTunnel(r *http.Request, nodeID int64) (int64, error) {
	rows, err := a.store.QueryMaps(r.Context(), "SELECT id FROM tunnel WHERE in_node_id=? AND type=1 ORDER BY id DESC LIMIT 1", nodeID)
	if err == nil && len(rows) > 0 {
		return requestInt64(rows[0]["id"])
	}
	node, err := a.store.NodeByID(r.Context(), nodeID)
	if err != nil {
		return 0, err
	}
	now := time.Now().UnixMilli()
	return a.store.InsertMap(r.Context(), "tunnel", map[string]any{"name": fmt.Sprintf("inbound-tunnel-node%d", nodeID), "traffic_ratio": 1, "in_node_id": nodeID, "in_ip": node.IP, "out_node_id": nodeID, "out_ip": node.ServerIP, "type": 1, "protocol": "tls", "flow": 1, "tcp_listen_addr": "0.0.0.0", "udp_listen_addr": "0.0.0.0", "created_time": now, "updated_time": now, "status": 1})
}
func (a *API) ensureInboundLine(r *http.Request, userID, nodeID int64, landing, flow, expiry any) (string, error) {
	query := "SELECT id,sub_token FROM inbound_line WHERE user_id=? AND node_id=?"
	args := []any{userID, nodeID}
	if landing == nil {
		query += " AND landing_id IS NULL"
	} else {
		query += " AND landing_id=?"
		args = append(args, landing)
	}
	query += " LIMIT 1"
	rows, err := a.store.QueryMaps(r.Context(), query, args...)
	if err != nil {
		return "", err
	}
	if len(rows) > 0 {
		token := toString(rows[0]["subToken"])
		if token == "" {
			token = randomToken()
		}
		id, _ := requestInt64(rows[0]["id"])
		values := map[string]any{"sub_token": token, "status": 1, "updated_time": time.Now().UnixMilli()}
		if flow != nil {
			values["flow"] = flow
		}
		if expiry != nil {
			values["exp_time"] = expiry
		}
		return token, a.store.UpdateMap(r.Context(), "inbound_line", id, values)
	}
	token := randomToken()
	now := time.Now().UnixMilli()
	_, err = a.store.InsertMap(r.Context(), "inbound_line", map[string]any{"user_id": userID, "node_id": nodeID, "landing_id": landing, "sub_token": token, "flow": flow, "exp_time": expiry, "status": 1, "created_time": now, "updated_time": now})
	return token, err
}
func (a *API) allocateForwardPort(r *http.Request, nodeID int64) (int, error) {
	node, err := a.store.NodeByID(r.Context(), nodeID)
	if err != nil {
		return 0, err
	}
	start, end := node.PortStart, node.PortEnd
	if start <= 39999 && end >= 20000 {
		start = max(start, 20000)
		end = min(end, 39999)
	}
	usedRows, _ := a.store.QueryMaps(r.Context(), "SELECT f.in_port AS port FROM forward f JOIN tunnel t ON t.id=f.tunnel_id WHERE t.in_node_id=? UNION ALL SELECT f.out_port AS port FROM forward f JOIN tunnel t ON t.id=f.tunnel_id WHERE t.out_node_id=? AND f.out_port IS NOT NULL", nodeID, nodeID)
	used := map[int64]bool{}
	for _, row := range usedRows {
		value, _ := requestInt64(row["port"])
		used[value] = true
	}
	for port := start; port <= end; port++ {
		if !used[int64(port)] {
			return port, nil
		}
	}
	return 0, errors.New("没有可用端口")
}
func buildGostServices(name string, port int, remote string, limiter any) []map[string]any {
	result := []map[string]any{}
	for _, protocol := range []string{"tcp", "udp"} {
		service := map[string]any{"name": name + "_" + protocol, "addr": fmt.Sprintf("0.0.0.0:%d", port), "handler": map[string]any{"type": protocol}, "listener": map[string]any{"type": protocol}, "forwarder": map[string]any{"nodes": []map[string]any{{"name": "node_1", "addr": remote}}, "selector": map[string]any{"strategy": "fifo", "maxFails": 3, "failTimeout": "10s"}}}
		if limiter != nil {
			service["limiter"] = fmt.Sprint(limiter)
		}
		if protocol == "udp" {
			service["listener"].(map[string]any)["metadata"] = map[string]any{"keepAlive": true}
		}
		result = append(result, service)
	}
	return result
}

func (a *API) unassignInbound(w http.ResponseWriter, r *http.Request) {
	id, ok := decodeID(w, r)
	if !ok {
		return
	}
	rows, _ := a.store.QueryMaps(r.Context(), "SELECT gost_forward_id,inbound_id FROM inbound_user WHERE id=?", id)
	if len(rows) == 0 {
		writeResponse(w, Failure("记录不存在"))
		return
	}
	forwardID, _ := requestInt64(rows[0]["gostForwardId"])
	inboundID, _ := requestInt64(rows[0]["inboundId"])
	_ = a.deleteForwardRecord(r, forwardID)
	_ = a.store.DeleteByID(r.Context(), "inbound_user", id)
	inbounds, _ := a.store.QueryMaps(r.Context(), "SELECT node_id FROM inbound WHERE id=?", inboundID)
	if len(inbounds) > 0 {
		nodeID, _ := requestInt64(inbounds[0]["nodeId"])
		_ = a.pushSingbox(r, nodeID)
	}
	writeResponse(w, OK(nil))
}
func (a *API) deleteForwardRecord(r *http.Request, id int64) error {
	rows, _ := a.store.QueryMaps(r.Context(), "SELECT f.user_id,f.tunnel_id,t.in_node_id,t.out_node_id,t.type FROM forward f JOIN tunnel t ON t.id=f.tunnel_id WHERE f.id=?", id)
	if len(rows) > 0 {
		userID, _ := requestInt64(rows[0]["userId"])
		nodeID, _ := requestInt64(rows[0]["inNodeId"])
		name := fmt.Sprintf("%d_%d_0", id, userID)
		_ = a.nodeHub.SendCommand(r.Context(), nodeID, "DeleteService", map[string]any{"services": []string{name + "_tcp", name + "_udp"}})
		if numberOrZero(rows[0]["type"]) == 2 {
			outNodeID, _ := requestInt64(rows[0]["outNodeId"])
			_ = a.nodeHub.SendCommand(r.Context(), nodeID, "DeleteChains", map[string]any{"chain": name + "_chains"})
			_ = a.nodeHub.SendCommand(r.Context(), outNodeID, "DeleteService", map[string]any{"services": []string{name + "_tls"}})
		}
	}
	return a.store.DeleteByID(r.Context(), "forward", id)
}

func (a *API) inboundLineStatus(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if !decodeJSON(w, r, &body) {
		return
	}
	line, users, err := a.findLineAndUsers(r, body)
	if err != nil {
		writeResponse(w, Failure(err.Error()))
		return
	}
	status := numberOrZero(body["status"])
	lineID, _ := requestInt64(line["id"])
	_ = a.store.UpdateMap(r.Context(), "inbound_line", lineID, map[string]any{"status": status, "updated_time": time.Now().UnixMilli()})
	for _, user := range users {
		forwardID, _ := requestInt64(user["gostForwardId"])
		if status == 0 {
			a.pauseResumeForwardByID(r, forwardID, "PauseService")
		} else {
			a.pauseResumeForwardByID(r, forwardID, "ResumeService")
		}
	}
	writeResponse(w, OK(nil))
}
func (a *API) deleteInboundLine(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if !decodeJSON(w, r, &body) {
		return
	}
	line, users, err := a.findLineAndUsers(r, body)
	if err != nil {
		writeResponse(w, Failure(err.Error()))
		return
	}
	for _, user := range users {
		forwardID, _ := requestInt64(user["gostForwardId"])
		id, _ := requestInt64(user["id"])
		_ = a.deleteForwardRecord(r, forwardID)
		_ = a.store.DeleteByID(r.Context(), "inbound_user", id)
	}
	lineID, _ := requestInt64(line["id"])
	_ = a.store.DeleteByID(r.Context(), "inbound_line", lineID)
	_ = a.pushSingbox(r, numberOrZero(body["nodeId"]))
	writeResponse(w, OK(nil))
}
func (a *API) findLineAndUsers(r *http.Request, body map[string]any) (map[string]any, []map[string]any, error) {
	userID, nodeID := numberOrZero(body["userId"]), numberOrZero(body["nodeId"])
	query := "SELECT id FROM inbound_line WHERE user_id=? AND node_id=?"
	args := []any{userID, nodeID}
	if body["landingId"] == nil {
		query += " AND landing_id IS NULL"
	} else {
		query += " AND landing_id=?"
		args = append(args, body["landingId"])
	}
	lines, _ := a.store.QueryMaps(r.Context(), query+" LIMIT 1", args...)
	if len(lines) == 0 {
		return nil, nil, errors.New("线路不存在")
	}
	userQuery := "SELECT iu.id,iu.gost_forward_id FROM inbound_user iu JOIN inbound i ON i.id=iu.inbound_id WHERE iu.user_id=? AND i.node_id=?"
	userArgs := []any{userID, nodeID}
	if body["landingId"] == nil {
		userQuery += " AND i.landing_id IS NULL"
	} else {
		userQuery += " AND i.landing_id=?"
		userArgs = append(userArgs, body["landingId"])
	}
	users, _ := a.store.QueryMaps(r.Context(), userQuery, userArgs...)
	return lines[0], users, nil
}
func (a *API) pauseResumeForwardByID(r *http.Request, id int64, command string) {
	rows, _ := a.store.QueryMaps(r.Context(), "SELECT f.user_id,t.in_node_id,t.out_node_id,t.type FROM forward f JOIN tunnel t ON t.id=f.tunnel_id WHERE f.id=?", id)
	if len(rows) == 0 {
		return
	}
	userID, _ := requestInt64(rows[0]["userId"])
	nodeID, _ := requestInt64(rows[0]["inNodeId"])
	name := fmt.Sprintf("%d_%d_0", id, userID)
	_ = a.nodeHub.SendCommand(r.Context(), nodeID, command, map[string]any{"services": []string{name + "_tcp", name + "_udp"}})
	if numberOrZero(rows[0]["type"]) == 2 {
		outNodeID, _ := requestInt64(rows[0]["outNodeId"])
		_ = a.nodeHub.SendCommand(r.Context(), outNodeID, command, map[string]any{"services": []string{name + "_tls"}})
	}
	status := 1
	if command == "PauseService" {
		status = 0
	}
	_ = a.store.UpdateMap(r.Context(), "forward", id, map[string]any{"status": status, "updated_time": time.Now().UnixMilli()})
}

func (a *API) renameInbound(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if !decodeJSON(w, r, &body) {
		return
	}
	id, err := storeID(body)
	if err != nil {
		writeResponse(w, Failure("参数不全"))
		return
	}
	if err := a.store.UpdateMap(r.Context(), "inbound", id, map[string]any{"remark": toString(body["remark"]), "updated_time": time.Now().UnixMilli()}); err != nil {
		writeResponse(w, Failure("协议不存在"))
		return
	}
	writeResponse(w, OK(nil))
}
func (a *API) reloadNodeSingbox(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if !decodeJSON(w, r, &body) {
		return
	}
	nodeID, err := storeIDFrom(body, "nodeId")
	if err != nil {
		writeResponse(w, Failure("节点不存在"))
		return
	}
	if err := a.pushSingbox(r, nodeID); err != nil {
		writeResponse(w, Failure(err.Error()))
		return
	}
	writeResponse(w, OK(nil))
}

func (a *API) inboundUserSub(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if !decodeJSON(w, r, &body) {
		return
	}
	rows, _ := a.store.QueryMaps(r.Context(), "SELECT sub_token FROM inbound_user WHERE user_id=? AND sub_token IS NOT NULL LIMIT 1", numberOrZero(body["userId"]))
	if len(rows) == 0 {
		writeResponse(w, OK(""))
		return
	}
	writeResponse(w, OK(rows[0]["subToken"]))
}
func (a *API) inboundUserLines(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if !decodeJSON(w, r, &body) {
		return
	}
	a.writeUserLines(w, r, numberOrZero(body["userId"]))
}
func (a *API) myInboundLines(w http.ResponseWriter, r *http.Request) {
	uid, ok := currentUserID(w, r)
	if !ok {
		return
	}
	a.writeUserLines(w, r, uid)
}
func (a *API) writeUserLines(w http.ResponseWriter, r *http.Request, userID int64) {
	rows, err := a.store.QueryMaps(r.Context(), "SELECT il.node_id,n.name AS node_name,il.landing_id,l.name AS landing_name,il.sub_token,il.flow AS quota_gb,il.exp_time AS line_exp_time,il.status AS line_status,COUNT(iu.id) AS protocol_count,COALESCE(SUM(f.in_flow+f.out_flow),0) AS flow FROM inbound_line il JOIN node n ON n.id=il.node_id LEFT JOIN landing l ON l.id=il.landing_id LEFT JOIN inbound i ON i.node_id=il.node_id AND (i.landing_id=il.landing_id OR (i.landing_id IS NULL AND il.landing_id IS NULL)) LEFT JOIN inbound_user iu ON iu.inbound_id=i.id AND iu.user_id=il.user_id AND iu.status=1 LEFT JOIN forward f ON f.id=iu.gost_forward_id WHERE il.user_id=? GROUP BY il.id,il.node_id,n.name,il.landing_id,l.name,il.sub_token,il.flow,il.exp_time,il.status ORDER BY il.node_id,il.landing_id", userID)
	if err != nil {
		writeResponse(w, Error(-2, err.Error()))
		return
	}
	for _, line := range rows {
		if line["landingId"] == nil {
			line["type"] = "direct"
		} else {
			line["type"] = "relay"
		}
	}
	user, _ := a.store.UserByID(r.Context(), userID)
	result := map[string]any{"lines": rows}
	if len(rows) > 0 {
		token := user.AllSubToken.String
		if token == "" {
			token = randomToken()
			_ = a.store.UpdateMap(r.Context(), "user", userID, map[string]any{"all_sub_token": token, "updated_time": time.Now().UnixMilli()})
		}
		result["allSubToken"] = token
	}
	writeResponse(w, OK(result))
}

func (a *API) autoProvisionTargets(w http.ResponseWriter, r *http.Request) {
	items, err := a.store.QueryMaps(r.Context(), "SELECT * FROM inbound_auto_provision ORDER BY node_id,landing_id")
	if err != nil {
		writeResponse(w, Error(-2, err.Error()))
		return
	}
	writeResponse(w, OK(items))
}
func (a *API) setAutoProvisionTarget(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if !decodeJSON(w, r, &body) {
		return
	}
	nodeID := numberOrZero(body["nodeId"])
	landingID := numberOrZero(body["landingId"])
	if _, err := a.store.NodeByID(r.Context(), nodeID); err != nil {
		writeResponse(w, Failure("节点不存在"))
		return
	}
	if !a.hasProtocolGroup(r, nodeID, landingID) {
		message := "这台机器还没有直连协议"
		if landingID != 0 {
			message = "这条中转还没有协议"
		}
		writeResponse(w, Failure(message))
		return
	}
	enabled, _ := body["enabled"].(bool)
	rows, _ := a.store.QueryMaps(r.Context(), "SELECT id FROM inbound_auto_provision WHERE node_id=? AND landing_id=? LIMIT 1", nodeID, landingID)
	value := 0
	if enabled {
		value = 1
	}
	now := time.Now().UnixMilli()
	if len(rows) == 0 {
		_, _ = a.store.InsertMap(r.Context(), "inbound_auto_provision", map[string]any{"node_id": nodeID, "landing_id": landingID, "enabled": value, "created_time": now, "updated_time": now})
	} else {
		id, _ := requestInt64(rows[0]["id"])
		_ = a.store.UpdateMap(r.Context(), "inbound_auto_provision", id, map[string]any{"enabled": value, "updated_time": now})
	}
	result := map[string]any{"nodeId": nodeID, "landingId": body["landingId"], "enabled": enabled}
	if enabled {
		provisioned, skipped, failures := a.provisionSubscribersForTarget(r, nodeID, landingID)
		result["provisionedUsers"], result["skippedUsers"], result["errors"] = provisioned, skipped, failures
	}
	writeResponse(w, OK(result))
}
func (a *API) provisionSubscribers(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if !decodeJSON(w, r, &body) {
		return
	}
	nodeID := numberOrZero(body["nodeId"])
	landingID := numberOrZero(body["landingId"])
	provisioned, skipped, failures := a.provisionSubscribersForTarget(r, nodeID, landingID)
	writeResponse(w, OK(map[string]any{"nodeId": nodeID, "provisionedUsers": provisioned, "skippedUsers": skipped, "errors": failures}))
}

func (a *API) provisionSubscribersForTarget(r *http.Request, nodeID, landingID int64) (int, int, []string) {
	users, err := a.store.QueryMaps(r.Context(), "SELECT u.id FROM tms_user u JOIN user_subscription s ON s.user_id=u.id WHERE u.status=1 AND u.role_id<>0 AND s.status=1 AND (s.expires_at=0 OR s.expires_at>?) AND (s.traffic_limit_bytes=0 OR s.traffic_used_bytes<s.traffic_limit_bytes)", time.Now().UnixMilli())
	if err != nil {
		return 0, 0, []string{err.Error()}
	}
	provisioned, skipped := 0, 0
	failures := make([]string, 0)
	for _, user := range users {
		userID, idErr := requestInt64(user["id"])
		if idErr != nil {
			skipped++
			continue
		}
		assign := map[string]any{"userId": userID, "nodeId": nodeID}
		if landingID != 0 {
			assign["relay"], assign["landingId"] = true, landingID
		}
		result, assignErr := a.assignInboundGroup(r, assign)
		if assignErr != nil {
			skipped++
			failures = append(failures, fmt.Sprintf("用户 %d: %v", userID, assignErr))
			continue
		}
		assigned, _ := result["assigned"].(int)
		updated, _ := result["updated"].(int)
		if assigned+updated > 0 {
			provisioned++
		} else {
			skipped++
		}
	}
	return provisioned, skipped, failures
}

func (a *API) provisionAutoTargetsForUser(r *http.Request, userID int64) error {
	user, err := a.store.UserByID(r.Context(), userID)
	if err != nil || user.RoleID == 0 || user.Status != 1 {
		return err
	}
	rows, err := a.store.QueryMaps(r.Context(), "SELECT id,node_id,landing_id FROM inbound_auto_provision WHERE enabled=1 ORDER BY node_id,landing_id")
	if err != nil {
		return err
	}
	var failures []string
	for _, target := range rows {
		targetID, _ := requestInt64(target["id"])
		nodeID, nodeErr := requestInt64(target["nodeId"])
		landingID := numberOrZero(target["landingId"])
		if nodeErr != nil || !a.hasProtocolGroup(r, nodeID, landingID) {
			_ = a.store.UpdateMap(r.Context(), "inbound_auto_provision", targetID, map[string]any{"enabled": 0, "updated_time": time.Now().UnixMilli()})
			continue
		}
		assign := map[string]any{"userId": userID, "nodeId": nodeID}
		if landingID != 0 {
			assign["relay"], assign["landingId"] = true, landingID
		}
		if _, err := a.assignInboundGroup(r, assign); err != nil {
			failures = append(failures, fmt.Sprintf("节点 %d: %v", nodeID, err))
		}
	}
	if len(failures) > 0 {
		return errors.New(strings.Join(failures, "; "))
	}
	return nil
}

func (a *API) hasProtocolGroup(r *http.Request, nodeID, landingID int64) bool {
	query := "SELECT id FROM inbound WHERE node_id=?"
	args := []any{nodeID}
	if landingID == 0 {
		query += " AND landing_id IS NULL"
	} else {
		query += " AND landing_id=?"
		args = append(args, landingID)
	}
	query += " LIMIT 1"
	rows, err := a.store.QueryMaps(r.Context(), query, args...)
	return err == nil && len(rows) > 0
}

func (a *API) pushSingbox(r *http.Request, nodeID int64) error {
	inbounds, err := a.store.QueryMaps(r.Context(), "SELECT * FROM inbound WHERE node_id=? ORDER BY id", nodeID)
	if err != nil {
		return err
	}
	config := map[string]any{"log": map[string]any{"level": "warn"}, "inbounds": []any{}, "outbounds": []any{map[string]any{"type": "direct", "tag": "direct"}}, "route": map[string]any{"rules": []any{}, "final": "direct"}}
	inboundConfigs := []any{}
	outbounds := config["outbounds"].([]any)
	rules := []any{}
	landings := map[int64]string{}
	for _, in := range inbounds {
		id, _ := requestInt64(in["id"])
		users, _ := a.store.QueryMaps(r.Context(), "SELECT * FROM inbound_user WHERE inbound_id=? AND status=1", id)
		inboundConfigs = append(inboundConfigs, buildSingboxInbound(in, users))
		if in["landingId"] != nil {
			landingID, _ := requestInt64(in["landingId"])
			if _, ok := landings[landingID]; !ok {
				rows, _ := a.store.QueryMaps(r.Context(), "SELECT outbound_json FROM landing WHERE id=?", landingID)
				if len(rows) > 0 {
					var outbound map[string]any
					_ = json.Unmarshal([]byte(toString(rows[0]["outboundJson"])), &outbound)
					tag := fmt.Sprintf("landing-%d", landingID)
					outbound["tag"] = tag
					outbounds = append(outbounds, outbound)
					landings[landingID] = tag
				}
			}
			rules = append(rules, map[string]any{"inbound": []string{toString(in["tag"])}, "outbound": landings[landingID]})
		}
	}
	config["inbounds"], config["outbounds"] = inboundConfigs, outbounds
	config["route"] = map[string]any{"rules": rules, "final": "direct"}
	result := a.nodeHub.SendCommand(r.Context(), nodeID, "SetSingboxConfig", map[string]any{"config": config})
	if result.Msg != "OK" {
		return fmt.Errorf("下发 sing-box 配置失败:%s", result.Msg)
	}
	return nil
}
func buildSingboxInbound(in map[string]any, users []map[string]any) map[string]any {
	protocol := toString(in["protocol"])
	value := map[string]any{"type": protocol, "tag": in["tag"], "listen": "127.0.0.1", "listen_port": in["listenPort"]}
	userValues := []map[string]any{}
	for _, user := range users {
		entry := map[string]any{"name": fmt.Sprintf("u%v", user["userId"])}
		switch protocol {
		case "vless", "vmess":
			entry["uuid"] = user["uuid"]
			if protocol == "vless" {
				entry["flow"] = "xtls-rprx-vision"
			}
		case "tuic":
			entry["uuid"], entry["password"] = user["uuid"], user["password"]
		default:
			entry["password"] = user["password"]
		}
		userValues = append(userValues, entry)
	}
	switch protocol {
	case "shadowsocks":
		var cfg map[string]any
		_ = json.Unmarshal([]byte(toString(in["configJson"])), &cfg)
		value["method"], value["password"] = cfg["method"], cfg["password"]
	case "vless", "trojan":
		value["users"] = userValues
		value["tls"] = map[string]any{"enabled": true, "server_name": in["sni"], "reality": map[string]any{"enabled": true, "handshake": map[string]any{"server": in["dest"], "server_port": 443}, "private_key": in["privateKey"], "short_id": []any{in["shortId"]}}}
	case "vmess":
		value["users"] = userValues
	case "hysteria2", "tuic", "anytls":
		value["users"] = userValues
		value["tls"] = map[string]any{"enabled": true, "server_name": in["sni"], "certificate_path": "/etc/gost/certs/self.crt", "key_path": "/etc/gost/certs/self.key"}
	}
	return value
}

func sanitizeSNI(value string) string {
	value = strings.TrimSpace(value)
	if validSNI.MatchString(value) {
		return value
	}
	return ""
}
func randomToken() string {
	value := make([]byte, 16)
	_, _ = rand.Read(value)
	return hex.EncodeToString(value)
}
func randomUUID() string {
	value := make([]byte, 16)
	_, _ = rand.Read(value)
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	encoded := hex.EncodeToString(value)
	return encoded[:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:]
}
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
func cloneMap(source map[string]any) map[string]any {
	target := make(map[string]any, len(source))
	for key, value := range source {
		target[key] = value
	}
	return target
}
func camelMap(source map[string]any) map[string]any {
	result := map[string]any{}
	for key, value := range source {
		parts := strings.Split(key, "_")
		for i := 1; i < len(parts); i++ {
			if parts[i] != "" {
				parts[i] = strings.ToUpper(parts[i][:1]) + parts[i][1:]
			}
		}
		result[strings.Join(parts, "")] = value
	}
	return result
}
func clientLinkFromMaps(in map[string]any, server string, port int, uuid, password string) string {
	entry := store.SubscriptionEntry{Protocol: toString(in["protocol"]), Server: server, Port: port, UUID: uuid, Password: password, SNI: toString(in["sni"]), PublicKey: toString(in["publicKey"]), ShortID: toString(in["shortId"]), ConfigJSON: toString(in["configJson"]), Remark: toString(in["remark"])}
	return clientLink(entry, protocolName(entry.Protocol))
}
