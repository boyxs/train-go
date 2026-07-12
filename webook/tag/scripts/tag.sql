-- webook-tag 标签库表结构（与 tag/repository/dao model 对齐；AutoMigrate 之外的真相源）。webook 库。
-- MVP 硬删风格，三表均无 deleted_at：tag/tagging 的 untag 走物理删、tag_follow 走 status 翻转。

DROP TABLE IF EXISTS `tag`;
CREATE TABLE `tag` (
  `id`           bigint       NOT NULL AUTO_INCREMENT COMMENT '主键',
  `name`         varchar(30)  NOT NULL DEFAULT '' COMMENT '标签展示名（CJK 原样保留）',
  `slug`         varchar(30)  NOT NULL DEFAULT '' COMMENT 'URL 友好标识（归一：小写/trim/空白折叠/剥 path 不安全字符）',
  `type`         varchar(16)  NOT NULL DEFAULT 'topic' COMMENT '标签命名空间：topic=内容主题（默认）',
  `description`  varchar(255) NOT NULL DEFAULT '' COMMENT '标签描述',
  `ref_count`    bigint       NOT NULL DEFAULT 0 COMMENT '内容引用数（多少对象打了此标签，SyncByBiz 事务内 GREATEST 维护）',
  `follow_count` bigint       NOT NULL DEFAULT 0 COMMENT '关注数（多少用户关注此标签，tag_follow 翻转时 GREATEST 维护）',
  `created_at`   bigint       NOT NULL DEFAULT 0 COMMENT '创建时间（Unix 毫秒）',
  `updated_at`   bigint       NOT NULL DEFAULT 0 COMMENT '更新时间（Unix 毫秒）',
  PRIMARY KEY (`id`) USING BTREE,
  UNIQUE KEY `uk_tag_slug` (`slug`, `type`) USING BTREE COMMENT '同命名空间内 slug 唯一（并发建同名靠此兜底 + Upsert DoNothing）',
  KEY `idx_tag_type_refcount` (`type`, `ref_count`) USING BTREE COMMENT 'typeahead / 热门标签按 type + ref_count 排序'
) ENGINE = InnoDB CHARACTER SET = utf8mb4 COLLATE = utf8mb4_0900_ai_ci ROW_FORMAT = Dynamic COMMENT = '通用标签本体（与被标注对象解耦；MVP 硬删风格、无 deleted_at）';

DROP TABLE IF EXISTS `tagging`;
CREATE TABLE `tagging` (
  `id`         bigint      NOT NULL AUTO_INCREMENT COMMENT '主键',
  `tag_id`     bigint      NOT NULL DEFAULT 0 COMMENT '标签 id（tag.id）',
  `biz`        varchar(32) NOT NULL DEFAULT '' COMMENT '被标注对象业务类型（如 article；user/conversation 已预留）',
  `biz_id`     bigint      NOT NULL DEFAULT 0 COMMENT '被标注对象 id',
  `source`     varchar(16) NOT NULL DEFAULT 'author' COMMENT '标注来源：author=作者手打 / ai=AI 推荐',
  `created_at` bigint      NOT NULL DEFAULT 0 COMMENT '创建时间（Unix 毫秒；BizIdsByTag 按此倒序）',
  `updated_at` bigint      NOT NULL DEFAULT 0 COMMENT '更新时间（Unix 毫秒）',
  PRIMARY KEY (`id`) USING BTREE,
  UNIQUE KEY `uk_tagging_dedup` (`biz`, `biz_id`, `tag_id`) USING BTREE COMMENT '同一对象同一标签唯一（untag=物理删，无软删幽灵冲突）',
  KEY `idx_tagging_tag_biz` (`tag_id`, `biz`) USING BTREE COMMENT '按标签取某 biz 的对象（标签下文章 / 本周新增计数）',
  KEY `idx_tagging_target` (`biz`, `biz_id`) USING BTREE COMMENT '取某对象的全部标签（详情回显 / 列表批量）'
) ENGINE = InnoDB CHARACTER SET = utf8mb4 COLLATE = utf8mb4_0900_ai_ci ROW_FORMAT = Dynamic COMMENT = '通用多态标签关联（biz+biz_id 多态；untag 物理删）';

DROP TABLE IF EXISTS `tag_follow`;
CREATE TABLE `tag_follow` (
  `id`         bigint  NOT NULL AUTO_INCREMENT COMMENT '主键',
  `uid`        bigint  NOT NULL DEFAULT 0 COMMENT '关注者 uid',
  `tag_id`     bigint  NOT NULL DEFAULT 0 COMMENT '被关注标签 id（tag.id）',
  `status`     tinyint NOT NULL DEFAULT 0 COMMENT '关注状态：1=关注中 0=已取关（翻转不物理删）',
  `created_at` bigint  NOT NULL DEFAULT 0 COMMENT '首次关注时间（Unix 毫秒）',
  `updated_at` bigint  NOT NULL DEFAULT 0 COMMENT '最近状态变更时间（Unix 毫秒）',
  PRIMARY KEY (`id`) USING BTREE,
  UNIQUE KEY `uk_tag_follow_edge` (`uid`, `tag_id`) USING BTREE COMMENT '一个用户对一个标签一条边（FOR UPDATE 翻转）',
  KEY `idx_tag_follow_uid` (`uid`, `status`) USING BTREE COMMENT '我关注的标签列表'
) ENGINE = InnoDB CHARACTER SET = utf8mb4 COLLATE = utf8mb4_0900_ai_ci ROW_FORMAT = Dynamic COMMENT = '用户关注标签边（uid→tag_id；status 翻转维护）';
