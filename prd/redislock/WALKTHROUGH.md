# redislock 代码阅读向导

> 路径：`webook/pkg/redislock/`
> 目的：从「这是什么」到「读哪个文件 / 每行为什么这么写 / 自己能不能复现」一站式打通自研分布式锁库
> 受众：想深入原理并内化、准备独立复现分布式锁的开发者（你自己）
> 维护：代码大改时同步本文，重点是 §4 走读链路 / §5 决策表 / 头部锚点行
> 代码锚点：分支 `redislock-p3-reentrant`，基线 commit `ce5e0f5`（P1+P2 已提交），工作区含未提交的 P3 可重入 + P4 阻塞/公平锁（`fair.go` / `pubsub.go` / `fair_*.lua` 为 untracked）。**本文所有行号绑定该工作区快照**，代码漂移时同步更新。

---

## 1. 30 秒电梯陈词

**redislock 是干嘛的？**

多副本部署下，用同一个 Redis 当「裁判」，保证一段临界区同一时刻只有一个副本在跑。输入：一个业务 key（如 `cronx:lock:ranking`）+ 一组 Options；核心能力：原子抢锁 / 自动续约 / 可重入 / 公平排队 / fencing 防误写；输出：一个锁句柄 `RedisLock`，或「没抢到」。

它是**纯自研**（移除了 bsm/redislock 三方库），核心是 6 段 embed 的 Lua 脚本 + Go 侧的 watchdog / pub-sub / 装饰器编排。

**与外部的关系**：

```
+-------------------+        +-------------------+        +-------------------+
| worker cron       | ─────▶ |  redislock        | ─────▶ |  Redis            |
| (pkg/cronx)       |        |  Client / RedisLock |      |  (UniversalClient)|
| 首个消费者         | ◀───── |  6 段 Lua + watchdog | ◀──── |  hash/string/list/ |
+-------------------+        +-------------------+        |  zset/pubsub      |
                                      │                    +-------------------+
                                      ▼
                             +-------------------+
                             | prometheus 装饰器  |
                             | webook_lock_*     |
                             +-------------------+
```

**Redis 挂了会怎样**：`TryLock` 返回 `(nil, false, err)`，由调用方降级（cron 场景=本轮跳过，不阻塞主流程）。watchdog 遇网络错误且距上次成功续约 ≥ 租约时长时，视同丢锁触发 `OnLost` 并退出——**宁可让贤也不幻觉持锁**（§4.6 核心）。

---

## 2. 阅读路径（按角色）

| 角色 | 推荐顺序 | 预计 |
|------|---------|------|
| **想内化原理（你）** | §1 → §3 → §4 全程（尤其 §4.6 / §4.9 反例）→ §5 → §6 自己写一遍 | 3h |
| **想接入业务侧** | §1 → §4.11 应用接入 → ARCHITECTURE §2.6 用法示例 → 抄 cronx.Wrapper | 40m |
| **想加新能力（读写锁/信号量）** | §4.0 共同模式 → §4.4 / §4.8 抄一份 Lua + 获取变体 → §B 能力矩阵 | 2h |
| **只想搞懂某个细节** | §4 对应站 + 该站反例分析（watchdog=§4.6，公平锁=§4.8，fencing=§4.9） | 45m |
| **配套权威 spec** | [ARCHITECTURE.md](./ARCHITECTURE.md)（设计决策的权威来源，冲突以它为准） | — |

**强烈推荐配套读**：[ARCHITECTURE.md](./ARCHITECTURE.md) 是权威详细设计（v7，含 ADR / 任务拆分 / 风险）。本指南是「读它 + 读代码」的辅助：spec 讲**为什么这么设计**，本文讲**代码在哪、每行怎么落地、你怎么复现**。

---

## 3. 整体架构图

三层清晰分离：**契约（接口）→ 实现（工厂 + 句柄）→ 原子核心（Lua）**，横切能力（指标）用装饰器叠加。

```
+-----------------------------------------------------------------------+
|  pkg/redislock  （契约与实现分文件，多实现各占一文件——项目铁律）              |
|                                                                       |
|  契约层     redislock.go   Client / RedisLock 接口 + ErrLockNotHeld    |
|                                                                       |
|  工厂层     factory.go     NewClient(UniversalClient) → Client         |
|                                                                       |
|  实现层     client.go      RedisClient：TryLock/Lock 分发               |
|             lock.go        Lock 句柄：Unlock/Refresh/查询 + watchdog    |
|             pubsub.go      blockingAcquire：订阅唤醒 + 兜底轮询          |
|             fair.go        acquireFair / dequeueFair：公平队列          |
|             fence.go       acquireFencing + FenceAccepted 资源侧契约    |
|                                                                       |
|  参数层     options.go     Options + lockConfig（watchdog vs 固定租约）  |
|             consts.go      redislock:{k}:* key Pattern（hash tag）      |
|                                                                       |
|  原子核心   script.go      //go:embed 6 段 Lua                         |
|             scripts/*.lua  acquire/release/refresh/fence/fair_*        |
|                                                                       |
|  横切装饰   prometheus/     MetricsClient / ObservedLock（webook_lock_*）|
+-----------------------------------------------------------------------+
                 ▲                                    ▲
                 │ 注入 UniversalClient                │ 装饰
        +----------------+                    +------------------+
        | worker/ioc     |  InitLockClient    | lockprom.Builder |
        | (DI 装配)       |───────────────────▶| .Build(inner)    |
        +----------------+                    +------------------+
```

**读码起点建议**：`redislock.go`（40 行看懂契约）→ `client.go`（获取分发）→ `scripts/acquire.lua`（原子核心）→ `lock.go`（句柄 + watchdog）。

---

## 4. 沿调用链走读：一次 TryLock 的一生

主链路：`cli.TryLock(ctx, key, opts...)` → 参数/身份解析 → 选路（普通/公平/fencing）→ 跑 Lua → 造句柄（可能起 watchdog）→ 业务用锁 → `lk.Unlock()`。

### 4.0 共同模式

> 多个站共用的约定，先讲一次，后面不再重复。

**接口 + 实现 + 构造返接口**（项目铁律，`redislock.go:19-50`）：
```go
type Client interface { TryLock(...); Lock(...) }        // 契约（领域名）
type RedisClient struct { cmd redis.UniversalClient }    // 实现（技术前缀）
func NewClient(uc redis.UniversalClient) Client { ... }  // 构造返接口
```
句柄同理：接口 `RedisLock`、实现 `*Lock`。装饰器实现 `MetricsClient` / `ObservedLock`。**为什么句柄叫 `RedisLock` 不叫 `Lock`**：避免与 `Client.Lock()` 方法同词歧义（ADR-4）。

**持有者身份 = ownerToken**：Go 没有稳定的 goroutine id，所以「谁持有锁」必须显式标识。默认每次获取生成一个随机 uuid（`client.go:20` `newToken`），存进 hash 的 field；`Unlock`/`Refresh` 拿自己的 token 去比对，不匹配就是「不在我手里」。

**存储统一 hash 模型**：`hash{ownerToken: 重入计数}`（ADR-3）。非重入锁就是「单 field、计数恒 1」；重入就是「同 field +1」。一套 Lua 同时覆盖两种，不为重入单开代码路径。

