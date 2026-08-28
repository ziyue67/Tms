package com.admin.service;

import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;

import com.admin.entity.RedeemCode;
import com.admin.entity.SubscriptionPlan;
import com.admin.entity.UserSubscription;
import com.admin.common.event.SubscriptionActivatedEvent;
import com.admin.mapper.ForwardMapper;
import com.admin.mapper.QuotaUsageLogMapper;
import com.admin.mapper.RedeemCodeMapper;
import com.admin.mapper.StatisticsFlowMapper;
import com.admin.mapper.SubscriptionPlanMapper;
import com.admin.mapper.UserMapper;
import com.admin.mapper.UserSubscriptionMapper;
import com.baomidou.mybatisplus.core.conditions.query.QueryWrapper;
import com.baomidou.mybatisplus.core.conditions.update.UpdateWrapper;
import org.springframework.context.ApplicationEventPublisher;

import java.time.Instant;
import java.time.ZoneOffset;
import java.util.List;

import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.isNull;
import static org.mockito.Mockito.doReturn;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertTrue;

@ExtendWith(MockitoExtension.class)
class SubscriptionServiceTest {
    @Mock SubscriptionPlanMapper plans;
    @Mock UserSubscriptionMapper subscriptions;
    @Mock RedeemCodeMapper codes;
    @Mock ForwardMapper forwards;
    @Mock UserMapper users;
    @Mock StatisticsFlowMapper statistics;
    @Mock QuotaUsageLogMapper quotaLogs;
    @Mock ApplicationEventPublisher eventPublisher;

    private SubscriptionService service;

    @BeforeEach
    void setUp() {
        service = new SubscriptionService(plans, subscriptions, codes, forwards, users, statistics, quotaLogs, eventPublisher);
    }

    private long utc(String value) {
        return Instant.parse(value).toEpochMilli();
    }

    @Test
    void resetOn31stUsesFebruaryLastDayInNormalYear() {
        long actual = SubscriptionService.calculateNextReset(utc("2026-01-31T12:00:00Z"), 31, ZoneOffset.UTC);
        assertEquals(utc("2026-02-28T00:00:00Z"), actual);
    }

    @Test
    void resetOn31stUsesFebruary29InLeapYear() {
        long actual = SubscriptionService.calculateNextReset(utc("2028-01-31T12:00:00Z"), 31, ZoneOffset.UTC);
        assertEquals(utc("2028-02-29T00:00:00Z"), actual);
    }

    @Test
    void resetOn31stUsesApril30() {
        long actual = SubscriptionService.calculateNextReset(utc("2026-04-01T12:00:00Z"), 31, ZoneOffset.UTC);
        assertEquals(utc("2026-04-30T00:00:00Z"), actual);
    }

    @Test
    void pastResetMovesToNextMonth() {
        long actual = SubscriptionService.calculateNextReset(utc("2026-05-31T01:00:00Z"), 31, ZoneOffset.UTC);
        assertEquals(utc("2026-06-30T00:00:00Z"), actual);
    }

    @Test
    void resetDayZeroDisablesPeriodicReset() {
        assertEquals(0L, SubscriptionService.calculateNextReset(utc("2026-05-31T01:00:00Z"), 0, ZoneOffset.UTC));
    }

    @Test
    void strictSubscriptionAccessRejectsUsersWithoutAPlan() {
        when(subscriptions.selectOne(any(QueryWrapper.class))).thenReturn(null);

        assertFalse(service.hasUsableSubscription(42L));
    }

    @Test
    void strictSubscriptionAccessAcceptsActivePlan() {
        UserSubscription active = new UserSubscription();
        active.setId(7L);
        active.setUserId(42L);
        active.setStatus(1);
        active.setExpiresAt(0L);
        when(subscriptions.selectOne(any(QueryWrapper.class))).thenReturn(active);

        assertTrue(service.hasUsableSubscription(42L));
    }

    @Test
    void permanentPlanActivatesWithNoExpiryAndNoReset() {
        SubscriptionPlan plan = plan(7L, "permanent", 1000L * 1024 * 1024 * 1024, 0, 0);
        when(plans.selectById(7L)).thenReturn(plan);
        when(subscriptions.selectOne(any(QueryWrapper.class))).thenReturn(null);
        when(subscriptions.insert(any(UserSubscription.class))).thenAnswer(invocation -> {
            invocation.getArgument(0, UserSubscription.class).setId(88L);
            return 1;
        });

        UserSubscription activated = service.activate(42L, 7L);

        assertEquals(0L, activated.getExpiresAt());
        assertEquals(1000L * 1024 * 1024 * 1024, activated.getTrafficLimitBytes());
        assertEquals(0L, activated.getNextResetAt());
        assertEquals(0L, activated.getTrafficUsedBytes());
        verify(eventPublisher).publishEvent(any(SubscriptionActivatedEvent.class));
    }

    @Test
    void redeemCodeActivatesPermanentOneTimePlan() {
        SubscriptionPlan plan = plan(9L, "permanent", 1000L * 1024 * 1024 * 1024, 0, 0);
        RedeemCode code = new RedeemCode();
        code.setId(11L);
        code.setPlanId(9L);
        code.setStatus(1);
        when(codes.selectOne(any(QueryWrapper.class))).thenReturn(code);
        when(plans.selectById(9L)).thenReturn(plan);
        doReturn(1).when(codes).update((RedeemCode) isNull(), any(UpdateWrapper.class));
        when(subscriptions.selectOne(any(QueryWrapper.class))).thenReturn(null);
        when(subscriptions.insert(any(UserSubscription.class))).thenAnswer(invocation -> {
            invocation.getArgument(0, UserSubscription.class).setId(99L);
            return 1;
        });

        UserSubscription activated = service.redeem(42L, "ABCDE-FGHIJ-KLMNO-PQRST");

        assertEquals(0L, activated.getExpiresAt());
        assertEquals(1000L * 1024 * 1024 * 1024, activated.getTrafficLimitBytes());
        assertEquals(0L, activated.getNextResetAt());
        verify(codes).update((RedeemCode) isNull(), any(UpdateWrapper.class));
    }

    @Test
    void oneTimeQuotaKeepsUsedTrafficAtPeriodicCheckpoint() {
        SubscriptionPlan plan = plan(12L, "year", 1000L * 1024 * 1024 * 1024, 31, 0);
        UserSubscription active = new UserSubscription();
        active.setId(4L);
        active.setUserId(42L);
        active.setPlanId(12L);
        active.setStatus(1);
        active.setExpiresAt(System.currentTimeMillis() + 86_400_000L);
        active.setTrafficUsedBytes(500L);
        active.setNextResetAt(1L);
        when(subscriptions.selectList(any(QueryWrapper.class))).thenReturn(List.of(active));
        when(plans.selectById(12L)).thenReturn(plan);

        service.resetDueSubscriptions();

        assertEquals(500L, active.getTrafficUsedBytes());
        assertTrue(active.getNextResetAt() > System.currentTimeMillis());
        verify(subscriptions).updateById(active);
    }

    private SubscriptionPlan plan(long id, String unit, long trafficBytes, int resetDay, int resetQuota) {
        SubscriptionPlan plan = new SubscriptionPlan();
        plan.setId(id);
        plan.setName("测试套餐");
        plan.setStatus(1);
        plan.setRedeemable(1);
        plan.setValidityValue(1);
        plan.setValidityUnit(unit);
        plan.setTrafficBytes(trafficBytes);
        plan.setResetDay(resetDay);
        plan.setResetQuota(resetQuota);
        plan.setMaxForwards(0);
        return plan;
    }
}
