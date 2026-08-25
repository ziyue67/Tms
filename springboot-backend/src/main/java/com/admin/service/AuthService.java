package com.admin.service;

import com.admin.common.dto.RegisterDto;
import com.admin.common.lang.R;
import com.admin.common.utils.JwtUtil;
import com.admin.entity.User;
import com.baomidou.mybatisplus.core.conditions.query.QueryWrapper;
import com.baomidou.mybatisplus.extension.service.IService;
import org.springframework.security.crypto.bcrypt.BCryptPasswordEncoder;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.util.HashMap;
import java.util.Map;

@Service
public class AuthService {
    private final IService<User> users;
    private final EmailVerificationService verification;
    private final ViteConfigService configs;
    private final BCryptPasswordEncoder passwordEncoder = new BCryptPasswordEncoder();

    public AuthService(com.admin.service.impl.UserServiceImpl users,
                       EmailVerificationService verification,
                       ViteConfigService configs) {
        this.users = users;
        this.verification = verification;
        this.configs = configs;
    }

    public void sendRegisterCode(String email, String clientIp) {
        String normalized = EmailVerificationService.normalize(email);
        if (users.getOne(new QueryWrapper<User>().eq("email", normalized)) != null) {
            throw new IllegalArgumentException("邮箱已注册");
        }
        verification.send(normalized, clientIp);
    }

    @Transactional
    public R register(RegisterDto dto) {
        String email = EmailVerificationService.normalize(dto.getEmail());
        if (users.getOne(new QueryWrapper<User>().eq("email", email)) != null) return R.err("邮箱已注册");
        String username = dto.getUsername() == null ? "" : dto.getUsername().trim();
        if (username.isEmpty()) username = email.substring(0, email.indexOf('@'));
        if (users.getOne(new QueryWrapper<User>().eq("user", username)) != null) return R.err("用户名已存在");
        if (!verification.consume(email, dto.getCode())) return R.err("验证码错误或已过期");

        User user = new User();
        user.setUser(username);
        user.setEmail(email);
        user.setPwd(passwordEncoder.encode(dto.getPassword()));
        user.setRoleId(1);
        user.setStatus(1);
        user.setFlow(0L);
        user.setInFlow(0L);
        user.setOutFlow(0L);
        user.setNum(0);
        user.setCreatedTime(System.currentTimeMillis());
        user.setUpdatedTime(System.currentTimeMillis());
        users.save(user);
        Map<String, Object> result = new HashMap<>();
        result.put("token", JwtUtil.generateToken(user));
        result.put("name", user.getUser());
        result.put("role_id", user.getRoleId());
        return R.ok(result);
    }

    public Map<String, Object> publicConfig() {
        Map<String, Object> result = new HashMap<>();
        com.admin.entity.ViteConfig captcha = configs.getOne(new QueryWrapper<com.admin.entity.ViteConfig>().eq("name", "captcha_enabled"));
        result.put("captchaEnabled", captcha != null && "true".equalsIgnoreCase(captcha.getValue()));
        result.put("registrationEnabled", true);
        return result;
    }
}
