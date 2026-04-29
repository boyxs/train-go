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
  `id` bigint NOT NULL AUTO_INCREMENT COMMENT '主键',
  `title` longtext CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NULL COMMENT '标题',
  `content` longtext CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NULL COMMENT '正文（BLOB，查询时勿带出）',
  `abstract` varchar(256) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NULL DEFAULT NULL COMMENT '摘要（列表页展示）',
  `author_id` bigint NULL DEFAULT NULL COMMENT '作者 id（关联 user.id）',
  `status` tinyint UNSIGNED NULL DEFAULT NULL COMMENT '状态：1=未发表 2=已发表 3=仅自己可见',
  `category` varchar(32) NOT NULL DEFAULT '' COMMENT '分类：tech/career/life/ai/other',
  `created_at` bigint NULL DEFAULT NULL COMMENT '创建时间（Unix 毫秒）',
  `updated_at` bigint NULL DEFAULT NULL COMMENT '更新时间（Unix 毫秒）',
  `deleted_at` bigint NOT NULL DEFAULT 0 COMMENT '软删除时间（0=未删，非零=删除毫秒戳）',
  PRIMARY KEY (`id`) USING BTREE,
  INDEX `idx_article_author_id`(`author_id` ASC) USING BTREE,
  INDEX `idx_article_category`(`category` ASC) USING BTREE
) ENGINE = InnoDB AUTO_INCREMENT = 36 CHARACTER SET = utf8mb4 COLLATE = utf8mb4_0900_ai_ci ROW_FORMAT = Dynamic COMMENT = '文章制作库（草稿区，作者可编辑）';

