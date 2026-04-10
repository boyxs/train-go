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
  `abstract` varchar(256) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NULL DEFAULT NULL,
  `author_id` bigint NULL DEFAULT NULL,
  `status` tinyint UNSIGNED NULL DEFAULT NULL,
  `created_at` bigint NULL DEFAULT NULL,
  `updated_at` bigint NULL DEFAULT NULL,
  `deleted_at` bigint NOT NULL DEFAULT 0,
  PRIMARY KEY (`id`) USING BTREE,
  INDEX `idx_article_author_id`(`author_id` ASC) USING BTREE
) ENGINE = InnoDB AUTO_INCREMENT = 22 CHARACTER SET = utf8mb4 COLLATE = utf8mb4_0900_ai_ci ROW_FORMAT = Dynamic;

-- ----------------------------
-- Records of article (created_at/updated_at = Unix 毫秒, Asia/Shanghai)
-- ----------------------------
INSERT INTO `article` VALUES (1, 'Go 并发编程入门', 'goroutine 是 Go 语言最核心的并发原语。不同于操作系统线程，goroutine 由 Go runtime 调度，初始栈仅 2KB，创建成本极低。配合 channel 可以实现优雅的 CSP 并发模型...', 'goroutine 是 Go 语言最核心的并发原语。不同于操作系统线程，goroutine 由 Go runtime 调度，初始栈仅 2KB，创建成本极低。配合 channel 可以实现优雅的 CSP 并发模型...', 1, 2, 1772330400000, 1772330400000, 0);
INSERT INTO `article` VALUES (2, 'GORM 使用技巧总结', '总结了 GORM 的常见查询模式：Where 链式调用、Preload 预加载、Scopes 复用查询条件。事务处理推荐用 Transaction 回调而非手动 Begin/Commit...', '总结了 GORM 的常见查询模式：Where 链式调用、Preload 预加载、Scopes 复用查询条件。事务处理推荐用 Transaction 回调而非手动 Begin/Commit...', 1, 2, 1772433000000, 1772433000000, 0);
INSERT INTO `article` VALUES (3, 'Redis 缓存策略详解', 'Cache-Aside 是最常用的缓存模式：读时先查缓存，miss 则查 DB 并回填；写时先更新 DB，再删缓存。Write-Through 则由缓存层代理写入...', 'Cache-Aside 是最常用的缓存模式：读时先查缓存，miss 则查 DB 并回填；写时先更新 DB，再删缓存。Write-Through 则由缓存层代理写入...', 1, 1, 1772500500000, 1772500500000, 0);
INSERT INTO `article` VALUES (4, 'JWT 双 Token 认证实践', 'Access Token 设置短过期（30分钟），Refresh Token 设置长过期（7天）。前端在 401 时自动用 Refresh Token 换取新 Access Token，实现无感刷新...', 'Access Token 设置短过期（30分钟），Refresh Token 设置长过期（7天）。前端在 401 时自动用 Refresh Token 换取新 Access Token，实现无感刷新...', 1, 2, 1772611200000, 1772611200000, 0);
INSERT INTO `article` VALUES (5, 'Next.js App Router 迁移指南', '从 Pages Router 迁移到 App Router 的关键变化：文件系统路由从 pages/ 改为 app/，布局用 layout.tsx，数据获取从 getServerSideProps 改为 Server Components...', '从 Pages Router 迁移到 App Router 的关键变化：文件系统路由从 pages/ 改为 app/，布局用 layout.tsx，数据获取从 getServerSideProps 改为 Server Components...', 1, 1, 1772680800000, 1772680800000, 0);
INSERT INTO `article` VALUES (6, 'React 19 新特性速览', 'React 19 引入了 use() hook 用于读取 Promise 和 Context，Actions 简化了表单提交，useOptimistic 支持乐观更新。Server Components 成为一等公民...', 'React 19 引入了 use() hook 用于读取 Promise 和 Context，Actions 简化了表单提交，useOptimistic 支持乐观更新。Server Components 成为一等公民...', 1, 2, 1772757900000, 1772757900000, 0);
INSERT INTO `article` VALUES (7, 'Ant Design 表单最佳实践', 'Form.useForm() 获取表单实例，Form.Item 的 name 属性对应字段路径。校验用 rules 数组，异步校验返回 Promise。setFieldsValue 用于回填数据...', 'Form.useForm() 获取表单实例，Form.Item 的 name 属性对应字段路径。校验用 rules 数组，异步校验返回 Promise。setFieldsValue 用于回填数据...', 1, 1, 1772859600000, 1772859600000, 0);
INSERT INTO `article` VALUES (8, 'Docker 容器化部署 Go 应用', '多阶段构建：第一阶段用 golang:alpine 编译，第二阶段用 scratch 或 alpine 作为运行镜像，最终镜像仅 10-20MB。docker-compose 编排 MySQL + Redis + App...', '多阶段构建：第一阶段用 golang:alpine 编译，第二阶段用 scratch 或 alpine 作为运行镜像，最终镜像仅 10-20MB。docker-compose 编排 MySQL + Redis + App...', 1, 2, 1772955000000, 1772955000000, 0);
INSERT INTO `article` VALUES (9, 'MySQL 索引优化实战', '慢查询日志开启后，用 EXPLAIN 分析执行计划。复合索引遵循最左前缀原则，覆盖索引避免回表。避免在索引列上使用函数或隐式类型转换...', '慢查询日志开启后，用 EXPLAIN 分析执行计划。复合索引遵循最左前缀原则，覆盖索引避免回表。避免在索引列上使用函数或隐式类型转换...', 1, 1, 1773021600000, 1773021600000, 0);
INSERT INTO `article` VALUES (10, 'Gin 中间件开发指南', '中间件签名 func(c *gin.Context)，用 c.Next() 传递到下一个处理器，c.Abort() 中断链。常见中间件：日志记录、JWT 鉴权、限流、CORS、Recovery...', '中间件签名 func(c *gin.Context)，用 c.Next() 传递到下一个处理器，c.Abort() 中断链。常见中间件：日志记录、JWT 鉴权、限流、CORS、Recovery...', 1, 2, 1773133200000, 1773133200000, 0);
INSERT INTO `article` VALUES (11, 'TypeScript 类型体操入门', '从基础泛型 T 到条件类型 T extends U ? X : Y，再到 infer 推断和模板字面量类型。Utility Types 如 Partial、Required、Pick、Omit 是日常必备...', '从基础泛型 T 到条件类型 T extends U ? X : Y，再到 infer 推断和模板字面量类型。Utility Types 如 Partial、Required、Pick、Omit 是日常必备...', 1, 3, 1773192600000, 1773192600000, 0);
INSERT INTO `article` VALUES (12, 'Wire 依赖注入详解', 'Wire 是 Google 的编译时依赖注入工具。Provider 提供依赖，Injector 组装依赖图。wire.Build() 声明绑定关系，wire gen 生成代码，零运行时开销...', 'Wire 是 Google 的编译时依赖注入工具。Provider 提供依赖，Injector 组装依赖图。wire.Build() 声明绑定关系，wire gen 生成代码，零运行时开销...', 1, 2, 1773295200000, 1773295200000, 0);
INSERT INTO `article` VALUES (13, '前后端联调规范', 'API 响应统一 {code, msg, data} 格式。认证用 x-access-token / x-refresh-token header。前端 axios 拦截器处理 token 刷新和错误提示...', 'API 响应统一 {code, msg, data} 格式。认证用 x-access-token / x-refresh-token header。前端 axios 拦截器处理 token 刷新和错误提示...', 1, 1, 1773370800000, 1773370800000, 0);
INSERT INTO `article` VALUES (14, 'Go 单元测试与 Mock', 'testify/assert 提供丰富断言，testify/suite 支持测试套件。go.uber.org/mock 生成 interface mock，表驱动测试是 Go 社区标准模式...', 'testify/assert 提供丰富断言，testify/suite 支持测试套件。go.uber.org/mock 生成 interface mock，表驱动测试是 Go 社区标准模式...', 1, 2, 1773477000000, 1773477000000, 0);
INSERT INTO `article` VALUES (15, 'Tailwind CSS 4.0 迁移笔记', 'v4 重大变化：配置从 JS 文件改为 CSS-first（@import tailwindcss），移除 postcss 和 autoprefixer 依赖。@apply 仍可用但推荐直接写 class...', 'v4 重大变化：配置从 JS 文件改为 CSS-first（@import tailwindcss），移除 postcss 和 autoprefixer 依赖。@apply 仍可用但推荐直接写 class...', 1, 1, 1773532800000, 1773532800000, 0);
INSERT INTO `article` VALUES (16, '分布式限流算法对比', '令牌桶允许突发流量，漏桶平滑输出，滑动窗口精确控制。Redis + Lua 脚本实现原子操作，避免竞态条件。生产环境推荐滑动窗口...', '令牌桶允许突发流量，漏桶平滑输出，滑动窗口精确控制。Redis + Lua 脚本实现原子操作，避免竞态条件。生产环境推荐滑动窗口...', 1, 2, 1773639900000, 1773639900000, 0);
INSERT INTO `article` VALUES (17, 'ESLint 9 Flat Config 实践', 'ESLint 9 废弃 .eslintrc，改用 eslint.config.mjs 扁平配置。next/core-web-vitals 原生支持 flat config，prettier 必须放在最后覆盖格式化规则...', 'ESLint 9 废弃 .eslintrc，改用 eslint.config.mjs 扁平配置。next/core-web-vitals 原生支持 flat config，prettier 必须放在最后覆盖格式化规则...', 1, 1, 1773714600000, 1773714600000, 0);
INSERT INTO `article` VALUES (18, 'Go 错误处理哲学', 'Go 不用异常，用返回值。errors.Is 判断错误链，errors.As 提取特定类型。fmt.Errorf + %w 包装错误保留上下文。sentinel error 定义在包级别...', 'Go 不用异常，用返回值。errors.Is 判断错误链，errors.As 提取特定类型。fmt.Errorf + %w 包装错误保留上下文。sentinel error 定义在包级别...', 1, 2, 1773817200000, 1773817200000, 0);
INSERT INTO `article` VALUES (19, 'Kubernetes 入门：部署第一个应用', 'Pod 是最小调度单元，Deployment 管理副本集，Service 暴露网络。kubectl apply -f 声明式部署，ConfigMap 和 Secret 管理配置...', 'Pod 是最小调度单元，Deployment 管理副本集，Service 暴露网络。kubectl apply -f 声明式部署，ConfigMap 和 Secret 管理配置...', 1, 3, 1773882000000, 1773882000000, 0);
INSERT INTO `article` VALUES (20, '小微书项目架构复盘', '回顾整个项目的分层设计：Handler → Service → Repository → DAO/Cache。Wire DI 解耦依赖，双表设计分离制作库和线上库。前端 Next.js App Router + antd...', '回顾整个项目的分层设计：Handler → Service → Repository → DAO/Cache。Wire DI 解耦依赖，双表设计分离制作库和线上库。前端 Next.js App Router + antd...', 1, 2, 1773999000000, 1773999000000, 0);
INSERT INTO `article` VALUES (21, '如何写好技术文章', '好的技术文章需要：明确的目标读者、清晰的问题定义、循序渐进的讲解、可运行的代码示例、踩坑记录和最佳实践总结。标题要具体不要模糊...', '好的技术文章需要：明确的目标读者、清晰的问题定义、循序渐进的讲解、可运行的代码示例、踩坑记录和最佳实践总结。标题要具体不要模糊...', 1, 1, 1774065600000, 1774065600000, 0);

