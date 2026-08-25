package com.admin.service.payment;

import com.admin.entity.PaymentOrder;
import com.admin.entity.SubscriptionPlan;

/** Provider-specific checkout creation and signed callback validation. */
public interface PaymentProviderAdapter {
    String key();
    PaymentCheckout createCheckout(PaymentOrder order, SubscriptionPlan plan);
    String orderNo(PaymentCallback callback);
    String tradeNo(PaymentCallback callback);
    void verify(PaymentOrder order, PaymentCallback callback);
}
