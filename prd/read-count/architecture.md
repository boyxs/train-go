# 阅读量统计 — 架构设计

## 1. 模块结构

```
Handler (web/article.go)                         ← 复用现有 ReaderHandler/AuthorHandler
  │
  ├─ ArticleReaderService                        ← 注入 InteractiveService
  │     └─ Detail: 调 InteractiveService.IncrReadCnt（不阻塞主流程）
  │     └─ Page:   调 InteractiveService.GetByIds（批量查阅读量）
  │
  ├─ ArticleAuthorService                        ← 注入 InteractiveService
  │     └─ Detail: 调 InteractiveService.Get（查单篇阅读量）
  │     └─ Page:   调 InteractiveService.GetByIds（批量查阅读量）
  │
  └─ InteractiveService (service/interactive.go) ← 新增
        └─ InteractiveRepository (repository/interactive.go)  [Cache-Aside]
              ├─ InteractiveCache (cache/interactive.go)       [Redis HINCRBY]
              └─ InteractiveDAO (dao/interactive.go)           [UPSERT]
```

### 新增文件清单

| 层 | 文件 | 说明 |
|----|------|------|
| Domain | `internal/domain/interactive.go` | Interactive 领域模型 |
| DAO | `internal/repository/dao/interactive.go` | interactive 表 GORM 模型 + CRUD |
| Cache | `internal/repository/cache/interactive.go` | Redis Hash 读写 |
| Repository | `internal/repository/interactive.go` | Cache-Aside 编排 |
| Service | `internal/service/interactive.go` | 业务逻辑 |
| Consts | `internal/consts/user.go` | 新增 `InteractivePattern` 常量 |

### 修改文件清单

| 文件 | 变更 |
|------|------|
| `internal/service/article.go` | ArticleReaderService/ArticleAuthorService 注入 InteractiveService |
| `internal/web/article.go` | ArticleVO/ReaderArticleVO 增加 ReadCnt 字段，Detail/Page 返回阅读量 |
| `wire.go` | 新增 Interactive 链路 Provider |

---

## 2. 各层接口定义

### 2.1 Domain

```go
// internal/domain/interactive.go
package domain

type Interactive struct {
    BizId      int64
    Biz        string
    ReadCnt    int64
    LikeCnt    int64  // 预留
    CollectCnt int64  // 预留
}
```

### 2.2 DAO

```go
// internal/repository/dao/interactive.go
package dao

type InteractiveDAO interface {
    IncrReadCnt(ctx context.Context, biz string, bizId int64) error
    Get(ctx context.Context, biz string, bizId int64) (Interactive, error)
    GetByIds(ctx context.Context, biz string, bizIds []int64) ([]Interactive, error)
}

// GORM 模型（DAO 层不依赖 domain）
type Interactive struct {
    Id         int64  `gorm:"primaryKey,autoIncrement"`
    BizId      int64  `gorm:"uniqueIndex:uk_biz"`
    Biz        string `gorm:"type:varchar(64);uniqueIndex:uk_biz"`
    ReadCnt    int64
    LikeCnt    int64
    CollectCnt int64
    Ctime      int64
    Utime      int64
}
```

**IncrReadCnt 实现要点**：UPSERT 模式
```go
// INSERT INTO interactive (biz_id, biz, read_cnt, ctime, utime)
// VALUES (?, ?, 1, ?, ?)
// ON DUPLICATE KEY UPDATE read_cnt = read_cnt + 1, utime = ?
```

**GetByIds 实现要点**：`WHERE biz = ? AND biz_id IN (?)`，返回 slice，调用方按 bizId 索引。

### 2.3 Cache

```go
// internal/repository/cache/interactive.go
package cache

type InteractiveCache interface {
    IncrReadCntIfPresent(ctx context.Context, biz string, bizId int64) error
    Get(ctx context.Context, biz string, bizId int64) (domain.Interactive, error)
    Set(ctx context.Context, intr domain.Interactive) error
}
```

**Key 设计**：`interactive:{biz}:{bizId}`（如 `interactive:article:42`）

**IncrReadCntIfPresent**：先检查 key 是否存在，存在则 `HINCRBY`，不存在则跳过（不回源）。用 Lua 脚本保证原子性：
```lua
local key = KEYS[1]
if redis.call('EXISTS', key) == 1 then
    redis.call('HINCRBY', key, 'read_cnt', 1)
    return 1
end
return 0
```

**Get**：`HGETALL` 取出全部字段，反序列化为 `domain.Interactive`。

**Set**：`HSET` 写入全部字段 + `EXPIRE` 设置 TTL（24h + 随机抖动）。

### 2.4 Repository

```go
// internal/repository/interactive.go
package repository

type InteractiveRepository interface {
    IncrReadCnt(ctx context.Context, biz string, bizId int64) error
    Get(ctx context.Context, biz string, bizId int64) (domain.Interactive, error)
    GetByIds(ctx context.Context, biz string, bizIds []int64) ([]domain.Interactive, error)
}
```

### 2.5 Service