**原子性靠 Lua**：Redis 单线程执行整段 Lua，天然把「检查 + 修改」多步操作变成不可分割的一步——这是分布式锁正确性的地基，所有「先查后改」都必须在同一段 Lua 内完成，绝不能拆成两次客户端往返。

### 4.1 起点：`Client.TryLock` / `Client.Lock` 入口

**位置**：`client.go:31-62`
**为什么从这里**：所有获取都从这两个方法进；`TryLock` 是主 API（cron / 压测都用它），`Lock` 是纯阻塞语义（ctx 即等待上限）。

**这一步做了什么**：
- `TryLock`：`applyOptions` 合并默认值 → `validate()` 组合校验 → `resolveToken` 定身份 → 按有无 `WaitTime` 走「单次尝试」或「阻塞获取」。
- `Lock`：直接走阻塞路径，`deadline` 传零值（表示无 WaitTime 上限，只受 ctx 约束）。
- 两者的成功/失败语义在契约里钉死：`TryLock` 被占返 `(nil, false, nil)`——被占是**正常业务分支不是错误**。

**设计与原理**：
- `TryLock` 与 `Lock` 的分工：`TryLock(WaitTime=0)` 非阻塞、软失败；`TryLock(WaitTime>0)` 限时阻塞；`Lock` 阻塞到 ctx。三种阻塞程度用**同一套** `blockingAcquire` 实现（§4.7），只是 `deadline` 参数不同。
- 关键不变量：**被占 ≠ error**。网络错误 / ctx 取消才返 error。这条契约让调用方 `if !ok { return }` 就能表达「别人在跑，我跳过」，无需 `errors.Is` 判别。

**复现要点**：
- 必须有：`(RedisLock, bool, error)` 三返回值把「拿到 / 被占 / 出错」三态分开。
- 易踩坑：把「被占」当 error 返回 → 调用方被迫写错误分支，cron 场景每轮都误报错误指标。
- 可省略：`Lock` 阻塞方法初版可不做，只留 `TryLock`。

**追下一步**：→ §4.2 参数与身份解析（`applyOptions` / `resolveToken`，触发于 `client.go:32,36`）

### 4.2 站 1：参数解析与持有者身份

**位置**：`options.go:82-124`（config）+ `client.go:24-29`（token）

**这一步做了什么**：
- `applyOptions`（`options.go:82`）：起一个带默认值的 `lockConfig`（watchdog 租约 30s、兜底轮询 100ms），逐个 apply 调用方的 functional option。
- `validate`（`options.go:119`）：目前只校验一条——`WithFair` 与 `WithFencing` 不能同时开（需专门的 fair+fence 原子脚本，P4 未做），冲突直接 fail-loud 返 `ErrFairFencingUnsupported`。
- `resolveToken`（`client.go:24`）：`WithReentrant(ownerId)` 传了非空身份就用它（可重入、跨 goroutine 共享），否则每次生成随机 uuid（身份独立、天然不可重入）。

**设计与原理**：
- **全部参数经 Options 交付**（ADR-6）：签名保持 `(ctx, key, opts...)` 干净，不做 `TryLock(ctx, key, ttl, wait, fair...)` 这种位置参数地狱。加新能力=加一个 `WithXxx`，不动签名。
- **fail-loud 而非静默降级**：`fair+fencing` 组合若静默丢掉 fencing，调用方会误以为有真安全。宁可在获取入口直接报错（`options.go:120`）。
- 关键不变量：`ownerId == ""` ⟺ 不可重入。重入身份是**显式**的，绝不猜。

**复现要点**：
- 必须有：functional options + 一个 `lockConfig` 聚合参数 + 默认值兜底。
- 易踩坑：想学 Java 用 ThreadLocal 存 goroutine 身份——Go 没有稳定 gid，别 hack（ADR-2）。重入要么显式传 ownerId，要么不支持。
- 可省略：`validate` 组合校验初版可不做（只要不同时暴露 fair+fencing）。

**追下一步**：→ §4.3 key 布局（获取前要先知道往哪个 Redis key 写，`consts.go`）

### 4.3 站 2：数据模型与 key 布局

**位置**：`consts.go:1-24`

**这一步做了什么**：一把锁最多用到 5 个 Redis key，全部以 `redislock:{k}:` 为前缀，`{k}` 是调用方原始 key：

| 函数 | key 形态 | 类型 | 用途 |
|------|---------|------|------|
| `lockKey` | `redislock:{k}:lock` | hash | 锁主体 `{ownerToken: 计数}` |
| `fenceKey` | `redislock:{k}:fence` | string(INCR) | 单调 fencing 计数器 |
| `channelKey` | `redislock:{k}:ch` | pub/sub | 释放通知唤醒等待者 |
| `queueKey` | `redislock:{k}:queue` | list | 公平锁 FIFO 队列 |
| `qtsKey` | `redislock:{k}:qts` | zset | 公平锁死等待者逐出（token→deadline） |

**设计与原理**：
- **`{k}` 花括号是 Redis Cluster 的 hash tag**：集群按 key 算 slot 分片，但花括号内的内容才参与哈希。把一把锁的 5 个 key 都套上同一个 `{k}`，它们必落**同一个 slot**——这样多 key 的 Lua 脚本（如 release 同时操作 `lock` + `ch`）才不会报 `CROSSSLOT`。单机下花括号无副作用。
- 关键不变量：**同一把锁的所有 key 必须同 tag**。`ClusterClient.Eval` 按首 key 的 slot 路由，若某个 key 的 tag 不一致，Lua 会跨槽执行失败。
- 时间统一 int64 毫秒（`pexpire` / `pttl`），对齐项目全链路时间戳规范。

**复现要点**：
- 必须有：key 前缀集中定义（不散写 `fmt.Sprintf`），为集群预留 hash tag。
- 易踩坑：单机开发时 key 不加 hash tag，一上集群多 key Lua 全报 CROSSSLOT，且难复现。
- 可省略：初版只做 `lock` + `ch` 两个 key（公平/fencing 的 key 后续能力再加）。

**追下一步**：→ §4.4 获取核心 `acquire.lua`（拿到 key 就该抢锁了）

### 4.4 站 3：获取核心 `acquire.lua`（重入 hash 模型）

**位置**：`client.go:66-81`（`tryAcquire`）+ `scripts/acquire.lua:1-14`

**这一步做了什么**：
- `tryAcquire`（`client.go:66`）先按能力选路：`fair` → `acquireFair`（§4.8）；`fencing` → `acquireFencing`（§4.9）；都没有 → 跑 `acquireScript`。
- `acquire.lua` 的核心判断（`scripts/acquire.lua:4-8`）：
  ```lua
  if redis.call('exists', KEYS[1]) == 0
          or redis.call('hexists', KEYS[1], ARGV[2]) == 1 then
      redis.call('hincrby', KEYS[1], ARGV[2], 1)  -- 首次=1 / 重入 +1
      redis.call('pexpire', KEYS[1], ARGV[1])
      return -1
  end
  ```
- 返回值语义（钉死）：`-1` = 成功（首次或重入）；`>=0` = 被占，值为锁剩余 pttl（给阻塞路径算兜底等待用）。

