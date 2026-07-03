-- webook 评论服务表结构
-- 同步自 comment/repository/dao/comment.go，改表必须同步此文件
-- 点赞/热度由 interaction 服务（biz="comment"）负责，本服务不存点赞数据

DROP TABLE IF EXISTS `comment`;

CREATE TABLE `comment` (
  `id`         bigint        NOT NULL AUTO_INCREMENT COMMENT '主键',
  `biz`        varchar(32)   NOT NULL DEFAULT ''     COMMENT '业务类型：article（P0 仅此）',
  `biz_id`     bigint        NOT NULL DEFAULT 0      COMMENT '业务对象 ID（文章 ID）',
  `uid`        bigint        NOT NULL DEFAULT 0      COMMENT '评论者用户 ID',
  `root_id`    bigint        NOT NULL DEFAULT 0      COMMENT '根评论 ID（一级评论=0；回复继承祖先 root_id）',
  `pid`        bigint                 DEFAULT NULL   COMMENT '父评论 ID（一级评论为 NULL；自关联外键）',
  `content`    varchar(1000) NOT NULL DEFAULT ''     COMMENT '评论内容（业务限 ≤500 字）',
  `reply_cnt`  bigint        NOT NULL DEFAULT 0      COMMENT '回复数：一级评论=整楼回复数（写/删回复增减楼根）；楼内回复恒 0',
  `created_at` bigint        NOT NULL DEFAULT 0      COMMENT '创建时间（Unix 毫秒）',
  `updated_at` bigint        NOT NULL DEFAULT 0      COMMENT '更新时间（Unix 毫秒）',
  `deleted_at` bigint        NOT NULL DEFAULT 0      COMMENT '软删除时间（0=未删）',
  PRIMARY KEY (`id`) USING BTREE,
  INDEX `idx_comment_biz_root` (`biz` ASC, `biz_id` ASC, `root_id` ASC, `id` ASC) USING BTREE,
  INDEX `idx_comment_root` (`root_id` ASC, `id` ASC) USING BTREE,
  INDEX `idx_comment_uid` (`uid` ASC) USING BTREE,
  INDEX `idx_comment_pid` (`pid` ASC) USING BTREE,
  -- 自关联外键：pid 引用本表 id。软删（UPDATE deleted_at）不触发 CASCADE；
  -- 级联由 DAO 显式实现：删一级评论整楼软删，删楼内回复保留其子回复。
  CONSTRAINT `fk_comment_parent` FOREIGN KEY (`pid`) REFERENCES `comment` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB CHARACTER SET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci ROW_FORMAT=Dynamic COMMENT='评论（盖楼无限嵌套：root_id 标记楼，pid 直接父+自关联外键；点赞走 interaction）';
