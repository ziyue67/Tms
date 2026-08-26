package com.admin.common.event;

/** Published after a subscription is activated, redeemed, or renewed. */
public record SubscriptionActivatedEvent(long userId) {
}
