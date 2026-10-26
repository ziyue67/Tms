package httpapi

import (
	"context"
	"strings"
)

// isUDPOnlyProtocol reports whether the inbound protocol listens only on UDP.
//
// A TCP dial to one of these ports can never succeed, so treating the refused
// connection as a forward failure produces a permanent false alarm.
func isUDPOnlyProtocol(protocol string) bool {
	switch strings.ToLower(strings.TrimSpace(protocol)) {
	case "hysteria", "hysteria2", "tuic":
		return true
	default:
		return false
	}
}

// inboundProtocolForPort resolves the local inbound behind a forward target.
// diagnoseForward only has the node and target port, while inbound.protocol is
// the authoritative source for deciding whether TCP is applicable.
func (a *API) inboundProtocolForPort(ctx context.Context, nodeID int64, port int) string {
	if nodeID <= 0 || port <= 0 {
		return ""
	}
	rows, err := a.store.QueryMaps(
		ctx,
		"SELECT protocol FROM inbound WHERE node_id=? AND listen_port=? ORDER BY id LIMIT 1",
		nodeID,
		port,
	)
	if err != nil || len(rows) == 0 {
		return ""
	}
	return strings.TrimSpace(toString(rows[0]["protocol"]))
}

func skippedUDPProbeResult(nodeID int64, nodeName, host string, port int, description, protocol string) map[string]any {
	return map[string]any{
		"nodeId":      nodeID,
		"nodeName":    nodeName,
		"targetIp":    host,
		"targetPort":  port,
		"description": description,
		"success":     true, // Keep legacy clients from rendering this as a failure.
		"udpOnly":     true,
		"protocol":    strings.ToLower(strings.TrimSpace(protocol)),
		"message":     "目标为 UDP-only 协议，已跳过 TCP 探活",
	}
}
