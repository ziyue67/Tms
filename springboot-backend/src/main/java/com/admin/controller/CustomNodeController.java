package com.admin.controller;

import com.admin.common.annotation.RequireRole;
import com.admin.common.lang.R;
import com.admin.service.CustomNodeService;
import org.springframework.web.bind.annotation.*;
import java.util.Map;
import java.util.List;
import java.util.ArrayList;

@RestController @CrossOrigin @RequestMapping("/api/v1/custom-nodes")
public class CustomNodeController {
    private final CustomNodeService service;
    public CustomNodeController(CustomNodeService service) { this.service = service; }
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
    @RequireRole @PostMapping("/{nodeId}/disable") public R disable(@PathVariable Long nodeId) { service.disable(nodeId); return R.ok(); }
    /** Permanently remove the imported link and its legacy assignment rows. */
    @RequireRole @DeleteMapping("/{nodeId}") public R delete(@PathVariable Long nodeId) { service.delete(nodeId); return R.ok(); }
}
