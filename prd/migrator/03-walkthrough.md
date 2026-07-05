# 第一部分：30 分钟入门

> 路径：`prd/migrator/03-walkthrough.md`
> 目的：从「这是什么」到「读哪个文件 → 测试验什么 → 为什么这样设计」一站式打通
> 受众：第一次接手 migrator 代码 / 想完整理解迁移框架原理的人
> 维护：代码大改时同步本文（特别是 §6 引擎调用链 / §10 决策表）
> 代码锚点：commit `HEAD`（行号绑此版本，代码改动时同步更新）

> 📌 **v1 实现关键对齐点**（本文档以模块设计意图为主线讲解；下列实现细节以代码为准）：
> - `SourceFactory` 按读取语义分 `BuildFullSrc` / `BuildIncrSrc` / `BuildDst` 三方法（见 [`adr/0002-source-factory-three-methods.md`](./adr/0002-source-factory-three-methods.md)）
> - `BinlogClient.Subscribe(ctx, fromPos)` 签名，binlog file/pos 续订（GTID 模式属 v2 范围）
> - `BinlogEvent` / `ChangeEvent` 合并为 type alias
> - `MySQLSource` 不支持 IncrSubscribe（设计如此），cdc 任务增量阶段走 `CanalSource`
> - `ESSource` + `MongoSource` 落地（mysql/es/mongo 三种异构对账闭环；详见 §10 决策表）

---

## 1. 30 秒电梯陈词

**webook-migrator 是干嘛的？**

把任意一张 MySQL 业务表的数据 **零停机** 迁到另一处（另一台 MySQL、ES、ClickHouse、MongoDB、Kafka……），并提供完整的 **切流状态机 + 灰度 + 双向回滚 + 对账 + 审计** 控制面，业务方只需注入两个接口（`SwitchReader` / `DualWriter`）就能接入。

**和业务服务的关系**：

```
webook-core / webook-chat                    webook-migrator
─────────────────────                         ──────────────
业务请求                                       控制台 14 endpoint
   │                                                │
   ├─→ migratorsdk.DualWriter ─────╮          ┌────┴────┐
   │   按 stage 双写 OLD / NEW     │          │ 全量 +  │
   │                               │          │ 增量 +  │
   └─→ migratorsdk.SwitchReader    │          │ 对账 +  │
       按 gray% 读 OLD / NEW       │          │ 切流    │
                                   │          └────┬────┘
                                   ▼               │
                          Redis: stage/gray ◄──────┘
                          （路由决策的唯一真相源）
```

业务侧调用 SDK；SDK 读 Redis 上的 stage/gray 决定路由 OLD / NEW；migrator 服务负责后台跑全量/增量/对账 + 写 Redis 控制路由切换。**migrator 服务挂掉不影响业务**（SDK 失败降级 SideOld）。

---

## 2. 阅读顺序（按角色选）

| 角色 | 推荐顺序 | 预计 |
|------|---------|------|
| **新人 onboard** | §1 → §3 → §6.1（FullEngine 浅读）→ §10 → 跑测试 | 1h |
| **想接入业务侧 SDK** | §1 → §5（数据流-业务路径）→ `webook/internal/migratorsdk/` 4 个文件 → §6.4 SwitchService | 30m |
| **想加新 Sink（如 Mongo）** | §8 Source/Sink 抽象 → `pipeline/sink/mysql.go` → 抄一份改 | 1h |
| **想理解 partition 并行** | §6.2 IncrEngine 全文 + `service/incr/incr.go` + `incr_test.go` | 1h |
| **想理解切流状态机** | §6.4 + architecture.md §5 + `service/switching/switch.go` | 45m |
| **要做 cutover** | 跳到 [`zero-downtime-playbook.md`](./04-cutover-playbook.md)，不要读这里 | — |
| **要 oncall 处置告警** | 跳到 [`runbooks/`](./runbooks/)，不要读这里 | — |

**强烈推荐配套读**：[architecture.md](./02-architecture.md) 是权威 spec；本指南是「读它 + 读代码」的辅助。冲突以 architecture.md 为准。

---

## 3. 整体架构图

```
┌───────────────────────────────────────────────────────────────────┐
│                   webook 主仓 (github.com/webook)                  │
│                                                                    │
│  ┌─────────────────┐   ┌─────────────────┐   ┌─────────────────┐  │
│  │  webook-core    │   │  webook-chat    │   │ webook-migrator │  │
│  │  :8010          │   │  :8020          │   │ :8030           │  │
│  │                 │   │                 │   │                 │  │
│  │ ┌─────────────┐ │   │ ┌─────────────┐ │   │ ┌─────────────┐ │  │
│  │ │ DAO 层      │ │   │ │ DAO 层      │ │   │ │ 14 endpoint │ │  │
│  │ │  ↑          │ │   │ │             │ │   │ │  ↓          │ │  │
│  │ │ migratorsdk │ │   │ │ (未接入)    │ │   │ │ 5 Engine    │ │  │
│  │ │ DualWriter/ │ │   │ └─────────────┘ │   │ │ Full/Incr/  │ │  │
│  │ │ SwitchReader│ │   │                 │   │ │ Verify/     │ │  │
│  │ └──────┬──────┘ │   │                 │   │ │ Switch/SDK  │ │  │
│  │        │        │   │                 │   │ └──────┬──────┘ │  │
│  └────────┼────────┘   └─────────────────┘   └────────┼────────┘  │
│           │ R/W 决策                                    │ 控制读写  │
│           ▼                                             ▼          │
│  ┌─────────────────────────────────────────────────────────────┐  │
│  │  Redis（路由决策唯一真相源）                                │  │
│  │  migrator:stage:{taskName}            string Stage             │  │
│  │  migrator:gray:{taskName}             int 0-100                │  │
│  │  migrator:cutover_propose:{taskName}  string actor (10min TTL) │  │
│  │  migrator:throttle:{taskId}  # 仅 migrator 内部，不与 SDK 共享         qps_limit (动态调速)     │  │
│  └─────────────────────────────────────────────────────────────┘  │
│                                                                    │
│  ┌─────────────────────────────────────────────────────────────┐  │
│  │  MySQL                                                       │  │
│  │  ├─ webook         业务库（webook-core 用）                  │  │
│  │  ├─ webook_chat    聊天库（webook-chat 用）                  │  │
│  │  └─ webook_migrator 控制库（migrator 用）— 5 表               │  │
│  │     ├─ task          任务定义                       │  │
│  │     ├─ checkpoint    全量分片 / 增量 binlog 位点    │  │
│  │     ├─ validate_log  对账差异（仅记录差异）         │  │
│  │     ├─ audit_log     操作审计（合规留存 1 年）      │  │
│  │     └─ dead_letter             双写失败兜底                   │  │
│  └─────────────────────────────────────────────────────────────┘  │
└───────────────────────────────────────────────────────────────────┘
```

---

## 4. 三阶段迁移流程

把 OLD 表的数据迁到 NEW 表，分四步走，每步可独立回滚（详细见 [architecture.md §3](./02-architecture.md)）：

```
T0 ─────────────  stage = SRC_ONLY  ─────────────  gray=0
                  业务只读写 OLD，NEW 不存在
                  ▼
                  接入双写：业务接入 migratorsdk.DualWriter
                  ▼
T1 ─────────────  stage = SRC_FIRST ─────────────  gray=0
                  双写 OLD（必成）+ NEW（异步，失败入 dead_letter）
                  ▼
                  ① 启动 FullEngine：扫 OLD 全表 → 写 NEW
                  ② 启动 IncrEngine：CDC OLD binlog → 写 NEW（追平历史空窗）
                  ③ 启动 VerifyEngine：对账 OLD vs NEW，差异 → validate_log
                  ▼
                  灰度切读：gray = 10 → 30 → 50 → 100
                  ▼
T2 ─────────────  stage = SRC_FIRST ─────────────  gray=100
                  全部读 NEW；双写 OLD + NEW（NEW 也强制成功）
                  ▼
                  双人复核 cutover_propose + cutover_approve
                  ▼
T3 ─────────────  stage = DST_FIRST ─────────────  gray=100
                  全部读 NEW + 双写 NEW（必成）+ OLD（兜底）
                  30s 过渡期（避免切写瞬间空窗）
                  ▼
T4 ─────────────  stage = DST_ONLY  ─────────────  切流完成（不可逆）
                  全部读写 NEW；OLD 转只读、停止更新
                  ▼ observation 期（监控 + 对账）
T5 ─────────────  stage = DST_ONLY  ─────────────  收尾 / OLD 下线
                  观察期满，运维手动下线 OLD（v1 无 closed 状态）
                  ✅ 迁移完成，不可回滚
```

**五个保证**（缺一不可）：

| 保证 | 实现 |
|------|------|
| **幂等** | Sink `INSERT ... ON DUPLICATE KEY UPDATE` + Version 乐观锁，崩溃重启可重放不重复写 |
| **顺序** | 同 PK FNV-hash 落同 partition，单行变更必有序 |
| **一致** | 双写打平 + 全量补历史 + 增量追实时 = 三层兜底 |
| **可重放** | checkpoint 持久化（全量 PK range / 增量 binlog 位点），crash 续传 |
| **可回滚** | 双写期 gray 调 0 / rollback 回 SRC_FIRST；DST_ONLY 单写后不可逆 |

**三大坑**（详见 architecture.md §3.7，本指南 §10 给代码层面解法）：

1. 旧 binlog 覆盖新值
2. read-your-write 破坏（切读时）
3. 切写瞬间空窗

---

## 5. 数据流详解

### 5.1 业务请求路径（webook-core 写一条 article）

```
 1. HTTP POST /articles  →  articleHandler.Edit
 2. articleHandler → articleService.Save → cachedArticleRepository.Sync
 3. cachedArticleRepository.Sync 调 migratorsdk.DualWriter.Write
 4. DualWriter 读 Redis migrator:stage:article_v1 → 决定写策略：
    ├─ SRC_ONLY            → 只调 oldDAO.Sync（NEW 不存在）
    ├─ SRC_FIRST           → 先 oldDAO（必成）；newDAO 异步重试（失败入 dead_letter）
    ├─ DST_FIRST (30s 过渡)→ 严格双写：oldDAO + newDAO 都必成
    └─ DST_ONLY            → 只调 newDAO.Sync
 5. DualWriter 返 nil → service 层继续 → handler 响应 200
```

**关键点**：
- SDK 不调 migrator gRPC，只读 Redis；migrator 服务挂掉 SDK 仍可用（降级 SideOld）
- DualWriter.fn 是业务方提供的写函数，SDK 只决定调几次、哪侧
- 失败兜底：DST_ONLY 单写失败 → 业务 error 上抛；双写场景失败 → FailureRecorder 入 dead_letter

### 5.2 业务读路径

```
 1. HTTP GET /articles/:id  →  articleHandler.Detail
 2. articleService.GetByID → migratorsdk.SwitchReader.ChooseSide(userID)
 3. ChooseSide 读 Redis migrator:stage:* + migrator:gray:* → 返 SideOld / SideNew
    ├─ stage = SRC_FIRST   + hash(userID)%100 < gray → SideNew
    ├─ stage = SRC_FIRST   + hash(userID)%100 ≥ gray → SideOld
    ├─ stage = DST_FIRST                              → SideNew
    └─ stage = DST_ONLY                              → SideNew
 4. service 按 side 选 oldDAO.FindById 或 newDAO.FindById
```

