package com.admin.entity;

import com.baomidou.mybatisplus.annotation.TableName;
import lombok.Data;

@Data
@TableName("redeem_code")
public class RedeemCode {
    private Long id;
    private Long planId;
    private String codeHash;
    private String codePreview;
    private String batchId;
    private Integer status;
    private Long usedBy;
    private Long usedTime;
    private Long expiresAt;
    private String remark;
    private Long createdTime;
}
