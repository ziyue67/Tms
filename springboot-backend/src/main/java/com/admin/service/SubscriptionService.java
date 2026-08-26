package com.admin.service;

import com.admin.common.lang.R;
import com.admin.entity.*;
import com.admin.mapper.*;
import com.admin.entity.StatisticsFlow;
import com.baomidou.mybatisplus.core.conditions.query.QueryWrapper;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.security.SecureRandom;
import java.time.Instant;
import java.time.ZoneId;
import java.time.ZonedDateTime;
import java.util.*;

@Service
public class SubscriptionService {
    private final SubscriptionPlanMapper plans;
    private final UserSubscriptionMapper subscriptions;
    private final RedeemCodeMapper codes;
    private final ForwardMapper forwards;
    private final UserMapper users;
    private final StatisticsFlowMapper statistics;
    private final QuotaUsageLogMapper quotaLogs;
    private final SecureRandom random = new SecureRandom();

    public SubscriptionService(SubscriptionPlanMapper plans, UserSubscriptionMapper subscriptions, RedeemCodeMapper codes, ForwardMapper forwards, UserMapper users, StatisticsFlowMapper statistics, QuotaUsageLogMapper quotaLogs) {
        this.plans = plans; this.subscriptions = subscriptions; this.codes = codes; this.forwards = forwards; this.users = users; this.statistics = statistics; this.quotaLogs = quotaLogs;
    }

    public List<SubscriptionPlan> publicPlans() {
        return plans.selectList(new QueryWrapper<SubscriptionPlan>()
                .and(w -> w.isNull("status").or().eq("status", 1))
                .and(w -> w.isNull("for_sale").or().eq("for_sale", 1))
                .orderByAsc("sort_order", "id"));
    }

    public List<RedeemCode> redeemCodes(Long planId, Integer status) {
        QueryWrapper<RedeemCode> query = new QueryWrapper<RedeemCode>().orderByDesc("id");
        if (planId != null) query.eq("plan_id", planId);
        if (status != null) query.eq("status", status);
        return codes.selectList(query);
    }

    public List<QuotaUsageLog> auditLogs(long userId) {
        return quotaLogs.selectList(new QueryWrapper<QuotaUsageLog>().eq("user_id", userId).orderByDesc("id").last("limit 100"));
    }

    public UserSubscription current(long userId) {
        UserSubscription item = subscriptions.selectOne(new QueryWrapper<UserSubscription>().eq("user_id", userId).eq("status", 1).orderByDesc("id").last("limit 1"));
        return enrich(item);
    }

    /** Administrative lifecycle operations must still see a disabled subscription record. */
    public UserSubscription latest(long userId) {
        return enrich(subscriptions.selectOne(new QueryWrapper<UserSubscription>().eq("user_id", userId).orderByDesc("id").last("limit 1")));
    }

    /** Subscription links must stop advertising an account once its quota or expiry is reached. */
    public boolean isSubscriptionUsable(long userId) {
        UserSubscription item = latest(userId);
        if (item == null) return true;
        if (item.getStatus() != null && item.getStatus() != 1) return false;
        return quotaLimitError(userId) == null;
    }

    private UserSubscription enrich(UserSubscription item) {
        if (item == null || item.getPlanId() == null) return item;
        SubscriptionPlan plan = plans.selectById(item.getPlanId());
        if (plan != null) { item.setPlanName(plan.getName()); item.setPlanDescription(plan.getDescription()); item.setPlanValidityValue(plan.getValidityValue()); item.setPlanValidityUnit(plan.getValidityUnit()); }
        return item;
    }

    @Transactional
    public UserSubscription adjust(long userId, Map<String, Object> values) {
        UserSubscription item = latest(userId);
        if (item == null) throw new IllegalArgumentException("该用户没有套餐记录，请先分配套餐");
        long now = System.currentTimeMillis();
        if (values.containsKey("planId")) {
            long planId = number(values.get("planId"));
            SubscriptionPlan plan = plans.selectById(planId);
            if (plan == null) throw new IllegalArgumentException("套餐不存在");
            item.setPlanId(planId);
        }
        if (values.containsKey("expiresAt")) item.setExpiresAt(number(values.get("expiresAt")));
        if (values.containsKey("trafficLimitBytes")) item.setTrafficLimitBytes(number(values.get("trafficLimitBytes")));
        if (values.containsKey("trafficUsedBytes")) item.setTrafficUsedBytes(number(values.get("trafficUsedBytes")));
        if (values.containsKey("nextResetAt")) item.setNextResetAt(number(values.get("nextResetAt")));
        if (values.containsKey("maxForwards")) item.setMaxForwards((int) number(values.get("maxForwards")));
        if (values.containsKey("status")) item.setStatus((int) number(values.get("status")));
        item.setUpdatedTime(now);
        subscriptions.updateById(item);
        audit(item, "manual_adjust", 0L, "admin");
        return item;
    }

