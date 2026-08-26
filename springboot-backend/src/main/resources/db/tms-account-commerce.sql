-- TMS account, subscription and commerce migration (MySQL 5.7+).
-- Safe to rerun after gost.sql. All statements are additive for existing installs.
SET @tms_schema = DATABASE();
SET @tms_sql = (SELECT IF(COUNT(*) = 0,
  'ALTER TABLE `user` ADD COLUMN `email` varchar(190) DEFAULT NULL COMMENT ''注册邮箱''',
  'SELECT 1') FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = @tms_schema AND TABLE_NAME = 'user' AND COLUMN_NAME = 'email');
PREPARE tms_stmt FROM @tms_sql; EXECUTE tms_stmt; DEALLOCATE PREPARE tms_stmt;
SET @tms_sql = (SELECT IF(COUNT(*) = 0,
  'ALTER TABLE `user` ADD UNIQUE KEY `uk_user_email` (`email`)',
  'SELECT 1') FROM information_schema.STATISTICS WHERE TABLE_SCHEMA = @tms_schema AND TABLE_NAME = 'user' AND INDEX_NAME = 'uk_user_email');
PREPARE tms_stmt FROM @tms_sql; EXECUTE tms_stmt; DEALLOCATE PREPARE tms_stmt;

CREATE TABLE IF NOT EXISTS `subscription_plan` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `name` varchar(100) NOT NULL,
  `description` varchar(500) DEFAULT NULL,
  `price` decimal(12,2) NOT NULL DEFAULT 0,
  `currency` varchar(10) NOT NULL DEFAULT 'CNY',
  `validity_value` int NOT NULL,
  `validity_unit` varchar(10) NOT NULL DEFAULT 'month',
  `traffic_bytes` bigint NOT NULL DEFAULT 0,
  `reset_day` tinyint NOT NULL DEFAULT 1 COMMENT '每月 1-31 日，不存在的日期取当月最后一天',
  `reset_quota` tinyint NOT NULL DEFAULT 1 COMMENT '1=每月恢复完整额度，0=有效期内总量不恢复',
  `max_forwards` int NOT NULL DEFAULT 0,
  `for_sale` tinyint NOT NULL DEFAULT 1,
  `redeemable` tinyint NOT NULL DEFAULT 1,
  `sort_order` int NOT NULL DEFAULT 0,
  `status` tinyint NOT NULL DEFAULT 1,
  `created_time` bigint NOT NULL,
  `updated_time` bigint NOT NULL,
  PRIMARY KEY (`id`), KEY `idx_plan_status` (`status`,`for_sale`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

SET @tms_sql = (SELECT IF(COUNT(*) = 0,
  'ALTER TABLE `subscription_plan` ADD COLUMN `reset_quota` tinyint NOT NULL DEFAULT 1 COMMENT ''1=每月恢复完整额度，0=有效期内总量不恢复'' AFTER `reset_day`',
  'SELECT 1') FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = @tms_schema AND TABLE_NAME = 'subscription_plan' AND COLUMN_NAME = 'reset_quota');
PREPARE tms_stmt FROM @tms_sql; EXECUTE tms_stmt; DEALLOCATE PREPARE tms_stmt;

CREATE TABLE IF NOT EXISTS `user_subscription` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `user_id` bigint NOT NULL,
  `plan_id` bigint NOT NULL,
  `starts_at` bigint NOT NULL,
  `expires_at` bigint NOT NULL,
  `traffic_limit_bytes` bigint NOT NULL DEFAULT 0,
  `traffic_used_bytes` bigint NOT NULL DEFAULT 0,
  `next_reset_at` bigint DEFAULT NULL,
  `max_forwards` int NOT NULL DEFAULT 0,
  `used_forwards` int NOT NULL DEFAULT 0,
  `status` tinyint NOT NULL DEFAULT 1,
  `created_time` bigint NOT NULL,
  `updated_time` bigint NOT NULL,
  PRIMARY KEY (`id`), UNIQUE KEY `uk_user_active_subscription` (`user_id`), KEY `idx_subscription_expiry` (`status`,`expires_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS `redeem_code` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `plan_id` bigint NOT NULL,
  `code_hash` char(64) NOT NULL,
  `code_preview` varchar(20) NOT NULL,
  `batch_id` varchar(64) DEFAULT NULL,
  `status` tinyint NOT NULL DEFAULT 1,
  `used_by` bigint DEFAULT NULL,
  `used_time` bigint DEFAULT NULL,
  `expires_at` bigint DEFAULT NULL,
  `remark` varchar(255) DEFAULT NULL,
  `created_time` bigint NOT NULL,
  PRIMARY KEY (`id`), UNIQUE KEY `uk_redeem_hash` (`code_hash`), KEY `idx_redeem_status` (`status`,`plan_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS `quota_usage_log` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `user_id` bigint NOT NULL,
  `subscription_id` bigint DEFAULT NULL,
  `event_type` varchar(32) NOT NULL,
  `amount` bigint NOT NULL DEFAULT 0,
  `metadata` json DEFAULT NULL,
  `created_time` bigint NOT NULL,
  PRIMARY KEY (`id`), KEY `idx_quota_user_time` (`user_id`,`created_time`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS `payment_order` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `order_no` varchar(64) NOT NULL,
  `user_id` bigint NOT NULL,
  `plan_id` bigint NOT NULL,
  `provider` varchar(20) NOT NULL,
  `amount` decimal(12,2) NOT NULL,
  `currency` varchar(10) NOT NULL DEFAULT 'CNY',
  `status` varchar(20) NOT NULL DEFAULT 'pending',
  `provider_trade_no` varchar(128) DEFAULT NULL,
  `callback_payload` text,
  `paid_at` bigint DEFAULT NULL,
  `created_time` bigint NOT NULL,
  `updated_time` bigint NOT NULL,
  PRIMARY KEY (`id`), UNIQUE KEY `uk_payment_order_no` (`order_no`), KEY `idx_payment_user` (`user_id`,`created_time`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS `custom_node` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `name` varchar(255) NOT NULL,
  `protocol` varchar(32) NOT NULL,
  `raw_link` text NOT NULL,
  `parsed_json` text NOT NULL,
  `visibility` varchar(12) NOT NULL DEFAULT 'global',
  `status` tinyint NOT NULL DEFAULT 1,
  `created_time` bigint NOT NULL,
  `updated_time` bigint NOT NULL,
  PRIMARY KEY (`id`), KEY `idx_custom_node_status` (`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS `user_custom_node` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `user_id` bigint NOT NULL,
  `custom_node_id` bigint NOT NULL,
  `status` tinyint NOT NULL DEFAULT 1,
  `created_time` bigint NOT NULL,
  PRIMARY KEY (`id`), UNIQUE KEY `uk_user_custom_node` (`user_id`,`custom_node_id`), KEY `idx_custom_node_user` (`custom_node_id`,`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
