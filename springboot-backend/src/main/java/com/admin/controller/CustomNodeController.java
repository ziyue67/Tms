package com.admin.controller;

import com.admin.common.annotation.RequireRole;
import com.admin.common.lang.R;
import com.admin.service.CustomNodeService;
import org.springframework.web.bind.annotation.*;
import java.util.Map;

@RestController @CrossOrigin @RequestMapping("/api/v1/custom-nodes")
public class CustomNodeController {
    private final CustomNodeService service;
    public CustomNodeController(CustomNodeService service) { this.service = service; }
    @RequireRole @GetMapping public R list() { return R.ok(service.listWithAssignments()); }
    @RequireRole @PostMapping public R importNode(@RequestBody Map<String,String> body) { try { return R.ok(service.importVless(body.get("name"), body.get("link"))); } catch (IllegalArgumentException e) { return R.err(e.getMessage()); } }
    @RequireRole @PostMapping("/{nodeId}/assign") public R assign(@PathVariable Long nodeId, @RequestBody Map<String,Object> body) { try { service.assign(nodeId, Long.valueOf(String.valueOf(body.get("userId")))); return R.ok(); } catch (IllegalArgumentException e) { return R.err(e.getMessage()); } }
    @RequireRole @DeleteMapping("/{nodeId}/assign/{userId}") public R unassign(@PathVariable Long nodeId, @PathVariable Long userId) { service.unassign(nodeId, userId); return R.ok(); }
    @RequireRole @DeleteMapping("/{nodeId}") public R disable(@PathVariable Long nodeId) { service.disable(nodeId); return R.ok(); }
}