**设计与原理**：
- **一个 if 同时覆盖「首次获取」和「重入」**：`exists==0`（锁不存在→首次）**或** `hexists ownerToken==1`（这个 field 已是我→重入），两种情况都是 `hincrby +1`。非重入锁计数恒为 1，重入锁计数累加。这就是 ADR-3「统一 hash 模型」的落地——不为重入写第二段代码。
- **为什么返回 pttl 而不是简单的 true/false**：阻塞路径（§4.7）需要知道「还要等多久锁才自然过期」，好在持有者崩溃、不发释放通知时也能兜底轮询。被占时把剩余 pttl 带回去，`blockWait` 用它算 `min(pttl, retryInterval)`。
- 关键不变量：**每次获取/重入都 `pexpire` 刷新租约**。哪怕重入，也顺手续一次租约，避免深层重入期间锁过期。

**复现要点**：
- 必须有：`exists || hexists` 双分支合并 + `hincrby` + `pexpire`，全在一段 Lua 内原子完成。
- 易踩坑：用 `SET key val NX PX ttl`（string 模型）做初版是对的，但一旦要重入就卡死——string 模型没法记「同一持有者第几次」。hash 模型是为重入铺的路。
- 可省略：`ttl < 0` 的防御分支（`acquire.lua:10-13`，理论上 exists 后 pttl 不会 <0）初版可不写。

**追下一步**：→ §4.5 句柄诞生与释放（拿到 `-1` 就该造 `RedisLock` 句柄了）

### 4.5 站 4：句柄诞生与释放（`newLock` + `release.lua`）

**位置**：`lock.go:30-45`（`newLock`）+ `lock.go:51-68`（`Unlock`）+ `scripts/release.lua:1-13`

**这一步做了什么**：
- `newLock`（`lock.go:30`）：把 `key/token/leaseMs/fence` 装进 `*Lock`，构造后这几个字段**不可变**。若 `watchdogEnabled()`（没设固定租约）就起一个后台 goroutine 跑 watchdog。
- `Unlock`（`lock.go:51`）：**先停 watchdog**（`stopOnce.Do(close(stop))`），再在 `innerMu` 保护下跑 `releaseScript`。
- `release.lua` 三态返回：`-1` = 不在我手里；`0` = 重入未归零仍持有；`1` = 完全释放并 `publish` 唤醒等待者。

**设计与原理**：
- **为什么 Unlock 先停 watchdog 再释放**（顺序不能反）：若先释放锁、watchdog 还活着，ticker 下一拍可能又把已删的锁 `pexpire` 续回来（其实 hexists 会失败，但更糟的是竞态窗口内的语义混乱）。先 `close(stop)` 保证释放后不再有续约动作。`sync.Once` 保证多次 Unlock / ForceUnlock 不会 double-close panic。
- **`innerMu` 串行化的是什么**：句柄的不可变字段无需锁，但 `Unlock`（release 脚本）和 watchdog（refresh 脚本）可能并发操作同一个 inner Redis 句柄。`innerMu` 把「续约」和「释放」两个脚本执行串起来，避免 go-redis 句柄的并发争用。
- **释放的原子递减**（`release.lua:7-13`）：`hincrby -1` 后 `>0` 说明还有重入层，只 `pexpire` 续租返 `0`；归零则 `del` + `publish`。**重入 N 次必须 Unlock N 次才真释放**——这是可重入锁的核心契约。
- 关键不变量：释放前先 `hexists ownerToken`（`release.lua:4`），不是自己就返 `-1` → Go 侧转 `ErrLockNotHeld`。**防误删他人锁**：你的锁 TTL 过期被别人拿走后，你的 Unlock 不会删掉别人的锁。

**重入计数时序**（同一 ownerToken="A"，acquire ×2 / unlock ×2）：
```
t0  TryLock(WithReentrant("A"))  exists=0    → hincrby A +1 → 计数=1，pexpire  得句柄
t1  TryLock(WithReentrant("A"))  hexists A=1  → hincrby A +1 → 计数=2           得句柄
t2  Unlock                        hincrby A -1 → 计数=1(>0)  → 只 pexpire，返 0（仍持有）
t3  Unlock                        hincrby A -1 → 计数=0      → del + publish，返 1（真释放）
```
跨 goroutine 共享同一临界区时各方传同一 `ownerId` 即可重入；**释放次数必须等于获取次数**才 `del`。默认（随机 token）天然不可重入——第二次获取的 token 不同，`exists!=0 且 hexists!=1`，会被自己挡住返回 pttl。

**复现要点**：
- 必须有：release 校验 ownerToken + 计数递减 + 归零才 del + 归零才 publish。
- 易踩坑：`Unlock` 用业务 ctx——业务 ctx 常已 cancel，Unlock 会失败。**应该**在 defer 里用独立 ctx（`context.Background()` 或新 `WithTimeout`），见契约注释 `redislock.go:34-35` 和 cronx 的 `release`（`wrapper.go:107-109`）。
- 可省略：`ForceUnlock`（`lock.go:70`，不校验持有者的兜底删锁）初版可不做。

**追下一步**：→ §4.6 watchdog 后台续约（`newLock` 起的那个 goroutine 在干什么）

### 4.6 站 5：Watchdog 三分支 —— 反例分析①「幻觉持锁」

**位置**：`lock.go:139-169`（`runWatchdog`）+ `lock.go:91-99`（`doRefresh`）+ `scripts/refresh.lua:1-7`

**这一步做了什么**：没设 `WithLeaseTime` 时，锁按 `watchdogTimeout`（默认 30s）拿租约，后台 goroutine 每 `watchdogTimeout/3`（默认 10s）跑一次续约脚本，直到 `Unlock` 关掉 `stop`。续约结果分三支处理。

**设计与原理**——这是全库最关键的正确性设计，用反例分析展开：

**场景（幻觉持锁时序）**：
```
t=0     副本 A 拿到锁，租约 30s，watchdog 每 10s 续约
t=5     A 进程发生长 GC / STW 暂停（或宿主机卡顿），watchdog goroutine 也被冻住
t=30    锁租约到期，Redis 自动删除 → 锁空出
t=31    副本 B 抢到同一把锁，开始跑临界区
t=40    A 的 GC 结束，watchdog 恢复，A 仍自认为持锁
        --> A 和 B 同时在临界区！双写！
```

**根因**：Go 侧 goroutine 被暂停期间，Redis 的 TTL 照常流逝。暂停结束后如果 watchdog「不管三七二十一继续自认持锁」，就会产生「A 以为自己持锁、其实锁早被 B 拿走」的幻觉。

**解法**（`lock.go:152-166`，P0 修复，务必保留）——续约三分支：
```go
case err == nil && ok:                       // 续约成功
    lastOK = time.Now(); l.fireOnRefresh()
case err == nil && !ok:                       // token 不是我了 → 干净丢锁
    l.fireOnLost(ErrLockNotHeld); return
default:                                       // 网络错误 / 超时
    if time.Since(lastOK) >= leaseDur {        // 距上次成功 >= 租约：锁早该过期
        l.fireOnLost(err); return              // 视同丢锁，告警 + 退出 goroutine
    }
```
- 第一支「成功」：刷新 `lastOK` 时间戳（这是判断第三支的基准）。
- 第二支「`refresh.lua` 返回 0」：`hexists` 不再命中我的 token，说明锁被别人拿走了，干净地丢锁——触发 `OnLost` 并 `return` 退出 goroutine。
- 第三支「网络错误」：不能立刻判丢（可能只是抖动），但**如果距上次成功续约已 ≥ 一个租约时长，那么锁在 Redis 侧早该过期了**，继续自认持锁就是幻觉 → 触发 `OnLost` 并退出。

