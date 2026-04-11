# 小微书 — 阅读量统计 PRD

## 1. 功能概述

为文章模块增加阅读量统计能力，让**读者**看到文章热度，让**作者**了解内容表现。

### 核心目标

- 读者打开文章详情时自动记录一次阅读
- 文章广场/详情页展示阅读量
- 作者后台展示各文章阅读量

---

## 2. 用户故事

| 优先级 | 角色 | 故事 |
|--------|------|------|
| P0 | 读者 | 打开文章详情页时，系统自动记录阅读量 |
| P0 | 读者 | 在文章详情页看到阅读量，了解文章热度 |
| P0 | 读者 | 在文章广场列表看到每篇文章的阅读量 |
| P0 | 作者 | 在我的文章列表看到每篇文章的阅读量 |
| P1 | 作者 | 在文章详情（作者视角）看到阅读量 |

---

## 3. 计数规则

| 规则 | 说明 |
|------|------|
| 触发时机 | 读者请求 `/article/reader/detail` 时触发 |
| 计数粒度 | 每次请求 +1（简单计数，不去重） |
| 作者自阅 | 计入阅读量（不区分身份） |
| 爬虫/刷量 | V1 不做防刷，后续可加 IP + UA 限流 |
| 未发布文章 | 不统计（仅 published_article 可被阅读） |

> **V2 去重策略（待办）**：基于 `(article_id, user_id/ip)` 相同用户/IP 24h 内只计 1 次

---

## 4. 数据模型

### interactive 表（互动统计）

为后续点赞/收藏/评论预留扩展，使用通用互动表：

```sql
CREATE TABLE interactive (
    id         BIGINT PRIMARY KEY AUTO_INCREMENT,
    biz_id     BIGINT NOT NULL,          -- 业务 ID（文章 ID）
    biz        VARCHAR(64) NOT NULL,     -- 业务类型（"article"）
    read_cnt   BIGINT NOT NULL DEFAULT 0, -- 阅读量
    like_cnt   BIGINT NOT NULL DEFAULT 0, -- 点赞数（预留）
    collect_cnt BIGINT NOT NULL DEFAULT 0, -- 收藏数（预留）
    ctime      BIGINT NOT NULL,
    utime      BIGINT NOT NULL,
    UNIQUE KEY uk_biz (biz_id, biz)
);
```

**设计考量**：
- 使用 `biz + biz_id` 通用设计，未来可复用于其他业务
- 计数字段用 `BIGINT`，避免溢出
- UPSERT 模式：`INSERT ... ON DUPLICATE KEY UPDATE read_cnt = read_cnt + 1`

---

## 5. API 接口

### 新增接口

| Method | 路径 | 说明 | 认证 |
|--------|------|------|------|
| POST | `/article/reader/detail` | 阅读文章（**复用现有接口，内部增加计数逻辑**） | 否 |

### 修改接口（响应增加 readCnt 字段）

| Method | 路径 | 变化 |
|--------|------|------|
| POST | `/article/reader/page` | 响应增加 `readCnt` |
| POST | `/article/reader/detail` | 响应增加 `readCnt` |
| POST | `/article/page` | 响应增加 `readCnt`（作者视角） |
| POST | `/article/detail` | 响应增加 `readCnt`（作者视角） |

### 响应结构变化

```json
// 文章列表项增加
{
  "id": 1,
  "title": "...",
  "abstract": "...",
  "readCnt": 1234,
  ...
}

// 文章详情增加
{
  "id": 1,
  "title": "...",
  "content": "...",
  "readCnt": 1234,
  ...
}
```

---

## 6. 缓存策略

| 项 | 方案 |
|---|---|
| 缓存位置 | Redis Hash `interactive:article:{id}` |
| 字段 | `read_cnt`（后续加 `like_cnt` `collect_cnt`） |
| 写入 | 阅读时先 `HINCRBY` Redis，异步/同步写 DB |
| 读取 | 先查 Redis，miss 时回源 DB 并回填 |
| TTL | 24h + 随机抖动 |
| 一致性 | 最终一致，Redis 为准，DB 为兜底 |

---

## 7. 页面展示

### 7.1 文章广场（Feed）

卡片底部元信息区增加阅读量：
```
作者名 · 2024-01-01 · 👁 1234
```

### 7.2 文章阅读页

标题下方元信息区增加阅读量：
```
作者名 · 2024-01-01 · 👁 1234 次阅读
```

### 7.3 作者文章列表

- 桌面 Table：增加「阅读量」列
- 移动端卡片：底部显示阅读量

### 7.4 数字格式化

| 范围 | 展示 |
|------|------|
| < 1000 | 原始数字：`999` |
| 1000 ~ 9999 | `1.2k` |
| ≥ 10000 | `1.2w` |

---

## 8. 架构层次

```
Handler (web/article.go)
  → Service (service/article.go + service/interactive.go)
    → InteractiveRepo (repository/interactive.go)  [Cache-Aside]
      → InteractiveCache (cache/interactive.go)     [Redis HINCRBY]
      → InteractiveDAO (dao/interactive.go)         [UPSERT]
```

### 模块职责

| 层 | 文件 | 职责 |
|----|------|------|
| Domain | `domain/interactive.go` | Interactive 领域模型 |
| DAO | `dao/interactive.go` | interactive 表 CRUD |
| Cache | `cache/interactive.go` | Redis Hash 读写 |
| Repository | `repository/interactive.go` | Cache-Aside 编排 |
| Service | `service/interactive.go` | 业务逻辑（增加阅读量、查询计数） |
| Handler | `web/article.go` | 在 reader/detail 中调用 Service |

---

## 9. 边界与约束

| 约束 | 规则 |
|------|------|
| V1 不去重 | 每次请求 +1，后续迭代加去重 |
| 不阻塞主流程 | 计数失败不影响文章详情返回 |
| 异步容错 | Redis HINCRBY 失败时降级写 DB |
| interactive 表通用 | biz+biz_id 设计，可复用于点赞/收藏 |
| 前端只读 | 前端不主动调计数接口，由 reader/detail 自动触发 |

---

## 10. 测试计划

| 类型 | 覆盖 |
|------|------|
| 集成测试 | reader/detail 请求后 read_cnt +1 |
| 集成测试 | reader/page 返回 readCnt 字段 |
| 集成测试 | 作者 page/detail 返回 readCnt |
| 集成测试 | 并发阅读计数准确性 |
| 单元测试 | InteractiveRepo Cache-Aside 逻辑 |
| 单元测试 | 数字格式化函数 |

---

## 11. 原型文件

所有原型在 `docs/read-count/read-count.pen`（Pencil 格式），PNG 导出在 `docs/read-count/prototypes/`。

| 原型 | 说明 |
|------|------|
| `AhTtg.png` | 文章广场卡片（含阅读量） |
| `ogOV9.png` | 文章阅读页头部（含阅读量） |
| `3G6L5.png` | 作者文章列表（桌面端 Table + 移动端卡片） |
