package com.admin.controller;

import com.admin.common.lang.R;
import com.admin.common.utils.JwtUtil;
import com.admin.service.PaymentService;
import com.admin.service.payment.PaymentCallback;
import org.springframework.web.bind.annotation.*;

import javax.servlet.http.HttpServletRequest;
import java.util.Collections;
import java.util.Enumeration;
import java.util.LinkedHashMap;
import java.util.Map;

@RestController
@CrossOrigin
@RequestMapping("/api/v1/payment")
public class PaymentController {
    private final PaymentService service;
    public PaymentController(PaymentService service) { this.service = service; }

    @GetMapping("/providers") public R providers() { return R.ok(service.enabledProviders()); }

    @PostMapping("/orders")
    public R create(@RequestBody Map<String, Object> body) {
        try { return R.ok(service.createCheckout(JwtUtil.getUserIdFromToken(), Long.parseLong(body.get("planId").toString()), String.valueOf(body.getOrDefault("provider", "manual")))); }
        catch (IllegalArgumentException e) { return R.err(e.getMessage()); }
    }

    @GetMapping("/orders/{orderNo}") public R get(@PathVariable String orderNo) { return R.ok(service.get(JwtUtil.getUserIdFromToken(), orderNo)); }

    @PostMapping("/wechat/notify")
    public R wechat(@RequestBody String rawBody, HttpServletRequest request) { return callback("wechat", rawBody, Collections.emptyMap(), request); }

    @PostMapping("/stripe/webhook")
    public R stripe(@RequestBody String rawBody, HttpServletRequest request) { return callback("stripe", rawBody, Collections.emptyMap(), request); }

    @PostMapping("/{provider}/notify")
    public R callback(@PathVariable String provider, @RequestParam Map<String, String> values, HttpServletRequest request) { return callback(provider, "", values, request); }

    private R callback(String provider, String rawBody, Map<String, String> values, HttpServletRequest request) {
        try { return R.ok(service.callback(provider, new PaymentCallback(rawBody, values, headers(request)))); }
        catch (IllegalArgumentException e) { return R.err(e.getMessage()); }
    }

    private Map<String, String> headers(HttpServletRequest request) {
        Map<String, String> result = new LinkedHashMap<>();
        Enumeration<String> names = request.getHeaderNames();
        while (names.hasMoreElements()) { String name = names.nextElement(); result.put(name, request.getHeader(name)); }
        return result;
    }
}