-- ----------------------------
-- Table structure for published_article
-- ----------------------------
DROP TABLE IF EXISTS `published_article`;
CREATE TABLE `published_article`  (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `title` longtext CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NULL,
  `content` longtext CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NULL,
  `abstract` varchar(256) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NULL DEFAULT NULL,
  `author_id` bigint NULL DEFAULT NULL,
  `status` tinyint UNSIGNED NULL DEFAULT NULL,
  `created_at` bigint NULL DEFAULT NULL,
  `updated_at` bigint NULL DEFAULT NULL,
  `deleted_at` bigint NOT NULL DEFAULT 0,
  PRIMARY KEY (`id`) USING BTREE,
  INDEX `idx_published_article_author_id`(`author_id` ASC) USING BTREE
) ENGINE = InnoDB AUTO_INCREMENT = 21 CHARACTER SET = utf8mb4 COLLATE = utf8mb4_0900_ai_ci ROW_FORMAT = Dynamic;

-- ----------------------------
-- Records of published_article
-- ----------------------------
INSERT INTO `published_article` VALUES (1, 'Go 并发编程入门', 'goroutine 是 Go 语言最核心的并发原语。不同于操作系统线程，goroutine 由 Go runtime 调度，初始栈仅 2KB，创建成本极低。配合 channel 可以实现优雅的 CSP 并发模型...', 'goroutine 是 Go 语言最核心的并发原语。不同于操作系统线程，goroutine 由 Go runtime 调度，初始栈仅 2KB，创建成本极低。配合 channel 可以实现优雅的 CSP 并发模型...', 1, 2, 1772330400000, 1772330400000, 0);
INSERT INTO `published_article` VALUES (2, 'GORM 使用技巧总结', '总结了 GORM 的常见查询模式：Where 链式调用、Preload 预加载、Scopes 复用查询条件。事务处理推荐用 Transaction 回调而非手动 Begin/Commit...', '总结了 GORM 的常见查询模式：Where 链式调用、Preload 预加载、Scopes 复用查询条件。事务处理推荐用 Transaction 回调而非手动 Begin/Commit...', 1, 2, 1772433000000, 1772433000000, 0);
INSERT INTO `published_article` VALUES (4, 'JWT 双 Token 认证实践', 'Access Token 设置短过期（30分钟），Refresh Token 设置长过期（7天）。前端在 401 时自动用 Refresh Token 换取新 Access Token，实现无感刷新...', 'Access Token 设置短过期（30分钟），Refresh Token 设置长过期（7天）。前端在 401 时自动用 Refresh Token 换取新 Access Token，实现无感刷新...', 1, 2, 1772611200000, 1772611200000, 0);
INSERT INTO `published_article` VALUES (6, 'React 19 新特性速览', 'React 19 引入了 use() hook 用于读取 Promise 和 Context，Actions 简化了表单提交，useOptimistic 支持乐观更新。Server Components 成为一等公民...', 'React 19 引入了 use() hook 用于读取 Promise 和 Context，Actions 简化了表单提交，useOptimistic 支持乐观更新。Server Components 成为一等公民...', 1, 2, 1772757900000, 1772757900000, 0);
INSERT INTO `published_article` VALUES (8, 'Docker 容器化部署 Go 应用', '多阶段构建：第一阶段用 golang:alpine 编译，第二阶段用 scratch 或 alpine 作为运行镜像，最终镜像仅 10-20MB。docker-compose 编排 MySQL + Redis + App...', '多阶段构建：第一阶段用 golang:alpine 编译，第二阶段用 scratch 或 alpine 作为运行镜像，最终镜像仅 10-20MB。docker-compose 编排 MySQL + Redis + App...', 1, 2, 1772955000000, 1772955000000, 0);
INSERT INTO `published_article` VALUES (10, 'Gin 中间件开发指南', '中间件签名 func(c *gin.Context)，用 c.Next() 传递到下一个处理器，c.Abort() 中断链。常见中间件：日志记录、JWT 鉴权、限流、CORS、Recovery...', '中间件签名 func(c *gin.Context)，用 c.Next() 传递到下一个处理器，c.Abort() 中断链。常见中间件：日志记录、JWT 鉴权、限流、CORS、Recovery...', 1, 2, 1773133200000, 1773133200000, 0);
INSERT INTO `published_article` VALUES (12, 'Wire 依赖注入详解', 'Wire 是 Google 的编译时依赖注入工具。Provider 提供依赖，Injector 组装依赖图。wire.Build() 声明绑定关系，wire gen 生成代码，零运行时开销...', 'Wire 是 Google 的编译时依赖注入工具。Provider 提供依赖，Injector 组装依赖图。wire.Build() 声明绑定关系，wire gen 生成代码，零运行时开销...', 1, 2, 1773295200000, 1773295200000, 0);
INSERT INTO `published_article` VALUES (14, 'Go 单元测试与 Mock', 'testify/assert 提供丰富断言，testify/suite 支持测试套件。go.uber.org/mock 生成 interface mock，表驱动测试是 Go 社区标准模式...', 'testify/assert 提供丰富断言，testify/suite 支持测试套件。go.uber.org/mock 生成 interface mock，表驱动测试是 Go 社区标准模式...', 1, 2, 1773477000000, 1773477000000, 0);
INSERT INTO `published_article` VALUES (16, '分布式限流算法对比', '令牌桶允许突发流量，漏桶平滑输出，滑动窗口精确控制。Redis + Lua 脚本实现原子操作，避免竞态条件。生产环境推荐滑动窗口...', '令牌桶允许突发流量，漏桶平滑输出，滑动窗口精确控制。Redis + Lua 脚本实现原子操作，避免竞态条件。生产环境推荐滑动窗口...', 1, 2, 1773639900000, 1773639900000, 0);
INSERT INTO `published_article` VALUES (18, 'Go 错误处理哲学', 'Go 不用异常，用返回值。errors.Is 判断错误链，errors.As 提取特定类型。fmt.Errorf + %w 包装错误保留上下文。sentinel error 定义在包级别...', 'Go 不用异常，用返回值。errors.Is 判断错误链，errors.As 提取特定类型。fmt.Errorf + %w 包装错误保留上下文。sentinel error 定义在包级别...', 1, 2, 1773817200000, 1773817200000, 0);
INSERT INTO `published_article` VALUES (20, '小微书项目架构复盘', '回顾整个项目的分层设计：Handler → Service → Repository → DAO/Cache。Wire DI 解耦依赖，双表设计分离制作库和线上库。前端 Next.js App Router + antd...', '回顾整个项目的分层设计：Handler → Service → Repository → DAO/Cache。Wire DI 解耦依赖，双表设计分离制作库和线上库。前端 Next.js App Router + antd...', 1, 2, 1773999000000, 1773999000000, 0);

