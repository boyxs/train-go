# redislock 详细设计：安全可靠的自研分布式锁库

> 状态：**详细设计 v7，已确认**（architect 产出；本文档为新会话实施的唯一依据，力求自包含）
> 实现进度（见 CHANGELOG 2026-07-08）：**P1 单机/集群自研核心 + P2 fencing + P3 可重入（`WithReentrant`）+ P4 阻塞增强（pub/sub 唤醒 + 公平锁 `WithFair`）已落地**；P5（多主 quorum + 部署重构）待按消费者需求推进。
> 范围：`webook/pkg/redislockx` → 重命名并重设计为 `webook/pkg/redislock`
> 定位：**纯自研**，全量能力（P1–P5），**单机 + 集群 + 多主 quorum 三拓扑**
> 命名：接口 `Client` / `RedisLock`（自研、领域名，无外部产品术语）；参数语义借鉴成熟分布式锁（`waitTime`/`leaseTime`），但**全部经 Options 交付**
> 当前唯一消费者：worker cron leader 选举（`pkg/cronx` → `Client.TryLock`）

---

## 0. 两条"安全"真相（决定全套取舍，务必先读）

1. **Fencing token 是唯一真安全。** 挡不住的时序：持有者 GC/STW 暂停 → 锁 TTL 过期被别人拿走 → 暂停结束仍以为持锁去写 → 双写。唯一解 = **单调令牌 + 资源侧校验**（资源记已见最大 token，拒绝更小者）。它是**协议**、需被保护资源配合，不是锁内部一个开关。
2. **多主 quorum / 集群解决"可用性"，不解决"安全性"。** 抗节点故障靠多数派 / 集群故障转移；安全仍靠 fencing 补。二者搭配才成立。

→ 落地顺序：**fencing 优先级最高（P2），多节点拓扑最后（P5，绑部署重构）**。

## 0.5 交接 / 当前工作区状态（新会话必读）

- **旧包 `pkg/redislockx/` 有未提交改动**（上一会话产出）：① P0 watchdog 修复（续约遇**网络错误**且距上次成功 ≥ ttl 时视同丢锁 → 触发 `OnLost` 并退出，杜绝幻觉持锁 + goroutine 泄漏）；② §8 命名对齐（`redisClient/redisLock/metricsClient/observedLock` → 导出 `RedisClient/RedisLock/MetricsClient/ObservedLock`，`safeOnLost/safeOnRefresh` → `fireOnLost/fireOnRefresh`）。
- **本重设计会 supersede 旧包**（改名 + 重写核心）。**P0 watchdog 逻辑必须原样搬进新 `lock.go`**（见 Phase 1 任务 2 / §3.2）。
- **建议起步**：先把上述 ①② 作为一个 checkpoint commit 到旧 `redislockx`（保住 P0 修复历史），再从 Phase 1 起做重写式重设计（新包 `redislock` + 改名）。也可直接在重写里吸收，但独立 commit 历史更干净。
- **从哪开始**：Phase 1 任务 1（`script.go` + `consts.go`）。建议先落 **P1+P2** 上线（真安全 + 去 bsm + 双拓扑 + cron 零回归），P3-P5 按后续消费者需求逐段推。

---

## 1. 需求摘要 + 决策

**一句话**：把 `pkg/redislockx` 重命名为 `pkg/redislock` 并重设计为**安全、可靠、纯自研**的通用锁库——补齐可重入 / 公平锁 / pub-sub 阻塞 / 多主 quorum / fencing 五能力，支持单机与 Redis 集群两种部署，沿用本仓 `Client.TryLock/Lock → 句柄` 获取模型；worker cron 行为零回归即基线达成，各能力单测/集成测通过即完成。

**Goals**
- 沿用本仓 `Client.TryLock/Lock → RedisLock` 获取模型；句柄接口领域名 `RedisLock`（非裸 `Lock`）
- 参数语义（`waitTime`/`leaseTime`）**经 Options 交付**，签名保持 `(ctx, key, opts ...Options)` 干净
- 五能力各自独立开关；单机/集群同一实现（`UniversalClient` + hash tag），多主 quorum 独立工厂
- 每能力有明确**正确性契约 + 失败语义 + 可观测指标**

**Non-Goals（KISS）**：读写锁 / 信号量 / CountDownLatch / 联锁家族（无消费者）；异步/Reactive API；跨语言互通。

