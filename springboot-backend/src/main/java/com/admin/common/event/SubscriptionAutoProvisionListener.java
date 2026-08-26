package com.admin.common.event;

import com.admin.service.InboundService;
import lombok.extern.slf4j.Slf4j;
import org.springframework.stereotype.Component;
import org.springframework.transaction.event.TransactionPhase;
import org.springframework.transaction.event.TransactionalEventListener;

/** Creates per-user metered credentials only after the subscription transaction commits. */
@Slf4j
@Component
public class SubscriptionAutoProvisionListener {

    private final InboundService inboundService;

    public SubscriptionAutoProvisionListener(InboundService inboundService) {
        this.inboundService = inboundService;
    }

    @TransactionalEventListener(phase = TransactionPhase.AFTER_COMMIT)
    public void onSubscriptionActivated(SubscriptionActivatedEvent event) {
        try {
            inboundService.provisionAutoTargetsForUser(event.userId());
        } catch (Exception e) {
            log.error("Automatic protocol provisioning failed for user {}", event.userId(), e);
        }
    }
}
