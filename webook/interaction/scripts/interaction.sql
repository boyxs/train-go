-- webook 互动服务表结构（点赞/收藏/浏览/计数）
-- 同步自 interaction/repository/dao/interaction.go，改表必须同步此文件
-- 通用限界上下文：按 (biz, biz_id) 标识业务对象，不认 article/comment，新增可互动实体 = 新 biz 值

DROP TABLE IF EXISTS `interaction`;

-- 互动聚合表（每个业务对象一行，聚合阅读/点赞/收藏数）
CREATE TABLE `interaction` (
  `id`            bigint      NOT NULL AUTO_INCREMENT COMMENT '主键',
  `biz`           varchar(64) NOT NULL DEFAULT ''     COMMENT '业务类型：article/comment 等',
  `biz_id`        bigint      NOT NULL DEFAULT 0      COMMENT '业务对象 id（文章 id / 评论 id 等）',
  `read_count`    bigint      NOT NULL DEFAULT 0      COMMENT '阅读数',
  `like_count`    bigint      NOT NULL DEFAULT 0      COMMENT '点赞数',
  `collect_count` bigint      NOT NULL DEFAULT 0      COMMENT '收藏数',
  `created_at`    bigint      NOT NULL DEFAULT 0      COMMENT '创建时间（Unix 毫秒）',
  `updated_at`    bigint      NOT NULL DEFAULT 0      COMMENT '更新时间（Unix 毫秒）',
  -- 无 deleted_at：interaction 无删除路径，不用 gorm soft_delete（否则 SELECT 注入 deleted_at=0 会排除既有 NULL 行）
  PRIMARY KEY (`id`) USING BTREE,
  UNIQUE INDEX `uk_interaction_biz` (`biz` ASC, `biz_id` ASC) USING BTREE
) ENGINE=InnoDB CHARACTER SET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci ROW_FORMAT=Dynamic COMMENT='互动聚合表（每个业务对象一行，聚合阅读/点赞/收藏数）';

DROP TABLE IF EXISTS `user_interaction`;

-- 用户互动关系（每个 user × biz × biz_id 一行，记录点赞/收藏状态）
CREATE TABLE `user_interaction` (
  `id`         bigint      NOT NULL AUTO_INCREMENT COMMENT '主键',
  `biz`        varchar(64) NOT NULL DEFAULT ''     COMMENT '业务类型',
  `biz_id`     bigint      NOT NULL DEFAULT 0      COMMENT '业务对象 id',
  `user_id`    bigint      NOT NULL DEFAULT 0      COMMENT '用户 id',
  `liked`      tinyint(1)  NOT NULL DEFAULT 0      COMMENT '是否点赞：0=否 1=是',
  `collected`  tinyint(1)  NOT NULL DEFAULT 0      COMMENT '是否收藏：0=否 1=是',
  `created_at` bigint      NOT NULL DEFAULT 0      COMMENT '创建时间（Unix 毫秒）',
  `updated_at` bigint      NOT NULL DEFAULT 0      COMMENT '更新时间（Unix 毫秒）',
  -- 无 deleted_at：无删除路径，不用 gorm soft_delete（避免 deleted_at=0 过滤排除既有 NULL 行）
  PRIMARY KEY (`id`) USING BTREE,
  UNIQUE INDEX `uk_user_interaction_biz` (`biz` ASC, `biz_id` ASC, `user_id` ASC) USING BTREE
) ENGINE=InnoDB CHARACTER SET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci ROW_FORMAT=Dynamic COMMENT='用户互动关系（每个 user × biz 一行，记录点赞/收藏状态）';