**ADR**
- **ADR-1 自研 Lua 核心、移除 bsm/redislock**：可重入(hash)/pub-sub/公平/fencing 都是 bsm 做不到的。移除 bsm 后 `redislock` 包名腾出，故改名 `redislockx`→`redislock`。
- **ADR-2 重入身份显式传 `ownerID`**：Go 无稳定 goroutine id，不 hack；默认句柄自带随机 ownerToken，`WithReentrant(ownerID)` 显式共享。
- **ADR-3 存储统一 hash 模型**：`hash{ownerToken: 重入计数}`，非重入即单 field 计数恒 1。
- **ADR-4 获取模型沿用本仓、句柄领域名**：`Client.TryLock`(软失败返 bool)/`Client.Lock`(阻塞至 ctx)，返回 `RedisLock`；不引入两级 `getLock`/`Mutex`。
- **ADR-5 三拓扑两实现**：单机/集群共用一份 Lua（都是 `redis.UniversalClient`），靠 key hash tag 同槽；多主 quorum 独立工厂 `NewQuorumClient`。
- **ADR-6 参数经 Options、watchdog 由 leaseTime 决定**：`WithLeaseTime` 未设 → watchdog 模式（租约 = `WithWatchdogTimeout`，默认 30s，每 /3 续约）；设了 `WithLeaseTime(d>0)` → 固定租约 d、无 watchdog。`WithWaitTime` 控 `TryLock` 阻塞上限。

---

## 2. 接口与参数（契约先行）

### 2.1 三种拓扑

| 拓扑 | 构造 | 底层类型 | 说明 |
|------|------|---------|------|
| **单机** | `NewClient(uc)` | `*redis.Client` | 现状；开发/中小规模 |
| **集群** | `NewClient(uc)` | `*redis.ClusterClient` | 原生分片+故障转移；**同一构造**，hash tag 适配 |
| **多主 quorum** | `NewQuorumClient([]uc, ...)` | N 个独立 `UniversalClient` | 抗单主故障；每节点可单机可集群 |

`uc redis.UniversalClient`：`*redis.Client`/`*redis.ClusterClient`/`*redis.Ring` 均实现，含 `Eval` + `Subscribe`（pub-sub 阻塞需要）。单机↔集群切换对调用方透明，只换注入类型。

### 2.2 Redis 数据模型（key = `redislock:` 前缀 + `{k}` hash tag，`{k}` 为调用方 key）

| key | 类型 | 用途 | 生命周期 |
|------|------|------|-----|
| `redislock:{k}:lock` | hash `{ownerToken: 重入计数}` | 锁主体 | 租约到期 / watchdog 续约 |
| `redislock:{k}:fence` | string（`INCR`） | 单调 fencing 计数器 | **持久/超长**（过期→单调断裂，见风险） |
| `redislock:{k}:queue` | list（ownerToken 有序） | 公平锁 FIFO 队列 | 随锁 |
| `redislock:{k}:qts` | zset（ownerToken→deadline ms） | 公平锁死等待者逐出 | 随锁 |
| `redislock:{k}:ch` | pub/sub channel | 释放通知唤醒等待者 | N/A |

`{k}` 花括号是 Redis Cluster hash tag：一把锁的全部 key 落同一 slot（化解多 key Lua 的 `CROSSSLOT`）。示例：调用方 key `cronx:lock:ranking` → `redislock:{cronx:lock:ranking}:lock` 等。key Pattern 集中 `consts.go`，时间统一 int64 毫秒。

### 2.3 Go 接口契约

