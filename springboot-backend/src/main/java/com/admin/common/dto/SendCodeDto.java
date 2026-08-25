package com.admin.common.dto;

import lombok.Data;

import javax.validation.constraints.Email;
import javax.validation.constraints.NotBlank;

@Data
public class SendCodeDto {
    @NotBlank @Email
    private String email;
}
