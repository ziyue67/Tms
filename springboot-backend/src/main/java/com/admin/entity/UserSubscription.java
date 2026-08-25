package com.admin.entity;

import com.baomidou.mybatisplus.annotation.TableName;
import lombok.Data;

@Data
@TableName("user_subscription")
public class UserSubscription {
    private Long id;
    private Long userId;
    private Long planId;
    private Long startsAt;
    private Long expiresAt;
    private Long trafficLimitBytes;
    private Long trafficUsedBytes;
    private Long nextResetAt;
    private Integer maxForwards;
    private Integer usedForwards;
    private Integer status;
    private Long createdTime;
    private Long updatedTime;
}
