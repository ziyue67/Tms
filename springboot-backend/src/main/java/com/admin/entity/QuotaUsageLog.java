package com.admin.entity;

import com.baomidou.mybatisplus.annotation.TableName;
import lombok.Data;

/** Immutable audit record for subscription quota changes. */
@Data
@TableName("quota_usage_log")
public class QuotaUsageLog {
    private Long id;
    private Long userId;
    private Long subscriptionId;
    private String eventType;
    private Long amount;
    private String metadata;
    private Long createdTime;
}