```go
// redislock.go —— 只放接口 + 共享值类型（禁放实现）
type Client interface {
    // TryLock 软获取：拿到返回 (lock, true, nil)；被占返回 (nil, false, nil)（非 error）。
    // WithWaitTime>0 时最多阻塞该时长再放弃。
    TryLock(ctx context.Context, key string, opts ...Options) (RedisLock, bool, error)
    // Lock 阻塞获取：ctx 即等待上限，拿到或 ctx.Done。失败返 error。
    Lock(ctx context.Context, key string, opts ...Options) (RedisLock, error)
}

type RedisLock interface {
    Key() string
    Token() string                                          // 本句柄 ownerToken
    Unlock(ctx context.Context) error                       // 重入减计数，归零才真释放 + 通知等待者
    ForceUnlock(ctx context.Context) (bool, error)          // 强制删锁，不校验持有者（管理/兜底用）
    Refresh(ctx context.Context) error                      // 手动续约一个租约周期（watchdog 之外）
    // 状态查询
    IsLocked(ctx context.Context) (bool, error)             // 锁是否被任何人持有
    IsHeldByMe(ctx context.Context) (bool, error)           // 是否被本句柄 ownerToken 持有
    HoldCount(ctx context.Context) (int, error)             // 本句柄重入深度
    TTL(ctx context.Context) (time.Duration, error)         // 剩余租约（0=已释放/过期）
    // fencing
    Fence() int64                                           // 本次持有的 fencing 令牌（WithFencing 时 >0，否则 0）
}

var ErrLockNotHeld = errors.New("redislock: lock not held")  // Unlock/Refresh 时锁不在自己手里
```

工厂 `redislock.Client`、句柄 `redislock.RedisLock`；方法 `TryLock/Lock/Unlock` 与句柄类型 `RedisLock` 不同词，无歧义。

### 2.4 Options（全部参数经此交付；`options.go`，类型名 `Options`）

| Option | 作用 | 默认 | 适用 |
|--------|------|------|------|
| `WithWaitTime(d)` | `TryLock` 拿不到时最多阻塞 d 再返回 false | `0`（立即返回） | TryLock（Lock 用 ctx） |
| `WithLeaseTime(d)` | 固定租约 d、**关闭 watchdog** | 未设 → watchdog 模式 | 两者 |
| `WithWatchdogTimeout(d)` | watchdog 模式的租约（续约每 d/3） | `30s` | watchdog 模式 |
| `WithReentrant(ownerID)` | 显式 ownerToken，跨 goroutine 共享同一持有者 | 句柄随机 token | 两者 |
| `WithFair()` | 公平排队获取（FIFO） | 关（抢占式） | 两者 |
| `WithFencing()` | 启用 fencing，`Fence()` 返回单调令牌 | 关（`Fence()`=0） | 两者 |
| `WithRetryInterval(d)` | pub/sub 之外的兜底轮询间隔 | `100ms` | 阻塞路径 |
| `WithOnLost(fn)` | watchdog 续约失败（丢锁）回调 | nil | watchdog |
| `WithOnRefresh(fn)` | watchdog 每次成功续约回调 | nil | watchdog |

多主专属（`NewQuorumClient(nodes, ...QuorumOptions)` 构造入参）：`WithQuorumTimeout(d)`（单节点获取超时，默认 200ms）、`WithClockDrift(d)`（有效期扣减余量，默认 leaseMs*0.01+2ms）。

### 2.5 参数语义矩阵（watchdog vs 固定租约 × 阻塞 vs 非阻塞）

| `WithLeaseTime` | `WithWaitTime` / 方法 | 行为 |
|---|---|---|
| 未设 | 未设 + `TryLock` | 立即尝试；拿到则 watchdog（30s 租约，10s 续约） |
| 未设 | `5s` + `TryLock` | 最多等 5s；拿到则 watchdog |
| 未设 | `Lock` | 阻塞至 ctx；拿到则 watchdog |
| `30s` | 未设 + `TryLock` | 立即尝试；拿到则固定 30s 租约、无 watchdog、到期自动释放 |
| `30s` | `Lock` | 阻塞至 ctx；拿到则固定 30s 租约、无 watchdog |

### 2.6 用法示例

```go
cli := redislock.NewClient(rdb)                       // rdb: *redis.Client 或 *redis.ClusterClient

// cron leader 选举（watchdog，非阻塞）—— 等价 worker 现状
lk, ok, err := cli.TryLock(ctx, "cronx:lock:ranking", redislock.WithWatchdogTimeout(30*time.Second))
if err != nil { /* Redis 抖动，调用方降级 */ }
if !ok { return }                                     // 别的副本在跑，本轮跳过
defer lk.Unlock(context.Background())

// 短临界区，固定租约无 watchdog
lk, ok, _ := cli.TryLock(ctx, "k", redislock.WithLeaseTime(3*time.Second))

// 阻塞获取（ctx 为等待上限）+ 公平
lk, err := cli.Lock(ctx, "k", redislock.WithFair())

// 安全写：fencing
lk, ok, _ := cli.TryLock(ctx, "k", redislock.WithFencing())
_ = repo.WriteWithFence(ctx, data, lk.Fence())        // 资源侧按 fence 校验（§3.3）

// 多主
qcli := redislock.NewQuorumClient([]redis.UniversalClient{r1, r2, r3})
lk, ok, _ := qcli.TryLock(ctx, "k")
```