**read-your-write 保证**：同一 userID 在同 stage+gray 下始终命中同侧（hash 路由确定性），不会「写一侧读另一侧」。

### 5.3 CDC 路径（IncrEngine 追平 OLD → NEW）

```
 1. webook-core 业务正常写 OLD（产生 binlog）
 2. CanalSource（migrator 内）订阅 OLD binlog
    └─ 实现：`GoMySQLCanalClient`（基于 go-mysql-org/go-mysql/canal）；factory 按 `task.Mode == cdc` 切到 CanalSource
 3. CanalSource 转 BinlogEvent → ChangeEvent → 推到 source2disp channel
 4. dispatcher goroutine 按 PK FNV-hash 路由到 N 个 partChans[i]
 5. N 个 worker goroutine 各自消费 partChans[i]：
    a. 攒批（默认 100 条）
    b. 攒满 / channel 关 → Sink.Apply（MySQL: INSERT ... ON DUPLICATE KEY UPDATE）
    c. Apply 成功 → updateCheckpoint(taskId, partition, lastBinlogPos)
 6. Sink 用 Version 乐观锁：col = IF(VALUES(version) > version, VALUES(col), col)
    └─ 防「旧 binlog 覆盖新值」（坑 1）
```

**partition 并行的本质**：单一订阅流 + 多 worker 并发消费 + 单行顺序保证。详见 §6.2。

---

## 6. 五大引擎

### 6.0 共同模式

所有引擎遵循 **接口 + Internal 实现 + 构造函数返接口** 的硬规则：

```go
// 接口
type FullEngine interface { Run(ctx, taskId, shards) error; Pause(taskId) error }

// 实现
type InternalFullEngine struct { taskSvc, ckptRepo, srcFactory, sinkFactory, transformReg, l, cfg ... }

// 构造函数返接口（wire 注入时只看接口签名）
func NewFullEngine(...) FullEngine { return &InternalFullEngine{...} }
```

所有引擎都用 `sync.Map` 维护 `taskId → context.CancelFunc`，`Pause(taskId)` 调对应 cancel 优雅退出。所有 worker 通过 `golang.org/x/sync/errgroup` 协调错误传播。

### 6.1 FullEngine — 全量分片并行

**职责**：把 OLD 全表 PK 范围切 N 片，并发跑 N 个 worker 扫 + 写。

**调用链**：

```
handler /tasks/:id/start {phase:"full"}
   ↓
TaskHandler.Start
   ↓
1. svc.Get(taskId) 拿 Task
2. resolveShards(ctx, task):
   ├─ task.Shards 配了 → 直接用
   └─ 没配 → srcSource.(PKRanger).PKRange + full.PlanShards 自动 16 切片
3. svc.GetThrottle(taskId)（经 ThrottleRepository→ThrottleCache）覆盖各分片 QPSLimit（动态调速持久化）
4. fullEng.Run(ctx, taskId, shards)
   ↓
FullEngine.Run:
- errgroup 起 N 个 worker，每个跑一个 ShardSpec：
  for {
    rows, err = src.FullScan(ShardSpec)  // GORM 分页 SELECT WHERE id BETWEEN ?
    if len(rows) == 0 { break }
    snk.Apply(rows)                       // INSERT ... ON DUPLICATE KEY UPDATE
    ckptRepo.Save(taskId, phase=full, shard_no, cursor=lastID)
  }
- 任一 worker err → errgroup 取消所有 worker → Run 返 err
- 所有 worker 完成 → Run 返 nil
```

**关键文件**：
- `service/full/full.go`（实现）
- `service/full/full_test.go`（12 subtest：单分片 / 多分片 / Pause / 中途 err / checkpoint 续传）

**配置入口**：
```yaml
migrator:
  full:
    batchSize: 1000     # 每批攒到多少行触发 Sink.Apply
    channelBuf: 4096    # Source → consumer 的 chan 缓冲
```

### 6.2 IncrEngine — 增量 + Partition 并行 ⭐

**职责**：订阅 binlog → PK 分发 → 并行 Sink 写 + 位点续传。

**为什么 partition**：单 worker 串行追不上业务峰值 qps；按 PK hash 分到 N 个 worker 并行消费，**保证单行顺序**（坑：t1=update→t2=delete 错位变成 delete→update 数据就错了）。

**调用链**：

```
handler /tasks/:id/start {phase:"incr"}
   ↓
TaskHandler.Start → incrEng.Run(ctx, taskId)
   ↓
IncrEngine.Run（service/incr/incr.go L103）:

[Step 1] 加载所有 partition checkpoint
   partCkpts := loadAllPartitionCheckpoints(taskId, n=PartitionCount)
   subCkpt   := minPartitionCkpt(partCkpts)  // ← 多 partition resume 关键

[Step 2] 启 subscriber goroutine
   go src.IncrSubscribe(subCkpt, source2disp)
        ↓
   CanalSource 把 binlog → ChangeEvent 推 source2disp

[Step 3] 启 dispatcher goroutine
   for change := range source2disp:
       idx := FNV(change.PK) % n
       partChans[idx] <- change

[Step 4] 启 N 个 worker（errgroup 协调）
   for i := 0..n-1:
       g.Go(func() {
           runPartition(taskId, i, startPos=partCkpts[i].CursorValue, partChans[i])
       })

[Step 5] errgroup.Wait + 读 subErrCh
```

**runPartition 内部**：

```go
batch := make([]Mutation, 0, BatchSize)
var lastPos string
flush := func() {
    snk.Apply(batch)
    if compareBinlogPos(lastPos, startPos) > 0 {  // ← ckpt 防回退
        updateCheckpointForPartition(taskId, partition, lastPos)
    }
    batch = batch[:0]
}
for change := range partChans[i]:
    batch = append(batch, changeToMutation(change))
    lastPos = change.BinlogPos
    if len(batch) >= BatchSize { flush() }
flush()  // 退出前 flush 残留
```

**两个核心正确性细节**：

### 细节 1：min-ckpt resume

多 partition 时不同 partition 的 ckpt 推进速度不同（partition 0 可能已写到 binlog pos=100，partition 5 还停在 pos=50）。crash 重启时**必须从 min(各 partition ckpt) 重订阅**，否则 slow partition 未 flush 的事件丢失：

```
crash 前：P0 ckpt=100, P5 ckpt=50
        partition 5 的 channel 里还有事件 e1(pos=60), e2(pos=70), e3(pos=80) 未 flush
crash 重启：
  ❌ 错误：从 P0 的 100 重订阅 → 60/70/80 永远不再被 dispatch → 数据丢失
  ✅ 正确：从 min(100,50)=50 重订阅 → 60/70/80 被重新 dispatch 到 P5 → 不丢
```

实现：`minPartitionCkpt(ckpts)` 选 CursorValue 最小的；空 cursor（首次 flush 的 partition）视为「比任何已写位点都早」，直接返回。

### 细节 2：ckpt 防回退

min-ckpt resume 后，fast partition（P0）会收到 [50, 100] 这段重放事件。Sink 幂等保证重放安全（Version 乐观锁过滤老事件），但 **P0 的 ckpt 不能被覆盖到 50**——否则下次 crash 又从 50 起跑，重放范围无限放大。

实现：worker 保留 `startPos`（本 partition 启动时的 ckpt 值），flush 时仅在 `compareBinlogPos(lastPos, startPos) > 0` 才写 ckpt DB。

### 细节 3：BinlogPos 比较

格式 `"file/pos"`（如 `"mysql-bin.000001/4096"`）。`compareBinlogPos`：
- 先比 file 名（字典序；MySQL binlog 文件命名 zero-padded `mysql-bin.000001` 保证字典序与数字序一致）
- 再比 pos 数字（**不能字典序**，因为 `"100" < "99"` 字典序但 100 > 99 数值序）
- malformed 输入回退字符串比较（极端兼容）

**关键文件**：
- `service/incr/incr.go`：实现（279 行）
- `service/incr/incr_test.go`：测试（11 + 4 partition resume + 4 compare + 3 minCkpt subtest）
- `pipeline/source/canal.go`：BinlogClient 抽象 + ParseBinlogPos 助手
- `pipeline/sink/mysql.go`：Version 乐观锁 SQL 生成

**配置入口**：
```yaml
migrator:
  incr:
    batchSize: 100        # 增量小批量低延迟
    channelBuf: 4096
    partitionCount: 1     # 1=单 worker（默认）；4/8/16 启用并行
```

### 6.3 VerifyEngine — 采样对账 + Repair

**职责**：扫 src + dst 同 PK，字段不一致 → 落 `validate_log`；按需 Repair（src/dst overwrite / mark_only）。

**调用链**：

```
handler /tasks/:id/verify {mode:"sample", rate:0.01}
   ↓
TaskHandler.Verify → verEng.Sample(taskId, 0.01)
   ↓
Sample:
1. srcSource.FullScan + dstSource.FullScan 并发
2. 同 PK 入采样池（按 rate 选样）
3. 字段比对（domain 层 diffAndLog）
4. 差异 → validateRepo.BatchInsert（ValidateLogRepository → validate_log，upsert 去重）

handler /tasks/:id/repair {strategy:"src_overwrite_dst", ids:[1,2,3]}
   ↓
verEng.Repair(taskId, RepairSrcOverwriteDst, ids)
   ↓
Repair:
- 拉 validate_log 中 ids 的 diff_detail（含 src snapshot）
- 按 strategy：
  ├─ src_overwrite_dst → dstSink.Apply(src_snapshot)
  ├─ dst_overwrite_src → srcSink.Apply(dst_snapshot)
  └─ mark_only         → repaired=1（不动数据）
- 更新 validate_log.repaired
```

**关键文件**：
- `service/verify/verify.go`（13 subtest）

### 6.4 SwitchService — 切流状态机

**职责**：切流四阶段推进 / 回滚 / 灰度调节；**进 DST_ONLY 强制双人复核**。

**状态机**（详见 architecture.md §5.1）：

```
SRC_ONLY → SRC_FIRST → DST_FIRST → DST_ONLY（终态，status=switched）
               ↑           │
               └───────────┘ rollback（仅双写期 → SRC_FIRST；SRC_FIRST 自身幂等）
   SRC_ONLY 未进双写、DST_ONLY 单写不可逆 → 均拒绝（ErrRollbackNotAllowed）
```

**调用链**：

```
handler /tasks/:id/switch {stage:"DST_ONLY", action:"propose|approve"}
   ↓
TaskHandler.Switch → swSvc.SetStage(taskId, "DST_ONLY", proposeActor, approveActor)
   ↓
SetStage:
1. 校验 next stage 合法（不能跳级 / 不能逆推）
2. 进 DST_ONLY 需要双人复核：
   ├─ action=propose → Redis SET migrator:cutover_propose:{taskName} = actor, TTL 10min
   └─ action=approve → 
       a. Redis GET migrator:cutover_propose:{taskName}
       b. propose == approve 同 actor → ErrApprovalSameActor (409)
       c. propose 不存在 / 已过期 → ErrProposeNotFound (412)
       d. 通过 → 真正写 Redis SET migrator:stage:{taskName} = "DST_ONLY"
3. 同步更新 task.gray_percent（task table 是冗余持久化，Redis 才是路由决策源）
```

**handler 层 audit** 配套：
- AuditMiddleware：所有写操作落 `audit_log`（合规留存 1 年）
- 写幂等不靠中间件：不可逆操作（switch / repair）走 MySQL 唯一索引 + 状态机 + `IsRunning` 兜底

