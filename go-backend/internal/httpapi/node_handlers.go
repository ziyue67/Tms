package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/ziyue67/tms/go-backend/internal/store"
)

type nodeRequest struct {
	ID        *int64 `json:"id"`
	Name      string `json:"name"`
	IP        string `json:"ip"`
	ServerIP  string `json:"serverIp"`
	Domain    string `json:"domain"`
	PortStart *int   `json:"portSta"`
	PortEnd   *int   `json:"portEnd"`
	HTTP      *int   `json:"http"`
	TLS       *int   `json:"tls"`
	SOCKS     *int   `json:"socks"`
}

func (a *API) createNode(w http.ResponseWriter, r *http.Request) {
	request, ok := decodeNodeRequest(w, r, false)
	if !ok {
		return
	}
	if _, err := a.store.CreateNode(r.Context(), request.nodeInput()); err != nil {
		a.logger.Error("create node", "error", err)
		writeResponse(w, Failure("节点创建失败"))
		return
	}
	writeResponse(w, OK("节点创建成功"))
}

func (a *API) listNodes(w http.ResponseWriter, r *http.Request) {
	nodes, err := a.store.Nodes(r.Context())
	if err != nil {
		a.logger.Error("list nodes", "error", err)
		writeResponse(w, Error(-2, "查询节点失败"))
		return
	}
	result := make([]map[string]any, 0, len(nodes))
	for _, node := range nodes {
		node.Secret.Valid = false
		encoded, _ := json.Marshal(node)
		item := make(map[string]any)
		_ = json.Unmarshal(encoded, &item)
		if a.nodeHub != nil {
			if state, exists := a.nodeHub.State(node.ID); exists {
				for key, value := range state {
					item[key] = value
				}
			}
		}
		result = append(result, item)
	}
	writeResponse(w, OK(result))
}

func (a *API) updateNode(w http.ResponseWriter, r *http.Request) {
	request, ok := decodeNodeRequest(w, r, true)
	if !ok {
		return
	}
	existing, err := a.store.NodeByID(r.Context(), *request.ID)
	if store.IsNotFound(err) {
		writeResponse(w, Failure("节点不存在"))
		return
	}
	if err != nil {
		writeResponse(w, Error(-2, "节点更新失败"))
		return
	}
	changed := (request.HTTP != nil && *request.HTTP != existing.HTTP) || (request.TLS != nil && *request.TLS != existing.TLS) || (request.SOCKS != nil && *request.SOCKS != existing.SOCKS)
	if existing.Status == 1 && changed && a.nodeHub != nil {
		result := a.nodeHub.SendCommand(r.Context(), existing.ID, "SetProtocol", map[string]any{"http": request.HTTP, "tls": request.TLS, "socks": request.SOCKS})
		if result.Msg != "OK" {
			writeResponse(w, Failure(result.Msg))
			return
		}
	}
	if err := a.store.UpdateNode(r.Context(), existing.ID, request.nodeInput()); err != nil {
		a.logger.Error("update node", "node_id", existing.ID, "error", err)
		writeResponse(w, Failure("节点更新失败"))
		return
	}
	writeResponse(w, OK("节点更新成功"))
}

func (a *API) renameNode(w http.ResponseWriter, r *http.Request) {
	var request struct {
		ID   *int64 `json:"id"`
		Name string `json:"name"`
	}
	if !decodeJSON(w, r, &request) {
		return
	}
	if request.ID == nil {
		writeResponse(w, Failure("参数不全"))
		return
	}
	if _, err := a.store.NodeByID(r.Context(), *request.ID); store.IsNotFound(err) {
		writeResponse(w, Failure("节点不存在"))
		return
	} else if err != nil {
		writeResponse(w, Failure("节点更新失败"))
		return
	}
	updated, err := a.store.RenameNode(r.Context(), *request.ID, strings.TrimSpace(request.Name))
	if err != nil || !updated {
		writeResponse(w, Failure("节点更新失败"))
		return
	}
	writeResponse(w, OK(nil))
}