---

## 3. 能力详细设计

### 3.1 获取 / 释放 / 续约（统一 hash + 重入；核心 Lua）

**获取**（返回 -1=成功/重入；>=0=被占，值为剩余 pttl 作等待提示）：
```lua
-- KEYS[1]=redislock:{k}:lock  ARGV[1]=leaseMs  ARGV[2]=ownerToken
if redis.call('exists', KEYS[1]) == 0
   or redis.call('hexists', KEYS[1], ARGV[2]) == 1 then
    redis.call('hincrby', KEYS[1], ARGV[2], 1)          -- 首次=1 / 重入 +1
    redis.call('pexpire', KEYS[1], ARGV[1])
    return -1
end
return redis.call('pttl', KEYS[1])
```

**释放**（-1=不在我手里；0=重入未归零仍持有；1=完全释放）：
```lua
-- KEYS[1]=redislock:{k}:lock  KEYS[2]=redislock:{k}:ch
-- ARGV[1]=leaseMs  ARGV[2]=ownerToken  ARGV[3]=unlockMsg
if redis.call('hexists', KEYS[1], ARGV[2]) == 0 then return -1 end
if redis.call('hincrby', KEYS[1], ARGV[2], -1) > 0 then
    redis.call('pexpire', KEYS[1], ARGV[1]); return 0
end
redis.call('del', KEYS[1])
redis.call('publish', KEYS[2], ARGV[3])                 -- 唤醒等待者
return 1
```

**续约**（1=成功；0=不在我手里 → `ErrLockNotHeld` / watchdog 视同丢锁）：
```lua
-- KEYS[1]=redislock:{k}:lock  ARGV[1]=leaseMs  ARGV[2]=ownerToken
if redis.call('hexists', KEYS[1], ARGV[2]) == 1 then
    redis.call('pexpire', KEYS[1], ARGV[1]); return 1
end
return 0
```

**查询**：`IsLocked`=`EXISTS lock`；`HoldCount`/`IsHeldByMe`=`HGET lock ownerToken`（false→0）；`TTL`=`PTTL lock`。

失败语义：`TryLock` 被占→`(nil,false,nil)`；网络错误→`(nil,false,err)`。`Unlock`/`Refresh` 返 `ErrLockNotHeld`（token 不匹配/已过期）。

### 3.2 Watchdog（沿用本仓 + P0 修复，务必搬进新 `lock.go`）

- `WithLeaseTime` 未设时启用：Redis 租约 = `watchdogTimeout`（默认 30s），后台 goroutine 每 `watchdogTimeout/3` 跑续约脚本（§3.1）。
- `sync.Mutex`（`innerMu`）串行化 inner Redis 句柄的跨 goroutine 访问；`stop chan` + `sync.Once` 停 watchdog（Unlock 先停再释放）。
- 续约三分支（**P0 修复，必须保留**）：成功 → `fireOnRefresh`；token 不匹配（返回 0）→ `fireOnLost` + 退出（干净丢锁）；**网络错误 → 静默重试，但距上次成功 ≥ 租约时长时视同丢锁 → `fireOnLost` + 退出**（杜绝幻觉持锁与 goroutine 泄漏）。
- 回调 `fireOnLost/fireOnRefresh` 包 `recover`，回调 panic 不拖崩 watchdog。pkg 层不依赖项目 logger，可观测靠回调 + prometheus 装饰器。

### 3.3 Fencing（唯一真安全；P2）

- 启用 `WithFencing()`：**全新获取**（非重入）成功时 `INCR redislock:{k}:fence`，值缓存到句柄，`Fence()` 返回；重入不 bump（沿用）。
- **资源侧契约（安全的落点，必须让消费者遵守）**：
  ```
  fence := lk.Fence()                         // 单调递增 int64
  // 写被保护资源时带上 fence，资源侧持久化 last_fence 并校验：
  //   DB 条件写: UPDATE res SET ..., fence=:f WHERE id=:id AND fence < :f   (影响行数=0 → 拒绝)
  //   应用层:    if incoming.fence <= stored.last_fence { reject }
  ```