**关键文件**：
- `service/switching/switch.go`（实现 19 subtest）
- `web/middleware/audit.go`（v1 实际启用的唯一写中间件）

### 6.5 业务侧 SDK（webook/internal/migratorsdk/）

**职责**：业务 DAO 接入迁移服务的最小接口；NoOp 实现零开销，启用时换 Redis 实现。

```go
// SwitchReader 读路由
type SwitchReader interface {
    ChooseSide(ctx context.Context, taskName string, hashKey int64) (Side, error)
}

// DualWriter 写策略
type DualWriter interface {
    Write(ctx context.Context, taskName string, fn func(side Side) error) error
}

// Side
const (
    SideOld Side = "old"
    SideNew Side = "new"
)
```

**接入示例**（业务 DAO 层）：

```go
type cachedArticleRepository struct {
    oldDAO ArticleAuthorDAO
    newDAO ArticleNewDAO
    sw     migratorsdk.SwitchReader
    dw     migratorsdk.DualWriter
}

func (r *cachedArticleRepository) Sync(ctx context.Context, a Article) (int64, error) {
    return r.dw.Write(ctx, "article_v1", func(side migratorsdk.Side) error {
        if side == migratorsdk.SideOld {
            return r.oldDAO.Sync(ctx, a)
        }
        return r.newDAO.Sync(ctx, a)
    })
}
```

**NoOp / Redis 切换**：`migrator.sdk.enabled: true` yaml flag 决定 wire 注入哪个实现，业务方注入 `migratorsdk.SwitchReader / DualWriter` 接口无感知具体实现。

**关键文件**：
- `webook/internal/migratorsdk/sdk.go`（接口）
- `webook/internal/migratorsdk/noop.go`（零开销默认）
- `webook/internal/migratorsdk/redis.go`（启用实现 + FailureRecorder 兜底）

---

## 7. 目录骨架（按调用顺序）

```
webook/migrator/
├── main.go                       入口：initViper + InitApp + Server.Run
├── wire.go / wire_gen.go         Wire DI（修改 wire.go 后跑 wire ./...）
├── config/{local,test}.yaml      环境配置
├── consts/{dao,auth}.go          DAO 枚举 + JWT 验签密钥
├── domain/                       Task / Checkpoint / Stage / Mode/Kind/TableMapping
├── errs/errs.go                  业务 sentinel
├── ioc/                          8 个 Provider：DB / Redis / Logger / OTel / Engines / ...
├── pipeline/                     ★ 抽象层（引擎依赖这里的接口）
│   ├── source/source.go          Source 接口 + ShardSpec + Row + ChangeEvent
│   ├── source/mysql.go           MySQLSource（全量分片 SELECT）
│   ├── source/canal.go           CanalSource（增量 binlog）+ BinlogClient
│   └── sink/sink.go              Sink 接口 + Mutation
│       sink/mysql.go             MySQLSink（INSERT ... ON DUP KEY UPDATE + 乐观锁）
├── repository/                   数据访问（域内唯一触 dao/cache 的层）
│   ├── task.go                   TaskRepository（CRUD + 灰度冗余列）
│   ├── checkpoint.go             CheckpointRepository（引擎游标持久化）
│   ├── validate_log.go           ValidateLogRepository（对账差异）
│   ├── dead_letter.go            DeadLetterRepository（死信消费侧）
│   ├── audit_log.go              AuditLogRepository（审计 append-only）
│   ├── throttle.go               ThrottleRepository（限速配置）
│   ├── switch_state.go           SwitchStateRepository（stage/gray/propose）
│   ├── cache/                    ThrottleCache + SwitchStateCache（Redis）
│   └── dao/                      5 张表 DAO + init_table
├── service/                      ★ 业务逻辑（只依赖 repository 接口）
│   ├── task.go                   TaskService（CRUD + 字段校验 + throttle 三方法）
│   ├── full/                     FullEngine
│   ├── incr/                     IncrEngine（partition 并行 + RunningTasks 监控枚举）
│   ├── verify/                   VerifyEngine（含 ListMismatch）
│   ├── replay/                   ReplayService（死信重放）
│   └── switching/                SwitchService（状态机 + 双人复核）
├── web/                          路由层
│   ├── task.go                   14 endpoint 全集
│   ├── result.go                 type Result = ginx.Result
│   └── middleware/               audit
└── integration/                  e2e 测试（基础设施不可用时 skip）
```

**推荐第一次读的顺序**：

1. `main.go`（看到 viper + App + Server）
2. `wire.go`（看到 DI 全景）
3. `domain/`（看核心实体）
4. `pipeline/source/source.go` + `pipeline/sink/sink.go`（看核心抽象）
5. `service/full/full.go`（最简单的引擎，理解 errgroup 模式）
6. `service/incr/incr.go`（partition 并行 + min ckpt resume）
7. `service/switching/switch.go`（状态机 + 双人复核）
8. `web/task.go`（看 endpoint 怎么编排引擎）
9. 各层 `*_test.go`（看测试用例理解边界）

---

## 8. Source / Sink 抽象设计

### 8.1 设计动机

迁移引擎要支持「任意源 → 任意 sink」组合（MySQL → MySQL、MySQL → ES、MySQL → ClickHouse、MySQL → Kafka……），如果引擎直接依赖具体实现就要为每种组合写一份引擎。

**解法**：抽出 `Source` / `Sink` 接口屏蔽数据源差异，引擎只依赖接口。新增 Sink 只需实现接口 + ioc 注册，引擎零改动。

### 8.2 Source 接口

```go
type Source interface {
    FullScan(ctx, shard ShardSpec, out chan<- Row) error
    IncrSubscribe(ctx, ckpt Checkpoint, out chan<- ChangeEvent) error
    SaveCheckpoint(ctx, ckpt Checkpoint) error
    Close() error
}

// 可选能力（type assertion）
type PKRanger interface { PKRange(ctx) (min, max int64, err error) }
type LagReporter interface { Lag(taskId int64) int64 }
```

**实现矩阵**：

| 实现 | FullScan | IncrSubscribe | PKRanger | LagReporter |
|------|----------|---------------|----------|-------------|
| `MySQLSource` | ✅ GORM 分页 SELECT | ❌ ErrIncrNotSupported | ✅ SELECT min/max | ❌ |
| `CanalSource` | ❌ ErrFullScanNotSupported | ✅ go-mysql canal | ❌ | ✅ now - lastEventTs |

**工厂层**：`SourceFactory` / `SinkFactory` 接口（`pipeline/source/factory.go` / `pipeline/sink/factory.go`）按 `task.Tables()` + `tableIdx` 解析表配置构造 Source/Sink。引擎按 `task.Tables()` 长度迭代每张表独立 build src/snk + 跑分片；checkpoint shard_no 编码 `tableIdx * ShardStride + shardNo` 区分多表。

**异构 Sink 分发**：`InternalSinkFactory` 持 `HeteroSinkBuilder`，按 `task.SinkType` 切到 `ESSink` / `ClickHouseSink` / `MongoSink` / `KafkaSink`（实现见 `pipeline/sink/{es,clickhouse,mongo,kafka}.go`）。

**Canal 真订阅**：`InternalSourceFactory` 持 `canalClientBuilder`，按 `task.Mode == cdc` 切到 `CanalSource`，底层 `GoMySQLCanalClient` 基于 go-mysql-org/go-mysql/canal 真订阅 binlog。

**三方法返回精确类型**（接口隔离 ISP）：调用方拿到啥就能用啥，无需试探；`PKRanger` 等可选能力才按需 type-assert：

```go
// 工厂接口签名（NewSourceFactory 返接口；具体实现 InternalSourceFactory 包私有）
// 按"读取语义 + task.SourceType/SinkType"分发，而非按 task.Mode（2026-05-25 review 后拆三方法，
// 修复 cdc 任务全量阶段误用 CanalSource 的 P0 bug）
type SourceFactory interface {
    // BuildFullSrc 全量扫描 src，按 SourceType 分发：mysql→MySQLSource / mongo→MongoSource
    BuildFullSrc(ctx context.Context, task domain.Task, tableIdx int) (FullSource, error)
    // BuildIncrSrc 增量订阅 src，按 SourceType 分发：mysql cdc→CanalSource / mongo→Change Stream
    BuildIncrSrc(ctx context.Context, task domain.Task, tableIdx int) (IncrSource, error)
    // BuildDst 对账读 dst 侧，按 SinkType 分发：mysql→MySQLSource / es→ESSource / mongo→MongoSource
    BuildDst(ctx context.Context, task domain.Task, tableIdx int) (FullSource, error)
}

// 引擎按 task.Tables() 长度迭代,每张表独立 build src/snk
for tableIdx := range tables {
    src, _ := srcFactory.BuildFullSrc(ctx, task, tableIdx)  // FullEngine 调这个
    dst, _ := srcFactory.BuildDst(ctx, task, tableIdx)      // VerifyEngine 用 BuildFullSrc + BuildDst
    // ...
}
```

### 8.3 Sink 接口

```go
type Sink interface {
    Apply(ctx, batch []Mutation) error  // 原子写入一批；必须幂等
    Close() error
}

type Mutation struct {
    Op      string // insert / update / delete
    Table   string
    PK      int64
    Cols    map[string]any
    Version int64  // 乐观锁版本（防旧 binlog 覆盖新值）
}
```

**MySQLSink 关键 SQL 模板**（启用乐观锁后）：

```sql
INSERT INTO target (id, title, content, version)
VALUES (?, ?, ?, ?), (?, ?, ?, ?), ...
ON DUPLICATE KEY UPDATE
  id      = IF(VALUES(version) > version, VALUES(id), id),
  title   = IF(VALUES(version) > version, VALUES(title), title),
  content = IF(VALUES(version) > version, VALUES(content), content),
  version = GREATEST(version, VALUES(version))
```

含义：仅当新事件的 version > 已有 version 时才覆盖；version 单调不回退。这就是解坑 1（旧 binlog 覆盖新值）的代码层面实现。

---

## 9. 控制库 5 表

每张表的角色与字段（DDL 在 `webook/migrator/scripts/migrator.sql`）：

| 表 | 用途 | 关键字段 | 谁写 | 谁读 |
|----|------|----------|------|------|
| `task` | 任务定义 | id, name, mode, kind, source_dsn_ref, sink_dsn_ref, tables_json, **status** (0-6,-1), gray_percent | handler CRUD | 所有引擎启动时 FindById |
| `checkpoint` | 续传位点 | task_id, **phase** (full/incr), **shard_no**, cursor_kind (id_range/binlog_pos/gtid), cursor_value | FullEngine / IncrEngine 各 worker | 启动时 load 续传 |
| `validate_log` | 对账差异 | task_id, table_name, biz_id, mismatch_kind (missing/extra/diff), diff_detail (JSON), repaired | VerifyEngine | Repair 时读 diff_detail |
| `audit_log` | 操作审计 | task_id, actor, action, payload, result, error_msg, client_ip | AuditMiddleware（所有写操作）| 合规 / 运维查询 |
| `dead_letter` | 双写失败兜底 | task_id, op, table_name, biz_id, payload, last_error, retry_count, replayed | FailureRecorder（业务侧）| ReplayDL endpoint |

**复合唯一索引**：
- `task.uni_task_name` — 任务名全局唯一
- `checkpoint.uk_checkpoint_task_phase_shard` — (task_id, phase, shard_no) 唯一，对应 Upsert 语义

---

## 10. 关键设计决策

