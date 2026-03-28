/*
 Navicat Premium Dump SQL

 Source Server         : localhost_3306
 Source Server Type    : MySQL
 Source Server Version : 80045 (8.0.45)
 Source Host           : localhost:3306
 Source Schema         : webook

 Target Server Type    : MySQL
 Target Server Version : 80045 (8.0.45)
 File Encoding         : 65001

 Date: 28/03/2026 13:00:11
*/

SET NAMES utf8mb4;
SET FOREIGN_KEY_CHECKS = 0;

-- ----------------------------
-- Table structure for article
-- ----------------------------
DROP TABLE IF EXISTS `article`;
CREATE TABLE `article`  (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `title` longtext CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NULL,
  `content` longtext CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NULL,
  `author_id` bigint NULL DEFAULT NULL,
  `status` tinyint UNSIGNED NULL DEFAULT NULL,
  `created_at` datetime(3) NULL DEFAULT NULL,
  `updated_at` datetime(3) NULL DEFAULT NULL,
  PRIMARY KEY (`id`) USING BTREE,
  INDEX `idx_article_author_id`(`author_id` ASC) USING BTREE
) ENGINE = InnoDB AUTO_INCREMENT = 22 CHARACTER SET = utf8mb4 COLLATE = utf8mb4_0900_ai_ci ROW_FORMAT = Dynamic;