- 未接资源侧校验 = 没上安全锁，只是"大概率互斥"（cron 幂等重算属可接受的 best-effort）。P2 交付 `fence.go` 提供 DB 条件写 helper 范例 + 文档明示消费者二选一。
- **多主/集群耦合**：`INCR` 必须单一权威源（多主/多分片各自 INCR 不单调）→ fencing 计数器固定落权威节点（多主用 `nodes[0]`）或外部 DB sequence；多主可用性不覆盖 fencing 源（固有短板，文档标注）。

### 3.4 pub/sub 阻塞（`Client.Lock` 与 `TryLock` 的 WaitTime 实现；P4）

流程：**先** `SUBSCRIBE redislock:{k}:ch`（订阅先于试获取，堵住"释放信号在订阅前发出"的丢失窗口）→ 试获取 → 拿到则退订返回；否则 `select { <-释放消息 / <-time.After(min(pttl, RetryInterval)) 兜底 / <-ctx.Done() }` 后重试。退出路径必退订，防连接/goroutine 泄漏。集群：7.0+ 用 sharded `SSUBSCRIBE`（channel 与锁同 tag、同 slot），旧版退化广播 `SUBSCRIBE`。

### 3.5 公平锁（`WithFair()`；P4，最复杂、正确性风险最高）

数据：`queue`（list，ownerToken 入队顺序）+ `qts`（zset，ownerToken→放弃 deadline）。**获取 Lua**：① 从队头清理 `now > deadline` 的死等待者（出 queue + zts）；② 锁空闲且（队空或队头==我）→ 获取（hincrby+pexpire）+ 出队，返回 -1；③ 否则入队尾（`RPUSH` + `zadd` deadline=now+waitTime+余量）返回等待时长。**释放**：减计数，归零则 del lock + 唤醒新队头 channel。与 §3.4 pub/sub 协作：等待者被唤醒后重试，只有队头能成功。**风险**：清理与获取的原子性、deadline 漂移——单独阶段 + 真 Redis 多等待者 FIFO / 死等待者逐出集成测。

### 3.6 多主 quorum（`NewQuorumClient`；P5，绑部署重构）

获取：记 `start` → 对 N 个独立节点逐个用 §3.1 脚本获取（每个带 `QuorumTimeout`）→ 成功数 `succ`；`validity = leaseMs - (now-start) - clockDrift`；若 `succ >= N/2+1 && validity > 0` → 持有（有效租约取 validity）；否则**对所有节点执行释放**、返回 `ErrNotObtained`。续约/释放对全节点执行，按多数派判定成败。节点须互相独立（非同集群主从）。watchdog 在多数派节点上续约。

### 3.7 单机 / 集群适配

统一 `redis.UniversalClient`；key 全程 hash tag 同槽（§2.2）化解 `CROSSSLOT`；pub/sub 按版本/类型选 sharded/广播（§3.4）。切换拓扑只换注入的 client，库代码零改动。

---

## 4. 风险

- **并发/正确性**：自研 Lua 必须全原子；公平锁"清理+获取"竞态最险 → 真 Redis 集成测。
- **集群**：多 key Lua `CROSSSLOT`（hash tag 化解）；pub/sub sharded vs 广播；`ClusterClient.Eval` 按首 key slot 路由，tag 不一致即错。
- **安全**：fencing 仅资源侧校验后生效；无 fencing 多主仍不安全；fence key 过期→单调断裂（持久/超长 TTL）。
- **性能**：公平锁 acquire 带清理 O(超时者)；pub/sub 订阅生命周期不当→泄漏；fencing 每次全新获取多一次 `INCR`。
- **可用性/运维**：多主需 ≥3 套独立 Redis，当前仅 1 → 重大基础设施变更 + playbook。
- **回归/改名**：`redislockx`→`redislock` + 句柄 `Lock`→`RedisLock` + 获取从 `ttl` 位置参数→Options，波及 import/调用点（worker、cronx、integration、mocks、`mk/mock.mk`）；移除 bsm 波及现有测试 + `pkg/go.mod`；string→hash 存储格式变更（锁短暂、低活跃窗口部署可接受）。
- **Go 特有**：无 goroutine id，重入身份显式（ADR-2）。