    @Transactional
    public void remove(long userId) {
        UserSubscription item = latest(userId);
        if (item == null) throw new IllegalArgumentException("该用户没有套餐记录");
        if (subscriptions.deleteById(item.getId()) != 1) throw new IllegalArgumentException("删除用户套餐失败");
    }

    @Transactional
    public UserSubscription resetQuota(long userId) {
        UserSubscription item = latest(userId);
        if (item == null) throw new IllegalArgumentException("该用户没有有效套餐");
        SubscriptionPlan plan = plans.selectById(item.getPlanId());
        long now = System.currentTimeMillis();
        item.setTrafficUsedBytes(0L);
        item.setNextResetAt(nextReset(now, plan == null ? 1 : plan.getResetDay()));
        item.setUpdatedTime(now);
        subscriptions.updateById(item);
        audit(item, "manual_reset", 0L, "admin");
        return item;
    }

    public void revokeCode(long id) {
        RedeemCode code = codes.selectById(id);
        if (code == null) throw new IllegalArgumentException("兑换码不存在");
        if (code.getStatus() != null && code.getStatus() == 0) throw new IllegalArgumentException("已使用的兑换码不能作废");
        code.setStatus(-1);
        codes.updateById(code);
    }

    /** Called by flow reporting; only a current, non-expired subscription accrues usage. */
    @Transactional
    public void recordTrafficUsage(long userId, long bytes) {
        if (bytes <= 0) return;
        UserSubscription active = current(userId);
        if (active == null || active.getExpiresAt() == null || (active.getExpiresAt() > 0 && active.getExpiresAt() <= System.currentTimeMillis())) return;
        resetIfDue(active, System.currentTimeMillis());
        com.baomidou.mybatisplus.core.conditions.update.UpdateWrapper<UserSubscription> update = new com.baomidou.mybatisplus.core.conditions.update.UpdateWrapper<>();
        update.eq("id", active.getId()).setSql("traffic_used_bytes = traffic_used_bytes + " + bytes).set("updated_time", System.currentTimeMillis());
        subscriptions.update(null, update);
        audit(active, "traffic", bytes, null);
    }

    @Transactional
    public void resetDueSubscriptions() {
        long now = System.currentTimeMillis();
        for (UserSubscription active : subscriptions.selectList(new QueryWrapper<UserSubscription>().eq("status", 1).le("next_reset_at", now))) resetIfDue(active, now);
    }

    private void resetIfDue(UserSubscription active, long now) {
        if (active.getNextResetAt() == null || active.getNextResetAt() <= 0 || active.getNextResetAt() > now) return;
        SubscriptionPlan plan = plans.selectById(active.getPlanId());
        boolean resetQuota = plan == null || plan.getResetQuota() == null || plan.getResetQuota() == 1;
        if (resetQuota) active.setTrafficUsedBytes(0L);
        active.setNextResetAt(nextReset(now, plan == null ? 1 : plan.getResetDay())); active.setUpdatedTime(now);
        subscriptions.updateById(active);
        audit(active, resetQuota ? "traffic_reset" : "traffic_period_checkpoint", 0L, resetQuota ? "quota_restored" : "quota_preserved");
    }

    public String quotaLimitError(long userId) {
        UserSubscription active = current(userId);
        if (active == null) return null;
        long now = System.currentTimeMillis(); resetIfDue(active, now);
        if (active.getExpiresAt() != null && active.getExpiresAt() > 0 && active.getExpiresAt() <= now) return "套餐已到期";
        if (active.getTrafficLimitBytes() != null && active.getTrafficLimitBytes() > 0 && active.getTrafficUsedBytes() != null && active.getTrafficUsedBytes() >= active.getTrafficLimitBytes()) return "套餐流量已用尽";
        return null;
    }

