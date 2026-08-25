package com.admin.service;

import com.admin.entity.CustomNode;
import com.admin.entity.UserCustomNode;
import com.admin.mapper.CustomNodeMapper;
import com.admin.mapper.UserCustomNodeMapper;
import com.alibaba.fastjson.JSON;
import com.alibaba.fastjson.JSONObject;
import com.baomidou.mybatisplus.core.conditions.query.QueryWrapper;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.net.URLDecoder;
import java.nio.charset.StandardCharsets;
import java.util.*;

/** Stores imported client links separately from server-side sing-box inbounds. */
@Service
public class CustomNodeService {
    private final CustomNodeMapper nodes;
    private final UserCustomNodeMapper assignments;

    public CustomNodeService(CustomNodeMapper nodes, UserCustomNodeMapper assignments) {
        this.nodes = nodes;
        this.assignments = assignments;
    }

    @Transactional
    public CustomNode importVless(String requestedName, String link) {
        JSONObject parsed = parseVless(link);
        String name = requestedName == null || requestedName.trim().isEmpty() ? parsed.getString("name") : requestedName.trim();
        if (name == null || name.isEmpty()) name = "自定义 VLESS 节点";
        CustomNode node = new CustomNode();
        node.setName(name); node.setProtocol("vless"); node.setRawLink(canonicalLink(link)); node.setParsedJson(parsed.toJSONString());
        node.setStatus(1); node.setCreatedTime(System.currentTimeMillis()); node.setUpdatedTime(System.currentTimeMillis());
        nodes.insert(node);
        return node;
    }

    @Transactional
    public void assign(Long nodeId, Long userId) {
        if (nodes.selectById(nodeId) == null) throw new IllegalArgumentException("自定义节点不存在");
        UserCustomNode existing = assignments.selectOne(new QueryWrapper<UserCustomNode>().eq("custom_node_id", nodeId).eq("user_id", userId).last("limit 1"));
        if (existing == null) {
            UserCustomNode item = new UserCustomNode(); item.setUserId(userId); item.setCustomNodeId(nodeId); item.setStatus(1); item.setCreatedTime(System.currentTimeMillis()); assignments.insert(item);
        } else if (existing.getStatus() == null || existing.getStatus() != 1) {
            existing.setStatus(1); assignments.updateById(existing);
        }
    }

    public void unassign(Long nodeId, Long userId) {
        assignments.delete(new QueryWrapper<UserCustomNode>().eq("custom_node_id", nodeId).eq("user_id", userId));
    }

    public void disable(Long nodeId) {
        CustomNode node = new CustomNode(); node.setId(nodeId); node.setStatus(0); node.setUpdatedTime(System.currentTimeMillis()); nodes.updateById(node);
    }

    public List<CustomNode> list() { return nodes.selectList(new QueryWrapper<CustomNode>().orderByDesc("id")); }

    public List<Map<String, Object>> listWithAssignments() {
        List<Map<String, Object>> result = new ArrayList<>();
        for (CustomNode node : list()) {
            Map<String, Object> item = new LinkedHashMap<>();
            item.put("id", node.getId()); item.put("name", node.getName()); item.put("protocol", node.getProtocol()); item.put("status", node.getStatus()); item.put("createdTime", node.getCreatedTime());
            List<UserCustomNode> rows = assignments.selectList(new QueryWrapper<UserCustomNode>().eq("custom_node_id", node.getId()).eq("status", 1));
            List<Long> userIds = new ArrayList<>(); for (UserCustomNode row : rows) userIds.add(row.getUserId());
            item.put("userIds", userIds);
            result.add(item);
        }
        return result;
    }

    public List<CustomNode> activeForUser(Long userId) {
        List<UserCustomNode> rows = assignments.selectList(new QueryWrapper<UserCustomNode>().eq("user_id", userId).eq("status", 1));
        if (rows.isEmpty()) return Collections.emptyList();
        List<Long> ids = new ArrayList<>(); for (UserCustomNode row : rows) ids.add(row.getCustomNodeId());
        return nodes.selectList(new QueryWrapper<CustomNode>().in("id", ids).eq("status", 1));
    }

