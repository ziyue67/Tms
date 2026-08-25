package com.admin.controller;

import com.admin.common.dto.RegisterDto;
import com.admin.common.dto.SendCodeDto;
import com.admin.common.dto.LoginDto;
import com.admin.common.dto.ResetPasswordDto;
import com.admin.common.lang.R;
import com.admin.service.AuthService;
import com.admin.service.UserService;
import org.springframework.validation.annotation.Validated;
import org.springframework.web.bind.annotation.*;

import javax.servlet.http.HttpServletRequest;

@RestController
@CrossOrigin
@RequestMapping("/api/v1/auth")
public class AuthController {
    private final AuthService authService;
    private final UserService userService;

    public AuthController(AuthService authService, UserService userService) { this.authService = authService; this.userService = userService; }

    /** Compatibility login endpoint for account clients; existing /user/login remains unchanged. */
    @PostMapping("/login")
    public R login(@Validated @RequestBody LoginDto dto) { return userService.login(dto); }

    @PostMapping("/send-register-code")
    public R sendRegisterCode(@Validated @RequestBody SendCodeDto dto, HttpServletRequest request) {
        try {
            authService.sendRegisterCode(dto.getEmail(), clientIp(request));
            return R.ok();
        } catch (IllegalArgumentException e) {
            return R.err(e.getMessage());
        }
    }

    @PostMapping({"/send-reset-code", "/forgot-password"})
    public R sendResetCode(@Validated @RequestBody SendCodeDto dto, HttpServletRequest request) {
        try {
            authService.sendResetCode(dto.getEmail(), clientIp(request));
            // Same response for known and unknown email addresses prevents enumeration.
            return R.ok();
        } catch (IllegalArgumentException e) {
            // Keep rate-limit/configuration errors useful without exposing account existence.
            return R.err(e.getMessage());
        }
    }

    @PostMapping("/reset-password")
    public R resetPassword(@Validated @RequestBody ResetPasswordDto dto) {
        try { return authService.resetPassword(dto); }
        catch (IllegalArgumentException e) { return R.err(e.getMessage()); }
    }

    @PostMapping("/register")
    public R register(@Validated @RequestBody RegisterDto dto) {
        try { return authService.register(dto); }
        catch (IllegalArgumentException e) { return R.err(e.getMessage()); }
    }

    @GetMapping("/config")
    public R config() { return R.ok(authService.publicConfig()); }

    private String clientIp(HttpServletRequest request) {
        return request.getRemoteAddr();
    }
}