-- ----------------------------
-- Records of article
-- ----------------------------
INSERT INTO `article` VALUES (1, 'Go 并发编程入门', 'goroutine 是 Go 语言最核心的并发原语。不同于操作系统线程，goroutine 由 Go runtime 调度，初始栈仅 2KB，创建成本极低。配合 channel 可以实现优雅的 CSP 并发模型...', 1, 2, '2026-03-01 10:00:00.000', '2026-03-01 10:00:00.000');
INSERT INTO `article` VALUES (2, 'GORM 使用技巧总结', '总结了 GORM 的常见查询模式：Where 链式调用、Preload 预加载、Scopes 复用查询条件。事务处理推荐用 Transaction 回调而非手动 Begin/Commit...', 1, 2, '2026-03-02 14:30:00.000', '2026-03-02 14:30:00.000');
INSERT INTO `article` VALUES (3, 'Redis 缓存策略详解', 'Cache-Aside 是最常用的缓存模式：读时先查缓存，miss 则查 DB 并回填；写时先更新 DB，再删缓存。Write-Through 则由缓存层代理写入...', 1, 1, '2026-03-03 09:15:00.000', '2026-03-03 09:15:00.000');
INSERT INTO `article` VALUES (4, 'JWT 双 Token 认证实践', 'Access Token 设置短过期（30分钟），Refresh Token 设置长过期（7天）。前端在 401 时自动用 Refresh Token 换取新 Access Token，实现无感刷新...', 1, 2, '2026-03-04 16:00:00.000', '2026-03-04 16:00:00.000');
INSERT INTO `article` VALUES (5, 'Next.js App Router 迁移指南', '从 Pages Router 迁移到 App Router 的关键变化：文件系统路由从 pages/ 改为 app/，布局用 layout.tsx，数据获取从 getServerSideProps 改为 Server Components...', 1, 1, '2026-03-05 11:20:00.000', '2026-03-05 11:20:00.000');
INSERT INTO `article` VALUES (6, 'React 19 新特性速览', 'React 19 引入了 use() hook 用于读取 Promise 和 Context，Actions 简化了表单提交，useOptimistic 支持乐观更新。Server Components 成为一等公民...', 1, 2, '2026-03-06 08:45:00.000', '2026-03-06 08:45:00.000');
INSERT INTO `article` VALUES (7, 'Ant Design 表单最佳实践', 'Form.useForm() 获取表单实例，Form.Item 的 name 属性对应字段路径。校验用 rules 数组，异步校验返回 Promise。setFieldsValue 用于回填数据...', 1, 1, '2026-03-07 13:00:00.000', '2026-03-07 13:00:00.000');
INSERT INTO `article` VALUES (8, 'Docker 容器化部署 Go 应用', '多阶段构建：第一阶段用 golang:alpine 编译，第二阶段用 scratch 或 alpine 作为运行镜像，最终镜像仅 10-20MB。docker-compose 编排 MySQL + Redis + App...', 1, 2, '2026-03-08 15:30:00.000', '2026-03-08 15:30:00.000');
INSERT INTO `article` VALUES (9, 'MySQL 索引优化实战', '慢查询日志开启后，用 EXPLAIN 分析执行计划。复合索引遵循最左前缀原则，覆盖索引避免回表。避免在索引列上使用函数或隐式类型转换...', 1, 1, '2026-03-09 10:00:00.000', '2026-03-09 10:00:00.000');
INSERT INTO `article` VALUES (10, 'Gin 中间件开发指南', '中间件签名 func(c *gin.Context)，用 c.Next() 传递到下一个处理器，c.Abort() 中断链。常见中间件：日志记录、JWT 鉴权、限流、CORS、Recovery...', 1, 2, '2026-03-10 17:00:00.000', '2026-03-10 17:00:00.000');
INSERT INTO `article` VALUES (11, 'TypeScript 类型体操入门', '从基础泛型 T 到条件类型 T extends U ? X : Y，再到 infer 推断和模板字面量类型。Utility Types 如 Partial、Required、Pick、Omit 是日常必备...', 1, 3, '2026-03-11 09:30:00.000', '2026-03-11 09:30:00.000');
INSERT INTO `article` VALUES (12, 'Wire 依赖注入详解', 'Wire 是 Google 的编译时依赖注入工具。Provider 提供依赖，Injector 组装依赖图。wire.Build() 声明绑定关系，wire gen 生成代码，零运行时开销...', 1, 2, '2026-03-12 14:00:00.000', '2026-03-12 14:00:00.000');
INSERT INTO `article` VALUES (13, '前后端联调规范', 'API 响应统一 {code, msg, data} 格式。认证用 x-access-token / x-refresh-token header。前端 axios 拦截器处理 token 刷新和错误提示...', 1, 1, '2026-03-13 11:00:00.000', '2026-03-13 11:00:00.000');
INSERT INTO `article` VALUES (14, 'Go 单元测试与 Mock', 'testify/assert 提供丰富断言，testify/suite 支持测试套件。go.uber.org/mock 生成 interface mock，表驱动测试是 Go 社区标准模式...', 1, 2, '2026-03-14 16:30:00.000', '2026-03-14 16:30:00.000');
INSERT INTO `article` VALUES (15, 'Tailwind CSS 4.0 迁移笔记', 'v4 重大变化：配置从 JS 文件改为 CSS-first（@import tailwindcss），移除 postcss 和 autoprefixer 依赖。@apply 仍可用但推荐直接写 class...', 1, 1, '2026-03-15 08:00:00.000', '2026-03-15 08:00:00.000');
INSERT INTO `article` VALUES (16, '分布式限流算法对比', '令牌桶允许突发流量，漏桶平滑输出，滑动窗口精确控制。Redis + Lua 脚本实现原子操作，避免竞态条件。生产环境推荐滑动窗口...', 1, 2, '2026-03-16 13:45:00.000', '2026-03-16 13:45:00.000');
INSERT INTO `article` VALUES (17, 'ESLint 9 Flat Config 实践', 'ESLint 9 废弃 .eslintrc，改用 eslint.config.mjs 扁平配置。next/core-web-vitals 原生支持 flat config，prettier 必须放在最后覆盖格式化规则...', 1, 1, '2026-03-17 10:30:00.000', '2026-03-17 10:30:00.000');
INSERT INTO `article` VALUES (18, 'Go 错误处理哲学', 'Go 不用异常，用返回值。errors.Is 判断错误链，errors.As 提取特定类型。fmt.Errorf + %w 包装错误保留上下文。sentinel error 定义在包级别...', 1, 2, '2026-03-18 15:00:00.000', '2026-03-18 15:00:00.000');
INSERT INTO `article` VALUES (19, 'Kubernetes 入门：部署第一个应用', 'Pod 是最小调度单元，Deployment 管理副本集，Service 暴露网络。kubectl apply -f 声明式部署，ConfigMap 和 Secret 管理配置...', 1, 3, '2026-03-19 09:00:00.000', '2026-03-19 09:00:00.000');
INSERT INTO `article` VALUES (20, '小微书项目架构复盘', '回顾整个项目的分层设计：Handler → Service → Repository → DAO/Cache。Wire DI 解耦依赖，双表设计分离制作库和线上库。前端 Next.js App Router + antd...', 1, 2, '2026-03-20 17:30:00.000', '2026-03-20 17:30:00.000');
INSERT INTO `article` VALUES (21, '如何写好技术文章', '好的技术文章需要：明确的目标读者、清晰的问题定义、循序渐进的讲解、可运行的代码示例、踩坑记录和最佳实践总结。标题要具体不要模糊...', 1, 1, '2026-03-21 12:00:00.000', '2026-03-21 12:00:00.000');

