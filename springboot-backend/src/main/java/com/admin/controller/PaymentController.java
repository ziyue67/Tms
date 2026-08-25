package com.admin.controller;

import com.admin.common.lang.R;
import com.admin.common.utils.JwtUtil;
import com.admin.service.PaymentService;
import org.springframework.web.bind.annotation.*;
import java.util.Map;

@RestController @CrossOrigin @RequestMapping("/api/v1/payment")
public class PaymentController {
    private final PaymentService service;
    public PaymentController(PaymentService service){this.service=service;}
    @GetMapping("/providers") public R providers(){return R.ok(service.enabledProviders());}
    @PostMapping("/orders") public R create(@RequestBody Map<String,Object> body){try{return R.ok(service.create(JwtUtil.getUserIdFromToken(),Long.parseLong(body.get("planId").toString()),String.valueOf(body.getOrDefault("provider","manual"))));}catch(IllegalArgumentException e){return R.err(e.getMessage());}}
    @GetMapping("/orders/{orderNo}") public R get(@PathVariable String orderNo){return R.ok(service.get(JwtUtil.getUserIdFromToken(),orderNo));}
    @PostMapping("/{provider}/notify") public R callback(@PathVariable String provider,@RequestBody Map<String,String> body){try{return R.ok(service.callback(provider,body.get("orderNo"),body.get("tradeNo"),body.get("signature"),body.toString()));}catch(IllegalArgumentException e){return R.err(e.getMessage());}}
    @PostMapping("/stripe/webhook") public R stripe(@RequestBody Map<String,String> body){try{return R.ok(service.callback("stripe",body.get("orderNo"),body.get("tradeNo"),body.get("signature"),body.toString()));}catch(IllegalArgumentException e){return R.err(e.getMessage());}}
}
