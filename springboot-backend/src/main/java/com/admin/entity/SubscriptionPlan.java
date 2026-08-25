package com.admin.entity;

import com.baomidou.mybatisplus.annotation.TableName;
import lombok.Data;

@Data
@TableName("subscription_plan")
public class SubscriptionPlan {
    private Long id;
    private String name;
    private String description;
    private java.math.BigDecimal price;
    private String currency;
    private Integer validityValue;
    private String validityUnit;
    private Long trafficBytes;
    private Integer resetDay;
    private Integer maxForwards;
    private Integer forSale;
    private Integer redeemable;
    private Integer sortOrder;
    private Integer status;
    private Long createdTime;
    private Long updatedTime;
}