    public String forwardLimitError(long userId) {
        UserSubscription active = current(userId);
        if (active == null) return null;
        String quota = quotaLimitError(userId);
        if (quota != null) return quota + "，无法创建转发";
        if (active.getMaxForwards() != null && active.getMaxForwards() > 0) {
            long count = forwards.selectCount(new QueryWrapper<com.admin.entity.Forward>().eq("user_id", userId).ne("status", -1));
            if (count >= active.getMaxForwards()) return "已达到套餐转发数量上限";
        }
        return null;
    }

    public Map<String, Object> dashboard(long userId) {
        UserSubscription active = current(userId);
        User user = users.selectById(userId);
        Map<String, Object> result = new LinkedHashMap<>();
        long used = active == null || active.getTrafficUsedBytes() == null ? 0 : active.getTrafficUsedBytes();
        long limit = active == null || active.getTrafficLimitBytes() == null ? 0 : active.getTrafficLimitBytes();
        result.put("totalTrafficBytes", limit); result.put("usedTrafficBytes", used); result.put("remainingTrafficBytes", limit > 0 ? Math.max(0, limit - used) : 0);
        result.put("expiresAt", active == null ? null : active.getExpiresAt()); result.put("nextResetAt", active == null ? null : active.getNextResetAt());
        long forwardCount = forwards.selectCount(new QueryWrapper<com.admin.entity.Forward>().eq("user_id", userId).ne("status", -1));
        result.put("forwardCount", forwardCount); result.put("forwardLimit", active == null ? 0 : active.getMaxForwards()); result.put("planId", active == null ? null : active.getPlanId());
        result.put("planName", active == null ? null : active.getPlanName()); result.put("planDescription", active == null ? null : active.getPlanDescription());
        result.put("validityValue", active == null ? null : active.getPlanValidityValue()); result.put("validityUnit", active == null ? null : active.getPlanValidityUnit());
        long cutoff = System.currentTimeMillis() - 24L * 60 * 60 * 1000;
        List<StatisticsFlow> hourly = statistics.selectList(new QueryWrapper<StatisticsFlow>().eq("user_id", userId).ge("created_time", cutoff).orderByAsc("created_time"));
        result.put("last24Hours", hourly); result.put("accountUsedTrafficBytes", user == null ? 0 : safe(user.getInFlow()) + safe(user.getOutFlow()));
        return result;
    }

    @Transactional
    public UserSubscription activate(long userId, long planId) {
        SubscriptionPlan plan = plans.selectById(planId);
        if (plan == null || !enabled(plan.getStatus())) throw new IllegalArgumentException("套餐不存在或已停用");
        long now = System.currentTimeMillis();
        UserSubscription old = latest(userId);
        long start = old != null && old.getExpiresAt() != null && old.getExpiresAt() > now ? old.getExpiresAt() : now;
        ZonedDateTime z = Instant.ofEpochMilli(start).atZone(ZoneId.systemDefault());
        boolean permanent = "permanent".equalsIgnoreCase(plan.getValidityUnit());
        ZonedDateTime expiry = permanent ? null : ("year".equalsIgnoreCase(plan.getValidityUnit()) ? z.plusYears(plan.getValidityValue()) : z.plusMonths(plan.getValidityValue()));
        UserSubscription item = old == null ? new UserSubscription() : old;
        item.setUserId(userId); item.setPlanId(planId); item.setStartsAt(now); item.setExpiresAt(permanent ? 0L : expiry.toInstant().toEpochMilli());
        item.setTrafficLimitBytes(plan.getTrafficBytes()); item.setTrafficUsedBytes(0L); item.setNextResetAt(nextReset(now, plan.getResetDay()));
        item.setMaxForwards(plan.getMaxForwards()); item.setUsedForwards(old == null ? 0 : old.getUsedForwards()); item.setStatus(1); item.setUpdatedTime(now);
        if (old == null) { item.setCreatedTime(now); subscriptions.insert(item); } else subscriptions.updateById(item);
        audit(item, "plan_activated", 0L, "planId=" + planId);
        return enrich(item);
    }

    @Transactional
    public String generateCode(long planId, String batch, Long expiresAt) {
        String raw = randomCode();
        RedeemCode item = new RedeemCode(); item.setPlanId(planId); item.setCodeHash(sha256(raw)); item.setCodeValue(raw); item.setCodePreview(raw.substring(0, 4) + "****");
        item.setBatchId(batch); item.setStatus(1); item.setExpiresAt(expiresAt); item.setCreatedTime(System.currentTimeMillis()); codes.insert(item);
        return raw;
    }

