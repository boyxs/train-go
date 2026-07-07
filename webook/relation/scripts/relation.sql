-- webook-relation 用户关系库表结构（与 relation/repository/dao model 对齐；AutoMigrate 之外的真相源）。
-- 关注边用 status 翻转维护，无删除路径故 relation_follow/relation_stats 不加 deleted_at（同 interaction 计数表）。

DROP TABLE IF EXISTS `relation_follow`;
CREATE TABLE `relation_follow` (
  `id`          bigint  NOT NULL AUTO_INCREMENT COMMENT '主键',
  `follower_id` bigint  NOT NULL DEFAULT 0 COMMENT '关注者 uid',
  `followee_id` bigint  NOT NULL DEFAULT 0 COMMENT '被关注者 uid',
  `status`      tinyint NOT NULL DEFAULT 0 COMMENT '关注状态：1=关注中 0=已取关（翻转不物理删）',
  `created_at`  bigint  NOT NULL DEFAULT 0 COMMENT '首次关注时间（Unix 毫秒）',
  `updated_at`  bigint  NOT NULL DEFAULT 0 COMMENT '最近状态变更时间（Unix 毫秒）',
  PRIMARY KEY (`id`) USING BTREE,
  UNIQUE KEY `uk_relation_follow_edge` (`follower_id`, `followee_id`) USING BTREE,
  KEY `idx_relation_follow_er` (`follower_id`, `status`) USING BTREE COMMENT '我的关注列表；InnoDB 聚簇 id 已覆盖游标',
  KEY `idx_relation_follow_ee` (`followee_id`, `status`) USING BTREE COMMENT '我的粉丝列表 + feed 写扩散拉粉丝'
) ENGINE = InnoDB CHARACTER SET = utf8mb4 COLLATE = utf8mb4_0900_ai_ci ROW_FORMAT = Dynamic COMMENT = '用户关注边（单向 follower→followee；status 翻转维护）';

DROP TABLE IF EXISTS `relation_stats`;
CREATE TABLE `relation_stats` (
  `id`           bigint NOT NULL AUTO_INCREMENT COMMENT '主键',
  `uid`          bigint NOT NULL DEFAULT 0 COMMENT '用户 uid',
  `followee_cnt` bigint NOT NULL DEFAULT 0 COMMENT '关注数：该用户关注了多少人',
  `follower_cnt` bigint NOT NULL DEFAULT 0 COMMENT '粉丝数：多少人关注该用户',
  `created_at`   bigint NOT NULL DEFAULT 0 COMMENT '创建时间（Unix 毫秒）',
  `updated_at`   bigint NOT NULL DEFAULT 0 COMMENT '更新时间（Unix 毫秒）',
  PRIMARY KEY (`id`) USING BTREE,
  UNIQUE KEY `uk_relation_stats_uid` (`uid`) USING BTREE
) ENGINE = InnoDB CHARACTER SET = utf8mb4 COLLATE = utf8mb4_0900_ai_ci ROW_FORMAT = Dynamic COMMENT = '用户关系聚合计数（每用户一行，关注/取关翻转时维护）';

DROP TABLE IF EXISTS `relation_block`;
CREATE TABLE `relation_block` (
  `id`          bigint NOT NULL AUTO_INCREMENT COMMENT '主键',
  `uid`         bigint NOT NULL DEFAULT 0 COMMENT '拉黑发起者 uid',
  `blocked_uid` bigint NOT NULL DEFAULT 0 COMMENT '被拉黑者 uid',
  `created_at`  bigint NOT NULL DEFAULT 0 COMMENT '拉黑时间（Unix 毫秒）',
  PRIMARY KEY (`id`) USING BTREE,
  UNIQUE KEY `uk_relation_block_edge` (`uid`, `blocked_uid`) USING BTREE,
  KEY `idx_relation_block_uid` (`uid`) USING BTREE COMMENT '黑名单列表（取消拉黑=物理删）'
) ENGINE = InnoDB CHARACTER SET = utf8mb4 COLLATE = utf8mb4_0900_ai_ci ROW_FORMAT = Dynamic COMMENT = '用户拉黑黑名单';