**反例**：如果没有第三支（网络错误就无脑静默重试）会怎样？
```
Redis 网络分区，续约请求全部超时
watchdog 无脑重试，永远进不了「丢锁」分支
--> ① 锁在 Redis 侧早已过期、别人已持有，A 却永远自认持锁（幻觉）
--> ② 网络永不恢复时，这个 goroutine 永远不退出（泄漏）
```
第三支用 `time.Since(lastOK) >= leaseDur` 这一个判断，同时堵死了「幻觉持锁」和「goroutine 泄漏」两个问题。

- 关键不变量：**丢锁必退出**。任何一支判定丢锁都 `return`，不留活着但已失效的 watchdog。回调 `fireOnLost/fireOnRefresh` 都包 `recover`（`lock.go:173-187`），回调 panic（如指标库重复注册）不拖崩整个进程。

**复现要点**：
- 必须有：`lastOK` 时间戳 + 三分支 + 丢锁 `return`。第三支的「距上次成功 ≥ 租约」判断是灵魂，别省。
- 易踩坑：只做「成功/失败」两分支，把网络错误归到「失败=丢锁」——网络抖一下就误判丢锁，锁频繁被判失效；或归到「继续重试」——就是上面的反例。必须三分支。
- 可省略：`OnRefresh` 回调（`lock.go:181`）初版可不做，但 `OnLost` 是告警命脉，不能省。

**追下一步**：→ §4.7 阻塞获取（`TryLock` 拿不到但愿意等时，怎么等）

### 4.7 站 6：阻塞获取 pub/sub（`blockingAcquire`）

**位置**：`pubsub.go:17-52`（`blockingAcquire`）+ `pubsub.go:57-74`（`blockWait`）

**这一步做了什么**：`TryLock(WaitTime>0)` 和 `Lock` 都走这里。流程：**先订阅**释放通道 → 循环试获取 → 拿到就返回；被占则 `select` 等「释放通知 / 兜底定时器 / ctx.Done」三者之一，然后重试。

**设计与原理**——第二个微妙点，用反例展开：

**为什么订阅必须先于试获取**（`pubsub.go:18` 的 `Subscribe` 在 `for` 循环之前）：
```
反例（先试获取、再订阅）：
t=0   等待者试获取 → 被占，准备去订阅
t=1   持有者恰好 Unlock → publish "released"（此刻还没人订阅，消息丢失）
t=2   等待者才 SUBSCRIBE → 永远等不到已经发过的通知
      --> 除非兜底轮询到点，否则白等一整个 retryInterval（甚至更久）
```
**解法**：先 `Subscribe` 再进循环试获取。这样「订阅」早于「可能触发释放的那次获取尝试」，堵死了「释放信号在订阅前发出」的丢失窗口。所有退出路径经 `defer pubsub.Close()` 退订，防连接 / goroutine 泄漏（`pubsub.go:19`）。

**兜底轮询为什么不能省**（`blockWait`，`pubsub.go:57`）：pub/sub 唤醒是「优化」不是「正确性保证」——持有者**崩溃**时不会 publish，等待者只能靠轮询在锁自然过期附近醒来。`blockWait` = `min(pttl, retryInterval)` 并受 deadline 约束：正常时 100ms 轮询一次兜底，锁快过期时（pttl 小）更密地重试。

- 关键不变量：`deadline` 零值区分两种语义——`Lock`（deadline 零，只受 ctx 约束）vs `TryLock+WaitTime`（deadline 非零，到点软失败返 `(nil,false,nil)`）。`blockWait` 返回 `<=0` 即 deadline 已到（`pubsub.go:66`）。
- 集群增强预留：`pubsub.go:14-16` 注明 7.0+ 可用 sharded `SSUBSCRIBE`（channel 与锁同 hash-tag、同 slot）省广播开销；当前用广播 `SUBSCRIBE`，单机/集群都正确（集群下退化为广播），留待有集群消费者时优化。

**复现要点**：
- 必须有：先订阅 → 循环{试获取 → select(通知/定时器/ctx)}→ defer 退订。轮询兜底不能省（崩溃的持有者不发通知）。
- 易踩坑：忘了退订 → 每次阻塞获取泄漏一个 pub/sub 连接；先获取后订阅 → 丢唤醒。
- 可省略：sharded pub/sub 优化（有集群消费者前不用做）。

**追下一步**：→ §4.8 公平锁（阻塞获取默认是抢占式，谁醒得快谁拿，怎么改成 FIFO 排队）

### 4.8 站 7：公平锁四步 Lua —— 反例分析②「饿死」

**位置**：`fair.go:16-37`（`acquireFair` / `dequeueFair`）+ `scripts/fair_acquire.lua:1-44`

**这一步做了什么**：`WithFair()` 时 `tryAcquire` 走 `acquireFair`，跑 `fair_acquire.lua`。它在**一段原子脚本**里干四件事：清理死等待者 → 重入判断 → FIFO 获取 → 入队/刷新心跳。与 §4.7 的 `blockingAcquire` 唤醒循环协作：被唤醒后重试，但只有队头能成功。

**设计与原理**——为什么需要公平锁，用反例展开：

**反例（抢占式饿死）**：默认（非公平）阻塞获取是「谁醒得快谁拿」。
```
等待者 A 已经等了 5s，等待者 B、C 刚来
持有者释放 → publish 唤醒所有等待者
A、B、C 同时重试 → 恰好 B 的网络往返最快，B 拿到
下一轮又是 C 抢先... A 可能永远抢不到 --> 早等者被后来者反复插队饿死
```
**解法**——`fair_acquire.lua` 四步（每步都在同一原子脚本内）：

第 1 步 清理死等待者（`fair_acquire.lua:7-14`）：从队头循环 `lpop` 掉 `deadline <= now` 的 token（崩溃/放弃者停止刷新 deadline 就会被清），不让死者堵死队列头。
```lua
while true do
    local head = redis.call('lindex', KEYS[2], 0)
    if not head then break end
    local dl = redis.call('zscore', KEYS[3], head)
    if (not dl) or (tonumber(dl) > tonumber(ARGV[3])) then break end
    redis.call('lpop', KEYS[2]); redis.call('zrem', KEYS[3], head)
end
```
第 2 步 重入（`:17-21`）：已被我持有 → 计数 +1（公平锁同样支持重入）。
第 3 步 FIFO 获取（`:24-33`）：锁空闲 **且**（队空 **或** 队头是我）→ 出队 + `hincrby` 获取。**只有队头能拿到**，这是 FIFO 的落点。
第 4 步 排队（`:36-44`）：拿不到 → 首次 `rpush` 入队尾 + `zadd` 刷新 deadline（心跳），返回锁剩余 pttl。

**心跳与逐出的配合**（`options.go:114-116` `heartbeatMs = 3×retryInterval`）：活着的等待者每次重试（间隔 ≤ retryInterval）都在第 4 步刷新 deadline，3× 余量下不会被误逐；崩溃者停止刷新，约 3×retryInterval 后被队头的第 1 步清理逐出。