    @Transactional
    public UserSubscription redeem(long userId, String rawCode) {
        if (rawCode == null || rawCode.trim().isEmpty()) throw new IllegalArgumentException("请输入兑换码");
        RedeemCode item = codes.selectOne(new QueryWrapper<RedeemCode>().eq("code_hash", sha256(rawCode)).eq("status", 1).last("limit 1"));
        long now = System.currentTimeMillis();
        if (item == null || (item.getExpiresAt() != null && item.getExpiresAt() > 0 && item.getExpiresAt() < now)) throw new IllegalArgumentException("兑换码无效、已使用或已过期");
        SubscriptionPlan plan = plans.selectById(item.getPlanId());
        if (plan == null || !enabled(plan.getStatus()) || !enabled(plan.getRedeemable())) throw new IllegalArgumentException("该兑换码对应套餐不可兑换");
        com.baomidou.mybatisplus.core.conditions.update.UpdateWrapper<RedeemCode> consume = new com.baomidou.mybatisplus.core.conditions.update.UpdateWrapper<RedeemCode>()
                .eq("id", item.getId()).eq("status", 1).set("status", 0).set("used_by", userId).set("used_time", now);
        if (codes.update(null, consume) != 1) throw new IllegalArgumentException("兑换码已被使用");
        UserSubscription activated = activate(userId, item.getPlanId());
        audit(activated, "redeem", 0L, "codeId=" + item.getId());
        return activated;
    }

    private long nextReset(long from, Integer day) {
        return calculateNextReset(from, day == null ? 1 : day, ZoneId.systemDefault());
    }

    static long calculateNextReset(long from, int day, ZoneId zone) {
        if (day <= 0) return 0L;
        int requestedDay = Math.min(31, Math.max(1, day));
        ZonedDateTime now = Instant.ofEpochMilli(from).atZone(zone);
        ZonedDateTime candidate = resetAtMonth(now, requestedDay);
        if (!candidate.isAfter(now)) candidate = resetAtMonth(now.plusMonths(1), requestedDay);
        return candidate.toInstant().toEpochMilli();
    }

    /** Uses the last calendar day when a month does not have the configured day. */
    private static ZonedDateTime resetAtMonth(ZonedDateTime month, int requestedDay) {
        int actualDay = Math.min(requestedDay, month.toLocalDate().lengthOfMonth());
        return month.withDayOfMonth(actualDay).withHour(0).withMinute(0).withSecond(0).withNano(0);
    }
    private long safe(Long value) { return value == null ? 0L : value; }
    /** NULL flags are from databases created before the commerce migration and mean the old enabled default. */
    private boolean enabled(Integer value) { return value == null || value == 1; }
    private long number(Object value) {
        if (value == null) throw new IllegalArgumentException("套餐调整参数不能为空");
        try { return Long.parseLong(String.valueOf(value)); }
        catch (NumberFormatException e) { throw new IllegalArgumentException("套餐调整参数格式错误"); }
    }
    private void audit(UserSubscription subscription, String eventType, long amount, String metadata) {
        if (subscription == null || subscription.getUserId() == null) return;
        QuotaUsageLog log = new QuotaUsageLog();
        log.setUserId(subscription.getUserId()); log.setSubscriptionId(subscription.getId()); log.setEventType(eventType); log.setAmount(amount); log.setMetadata(jsonMetadata(metadata)); log.setCreatedTime(System.currentTimeMillis());
        quotaLogs.insert(log);
    }
    private String jsonMetadata(String value) {
        if (value == null) return null;
        return "{\"detail\":\"" + value.replace("\\", "\\\\").replace("\"", "\\\"").replace("\n", "\\n").replace("\r", "\\r") + "\"}";
    }
    private String randomCode() { String alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"; StringBuilder b = new StringBuilder(); for (int i=0;i<20;i++) { if (i>0 && i%5==0) b.append('-'); b.append(alphabet.charAt(random.nextInt(alphabet.length()))); } return b.toString(); }
    private String sha256(String value) { try { byte[] d=MessageDigest.getInstance("SHA-256").digest(value.trim().toUpperCase(Locale.ROOT).getBytes(StandardCharsets.UTF_8)); StringBuilder b=new StringBuilder(); for(byte x:d)b.append(String.format("%02x",x)); return b.toString(); } catch(Exception e){ throw new IllegalStateException(e); } }
}
