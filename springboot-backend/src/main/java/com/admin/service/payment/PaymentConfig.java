package com.admin.service.payment;

import com.admin.entity.ViteConfig;
import com.admin.service.ViteConfigService;
import com.baomidou.mybatisplus.core.conditions.query.QueryWrapper;
import org.springframework.stereotype.Component;

@Component
public class PaymentConfig {
    private final ViteConfigService configs;

    public PaymentConfig(ViteConfigService configs) { this.configs = configs; }

    public String get(String name) { return get(name, ""); }

    public String get(String name, String fallback) {
        ViteConfig item = configs.getOne(new QueryWrapper<ViteConfig>().eq("name", name));
        if (item == null || item.getValue() == null || item.getValue().trim().isEmpty()) return fallback;
        return item.getValue().trim();
    }

    public boolean enabled(String provider) {
        return "true".equalsIgnoreCase(get("payment_" + provider + "_enabled", "false"));
    }
}