---

## 5. 任务拆分（分阶段，可独立验收/上线）

**Phase 1 — 改名 + 自研核心 + Options 参数 + 单机/集群（基线）**
| # | 任务 | 依赖 | 验收 |
|---|------|------|------|
| 1 | 建 `pkg/redislock`；`script.go`(embed 获取/释放/续约 Lua) + ownerToken 生成 + `consts.go`(`redislock:{k}` hash-tag Pattern) | 无 | Lua 单测(miniredis) |
| 2 | 接口 `Client`/`RedisLock` + 单节点实现（§3.1）+ watchdog（§3.2，**搬入 P0 修复**） | 1 | 单测全绿 |
| 3 | Options 全套（§2.4）+ 参数语义（§2.5：waitTime/leaseTime/watchdog）+ 查询方法 + `ForceUnlock` | 2 | 单测覆盖参数矩阵 |
| 4 | 集群支持：`UniversalClient` 化 + 真 ClusterClient 集成测（多 key 同槽） | 2 | 集群集成测绿 |
| 5 | 移除 bsm + 全仓改名（`redislockx`→`redislock`、`Lock`→`RedisLock`、cronx 调用点见 §D） | 3 | `make verify`(GOWORK=off) + cron 集成测零回归 |

**Phase 2 — Fencing（真安全）**：6 `WithFencing()`+`Fence()`(§3.3)〔验收：跨获取单调〕；7 资源侧契约文档 + `fence.go` DB 条件写 helper 范例〔文档+编译〕。

**Phase 3 — 可重入**：8 `WithReentrant(ownerID)` 跨 goroutine 共享 + `HoldCount`〔同 owner 重入 N 次/释放 N 次才真释放/跨 owner 互斥〕。

**Phase 4 — 阻塞增强**：9 `Client.Lock`+WaitTime 走 pub/sub(§3.4) + 订阅生命周期〔释放后近即时拿到、无泄漏〕；10 公平锁 `WithFair()`(§3.5)**（大）**〔多等待者 FIFO、死等待者逐出〕。

**Phase 5 — 多主 quorum（绑部署）**：11 `NewQuorumClient`(§3.6)**（大）**〔3 节点、挂 1 可用、挂 2 失败〕；12 fencing 单一权威源接入多主〔多主下 fence 单调〕；13 部署 ≥3 Redis(compose/prod)+prometheus+告警+playbook〔对照部署铁律 14 项〕。

**贯穿**：14 prometheus 指标扩展——现有 `webook_lock_{acquire_total,held_seconds,wait_seconds,watchdog_lost_total}` + 新增 `fence_issued_total`/`reentrant_depth`/`fair_queue_wait_seconds`/`quorum_fail_total`〔/metrics 可见〕；15 benchmark + 真实压测（§7）——每能力落地即补对应 `Benchmark*`，`loadtest` 子包 + `TestLoad` 入口跑真 Redis〔互斥/公平/单调不变量 0 违规 + QPS/分位有数〕。

---

## 6. 测试策略

- **单测（miniredis，无外部依赖）**：Lua 逻辑（获取/重入/释放归零/续约/查询）；参数矩阵（§2.5 各组合）；watchdog 三分支（用 `WithOnLost/OnRefresh` 探针 + 关 miniredis 造网络错误验 P0）；fencing 单调。**一个源文件测试集中一个 `_test.go`**（项目铁律 §9）。
- **集成测（真 Redis，`internal/integration`）**：100 goroutine 抢锁只 1 赢；跨 Client 互斥；watchdog 保活（wall-clock）；公平 FIFO；多主 3 节点容错。
- **集群测（真 ClusterClient）**：多 key 同槽不报 `CROSSSLOT`；sharded pub/sub。
- **`-race`**：并发路径必跑（本机 MinGW 缺失时以 CI 为准）。
- **benchmark + 压测**：见 §7——微基准 `Benchmark*` 量化开销/收益 + `loadtest` 真实并发 harness 校验互斥/公平/单调不变量（`MutexViolations` 必须 0）。
- **mock**：接口改后 `make -f mk/mock.mk mockgen` 重生成，勿手改 `mocks/`。

---

