package com.admin.entity;

import com.baomidou.mybatisplus.annotation.TableName;
import com.baomidou.mybatisplus.annotation.TableField;
import com.admin.common.typehandler.JsonTextTypeHandler;
import lombok.Data;

/** Immutable audit record for subscription quota changes. */
@Data
@TableName(value = "quota_usage_log", autoResultMap = true)
public class QuotaUsageLog {
    private Long id;
    private Long userId;
    private Long subscriptionId;
    private String eventType;
    private Long amount;
    @TableField(value = "metadata", typeHandler = JsonTextTypeHandler.class)
    private String metadata;
    private Long createdTime;
}
