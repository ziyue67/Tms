package com.admin.controller;

import com.admin.common.lang.R;
import com.admin.common.utils.JwtUtil;
import com.admin.service.SubscriptionService;
import org.springframework.web.bind.annotation.*;
import java.util.Map;

@RestController
@CrossOrigin
@RequestMapping("/api/v1/subscription")
public class SubscriptionController {
    private final SubscriptionService service;
    public SubscriptionController(SubscriptionService service) { this.service = service; }
    @GetMapping("/plans") public R plans() { return R.ok(service.publicPlans()); }
    @GetMapping("/current") public R current() { return R.ok(service.current(JwtUtil.getUserIdFromToken())); }
    @GetMapping("/dashboard") public R dashboard() { return R.ok(service.dashboard(JwtUtil.getUserIdFromToken())); }
    @PostMapping("/redeem") public R redeem(@RequestBody Map<String,String> body) { try { return R.ok(service.redeem(JwtUtil.getUserIdFromToken(), body.get("code"))); } catch (IllegalArgumentException e) { return R.err(e.getMessage()); } }
}