## 7. Benchmark + 真实压测（验证"真实有用"，非"跑通即止"）

> 目标不是通过测试，而是**量化每项能力的收益与代价**，并在真实 Redis/集群/多主上、真实并发下验证正确性（互斥不能靠"看起来对"）。

### 7.1 Go Benchmark（微基准 `bench_test.go`）

| Benchmark | 验证什么 |
|-----------|---------|
| `BenchmarkTryLock_Uncontended` | 无竞争 获取+释放 吞吐（基线） |
| `BenchmarkTryLock_Contended/{2,8,32,128}` | N goroutine 抢同 key 的吞吐与失败率 |
| `BenchmarkReentrant` | 重入 N 层开销 vs 非重入 |
| `BenchmarkWatchdogOverhead` | 持锁期 watchdog 后台开销（开 vs 关） |
| `BenchmarkBlockingLock_PubSubVsPoll` | **pub/sub 阻塞 vs 轮询获取延迟**（证明 §3.4 收益） |
| `BenchmarkFencingOverhead` | `WithFencing` 额外 `INCR` 开销 |
| `BenchmarkQuorum3` | 多主获取延迟 vs 单机（量化可用性代价） |

`b.ReportMetric` 报 p99 等自定义指标；单机 / 集群各跑一遍对比。

### 7.2 真实压测 harness（`pkg/redislock/loadtest` 子包，持久留用、可复跑，非一次性脚本）

```go
type Config struct {
    Concurrency int             // 并发 goroutine
    Duration    time.Duration
    KeyCount    int             // 竞争 key 数（1 = 最强竞争）
    Mode        string          // single / cluster / quorum
    WaitTime    time.Duration   // TryLock 等待上限
    LeaseTime   time.Duration   // 0 = watchdog
    Fair, Fencing bool
}
type Report struct {
    Acquired, Busy, Errors int64
    QPS            float64
    P50, P90, P99  time.Duration  // 获取延迟分位
    MutexViolations      int64    // ★ 同一 key 同时 >1 持有者的次数 —— 必须 = 0（真互斥的铁证）
    FIFOViolations       int64    // 公平模式：乱序次数，应 = 0
    FenceMonotonicBreaks int64    // fencing：令牌非单调次数，应 = 0
}
func (c Config) Run(ctx context.Context, cli Client) (Report, error)
```

- **互斥不变量校验**：每 key 一个共享计数器，acquire 后原子 +1、临界区内断言其值必须 ≤ 1、release 前 -1；一旦 > 1 → `MutexViolations++`。真实并发下这是"只有一个持有者"最直接的证据（比断言 Redis 状态更贴近业务真相）。
- **入口**：① `loadtest_test.go` 的 `TestLoad`（`go test -run TestLoad -args -conc=64 -dur=30s -mode=single`，连 `config/test.yaml` 真 Redis，可作可选 CI job）；② 可选 `cmd/redislock-loadtest` 独立二进制，压 dev/staging Redis。
- 任一不变量违规 > 0 → **FAIL**；正常则打印 QPS / 分位 / 各计数。

### 7.3 "真实有用"判据（验收基线）

- **正确性**：各模式 `MutexViolations = 0`；公平模式 `FIFOViolations = 0`；fencing `FenceMonotonicBreaks = 0`。
- **收益量化**：pub/sub 阻塞获取 p99 显著低于同条件轮询；集群相对单机吞吐提升有数。
- **代价量化**：fencing / quorum / watchdog 开销落到具体数字、写进文档，供消费者取舍。

---

## A. 前瞻性设计（全量前瞻）

| 维度 | 问题 | 方案 |
|------|------|---------|
| 扩展性 | 第二个业务方接入改动多大？ | 面向能力：ownerToken/key 通用、能力由 Options 组合；新消费者只组合 Options，不动核心 Lua/存储。单机↔集群换 client 类型即可。 |
| 可用性 | 依赖挂了主流程能跑吗？ | 单机不可达→返 error 由调用方降级（参考 migrator SDK 降级 SideOld）；集群故障转移；多主多数派容错；watchdog 网络错误超租约视同丢锁(P0)。 |
| 容错性 | 重复/并发/极端输入安全吗？ | 重入 `HINCRBY` 原子；fencing 单调防重放/防暂停误写；公平队列超时逐出防死等；watchdog 续约幂等；释放校验 ownerToken 防误删他人锁。 |
| 可观测性 | 出问题 5 分钟能定位吗？ | 结构化指标(见任务 14)；pkg 靠回调 + prometheus 装饰器；关键失败经 `WithOnLost` 上抛。 |

