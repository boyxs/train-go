# 性能优化

对指定模块/接口做性能分析和优化。

## Step 1: 性能瓶颈定位

读取相关代码，逐层排查以下问题：

### DAO 层（`internal/repository/dao/`）
- **N+1 查询** — 循环内的数据库查询
- **缺失索引** — WHERE/ORDER BY 字段没有 `gorm:"index"`
- **全量加载** — SELECT * 加载了不需要的大字段（如 BLOB/TEXT）
- **多余查询** — UPDATE 后又 SELECT 同一条记录

### Repository 层（`internal/repository/`）
- **缓存击穿** — 热点 key 过期瞬间大量请求打到 DB
- **缓存穿透** — 查询不存在的数据反复穿透到 DB
- **缓存不一致** — 更新 DB 后缓存删除失败导致脏数据
- **缺少缓存** — 高频查询路径没有缓存
- **缺少批量查询** — 没有 `FindByIds` 导致循环单条查询

### Service 层（`internal/service/`）
- **串行操作** — 无依赖的操作没有用 `errgroup` 并行
- **重复计算** — 同一数据在一次请求中被多次计算/查询
- **阻塞操作** — CPU 密集操作（如 bcrypt）阻塞请求线程

### Web 层（`internal/web/`）
- **过度返回** — 返回了客户端不需要的字段
- **缺少分页** — 列表接口没有分页
- **中间件开销** — 全局中间件对每个请求的额外开销

### 基础设施（`ioc/`）
- **连接池配置** — MySQL/Redis 连接池大小是否合理
- **日志开销** — 是否在生产环境开启了 body 日志

## Step 2: 优化方案

对每个发现的问题，输出：

```
### 问题 N: [问题名称]
- **位置**: 文件:行号
- **现状**: 当前代码做了什么
- **影响**: 性能影响描述
- **方案**: 优化方案
- **预估效果**: 优化后预期改善
```

### 常用优化手段
- **添加索引** — GORM tag `gorm:"index"` / `gorm:"uniqueIndex"`
- **批量查询 + Map** — `WHERE id IN (?)` 一次查出，用 map 做 O(1) 查找
- **errgroup 并行** — 无依赖的查询用 `golang.org/x/sync/errgroup` 并行
- **singleflight** — 缓存击穿防护，同一 key 并发只放一个请求查 DB
- **Select 指定列** — `db.Select("id, title, status")` 不加载大字段
- **Redis Pipeline** — 多次 Redis 操作合并为一次网络往返
- **连接池调优** — `SetMaxOpenConns` / `SetMaxIdleConns` / `SetConnMaxLifetime`
- **分页/游标** — 大数据量不全量返回

## Step 3: 实施
- 等确认方案后逐项实施
- 每项优化后运行 `go test ./...` 确认无回归

## Step 4: 验证
- 对比优化前后的查询次数和数据流转方式
- 运行 `go build ./...` + `go vet ./...` + `go test ./...`
- 确认无回归问题
