package com.admin.entity;

import com.baomidou.mybatisplus.annotation.TableName;
import com.fasterxml.jackson.databind.annotation.JsonSerialize;
import com.fasterxml.jackson.databind.ser.std.ToStringSerializer;
import lombok.Data;

@Data
@TableName("user_subscription")
public class UserSubscription {
    @JsonSerialize(using = ToStringSerializer.class)
    private Long id;
    @JsonSerialize(using = ToStringSerializer.class)
    private Long userId;
    @JsonSerialize(using = ToStringSerializer.class)
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