**非阻塞公平锁的优雅退出**（`client.go:44-46` + `pubsub.go:34,43`）：公平锁获取失败时脚本已把你入队了。若你并不打算等（`TryLock` 无 WaitTime，或 WaitTime 到点，或 ctx 取消），必须 `dequeueFair`（`fair.go:32`，跑 `fair_cancel.lua` 从 queue+qts 移除自己），否则你会占着队头位置堵死后面的人，直到 deadline 才被逐出。

- 关键不变量：清理 + 重入 + 获取 + 入队**必须在同一段 Lua 原子完成**。若拆成多次往返，「清理完到获取之间」别的等待者可能插进队头，FIFO 破坏。这是全库并发正确性风险最高的一段（见 ARCHITECTURE §4 风险）。

**深挖：为什么四步必须在同一段 Lua（清理-获取原子性竞态）**

假设把「看队头」和「获取」拆成两次客户端往返：

反例（拆成两次往返）：
```
队列 [B(队头), A]，锁刚空闲；X 是代表 A 的客户端
t0   X: LINDEX 看队头 → 读到 B，"不是我，不能拿"          （判断成立）
t0'  同时，代表 B 的客户端跑完整获取 → B 出队并拿到锁，队列变 [A]
t1   锁很快释放，队列 [A]，A 此刻已是队头
t2   X: 基于 t0 的陈旧结论"队头不是我"，继续傻等
     --> A 明明已排到队头却错过这一轮，FIFO 时序被打乱
```
更糟的变体：若 X 读到「队头恰是我」后才发获取命令，这两步之间别的等待者的完整获取流程能整个挤进来，X 基于过期判断误取 → **两个持有者，互斥破坏**。

根因：FIFO 正确性依赖「判断队头==我」与「出队+获取」之间**没有任何其他命令插入**；只要跨客户端往返，这个 TOCTOU（检查时到使用时）窗口就存在。解法就是把四步塞进**一段** Lua——Redis 单线程执行整段脚本期间没有别的命令能插进来，「判断」与「动作」对外是一个不可分割的原子操作。所以公平锁的集成测必须用**真 Redis + 多等待者**跑（miniredis 串行执行会掩盖这类竞态）。

**复现要点**：
- 必须有：list(FIFO 队列) + zset(deadline 逐出) + 四步原子脚本 + 失败时的优雅退队。
- 易踩坑：① 忘了死等待者逐出 → 一个崩溃的等待者永久堵死队头，后面全饿死；② 非阻塞路径忘了 `dequeueFair` → 队列被「来试一下就走」的人塞满。
- 可省略：公平锁是「大」能力（ARCHITECTURE 任务 10），无饿死困扰的场景（如 cron 单抢）根本不用开。

**追下一步**：→ §4.9 fencing（前面所有能力都是「大概率互斥」，真安全靠这个）

### 4.9 站 8：Fencing 唯一真安全 —— 反例分析③「双写」

**位置**：`fence.go:13-44`（`acquireFencing` / `FenceAccepted`）+ `scripts/fence.lua:1-21`

**这一步做了什么**：`WithFencing()` 时走 `acquireFencing`，跑 `fence.lua`。它在获取的同一段 Lua 内，**仅对全新获取** `INCR` 一个单调计数器，令牌随句柄返回（`Fence()`）；重入不 bump。返回数组 `{status, fence}`。

**设计与原理**——这是 ARCHITECTURE §0「两条安全真相」的第一条，也是理解整个库定位的关键：

**反例（为什么 TTL + 续约挡不住双写）**：这正是 §4.6 幻觉持锁的续集——就算 watchdog 完美，也有挡不住的时序：
```
t=0   A 拿到锁（fence=7），准备写数据库
t=5   A 长 GC 暂停
t=30  锁过期，B 拿到锁（fence=8），B 写库：last_fence=8
t=40  A GC 结束，watchdog 已判丢锁并退出，但 A 手里的业务代码还在跑
      A 拿着「陈旧的 fence=7」去写库
      --> 如果库不校验 fence，A 覆盖了 B 的写！双写！
```
**根因**：锁只能保证「大概率同一时刻一个持有者」，但挡不住「暂停结束后的过期持有者仍在执行」。这是分布式锁的**固有**局限，不是实现 bug。

**解法**——单调令牌 + **资源侧校验**（缺一不可）：
- 锁侧（`fence.lua:7-11`）：全新获取才 `INCR`，保证令牌单调递增，且**不设过期**（`fence.lua` 注释：过期会导致单调断裂，是已知风险）。
- 资源侧（`fence.go:42-44` `FenceAccepted`，二选一）：
  ```go
  // 应用层校验：
  if !redislock.FenceAccepted(lk.Fence(), stored.LastFence) { return ErrStaleFence }
  // 或 DB 条件写（天然原子、免读-改-写竞态）：
  // UPDATE res SET data=?, fence=? WHERE id=? AND fence < ?   -- 影响行数=0 → 拒绝
  ```
  回到反例：B 写入 `last_fence=8` 后，A 拿 `fence=7` 来写，`7 > 8` 为假 → **拒绝**。双写被资源侧挡下。

- 关键不变量：**未接资源侧校验 = 没上安全锁**，只是「大概率互斥」。`fence.go:41` 说得很直白。cron 场景幂等重算属可接受的 best-effort，所以 cron 消费者没开 fencing。真正要「绝不双写」（转账、扣款）才必须资源侧配合。
- 集群/多主耦合：`INCR` 必须单一权威源，多主/多分片各自 INCR 不单调 → fencing 计数器固定落权威节点（ARCHITECTURE §3.3，P5 事项）。

**复现要点**：
- 必须有：获取时原子 INCR（仅全新获取）+ 令牌随句柄返回 + **文档明示资源侧必须校验**。
- 易踩坑：以为「开了 WithFencing 就安全了」——不接资源侧校验等于没开。fencing 是**协议**，需要被保护资源配合，不是锁内部一个开关。
- 可省略：如果消费者都是幂等的（cron 重算），可以完全不做 fencing。

**追下一步**：→ §4.10 参数语义矩阵（前面走完了所有能力，回头把「租约模式」这条主线串清）

### 4.10 站 9：参数语义矩阵（watchdog vs 固定租约）

**位置**：`options.go:93-109`（`leaseMs` / `watchdogEnabled` / `watchdogInterval`）

**这一步做了什么**：把「租约多长、要不要后台续约」两个正交问题，收敛成一条规则——**是否设 `WithLeaseTime` 决定一切**：

| `WithLeaseTime` | `WithWaitTime` / 方法 | 行为 |
|---|---|---|
| 未设 | 未设 + `TryLock` | 立即尝试；拿到则 watchdog（30s 租约，10s 续约） |
| 未设 | `5s` + `TryLock` | 最多阻塞等 5s；拿到则 watchdog |
| 未设 | `Lock` | 阻塞至 ctx；拿到则 watchdog |
| `30s` | 未设 + `TryLock` | 立即尝试；拿到则固定 30s 租约、**无 watchdog**、到期自动释放 |
| `30s` | `Lock` | 阻塞至 ctx；拿到则固定 30s 租约、无 watchdog |