| 维度 | 决策 | 理由 / 何时改 |
|------|------|---------------|
| Go module | 共享主仓 `github.com/webook`，**不独立 go.mod** | 复用 webook/pkg/；要拆独立服务再独立 module |
| 代码位置 | `webook/migrator/`（与 chat 平级，无 `internal/`） | 独立服务但同 module；`internal/` 一层留给主仓 |
| 接口命名 | 各层接口 + `Internal` 或技术前缀实现 | 主仓硬规则（webook/CLAUDE.md） |
| 响应类型 | `type Result = ginx.Result`（type alias） | 主仓硬规则；前端 / 网关统一处理 |
| Handler 签名 | `(ctx) (Result, error)` + `ginx.Wrap` 装饰 | 框架自动转 HTTP；业务代码只写业务 |
| 业务错误 | `pkg/errs.New(httpCode, msg)` 定义 sentinel | 自带 HTTP code，框架自动转 HTTP |
| Logger | `pkg/logger.LoggerX` + Field helpers | 主仓硬规则；测试用 NewNopLogger() |
| Stage 持久化 | Redis `migrator:stage:{taskName}` | **路由决策唯一真相源**；migrator 服务挂掉业务仍可路由 |
| Source/Sink wire 二实例 | `type SrcSource source.Source` named type | wire 类型区分；显式 cast 回接口使用 |
| 进 DST_ONLY 双人复核 | propose + approve 不同 actor，10min TTL | 切流是不可逆操作，双人复核降低误操作 |
| **partition 并行 resume** | `min(各 partition ckpt)` 作起点 + worker 防 ckpt 回退 | slow partition 未 flush 事件不丢；fast partition ckpt 不被重放覆盖到小值 |
| **PK 分发用 FNV-hash** | 不用裸 PK%n | 连续 PK 取模分布不均（1..100 全落 bucket 0..3） |
| **Sink Version 乐观锁** | `IF(VALUES(version) > version, ...)` + `GREATEST(version, VALUES(version))` | 解坑 1：旧 binlog 不覆盖新值 |
| **SDK 默认 NoOp 实现** | yaml flag `migrator.sdk.enabled` 切换 | 未启用迁移时零开销；启用时换 Redis 实现 |
| **SDK 不调 migrator gRPC** | 直读 Redis（stage/gray） | migrator 服务挂掉不拖业务；Redis 不可达降级 SideOld |
| **Cache-Aside 写后清缓存** | repository 层职责（不放 service 兜底）| webook/CLAUDE.md 硬规则；接口收敛减少漏写 |
| 集成测试不可用基础设施 → skip | `t.Skipf("mysql unreachable: %v", err)` | CI 无 MySQL 也能跑（自动跳）；本地起 docker 才真跑 |

---

## 11. 测试组织

按包分布与重点 case：

| 包 | subtest 数 | 重点验什么 |
|----|-----------|-----------|
| `domain` | 11 | Stage 推进合法性 / Mode/Kind 校验 / TableMapping JSON 序列化 |
| `repository` | 7 | TaskRepository CRUD 边界 |
| `repository/cache` | 6 | ThrottleCache 读写 / 坏值修复 / TTL jitter |
| `repository/dao` | 12 | 5 张表 GORM CRUD + 软删除行为 |
| `service` | 11 | TaskService 字段校验 / Stage 推进 |
| `service/full` | 12 | 单分片 / 多分片 / Pause / 中途 err / checkpoint 续传 |
| `service/incr` | **22** | 基础 7 + partition 并行 2 + **partition resume 4** + compareBinlogPos 4 + minPartitionCkpt 3 + Lag 2 |
| `service/verify` | 13 | Sample / Full / Repair（src_overwrite / dst_overwrite / mark_only） |
| `service/switching` | 19 | 状态机 4 阶段推进 + 回滚 + 双人复核（同 actor 拒 / 过期拒） |
| `web` | 32 | 14 endpoint 路由 + 参数绑定 + 错误映射 + 中间件挂载 |
| `web/middleware` | 6 | Audit 落表 / dsnRef 脱敏 / action 映射 |
| `pipeline/source` | 30 | MySQLSource 分页 / PKRange / CanalSource binlog → ChangeEvent / Lag |
| `pipeline/sink` | 7 | MySQLSink batch INSERT / 乐观锁 SQL 生成 |
| `integration` | 3 | e2e（需 mysql + redis） |

**跑全套**：
```bash
cd webook && go test ./migrator/... -count=1
```

**只跑 partition 相关**：
```bash
cd webook && go test ./migrator/service/incr/... -v -run "Partition|Compare|MinCkpt"
```

---

## 12. 常见操作的代码路径

### 12.1 创建任务

```
curl POST /migrator/tasks ─→ TaskHandler.Create ─→ TaskService.Create ─→
TaskRepository.Create ─→ GormTaskDAO.Insert
  └─ AuditMiddleware（v1 启用；落 audit_log）
```

### 12.2 启动全量

```
curl POST /tasks/:id/start {phase:"full"} ─→ TaskHandler.Start
  ├─ Get task
  ├─ resolveShards（自动 PKRange + PlanShards）
  ├─ TaskService.GetThrottle（经 ThrottleRepository）覆盖 QPSLimit
  └─ go fullEng.Run(ctx, taskId, shards)（异步执行，立即返 200）
```

### 12.3 启动增量

```
curl POST /tasks/:id/start {phase:"incr"} ─→ TaskHandler.Start
  └─ go incrEng.Run(ctx, taskId)
       ├─ loadAllPartitionCheckpoints
       ├─ minPartitionCkpt → IncrSubscribe 起点
       ├─ subscriber goroutine
       ├─ dispatcher goroutine
       └─ N worker goroutine（errgroup）
```

### 12.4 切流推进到 DST_ONLY

```
1. curl POST /tasks/:id/switch {stage:"DST_ONLY", action:"propose"} as actorA
   └─ SwitchService.SetStage → Redis SET migrator:cutover_propose:{taskName} = actorA, TTL 10min

2. curl POST /tasks/:id/switch {stage:"DST_ONLY", action:"approve"} as actorB
   └─ SwitchService.SetStage
        ├─ Redis GET migrator:cutover_propose:{taskName} = actorA
        ├─ actorA != actorB ✅
        └─ Redis SET migrator:stage:{taskName} = "DST_ONLY"

3. 同 actorA approve → 409 ErrApprovalSameActor
4. 10min 未 approve → 412 ErrProposeNotFound
```

### 12.5 业务方读路径切换 OLD → NEW

零代码改动；业务 DAO 已注入 `SwitchReader`，stage/gray 一改 Redis，下一次请求自动按新规则路由。

### 12.6 修复对账差异

```
curl POST /tasks/:id/repair {strategy:"src_overwrite_dst", ids:[1,2,3]}
  ├─ 从 validate_log 拉 diff_detail（含 src snapshot）
  ├─ dstSink.Apply(src_snapshot)
  └─ 更新 validate_log.repaired = 1
```

---

## 13. 下一步学习建议

按你的目标选：

### A. 想接业务

1. 读 §5.1 + §6.5
2. 看 `webook/internal/migratorsdk/` 4 个文件（sdk / noop / redis / FailureRecorder）
3. 看 `webook/internal/ioc/migrator_sdk.go`（yaml flag 切 NoOp / Redis）
4. 看主仓 `wire.go`（怎么注入两个接口）
5. 自己挑一张表（如 article），实现 `oldDAO` + `newDAO` + 改 Repository 接入 SDK

### B. 想加新 Sink（如 Mongo）

1. 读 §8
2. 抄 `pipeline/sink/mysql.go` 改 Mongo（实现 `Apply` + `Close`）
3. ioc 加 `InitMongoSink`
4. 写测试（参考 `mysql_test.go`）

### C. 想理解切流状态机

1. 读 architecture.md §5（权威定义）
2. 读 `service/switching/switch.go`
3. 跑 `go test ./migrator/service/switching/ -v`，看 19 个测试覆盖所有状态推进 + 拒绝场景

### D. 想理解 IncrEngine 全栈

1. 读 §6.2 全文
2. 读 `service/incr/incr.go`（279 行，注释密度高）
3. 跑 `go test ./migrator/service/incr/ -v` 看 22 个 subtest 输出
4. 模拟 crash 场景：多 partition + 不同 ckpt → resume → 验证 min(ckpt) 行为
5. 真集成：实现 `GoMySQLCanalClient` 替代 `MySQLSource`（接口已就位）

### E. 想做 cutover

**不要读这里**，直接 [zero-downtime-playbook.md](./04-cutover-playbook.md) + cutover-checklist.md。

---

## 14. 实现状态

引擎契约稳定，核心扩展点全部落地：

1. **Canal binlog 真订阅** ✅：`GoMySQLCanalClient` 基于 go-mysql-org/go-mysql/canal 实现 `BinlogClient` 接口；`SourceFactory` 按 `task.Mode == cdc` 自动切到 `CanalSource`；上线时需要 master my.cnf 配 `server-id` + `binlog_format=ROW` + `binlog_row_image=FULL`
2. **按 task 动态调度多表** ✅：`domain.Task.Tables()` / `PickTable(idx)` 暴露多张表；factory `BuildSrc/BuildDst(ctx, task, tableIdx)`；引擎按 task 内 tables 迭代；checkpoint shard_no 编码 `tableIdx * ShardStride + shardNo` 区分
3. **异构 Sink** ✅：MySQL / ES / ClickHouse / Mongo / Kafka 五套实现；按 `task.SinkType` 在 `InternalSinkFactory.heteroBuilder` 内分发
4. **认证 + 限流** ✅：JWT 装配（`server.http.jwt.disabled` 可关）+ IP 级 rate-limit；RBAC scope 中间件 v1 未挂（端点 scope 标注为设计意图，接入 SSO 后挂回，见 `02-architecture.md` §11）
5. **业务侧 SDK 接入** ✅：`internal/migratorsdk` SDK 接口 + Redis 实现；webook-core `CacheArticleReaderRepository` 已集成（Upsert/Delete 双写、FindById/FindByIds 切读、Page 不切）；yaml `migrator.sdk.enabled: false` 默认零开销

接口契约 / 状态机 / 错误码 / 表结构以 [architecture.md](./02-architecture.md) 为权威；与本指南冲突时回退到 architecture.md。

---

# 第二部分：完整学习手册

> §1-14 是「30 分钟入门」；§15-22 是「完整学习 + 实战上手」。按学习路径设计：
> A 业务全周期（§15-18）→ 知道整个迁移在做什么
> B 代码深度（§19）→ 知道每个 package 为何这样写
> C 实战手册（§20-21）→ 知道怎么动手跑、怎么处置告警
> D 深入原理（§22）→ 知道踩坑背后的工程取舍

---

## 15. 业务全周期：D-3 → D12 完整时间线

迁移是个**12 天工程**（典型规模千万级表）。每天都有明确动作 + 验收门槛 + 回滚兜底。

### 15.1 阶段总表

