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
    public CustomNodeController(CustomNodeService service, InboundService inboundService) {
        this.service = service;
        this.inboundService = inboundService;
    }
    @RequireRole @GetMapping public R list() { return R.ok(service.listWithAssignments()); }
    @RequireRole @PostMapping public R importNode(@RequestBody Map<String,Object> body) {
        try {
            List<Long> userIds = new ArrayList<>(); Object raw = body.get("userIds");
            if (raw instanceof List<?> list) for (Object id : list) userIds.add(Long.valueOf(String.valueOf(id)));
            CustomNode custom = service.importNode(String.valueOf(body.getOrDefault("name", "")), String.valueOf(body.get("link")), String.valueOf(body.getOrDefault("visibility", "global")), userIds);
            Object rawIngress = body.get("ingressNodeId");
            if (rawIngress != null && !String.valueOf(rawIngress).isBlank()) {
                return createImportedMeteredLine(custom, Long.valueOf(String.valueOf(rawIngress)), body);
            }
            return R.ok(custom);
        } catch (IllegalArgumentException e) { return R.err(e.getMessage()); }
    }

    /**
     * Metering is only created as part of importing a link. There is deliberately no
     * follow-up "set as metered relay" endpoint: the source link is temporary and is
     * removed as soon as the selected ingress has been provisioned.
     */
    private R createImportedMeteredLine(CustomNode custom, Long ingressNodeId, Map<String, Object> body) {
        R created = inboundService.oneClickRelay(ingressNodeId, custom.getRawLink(), "导入-" + custom.getName(),
                body.get("sni") == null ? null : String.valueOf(body.get("sni")));
        if (created.getCode() != 0) return created;

        Long landingId = null;
        if (created.getData() instanceof List<?> rows && !rows.isEmpty()
                && rows.get(0) instanceof com.admin.entity.Inbound) {
            landingId = ((com.admin.entity.Inbound) rows.get(0)).getLandingId();
        }
        if (landingId == null) return R.err("自动计费线路创建失败：未找到中转出口");

        // This assigns the generated, user-identifiable TMS protocols to every active
        // package user. Package quota and expiry enforcement therefore applies here.
        R provisioned = inboundService.provisionSubscribedUsers(ingressNodeId, landingId);
        if (provisioned.getCode() != 0) return R.err("计费线路已创建，但套餐用户分配失败:" + provisioned.getMsg());

        service.delete(custom.getId());
        Map<String, Object> result = new java.util.LinkedHashMap<>();
        result.put("relay", created.getData());
        result.put("provision", provisioned.getData());
        result.put("ingressNodeId", ingressNodeId);
        result.put("landingId", landingId);
        return R.ok(result);
    }
    @RequireRole @PostMapping("/{nodeId}/assign") public R assign(@PathVariable Long nodeId, @RequestBody Map<String,Object> body) { try { service.assign(nodeId, Long.valueOf(String.valueOf(body.get("userId")))); return R.ok(); } catch (IllegalArgumentException e) { return R.err(e.getMessage()); } }
    @RequireRole @DeleteMapping("/{nodeId}/assign/{userId}") public R unassign(@PathVariable Long nodeId, @PathVariable Long userId) { service.unassign(nodeId, userId); return R.ok(); }
    /** Stop publishing the node while keeping its record for audit/re-enable. */
    @RequireRole @PostMapping("/{nodeId}/disable") public R disable(@PathVariable Long nodeId) { try { return R.ok(service.disable(nodeId)); } catch (IllegalArgumentException e) { return R.err(e.getMessage()); } }
    /** Re-enable a previously disabled external node. */
    @RequireRole @PostMapping("/{nodeId}/enable") public R enable(@PathVariable Long nodeId) { try { return R.ok(service.enable(nodeId)); } catch (IllegalArgumentException e) { return R.err(e.getMessage()); } }
    /** Permanently remove the imported link and its legacy assignment rows. */
    @RequireRole @DeleteMapping("/{nodeId}") public R delete(@PathVariable Long nodeId) { try { service.delete(nodeId); return R.ok(); } catch (IllegalArgumentException e) { return R.err(e.getMessage()); } }
}