-- ----------------------------
-- Table structure for user
-- ----------------------------
DROP TABLE IF EXISTS `user`;
CREATE TABLE `user`  (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `created_at` bigint NULL DEFAULT NULL,
  `updated_at` bigint NULL DEFAULT NULL,
  `deleted_at` bigint NOT NULL DEFAULT 0,
  `email` varchar(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NULL DEFAULT NULL,
  `password` varchar(256) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NULL DEFAULT NULL,
  `nickname` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NULL DEFAULT NULL,
  `birthday` bigint NULL DEFAULT NULL,
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
-- Records of user (birthday/updated_at = Unix 毫秒)
-- ----------------------------
INSERT INTO `user` VALUES (1, NULL, 1774621432248, 0, '3236447743@qq.com', '$2a$10$QAS0Xqqoe3DtBxzVev5NzOl02HLq2rJJrf4dJ3aOyyVxHIQ.J8FNW', 'tommy', 1774621431000, 'see my name.', NULL, NULL, NULL);
INSERT INTO `user` VALUES (101, NULL, NULL, 0, '123456@qq.com', '$2a$10$QAS0Xqqoe3DtBxzVev5NzOl02HLq2rJJrf4dJ3aOyyVxHIQ.J8FNW', 'tommy', NULL, 'say my name', NULL, NULL, NULL);
INSERT INTO `user` VALUES (102, NULL, NULL, 0, NULL, '', 'tommy', NULL, 'say my name', '18608261234', NULL, NULL);

-- ----------------------------
-- Table structure for interaction
-- ----------------------------
DROP TABLE IF EXISTS `interaction`;
CREATE TABLE `interaction`  (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `biz` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT '',
  `biz_id` bigint NOT NULL DEFAULT 0,
  `read_count` bigint NOT NULL DEFAULT 0,
  `like_count` bigint NOT NULL DEFAULT 0,
  `collect_count` bigint NOT NULL DEFAULT 0,
  `created_at` bigint NULL DEFAULT NULL,
  `updated_at` bigint NULL DEFAULT NULL,
  PRIMARY KEY (`id`) USING BTREE,
  UNIQUE INDEX `uk_biz`(`biz` ASC, `biz_id` ASC) USING BTREE
) ENGINE = InnoDB CHARACTER SET = utf8mb4 COLLATE = utf8mb4_0900_ai_ci ROW_FORMAT = Dynamic;

-- ----------------------------
-- Table structure for user_interaction
-- ----------------------------
DROP TABLE IF EXISTS `user_interaction`;
CREATE TABLE `user_interaction`  (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `biz` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT '',
  `biz_id` bigint NOT NULL DEFAULT 0,
  `user_id` bigint NOT NULL DEFAULT 0,
  `liked` tinyint(1) NOT NULL DEFAULT 0,
  `collected` tinyint(1) NOT NULL DEFAULT 0,
  `created_at` bigint NULL DEFAULT NULL,
  `updated_at` bigint NULL DEFAULT NULL,
  PRIMARY KEY (`id`) USING BTREE,
  UNIQUE INDEX `uk_user_biz`(`biz` ASC, `biz_id` ASC, `user_id` ASC) USING BTREE
) ENGINE = InnoDB CHARACTER SET = utf8mb4 COLLATE = utf8mb4_0900_ai_ci ROW_FORMAT = Dynamic;

-- ----------------------------
-- Table structure for conversation (AI 客服对话)
-- ----------------------------
CREATE TABLE IF NOT EXISTS `conversation` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `user_id` bigint NOT NULL,
  `title` varchar(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT '',
  `created_at` bigint NOT NULL DEFAULT 0,
  `updated_at` bigint NOT NULL DEFAULT 0,
  PRIMARY KEY (`id`) USING BTREE,
  INDEX `idx_user_updated` (`user_id` ASC, `updated_at` DESC) USING BTREE
) ENGINE = InnoDB CHARACTER SET = utf8mb4 COLLATE = utf8mb4_0900_ai_ci ROW_FORMAT = Dynamic;

-- ----------------------------
-- Table structure for message (AI 客服消息)
-- ----------------------------
CREATE TABLE IF NOT EXISTS `message` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `conversation_id` bigint NOT NULL,
  `role` varchar(16) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL,
  `content` text CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL,
  `tool_calls` json NULL,
  `token_used` int NOT NULL DEFAULT 0,
  `created_at` bigint NOT NULL DEFAULT 0,
  PRIMARY KEY (`id`) USING BTREE,
  INDEX `idx_conv_created` (`conversation_id` ASC, `created_at` ASC) USING BTREE
) ENGINE = InnoDB CHARACTER SET = utf8mb4 COLLATE = utf8mb4_0900_ai_ci ROW_FORMAT = Dynamic;

-- ----------------------------
-- Table structure for ai_click_events (AI 点击埋点)
-- ----------------------------
CREATE TABLE IF NOT EXISTS `ai_click_events` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `user_id` bigint NOT NULL,
  `article_id` bigint NOT NULL,
  `conversation_id` bigint NOT NULL,
  `source` varchar(32) NOT NULL DEFAULT 'ai_chat',
  `created_at` bigint NOT NULL DEFAULT 0,
  `updated_at` bigint NOT NULL DEFAULT 0,
  `deleted_at` bigint NOT NULL DEFAULT 0,
  PRIMARY KEY (`id`) USING BTREE,
  UNIQUE INDEX `uk_dedup` (`user_id` ASC, `article_id` ASC, `conversation_id` ASC, `source` ASC) USING BTREE,
  INDEX `idx_article` (`article_id` ASC) USING BTREE,
  INDEX `idx_created` (`created_at` ASC) USING BTREE
) ENGINE = InnoDB CHARACTER SET = utf8mb4 COLLATE = utf8mb4_0900_ai_ci ROW_FORMAT = Dynamic;