-- ----------------------------
-- Table structure for published_article
-- ----------------------------
DROP TABLE IF EXISTS `published_article`;
CREATE TABLE `published_article`  (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `title` longtext CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NULL,
  `content` longtext CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NULL,
  `author_id` bigint NULL DEFAULT NULL,
  `status` tinyint UNSIGNED NULL DEFAULT NULL,
  `created_at` datetime(3) NULL DEFAULT NULL,
  `updated_at` datetime(3) NULL DEFAULT NULL,
  PRIMARY KEY (`id`) USING BTREE,
  INDEX `idx_published_article_author_id`(`author_id` ASC) USING BTREE
) ENGINE = InnoDB AUTO_INCREMENT = 21 CHARACTER SET = utf8mb4 COLLATE = utf8mb4_0900_ai_ci ROW_FORMAT = Dynamic;

-- ----------------------------
-- Records of published_article
-- ----------------------------
INSERT INTO `published_article` VALUES (1, 'Go 并发编程入门', 'goroutine 是 Go 语言最核心的并发原语。不同于操作系统线程，goroutine 由 Go runtime 调度，初始栈仅 2KB，创建成本极低。配合 channel 可以实现优雅的 CSP 并发模型...', 1, 2, '2026-03-01 10:00:00.000', '2026-03-01 10:00:00.000');
INSERT INTO `published_article` VALUES (2, 'GORM 使用技巧总结', '总结了 GORM 的常见查询模式：Where 链式调用、Preload 预加载、Scopes 复用查询条件。事务处理推荐用 Transaction 回调而非手动 Begin/Commit...', 1, 2, '2026-03-02 14:30:00.000', '2026-03-02 14:30:00.000');
INSERT INTO `published_article` VALUES (4, 'JWT 双 Token 认证实践', 'Access Token 设置短过期（30分钟），Refresh Token 设置长过期（7天）。前端在 401 时自动用 Refresh Token 换取新 Access Token，实现无感刷新...', 1, 2, '2026-03-04 16:00:00.000', '2026-03-04 16:00:00.000');
INSERT INTO `published_article` VALUES (6, 'React 19 新特性速览', 'React 19 引入了 use() hook 用于读取 Promise 和 Context，Actions 简化了表单提交，useOptimistic 支持乐观更新。Server Components 成为一等公民...', 1, 2, '2026-03-06 08:45:00.000', '2026-03-06 08:45:00.000');
INSERT INTO `published_article` VALUES (8, 'Docker 容器化部署 Go 应用', '多阶段构建：第一阶段用 golang:alpine 编译，第二阶段用 scratch 或 alpine 作为运行镜像，最终镜像仅 10-20MB。docker-compose 编排 MySQL + Redis + App...', 1, 2, '2026-03-08 15:30:00.000', '2026-03-08 15:30:00.000');
INSERT INTO `published_article` VALUES (10, 'Gin 中间件开发指南', '中间件签名 func(c *gin.Context)，用 c.Next() 传递到下一个处理器，c.Abort() 中断链。常见中间件：日志记录、JWT 鉴权、限流、CORS、Recovery...', 1, 2, '2026-03-10 17:00:00.000', '2026-03-10 17:00:00.000');
INSERT INTO `published_article` VALUES (12, 'Wire 依赖注入详解', 'Wire 是 Google 的编译时依赖注入工具。Provider 提供依赖，Injector 组装依赖图。wire.Build() 声明绑定关系，wire gen 生成代码，零运行时开销...', 1, 2, '2026-03-12 14:00:00.000', '2026-03-12 14:00:00.000');
INSERT INTO `published_article` VALUES (14, 'Go 单元测试与 Mock', 'testify/assert 提供丰富断言，testify/suite 支持测试套件。go.uber.org/mock 生成 interface mock，表驱动测试是 Go 社区标准模式...', 1, 2, '2026-03-14 16:30:00.000', '2026-03-14 16:30:00.000');
INSERT INTO `published_article` VALUES (16, '分布式限流算法对比', '令牌桶允许突发流量，漏桶平滑输出，滑动窗口精确控制。Redis + Lua 脚本实现原子操作，避免竞态条件。生产环境推荐滑动窗口...', 1, 2, '2026-03-16 13:45:00.000', '2026-03-16 13:45:00.000');
INSERT INTO `published_article` VALUES (18, 'Go 错误处理哲学', 'Go 不用异常，用返回值。errors.Is 判断错误链，errors.As 提取特定类型。fmt.Errorf + %w 包装错误保留上下文。sentinel error 定义在包级别...', 1, 2, '2026-03-18 15:00:00.000', '2026-03-18 15:00:00.000');
INSERT INTO `published_article` VALUES (20, '小微书项目架构复盘', '回顾整个项目的分层设计：Handler → Service → Repository → DAO/Cache。Wire DI 解耦依赖，双表设计分离制作库和线上库。前端 Next.js App Router + antd...', 1, 2, '2026-03-20 17:30:00.000', '2026-03-20 17:30:00.000');