**设计与原理**：
- `watchdogEnabled()`（`options.go:102`）= `leaseTime <= 0`。设了固定租约就**关**watchdog；没设就走 watchdog 模式，租约 = `watchdogTimeout`（默认 30s），续约每 `/3`。`leaseMs()`（`options.go:94`）统一算本次租约毫秒：固定租约优先，否则 watchdog 租约。
- **为什么两种模式**：长临界区（cron 重算 1-2min，时长不定）用 watchdog——只要活着就一直续约，不怕临界区超过租约；短临界区（明确 3s 内完成）用固定租约——省掉后台 goroutine，到期自动释放兜底。
- 关键不变量：`WithWaitTime` 只控**获取阶段等多久**，`WithLeaseTime`/`WithWatchdogTimeout` 控**持有阶段租约多长**，两者正交，别混。

**复现要点**：
- 必须有：一个「设了固定租约就关 watchdog」的开关判断。
- 易踩坑：以为 `WithWaitTime` 和 `WithLeaseTime` 是一回事——一个管等锁、一个管持锁。
- 可省略：固定租约模式（初版可只做 watchdog）。

**追下一步**：→ §4.11 应用接入（库讲完了，看真实消费者怎么把它装起来用）

### 4.11 站 10：应用接入（cronx + DI + 指标）

**位置**：`worker/ioc/redis.go:26-54`（DI + 锁 client 校准）+ `pkg/cronx/wrapper.go:89-114`（消费者）+ `pkg/redislock/prometheus/builder.go:120-174`（装饰器）

**这一步做了什么**：把三段真实接入串起来看——DI 怎么装、消费者怎么用、指标怎么埋。

**① DI 装配**（`worker/ioc/redis.go:51`）：
```go
func InitLockClient(cmd redis.UniversalClient) redislock.Client {
    return lockprom.NewPrometheusBuilder("webook", "lock", "分布式锁").
        Build(redislock.NewClient(cmd))
}
```
一个 `redis.UniversalClient` 注进 `NewClient`，外面套一层 prometheus 装饰器。单机注入 `*redis.Client`、集群注入 `*redis.ClusterClient`，库代码零改动（§4.3 hash tag 已铺好路）。

**锁专用 Redis client 的关键校准**（`worker/ioc/redis.go:26-39`，`InitRedis`）：喂给锁的 client 与 cache 的共享 client 刻意分道——
- `MaxRetries=-1` **关自动重试**：`acquire` 脚本是 `hincrby` 计数、**非幂等**。go-redis 在「命令已执行但响应丢失」时会重发 → 重复 `+1` → 计数虚高、`Unlock` 减不到 0、锁滞留到 lease 过期（这是 §4.6 之外的另一条幻觉持有路径，期间别的副本抢不到）。锁的瞬时错误交给调用方降级 + watchdog 自身的重试循环，不靠 go-redis 静默重试（`refresh`/`release` 幂等，唯 `acquire` 有此患，故整体关重试最稳）。
- `ContextTimeoutEnabled=true`：让 ctx deadline 真正作用到 I/O，`Lock`/`TryLock` 的 ctx 上限能被及时打断。

**② 真实消费者**（cronx.Wrapper，`wrapper.go:89`）：
```go
lock, ok, err := w.lock.TryLock(ctx, lockKey, redislock.WithWatchdogTimeout(w.lockTTL))
if err != nil { /* 记 error 指标，本轮放弃 */ }
if !ok { /* 记 skipped 指标，别的副本在跑 */ }
defer w.release(name, lock)   // release 用独立 ctx（wrapper.go:107）
```
cron 用**非阻塞 + watchdog**：`TryLock` 无 WaitTime（抢不到就跳过本轮，不排队），`WithWatchdogTimeout(30s)`（30s 是 crash 让贤窗口，10s 续约保活）。这正是 §4.1 + §4.6 + §4.10 三条主线的落地。

**③ 指标装饰器**（`prometheus/builder.go`）：`MetricsClient` 实现 `Client`，切点在 acquire（success/busy/error）/ wait_seconds / held_seconds / watchdog_lost / fence_issued。两个巧思：
- `withOnLost`（`builder.go:120`）：装饰器把「watchdog 丢锁 → `watchdog_lost_total++`」作为**默认** `OnLost` 注入到调用方 opts **前面**。因为 Options 是 for-range 顺序 apply，调用方若自己也传 `WithOnLost` 会**覆盖**默认——所以 cronx **绝不能**再传 `WithOnLost`，否则丢锁指标失灵（`worker/ioc/cron.go:23` 注释专门警告）。
- `ObservedLock`（`builder.go:164`）：代理真句柄，`Unlock` 时观测持有时长 `held_seconds`，其余方法透传内嵌的 `RedisLock`（Go 嵌入 = 免费透传）。

**设计与原理**：
- **装饰器模式挂指标**（不侵入核心）：核心 `RedisClient` 不 import prometheus，pkg 层零业务依赖。可观测性靠「回调（OnLost/OnRefresh）+ 装饰器」两条路，符合「pkg 不依赖项目 logger」的约束。
- 关键不变量：指标命名 `webook_lock_*`（subsystem=lock），**不带服务名**，service 靠 prometheus 注入的 `job` label 区分（项目「Metric 命名」铁律）。

**复现要点**：
- 必须有：`Client` 接口让装饰器能包一层；构造函数返接口（否则没法替换成装饰器）。
- 易踩坑：在核心里直接埋指标——违反 pkg 零依赖；调用方和装饰器都传 `OnLost` 导致覆盖。
- 可省略：装饰器初版可不做，先让核心跑通。

**追下一步**：→ §4.12 终点

### 4.12 终点：结束状态

**位置**：`lock.go:51-68`（`Unlock` 返回）
**结束状态**：一次完整的锁生命周期结束后——
- 成功路径：`Unlock` 计数归零 → Redis `lock` key 被 `del` → `publish "released"` 唤醒等待者 → watchdog goroutine 已在 Unlock 开头 `close(stop)` 退出 → 装饰器观测到 `held_seconds`。外部可观察：锁 key 消失，等待者中的（公平锁下的队头）被唤醒并拿到锁。
- 丢锁路径：watchdog 判定丢锁 → `fireOnLost` → 装饰器 `watchdog_lost_total++` → 告警。业务侧句柄仍在手里但已失效，后续 Unlock 会返 `ErrLockNotHeld`（cronx 对这个 error 静默，`wrapper.go:110`）。

---

## 5. 关键设计决策

