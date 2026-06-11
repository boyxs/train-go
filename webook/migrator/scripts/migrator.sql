-- ── webook-migrator 控制库 schema ──────────────────────────
-- 用法：mysql -h <host> -u root -p < migrator.sql
-- 前置：CREATE DATABASE webook_migrator DEFAULT CHARSET utf8mb4 COLLATE utf8mb4_0900_ai_ci;
-- 关联：architecture.md §4.1 / PRD.md §8.2 数据依赖
-- 严格遵循 webook/CLAUDE.md「数据表规范」10 项

USE `webook_migrator`;

SET NAMES utf8mb4;
SET FOREIGN_KEY_CHECKS = 0;

-- ── task ───────────────────────────────────────────
DROP TABLE IF EXISTS `task`;
CREATE TABLE `task` (
  `id`              bigint        NOT NULL AUTO_INCREMENT COMMENT '主键',
  `name`            varchar(128)  NOT NULL                COMMENT '任务名（业务可读，全局唯一）',
  `mode`            varchar(16)   NOT NULL                COMMENT '模式：dual_write=应用层双写 / cdc=binlog 通道',
  `kind`            varchar(32)   NOT NULL                COMMENT '类型：cross_dc=跨机房 / sharding=分库分表 / schema=schema 演进 / heterogeneous=异构',
  `source_type`     varchar(32)   NOT NULL DEFAULT 'mysql' COMMENT '源类型：mysql / mongo（默认 mysql；不传按 MySQL 源处理）',
  `source_dsn_ref`  varchar(64)   NOT NULL                COMMENT '源 DSN 引用（Vault/Secret 路径，禁止明文入库）',
  `sink_type`       varchar(32)   NOT NULL                COMMENT '目标类型：mysql / es / clickhouse / mongo / tidb / kafka',
  `sink_dsn_ref`    varchar(64)   NOT NULL                COMMENT '目标 DSN 引用',
  `tables_json`     text          NOT NULL                COMMENT '涉及表 JSON：[{src,dst,partitionKey,filter,transform,sensitiveColumns}]',
  `status`          tinyint       NOT NULL DEFAULT 0      COMMENT '状态：0=created 1=full_running 2=full_done 3=incr_running 5=switched -1=failed',
  `gray_percent`    smallint      NOT NULL DEFAULT 0      COMMENT '灰度切读百分比 0-100',
  `consistency`     varchar(16)   NOT NULL DEFAULT 'eventual' COMMENT '一致性等级：eventual / read_after_write / strong',
  `created_at`      bigint        NOT NULL DEFAULT 0      COMMENT '创建时间（Unix 毫秒）',
  `updated_at`      bigint        NOT NULL DEFAULT 0      COMMENT '更新时间（Unix 毫秒）',
  `deleted_at`      bigint        NOT NULL DEFAULT 0      COMMENT '软删除时间（0=未删）',
  PRIMARY KEY (`id`) USING BTREE,
  UNIQUE INDEX `uni_task_name` (`name` ASC) USING BTREE,
  INDEX `idx_task_status` (`status` ASC) USING BTREE
) ENGINE = InnoDB CHARACTER SET = utf8mb4 COLLATE = utf8mb4_0900_ai_ci ROW_FORMAT = Dynamic COMMENT = '迁移任务定义';

-- ── checkpoint ─────────────────────────────────────
DROP TABLE IF EXISTS `checkpoint`;
CREATE TABLE `checkpoint` (
  `id`               bigint        NOT NULL AUTO_INCREMENT COMMENT '主键',
  `task_id`          bigint        NOT NULL                COMMENT '所属任务',
  `phase`            varchar(16)   NOT NULL                COMMENT '阶段：full / incr',
  `shard_no`         int           NOT NULL DEFAULT 0      COMMENT '分片编号（全量按 ID 分片，增量恒为 0）',
  `cursor_kind`      varchar(16)   NOT NULL                COMMENT '游标类型：id_range / binlog_pos / gtid',
  `cursor_value`     varchar(256)  NOT NULL DEFAULT ''     COMMENT '游标值 JSON',
  `progress_percent` decimal(5,2)  NOT NULL DEFAULT 0      COMMENT '进度百分比 0-100',
  `last_lag_ms`      bigint        NOT NULL DEFAULT 0      COMMENT '最近一次同步延迟（毫秒）',
  `version`          bigint        NOT NULL DEFAULT 0      COMMENT '乐观锁版本号',
  `updated_at`       bigint        NOT NULL DEFAULT 0      COMMENT '更新时间（Unix 毫秒）',
  PRIMARY KEY (`id`) USING BTREE,
  UNIQUE INDEX `uk_checkpoint_task_phase_shard` (`task_id`, `phase`, `shard_no`) USING BTREE
) ENGINE = InnoDB CHARACTER SET = utf8mb4 COLLATE = utf8mb4_0900_ai_ci ROW_FORMAT = Dynamic COMMENT = '迁移断点：全量分片游标 + 增量 binlog 位点';