| 阶段 | 时间 | switch_stage | gray | 主动作 | 验收门槛 |
|------|------|-------------|------|--------|---------|
| **D-3 准备** | 3 天前 | `SRC_ONLY` | 0 | 创建 task / preflight / 起 canal-server | preflight 全绿 |
| **D-2 灰度业务** | 2 天前 | `SRC_FIRST` | 0 | 业务侧改造接入 SDK / 部署 dev 验证 | 双写无 dead_letter |
| **D-1 全量+增量** | 前一天 | `SRC_FIRST` | 0 | `POST /tasks/:id/start full+incr` → checkpoint 推进 → verify 采样 | mismatch_rate < 0.001% |
| **D0 灰度切读 10%** | 上线日 | `SRC_FIRST` | 10 | `POST /tasks/:id/gray {percent:10}` | NEW 侧 P99 < OLD * 1.5 |
| **D1-D3 灰度推进** | 1-3 日 | `SRC_FIRST` | 30 → 50 → 80 | 每升一档观察 24h + 5xx + lag | 业务无感知 |
| **D4 灰度满** | 4 日 | `SRC_FIRST` | 100 | 全部读 NEW；双写仍开 | 24h 无回滚事件 |
| **D5-D6 cutover propose** | 5-6 日 | `SRC_FIRST→DST_FIRST` | 100 | 双人复核 propose+approve → 30s 过渡双写 | propose 不同 actor |
| **D7-D13 DST_ONLY 观察期** | 7-13 日 | `DST_ONLY` | — | 监控 + 对账，确认 NEW 稳定（不可逆） | 5xx 正常 + mismatch=0 |
| **D14 收尾** | 14 日 | `DST_ONLY`（switched）| — | 运维手动归档下线 OLD（v1 无 closed 状态）| 业务读写全 NEW |

**关键 D 日**（cutover prod）：**只允许工作日 + 业务低峰 + 主备 oncall 在场**。周五 / 节前禁切。

### 15.2 阶段间「不可回滚」断点

```
SRC_ONLY ────► SRC_FIRST ────► DST_FIRST ────► DST_ONLY（终态）
                  ↑               ↑                 │
                  └───────────────┘                 │
       双写期 rollback → SRC_FIRST     ✗ DST_ONLY 单写后不可回滚（OLD 停滞）
```

**各 stage 的回滚成本**（小→大）：

| stage | 回滚动作 | 成本 |
|-------|----------|------|
| `SRC_FIRST/gray>0` | gray ← 0 | 秒级 |
| `SRC_FIRST/gray=100` | gray ← 0 | 秒级 |
| `DST_FIRST/30s` | `POST /switch {stage:SRC_FIRST,action:rollback}` | 秒级 |
| `DST_ONLY`（单写） | — | **不可回滚**（OLD 停滞，point of no return） |

### 15.3 业务侧改造时机

| 时点 | 业务动作 | 何处改 |
|------|---------|--------|
| **D-3 前** | yaml `migrator.sdk.enabled: false`（默认） | webook-core 配置不改 |
| **D-3** | DAO/Repository 改造接入 SDK（已落 article_reader） | `CacheArticleReaderRepository` |
| **D-2** | 灰度环境 `migrator.sdk.enabled: true` + 重启 | dev/staging |
| **D0** | 生产 enabled: true + 重启（业务零感知，stage=SRC_FIRST gray=0 行为同关闭） | prod yaml |
| **D14+** | 业务代码可拆 SDK（OLD 不存在了，但 SDK 仍兼容 SRC_ONLY=空，读写 OLD）| Repository |

---

## 16. 接入手册：业务侧 SDK 集成 + 本地验证

### 16.1 集成模板（30 行代码改 Repository）

参考实现：`webook/internal/repository/article_reader.go`

**Step 1**：Repository struct 加 4 字段
```go
type CacheArticleReaderRepository struct {
    oldDAO       dao.ArticleReaderDAO
    newDAO       dao.ArticleReaderDAO     // 新加
    cache        cache.ArticleCache
    switchReader migratorsdk.SwitchReader // 新加
    dualWriter   migratorsdk.DualWriter   // 新加
    taskName     string                   // 新加
    l            logger.LoggerX
}
```

**Step 2**：构造函数接收新参数
```go
func NewCacheArticleReaderRepository(
    oldDAO dao.ArticleReaderDAO,
    newDAO dao.ArticleReaderNewDAO,        // named type 避免 wire 冲突
    c cache.ArticleCache,
    sw migratorsdk.SwitchReader,
    dw migratorsdk.DualWriter,
    taskName migratorsdk.TaskName,
    l logger.LoggerX,
) ArticleReaderRepository { ... }
```

**Step 3**：写路径用 DualWriter 包裹
```go
func (r *CacheArticleReaderRepository) Upsert(ctx, article) error {
    err := r.dualWriter.Write(ctx, r.taskName, func(side migratorsdk.Side) error {
        return r.daoBySide(side).Upsert(ctx, entity)
    })
    // ...
}
```

**Step 4**：读路径用 SwitchReader 决策
```go
func (r *CacheArticleReaderRepository) FindById(ctx, id) (domain.Article, error) {
    art, err := r.cache.GetPub(ctx, id)
    if err == nil { return art, nil }
    side := r.chooseSide(ctx, id)  // 包装 sw.ChooseSide + 错误处理
    pub, err := r.daoBySide(side).FindById(ctx, id)
    // ...
}
```

**Step 5**：wire 接通
```go
// internal/wire.go migratorSDKProviderSet 加：
ioc.InitMigratorSDKSwitchReader,
ioc.InitMigratorSDKDualWriter,
ioc.InitMigratorSDKTaskName,
dao.NewGormArticleReaderNewDAO,
```

**Step 6**：yaml 默认关
```yaml
migrator:
  sdk:
    enabled: false  # ★ 默认 NoOp 零开销
    taskName: "published_article_v1"
```

### 16.2 命名约定（webook 强一致）

| 角色 | 命名 | 例 |
|------|------|----|
| OLD DAO | 现有接口 | `dao.ArticleReaderDAO` |
| NEW DAO（named type） | `[实体]New[层]` | `dao.ArticleReaderNewDAO` |
| Repository struct | `Cache[实体][层]Repository` | `CacheArticleReaderRepository` |
| Repository 字段 | `oldDAO` / `newDAO` / `switchReader` / `dualWriter` / `taskName` | （勿用 `sw`/`dw` 缩写）|
| 错误处理 helper | `chooseSide(ctx, hashKey)` | 包装 SwitchReader 错误 + 降级 SideOld |

### 16.3 不能切的路径

| 路径 | 原因 | 怎么办 |
|------|------|--------|
| **Page 分页** | 跨侧分页同 offset 看到不同列表 | 始终走 OLD（已实现） |
| **跨表 JOIN** | 同事务跨 OLD/NEW 表无法保证一致 | 业务层先单表查再 merge |
| **存储过程 / 触发器** | OLD 触发器写其他表，NEW 没有 | 必须迁完触发器再切流 |
| **业务层 SELECT FOR UPDATE 锁** | NEW 表无原锁 | cutover 前临时禁用乐观锁路径 |

### 16.4 验证手段（3 层）

| 层 | 工具 | 命令 | 验证什么 |
|----|------|------|---------|
| 单测 | gomock | `go test ./internal/repository/ -run TestCacheArticleReaderRepository_MigratorSDK` | 4 stage × 7 子测：路由分发正确 |
| 集成测试 | miniredis | `go test ./internal/repository/ -run TestCacheArticleReaderRepository_SDKIntegration` | 5 子测：真切 Redis stage → 行为符合 |
| dev e2e（全链路） | docker compose | 见 §20 | 含 migrator 服务的完整业务链路 |
| dev e2e（仅 SDK） | redis-cli 模拟 | 见 [`webook/migrator/README.md` 附录 A](../../webook/migrator/README.md) | 不起 migrator 服务，主服务 + 中间件即可 |

### 16.5 本地 SDK 自测（不依赖 migrator 服务）

> 动手清单（8 步 + 验收点）已整体迁移到 [`webook/migrator/README.md` 附录 A](../../webook/migrator/README.md)（README 定位「动手手册」，本指南是「原理 / 学习」）。
>
> **简要场景与方法**：
>
> - **场景**：接入完 SDK 想验收 / cutover 前演练 / 怀疑接错时复现
> - **方法**：只起 MySQL + Redis + webook-core，用 redis-cli `SET / DEL migrator:stage:{taskName}` + `migrator:gray:{taskName}` 直接模拟 4 个 stage（SRC_ONLY / SRC_FIRST / DST_FIRST / DST_ONLY），双侧表 SELECT 对照写路径分流、API 调用看读路径切换
> - **期望对照**（详见 README 附录 A 表格）：SRC_ONLY 只动 OLD / SRC_FIRST 双写宽松 / DST_FIRST 严格双写 / DST_ONLY 只动 NEW
> - **故障降级**：Redis 挂掉时业务 API 仍 200，行为等价 SRC_ONLY

---

## 17. cutover 实操：curl 推 stage + 双人复核 + rollback

### 17.1 一页纸命令清单

```bash
# 0. 起所有服务（参考 §20.1）
docker compose --env-file deploy/.env.dev up -d

# 1. 创建 task（D-3）
curl -X POST http://localhost:8030/migrator/tasks \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "name": "published_article_v1",
    "mode": "cdc",
    "kind": "schema",
    "sourceDsnRef": "vault:webook-mysql/published_article",
    "sinkType": "mysql",
    "sinkDsnRef": "vault:webook-mysql/published_article_v1",
    "tables": [{"src":"published_article","dst":"published_article_v1","partitionKey":"id"}]
  }'
# → { "taskId": 1 }

# 2. preflight 检查（D-3，建任务前的源检查，传 DSN + 表名，不收 taskId）
curl -X POST http://localhost:8030/migrator/preflight \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"sourceDsnRef":"vault:src","tables":["article"]}'
# → 检查 DSN 通 / 表结构对齐 / Canal 配置 / 监控就绪

# 3. 全量启动（D-1）
curl -X POST http://localhost:8030/migrator/tasks/1/start \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"phase": "full"}'
# 监控：GET /tasks/1（status=2 表示 full_done）

# 4. 增量启动（D-1，紧跟全量）
curl -X POST http://localhost:8030/migrator/tasks/1/start \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"phase": "incr"}'
# 监控：GET /tasks/1/lag（< 30000ms 即 30s）

# 5. 对账采样（D0 前）
curl -X POST http://localhost:8030/migrator/tasks/1/verify \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"mode": "sample", "sampleRate": 0.01}'
# → { "mismatch": 0 } 才推进

# 6. 灰度推进（D0-D4，每天升一档）
curl -X POST http://localhost:8030/migrator/tasks/1/gray \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"percent": 10}'
# 10 → 30 → 50 → 80 → 100

# 7. cutover propose（D5，actorA）
curl -X POST http://localhost:8030/migrator/tasks/1/switch \
  -H "Authorization: Bearer $TOKEN_A" \
  -H "X-Cutover-Approver: actorA" \
  -d '{"stage": "DST_ONLY", "action": "propose", "propose": "actorA"}'
# → { "proposed_at": "...", "ttl": "10m" }

# 8. cutover approve（D5，actorB，必须 actorA != actorB）
curl -X POST http://localhost:8030/migrator/tasks/1/switch \
  -H "Authorization: Bearer $TOKEN_B" \
  -H "X-Cutover-Approver: actorB" \
  -d '{"stage": "DST_ONLY", "action": "approve", "approve": "actorB"}'
# → { "stage": "DST_ONLY" }

# 9. 任意步 rollback（紧急）
curl -X POST http://localhost:8030/migrator/tasks/1/switch \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"stage": "SRC_FIRST", "action": "rollback"}'
# 业务读写瞬间回 OLD（gray=100 时无感）

# 10. 收尾 / OLD 下线（观察期满）
# v1 无 closed action/API：确认 NEW 稳定后由运维手动归档下线 OLD 表；task 停在 switched。
```

