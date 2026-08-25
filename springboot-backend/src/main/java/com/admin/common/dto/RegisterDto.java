package com.admin.common.dto;

import lombok.Data;

import javax.validation.constraints.Email;
import javax.validation.constraints.NotBlank;
import javax.validation.constraints.Size;

@Data
public class RegisterDto {
    @NotBlank @Email
    private String email;
    @NotBlank @Size(min = 6, max = 72)
    private String password;
    @NotBlank
    private String code;
    private String username;
}