| 维度 | 决策 | 理由 | 何时改 |
|------|------|------|--------|
| 数据模型 | `hash{ownerToken: 计数}` 统一模型 | 重入=同 field +1，非重入即单 field 恒 1；一套 Lua 覆盖两种（ADR-3） | 需要给锁存多字段元数据时 |
| key 布局 | `redislock:{k}:*` hash tag | 集群下一把锁全部 key 落同 slot，化解多 key Lua CROSSSLOT | 换存储引擎 / 弃用集群 |
| 接口契约 | `TryLock` 被占返 `(nil,false,nil)` 非 error | 被占是正常业务分支不是错误；error 只留给网络/ctx | 永不（契约根基） |
| 持有者身份 | 随机 uuid token，重入靠显式 `WithReentrant` | Go 无稳定 goroutine id，不 hack ThreadLocal（ADR-2） | Go 提供稳定 gid（不会发生） |
| 并发模型 | Lua 服务端原子 + 客户端 `innerMu` 串行续约/释放 | 单线程 Lua 保跨 key 原子；innerMu 防句柄并发争用 | 永不 |
| 错误处理 | 释放/续约先校验 ownerToken，不匹配返 `ErrLockNotHeld` | 防误删他人锁（自己过期后别人已持有） | 永不 |
| 客户端重试 | 喂锁的 redis client `MaxRetries=-1` 关自动重试 | `acquire`(hincrby) 非幂等，命令重发→重复 +1→计数虚高→锁滞留（幻觉持有） | 永不 |
| **续约容错（核心）** | watchdog 三分支：网络错误且距上次成功 ≥ 租约 → 视同丢锁退出 | 同时堵死「幻觉持锁」+「goroutine 泄漏」（P0） | 永不（安全命脉） |
| **安全模型（核心）** | fencing 单调令牌 + **资源侧校验** | 唯一挡得住 GC/STW 暂停后过期持有者误写的手段（ARCHITECTURE §0） | 永不 |
| **公平算法（核心）** | list(FIFO) + zset(deadline) 四步原子 Lua | 抢占式会饿死早等者；zset 心跳逐出崩溃等待者 | 重构公平语义时 |
| 阻塞唤醒 | 订阅先于试获取 + pttl 兜底轮询 | 堵死「释放信号在订阅前发出」丢失窗口；崩溃者不 publish 靠轮询兜底 | 换消息机制 |

---

## 6. 自己动手：最小复现骨架

> 目标：不抄原代码，用 60 行搞清楚「哪些是分布式锁的必要复杂性、哪些是本库的工程化加料」。分三块写。

**块 1：获取 + 释放的原子核心**（这是地基，Lua 必须原子）
```lua
-- acquire.lua：-1=成功/重入；>=0=被占返 pttl
if redis.call('exists', KEYS[1])==0 or redis.call('hexists', KEYS[1], ARGV[2])==1 then
    redis.call('hincrby', KEYS[1], ARGV[2], 1)
    redis.call('pexpire', KEYS[1], ARGV[1]); return -1
end
return redis.call('pttl', KEYS[1])
-- release.lua：先校验 ownerToken，计数归零才 del
if redis.call('hexists', KEYS[1], ARGV[2])==0 then return -1 end
if redis.call('hincrby', KEYS[1], ARGV[2], -1)>0 then return 0 end
redis.call('del', KEYS[1]); return 1
```

**块 2：Go 侧最小句柄 + 获取**（伪代码）
```go
type Lock struct { cmd redis.Cmdable; key, token string; leaseMs int64 }

func TryLock(ctx, cmd, key string) (*Lock, bool, error) {
    token := uuid.NewString()
    res, err := acquire.Run(ctx, cmd, []string{"lock:"+key}, 30000, token).Int64()
    if err != nil { return nil, false, err }        // 网络错误
    if res != -1 { return nil, false, nil }          // 被占（非 error！）
    return &Lock{cmd, "lock:"+key, token, 30000}, true, nil
}
func (l *Lock) Unlock(ctx) error {
    res, err := release.Run(ctx, l.cmd, []string{l.key}, l.token).Int64()
    if err != nil { return err }
    if res == -1 { return ErrLockNotHeld }           // 不在我手里
    return nil
}
```

**块 3：watchdog 灵魂三分支**（伪代码，正确性关键）
```go
lastOK := time.Now()
for range ticker.C {
    ok, err := refresh(ctx)
    if err == nil && ok { lastOK = time.Now(); continue }   // 续约成功
    if err == nil && !ok { onLost(); return }               // token 不是我了
    if time.Since(lastOK) >= leaseDur { onLost(); return }  // 网络错误+超租约=幻觉，退出
}
```

对照原代码理解：

- **必要复杂性**（骨架也必须做）：① Lua 原子获取/释放；② 被占 ≠ error 的三态返回；③ 释放校验 ownerToken 防误删；④ watchdog 三分支（若要长临界区保活）；⑤ hash 模型（若要重入）。
- **工程化加料**（骨架可先不做）：① 公平锁 queue+zset（无饿死场景不用）；② fencing（幂等业务不用）；③ pub/sub 唤醒（纯轮询也对，只是慢）；④ 集群 hash tag（单机不用）；⑤ prometheus 装饰器；⑥ `innerMu` 串行化（单 goroutine 用锁时不需要）。

---

## 7. 学完了，可以做什么

读完本指南，你应该能独立完成：

1. **给任意多副本任务加分布式锁**：注入 `redislock.Client`，`TryLock` + `defer Unlock`（独立 ctx），30 行搞定，照抄 `cronx.Wrapper`（§4.11）。
2. **选对参数**：长临界区用 watchdog、短临界区用 `WithLeaseTime`；要排队用 `WithFair`；跨 goroutine 共享锁用 `WithReentrant`（§4.10）。
3. **给要求「绝不双写」的业务上真安全**：`WithFencing()` + 资源侧 `FenceAccepted` 或 DB 条件写（§4.9），并说清「不接资源侧校验 = 没上安全锁」。
4. **从零复现一个分布式锁**：照 §6 三块骨架，先 Lua 原子核心 + 三态返回，再按需加 watchdog/重入/公平/fencing。
5. **诊断线上锁问题**：`watchdog_lost_total` 涨=幻觉持锁风险；`acquire_total{result=busy}` 高=争用激烈；`wait_seconds` P99 高=阻塞等待久（§4.11 指标）。
6. **加新锁能力**（读写锁/信号量）：照 §4.0 共同模式 + §4.8 公平锁的「Lua 变体 + 获取变体」范式扩展。

不会的去 [ARCHITECTURE.md](./ARCHITECTURE.md)（权威 spec）+ 直接读代码（本指南每站有 file:line 索引）。

---

## 可选章节

### A. 能力开关矩阵

| Option | 走哪个获取变体 | 用到的 Lua | 用到的 key | 阻塞路径才有意义 |
|--------|--------------|-----------|-----------|:---:|
| （默认） | `tryAcquire` 直跑 | acquire | lock | — |
| `WithReentrant(id)` | 同上（token=id） | acquire（hexists 命中即重入） | lock | — |
| `WithWaitTime(d)` / `Lock` | `blockingAcquire` | acquire + SUBSCRIBE | lock + ch | 是 |
| `WithFair()` | `acquireFair` | fair_acquire / fair_cancel | lock + queue + qts + ch | 是 |
| `WithFencing()` | `acquireFencing` | fence | lock + fence | — |
| `WithFair()`+`WithFencing()` | 获取入口 `validate()` 直接报错 | — | — | 不支持（`ErrFairFencingUnsupported`） |

### B. 实现状态（防「待办误当已实现」）

对齐 ARCHITECTURE.md 头部与 CHANGELOG（2026-07-08）：

| 阶段 | 能力 | 状态 | 落点 |
|------|------|:---:|------|
| P1 | 改名去 bsm + 自研 Lua 核心 + 单机/集群 + Options | done（`ce5e0f5` 已提交） | client/lock/script/options |
| P2 | fencing 真安全 | done（`ce5e0f5` 已提交） | fence.go + fence.lua |
| P3 | 可重入 `WithReentrant` | done（工作区未提交） | options.go + client.go resolveToken |
| P4 | 阻塞增强（pub/sub）+ 公平锁 `WithFair` | done（工作区未提交） | pubsub.go + fair.go + fair_*.lua |
| P5 | 多主 quorum + 部署重构 | **pending**（无消费者，未动工） | 规划中的 quorum.go / factory.NewQuorumClient |