-- ----------------------------
-- Records of article (created_at/updated_at = Unix 毫秒, Asia/Shanghai)
-- ----------------------------
INSERT INTO `article` VALUES (1, 'Go 并发编程入门', 'goroutine 是 Go 语言最核心的并发原语。不同于操作系统线程，goroutine 由 Go runtime 调度，初始栈仅 2KB，创建成本极低。配合 channel 可以实现优雅的 CSP 并发模型...', 'goroutine 是 Go 语言最核心的并发原语。不同于操作系统线程，goroutine 由 Go runtime 调度，初始栈仅 2KB，创建成本极低。配合 channel 可以实现优雅的 CSP 并发模型...', 1, 2, 'tech', 1772330400000, 1772330400000, 0);
INSERT INTO `article` VALUES (2, 'GORM 使用技巧总结', '总结了 GORM 的常见查询模式：Where 链式调用、Preload 预加载、Scopes 复用查询条件。事务处理推荐用 Transaction 回调而非手动 Begin/Commit...', '总结了 GORM 的常见查询模式：Where 链式调用、Preload 预加载、Scopes 复用查询条件。事务处理推荐用 Transaction 回调而非手动 Begin/Commit...', 1, 2, 'tech', 1772433000000, 1772433000000, 0);
INSERT INTO `article` VALUES (3, 'Redis 缓存策略详解', 'Cache-Aside 是最常用的缓存模式：读时先查缓存，miss 则查 DB 并回填；写时先更新 DB，再删缓存。Write-Through 则由缓存层代理写入...', 'Cache-Aside 是最常用的缓存模式：读时先查缓存，miss 则查 DB 并回填；写时先更新 DB，再删缓存。Write-Through 则由缓存层代理写入...', 1, 1, 'tech', 1772500500000, 1772500500000, 0);
INSERT INTO `article` VALUES (4, 'JWT 双 Token 认证实践', 'Access Token 设置短过期（30分钟），Refresh Token 设置长过期（7天）。前端在 401 时自动用 Refresh Token 换取新 Access Token，实现无感刷新...', 'Access Token 设置短过期（30分钟），Refresh Token 设置长过期（7天）。前端在 401 时自动用 Refresh Token 换取新 Access Token，实现无感刷新...', 1, 2, 'tech', 1772611200000, 1772611200000, 0);
INSERT INTO `article` VALUES (5, 'Next.js App Router 迁移指南', '从 Pages Router 迁移到 App Router 的关键变化：文件系统路由从 pages/ 改为 app/，布局用 layout.tsx，数据获取从 getServerSideProps 改为 Server Components...', '从 Pages Router 迁移到 App Router 的关键变化：文件系统路由从 pages/ 改为 app/，布局用 layout.tsx，数据获取从 getServerSideProps 改为 Server Components...', 1, 1, 'other', 1772680800000, 1772680800000, 0);
INSERT INTO `article` VALUES (6, 'React 19 新特性速览', 'React 19 引入了 use() hook 用于读取 Promise 和 Context，Actions 简化了表单提交，useOptimistic 支持乐观更新。Server Components 成为一等公民...', 'React 19 引入了 use() hook 用于读取 Promise 和 Context，Actions 简化了表单提交，useOptimistic 支持乐观更新。Server Components 成为一等公民...', 1, 2, 'other', 1772757900000, 1772757900000, 0);
INSERT INTO `article` VALUES (7, 'Ant Design 表单最佳实践', 'Form.useForm() 获取表单实例，Form.Item 的 name 属性对应字段路径。校验用 rules 数组，异步校验返回 Promise。setFieldsValue 用于回填数据...', 'Form.useForm() 获取表单实例，Form.Item 的 name 属性对应字段路径。校验用 rules 数组，异步校验返回 Promise。setFieldsValue 用于回填数据...', 1, 1, 'other', 1772859600000, 1772859600000, 0);
INSERT INTO `article` VALUES (8, 'Docker 容器化部署 Go 应用', '多阶段构建：第一阶段用 golang:alpine 编译，第二阶段用 scratch 或 alpine 作为运行镜像，最终镜像仅 10-20MB。docker-compose 编排 MySQL + Redis + App...', '多阶段构建：第一阶段用 golang:alpine 编译，第二阶段用 scratch 或 alpine 作为运行镜像，最终镜像仅 10-20MB。docker-compose 编排 MySQL + Redis + App...', 1, 2, 'tech', 1772955000000, 1772955000000, 0);
INSERT INTO `article` VALUES (9, 'MySQL 索引优化实战', '慢查询日志开启后，用 EXPLAIN 分析执行计划。复合索引遵循最左前缀原则，覆盖索引避免回表。避免在索引列上使用函数或隐式类型转换...', '慢查询日志开启后，用 EXPLAIN 分析执行计划。复合索引遵循最左前缀原则，覆盖索引避免回表。避免在索引列上使用函数或隐式类型转换...', 1, 1, 'tech', 1773021600000, 1773021600000, 0);
INSERT INTO `article` VALUES (10, 'Gin 中间件开发指南', '中间件签名 func(c *gin.Context)，用 c.Next() 传递到下一个处理器，c.Abort() 中断链。常见中间件：日志记录、JWT 鉴权、限流、CORS、Recovery...', '中间件签名 func(c *gin.Context)，用 c.Next() 传递到下一个处理器，c.Abort() 中断链。常见中间件：日志记录、JWT 鉴权、限流、CORS、Recovery...', 1, 2, 'tech', 1773133200000, 1773133200000, 0);
INSERT INTO `article` VALUES (11, 'TypeScript 类型体操入门', '从基础泛型 T 到条件类型 T extends U ? X : Y，再到 infer 推断和模板字面量类型。Utility Types 如 Partial、Required、Pick、Omit 是日常必备...', '从基础泛型 T 到条件类型 T extends U ? X : Y，再到 infer 推断和模板字面量类型。Utility Types 如 Partial、Required、Pick、Omit 是日常必备...', 1, 3, 'other', 1773192600000, 1773192600000, 0);
INSERT INTO `article` VALUES (12, 'Wire 依赖注入详解', 'Wire 是 Google 的编译时依赖注入工具。Provider 提供依赖，Injector 组装依赖图。wire.Build() 声明绑定关系，wire gen 生成代码，零运行时开销...', 'Wire 是 Google 的编译时依赖注入工具。Provider 提供依赖，Injector 组装依赖图。wire.Build() 声明绑定关系，wire gen 生成代码，零运行时开销...', 1, 2, 'tech', 1773295200000, 1773295200000, 0);
INSERT INTO `article` VALUES (13, '前后端联调规范', 'API 响应统一 {code, msg, data} 格式。认证用 x-access-token / x-refresh-token header。前端 axios 拦截器处理 token 刷新和错误提示...', 'API 响应统一 {code, msg, data} 格式。认证用 x-access-token / x-refresh-token header。前端 axios 拦截器处理 token 刷新和错误提示...', 1, 1, 'career', 1773370800000, 1773370800000, 0);
INSERT INTO `article` VALUES (14, 'Go 单元测试与 Mock', 'testify/assert 提供丰富断言，testify/suite 支持测试套件。go.uber.org/mock 生成 interface mock，表驱动测试是 Go 社区标准模式...', 'testify/assert 提供丰富断言，testify/suite 支持测试套件。go.uber.org/mock 生成 interface mock，表驱动测试是 Go 社区标准模式...', 1, 2, 'tech', 1773477000000, 1773477000000, 0);
INSERT INTO `article` VALUES (15, 'Tailwind CSS 4.0 迁移笔记', 'v4 重大变化：配置从 JS 文件改为 CSS-first（@import tailwindcss），移除 postcss 和 autoprefixer 依赖。@apply 仍可用但推荐直接写 class...', 'v4 重大变化：配置从 JS 文件改为 CSS-first（@import tailwindcss），移除 postcss 和 autoprefixer 依赖。@apply 仍可用但推荐直接写 class...', 1, 1, 'other', 1773532800000, 1773532800000, 0);
INSERT INTO `article` VALUES (16, '分布式限流算法对比', '令牌桶允许突发流量，漏桶平滑输出，滑动窗口精确控制。Redis + Lua 脚本实现原子操作，避免竞态条件。生产环境推荐滑动窗口...', '令牌桶允许突发流量，漏桶平滑输出，滑动窗口精确控制。Redis + Lua 脚本实现原子操作，避免竞态条件。生产环境推荐滑动窗口...', 1, 2, 'tech', 1773639900000, 1773639900000, 0);
INSERT INTO `article` VALUES (17, 'ESLint 9 Flat Config 实践', 'ESLint 9 废弃 .eslintrc，改用 eslint.config.mjs 扁平配置。next/core-web-vitals 原生支持 flat config，prettier 必须放在最后覆盖格式化规则...', 'ESLint 9 废弃 .eslintrc，改用 eslint.config.mjs 扁平配置。next/core-web-vitals 原生支持 flat config，prettier 必须放在最后覆盖格式化规则...', 1, 1, 'other', 1773714600000, 1773714600000, 0);
INSERT INTO `article` VALUES (18, 'Go 错误处理哲学', 'Go 不用异常，用返回值。errors.Is 判断错误链，errors.As 提取特定类型。fmt.Errorf + %w 包装错误保留上下文。sentinel error 定义在包级别...', 'Go 不用异常，用返回值。errors.Is 判断错误链，errors.As 提取特定类型。fmt.Errorf + %w 包装错误保留上下文。sentinel error 定义在包级别...', 1, 2, 'tech', 1773817200000, 1773817200000, 0);
INSERT INTO `article` VALUES (19, 'Kubernetes 入门：部署第一个应用', 'Pod 是最小调度单元，Deployment 管理副本集，Service 暴露网络。kubectl apply -f 声明式部署，ConfigMap 和 Secret 管理配置...', 'Pod 是最小调度单元，Deployment 管理副本集，Service 暴露网络。kubectl apply -f 声明式部署，ConfigMap 和 Secret 管理配置...', 1, 3, 'tech', 1773882000000, 1773882000000, 0);
INSERT INTO `article` VALUES (20, '小微书项目架构复盘', '回顾整个项目的分层设计：Handler → Service → Repository → DAO/Cache。Wire DI 解耦依赖，双表设计分离制作库和线上库。前端 Next.js App Router + antd...', '回顾整个项目的分层设计：Handler → Service → Repository → DAO/Cache。Wire DI 解耦依赖，双表设计分离制作库和线上库。前端 Next.js App Router + antd...', 1, 2, 'career', 1773999000000, 1773999000000, 0);
INSERT INTO `article` VALUES (21, '如何写好技术文章', '好的技术文章需要：明确的目标读者、清晰的问题定义、循序渐进的讲解、可运行的代码示例、踩坑记录和最佳实践总结。标题要具体不要模糊...', '好的技术文章需要：明确的目标读者、清晰的问题定义、循序渐进的讲解、可运行的代码示例、踩坑记录和最佳实践总结。标题要具体不要模糊...', 1, 1, 'career', 1774065600000, 1774065600000, 0);
-- 扩充 14 篇（id 22~35）用于榜单分页测试
INSERT INTO `article` VALUES (22, 'Redis 数据类型速查', 'String/List/Hash/Set/ZSet 五大基础类型 + Stream/Geo/HyperLogLog 进阶类型，常见命令速记卡。', 'String/List/Hash/Set/ZSet 五大基础类型 + Stream/Geo/HyperLogLog 进阶类型，常见命令速记卡。', 1, 1, 'tech', 1774152000000, 1774152000000, 0);
INSERT INTO `article` VALUES (23, '微服务拆分反模式', '分布式单体、过早拆分、共享数据库等常见反模式。拆分原则：业务边界清晰、数据隔离、独立部署。', '分布式单体、过早拆分、共享数据库等常见反模式。拆分原则：业务边界清晰、数据隔离、独立部署。', 1, 2, 'tech', 1774238400000, 1774238400000, 0);
INSERT INTO `article` VALUES (24, '前端性能优化清单', '首屏、交互、网络三大维度 20 项清单。Core Web Vitals 指标含义与优化手段。', '首屏、交互、网络三大维度 20 项清单。Core Web Vitals 指标含义与优化手段。', 1, 3, 'tech', 1774324800000, 1774324800000, 0);
INSERT INTO `article` VALUES (25, '代码评审 10 条原则', '评审关注正确性、可读性、一致性。避免吹毛求疵，聚焦真实风险。对事不对人。', '评审关注正确性、可读性、一致性。避免吹毛求疵，聚焦真实风险。对事不对人。', 1, 1, 'career', 1774411200000, 1774411200000, 0);
INSERT INTO `article` VALUES (26, '远程工作沟通法则', '异步优先、文字留痕、会议前置议程。避免"立即回复"文化拖累深度工作。', '异步优先、文字留痕、会议前置议程。避免"立即回复"文化拖累深度工作。', 1, 2, 'career', 1774497600000, 1774497600000, 0);
INSERT INTO `article` VALUES (27, '程序员健康指南', '久坐、视疲劳、颈椎压力三大隐患。站立办公、番茄工作法、每日步行指标。', '久坐、视疲劳、颈椎压力三大隐患。站立办公、番茄工作法、每日步行指标。', 1, 3, 'life', 1774584000000, 1774584000000, 0);
INSERT INTO `article` VALUES (28, 'ChatGPT 提示词模板', 'Role/Task/Context/Format 四段式提示词。Few-shot 举例 + Chain-of-Thought 拆步。', 'Role/Task/Context/Format 四段式提示词。Few-shot 举例 + Chain-of-Thought 拆步。', 1, 1, 'ai', 1774670400000, 1774670400000, 0);
INSERT INTO `article` VALUES (29, 'AI 绘画工具对比', 'Midjourney / DALL·E / SD 各自定位：M 偏艺术、D 偏精准、SD 偏可控。', 'Midjourney / DALL·E / SD 各自定位：M 偏艺术、D 偏精准、SD 偏可控。', 1, 2, 'ai', 1774756800000, 1774756800000, 0);
INSERT INTO `article` VALUES (30, '副业时间管理', '主业不摸鱼、副业不透支。周末 2 小时 × 52 周 > 零散加班。', '主业不摸鱼、副业不透支。周末 2 小时 × 52 周 > 零散加班。', 1, 3, 'life', 1774843200000, 1774843200000, 0);
INSERT INTO `article` VALUES (31, 'PostgreSQL vs MySQL', 'PG 在 JSON、CTE、窗口函数、类型系统更强；MySQL 在生态、运维门槛、复制成熟度占优。', 'PG 在 JSON、CTE、窗口函数、类型系统更强；MySQL 在生态、运维门槛、复制成熟度占优。', 1, 1, 'tech', 1774929600000, 1774929600000, 0);
INSERT INTO `article` VALUES (32, '技术博客运营心得', 'SEO 关键词选题、Markdown 工作流、图床与 CDN、订阅与互动。', 'SEO 关键词选题、Markdown 工作流、图床与 CDN、订阅与互动。', 1, 2, 'career', 1775016000000, 1775016000000, 0);
INSERT INTO `article` VALUES (33, 'Git 高级技巧十则', 'rebase -i / cherry-pick / worktree / bisect / reflog 等进阶命令的典型场景。', 'rebase -i / cherry-pick / worktree / bisect / reflog 等进阶命令的典型场景。', 1, 3, 'tech', 1775102400000, 1775102400000, 0);
INSERT INTO `article` VALUES (34, '情绪管理与心流', '识别扳机点、微暂停、番茄节奏。心流的进入条件：清晰目标 + 即时反馈 + 挑战匹配。', '识别扳机点、微暂停、番茄节奏。心流的进入条件：清晰目标 + 即时反馈 + 挑战匹配。', 1, 1, 'life', 1775188800000, 1775188800000, 0);
INSERT INTO `article` VALUES (35, 'Claude 对话技巧', '长上下文、工具调用、结构化输出。Artifacts 和 MCP 扩展能力使用场景。', '长上下文、工具调用、结构化输出。Artifacts 和 MCP 扩展能力使用场景。', 1, 2, 'ai', 1775275200000, 1775275200000, 0);