### 17.2 双人复核陷阱

`migrator:cutover_propose:{taskName}` Redis key TTL = **10 分钟**。超时需重新 propose：

```bash
# 如果 actorB approve 时返 412 ErrProposeNotFound：
# → propose 已过期或不存在
# → actorA 必须重新 propose
```

**同 actor propose+approve 返 409 ErrApprovalSameActor**。

### 17.3 监控仪表盘要看的指标

| 指标 | 阈值 | 含义 |
|------|------|------|
| `GET /tasks/:id/lag` dstLagMs（API） | < 30000 | binlog 追平延迟（毫秒；v1 无业务 metric，走 API） |
| `verify` mismatchCount / `/mismatch`（API） | < 0.001% | 对账差异率 |
| `GET /tasks/:id` checkpoints progress（API） | → 100 | 全量推进进度 |
| `dead_letter` 行增速（mysql） | < 1/min | 死信积累速率 |
| `webook_http_*` P99（基础设施 metric） | < OLD * 1.5 | 切读后业务 P99 |

---

## 18. OLD 下线 + 收尾

### 18.1 DST_ONLY 是 point of no return

```
T0  DST_ONLY 切完    业务全部读写 NEW，OLD 转只读、停止更新
                    一旦单写 NEW，OLD 不再有新数据 → 不可回滚
```

**所以切到 DST_ONLY 前必须充分对账**：cutover checklist 强制 `mismatch_rate < 0.001%` + `verify mode=full` 24h 内通过。双写期（SRC_FIRST / DST_FIRST）才是回滚窗口，进 DST_ONLY 即 point of no return。

### 18.2 OLD 下线（收尾）

观察期（监控 + 对账）确认 NEW 稳定后，由运维手动归档下线 OLD 表（见 §18.3 checklist）。

> v1 无 `closed` action/API、无 `task.status=closed`：任务记录停在 `switched`，OLD 下线是库侧手动操作，不经 migrator API。

### 18.3 OLD 表下线 checklist

```sql
-- 1. 验证 NEW 表行数 = 应该的总行数
SELECT COUNT(*) FROM published_article_v1;

-- 2. 备份 OLD 表（保留 30 天审计）
CREATE TABLE published_article_archive_20260601 AS SELECT * FROM published_article;

-- 3. 重命名 OLD 表（防误用）
RENAME TABLE published_article TO _deprecated_published_article;

-- 4. 应用代码删 oldDAO 字段（可选；保留也无害）
```

---

## 19. 代码深度解读（按 package 逐个攻克）

每个 package 给出：**读什么文件先 / 关键代码块 / Why 决策**。

### 19.1 `domain` — 业务实体 + 状态机

**先读**：`task.go` → `checkpoint.go` → `switch.go`

| 实体 | 字段 | Why 这样设计 |
|------|------|-------------|
| `Task` | Mode (dual_write/cdc) / Kind (cross_dc/sharding/schema/heterogeneous) / TablesJSON / Status | Mode = 迁移机制；Kind = 业务分类；解耦让一个 API 表面支持四类迁移 |
| `Task.Tables()` | 反序列化 TablesJSON → []TableMapping，归一化 PartitionKey="id" 兜底 | 多表 task：一个 task 可承载 N 张表（避免给每张表创建 task 的运维成本） |
| `Task.PickTable(idx)` | 取第 idx 张表，越界返 error | factory 按 tableIdx 动态构造 Source/Sink |
| `EncodeShardNo(tableIdx, shardNo)` | `tableIdx * ShardStride + shardNo`（ShardStride=10000） | checkpoint shard_no 编码：每张表最多 10000 shards，最多 21474 张表，覆盖业务场景 |
| `Stage` | SRC_ONLY / SRC_FIRST / DST_FIRST / DST_ONLY | 行业标准四阶段命名，与 architecture.md §5 状态机对齐 |
| `Mode` vs `Stage` | Mode 是 task 永久属性，Stage 是 task 运行时状态 | 容易混；架构 §1.2 有澄清表 |

**关键决策**：
- TablesJSON 用 JSON 字符串而非关联表 — 简化 task CRUD（一行 SELECT 拿全部），代价是查询不能按表名 filter（业务上不需要）
- ShardStride=10000 而非 1000：留余量给真大表（千万级表分 10 万 shard）

### 19.2 `consts` — 共享常量

**先读**：`dao.go`（DAO 枚举） → `auth.go`（JWT 密钥）

| 常量 | 用途 |
|------|------|
| `PhaseFull` / `PhaseIncr` | checkpoint.phase 列枚举 |
| `CursorKindIDRange` / `CursorKindBinlog` / `CursorKindGTID` | checkpoint.cursor_kind 列枚举 |
| `MismatchKindMissing` / `MismatchKindExtra` / `MismatchKindDiff` | validate_log.mismatch_kind 列枚举 |
| `DirectionSrcToDst` / `DirectionDstToSrc` | validate_log.direction 列枚举 |
| `AccessKey` | JWT 验签密钥（与 webook-core 同源） |
| `UserKey` / `UserSsidPattern` | gin context user claims key + Redis ssid 失效列表 pattern |

**关键决策**：
- JWT 密钥写死在 `consts/auth.go` 不读 yaml：与 webook-core / chat 保持一致；密钥变化是 ops 事件而非 deploy 事件
- 所有 Redis key prefix 在 service 包内（如 `switching.KeyStage`），不集中放 consts — 让"业务模块拥有自己的 keys"原则

### 19.3 `errs` — 业务错误 sentinel

**先读**：`errs.go`（全部 sentinel 一文件）

| 错误 | HTTP code | 何时返 |
|------|-----------|------|
| `ErrInvalidArgument` | 400 | 参数校验失败（用 `WithCause` 附详情）|
| `ErrTaskNotFound` | 404 | task.FindById 找不到 |
| `ErrTaskDuplicateName` | 409 | task.Insert UNIQUE INDEX 冲突 |
| `ErrInvalidGrayPercent` | 400 | gray% 不在 0-100 |
| `ErrInvalidStageTransition` | 400 | 状态机非法跳转（如 SRC_ONLY → DST_ONLY 跨级）|
| `ErrApprovalSameActor` | 409 | 双人复核 propose 和 approve 同 actor |
| `ErrProposeNotFound` | 412 | approve 时 propose 不存在或过期 |
| `ErrInvalidSampleRate` | 400 | verify rate 不在 (0, 1] |

**关键决策**：
- 用 `pkg/errs.New(httpCode, msg)` 而非 `errors.New`：自带 HTTP code，`ginx.Wrap` 框架自动转 HTTP response
- 不引入 `ErrCodeXxx string` 字符串错误码维度：webook/CLAUDE.md「Web 层 7 规则」明确禁止双契约

### 19.4 `repository` + `dao` + `cache` — 5 张表的 CRUD

**先读**：`scripts/migrator.sql`（5 表 schema） → `dao/task.go` → `repository/task.go`

```
repository/
├── task.go                      Task CRUD + ListOpts 分页 + UpdateGrayPercent
├── checkpoint.go / validate_log.go / dead_letter.go / audit_log.go
│                                各表仓储：domain↔dao 转换（toDomain/toEntity + slicex.Map）
├── throttle.go / switch_state.go
│                                Redis 态仓储（包装 cache 层）
├── cache/
│   ├── throttle.go              ThrottleCache（限速持久化）
│   └── switch_state.go          SwitchStateCache（stage/gray/propose 键）
└── dao/
    ├── task.go        task 表
    ├── checkpoint.go  checkpoint 表（FindByTaskAndPhase / Upsert）
    ├── validate_log.go validate_log 表（BatchInsert / FindByIDs / MarkRepaired）
    ├── audit_log.go   audit_log 表（Insert 异步落表）
    ├── dead_letter.go           dead_letter 表（ListUnreplayed / MarkReplayed / IncrementRetry）
    └── init_table.go            AutoMigrate 5 张表
```

**关键文件**：

| 文件 | 关键 method | Why |
|------|-------------|----|
| `dao/task.go` | `UpdateStatus(id, status int8)` / `UpdateGrayPercent(id, percent int16)` | task 状态机推进 = SQL UPDATE，单字段；不全字段覆盖避免覆盖人工 SET 字段 |
| `dao/checkpoint.go` | `Upsert(ckpt)` 用 OnConflict | (task_id, phase, shard_no) UNIQUE INDEX → 同 partition 重复写自动 update |
| `dao/validate_log.go` | `BatchInsert(logs)` | 千条差异同事务写，减少 round-trip |
| `cache/throttle.go` | `Set` 无 TTL | 限速配置一直生效到下次 Clear（无 TTL，不自动过期） |

### 19.5 `service.task` + `service.switching` — 业务编排

**先读**：`task.go`（CRUD） → `switching/switch.go`（状态机推进）

**`TaskService`**（CRUD + 字段校验）：
- `Create(req)` 校验 + tablesJSON marshal + 写 task
- `Get(id)` Repository.FindById → domain.Task
- `List(opts)` 按 status filter + offset/limit

**`SwitchService`**（切流状态机）：
- `SetGray(taskId, percent)` Redis SET + task.gray_percent 冗余同步
- `SetStage(taskId, next, propose, approve)` 进 DST_ONLY 强制双人复核（Redis 临时 key propose_actor 10min TTL）
- `Rollback(taskId)` 双写期（SRC_FIRST/DST_FIRST）→ SRC_FIRST（SRC_FIRST 幂等）；DST_ONLY/SRC_ONLY 拒绝

**关键决策**：
- Stage 存 Redis 而非 MySQL：路由决策路径每次读 Redis（毫秒级 RTT）；MySQL 只作冗余持久化
- task.gray_percent 同步到 task 表是为「task 列表页直接显示 gray%」，但路由决策仍读 Redis（避免数据库压力）
- 双人复核 propose TTL 10 分钟：避免 propose 后跑去吃饭，approve 过来已经"过期"

### 19.6 `service.full` + `service.incr` + `service.verify` — 三大引擎

**先读**：§6 各引擎调用链已覆盖；这里补函数级索引（按函数名 grep，行号会随代码演进漂移）

| 引擎 | 函数 | 关键代码块 |
|------|-----|----------|
| FullEngine | `(*InternalFullEngine).Run` | 按 `task.Tables()` 迭代每张表 + errgroup |
| FullEngine | `(*InternalFullEngine).resolveShards` | PKRanger 自动切片 16 片（被 Run 调用）|
| FullEngine | `(*InternalFullEngine).runShard` | 攒批 + Apply + checkpoint 闭环 |
| IncrEngine | `(*InternalIncrEngine).Run` | 多表迭代 → 调 runTable |
| IncrEngine | `(*InternalIncrEngine).runTable` | dispatcher + N workers errgroup |
| IncrEngine | `(*InternalIncrEngine).runPartition` | `startPos` 参数防 ckpt 回退 |
| IncrEngine | `(*InternalIncrEngine).loadAllPartitionCheckpoints` | min-ckpt resume 关键 |
| IncrEngine | `minPartitionCkpt` / `compareBinlogPos` | 比较 file/pos 字符串选最小 |
| VerifyEngine | `(*InternalVerifyEngine).Sample` | 多表迭代 → 调 sampleTable |
| VerifyEngine | `(*InternalVerifyEngine).sampleTable` | 单表采样：并行 FullScan + diff |
| VerifyEngine | `(*InternalVerifyEngine).repairAcrossTables` | 按 `validate_log.biz_table` 路由到对应表 sink |