```go
// internal/service/interactive.go
package service

type InteractiveService interface {
    IncrReadCnt(ctx context.Context, biz string, bizId int64) error
    Get(ctx context.Context, biz string, bizId int64) (domain.Interactive, error)
    GetByIds(ctx context.Context, biz string, bizIds []int64) (map[int64]domain.Interactive, error)
}
```

**GetByIds 返回 map**：`map[bizId]Interactive`，方便 Handler 层按文章 ID 快速匹配阅读量。

---

## 3. 数据流

### 3.1 阅读计数流程（reader/detail 触发）

```
客户端 POST /article/reader/detail {id: 42}
  │
  ▼
ReaderHandler.Detail
  │
  ├─ readerSvc.Detail(ctx, 42)           ── 查文章（主流程）
  │     └─ readerRepo.FindById → DAO/Cache
  │
  ├─ go intrSvc.IncrReadCnt(ctx, "article", 42)  ── 异步计数（不阻塞）
  │     │
  │     ▼
  │   InternalInteractiveService.IncrReadCnt
  │     └─ repo.IncrReadCnt
  │           ├─ dao.IncrReadCnt   ── UPSERT: read_cnt + 1
  │           └─ cache.IncrReadCntIfPresent  ── 有缓存则 HINCRBY
  │
  ├─ intrSvc.Get(ctx, "article", 42)     ── 查当前阅读量（同步）
  │     └─ repo.Get
  │           ├─ cache.Get  ── hit → 返回
  │           └─ miss → dao.Get → cache.Set → 返回
  │
  ▼
返回 {code: 0, data: {id, title, content, ..., readCnt: 1235}}
```

**关键决策**：
- `IncrReadCnt` 使用 goroutine 异步执行，计数失败仅记日志，不影响文章返回
- `Get` 同步执行，确保返回的 readCnt 是（近似）最新值
- 因为先异步 Incr 再同步 Get，存在极端情况下 Get 拿到 Incr 前的值，可接受（最终一致）

### 3.2 列表查询流程（reader/page, article/page）

```
客户端 POST /article/reader/page {page: 1, pageSize: 10}
  │
  ▼
ReaderHandler.Page
  │
  ├─ readerSvc.Page(ctx, 1, 10)           ── 查文章列表
  │     └─ [...10 篇文章]
  │
  ├─ intrSvc.GetByIds(ctx, "article", [1,2,3,...,10])
  │     └─ repo.GetByIds
  │           └─ dao.GetByIds  ── WHERE biz='article' AND biz_id IN (1,2,...,10)
  │           （列表场景不走单条缓存，直接查 DB，避免 N 次缓存查询）
  │
  ▼
返回 {list: [{id, title, readCnt, ...}, ...], total: 100}
```

**关键决策**：
- `GetByIds` 批量查询直接走 DB，不逐条查缓存（避免 N+1 Redis 调用）
- 列表页对实时性要求低，DB 查询可接受

---

## 4. 缓存策略详细设计

### 4.1 缓存结构

| 项 | 值 |
|----|-----|
| Key | `interactive:article:{bizId}` |
| Type | Redis Hash |
| Fields | `read_cnt`, `like_cnt`, `collect_cnt` |
| TTL | 24h + rand(0, 5min) 抖动 |

### 4.2 读取策略（Cache-Aside）

```
Get(biz, bizId):
  1. cache.Get → hit → 返回
  2. miss → dao.Get → cache.Set(TTL) → 返回
  3. dao 也无数据 → 返回零值 Interactive（不缓存空值，因为 IncrReadCnt 会创建记录）
```

### 4.3 写入策略

```
IncrReadCnt(biz, bizId):
  1. dao.IncrReadCnt → UPSERT（DB 为权威数据源）
  2. cache.IncrReadCntIfPresent → 有缓存则 HINCRBY（无缓存不回源，等下次 Get 时自然加载）
  3. 任一步骤失败仅记日志，不返回错误给调用方
```

**为什么不在 cache miss 时回源**：避免写入路径上的额外 DB 查询。缓存会在读取路径自然加载。

### 4.4 Lua 脚本

```lua
-- incr_if_present.lua
-- KEYS[1] = interactive:article:{bizId}
-- ARGV[1] = field name (read_cnt)
-- ARGV[2] = delta (1)
local key = KEYS[1]
if redis.call('EXISTS', key) == 1 then
    redis.call('HINCRBY', key, ARGV[1], ARGV[2])
    return 1
end
return 0
```

用 `//go:embed incr_if_present.lua` 嵌入，与项目现有 Cache 层 Lua 脚本模式一致。

### 4.5 常量定义

```go
// internal/consts/user.go 新增
const (
    InteractivePattern = "interactive:%s:%d" // interactive:{biz}:{bizId}
)
var (
    InteractiveTTL = 24 * time.Hour
)
```

---

## 5. Wire 注入变更

### wire.go 新增 Provider

```go
func InitWebServer() *gin.Engine {
    wire.Build(
        // ... 现有 Provider ...

        // interactive 链路（新增）
        dao.NewGormInteractiveDAO,
        cache.NewRedisInteractiveCache,
        repository.NewCacheInteractiveRepository,
        service.NewInternalInteractiveService,
    )
    return gin.Default()
}
```