    public int activeCount(Long userId) { return activeForUser(userId).size(); }

    public JSONObject clashProxy(CustomNode node, Set<String> usedNames) {
        if (node == null || !"vless".equals(node.getProtocol())) return null;
        JSONObject src = JSON.parseObject(node.getParsedJson());
        Map<String, Object> proxy = new LinkedHashMap<>();
        proxy.put("name", uniqueName(node.getName(), usedNames)); proxy.put("type", "vless"); proxy.put("server", src.getString("server")); proxy.put("port", src.getInteger("port")); proxy.put("uuid", src.getString("uuid")); proxy.put("udp", true);
        String network = src.getString("type"); proxy.put("network", network == null || network.isEmpty() ? "tcp" : network);
        String security = src.getString("security");
        if ("tls".equalsIgnoreCase(security) || "reality".equalsIgnoreCase(security)) proxy.put("tls", true);
        if ("reality".equalsIgnoreCase(security)) {
            proxy.put("servername", empty(src.getString("sni"))); proxy.put("client-fingerprint", empty(src.getString("fp")));
            Map<String, Object> reality = new LinkedHashMap<>(); reality.put("public-key", empty(src.getString("pbk"))); reality.put("short-id", empty(src.getString("sid"))); proxy.put("reality-opts", reality);
        } else if ("tls".equalsIgnoreCase(security)) proxy.put("servername", empty(src.getString("sni")));
        if ("ws".equalsIgnoreCase(network)) { Map<String, Object> ws = new LinkedHashMap<>(); ws.put("path", empty(src.getString("path"))); String host = src.getString("host"); if (host != null && !host.isEmpty()) ws.put("headers", Collections.singletonMap("Host", host)); proxy.put("ws-opts", ws); }
        return new JSONObject(proxy);
    }

    private JSONObject parseVless(String supplied) {
        String link = canonicalLink(supplied);
        if (!link.regionMatches(true, 0, "vless://", 0, 8)) throw new IllegalArgumentException("目前仅支持 VLESS 分享链接");
        int at = link.indexOf('@'); int hash = link.indexOf('#'); int question = link.indexOf('?');
        if (at <= 8) throw new IllegalArgumentException("VLESS 链接缺少 UUID 或服务器地址");
        String uuid = link.substring(8, at); int addressEnd = question >= 0 ? question : (hash >= 0 ? hash : link.length()); String address = link.substring(at + 1, addressEnd);
        int colon = address.lastIndexOf(':'); if (colon <= 0 || colon == address.length() - 1) throw new IllegalArgumentException("VLESS 链接缺少端口");
        JSONObject out = new JSONObject(); out.put("uuid", uuid); out.put("server", address.substring(0, colon));
        try { out.put("port", Integer.parseInt(address.substring(colon + 1))); } catch (NumberFormatException e) { throw new IllegalArgumentException("VLESS 端口不正确"); }
        if (question >= 0) { int end = hash >= 0 ? hash : link.length(); for (String part : link.substring(question + 1, end).split("&")) { int eq = part.indexOf('='); if (eq > 0) out.put(decode(part.substring(0, eq)), decode(part.substring(eq + 1))); } }
        out.put("name", hash >= 0 ? decode(link.substring(hash + 1)) : "自定义 VLESS 节点");
        if (!out.containsKey("type")) out.put("type", "tcp"); if (!out.containsKey("security")) out.put("security", "none");
        return out;
    }
    private String canonicalLink(String value) { if (value == null) throw new IllegalArgumentException("分享链接不能为空"); return value.trim().replace("&amp;", "&"); }
    private String decode(String value) { try { return URLDecoder.decode(value, StandardCharsets.UTF_8.name()); } catch (Exception ignored) { return value; } }
    private String empty(String value) { return value == null ? "" : value; }
    private String uniqueName(String name, Set<String> used) { String base = name == null || name.isBlank() ? "自定义节点" : name.trim(); String candidate = base; int i = 2; while (!used.add(candidate)) candidate = base + " " + i++; return candidate; }
}