-- ----------------------------
-- Table structure for published_article
-- ----------------------------
DROP TABLE IF EXISTS `published_article`;
CREATE TABLE `published_article`  (
  `id` bigint NOT NULL AUTO_INCREMENT COMMENT '主键（与制作库 article.id 一致）',
  `title` longtext CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NULL COMMENT '标题（快照）',
  `content` longtext CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NULL COMMENT '正文（快照，BLOB）',
  `abstract` varchar(256) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NULL DEFAULT NULL COMMENT '摘要（快照）',
  `author_id` bigint NULL DEFAULT NULL COMMENT '作者 id',
  `status` tinyint UNSIGNED NULL DEFAULT NULL COMMENT '状态：2=公开可读 3=仅自己可见',
  `category` varchar(32) NOT NULL DEFAULT '' COMMENT '分类：tech/career/life/ai/other',
  `created_at` bigint NULL DEFAULT NULL COMMENT '发布时间（Unix 毫秒）',
  `updated_at` bigint NULL DEFAULT NULL COMMENT '更新时间（Unix 毫秒）',
  `deleted_at` bigint NOT NULL DEFAULT 0 COMMENT '软删除时间（0=未删）',
  PRIMARY KEY (`id`) USING BTREE,
  INDEX `idx_published_article_author_id`(`author_id` ASC) USING BTREE,
  INDEX `idx_published_article_category`(`category` ASC) USING BTREE
) ENGINE = InnoDB AUTO_INCREMENT = 36 CHARACTER SET = utf8mb4 COLLATE = utf8mb4_0900_ai_ci ROW_FORMAT = Dynamic COMMENT = '文章线上库（读者可见，发布后从 article 同步而来）';