### 依赖关系变更

```
Before:
  ArticleReaderService ← readerRepo
  ArticleAuthorService ← authorRepo, readerRepo

After:
  ArticleReaderService ← readerRepo, InteractiveService
  ArticleAuthorService ← authorRepo, readerRepo, InteractiveService
  InteractiveService   ← InteractiveRepository
  InteractiveRepository ← InteractiveDAO, InteractiveCache
```

**构造函数变更**：

```go
// service/article.go
func NewInternalArticleReaderService(
    readerRepo repository.ArticleReaderRepository,
    intrSvc InteractiveService,                    // 新增
) ArticleReaderService

func NewInternalArticleAuthorService(
    authorRepo repository.ArticleAuthorRepository,
    readerRepo repository.ArticleReaderRepository,
    intrSvc InteractiveService,                    // 新增
) ArticleAuthorService
```

---

## 6. API 接口变更

### 6.1 reader/detail（响应增加 readCnt + 内部触发计数）

**请求不变**：
```json
POST /article/reader/detail
{ "id": 42 }
```

**响应增加 readCnt**：
```json
{
  "code": 0,
  "data": {
    "id": 42,
    "title": "文章标题",
    "content": "文章内容...",
    "abstract": "摘要",
    "authorId": 1,
    "readCnt": 1235,
    "updatedAt": "2024-01-01 12:00:00"
  }
}
```

### 6.2 reader/page（响应增加 readCnt）

**请求不变**：
```json
POST /article/reader/page
{ "page": 1, "pageSize": 10 }
```

**响应列表项增加 readCnt**：
```json
{
  "code": 0,
  "data": {
    "list": [
      {
        "id": 42,
        "title": "文章标题",
        "abstract": "摘要",
        "authorId": 1,
        "readCnt": 1235,
        "updatedAt": "2024-01-01 12:00:00"
      }
    ],
    "total": 100
  }
}
```

### 6.3 article/detail（作者视角，响应增加 readCnt）

**请求不变**：
```json
POST /article/detail
{ "id": 42 }
```

**响应增加 readCnt**：
```json
{
  "code": 0,
  "data": {
    "id": 42,
    "title": "文章标题",
    "content": "内容...",
    "readCnt": 1235,
    "status": 2,
    "createdAt": "2024-01-01T12:00:00+08:00",
    "updatedAt": "2024-01-01T12:00:00+08:00"
  }
}
```

### 6.4 article/page（作者视角，响应增加 readCnt）

**请求不变**：
```json
POST /article/page
{ "page": 1, "pageSize": 10 }
```

**响应列表项增加 readCnt**：
```json
{
  "code": 0,
  "data": {
    "list": [
      {
        "id": 42,
        "title": "文章标题",
        "status": 2,
        "readCnt": 1235,
        "updatedAt": "2024-01-01 12:00:00"
      }
    ],
    "total": 50
  }
}
```

---

## 7. 前端适配点

### 7.1 类型定义

```typescript
// api/types.ts 或对应类型文件
interface ArticleVO {
  id: number;
  title: string;
  status: number;
  readCnt: number;    // 新增
  updatedAt: string;
}

interface ReaderArticleVO {
  id: number;
  title: string;
  abstract: string;
  authorId: number;
  readCnt: number;    // 新增
  updatedAt: string;
}
```

### 7.2 数字格式化工具函数

```typescript
// utils/format.ts
function formatCount(n: number): string {
  if (n < 1000) return String(n);
  if (n < 10000) return (n / 1000).toFixed(1).replace(/\.0$/, '') + 'k';
  return (n / 10000).toFixed(1).replace(/\.0$/, '') + 'w';
}
```

### 7.3 页面改动

| 页面 | 改动 |
|------|------|
| 文章广场（Feed 列表） | 卡片底部增加阅读量：`作者 · 日期 · 👁 1.2k` |
| 文章阅读页 | 标题下方元信息增加：`👁 1234 次阅读` |
| 作者文章列表（桌面 Table） | 增加「阅读量」列 |
| 作者文章列表（移动端卡片） | 底部显示阅读量 |

### 7.4 无需新增 API 调用

前端不需要主动调用计数接口。阅读量由 `reader/detail` 自动触发，所有列表/详情接口的响应中已包含 `readCnt` 字段，前端只需读取并展示。

---

## 8. 实现命名规范总结

| 层 | 接口 | 实现 | 文件 |
|----|------|------|------|
| Domain | — | `Interactive` | `domain/interactive.go` |
| DAO | `InteractiveDAO` | `GormInteractiveDAO` | `dao/interactive.go` |
| Cache | `InteractiveCache` | `RedisInteractiveCache` | `cache/interactive.go` |
| Repository | `InteractiveRepository` | `CacheInteractiveRepository` | `repository/interactive.go` |
| Service | `InteractiveService` | `InternalInteractiveService` | `service/interactive.go` |

命名遵循项目规范：`[技术限定][实体][层]`，Interactive 模块没有业务角色区分（不像 Article 分 Author/Reader），因此省略角色段。