func (a *API) deleteNode(w http.ResponseWriter, r *http.Request) {
	id, ok := decodeID(w, r)
	if !ok {
		return
	}
	if _, err := a.store.NodeByID(r.Context(), id); store.IsNotFound(err) {
		writeResponse(w, Failure("节点不存在"))
		return
	} else if err != nil {
		writeResponse(w, Failure("节点删除失败"))
		return
	}
	if err := a.store.DeleteNode(r.Context(), id); err != nil {
		writeResponse(w, Failure(err.Error()))
		return
	}
	if a.nodeHub != nil {
		a.nodeHub.DisconnectNode(id)
	}
	writeResponse(w, OK("节点删除成功"))
}

func (a *API) nodeInstallCommand(w http.ResponseWriter, r *http.Request) {
	id, ok := decodeID(w, r)
	if !ok {
		return
	}
	node, err := a.store.NodeByID(r.Context(), id)
	if store.IsNotFound(err) {
		writeResponse(w, Failure("节点不存在"))
		return
	}
	if err != nil {
		writeResponse(w, Failure("节点不存在"))
		return
	}
	panelAddress, err := a.configValue(r.Context(), "ip", "")
	if err != nil || panelAddress == "" {
		writeResponse(w, Failure("请先前往网站配置中设置ip"))
		return
	}
	start, end := node.PortStart, node.PortEnd
	if start <= 39999 && end >= 20000 {
		start = max(start, 20000)
		end = min(end, 39999)
	}
	if start < 1 || end > 65535 || start > end {
		start, end = 20000, 39999
	}
	command := "curl -L https://github.com/ziyue67/Tms/releases/latest/download/install.sh -o ./install.sh && chmod +x ./install.sh && ./install.sh -a " + processServerAddress(panelAddress) + " -s " + node.Secret.String + " -p " + fmt.Sprintf("%d:%d", start, end)
	writeResponse(w, OK(command))
}

func decodeNodeRequest(w http.ResponseWriter, r *http.Request, update bool) (nodeRequest, bool) {
	var request nodeRequest
	if !decodeJSON(w, r, &request) {
		return request, false
	}
	if update && request.ID == nil {
		writeResponse(w, Error(500, "节点ID不能为空"))
		return request, false
	}
	if strings.TrimSpace(request.Name) == "" {
		writeResponse(w, Error(500, "节点名称不能为空"))
		return request, false
	}
	if strings.TrimSpace(request.IP) == "" {
		writeResponse(w, Error(500, "入口IP不能为空"))
		return request, false
	}
	if strings.TrimSpace(request.ServerIP) == "" {
		writeResponse(w, Error(500, "服务器ip不能为空"))
		return request, false
	}
	if request.PortStart == nil {
		writeResponse(w, Failure("起始端口不能为空"))
		return request, false
	}
	if request.PortEnd == nil {
		writeResponse(w, Failure("结束端口不能为空"))
		return request, false
	}
	if *request.PortStart < 1 || *request.PortStart > 65535 || *request.PortEnd < 1 || *request.PortEnd > 65535 {
		writeResponse(w, Failure("端口必须在1-65535范围内"))
		return request, false
	}
	if *request.PortEnd < *request.PortStart {
		writeResponse(w, Failure("结束端口不能小于起始端口"))
		return request, false
	}
	return request, true
}

func (r nodeRequest) nodeInput() store.NodeInput {
	return store.NodeInput{Name: r.Name, IP: r.IP, ServerIP: r.ServerIP, Domain: strings.TrimSpace(r.Domain),
		PortStart: *r.PortStart, PortEnd: *r.PortEnd, HTTP: r.HTTP, TLS: r.TLS, SOCKS: r.SOCKS}
}

func decodeID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	var request map[string]any
	if !decodeJSON(w, r, &request) {
		return 0, false
	}
	value, exists := request["id"]
	if !exists {
		writeResponse(w, Error(500, "参数错误"))
		return 0, false
	}
	parsed, err := strconv.ParseInt(fmt.Sprint(value), 10, 64)
	if err != nil {
		if number, ok := value.(float64); ok {
			return int64(number), true
		}
		writeResponse(w, Error(500, "参数错误"))
		return 0, false
	}
	return parsed, true
}

func processServerAddress(address string) string {
	if address == "" || strings.HasPrefix(address, "[") {
		return address
	}
	lastColon := strings.LastIndex(address, ":")
	if lastColon < 0 {
		return address
	}
	host, port := address[:lastColon], address[lastColon:]
	if strings.Count(host, ":") >= 2 {
		return "[" + host + "]" + port
	}
	return address
}
