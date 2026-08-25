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
    @NotBlank @Size(min = 6, max = 72)
    private String code;
    @NotBlank @Size(min = 6, max = 72)
    @JsonAlias({"new_password", "password"})
    private String newPassword;
}