-- ----------------------------
-- Table structure for user
-- ----------------------------
DROP TABLE IF EXISTS `user`;
CREATE TABLE `user`  (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `created_at` datetime(3) NULL DEFAULT NULL,
  `updated_at` datetime(3) NULL DEFAULT NULL,
  `deleted_at` datetime(3) NULL DEFAULT NULL,
  `email` varchar(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NULL DEFAULT NULL,
  `password` varchar(256) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NULL DEFAULT NULL,
  `nickname` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NULL DEFAULT NULL,
  `birthday` datetime NULL DEFAULT NULL,
  `about_me` text CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NULL,
  `phone` varchar(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NULL DEFAULT NULL,
  `wechat_open_id` varchar(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NULL DEFAULT NULL,
  `wechat_union_id` longtext CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NULL,
  PRIMARY KEY (`id`) USING BTREE,
  UNIQUE INDEX `uni_user_email`(`email` ASC) USING BTREE,
  UNIQUE INDEX `uni_user_phone`(`phone` ASC) USING BTREE,
  UNIQUE INDEX `uni_user_wechat_open_id`(`wechat_open_id` ASC) USING BTREE,
  INDEX `idx_user_deleted_at`(`deleted_at` ASC) USING BTREE
) ENGINE = InnoDB AUTO_INCREMENT = 5202 CHARACTER SET = utf8mb4 COLLATE = utf8mb4_0900_ai_ci ROW_FORMAT = Dynamic;

-- ----------------------------
-- Records of user
-- ----------------------------
INSERT INTO `user` VALUES (1, NULL, '2026-03-27 14:23:52.248', NULL, '3236447743@qq.com', '$2a$10$QAS0Xqqoe3DtBxzVev5NzOl02HLq2rJJrf4dJ3aOyyVxHIQ.J8FNW', 'tommy', '2026-03-27 14:23:51', 'see my name.', NULL, NULL, NULL);
INSERT INTO `user` VALUES (101, NULL, NULL, NULL, '123456@qq.com', '$2a$10$QAS0Xqqoe3DtBxzVev5NzOl02HLq2rJJrf4dJ3aOyyVxHIQ.J8FNW', 'tommy', NULL, 'say my name', NULL, NULL, NULL);
INSERT INTO `user` VALUES (102, NULL, NULL, NULL, NULL, '', 'tommy', NULL, 'say my name', '18608261234', NULL, NULL);

SET FOREIGN_KEY_CHECKS = 1;
