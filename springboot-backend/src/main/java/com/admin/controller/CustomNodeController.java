package com.admin.controller;

import com.admin.common.annotation.RequireRole;
import com.admin.common.lang.R;
import com.admin.entity.CustomNode;
import com.admin.service.CustomNodeService;
import com.admin.service.InboundService;
import org.springframework.web.bind.annotation.*;
import java.util.Map;
import java.util.List;
import java.util.ArrayList;

@RestController @CrossOrigin @RequestMapping("/api/v1/custom-nodes")
public class CustomNodeController {
    private final CustomNodeService service;
    private final InboundService inboundService;
    public CustomNodeController(CustomNodeService service, InboundService inboundService) { this.service = service; this.inboundService = inboundService; }
    @RequireRole @GetMapping public R list() { return R.ok(service.listWithAssignments()); }
    @RequireRole @PostMapping public R importNode(@RequestBody Map<String,Object> body) {
        try {
            List<Long> userIds = new ArrayList<>(); Object raw = body.get("userIds");
            if (raw instanceof List<?> list) for (Object id : list) userIds.add(Long.valueOf(String.valueOf(id)));
            return R.ok(service.importNode(String.valueOf(body.getOrDefault("name", "")), String.valueOf(body.get("link")), String.valueOf(body.getOrDefault("visibility", "global")), userIds));
        } catch (IllegalArgumentException e) { return R.err(e.getMessage()); }
    }
    @RequireRole @PostMapping("/{nodeId}/assign") public R assign(@PathVariable Long nodeId, @RequestBody Map<String,Object> body) { try { service.assign(nodeId, Long.valueOf(String.valueOf(body.get("userId")))); return R.ok(); } catch (IllegalArgumentException e) { return R.err(e.getMessage()); } }
    @RequireRole @DeleteMapping("/{nodeId}/assign/{userId}") public R unassign(@PathVariable Long nodeId, @PathVariable Long userId) { service.unassign(nodeId, userId); return R.ok(); }
    /** Stop publishing the node while keeping its record for audit/re-enable. */
    @RequireRole @PostMapping("/{nodeId}/disable") public R disable(@PathVariable Long nodeId) { try { return R.ok(service.disable(nodeId)); } catch (IllegalArgumentException e) { return R.err(e.getMessage()); } }
    /** Convert an imported external link into a TMS ingress relay so GOST can account traffic per user. */
    @RequireRole @PostMapping("/{nodeId}/build-metered-relay") public R buildMeteredRelay(@PathVariable Long nodeId, @RequestBody Map<String,Object> body) {
        try {
            Object rawIngress = body.get("ingressNodeId");
            if (rawIngress == null) return R.err("请选择承载中转的 TMS 节点");
            Long ingressNodeId = Long.valueOf(String.valueOf(rawIngress));
            CustomNode custom = service.getForMeteredRelay(nodeId);
            R created = inboundService.oneClickRelay(ingressNodeId, custom.getRawLink(), "中转-" + custom.getName(), body.get("sni") == null ? null : String.valueOf(body.get("sni")));
            if (created.getCode() != 0) return created;
            // The original client-side node has no user identity and bypasses accounting.
            // Stop publishing it after the metered relay has been created.
            service.disable(nodeId);
            if (Boolean.parseBoolean(String.valueOf(body.getOrDefault("provisionSubscribedUsers", true)))) {
                Long landingId = null;
                if (created.getData() instanceof java.util.List<?> rows && !rows.isEmpty() && rows.get(0) instanceof com.admin.entity.Inbound) {
                    landingId = ((com.admin.entity.Inbound) rows.get(0)).getLandingId();
                }
                R provisioned = landingId == null
                        ? inboundService.provisionSubscribedUsers(ingressNodeId)
                        : inboundService.provisionSubscribedUsers(ingressNodeId, landingId);
                if (provisioned.getCode() != 0) return R.err("中转已创建，但全局分配失败:" + provisioned.getMsg());
                java.util.Map<String, Object> result = new java.util.LinkedHashMap<>();
                result.put("relay", created.getData());
                result.put("provision", provisioned.getData());
                return R.ok(result);
            }
            return created;
        } catch (IllegalArgumentException e) { return R.err(e.getMessage()); }
    }
    /** Permanently remove the imported link and its legacy assignment rows. */
    @RequireRole @DeleteMapping("/{nodeId}") public R delete(@PathVariable Long nodeId) { try { service.delete(nodeId); return R.ok(); } catch (IllegalArgumentException e) { return R.err(e.getMessage()); } }
}