-- ── validate_log ───────────────────────────────────
DROP TABLE IF EXISTS `validate_log`;
CREATE TABLE `validate_log` (
  `id`            bigint        NOT NULL AUTO_INCREMENT COMMENT '主键',
  `task_id`       bigint        NOT NULL                COMMENT '所属任务',
  `direction`     varchar(16)   NOT NULL                COMMENT '方向：src_to_dst / dst_to_src',
  `table_name`    varchar(64)   NOT NULL                COMMENT '表名',
  `biz_id`        varchar(64)   NOT NULL                COMMENT '业务主键（string：数值串 / Mongo ObjectID 等）',
  `mismatch_kind` varchar(32)   NOT NULL                COMMENT '差异类型：missing=目标缺 / extra=目标多 / diff=字段不一致',
  `diff_detail`   text                                   COMMENT 'JSON 差异详情（敏感字段 mask 后存）',
  `repaired`      tinyint       NOT NULL DEFAULT 0      COMMENT '是否已修复 0=否 1=是',
  `created_at`    bigint        NOT NULL DEFAULT 0      COMMENT '创建时间（Unix 毫秒）',
  `repaired_at`   bigint        NOT NULL DEFAULT 0      COMMENT '修复时间（Unix 毫秒）',
  PRIMARY KEY (`id`) USING BTREE,
  UNIQUE INDEX `uk_validate_log_dedup` (`task_id`, `table_name`, `biz_id`) USING BTREE,
  INDEX `idx_validate_log_task_repaired` (`task_id` ASC, `repaired` ASC) USING BTREE,
  INDEX `idx_validate_log_created`       (`created_at` ASC) USING BTREE
) ENGINE = InnoDB CHARACTER SET = utf8mb4 COLLATE = utf8mb4_0900_ai_ci ROW_FORMAT = Dynamic COMMENT = '迁移对账日志：同 (task,table,biz_id) 只保留一行；BatchInsert upsert 不覆盖 repaired';

-- ── audit_log ──────────────────────────────────────
DROP TABLE IF EXISTS `audit_log`;
CREATE TABLE `audit_log` (
  `id`         bigint         NOT NULL AUTO_INCREMENT COMMENT '主键',
  `task_id`    bigint         NOT NULL                COMMENT '所属任务',
  `actor`      varchar(64)    NOT NULL                COMMENT '操作者（用户名 / service account）',
  `action`     varchar(32)    NOT NULL                COMMENT '动作（扁平描述字符串，与 API stage 入参不同维度）：create/start/pause/set_gray/set_stage_SRC_FIRST/cutover_propose/cutover_approve/rollback/repair',
  `payload`    text                                   COMMENT 'JSON 入参（敏感字段 mask）',
  `result`     varchar(16)    NOT NULL                COMMENT 'success / fail',
  `error_msg`  varchar(512)                           COMMENT '失败原因（result=fail 时填）',
  `client_ip`  varchar(64)                            COMMENT '客户端 IP',
  `created_at` bigint         NOT NULL DEFAULT 0      COMMENT '操作时间（Unix 毫秒）',
  PRIMARY KEY (`id`) USING BTREE,
  INDEX `idx_audit_log_task_created` (`task_id` ASC, `created_at` ASC) USING BTREE,
  INDEX `idx_audit_log_actor`        (`actor` ASC, `created_at` ASC)  USING BTREE
) ENGINE = InnoDB CHARACTER SET = utf8mb4 COLLATE = utf8mb4_0900_ai_ci ROW_FORMAT = Dynamic COMMENT = '迁移操作审计（合规留存 1 年）';

-- ── dead_letter（死信队列） ──────────────────────────────────
DROP TABLE IF EXISTS `dead_letter`;
CREATE TABLE `dead_letter` (
  `id`           bigint         NOT NULL AUTO_INCREMENT COMMENT '主键',
  `task_id`      bigint         NOT NULL                COMMENT '所属任务',
  `op`           varchar(16)    NOT NULL                COMMENT '操作：insert / update / delete',
  `table_name`   varchar(64)    NOT NULL                COMMENT '源表名（=task.tables[].src，非目标表；replay 按它反查 tableIdx）',
  `biz_id`       varchar(64)    NOT NULL                COMMENT '业务主键（string：数值串 / Mongo ObjectID 等）',
  `payload`      text           NOT NULL                COMMENT 'JSON：完整 mutation 数据',
  `last_error`   varchar(1024)                          COMMENT '最后一次失败原因',
  `retry_count`  int            NOT NULL DEFAULT 0      COMMENT '已重试次数',
  `replayed`     tinyint        NOT NULL DEFAULT 0      COMMENT '是否已重放成功 0=否 1=是',
  `replay_failed` tinyint       NOT NULL DEFAULT 0      COMMENT '重放是否仍失败 0=否 1=是（需人工）',
  `created_at`   bigint         NOT NULL DEFAULT 0      COMMENT '入死信时间（Unix 毫秒）',
  `replayed_at`  bigint         NOT NULL DEFAULT 0      COMMENT '重放时间（Unix 毫秒）',
  PRIMARY KEY (`id`) USING BTREE,
  INDEX `idx_dead_letter_task_replayed` (`task_id` ASC, `replayed` ASC) USING BTREE,
  INDEX `idx_dead_letter_table_biz`     (`table_name` ASC, `biz_id` ASC) USING BTREE,
  INDEX `idx_dead_letter_created`       (`created_at` ASC) USING BTREE
) ENGINE = InnoDB CHARACTER SET = utf8mb4 COLLATE = utf8mb4_0900_ai_ci ROW_FORMAT = Dynamic COMMENT = '死信队列：双写失败兜底';

SET FOREIGN_KEY_CHECKS = 1;

-- ── 验证 ────────────────────────────────────────────────────
-- SELECT TABLE_NAME, TABLE_COMMENT FROM information_schema.TABLES
-- WHERE TABLE_SCHEMA = 'webook_migrator' ORDER BY TABLE_NAME;

-- 期望输出：
-- dead_letter             死信队列：双写失败兜底
-- audit_log     迁移操作审计（合规留存 1 年）
-- checkpoint    迁移断点：全量分片游标 + 增量 binlog 位点
-- task          迁移任务定义
-- validate_log  迁移对账日志：仅记录差异