-- ----------------------------
-- Records of published_article
-- ----------------------------
INSERT INTO `published_article` VALUES (1, 'Go 并发编程入门', 'goroutine 是 Go 语言最核心的并发原语。不同于操作系统线程，goroutine 由 Go runtime 调度，初始栈仅 2KB，创建成本极低。配合 channel 可以实现优雅的 CSP 并发模型...', 'goroutine 是 Go 语言最核心的并发原语。不同于操作系统线程，goroutine 由 Go runtime 调度，初始栈仅 2KB，创建成本极低。配合 channel 可以实现优雅的 CSP 并发模型...', 1, 2, 'tech', 1772330400000, 1772330400000, 0);
INSERT INTO `published_article` VALUES (2, 'GORM 使用技巧总结', '总结了 GORM 的常见查询模式：Where 链式调用、Preload 预加载、Scopes 复用查询条件。事务处理推荐用 Transaction 回调而非手动 Begin/Commit...', '总结了 GORM 的常见查询模式：Where 链式调用、Preload 预加载、Scopes 复用查询条件。事务处理推荐用 Transaction 回调而非手动 Begin/Commit...', 1, 2, 'tech', 1772433000000, 1772433000000, 0);
INSERT INTO `published_article` VALUES (4, 'JWT 双 Token 认证实践', 'Access Token 设置短过期（30分钟），Refresh Token 设置长过期（7天）。前端在 401 时自动用 Refresh Token 换取新 Access Token，实现无感刷新...', 'Access Token 设置短过期（30分钟），Refresh Token 设置长过期（7天）。前端在 401 时自动用 Refresh Token 换取新 Access Token，实现无感刷新...', 1, 2, 'tech', 1772611200000, 1772611200000, 0);
INSERT INTO `published_article` VALUES (6, 'React 19 新特性速览', 'React 19 引入了 use() hook 用于读取 Promise 和 Context，Actions 简化了表单提交，useOptimistic 支持乐观更新。Server Components 成为一等公民...', 'React 19 引入了 use() hook 用于读取 Promise 和 Context，Actions 简化了表单提交，useOptimistic 支持乐观更新。Server Components 成为一等公民...', 1, 2, 'other', 1772757900000, 1772757900000, 0);
INSERT INTO `published_article` VALUES (8, 'Docker 容器化部署 Go 应用', '多阶段构建：第一阶段用 golang:alpine 编译，第二阶段用 scratch 或 alpine 作为运行镜像，最终镜像仅 10-20MB。docker-compose 编排 MySQL + Redis + App...', '多阶段构建：第一阶段用 golang:alpine 编译，第二阶段用 scratch 或 alpine 作为运行镜像，最终镜像仅 10-20MB。docker-compose 编排 MySQL + Redis + App...', 1, 2, 'tech', 1772955000000, 1772955000000, 0);
INSERT INTO `published_article` VALUES (10, 'Gin 中间件开发指南', '中间件签名 func(c *gin.Context)，用 c.Next() 传递到下一个处理器，c.Abort() 中断链。常见中间件：日志记录、JWT 鉴权、限流、CORS、Recovery...', '中间件签名 func(c *gin.Context)，用 c.Next() 传递到下一个处理器，c.Abort() 中断链。常见中间件：日志记录、JWT 鉴权、限流、CORS、Recovery...', 1, 2, 'tech', 1773133200000, 1773133200000, 0);
INSERT INTO `published_article` VALUES (12, 'Wire 依赖注入详解', 'Wire 是 Google 的编译时依赖注入工具。Provider 提供依赖，Injector 组装依赖图。wire.Build() 声明绑定关系，wire gen 生成代码，零运行时开销...', 'Wire 是 Google 的编译时依赖注入工具。Provider 提供依赖，Injector 组装依赖图。wire.Build() 声明绑定关系，wire gen 生成代码，零运行时开销...', 1, 2, 'tech', 1773295200000, 1773295200000, 0);
INSERT INTO `published_article` VALUES (14, 'Go 单元测试与 Mock', 'testify/assert 提供丰富断言，testify/suite 支持测试套件。go.uber.org/mock 生成 interface mock，表驱动测试是 Go 社区标准模式...', 'testify/assert 提供丰富断言，testify/suite 支持测试套件。go.uber.org/mock 生成 interface mock，表驱动测试是 Go 社区标准模式...', 1, 2, 'tech', 1773477000000, 1773477000000, 0);
INSERT INTO `published_article` VALUES (16, '分布式限流算法对比', '令牌桶允许突发流量，漏桶平滑输出，滑动窗口精确控制。Redis + Lua 脚本实现原子操作，避免竞态条件。生产环境推荐滑动窗口...', '令牌桶允许突发流量，漏桶平滑输出，滑动窗口精确控制。Redis + Lua 脚本实现原子操作，避免竞态条件。生产环境推荐滑动窗口...', 1, 2, 'tech', 1773639900000, 1773639900000, 0);
INSERT INTO `published_article` VALUES (18, 'Go 错误处理哲学', 'Go 不用异常，用返回值。errors.Is 判断错误链，errors.As 提取特定类型。fmt.Errorf + %w 包装错误保留上下文。sentinel error 定义在包级别...', 'Go 不用异常，用返回值。errors.Is 判断错误链，errors.As 提取特定类型。fmt.Errorf + %w 包装错误保留上下文。sentinel error 定义在包级别...', 1, 2, 'tech', 1773817200000, 1773817200000, 0);
INSERT INTO `published_article` VALUES (20, '小微书项目架构复盘', '回顾整个项目的分层设计：Handler → Service → Repository → DAO/Cache。Wire DI 解耦依赖，双表设计分离制作库和线上库。前端 Next.js App Router + antd...', '回顾整个项目的分层设计：Handler → Service → Repository → DAO/Cache。Wire DI 解耦依赖，双表设计分离制作库和线上库。前端 Next.js App Router + antd...', 1, 2, 'career', 1773999000000, 1773999000000, 0);
-- 扩充 14 篇（id 22~35）与 article 表一致，保证 published 侧共 25 条，满足榜单分页测试
INSERT INTO `published_article` VALUES (22, 'Redis 数据类型速查', 'String/List/Hash/Set/ZSet 五大基础类型 + Stream/Geo/HyperLogLog 进阶类型，常见命令速记卡。', 'String/List/Hash/Set/ZSet 五大基础类型 + Stream/Geo/HyperLogLog 进阶类型，常见命令速记卡。', 1, 1, 'tech', 1774152000000, 1774152000000, 0);
INSERT INTO `published_article` VALUES (23, '微服务拆分反模式', '分布式单体、过早拆分、共享数据库等常见反模式。拆分原则：业务边界清晰、数据隔离、独立部署。', '分布式单体、过早拆分、共享数据库等常见反模式。拆分原则:业务边界清晰、数据隔离、独立部署。', 1, 2, 'tech', 1774238400000, 1774238400000, 0);
INSERT INTO `published_article` VALUES (24, '前端性能优化清单', '首屏、交互、网络三大维度 20 项清单。Core Web Vitals 指标含义与优化手段。', '首屏、交互、网络三大维度 20 项清单。Core Web Vitals 指标含义与优化手段。', 1, 3, 'tech', 1774324800000, 1774324800000, 0);
INSERT INTO `published_article` VALUES (25, '代码评审 10 条原则', '评审关注正确性、可读性、一致性。避免吹毛求疵，聚焦真实风险。对事不对人。', '评审关注正确性、可读性、一致性。避免吹毛求疵，聚焦真实风险。对事不对人。', 1, 1, 'career', 1774411200000, 1774411200000, 0);
INSERT INTO `published_article` VALUES (26, '远程工作沟通法则', '异步优先、文字留痕、会议前置议程。避免"立即回复"文化拖累深度工作。', '异步优先、文字留痕、会议前置议程。避免"立即回复"文化拖累深度工作。', 1, 2, 'career', 1774497600000, 1774497600000, 0);
INSERT INTO `published_article` VALUES (27, '程序员健康指南', '久坐、视疲劳、颈椎压力三大隐患。站立办公、番茄工作法、每日步行指标。', '久坐、视疲劳、颈椎压力三大隐患。站立办公、番茄工作法、每日步行指标。', 1, 3, 'life', 1774584000000, 1774584000000, 0);
INSERT INTO `published_article` VALUES (28, 'ChatGPT 提示词模板', 'Role/Task/Context/Format 四段式提示词。Few-shot 举例 + Chain-of-Thought 拆步。', 'Role/Task/Context/Format 四段式提示词。Few-shot 举例 + Chain-of-Thought 拆步。', 1, 1, 'ai', 1774670400000, 1774670400000, 0);
INSERT INTO `published_article` VALUES (29, 'AI 绘画工具对比', 'Midjourney / DALL·E / SD 各自定位：M 偏艺术、D 偏精准、SD 偏可控。', 'Midjourney / DALL·E / SD 各自定位：M 偏艺术、D 偏精准、SD 偏可控。', 1, 2, 'ai', 1774756800000, 1774756800000, 0);
INSERT INTO `published_article` VALUES (30, '副业时间管理', '主业不摸鱼、副业不透支。周末 2 小时 × 52 周 > 零散加班。', '主业不摸鱼、副业不透支。周末 2 小时 × 52 周 > 零散加班。', 1, 3, 'life', 1774843200000, 1774843200000, 0);
INSERT INTO `published_article` VALUES (31, 'PostgreSQL vs MySQL', 'PG 在 JSON、CTE、窗口函数、类型系统更强；MySQL 在生态、运维门槛、复制成熟度占优。', 'PG 在 JSON、CTE、窗口函数、类型系统更强；MySQL 在生态、运维门槛、复制成熟度占优。', 1, 1, 'tech', 1774929600000, 1774929600000, 0);
INSERT INTO `published_article` VALUES (32, '技术博客运营心得', 'SEO 关键词选题、Markdown 工作流、图床与 CDN、订阅与互动。', 'SEO 关键词选题、Markdown 工作流、图床与 CDN、订阅与互动。', 1, 2, 'career', 1775016000000, 1775016000000, 0);
INSERT INTO `published_article` VALUES (33, 'Git 高级技巧十则', 'rebase -i / cherry-pick / worktree / bisect / reflog 等进阶命令的典型场景。', 'rebase -i / cherry-pick / worktree / bisect / reflog 等进阶命令的典型场景。', 1, 3, 'tech', 1775102400000, 1775102400000, 0);
INSERT INTO `published_article` VALUES (34, '情绪管理与心流', '识别扳机点、微暂停、番茄节奏。心流的进入条件：清晰目标 + 即时反馈 + 挑战匹配。', '识别扳机点、微暂停、番茄节奏。心流的进入条件：清晰目标 + 即时反馈 + 挑战匹配。', 1, 1, 'life', 1775188800000, 1775188800000, 0);
INSERT INTO `published_article` VALUES (35, 'Claude 对话技巧', '长上下文、工具调用、结构化输出。Artifacts 和 MCP 扩展能力使用场景。', '长上下文、工具调用、结构化输出。Artifacts 和 MCP 扩展能力使用场景。', 1, 2, 'ai', 1775275200000, 1775275200000, 0);

-- ----------------------------
-- 本地测试辅助：把文章 created_at/updated_at 按 id 散到最近 72h（让 HotScore 分母不会被旧日期压成 0）
-- id 越小越新：id=1 → 3h 前，id=20 → 60h 前
-- ----------------------------
UPDATE `article` SET
  `created_at` = UNIX_TIMESTAMP(NOW())*1000 - (id * 3 * 3600 * 1000),
  `updated_at` = UNIX_TIMESTAMP(NOW())*1000 - (id * 3 * 3600 * 1000)
WHERE id > 0;
UPDATE `published_article` SET
  `created_at` = UNIX_TIMESTAMP(NOW())*1000 - (id * 3 * 3600 * 1000),
  `updated_at` = UNIX_TIMESTAMP(NOW())*1000 - (id * 3 * 3600 * 1000)
WHERE id > 0;