SET FOREIGN_KEY_CHECKS = 1;

-- ============================================================
-- 调试视图：将 bigint 毫秒时间戳显示为可读时间
-- 用法: SELECT * FROM v_article;
-- ============================================================

CREATE OR REPLACE VIEW v_article AS
SELECT id, title, abstract, author_id, status,
  FROM_UNIXTIME(created_at / 1000) AS created_at,
  FROM_UNIXTIME(updated_at / 1000) AS updated_at
FROM article;

CREATE OR REPLACE VIEW v_published_article AS
SELECT id, title, abstract, author_id, status,
  FROM_UNIXTIME(created_at / 1000) AS created_at,
  FROM_UNIXTIME(updated_at / 1000) AS updated_at
FROM published_article;

CREATE OR REPLACE VIEW v_interaction AS
SELECT id, biz, biz_id, read_count, like_count, collect_count,
  FROM_UNIXTIME(created_at / 1000) AS created_at,
  FROM_UNIXTIME(updated_at / 1000) AS updated_at
FROM interaction;

CREATE OR REPLACE VIEW v_user_interaction AS
SELECT id, biz, biz_id, user_id, liked, collected,
  FROM_UNIXTIME(created_at / 1000) AS created_at,
  FROM_UNIXTIME(updated_at / 1000) AS updated_at
FROM user_interaction;

CREATE OR REPLACE VIEW v_user AS
SELECT id, email, nickname,
  FROM_UNIXTIME(birthday / 1000) AS birthday,
  about_me, phone,
  FROM_UNIXTIME(created_at / 1000) AS created_at,
  FROM_UNIXTIME(updated_at / 1000) AS updated_at
FROM `user`;

CREATE OR REPLACE VIEW v_conversation AS
SELECT id, user_id, title,
  FROM_UNIXTIME(created_at / 1000) AS created_at,
  FROM_UNIXTIME(updated_at / 1000) AS updated_at
FROM conversation;

CREATE OR REPLACE VIEW v_message AS
SELECT id, conversation_id, role,
  LEFT(content, 100) AS content_preview,
  FROM_UNIXTIME(created_at / 1000) AS created_at
FROM message;

CREATE OR REPLACE VIEW v_ai_click_events AS
SELECT id, user_id, article_id, conversation_id, source,
  FROM_UNIXTIME(created_at / 1000) AS created_at,
  FROM_UNIXTIME(updated_at / 1000) AS updated_at
FROM ai_click_events
WHERE deleted_at = 0;