用 `grep -n "^func.*<funcName>" service/<pkg>/<file>.go` 拿当前行号。

### 19.7 `pipeline.source` — 读端抽象 + 5 实现

**先读**：`source.go`（接口） → `mysql.go`（MySQLSource） → `canal.go`（CanalSource 接口）→ `canal_client.go`（GoMySQLCanalClient SDK 集成）→ `factory.go`（工厂分发）

**接口契约**（`source.go`）：
- `Source.FullScan(ctx, shard, out)` / `IncrSubscribe(ctx, ckpt, out)` / `SaveCheckpoint` / `Close`
- 可选 `PKRanger.PKRange(ctx)` / `LagReporter.Lag(taskId)` — 通过 type assertion 检测

**实现矩阵**：

| 实现 | FullScan | IncrSubscribe | 何时用 |
|------|----------|---------------|--------|
| `MySQLSource` | ✅ GORM 分页 SELECT | ❌ ErrIncrNotSupported | task.Mode != cdc |
| `CanalSource` | ❌ ErrFullScanNotSupported | ✅ go-mysql canal | task.Mode == cdc |

**Canal 真订阅**（`canal_client.go`）：
- `GoMySQLCanalClient` 实现 `BinlogClient`，包装 `canal.Canal` SDK
- 持久态：`canalSrv` / `out` chan / `stopOnce` / `stopped` chan
- `Subscribe` 启动 RunFrom goroutine + ctx-done 监听 goroutine
- `canalEventHandler.OnRow` RowsEvent → BinlogEvent（update 类型 e.Rows 是 [before,after,before,after,...]）
- `pkColumnIndex` / `rowToMap` / `toInt64Loose` helper 处理 PK 提取 + 列映射

**Why 私有 helper**：
- `parseBinlogPosStr` / `pkColumnIndex` 等不导出 — 只在 canal SDK 集成上下文有意义；导出会污染包 API

### 19.8 `pipeline.sink` — 写端抽象 + 5 实现

**先读**：`sink.go`（接口） → `mysql.go`（含 Version 乐观锁 SQL 生成） → `es.go` / `clickhouse.go` / `mongo.go` / `kafka.go`

**异构 Sink 设计对照**：

| Sink | 写策略 | 乐观锁 |
|------|--------|--------|
| `MySQLSink` | `INSERT ... ON DUPLICATE KEY UPDATE`，单事务 upsert+delete | `col = IF(VALUES(version) > version, VALUES(col), col)` |
| `ESSink` | bulk API（index + delete actions） | `version_type=external` + `version=Mutation.Version` |
| `ClickHouseSink` | insert 走 ReplacingMergeTree(version) | 表 ENGINE 层去重（应用层不做） |
| `MongoSink` | ReplaceOne with upsert + delete | filter `{$or:[{version:{$lt:new}},{version:{$exists:false}}]}` |
| `KafkaSink` | SyncProducer SendMessages，key=PK | 下游消费者自己处理（key 同 partition 保单行顺序） |

**关键 SQL 模板**（MySQLSink）：

```sql
-- 启用乐观锁条件：batch 中所有行都含 version 列
INSERT INTO target (id, title, content, version)
VALUES (?, ?, ?, ?), ...
ON DUPLICATE KEY UPDATE
  title   = IF(VALUES(version) > version, VALUES(title), title),
  content = IF(VALUES(version) > version, VALUES(content), content),
  version = GREATEST(version, VALUES(version))
```

**Why GREATEST**：保证 version 单调不回退 — 即使 batch 内有重复 PK，最终 version = max(...)。

### 19.9 `web` + `middleware` — 14 endpoint + 中间件链

**先读**：`web/task.go`（14 endpoint） → `web/result.go`（Result alias） → `web/middleware/audit.go`

**14 endpoint**（`web/task.go:81-99` RegisterRoutes）：

```
CRUD:
  POST   /migrator/tasks                  Create  (ScopeWrite)
  GET    /migrator/tasks                  List    (ScopeRead)
  GET    /migrator/tasks/:id              Get     (ScopeRead)

Lifecycle:
  POST   /migrator/preflight              Preflight (ScopeWrite)
  POST   /migrator/tasks/:id/start        Start     (ScopeWrite)
  POST   /migrator/tasks/:id/pause        Pause     (ScopeWrite)
  POST   /migrator/tasks/:id/throttle     Throttle  (ScopeWrite)
  POST   /migrator/tasks/:id/gray         SetGray   (ScopeSwitch)
  POST   /migrator/tasks/:id/switch       SetSwitch (ScopeSwitch)
  GET    /migrator/tasks/:id/lag          Lag       (ScopeRead)
  POST   /migrator/tasks/:id/verify       Verify    (ScopeWrite)
  GET    /migrator/tasks/:id/mismatch     Mismatch  (ScopeRead)
  POST   /migrator/tasks/:id/repair       Repair    (ScopeRepair)
  POST   /migrator/tasks/:id/replay-dl    ReplayDL  (ScopeRepair)
```

**Middleware 链顺序**（`ioc/web.go:65-105`）：

```
metrics → otelgin → cors → jwt → ratelimit → accesslog → audit
```

- **metrics**：Prometheus（最外层，覆盖所有路径含 cors preflight）
- **otelgin**：trace 注入（紧跟 metrics，配 ContextWithFallback=true）
- **cors**：跨域（jwt 之前，OPTIONS 预检不带 token）
- **jwt**：认证（yaml `server.http.jwt.disabled: true` 跳过用于本地）
- **ratelimit**：IP 级 sliding-window，yaml 可覆盖（默认 100 req/s/IP）
- **accesslog**：访问日志（记录原始请求）
- **audit**：操作落 audit_log（异步，不阻塞业务路径）

**RBAC scope**（设计意图，v1 未挂中间件强制）：
- 端点标注的 `ScopeRead` / `ScopeWrite` / `ScopeSwitch` / `ScopeRepair` 是权限设计（见 `02-architecture.md` §11）
- v1 走运维侧账号权限管控；接入 webook-core SSO 签发链路后再挂 scope 校验中间件

### 19.10 `ioc` + `wire` — DI 全景

**先读**：`wire.go`（一个文件全部 Provider Set） → `wire_gen.go`（自动生成，不手编） → `ioc/*.go`（各 Provider）

**Provider Set 分组**：

| Set / 函数 | Provider | 提供什么 |
|----------|----------|---------|
| 基础设施 | InitDB / InitRedis / InitLogger / InitOTel / InitTimezone | *gorm.DB / redis.Cmdable / LoggerX / TracerProvider |
| pipeline | InitSourceFactory / InitSinkFactory | source.SourceFactory / sink.SinkFactory（含 Canal builder + 异构 builder） |
| Repository | NewTaskRepository / NewCheckpointRepository / NewValidateLogRepository / NewDeadLetterRepository / NewAuditLogRepository / NewThrottleRepository / NewSwitchStateRepository | 各仓储接口（service 层唯一数据入口） |
| Service | NewTaskService / NewSwitchService / replay.NewReplayService | service.TaskService / SwitchService / ReplayService |
| 五大引擎 | InitFullEngine / InitIncrEngine / InitVerifyEngine | 引擎接口（构造时持 factory，Run 时按 task 动态 build） |
| 监控 | InitMigrationMetrics | *MigrationMetricsCollector（webook_migration_* 业务指标，scrape 时实采） |
| Web | NewTaskHandler / InitWebServer / InitMiddlewares | TaskHandler / *gin.Engine / middleware 链 |

**关键决策**：
- **Wire 时不静态注入 Source/Sink**：handler/引擎只持 factory；Run 时按 task.id 调 factory.BuildSrc/BuildDst — 支持多 task 不同表
- **InitMiddlewares 接收 logger + cmd + 各 builder**：所有 middleware 在一处装配，便于看顺序

---

## 20. 实战手册：docker-compose + curl + Grafana

### 20.1 本地起完整环境

```bash
# 仓库根 deploy/docker-compose.yaml 起所有
cd C:/Go/work
docker compose --env-file deploy/.env.dev -f deploy/docker-compose.yaml up -d

# 验证
docker compose -f deploy/docker-compose.yaml ps
# 应看到：mysql / redis / etcd / otel-collector / webook-core / webook-migrator / nginx / prometheus / grafana
```

容器服务清单（按 nginx upstream 路由）：

| 服务 | 端口 | 路由前缀 |
|------|------|---------|
| webook-core | :8010 | `/api/*`（业务） |
| webook-chat | :8020 | `/chat/*` |
| webook-migrator | :8030 | `/migrator/*` |
| nginx | :80 | 反代上面三个 |
| Grafana | :3000 | 监控面板 |
| Prometheus | :9090 | 抓取 metrics |

### 20.2 跑端到端迁移演练（本地）

```bash
# 0. 准备数据
mysql -h localhost -u root -p13520 webook < webook/scripts/webook.sql

# 1. 设默认 JWT 签发（本地跳 JWT 校验，yaml server.http.jwt.disabled=true）
TOKEN=$(curl -s -X POST http://localhost:8010/users/login \
  -d '{"email":"test@test.com","password":"test"}' \
  | jq -r '.token')

# 2. 跑 §17.1 全流程
# ... curl 命令序列

# 3. 查 article_v1 表数据
mysql -h localhost -u root -p13520 webook -e "SELECT COUNT(*) FROM published_article_v1"
# 应 ≈ published_article 的行数

# 4. Grafana 看监控
open http://localhost:3000  # admin/admin
# Dashboard: webook-migrator overview
```

### 20.3 用 PostgREST 风格批量调

```bash
# 同时启动 full + incr（任意顺序，引擎独立）
for phase in full incr; do
  curl -X POST http://localhost:8030/migrator/tasks/1/start \
    -d "{\"phase\":\"$phase\"}"
done

# 等到 verify_mismatch_rate < 0.001%
while true; do
  RATE=$(curl -s http://localhost:8030/migrator/tasks/1/mismatch | jq '.rate')
  echo "mismatch_rate=$RATE"
  [ "$(echo "$RATE < 0.00001" | bc -l)" = "1" ] && break
  sleep 30
done

# 自动灰度推进
for p in 10 30 50 80 100; do
  curl -X POST http://localhost:8030/migrator/tasks/1/gray -d "{\"percent\":$p}"
  sleep 600  # 10 min 观察期
done
```

---

## 21. oncall 排查：告警 → runbook → 处置

### 21.1 告警与 runbook 对照

| 告警 | runbook | 立即动作 |
|------|---------|---------|
| `MigratorIncrLagSpike` | `runbooks/lag-spike.md` | check Canal 状态 → 调 partition + qps |
| `MigratorVerifyMismatchHigh` | `runbooks/mismatch-spike.md` | 停止灰度推进 + 手动 repair |
| `MigratorDeadLetterGrowing` | `runbooks/dead-letter-growing.md` | 检查 NEW 侧可用性 + replay-dl |
| `MigratorServiceDown` | `runbooks/migrator-service-down.md` | 重启 + 看 etcd 配置 |
| `MigratorCanalFailure` | `runbooks/canal-failure.md` | 检查 MySQL binlog + canal server |
| `MigratorKafkaBrokerDown` | `runbooks/kafka-broker-down.md` | Kafka Sink 场景：切到 SyncProducer mode |
| `MigratorSinkUnreachable` | `runbooks/sink-unreachable.md` | NEW Sink 不可达 |
| `MigratorSourcePressureHigh` | `runbooks/source-pressure-high.md` | OLD 源库压力高，降 qps |
| `CutoverPreconditionFailed` | `runbooks/cutover-rollback.md` | propose 前条件不满足 → 修复条件再 propose |

