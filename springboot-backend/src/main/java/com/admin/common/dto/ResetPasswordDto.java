package com.admin.common.dto;

import com.fasterxml.jackson.annotation.JsonAlias;
import lombok.Data;

import javax.validation.constraints.Email;
import javax.validation.constraints.NotBlank;
import javax.validation.constraints.Size;

@Data
public class ResetPasswordDto {
    @NotBlank @Email
    private String email;
    /** Legacy six-digit verification code. */
    @Size(min = 6, max = 256)
    private String code;
    /** sub2api-compatible one-time reset token from the email link. */
    @Size(min = 16, max = 256)
    private String token;
    @javax.validation.constraints.NotBlank @Size(min = 6, max = 72)
    @JsonAlias({"new_password", "password"})
    private String newPassword;

    public String credential() {
        return token != null && !token.isBlank() ? token.trim() : code;
    }
}
