package com.admin.common.task;

import com.admin.common.utils.GostUtil;
import com.admin.entity.Forward;
import com.admin.entity.Inbound;
import com.admin.entity.InboundLine;
import com.admin.entity.InboundUser;
import com.admin.entity.Tunnel;
import com.admin.entity.UserSubscription;
import com.admin.mapper.ForwardMapper;
import com.admin.mapper.InboundLineMapper;
import com.admin.mapper.InboundMapper;
import com.admin.mapper.InboundUserMapper;
import com.admin.mapper.TunnelMapper;
import com.admin.mapper.UserSubscriptionMapper;
import com.admin.service.SubscriptionService;
import com.baomidou.mybatisplus.core.conditions.query.QueryWrapper;
import org.springframework.scheduling.annotation.Scheduled;
import org.springframework.stereotype.Component;

/** Keeps already-connected protocol and relay services aligned with package access. */
@Component
public class SubscriptionAccessTask {
    private final SubscriptionService subscriptions;
    private final UserSubscriptionMapper subscriptionMapper;
    private final InboundUserMapper inboundUsers;
    private final InboundMapper inbounds;
    private final InboundLineMapper lines;
    private final ForwardMapper forwards;
    private final TunnelMapper tunnels;

    public SubscriptionAccessTask(SubscriptionService subscriptions, UserSubscriptionMapper subscriptionMapper,
                                  InboundUserMapper inboundUsers, InboundMapper inbounds,
                                  InboundLineMapper lines, ForwardMapper forwards, TunnelMapper tunnels) {
        this.subscriptions = subscriptions;
        this.subscriptionMapper = subscriptionMapper;
        this.inboundUsers = inboundUsers;
        this.inbounds = inbounds;
        this.lines = lines;
        this.forwards = forwards;
        this.tunnels = tunnels;
    }

    @Scheduled(cron = "30 * * * * ?")
    public void synchronizeProtocolAccess() {
        for (UserSubscription subscription : subscriptionMapper.selectList(new QueryWrapper<UserSubscription>())) {
            boolean usable = subscription.getStatus() != null && subscription.getStatus() == 1
                    && subscriptions.isSubscriptionUsable(subscription.getUserId());
            for (InboundUser credential : inboundUsers.selectList(new QueryWrapper<InboundUser>()
                    .eq("user_id", subscription.getUserId()))) {
                if (credential.getGostForwardId() == null) continue;
                Forward forward = forwards.selectById(credential.getGostForwardId());
                Inbound inbound = inbounds.selectById(credential.getInboundId());
                if (forward == null || inbound == null) continue;
                Tunnel tunnel = tunnels.selectById(forward.getTunnelId());
                if (tunnel == null) continue;
                String serviceName = forward.getId() + "_" + forward.getUserId() + "_0";
                if (!usable) {
                    if (forward.getStatus() == null || forward.getStatus() != 0) {
                        GostUtil.PauseService(tunnel.getInNodeId(), serviceName);
                        if (tunnel.getType() == 2) GostUtil.PauseRemoteService(tunnel.getOutNodeId(), serviceName);
                        forward.setStatus(0);
                        forwards.updateById(forward);
                    }
                    continue;
                }
                // A line explicitly disabled by the administrator must remain disabled.
                if (!lineEnabled(subscription.getUserId(), inbound)) continue;
                if (forward.getStatus() == null || forward.getStatus() != 1) {
                    GostUtil.ResumeService(tunnel.getInNodeId(), serviceName);
                    if (tunnel.getType() == 2) GostUtil.ResumeRemoteService(tunnel.getOutNodeId(), serviceName);
                    forward.setStatus(1);
                    forwards.updateById(forward);
                }
            }
        }
    }

    private boolean lineEnabled(Long userId, Inbound inbound) {
        QueryWrapper<InboundLine> query = new QueryWrapper<InboundLine>()
                .eq("user_id", userId).eq("node_id", inbound.getNodeId());
        if (inbound.getLandingId() == null) query.isNull("landing_id");
        else query.eq("landing_id", inbound.getLandingId());
        InboundLine line = lines.selectOne(query.last("limit 1"));
        return line == null || line.getStatus() == null || line.getStatus() == 1;
    }
}
