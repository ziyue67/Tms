package com.admin.service;

import org.junit.jupiter.api.Test;

import java.time.Instant;
import java.time.ZoneOffset;

import static org.junit.jupiter.api.Assertions.assertEquals;

class SubscriptionServiceTest {
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
}
