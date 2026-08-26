package com.admin.entity;

import com.baomidou.mybatisplus.annotation.TableName;
import com.fasterxml.jackson.databind.annotation.JsonSerialize;
import com.fasterxml.jackson.databind.ser.std.ToStringSerializer;
import lombok.Data;

@Data
@TableName("redeem_code")
public class RedeemCode {
    @JsonSerialize(using = ToStringSerializer.class)
    private Long id;
    @JsonSerialize(using = ToStringSerializer.class)
    private Long planId;
    private String codeHash;
    /** The original code is restricted to the administrator-only redemption-code API. */
    private String codeValue;
    private String codePreview;
    private String batchId;
    private Integer status;
    private Long usedBy;
    private Long usedTime;
    private Long expiresAt;
    private String remark;
    private Long createdTime;
}
