package com.admin.entity;

import com.baomidou.mybatisplus.annotation.TableName;
import lombok.Data;

@Data
@TableName("payment_order")
public class PaymentOrder {
    private Long id;
    private String orderNo;
    private Long userId;
    private Long planId;
    private String provider;
    private java.math.BigDecimal amount;
    private String currency;
    private String status;
    private String providerTradeNo;
    private String callbackPayload;
    private Long paidAt;
    private Long createdTime;
    private Long updatedTime;
}
