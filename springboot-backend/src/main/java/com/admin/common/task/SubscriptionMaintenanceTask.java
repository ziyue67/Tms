package com.admin.common.task;

import com.admin.service.SubscriptionService;
import org.springframework.scheduling.annotation.Scheduled;
import org.springframework.stereotype.Component;

/** Resets subscription-cycle traffic after the configured monthly reset day. */
@Component
public class SubscriptionMaintenanceTask {
    private final SubscriptionService subscriptions;
    public SubscriptionMaintenanceTask(SubscriptionService subscriptions) { this.subscriptions = subscriptions; }
    @Scheduled(cron = "15 * * * * ?")
    public void resetDueUsageCycles() { subscriptions.resetDueSubscriptions(); }
}
