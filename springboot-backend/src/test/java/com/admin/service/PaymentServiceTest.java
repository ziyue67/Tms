package com.admin.service;

import com.admin.mapper.PaymentOrderMapper;
import com.admin.mapper.SubscriptionPlanMapper;
import com.admin.service.payment.PaymentConfig;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;

import java.util.List;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.mockito.Mockito.verifyNoInteractions;
import static org.mockito.Mockito.when;

@ExtendWith(MockitoExtension.class)
class PaymentServiceTest {
    @Mock PaymentOrderMapper orders;
    @Mock SubscriptionPlanMapper plans;
    @Mock SubscriptionService subscriptions;
    @Mock PaymentConfig config;

    private PaymentService service;

    @BeforeEach
    void setUp() {
        service = new PaymentService(orders, plans, subscriptions, config, List.of());
    }

    @Test
    void disabledPaymentSystemRejectsNewOrdersAndProviders() {
        when(config.get("payment_enabled", "true")).thenReturn("false");

        IllegalArgumentException error = assertThrows(IllegalArgumentException.class,
                () -> service.createOrder(1L, 2L, "manual"));

        assertEquals("支付系统已关闭", error.getMessage());
        assertEquals(0, service.enabledProviders().size());
        verifyNoInteractions(plans, orders);
    }
}
