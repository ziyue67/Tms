package com.admin.controller;

import com.admin.common.annotation.RequireRole;
import com.admin.common.lang.R;
import com.admin.service.EmailVerificationService;
import org.springframework.web.bind.annotation.*;
import java.util.Map;

@RestController @CrossOrigin @RequestMapping("/api/v1/admin/email")
public class AdminEmailController {
    private final EmailVerificationService service;
    public AdminEmailController(EmailVerificationService service) { this.service = service; }
    @RequireRole @PostMapping("/test")
    public R test(@RequestBody Map<String,String> body) { try { service.sendTest(body.get("email")); return R.ok(); } catch (IllegalArgumentException e) { return R.err(e.getMessage()); } }
    @RequireRole @GetMapping("/audit")
    public R audit(@RequestParam(defaultValue = "50") int limit) { return R.ok(service.audit(limit)); }
    @RequireRole @GetMapping("/health")
    public R health() { return R.ok(service.health()); }
}