## B. 分层与文件（文件组织铁律：契约与实现分文件、多实现各占一文件）

```
pkg/redislock/
  redislock.go   # 接口 Client / RedisLock + ErrLockNotHeld + 共享值类型（禁放实现）
  factory.go     # NewClient(uc) / NewQuorumClient([]uc, ...QuorumOptions)
  client.go      # 单个 client 的 Client 实现（单机/集群共用 UniversalClient + hash-tag key；与 quorum.go 多 client 区分）
  quorum.go      # 多主 quorum Client（多数派编排）
  lock.go        # RedisLock 句柄实现 + watchdog（P0 迁移；TryLock/Lock 返回它）
  fair.go        # 公平锁：acquireFair + dequeueFair（FIFO 队列 + 超时逐出，Lua 在 scripts/）
  pubsub.go      # 阻塞获取订阅生命周期（当前广播 SUBSCRIBE；集群 sharded SSUBSCRIBE 待后续）
  fence.go       # fencing 生成 + 资源侧校验 helper
  script.go      # Lua 集中（//go:embed）
  options.go     # Options（functional options）+ WithReentrant/WithFair/…
  consts.go      # redislock:{k} hash-tag key Pattern
  prometheus/    # 指标装饰器（builder 链式，扩展新指标）
  loadtest/      # 真实压测：loadtest.go（Go 并发 harness）+ cmd/loadserver（JMeter HTTP 壳）+ jmeter/
```
> 落地偏离：**重入未单独成 `reentrant.go`**——`WithReentrant(ownerId)` 在 options.go，重入计数天然是 acquire/fair Lua 的 `hincrby`，无独立 Go 逻辑可封装。`quorum.go`（多主）随 P5。
命名：接口 `Client`/`RedisLock`；工厂实现 `RedisClient`(单机/集群)/`QuorumClient`(多主)；装饰器 `MetricsClient`/`ObservedLock`（实现 `RedisLock`）；构造函数返回接口；receiver 首字母小写；**所有 struct 导出**（§8）。

## C. DI / wire + 配置

- `worker/ioc/lock.go`：import → `pkg/redislock`，类型引用 → `redislock.RedisLock`；`InitLockClient` 入参 `redis.Cmdable` → `redis.UniversalClient`；按 `data.redislock.mode` 选 `NewClient`(single/cluster) / `NewQuorumClient`(quorum)。`cd worker && wire ./...` 重生成。
- **配置**（配置铁律：5 份 yaml 同构、snake_case、消费点就地读）：`data.redislock` 段——`mode`(single/cluster/quorum)、`nodes`(cluster/quorum 地址表)、`watchdog_timeout`、`fencing`(bool)、`fair`(bool)、`clock_drift`、`quorum_timeout`。密码走 `${REDIS_PASS}`。
- prometheus builder 扩展新指标；多主阶段按"服务拆分 14 项"同步 `deploy/prometheus` + `deploy/grafana` 告警。

## D. cron 迁移（`pkg/cronx/wrapper.go`，行为零回归）

```go
// 旧: lock, ok, err := w.lock.TryLock(ctx, lockKey, w.lockTTL)          // ttl 位置参数
// 新:
lock, ok, err := w.lock.TryLock(ctx, lockKey, redislock.WithWatchdogTimeout(w.lockTTL))
// 语义不变：非阻塞（无 WaitTime）+ watchdog（w.lockTTL=30s 租约，10s 续约）。
// 类型 redislockx.Lock → redislock.RedisLock；import 与 mocks 同步更新。
// cronx 的 WithLockTTL(30s) 映射到 WithWatchdogTimeout(30s)。
```

---

## 落地顺序建议

**P1（改名 + 单机/集群 + Options 参数 + 自研核心）+ P2（fencing）性价比最高**：真安全 + 去 bsm 黑盒 + 双拓扑 + 干净包名 + cron 零回归。可先只做这两阶段上线；P3-P5 按后续消费者需求逐段推，避免造出当前无人用的复杂度。多主 quorum（P5）绑基础设施重构，单列最后。