-- ----------------------------
-- Table structure for user
-- ----------------------------
DROP TABLE IF EXISTS `user`;
CREATE TABLE `user`  (
  `id` bigint NOT NULL AUTO_INCREMENT COMMENT '主键',
  `created_at` bigint NULL DEFAULT NULL COMMENT '注册时间（Unix 毫秒）',
  `updated_at` bigint NULL DEFAULT NULL COMMENT '更新时间（Unix 毫秒）',
  `deleted_at` bigint NOT NULL DEFAULT 0 COMMENT '软删除时间（0=未删）',
  `email` varchar(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NULL DEFAULT NULL COMMENT '邮箱（登录凭证）',
  `password` varchar(256) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NULL DEFAULT NULL COMMENT 'bcrypt 密码',
  `nickname` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NULL DEFAULT NULL COMMENT '昵称（展示）',
  `birthday` bigint NULL DEFAULT NULL COMMENT '生日（Unix 毫秒，0 点）',
  `about_me` text CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NULL COMMENT '个人简介',
  `phone` varchar(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NULL DEFAULT NULL COMMENT '手机号（登录凭证）',
  `wechat_open_id` varchar(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NULL DEFAULT NULL COMMENT '微信 OpenID（登录凭证）',
  `wechat_union_id` longtext CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NULL COMMENT '微信 UnionID（多端同用户识别）',
  PRIMARY KEY (`id`) USING BTREE,
  UNIQUE INDEX `uni_user_email`(`email` ASC) USING BTREE,
  UNIQUE INDEX `uni_user_phone`(`phone` ASC) USING BTREE,
  UNIQUE INDEX `uni_user_wechat_open_id`(`wechat_open_id` ASC) USING BTREE,
  INDEX `idx_user_deleted_at`(`deleted_at` ASC) USING BTREE
) ENGINE = InnoDB AUTO_INCREMENT = 5202 CHARACTER SET = utf8mb4 COLLATE = utf8mb4_0900_ai_ci ROW_FORMAT = Dynamic COMMENT = '用户账号（支持邮箱/手机/微信多种登录）';

-- ----------------------------
-- Records of user (birthday/updated_at = Unix 毫秒)
-- ----------------------------
INSERT INTO `user` VALUES (1, NULL, 1774621432248, 0, '3236447743@qq.com', '$2a$10$QAS0Xqqoe3DtBxzVev5NzOl02HLq2rJJrf4dJ3aOyyVxHIQ.J8FNW', 'tommy', 1774621431000, 'see my name.', NULL, NULL, NULL);
INSERT INTO `user` VALUES (101, NULL, NULL, 0, '123456@qq.com', '$2a$10$QAS0Xqqoe3DtBxzVev5NzOl02HLq2rJJrf4dJ3aOyyVxHIQ.J8FNW', 'tommy', NULL, 'say my name', NULL, NULL, NULL);
INSERT INTO `user` VALUES (102, NULL, NULL, 0, NULL, '', 'tommy', NULL, 'say my name', '18608261234', NULL, NULL);

-- ----------------------------
-- Table structure for interaction
-- ----------------------------
CREATE TABLE IF NOT EXISTS `interaction`  (
  `id` bigint NOT NULL AUTO_INCREMENT COMMENT '主键',
  `biz` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT '' COMMENT '业务类型：article/video 等',
  `biz_id` bigint NOT NULL DEFAULT 0 COMMENT '业务对象 id（文章 id / 视频 id）',
  `read_count` bigint NOT NULL DEFAULT 0 COMMENT '阅读数',
  `like_count` bigint NOT NULL DEFAULT 0 COMMENT '点赞数',
  `collect_count` bigint NOT NULL DEFAULT 0 COMMENT '收藏数',
  `created_at` bigint NULL DEFAULT NULL COMMENT '创建时间（Unix 毫秒）',
  `updated_at` bigint NULL DEFAULT NULL COMMENT '更新时间（Unix 毫秒）',
  PRIMARY KEY (`id`) USING BTREE,
  UNIQUE INDEX `uk_interaction_biz`(`biz` ASC, `biz_id` ASC) USING BTREE
) ENGINE = InnoDB CHARACTER SET = utf8mb4 COLLATE = utf8mb4_0900_ai_ci ROW_FORMAT = Dynamic COMMENT = '互动聚合表（每个业务对象一行，聚合阅读/点赞/收藏数）';

-- ----------------------------
-- Table structure for user_interaction
-- ----------------------------
CREATE TABLE IF NOT EXISTS `user_interaction`  (
  `id` bigint NOT NULL AUTO_INCREMENT COMMENT '主键',
  `biz` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT '' COMMENT '业务类型',
  `biz_id` bigint NOT NULL DEFAULT 0 COMMENT '业务对象 id',
  `user_id` bigint NOT NULL DEFAULT 0 COMMENT '用户 id',
  `liked` tinyint(1) NOT NULL DEFAULT 0 COMMENT '是否点赞：0=否 1=是',
  `collected` tinyint(1) NOT NULL DEFAULT 0 COMMENT '是否收藏：0=否 1=是',
  `created_at` bigint NULL DEFAULT NULL COMMENT '创建时间（Unix 毫秒）',
  `updated_at` bigint NULL DEFAULT NULL COMMENT '更新时间（Unix 毫秒）',
  PRIMARY KEY (`id`) USING BTREE,
  UNIQUE INDEX `uk_user_interaction_biz`(`biz` ASC, `biz_id` ASC, `user_id` ASC) USING BTREE
) ENGINE = InnoDB CHARACTER SET = utf8mb4 COLLATE = utf8mb4_0900_ai_ci ROW_FORMAT = Dynamic COMMENT = '用户互动关系（每个 user × biz 一行，记录点赞/收藏状态）';

-- ----------------------------
-- Table structure for conversation (AI 客服对话)
-- ----------------------------
CREATE TABLE IF NOT EXISTS `conversation` (
  `id` bigint NOT NULL AUTO_INCREMENT COMMENT '主键',
  `user_id` bigint NOT NULL COMMENT '用户 id',
  `title` varchar(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT '' COMMENT '会话标题（自动从首条 user 消息截取）',
  `created_at` bigint NOT NULL DEFAULT 0 COMMENT '创建时间（Unix 毫秒）',
  `updated_at` bigint NOT NULL DEFAULT 0 COMMENT '最后一条消息时间（Unix 毫秒，排序用）',
  `deleted_at` bigint NOT NULL DEFAULT 0 COMMENT '软删除时间（Unix 毫秒，0=未删）',
  PRIMARY KEY (`id`) USING BTREE,
  INDEX `idx_conversation_user_updated` (`user_id` ASC, `updated_at` DESC) USING BTREE
) ENGINE = InnoDB CHARACTER SET = utf8mb4 COLLATE = utf8mb4_0900_ai_ci ROW_FORMAT = Dynamic COMMENT = 'AI 客服会话（对话列表）';

-- ----------------------------
-- Table structure for message (AI 客服消息)
-- ----------------------------
CREATE TABLE IF NOT EXISTS `message` (
  `id` bigint NOT NULL AUTO_INCREMENT COMMENT '主键',
  `conversation_id` bigint NOT NULL COMMENT '所属会话 id',
  `role` varchar(16) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL COMMENT '角色：user/assistant/system/tool',
  `content` text CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL COMMENT '消息内容（长文本）',
  `tool_calls` json NULL COMMENT 'LLM 工具调用 JSON（assistant 消息的 function_call/tool_calls）',
  `token_used` int NOT NULL DEFAULT 0 COMMENT '本条消息 token 消耗（计费/监控）',
  `feedback` tinyint NOT NULL DEFAULT 0 COMMENT '用户反馈：-1=踩 0=未反馈 1=赞',
  `created_at` bigint NOT NULL DEFAULT 0 COMMENT '创建时间（Unix 毫秒，会话内顺序）',
  `updated_at` bigint NOT NULL DEFAULT 0 COMMENT '更新时间（Unix 毫秒）',
  `deleted_at` bigint NOT NULL DEFAULT 0 COMMENT '软删除时间（Unix 毫秒，0=未删）',
  PRIMARY KEY (`id`) USING BTREE,
  INDEX `idx_message_conversation_created` (`conversation_id` ASC, `created_at` ASC) USING BTREE
) ENGINE = InnoDB CHARACTER SET = utf8mb4 COLLATE = utf8mb4_0900_ai_ci ROW_FORMAT = Dynamic COMMENT = 'AI 客服消息（按 conversation_id 聚合）';

-- ----------------------------
-- Table structure for ai_click_events (AI 点击埋点)
-- ----------------------------
CREATE TABLE IF NOT EXISTS `ai_click_events` (
  `id` bigint NOT NULL AUTO_INCREMENT COMMENT '主键',
  `user_id` bigint NOT NULL COMMENT '点击用户 id',
  `article_id` bigint NOT NULL COMMENT '被点击文章 id',
  `conversation_id` bigint NOT NULL COMMENT '关联会话 id（非 AI 场景为 0）',
  `source` varchar(32) NOT NULL DEFAULT 'ai_chat' COMMENT '点击来源：ai_chat / ranking:{dim}:{rank} / search 等',
  `created_at` bigint NOT NULL DEFAULT 0 COMMENT '创建时间（Unix 毫秒）',
  `updated_at` bigint NOT NULL DEFAULT 0 COMMENT '更新时间（Unix 毫秒）',
  `deleted_at` bigint NOT NULL DEFAULT 0 COMMENT '软删除时间（0=未删）',
  PRIMARY KEY (`id`) USING BTREE,
  UNIQUE INDEX `uk_ai_click_events_dedup` (`user_id` ASC, `article_id` ASC, `conversation_id` ASC, `source` ASC) USING BTREE,
  INDEX `idx_ai_click_events_article_id` (`article_id` ASC) USING BTREE,
  INDEX `idx_ai_click_events_created_at` (`created_at` ASC) USING BTREE
) ENGINE = InnoDB CHARACTER SET = utf8mb4 COLLATE = utf8mb4_0900_ai_ci ROW_FORMAT = Dynamic COMMENT = '点击埋点事件（AI 对话 / 榜单 / 搜索等入口的文章点击）';

-- ----------------------------
-- Table structure for article_ranking (榜单归档表)
-- 每天 cron 在 00:10（测试 10min）把 Redis ZSet 里的 Top100 落库。
-- dimension: hot / new / best / category
-- category: 分区榜时为 tech/career/life/ai/other，其他维度为 ''
-- snapshot: JSON 序列化的 domain.ArticleRanking（含 title/author/stats，冻结后 article 变了也不影响）
-- ----------------------------
DROP TABLE IF EXISTS `article_ranking`;
CREATE TABLE `article_ranking` (
  `id` bigint NOT NULL AUTO_INCREMENT COMMENT '主键',
  `date` varchar(10) NOT NULL COMMENT '日分区键 YYYY-MM-DD（业务时区 Asia/Shanghai）',
  `dimension` varchar(16) NOT NULL COMMENT '榜单维度：hot/new/best/category',
  `category` varchar(32) NOT NULL DEFAULT '' COMMENT '分区榜的分类（tech/career 等）；总榜为空串',
  `rank` int NOT NULL COMMENT '名次（1 起，1 = 第一名）',
  `article_id` bigint NOT NULL COMMENT '文章 id',
  `score` double NOT NULL DEFAULT 0 COMMENT '分数（hot=HotScore / new=publish_ms / best=WilsonLB）',
  `snapshot` json NULL COMMENT '条目快照 JSON（含标题/作者/互动数，归档冻结，article 变动不受影响）',
  `created_at` bigint NOT NULL DEFAULT 0 COMMENT '归档时间（Unix 毫秒）',
  PRIMARY KEY (`id`) USING BTREE,
  UNIQUE KEY `uk_article_ranking_date_dim_cat_rank` (`date` ASC, `dimension` ASC, `category` ASC, `rank` ASC) USING BTREE,
  INDEX `idx_article_ranking_date` (`date` ASC) USING BTREE
) ENGINE = InnoDB CHARACTER SET = utf8mb4 COLLATE = utf8mb4_0900_ai_ci ROW_FORMAT = Dynamic COMMENT = '文章榜单归档表（每天 cron 把 Redis Top100 落库）';

-- ----------------------------
-- 本地测试辅助：给 published 文章造互动数据（让 HotScore 分子非零，分数显著）
-- read_count / like_count / collect_count 按榜单预期排序递减
-- ----------------------------
INSERT INTO `interaction` (biz, biz_id, read_count, like_count, collect_count, created_at, updated_at) VALUES
('article', 1,  12300, 1200, 340, UNIX_TIMESTAMP(NOW())*1000, UNIX_TIMESTAMP(NOW())*1000),
('article', 2,   9800,  890, 210, UNIX_TIMESTAMP(NOW())*1000, UNIX_TIMESTAMP(NOW())*1000),
('article', 4,   8200,  760, 180, UNIX_TIMESTAMP(NOW())*1000, UNIX_TIMESTAMP(NOW())*1000),
('article', 6,   5500,  440, 120, UNIX_TIMESTAMP(NOW())*1000, UNIX_TIMESTAMP(NOW())*1000),
('article', 8,   5100,  520, 140, UNIX_TIMESTAMP(NOW())*1000, UNIX_TIMESTAMP(NOW())*1000),
('article', 10,  4300,  410, 125, UNIX_TIMESTAMP(NOW())*1000, UNIX_TIMESTAMP(NOW())*1000),
('article', 12,  3800,  360,  95, UNIX_TIMESTAMP(NOW())*1000, UNIX_TIMESTAMP(NOW())*1000),
('article', 14,  2600,  220,  70, UNIX_TIMESTAMP(NOW())*1000, UNIX_TIMESTAMP(NOW())*1000),
('article', 16,  2100,  180,  55, UNIX_TIMESTAMP(NOW())*1000, UNIX_TIMESTAMP(NOW())*1000),
('article', 18,  1400,  130,  42, UNIX_TIMESTAMP(NOW())*1000, UNIX_TIMESTAMP(NOW())*1000),
('article', 20,   800,   60,  18, UNIX_TIMESTAMP(NOW())*1000, UNIX_TIMESTAMP(NOW())*1000),
-- 扩充 id 22~35，分数递减，和 published_article 扩充集一致
('article', 22,   700,   55,  16, UNIX_TIMESTAMP(NOW())*1000, UNIX_TIMESTAMP(NOW())*1000),
('article', 23,   650,   50,  14, UNIX_TIMESTAMP(NOW())*1000, UNIX_TIMESTAMP(NOW())*1000),
('article', 24,   600,   45,  12, UNIX_TIMESTAMP(NOW())*1000, UNIX_TIMESTAMP(NOW())*1000),
('article', 25,   550,   40,  11, UNIX_TIMESTAMP(NOW())*1000, UNIX_TIMESTAMP(NOW())*1000),
('article', 26,   500,   38,  10, UNIX_TIMESTAMP(NOW())*1000, UNIX_TIMESTAMP(NOW())*1000),
('article', 27,   450,   35,   9, UNIX_TIMESTAMP(NOW())*1000, UNIX_TIMESTAMP(NOW())*1000),
('article', 28,   400,   32,   8, UNIX_TIMESTAMP(NOW())*1000, UNIX_TIMESTAMP(NOW())*1000),
('article', 29,   350,   28,   7, UNIX_TIMESTAMP(NOW())*1000, UNIX_TIMESTAMP(NOW())*1000),
('article', 30,   300,   25,   6, UNIX_TIMESTAMP(NOW())*1000, UNIX_TIMESTAMP(NOW())*1000),
('article', 31,   250,   22,   5, UNIX_TIMESTAMP(NOW())*1000, UNIX_TIMESTAMP(NOW())*1000),
('article', 32,   200,   18,   5, UNIX_TIMESTAMP(NOW())*1000, UNIX_TIMESTAMP(NOW())*1000),
('article', 33,   150,   15,   4, UNIX_TIMESTAMP(NOW())*1000, UNIX_TIMESTAMP(NOW())*1000),
('article', 34,   100,   12,   3, UNIX_TIMESTAMP(NOW())*1000, UNIX_TIMESTAMP(NOW())*1000),
('article', 35,    50,    8,   2, UNIX_TIMESTAMP(NOW())*1000, UNIX_TIMESTAMP(NOW())*1000)
ON DUPLICATE KEY UPDATE
  read_count    = VALUES(read_count),
  like_count    = VALUES(like_count),
  collect_count = VALUES(collect_count),
  updated_at    = UNIX_TIMESTAMP(NOW())*1000;

-- ----------------------------
-- 模拟榜单历史数据（日期相对 CURDATE() 动态生成，脚本任何时候跑都有参考数据）：
--   CURDATE()-2 天：hot Top5（前天的历史榜）
--   CURDATE()-1 天：hot Top25 / new Top3 / best Top3 / category×4（昨日归档，fallback 首选）
-- 今日实时榜由 cron 写到 Redis，不在此表
-- 覆盖 published 表全部 25 条 id（1,2,4,6,8,10,12,14,16,18,20,22~35）—— Top20 触发前端分页 1/2
-- ----------------------------
-- date 列是 varchar(10) 'YYYY-MM-DD'（日分区键，行业标准做法）
-- 业务时区 Asia/Shanghai：用 CONVERT_TZ 把 UTC_TIMESTAMP 转 +08:00，不依赖 MySQL 服务器时区
SET @sh_now   := CONVERT_TZ(UTC_TIMESTAMP(), '+00:00', '+08:00');
SET @d_prev1  := DATE_FORMAT(@sh_now - INTERVAL 1 DAY, '%Y-%m-%d');
SET @d_prev2  := DATE_FORMAT(@sh_now - INTERVAL 2 DAY, '%Y-%m-%d');
SET @ts_prev1 := (UNIX_TIMESTAMP(UTC_TIMESTAMP()) - 86400) * 1000;
SET @ts_prev2 := (UNIX_TIMESTAMP(UTC_TIMESTAMP()) - 2 * 86400) * 1000;

INSERT INTO `article_ranking` (`date`, `dimension`, `category`, `rank`, `article_id`, `score`, `snapshot`, `created_at`) VALUES
-- 前天 hot Top5
(@d_prev2, 'hot', '', 1, 2, 8920, '{"rank":1,"articleId":2,"title":"GORM 使用技巧总结","author":{"id":1,"name":""},"category":"tech","clicks":10500,"likes":980,"collects":280,"score":8920,"scoreRatio":1,"trend":"same","trendDelta":0}', @ts_prev2),
(@d_prev2, 'hot', '', 2, 1, 7830, '{"rank":2,"articleId":1,"title":"Go 并发编程入门","author":{"id":1,"name":""},"category":"tech","clicks":9200,"likes":870,"collects":240,"score":7830,"scoreRatio":0.878,"trend":"same","trendDelta":0}', @ts_prev2),
(@d_prev2, 'hot', '', 3, 10, 6650, '{"rank":3,"articleId":10,"title":"Gin 中间件开发指南","author":{"id":1,"name":""},"category":"tech","clicks":7800,"likes":720,"collects":200,"score":6650,"scoreRatio":0.745,"trend":"same","trendDelta":0}', @ts_prev2),
(@d_prev2, 'hot', '', 4, 12, 5480, '{"rank":4,"articleId":12,"title":"Wire 依赖注入详解","author":{"id":1,"name":""},"category":"tech","clicks":6500,"likes":580,"collects":165,"score":5480,"scoreRatio":0.614,"trend":"same","trendDelta":0}', @ts_prev2),
(@d_prev2, 'hot', '', 5, 4, 4320, '{"rank":5,"articleId":4,"title":"JWT 双 Token 认证实践","author":{"id":1,"name":""},"category":"tech","clicks":5200,"likes":450,"collects":130,"score":4320,"scoreRatio":0.484,"trend":"same","trendDelta":0}', @ts_prev2),
-- 昨日 hot Top5（相对前天有升降，前端看"历史榜单"选昨日默认出这 5 条）
(@d_prev1, 'hot', '', 1, 1, 9832, '{"rank":1,"articleId":1,"title":"Go 并发编程入门","author":{"id":1,"name":""},"category":"tech","clicks":12300,"likes":1200,"collects":340,"score":9832,"scoreRatio":1,"trend":"up","trendDelta":1}', @ts_prev1),
(@d_prev1, 'hot', '', 2, 12, 7621, '{"rank":2,"articleId":12,"title":"Wire 依赖注入详解","author":{"id":1,"name":""},"category":"tech","clicks":9800,"likes":890,"collects":210,"score":7621,"scoreRatio":0.775,"trend":"up","trendDelta":2}', @ts_prev1),
(@d_prev1, 'hot', '', 3, 2, 6890, '{"rank":3,"articleId":2,"title":"GORM 使用技巧总结","author":{"id":1,"name":""},"category":"tech","clicks":8200,"likes":760,"collects":180,"score":6890,"scoreRatio":0.701,"trend":"down","trendDelta":2}', @ts_prev1),
(@d_prev1, 'hot', '', 4, 10, 5210, '{"rank":4,"articleId":10,"title":"Gin 中间件开发指南","author":{"id":1,"name":""},"category":"tech","clicks":5100,"likes":520,"collects":140,"score":5210,"scoreRatio":0.530,"trend":"down","trendDelta":1}', @ts_prev1),
(@d_prev1, 'hot', '', 5, 8, 4350, '{"rank":5,"articleId":8,"title":"Docker 容器化部署 Go 应用","author":{"id":1,"name":""},"category":"tech","clicks":4300,"likes":410,"collects":125,"score":4350,"scoreRatio":0.443,"trend":"new","trendDelta":0}', @ts_prev1),
-- 昨日 hot 6~20 名（补足 20 条满足 pageSize=20 分页测试；Redis 空时 fallback 走这里）
(@d_prev1, 'hot', '', 6,  4, 3900, '{"rank":6,"articleId":4,"title":"JWT 双 Token 认证实践","author":{"id":1,"name":""},"category":"tech","clicks":5200,"likes":450,"collects":130,"score":3900,"scoreRatio":0.397,"trend":"same","trendDelta":0}', @ts_prev1),
(@d_prev1, 'hot', '', 7, 14, 3500, '{"rank":7,"articleId":14,"title":"Go 单元测试与 Mock","author":{"id":1,"name":""},"category":"tech","clicks":2600,"likes":220,"collects":70,"score":3500,"scoreRatio":0.356,"trend":"up","trendDelta":2}', @ts_prev1),
(@d_prev1, 'hot', '', 8, 16, 3100, '{"rank":8,"articleId":16,"title":"分布式限流算法对比","author":{"id":1,"name":""},"category":"tech","clicks":2100,"likes":180,"collects":55,"score":3100,"scoreRatio":0.315,"trend":"down","trendDelta":1}', @ts_prev1),
(@d_prev1, 'hot', '', 9,  6, 2800, '{"rank":9,"articleId":6,"title":"React 19 新特性速览","author":{"id":1,"name":""},"category":"other","clicks":5500,"likes":440,"collects":120,"score":2800,"scoreRatio":0.285,"trend":"new","trendDelta":0}', @ts_prev1),
(@d_prev1, 'hot', '', 10, 18, 2500, '{"rank":10,"articleId":18,"title":"Go 错误处理哲学","author":{"id":1,"name":""},"category":"tech","clicks":1400,"likes":130,"collects":42,"score":2500,"scoreRatio":0.254,"trend":"same","trendDelta":0}', @ts_prev1),
(@d_prev1, 'hot', '', 11, 20, 2200, '{"rank":11,"articleId":20,"title":"小微书项目架构复盘","author":{"id":1,"name":""},"category":"career","clicks":800,"likes":60,"collects":18,"score":2200,"scoreRatio":0.224,"trend":"up","trendDelta":1}', @ts_prev1),
(@d_prev1, 'hot', '', 12, 22, 1950, '{"rank":12,"articleId":22,"title":"Redis 数据类型速查","author":{"id":1,"name":""},"category":"tech","clicks":700,"likes":55,"collects":16,"score":1950,"scoreRatio":0.198,"trend":"new","trendDelta":0}', @ts_prev1),
(@d_prev1, 'hot', '', 13, 23, 1700, '{"rank":13,"articleId":23,"title":"微服务拆分反模式","author":{"id":2,"name":""},"category":"tech","clicks":650,"likes":50,"collects":14,"score":1700,"scoreRatio":0.173,"trend":"down","trendDelta":2}', @ts_prev1),
(@d_prev1, 'hot', '', 14, 24, 1500, '{"rank":14,"articleId":24,"title":"前端性能优化清单","author":{"id":3,"name":""},"category":"tech","clicks":600,"likes":45,"collects":12,"score":1500,"scoreRatio":0.153,"trend":"same","trendDelta":0}', @ts_prev1),
(@d_prev1, 'hot', '', 15, 25, 1300, '{"rank":15,"articleId":25,"title":"代码评审 10 条原则","author":{"id":1,"name":""},"category":"career","clicks":550,"likes":40,"collects":11,"score":1300,"scoreRatio":0.132,"trend":"new","trendDelta":0}', @ts_prev1),
(@d_prev1, 'hot', '', 16, 26, 1150, '{"rank":16,"articleId":26,"title":"远程工作沟通法则","author":{"id":2,"name":""},"category":"career","clicks":500,"likes":38,"collects":10,"score":1150,"scoreRatio":0.117,"trend":"up","trendDelta":3}', @ts_prev1),
(@d_prev1, 'hot', '', 17, 27, 1000, '{"rank":17,"articleId":27,"title":"程序员健康指南","author":{"id":3,"name":""},"category":"life","clicks":450,"likes":35,"collects":9,"score":1000,"scoreRatio":0.102,"trend":"new","trendDelta":0}', @ts_prev1),
(@d_prev1, 'hot', '', 18, 28, 850,  '{"rank":18,"articleId":28,"title":"ChatGPT 提示词模板","author":{"id":1,"name":""},"category":"ai","clicks":400,"likes":32,"collects":8,"score":850,"scoreRatio":0.086,"trend":"same","trendDelta":0}', @ts_prev1),
(@d_prev1, 'hot', '', 19, 29, 700,  '{"rank":19,"articleId":29,"title":"AI 绘画工具对比","author":{"id":2,"name":""},"category":"ai","clicks":350,"likes":28,"collects":7,"score":700,"scoreRatio":0.071,"trend":"down","trendDelta":1}', @ts_prev1),
(@d_prev1, 'hot', '', 20, 30, 580,  '{"rank":20,"articleId":30,"title":"副业时间管理","author":{"id":3,"name":""},"category":"life","clicks":300,"likes":25,"collects":6,"score":580,"scoreRatio":0.059,"trend":"new","trendDelta":0}', @ts_prev1),
(@d_prev1, 'hot', '', 21, 31, 470,  '{"rank":21,"articleId":31,"title":"PostgreSQL vs MySQL","author":{"id":1,"name":""},"category":"tech","clicks":250,"likes":22,"collects":5,"score":470,"scoreRatio":0.048,"trend":"new","trendDelta":0}', @ts_prev1),
(@d_prev1, 'hot', '', 22, 32, 380,  '{"rank":22,"articleId":32,"title":"技术博客运营心得","author":{"id":2,"name":""},"category":"career","clicks":200,"likes":18,"collects":5,"score":380,"scoreRatio":0.039,"trend":"down","trendDelta":1}', @ts_prev1),
(@d_prev1, 'hot', '', 23, 33, 300,  '{"rank":23,"articleId":33,"title":"Git 高级技巧十则","author":{"id":3,"name":""},"category":"tech","clicks":150,"likes":15,"collects":4,"score":300,"scoreRatio":0.031,"trend":"same","trendDelta":0}', @ts_prev1),
(@d_prev1, 'hot', '', 24, 34, 230,  '{"rank":24,"articleId":34,"title":"情绪管理与心流","author":{"id":1,"name":""},"category":"life","clicks":100,"likes":12,"collects":3,"score":230,"scoreRatio":0.023,"trend":"new","trendDelta":0}', @ts_prev1),
(@d_prev1, 'hot', '', 25, 35, 150,  '{"rank":25,"articleId":35,"title":"Claude 对话技巧","author":{"id":2,"name":""},"category":"ai","clicks":50,"likes":8,"collects":2,"score":150,"scoreRatio":0.015,"trend":"new","trendDelta":0}', @ts_prev1),
-- 昨日 new Top3
(@d_prev1, 'new', '', 1, 20, 1773999000000, '{"rank":1,"articleId":20,"title":"小微书项目架构复盘","author":{"id":1,"name":""},"category":"career","clicks":520,"likes":42,"collects":15,"score":1773999000000,"scoreRatio":1,"trend":"new","trendDelta":0}', @ts_prev1),
(@d_prev1, 'new', '', 2, 18, 1773817200000, '{"rank":2,"articleId":18,"title":"Go 错误处理哲学","author":{"id":1,"name":""},"category":"tech","clicks":380,"likes":28,"collects":9,"score":1773817200000,"scoreRatio":0.9999,"trend":"new","trendDelta":0}', @ts_prev1),
(@d_prev1, 'new', '', 3, 16, 1773639900000, '{"rank":3,"articleId":16,"title":"分布式限流算法对比","author":{"id":1,"name":""},"category":"tech","clicks":260,"likes":22,"collects":7,"score":1773639900000,"scoreRatio":0.9999,"trend":"new","trendDelta":0}', @ts_prev1),
-- 昨日 best Top3（Wilson 分）
(@d_prev1, 'best', '', 1, 1, 0.82, '{"rank":1,"articleId":1,"title":"Go 并发编程入门","author":{"id":1,"name":""},"category":"tech","clicks":12300,"likes":1200,"collects":340,"score":0.82,"scoreRatio":1,"trend":"same","trendDelta":0}', @ts_prev1),
(@d_prev1, 'best', '', 2, 2, 0.78, '{"rank":2,"articleId":2,"title":"GORM 使用技巧总结","author":{"id":1,"name":""},"category":"tech","clicks":8200,"likes":760,"collects":180,"score":0.78,"scoreRatio":0.951,"trend":"same","trendDelta":0}', @ts_prev1),
(@d_prev1, 'best', '', 3, 10, 0.75, '{"rank":3,"articleId":10,"title":"Gin 中间件开发指南","author":{"id":1,"name":""},"category":"tech","clicks":5100,"likes":520,"collects":140,"score":0.75,"scoreRatio":0.915,"trend":"up","trendDelta":1}', @ts_prev1),
-- 昨日分区榜：tech / other / career
(@d_prev1, 'category', 'tech',   1, 1,  9832, '{"rank":1,"articleId":1,"title":"Go 并发编程入门","author":{"id":1,"name":""},"category":"tech","clicks":12300,"likes":1200,"collects":340,"score":9832,"scoreRatio":1,"trend":"same","trendDelta":0}', @ts_prev1),
(@d_prev1, 'category', 'tech',   2, 12, 7621, '{"rank":2,"articleId":12,"title":"Wire 依赖注入详解","author":{"id":1,"name":""},"category":"tech","clicks":9800,"likes":890,"collects":210,"score":7621,"scoreRatio":0.775,"trend":"same","trendDelta":0}', @ts_prev1),
(@d_prev1, 'category', 'other',  1, 6,  4220, '{"rank":1,"articleId":6,"title":"React 19 新特性速览","author":{"id":1,"name":""},"category":"other","clicks":5500,"likes":440,"collects":120,"score":4220,"scoreRatio":1,"trend":"same","trendDelta":0}', @ts_prev1),
(@d_prev1, 'category', 'career', 1, 20, 1850, '{"rank":1,"articleId":20,"title":"小微书项目架构复盘","author":{"id":1,"name":""},"category":"career","clicks":2800,"likes":190,"collects":65,"score":1850,"scoreRatio":1,"trend":"same","trendDelta":0}', @ts_prev1)
ON DUPLICATE KEY UPDATE
  article_id = VALUES(article_id),
  score      = VALUES(score),
  snapshot   = VALUES(snapshot),
  created_at = VALUES(created_at);

SET FOREIGN_KEY_CHECKS = 1;

-- ============================================================
-- 调试视图：将 bigint 毫秒时间戳显示为可读时间
-- 用法: SELECT * FROM v_article;
-- ============================================================

CREATE OR REPLACE VIEW v_article AS
SELECT id, title, abstract, author_id, status, category,
  FROM_UNIXTIME(created_at / 1000) AS created_at,
  FROM_UNIXTIME(updated_at / 1000) AS updated_at
FROM article;

CREATE OR REPLACE VIEW v_published_article AS
SELECT id, title, abstract, author_id, status, category,
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

CREATE OR REPLACE VIEW v_article_ranking AS
SELECT id, `date`, dimension, category, `rank`, article_id, score,
  LEFT(snapshot, 120) AS snapshot_preview,
  FROM_UNIXTIME(created_at / 1000) AS created_at
FROM article_ranking
ORDER BY `date` DESC, dimension, category, `rank`;