**注意**：`redislock.go:31`（Client 接口）尚无 `NewQuorumClient`，`ARCHITECTURE §2.6` 里的多主用法示例是 P5 目标、当前不可用。多主的设计走读见 [§E](#e-前瞻多主-quorump5设计已定代码未实现无-fileline-锚点)。

### C. 测试与压测组织

| 层 | 文件 | 验什么 | 怎么跑 |
|----|------|--------|--------|
| 单测（miniredis） | `*_test.go`（client/lock/fence/fair） | Lua 逻辑 / 参数矩阵 / watchdog 三分支 / fencing 单调 | `cd webook/pkg && go test ./redislock/...` |
| 集成测（真 Redis） | `internal/integration/redislock_test.go` | 100 goroutine 抢锁只 1 赢 / 跨 Client 互斥 / watchdog 保活 / 公平 FIFO | `cd webook/internal && go test ./integration/ -run Redislock` |
| 微基准 | `bench_test.go` | 各能力开销（重入/watchdog/pub-sub vs 轮询/fencing） | `go test -bench . ./redislock/` |
| 压测 harness | `loadtest/loadtest.go` + `loadtest_test.go` | **MutexViolations / FenceMonotonicBreaks 必须 = 0**（真互斥铁证）+ QPS/分位 | `cd webook/pkg && REDISLOCK_REDIS_PASS=xxx go test ./redislock/loadtest/ -run TestLoad -v`（并发/时长为表内硬编码用例、非 flag；地址走 env `REDISLOCK_REDIS_ADDR`，默认 `127.0.0.1:6379`） |
| JMeter 壳 | `loadtest/cmd/loadserver` | 外部协议压测工具驱动，同一套不变量口径 | 见 `loadtest/jmeter/README.md` |

**压测不变量的巧思**（`loadtest.go:109-123`）：每 key 一个原子计数器，acquire 后 `+1`、若 `>1` 即 `MutexViolations++`；退临界区时**先减计数再 Unlock**（此刻 Redis 锁仍在，避免「释放后被抢」的假阳性）。这比断言 Redis 内部状态更贴近「真的只有一个持有者」的业务真相。

### D. 常见坑速查（不要 / 应该）

- 不要：`defer lk.Unlock(ctx)` 用业务 ctx。应该：用独立 ctx（业务 ctx 已 cancel 时 Unlock 仍要走完）。
- 不要：把 `TryLock` 的「被占」当 error 处理。应该：`if !ok { return }`，只有第三个返回值是真错误。
- 不要：开了 `WithFencing()` 就以为安全了。应该：资源侧必须 `FenceAccepted` 或 DB 条件写，否则只是「大概率互斥」。
- 不要：在 cronx 里再传 `WithOnLost`。应该：留给 prometheus 装饰器记 `watchdog_lost_total`（自己传会覆盖它）。
- 不要：集群下让一把锁的多个 key 用不同 hash tag。应该：全部套同一个 `{k}`（`consts.go` 已保证）。
- 不要：非阻塞公平锁获取失败后不管。应该：`dequeueFair` 优雅退队，不占位堵后面的人。

### E. 前瞻：多主 quorum（P5，设计已定、代码未实现，无 file:line 锚点）

> 本节走的是 ARCHITECTURE §3.6 的**设计**，不是现有代码——`NewQuorumClient` 尚未落地（§B）。
> 读它是为完整理解锁库的可用性演进，不为复现现有代码。落地后应升级为 §4 正式走读站并补锚点。

**要解决什么**：单机/集群都改不了「唯一那台 Redis（或那个集群）挂了，锁就没了」。多主 = N 个**互相独立**的 Redis（非同集群主从），必须多数派同意才算拿到锁——挂掉少数节点仍可用。这是 Redlock 家族的思路。

**获取算法 + validity 计算**（伪代码，关键在 validity）：
```
start = now
for each of N 独立节点:                       -- 逐个用 §4.4 的 acquire 脚本，各带 QuorumTimeout(默认 200ms)
    if 该节点获取成功: succ++
validity = leaseMs - (now - start) - clockDrift    -- ★ 有效租约，不是 leaseMs
if succ >= N/2+1 and validity > 0: 持有，有效期取 validity
else:                              对所有节点执行释放，返回 ErrNotObtained
```

**为什么 validity 要从 leaseMs 里扣两笔**（最不显然、最容易写错处）：
- 扣 `(now - start)`：向 N 个节点**逐个**获取本身耗时；等你拿齐多数派，最早那个节点的租约已流逝 `(now-start)`。真正还能安全用的是「最早那把子锁的剩余租约」，保守取 `leaseMs - 已耗时`。
- 扣 `clockDrift`（默认 `leaseMs*0.01 + 2ms`）：各节点物理时钟有漂移，`pexpire` 计时不完全一致，扣一个余量防「你以为还有效、某节点已提前过期」的边界误判。
- 若 `validity <= 0`：获取多数派耗时已吃掉整个租约，拿到也是过期的 → 判失败并全部释放。

**worked example**：`leaseMs=30000`、获取耗时 250ms、`clockDrift=30000*0.01+2=302ms` → `validity = 30000 - 250 - 302 = 29448ms ≈ 29.4s`；watchdog 在多数派节点上按 validity 续约。

**时序（3 节点，挂 1 仍可用 / 挂 2 失败）**：
```
获取 k：node1 ✓  node2 ✓  node3 ✗(超时)   succ=2 >= 2(=3/2+1) → 持有，validity 内安全
再挂 1：node1 ✓  node2 ✗  node3 ✗          succ=1 <  2         → 释放 node1，ErrNotObtained
```

**固有短板**：fencing 的 `INCR` 必须**单一权威源**——多主各自 INCR 不单调，所以计数器只能固定落 `nodes[0]` 或外部 DB sequence，多主的可用性**不覆盖** fencing 源（ARCHITECTURE §3.3 标注）。安全（fencing）与可用性（quorum）在此没法同时最大化。

**为什么排最后（P5）**：它绑基础设施重构——需 ≥3 套互相独立的 Redis（当前仅 1 套）+ prometheus/grafana/告警/playbook 的部署铁律 14 项同步。没有真实消费者要求「抗单主故障」前，造它是无人用的复杂度。

---

## 维护清单

代码大改时同步本文：

- 改了 `client.go` / `lock.go` / `scripts/*.lua` → 重读并更新 §4 对应站的「设计与原理」段
- 改了 `options.go` 的参数语义 → 更新 §4.10 矩阵 + §5 决策表
- 加了新能力（读写锁/信号量/多主 quorum）→ 更新 §3 架构图 + §A 能力矩阵 + §B 实现状态
- 落地了 P5 多主 quorum → §B 状态改 done，§E 前瞻升级为 §4 正式走读站（补 file:line 锚点）
- 提交了工作区的 P3/P4 改动、行号漂移 → 更新头部「代码锚点」行的 commit hash + 各站 file:line
- 改了 prometheus 指标 → 更新 §4.11 + §7 诊断清单



