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
import java.util.Base64;
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
    public CustomNode importNode(String requestedName, String link) {
        return importNode(requestedName, link, "global", Collections.emptyList());
    }

    @Transactional
    public CustomNode importNode(String requestedName, String link, String requestedVisibility, List<Long> userIds) {
        JSONObject parsed = parseShareLink(link);
        String protocol = parsed.getString("protocol");
        parsed.remove("protocol");
        String name = requestedName == null || requestedName.trim().isEmpty() ? parsed.getString("name") : requestedName.trim();
        if (name == null || name.isEmpty()) name = "自定义 " + protocol.toUpperCase(Locale.ROOT) + " 节点";
        CustomNode node = new CustomNode();
        String visibility = "users".equalsIgnoreCase(requestedVisibility) ? "users" : "global";
        if ("users".equals(visibility) && (userIds == null || userIds.isEmpty())) throw new IllegalArgumentException("按用户订阅时至少选择一个用户");
        node.setName(name); node.setProtocol(protocol); node.setRawLink(canonicalLink(link)); node.setParsedJson(parsed.toJSONString()); node.setVisibility(visibility);
        node.setStatus(1); node.setCreatedTime(System.currentTimeMillis()); node.setUpdatedTime(System.currentTimeMillis());
        nodes.insert(node);
        if ("users".equals(visibility)) for (Long userId : new LinkedHashSet<>(userIds)) assign(node.getId(), userId);
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

    @Transactional
    public void delete(Long nodeId) {
        assignments.delete(new QueryWrapper<UserCustomNode>().eq("custom_node_id", nodeId));
        nodes.deleteById(nodeId);
    }

    public List<CustomNode> list() { return nodes.selectList(new QueryWrapper<CustomNode>().orderByDesc("id")); }

    public List<Map<String, Object>> listWithAssignments() {
        List<Map<String, Object>> result = new ArrayList<>();
        for (CustomNode node : list()) {
            Map<String, Object> item = new LinkedHashMap<>();
            item.put("id", node.getId()); item.put("name", node.getName()); item.put("protocol", node.getProtocol()); item.put("visibility", node.getVisibility() == null ? "global" : node.getVisibility()); item.put("status", node.getStatus()); item.put("createdTime", node.getCreatedTime());
            List<UserCustomNode> rows = assignments.selectList(new QueryWrapper<UserCustomNode>().eq("custom_node_id", node.getId()).eq("status", 1));
            List<Long> userIds = new ArrayList<>(); for (UserCustomNode row : rows) userIds.add(row.getUserId());
            item.put("userIds", userIds);
            result.add(item);
        }
        return result;
    }

    public List<CustomNode> activeForUser(Long userId) {
        // Imported links are external client-side subscription sources. Scope only
        // controls who receives the link; no local inbound/tunnel/GOST is created.
        QueryWrapper<CustomNode> global = new QueryWrapper<CustomNode>().eq("status", 1).and(w -> w.isNull("visibility").or().eq("visibility", "global"));
        List<CustomNode> result = new ArrayList<>(nodes.selectList(global));
        List<UserCustomNode> rows = assignments.selectList(new QueryWrapper<UserCustomNode>().eq("user_id", userId).eq("status", 1));
        if (!rows.isEmpty()) { List<Long> ids = new ArrayList<>(); for (UserCustomNode row : rows) ids.add(row.getCustomNodeId()); result.addAll(nodes.selectList(new QueryWrapper<CustomNode>().in("id", ids).eq("status", 1).eq("visibility", "users"))); }
        result.sort(Comparator.comparing(CustomNode::getId));
        return result;
    }

    public int activeCount(Long userId) { return activeForUser(userId).size(); }

    public JSONObject clashProxy(CustomNode node, Set<String> usedNames) {
        if (node == null || node.getParsedJson() == null) return null;
        JSONObject src = JSON.parseObject(node.getParsedJson());
        Map<String, Object> proxy = new LinkedHashMap<>();
        String protocol = node.getProtocol();
        proxy.put("name", uniqueName(node.getName(), usedNames));
        proxy.put("server", src.getString("server"));
        proxy.put("port", src.getInteger("port"));
        if (src.getString("server") == null || src.getInteger("port") == null) return null;
        switch (protocol) {
            case "vless": {
                proxy.put("type", "vless"); proxy.put("uuid", src.getString("uuid")); proxy.put("udp", true);
                String network = value(src, "type", "network", "tcp"); proxy.put("network", network);
                String security = value(src, "security", "security", "none");
                if ("tls".equalsIgnoreCase(security) || "reality".equalsIgnoreCase(security)) proxy.put("tls", true);
                if (!empty(src.getString("sni")).isEmpty()) proxy.put("servername", src.getString("sni"));
                if (!empty(src.getString("fp")).isEmpty()) proxy.put("client-fingerprint", src.getString("fp"));
                if ("reality".equalsIgnoreCase(security)) proxy.put("reality-opts", realityOpts(src));
                addWs(proxy, src, network); break;
            }
            case "trojan": {
                proxy.put("type", "trojan"); proxy.put("password", src.getString("password")); proxy.put("udp", true);
                proxy.put("sni", empty(src.getString("sni")));
                String security = src.getString("security");
                if ("reality".equalsIgnoreCase(security)) proxy.put("reality-opts", realityOpts(src));
                if ("reality".equalsIgnoreCase(security) || "tls".equalsIgnoreCase(security)) proxy.put("tls", true);
                addWs(proxy, src, value(src, "type", "network", "tcp")); break;
            }
            case "vmess": {
                proxy.put("type", "vmess"); proxy.put("uuid", src.getString("uuid")); proxy.put("alterId", number(src, "aid", 0));
                proxy.put("cipher", value(src, "scy", "cipher", "auto")); proxy.put("network", value(src, "net", "network", "tcp")); proxy.put("udp", true);
                if ("tls".equalsIgnoreCase(src.getString("tls")) || "1".equals(src.getString("tls"))) proxy.put("tls", true);
                if (!empty(src.getString("sni")).isEmpty()) proxy.put("servername", src.getString("sni"));
                addWs(proxy, src, value(src, "net", "network", "tcp")); break;
            }
            case "hysteria2": {
                proxy.put("type", "hysteria2"); proxy.put("password", src.getString("password")); proxy.put("sni", empty(src.getString("sni")));
                proxy.put("skip-cert-verify", insecure(src));
                if (!empty(src.getString("obfs")).isEmpty()) proxy.put("obfs", src.getString("obfs"));
                if (!empty(src.getString("obfs-password")).isEmpty()) proxy.put("obfs-password", src.getString("obfs-password"));
                break;
            }
            case "tuic": {
                proxy.put("type", "tuic"); proxy.put("uuid", src.getString("uuid")); proxy.put("password", src.getString("password"));
                proxy.put("sni", empty(src.getString("sni"))); proxy.put("skip-cert-verify", insecure(src)); proxy.put("udp-relay-mode", value(src, "udp_relay_mode", "udp-relay-mode", "native"));
                String alpn = src.getString("alpn"); if (!empty(alpn).isEmpty()) proxy.put("alpn", Arrays.asList(alpn.split(",")));
                if (!empty(src.getString("congestion_control")).isEmpty()) proxy.put("congestion-controller", src.getString("congestion_control"));
                break;
            }
            case "anytls": {
                proxy.put("type", "anytls"); proxy.put("password", src.getString("password")); proxy.put("sni", empty(src.getString("sni"))); proxy.put("skip-cert-verify", insecure(src));
                if (!empty(src.getString("fp")).isEmpty()) proxy.put("client-fingerprint", src.getString("fp")); break;
            }
            default: return null;
        }
        return new JSONObject(proxy);
    }

    private JSONObject parseShareLink(String supplied) {
        String link = canonicalLink(supplied);
        String lower = link.toLowerCase(Locale.ROOT);
        if (lower.startsWith("vmess://")) {
            String encoded = link.substring(link.indexOf("://") + 3);
            try {
                byte[] decoded = Base64.getDecoder().decode(padBase64(encoded));
                JSONObject out = JSON.parseObject(new String(decoded, StandardCharsets.UTF_8));
                out.put("protocol", "vmess"); out.put("server", out.getString("add")); out.put("port", parsePort(out.get("port")));
                out.put("uuid", out.getString("id")); out.put("name", empty(out.getString("ps")));
                out.put("net", value(out, "net", "network", "tcp"));
                return out;
            } catch (Exception e) { throw new IllegalArgumentException("VMess 分享链接不是有效的 Base64 JSON"); }
        }
        String protocol;
        if (lower.startsWith("vless://")) protocol = "vless";
        else if (lower.startsWith("trojan://")) protocol = "trojan";
        else if (lower.startsWith("hysteria2://") || lower.startsWith("hy2://")) protocol = "hysteria2";
        else if (lower.startsWith("tuic://")) protocol = "tuic";
        else if (lower.startsWith("anytls://")) protocol = "anytls";
        else throw new IllegalArgumentException("支持的协议: VLESS-Reality、Trojan-Reality、VMess、Hysteria2、TUIC、AnyTLS");
        String body = link.substring(link.indexOf("://") + 3);
        int hash = body.indexOf('#'); String name = hash >= 0 ? decode(body.substring(hash + 1)) : ""; if (hash >= 0) body = body.substring(0, hash);
        int q = body.indexOf('?'); String query = q >= 0 ? body.substring(q + 1) : ""; if (q >= 0) body = body.substring(0, q);
        int at = body.lastIndexOf('@'); if (at <= 0) throw new IllegalArgumentException("分享链接缺少凭证或服务器地址");
        String credential = decode(body.substring(0, at)); String address = body.substring(at + 1); String[] hp = splitHostPort(address);
        JSONObject out = new JSONObject(); out.put("protocol", protocol); out.put("server", hp[0]); out.put("port", Integer.parseInt(hp[1]));
        if ("tuic".equals(protocol) && credential.contains(":")) { String[] pair = credential.split(":", 2); out.put("uuid", pair[0]); out.put("password", pair[1]); }
        else if ("vless".equals(protocol)) out.put("uuid", credential); else out.put("password", credential);
        for (String part : query.split("&")) { if (part.isEmpty()) continue; int eq = part.indexOf('='); out.put(decode(eq > 0 ? part.substring(0, eq) : part), eq > 0 ? decode(part.substring(eq + 1)) : ""); }
        out.put("name", name); if (!out.containsKey("type")) out.put("type", "tcp"); if (!out.containsKey("security") && ("vless".equals(protocol) || "trojan".equals(protocol))) out.put("security", "none");
        return out;
    }
    private String[] splitHostPort(String address) { String a = address; if (a.startsWith("[")) { int end = a.indexOf(']'); if (end < 0 || end + 2 > a.length() || a.charAt(end + 1) != ':') throw new IllegalArgumentException("服务器地址或端口不正确"); return new String[]{a.substring(1, end), a.substring(end + 2)}; } int colon = a.lastIndexOf(':'); if (colon <= 0 || colon == a.length() - 1) throw new IllegalArgumentException("服务器地址或端口不正确"); return new String[]{a.substring(0, colon), a.substring(colon + 1)}; }
    private Integer parsePort(Object value) { try { return Integer.valueOf(String.valueOf(value)); } catch (Exception e) { throw new IllegalArgumentException("端口不正确"); } }
    private String padBase64(String value) { String v = value.replace('-', '+').replace('_', '/'); return v + "=".repeat((4 - v.length() % 4) % 4); }
    private String value(JSONObject o, String first, String second, String fallback) { String v = o.getString(first); return empty(v).isEmpty() ? (empty(o.getString(second)).isEmpty() ? fallback : o.getString(second)) : v; }
    private int number(JSONObject o, String key, int fallback) { try { return Integer.parseInt(String.valueOf(o.get(key))); } catch (Exception e) { return fallback; } }
    private boolean insecure(JSONObject o) { String v = o.getString("insecure"); return "1".equals(v) || "true".equalsIgnoreCase(v) || "yes".equalsIgnoreCase(v) || "1".equals(o.getString("allow_insecure")); }
    private Map<String, Object> realityOpts(JSONObject src) { Map<String, Object> r = new LinkedHashMap<>(); r.put("public-key", empty(src.getString("pbk"))); r.put("short-id", empty(src.getString("sid"))); return r; }
    private void addWs(Map<String, Object> proxy, JSONObject src, String network) { if ("ws".equalsIgnoreCase(network)) { Map<String, Object> ws = new LinkedHashMap<>(); ws.put("path", empty(src.getString("path"))); String host = src.getString("host"); if (!empty(host).isEmpty()) ws.put("headers", Collections.singletonMap("Host", host)); proxy.put("ws-opts", ws); } }
    private String canonicalLink(String value) { if (value == null) throw new IllegalArgumentException("分享链接不能为空"); return value.trim().replace("&amp;", "&"); }
    private String decode(String value) { try { return URLDecoder.decode(value, StandardCharsets.UTF_8.name()); } catch (Exception ignored) { return value; } }
    private String empty(String value) { return value == null ? "" : value; }
    private String uniqueName(String name, Set<String> used) { String base = name == null || name.isBlank() ? "自定义节点" : name.trim(); String candidate = base; int i = 2; while (!used.add(candidate)) candidate = base + " " + i++; return candidate; }
}