### 21.2 SOP：lag spike 处置

```bash
# 1. 看当前 lag
curl http://localhost:8030/migrator/tasks/1/lag

# 2. 看 Canal 状态
curl http://localhost:8030/health
# 看 binlog client 是否 connected

# 3. 临时降业务峰值 qps（如能）
# 或：调 partition 并行度
# 编辑 yaml migrator.incr.partitionCount: 4 → 8
# kubectl rollout restart migrator

# 4. 实在追不上 → rollback 灰度
curl -X POST http://localhost:8030/migrator/tasks/1/gray -d '{"percent":0}'
# 业务全部读 OLD，争取时间分析
```

### 21.3 SOP：mismatch spike 处置

```bash
# 1. 拉差异列表
curl http://localhost:8030/migrator/tasks/1/mismatch?limit=20 \
  | jq '.list[]'

# 2. 看差异类型分布
curl http://localhost:8030/migrator/tasks/1/mismatch \
  | jq '[.list[] | .mismatch_kind] | group_by(.) | map({kind:.[0], count:length})'
# 大量 "diff" → 双写时序问题（Version 乐观锁该启用了）
# 大量 "missing" → 全量未完成或丢数据
# 大量 "extra" → 反向 / 历史脏数据

# 3. Repair 修复
curl -X POST http://localhost:8030/migrator/tasks/1/repair \
  -d '{"strategy":"src_overwrite_dst","ids":[1,2,3]}'

# 4. 大量差异（>1000）→ 别 repair，先停灰度，找原因
```

---

## 22. 深入原理：三大坑 + partition 状态机 + Sink 乐观锁

### 22.1 三大坑详解（含 SQL + 时序图）

### 坑 1：旧 binlog 覆盖新值

**场景**（时序）：

```
t=1000ms  业务：UPDATE OLD.article SET title='A' WHERE id=1  ✅
t=1100ms  业务：UPDATE NEW.article SET title='B' WHERE id=1  ✅ NEW 现在是 B
t=1500ms  CDC：消费到 OLD 的 t=1000 binlog → INSERT INTO NEW ... ON DUP UPDATE title='A'
          ❌ NEW.title 回退到 A
```

**根因**：双写时业务先写 OLD 再写 NEW（确保 OLD 必成）。CDC 异步消费 OLD binlog，可能延迟到达，把 NEW 已经写入的更新值覆盖。

**Sink 乐观锁解法**（MySQLSink）：
```sql
UPDATE NEW.article
SET title = IF(VALUES(version) > version, VALUES(title), title),
    version = GREATEST(version, VALUES(version))
WHERE id = ?
```

- Mutation.Version = binlog 事件的 EventTs（毫秒戳）
- 当 incoming.version < existing.version 时，UPDATE 条件不满足 → 不写
- 必须**所有列都包**乐观锁条件，否则 partial update（一致性破坏）

### 坑 2：read-your-write 破坏（切读时）

**场景**：

```
T=0   user.42 写 OLD.article (id=100, title='hello')
T=1   user.42 GET /articles/100 → 切到 NEW（gray=50%）
T=2   NEW.article 还没有 id=100（CDC 还没消费完）→ 返 404 或旧值
```

**根因**：切读 gray% 随机分流时，可能"写 OLD 后下次读切到 NEW"。

**hash(user_id) % 100 < gray% 解法**：
- 同一 user 的所有请求始终命中同一侧（路由确定性）
- user.42 hash=15 → 15 < 50% → 一直走 NEW；但他写 OLD 时也走 NEW → 不可能"写 OLD 读 NEW"
- 关键代码：`migratorsdk/redis.go:67` `hashMod100(hashKey)` FNV-hash

### 坑 3：切写瞬间的写入空窗

**场景**：

```
t=0    SRC_FIRST：业务 写 OLD + 异步写 NEW
t=10s  bash: SET migrator:stage = DST_FIRST
t=10s.1ms  业务 A 写 OLD（用旧 stage）→ NEW 没写
t=10s.2ms  业务 B 写 NEW（用新 stage）→ OLD 没写
         ❌ 两条记录分散在两个表，互相不可见
```

**30s 双写过渡解法**（DST_FIRST 阶段）：
- DST_FIRST 期间 strict 双写：OLD + NEW 都必成
- 30s 后切 DST_ONLY，业务全部走 NEW
- 30s 内空窗段，OLD + NEW 都有写入 → 切完后通过 verify 对账兜底

### 22.2 partition 并行 min-ckpt resume 状态机

**完整状态机**（IncrEngine 内部）：

```
启动 Run
   │
   ├─► load 所有 partition ckpt
   │      partCkpts[0] = "f/100"   ◄── partition 0 上次 flush 到 pos=100
   │      partCkpts[1] = "f/50"    ◄── partition 1 上次 flush 到 pos=50
   │      partCkpts[2] = ""        ◄── partition 2 首次启动（无 ckpt）
   │
   ├─► subCkpt = min(...)
   │      因为 partition 2 是空 cursor → 选 partition 2 的 ""
   │      → IncrSubscribe 从最早起点
   │
   ├─► 起 errgroup
   │     subscriber goroutine 推 events 到 source2disp chan
   │     dispatcher goroutine 按 FNV(PK) hash 路由到 partChans[i]
   │     worker[0..n-1] goroutines 各自消费 partChans[i]
   │
   └─► 每个 worker 内部状态机：
          startPos = partCkpts[i].CursorValue   ◄── 启动时本 partition 的位点
          lastPos  = ""                          ◄── 当前 batch 的最后位点
          
          for change in ch:
              batch.append(change)
              lastPos = change.BinlogPos
              if len(batch) >= BatchSize:
                  flush()
          flush()  // 退出前最后一刷
          
          flush() 函数:
              Sink.Apply(batch)
              if lastPos > startPos:           ◄── 关键：防 ckpt 回退
                  updateCheckpoint(lastPos)
              batch = []
```

**关键不变量**：

1. **不同 partition 的 ckpt 独立**：partition 0 ckpt 不影响 partition 1 ckpt
2. **min(各 partition ckpt)** 作 resume 起点：确保最慢的 partition 也不丢事件
3. **fast partition 重放安全**：subscriber 从 min 起重订，fast partition 收到 `[min, fast_pos]` 区间重放事件 → Sink 幂等 + Version 乐观锁过滤
4. **ckpt 不回退**：`lastPos > startPos` 才写 — 否则 fast partition 的 ckpt 会被重放事件覆盖到小值

**反例分析**（如果不做 min-ckpt resume）：

```
crash 前：
  partition 0 ckpt = 100, partition 5 ckpt = 50

如果 resume 用 partition 0 的 ckpt = 100：
  IncrSubscribe 从 100 起
  partition 5 在 50-100 之间的事件 → 丢了
  ❌ 数据不一致
```

### 22.3 Sink 乐观锁 SQL 模板（自动启用条件）

`pipeline/sink/mysql.go` 内：

```go
// 触发条件：batch 中所有行都含 "version" 列
func enableOptimisticLock(batch []Mutation) bool {
    for _, m := range batch {
        if _, ok := m.Cols["version"]; !ok {
            return false  // 任何一行缺 version → 关
        }
    }
    return true
}
```

**为什么必须全有**：若部分行有 version 部分没有，VALUES(version) 在没有 version 列的行变成 NULL，IF 条件结果不可预测（NULL > 任何值 = NULL），可能不写也可能写。统一的写策略只能是「全有 → 启用」或「全无 → 禁用」。

**生成的 SQL 模板**（已启用）：

```sql
INSERT INTO `target` (`id`, `title`, `content`, `version`)
VALUES (?, ?, ?, ?), (?, ?, ?, ?), ...
ON DUPLICATE KEY UPDATE
  `id`      = IF(VALUES(`version`) > `version`, VALUES(`id`), `id`),
  `title`   = IF(VALUES(`version`) > `version`, VALUES(`title`), `title`),
  `content` = IF(VALUES(`version`) > `version`, VALUES(`content`), `content`),
  `version` = GREATEST(`version`, VALUES(`version`))
```

**未启用**（兼容旧业务无 version 列）：

```sql
INSERT INTO `target` (`id`, `title`, `content`)
VALUES (?, ?, ?), ...
ON DUPLICATE KEY UPDATE
  `id`    = VALUES(`id`),
  `title` = VALUES(`title`),
  `content` = VALUES(`content`)
```

---

## 学完了，可以做什么

读完本指南，你应该能独立完成：

1. **接入业务**：照 §16 模板把任何 Repository 加 SDK（30 行代码）
2. **跑迁移**：照 §17 curl 命令推 stage 全流程
3. **应急处置**：oncall 告警来时 §21 对照 runbook 知道立即动作
4. **改源代码**：要加新 Sink / 新 Source 直接看 §19 找模板（如新加 Doris Sink 抄 ESSink 改 50 行）
5. **诊断 bug**：partition / cutover / 乐观锁 / read-your-write 三大坑遇到能定位（§22）

不会的去 [architecture.md](./02-architecture.md)（权威 spec） + 直接读代码（本指南有 file:line 索引）。

---

## 23. 自己动手：最小复现骨架

> 抓核心，丢掉工程化加料。让你知道哪些是必要复杂性、哪些是生产工程化加料。

### 最小骨架（一个迷你 migrator，500 行内）

```go
// main.go
func main() {
    runFull(srcDB, dstDB, "users")           // 全量
    runIncr(srcDB, dstDB, loadCkpt())        // 增量永远循环
}

// full.go: 按 PK range 分页 SELECT + 幂等 INSERT
func runFull(src, dst *sql.DB, table string) {
    for cursor := 0; ; cursor += 1000 {
        rows := query(src, "SELECT * FROM "+table+" WHERE id > ? LIMIT 1000", cursor)
        if len(rows) == 0 { return }
        exec(dst, "INSERT INTO "+table+" VALUES (...) ON DUPLICATE KEY UPDATE ...", rows)
        saveCkpt(cursor)
    }
}

// incr.go: canal 订阅 binlog + 幂等写
func runIncr(src, dst *sql.DB, startPos string) {
    canal.RunFrom(startPos, func(e RowsEvent) {
        exec(dst, "INSERT ... ON DUPLICATE KEY UPDATE", e.Rows)
        saveCkpt(e.Pos)
    })
}
```

### 必要复杂性 vs 工程化加料

| 必须有（删了功能跑不通） | 可以省（删了功能仍跑，只是不健壮 / 不并行 / 不可切流） |
|------|------|
| `ON DUPLICATE KEY UPDATE`（幂等写） | partition 并行 |
| checkpoint 持久化 + 续传 | min-ckpt resume（单 partition 不需要） |
| binlog pos 排序（顺序保证） | Version 乐观锁（不双写就没"旧 binlog 覆盖新值"问题） |
| 全量 + 增量串行 | 双写 SDK / 状态机 / 双人复核 / 对账 / dead_letter |

### 自己实现的 5 步

1. `main.go`：起 full + incr goroutine（错误处理用 panic 简化）
2. `full.go`：分页 SELECT + 幂等 INSERT
3. `incr.go`：canal 订阅 + 幂等写
4. `ckpt.go`：KV 存 (full_cursor, binlog_pos)
5. 跑测试：源表插数据 → 看目标表是否同步

完成最小版后再逐项加"加料"，每加一项理解一次"为什么"。这是从复现到吃透原理最有效的路径。
