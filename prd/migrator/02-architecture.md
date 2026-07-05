# 数据迁移框架 — 架构设计

> 配套：`./01-product.md`（业务 PRD）/ `./04-cutover-playbook.md`（端到端实操 runbook）/ [`./runbooks/`](./runbooks/)（应急手册）/ [`./retros/`](./retros/)（演练记录）
> 服务：新增独立服务 `webook-migrator`（与 webook-core / webook-chat 并列）
> 风格：严格遵循 `C:\Go\work\CLAUDE.md` 的全局协作规则与服务拆分清单
> **本文档为权威定义**：术语 / 数字 / 阶段命名以本文档为准，PRD / playbook / runbooks 任何冲突回退到本文档

## 📌 v1 实现摘要

本文档为框架的**架构设计**（接口契约 / 状态机 / 表结构 / 端点矩阵），也是 v1 已交付能力的权威定义。下表为本文档与 v1 实现的关键对齐点：

| 维度 | v1 实现 |
|------|---------|
| `SourceFactory` 分发 | 按读取语义拆 `BuildFullSrc` / `BuildIncrSrc` / `BuildDst` 三方法（详见 [`adr/0002-source-factory-three-methods.md`](./adr/0002-source-factory-three-methods.md)） |
| `BinlogClient.Subscribe` | 签名 `Subscribe(ctx, fromPos)`，binlog file/pos 续订；不支持 GTID 模式（CanalSource 拒绝 gtid checkpoint） |
| DSN 解析 | `pipeline/dsn.StaticResolver` 注入（控制库自闭环，适合本机演示）；接口预留 `PerTaskResolver` 接 Vault/K8s Secret |
| 异构 verify dst | `BuildDst` 分发 mysql / es / mongo 三种 dst Source；CK / Kafka 作 dst 未实现 |
| 任意源 | `domain.SourceType`(mysql/mongo) + `pipeline/transform` Registry（Identity + `MongoToRelational`）；MongoSource 全量 find + 增量 Change Stream |
| 写请求幂等保护 | 不可逆操作（switch/repair）走 MySQL 唯一索引 + 状态机 + `IsRunning` 防双开（不用 Idempotency-Key header 中间件） |
| 权限模型 | JWT 已装配；RBAC scope 中间件 v1 不启用（接入 webook-core SSO 签发链路后挂回，详见 §11 权限设计） |

---

---

## 目录

- §1 整体架构
- §2 迁移驱动（Why）
- §3 不停机原理详解（How）
- §4 数据设计
- §5 切流状态机
- §6 HTTP 接口设计
- §7 分层与目录结构
- §8 核心接口签名
- §9 业务侧代码影响（对现在代码的实际改动）
- §10 前瞻性设计
- §11 权限设计
- §12 DI 变更（Wire）
- §13 服务拆分对照表（CLAUDE.md 14 项）
- §14 任务拆分
- §15 风险点
- §16 不在范围
- §17 关键决策（默认选项）

---

## 1. 整体架构

### 1.1 拓扑

```
                  ┌──────────────────┐
                  │  webook-migrator │ ← 新增独立服务（控制面）
                  │  Console API     │   启停 / 灰度 / 切流 / 对账 / 修复
                  └────────┬─────────┘
                           │
             ┌─────────────┼─────────────┐
             ▼             ▼             ▼
         ┌───────┐    ┌────────┐    ┌──────────┐
         │ FULL  │    │ INCR   │    │ VERIFY   │
         │ 全量  │    │ 增量   │    │ 对账     │
         └───┬───┘    └────┬───┘    └────┬─────┘
             │             │             │
             ▼             ▼             ▼
      Source(读) ─→ Pipeline(转换) ─→ Sink(写)

模式 A：应用层双写（业务侵入，强一致过渡）
   业务 Service ─┬→ MySQL_OLD（主）
                 └→ MySQL_NEW（副）

模式 B：binlog CDC（业务零侵入，最终一致）
   MySQL → Canal → IncrEngine（进程内 dispatcher 按 PK 分 partition）→ 多 Sink Adapter
                                     ├→ MySQL_NEW
                                     ├→ ES
                                     ├→ ClickHouse
                                     ├→ Mongo / TiDB / Doris / ...

四阶段切流：双写 → 全量 → 增量追平 → 对账切流（A/B 可叠加）
```

### 1.2 双模式叠加策略

| 模式 | 适用场景 | 优点 | 缺点 |
|---|---|---|---|
| **应用层双写** (`mode=dual_write`) | 同构 schema 演进 / 同库内拆表 | 强一致；无中间件依赖 | 业务侵入；需要补偿队列 |
| **CDC**（`mode=cdc`） | 跨机房 / 分库分表 / 异构 | 业务零侵入；可重放 | 最终一致；需 Canal 集群 |

**注意**：`mode` 是 task 配置字段（task 创建时确定迁移机制），与 `switch_stage`（运行时四阶段切流状态：`SRC_ONLY/SRC_FIRST/DST_FIRST/DST_ONLY`，见 §5.1）是不同维度。**不要混淆 `mode=dual_write` 与 `switch_stage=SRC_FIRST`**：前者描述迁移方式，后者描述切流当前到哪一阶段。

**强一致场景**（订单 / 支付）= 双写过渡期 + CDC 补偿历史，一致后切单写。两种模式不互斥，框架通过 `task.mode` 字段切换。

### 1.3 设计原则

- **控制面 / 数据面分离**：控制面 (Handler/Service) 在 webook-migrator 服务内；数据面 (Source/Sink Pipeline) 解耦，可独立扩展
- **Source / Sink 抽象**：所有读端实现 `Source`，所有写端实现 `Sink`；接入新 sink 引擎零改动
- **任务粒度**：以 `task` 为单位编排，task 内可挂多张表；不同 task 互相隔离
- **状态机驱动**：每个 task 有显式状态（created → full_running → ... → switched），所有操作必须遵循状态机转换约束
- **可重放**：全量分片 / 增量 binlog 位点都持久化；任意阶段崩溃恢复后续传
- **业务零侵入（默认）**：CDC 模式下业务无感；应用层双写时通过 SDK 包装 DAO，未启用时走 NoOp，不影响主流程

### 1.4 当前 webook 的"准迁移"现状

> 用户问：当前 webook 支持不停机迁移吗？

**结论：不支持**。`grep -rn 'migrat' webook/` 0 匹配，主仓没有任何迁移工具或框架。

主仓现有三处与"迁移"形似的机制，但都不算企业级迁移能力：

| 现状 | 实际定位 | 缺什么 |
|------|---------|--------|
| `internal/repository/dao/article_author.go` + `article_reader.go` 应用层同步双写 | **业务设计**（制作库 vs 线上库的发布流） | 无切流 / 灰度 / 回滚概念 |
| `internal/repository/dao/article_search.go` 同步 ES Upsert | **业务阻塞型异构同步** | 无异步 / 对账 / 补偿；ES 故障会拖垮文章发布主流程 |
| `internal/events/interaction/` Kafka 异步事件流 | **单向事件投递** | 无对账 / 回滚；不可作为通用 sink 框架 |

所以本方案目标 G3「零停机切流」是**从 0 到 1 新建**：现有业务代码零改动，独立 webook-migrator 服务承载。本节后面 §2 §3 解释驱动与原理，§9 给出真正接入业务时的代码影响清单。

---

## 2. 迁移驱动（为什么要做这套框架）

抽象目标见 PRD §1.2 G1-G6。本节讲**实际触发场景**——webook 在 1-2 年内可能遇到的真实问题。

### 2.1 实际触发场景

| 驱动 | 触发场景 | webook 中的对应表 |
|------|---------|-----------------|
| **跨机房 / 上云** | 自建 IDC → 阿里云 / 腾讯云；机房搬迁 | 全部 7 张业务表 |
| **容量瓶颈** | 单库 ≥ 800GB 备份打不动；单表 ≥ 5 亿行慢查询雪崩 | 主要是 `article.content`（BLOB 大字段）+ `interaction`（高写入） |
| **schema 演进** | 字段类型 / 长度不合理；旧字段拆分；亿级表 ALTER 锁表数小时 | `user.nickname VARCHAR(50)` 不够长；`article.status tinyint` 想改 varchar |
| **异构性能瓶颈** | 同步阻塞主流程；外部存储抖动拖垮业务 | `article_search.go` 同步 Upsert ES 是当前最大隐患 |
| **报表 / BI 需求** | OLTP 库直接跑分析卡死 | `interaction` / `ai_click_events` 需要导 ClickHouse |
| **降本** | 冷数据迁到对象存储 / 廉价存储 | 长尾 `article.content` 可以归档到 OSS |

### 2.2 不迁移的代价

| 场景 | 代价 |
|------|------|
| 直接跑亿级 `ALTER TABLE` | **业务停服 N 小时**（MySQL 8 在线 DDL 仍有元数据锁） |
| 直接 dump+restore 整库 | 停服 + 业务无 binlog 追平能力，迁完发现历史 N 分钟丢了 |
| 让 ES 故障继续拖垮文章发布 | 文章发布 P99 秒级抖动 → 用户流失 |
| 同步异构 sink 越加越多 | 每加一个外部存储业务 RT 增加 / 故障面增加 |

### 2.3 webook 当前的潜在迁移点

按现有代码挑出的"近期一定会撞上"的迁移场景，按风险倒序：

| 优先级 | 场景 | 类型 | 触发条件 |
|------|------|------|---------|
| P0 | `article_search.go` 同步 → 异步 CDC | 异构 / 性能 | 当前已是性能隐患，第一个落地（详见 §9.2） |
| P1 | `interaction` → ClickHouse 报表同步 | 异构 / OLAP | 后台报表慢查询时触发 |
| P1 | `article.content` BLOB 归档 OSS | 同构 / 容量 | 单库 > 500GB 触发 |
| P2 | `user` 表 schema 演进（nickname / phone 拆分） | 同构 / schema | 业务字段不够用时触发（详见 §9.3） |
| P2 | `interaction` / `user_interaction` 分库分表 | 同构 / 容量 | 单表 > 1 亿行触发 |
| P3 | 跨机房 / 上云 | 同构 / IDC | 业务规模触发或战略决定 |

---

## 3. 不停机原理详解（怎么做到零停机）

### 3.1 核心思想

不停机迁移的本质是「**写双份 + 读切换**」：让两套存储在切流前**先达到数据等价**，然后切读 → 切写 → 关旧库。中间任何一步都可逆。

四个阶段串起来：

```
[1] 双写打平                     [2] 全量回填
   业务写 OLD+NEW                  历史按 ID 分 16 片并行 scan
   OLD 必成 / NEW 失败 retry       幂等 INSERT ... ON DUPLICATE KEY
        │                                │
        └───────────┬────────────────────┘
                    ▼
            [3] 增量追平 (CDC)
   Canal 订阅 OLD binlog → IncrEngine 进程内按 PK 分 partition → 多 worker 写 NEW
                    │
                    ▼
            [4] 灰度切流
   gray 0% → 50% → 100% 按 user_id hash 分流
            │ verify pass
            ▼
   cutover: 写也切 NEW；OLD 转只读（DST_ONLY 不可逆，status=switched）
            │ 充分对账确认后运维手动下线 OLD（收尾）
            ▼
      OLD 下线，迁移完成
```

### 3.2 阶段 1：双写打平

**目标**：让从这一刻开始的所有新数据，在 OLD 和 NEW 都有。

**实现**：业务层（或 SDK 层）拦截写操作，先写 OLD（必成功），再写 NEW（失败 retry）：

```go
// 伪代码
func DualWrite(ctx context.Context, fn func(side Side) error) error {
    if err := fn(SideOld); err != nil {
        return err  // OLD 失败立即回滚，业务可见
    }
    go func() {
        for i := 0; i < 3; i++ {
            if err := fn(SideNew); err == nil { return }
            time.Sleep(backoff(i))
        }
        deadLetter.Send(ctx, fn)  // 三次失败入死信队列，告警
    }()
    return nil
}
```

**为什么 OLD 必成 / NEW 异步**：业务可见性必须保证 OLD 一致；NEW 是迁移目标，最终一致即可。如果 OLD-NEW 一起强一致（同步双写），任何一边抖动都会拖垮业务。

**适用模式**：`mode=dual_write`（同构 schema 演进 / 同库内拆表 / 强一致过渡）。CDC 模式（异构）跳过此阶段，业务零侵入。

### 3.3 阶段 2：全量回填

**目标**：把双写之前的历史数据，从 OLD 全量拉到 NEW。

**实现**：按 `id` 范围分 N 片（默认 16），多 worker 并行 scan：

```sql
-- 每个 worker 处理一个不重叠的 id 区间
SELECT * FROM article WHERE id BETWEEN ? AND ? AND id > ? ORDER BY id ASC LIMIT 1000;
-- 写入 NEW（幂等 upsert，可重放）：
INSERT INTO article (...) VALUES (...) ON DUPLICATE KEY UPDATE ...;
```

**关键点**：
1. **走 ReadReplica**：避免拖垮主库
2. **限速**：默认 10k qps/shard 可调，监控源库负载
3. **断点续传**：每写完一批 update `checkpoint.cursor_value = last_id`，崩溃恢复后从该值续
4. **幂等 upsert**：`ON DUPLICATE KEY UPDATE` 可重放，全量崩溃重启不会重复写
5. **不拷软删除**：`WHERE deleted_at = 0`，被软删数据不迁

**亿级吞吐**：16 worker × 5k qps = 80k row/s ≈ 12h 跑完亿级表。单表更大走 32 / 64 worker。

### 3.4 阶段 3：增量追平（CDC）

**目标**：双写之后业务持续写 OLD，需要让 NEW 实时跟上。

**实现**：
1. Canal 订阅 OLD 的 binlog，输出 ChangeEvent
2. IncrEngine 进程内 dispatcher 按 `FNV(PK) mod N` 路由到 N 个 partChan，保证同一行的所有变更落在同一 partition（天然有序）
3. N 个 incr worker 并行消费各自 partChan，每个 worker 处理一个 partition
4. worker 调 Sink 攒批 1000 条 / 100ms flush 写到 NEW
5. checkpoint 持久化 binlog pos / GTID，断点续传

**为什么按 PK 分区**：保证同一条记录的多次更新顺序一致——如果 t1=update→t2=delete 错位变成 t1=delete→t2=update，数据就错了。

**lag 计算**：`lag = now() - binlog_event_ts`，不是 `now() - worker_processed_ts`（后者 worker 卡住也算不出来）。

### 3.5 阶段 4：灰度切流

四步，每步都可回滚（详细的 switch_stage / API 入参枚举见 §5.1 / §6）：

| 阶段（switch_stage + gray%）| 切读 / 切写 | 风险 | 回滚成本 |
|----------------------------|------------|------|---------|
| `SRC_FIRST` / `gray=0` | 全部读 OLD；双写 OLD 必成 + NEW 异步 retry | 0 | 0 |
| `SRC_FIRST` / `gray=N%` | 按 `hash(user_id) % 100 < N` 分流 | 部分用户读 NEW，需保证 NEW 已追平 | 改回 0 即可 |
| `SRC_FIRST` / `gray=100` | 全部读 NEW，OLD 仍接受写（仍双写） | 全量读 NEW，业务感知 NEW 性能 | 改回 0 / 50 即可 |
| `DST_FIRST` / 30s 过渡 | 全部读 NEW；SRC + DST 同步双写（避免空窗）| 写口切换瞬间 | 启动 `rollback` 回退到 SRC_FIRST |
| `DST_ONLY`（切流完成）| 全部读写 NEW；OLD 转只读、停止更新 | **point of no return**，OLD 停滞 | 不可回滚（故切前必充分对账） |
| `DST_ONLY` 收尾 | 全部读写 NEW；OLD 充分确认后手动下线 | 任务收工（v1 无 closed 状态）| 不可回滚 |

**hash 分流为什么用 user_id**：保证同一用户始终命中同侧——避免「写完 OLD 还没同步到 NEW，下一秒读切到 NEW，看不到自己的写」（read-your-write 破坏）。如果业务键不是 user_id，用 `biz_id` / `entity_id` 一样的逻辑。

### 3.6 五个保证

| 保证 | 实现 |
|------|------|
| **幂等** | Sink Apply 用 `INSERT ... ON DUPLICATE KEY UPDATE` + Version 乐观锁，崩溃重启可重放不重复写 |
| **顺序** | dispatcher 按 PK FNV-hash 路由，同一行所有变更必到同一进程内 partition |
| **一致** | 阶段 1 双写打平 + 阶段 2 全量 + 阶段 3 增量 = 三层兜底，缺一不可 |
| **可重放** | checkpoint 持久化（id_range / binlog_pos / GTID），任何阶段崩溃恢复后续传 |
| **可回滚** | 双写期（SRC_FIRST / DST_FIRST）gray 调 0 / rollback 回 SRC_FIRST；DST_ONLY 单写后不可逆 |

### 3.7 三大坑及解法

### 坑 1：旧 binlog 覆盖新值

**场景**：业务先写 OLD(t=1)，再写 NEW(t=2)；CDC 把 OLD 的 t=1 binlog 同步到 NEW，**把 t=2 覆盖**了。

```
T0  双写: OLD.update(version=1, ts=1000)  ✅
T1  双写: NEW.update(version=2, ts=1100)  ✅ NEW 现在是 v=2
T2  CDC:  把 OLD 的 t=1 binlog 同步到 NEW  ❌ NEW 退化到 v=1
```

**解**：Sink Apply 时加乐观锁条件——

```sql
UPDATE NEW.article
SET ...
WHERE id = ? AND updated_at <= ?  -- ? 是 binlog 事件的 updated_at
```

或用 version 列：`WHERE version < incoming_version`。任意一种保证「新值不会被旧 binlog 覆盖」。

### 坑 2：read-your-write 破坏（切读时）

**场景**：用户 A 在 gray=50% 时写完 OLD，下一秒读切到 NEW，**NEW 还没收到这条更新**（CDC 有 lag）。用户看到自己的更新消失。

**解**：按 `hash(user_id) % 100 < gray%` 路由——

- 如果用户 A 的 hash 落在 < 50 区间，他读和写都走 OLD（一致）
- 如果落在 ≥ 50 区间，他读和写都走 NEW（一致；但要求 NEW 双写已开启）

关键：**同一用户始终命中同侧**，不会「写一侧读另一侧」。

### 坑 3：切写瞬间的写入空窗

**场景**：cutover 瞬间从「写 OLD」改成「写 NEW」，如果切换不是原子的，可能 t=1 写到 OLD，t=2 写到 NEW，**OLD 的 t=1 没同步到 NEW 就丢了**。

**解**：cutover 期间**短暂双写过渡**——

```
cutover 前    : read NEW  / write OLD（CDC 同步到 NEW）
cutover 中(N秒): read NEW / write OLD+NEW（短暂双写打平）
cutover 后    : read NEW  / write NEW （DST_ONLY 单写，OLD 转只读）
```

打平 N 秒后再切单写，保证不丢。N 取 binlog 同步 lag 的 P99 × 2（默认 30s）。

---

## 4. 数据设计

### 4.1 控制库（独立 `webook_migrator`）

不污染业务库。新建独立 schema，DBA 单独授权。

### 4.1.1 `task` — 迁移任务定义

```sql
CREATE TABLE `task` (
  `id`              bigint       NOT NULL AUTO_INCREMENT COMMENT '主键',
  `name`            varchar(128) NOT NULL COMMENT '任务名（业务可读，全局唯一）',
  `mode`            varchar(16)  NOT NULL COMMENT '模式：dual_write=应用层双写 / cdc=binlog 通道',
  `kind`            varchar(32)  NOT NULL COMMENT '类型：cross_dc=跨机房 / sharding=分库分表 / schema=schema 演进 / heterogeneous=异构',
  `source_dsn_ref`  varchar(64)  NOT NULL COMMENT '源 DSN 引用（Vault/Secret 路径，禁止明文入库）',
  `sink_type`       varchar(32)  NOT NULL COMMENT '目标类型：mysql / es / clickhouse / mongo / tidb / kafka',
  `sink_dsn_ref`    varchar(64)  NOT NULL COMMENT '目标 DSN 引用',
  `tables_json`     text         NOT NULL COMMENT '涉及表 JSON：[{src,dst,partitionKey,filter,transform}]',
  `status`          tinyint      NOT NULL DEFAULT 0 COMMENT '状态：0=created 1=full_running 2=full_done 3=incr_running 5=switched -1=failed',
  `gray_percent`    smallint     NOT NULL DEFAULT 0 COMMENT '灰度切读百分比 0-100',
  `consistency`     varchar(16)  NOT NULL DEFAULT 'eventual' COMMENT '一致性等级：eventual / read_after_write / strong',
  `created_at`      bigint       NOT NULL DEFAULT 0 COMMENT '创建时间（Unix 毫秒）',
  `updated_at`      bigint       NOT NULL DEFAULT 0 COMMENT '更新时间（Unix 毫秒）',
  `deleted_at`      bigint       NOT NULL DEFAULT 0 COMMENT '软删除（0=未删）',
  PRIMARY KEY (`id`) USING BTREE,
  UNIQUE INDEX `uni_task_name` (`name`),
  INDEX `idx_task_status` (`status`)
) ENGINE=InnoDB CHARACTER SET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci ROW_FORMAT=Dynamic COMMENT='迁移任务定义';
```

### 4.1.2 `checkpoint` — 断点续传

```sql
CREATE TABLE `checkpoint` (
  `id`               bigint       NOT NULL AUTO_INCREMENT COMMENT '主键',
  `task_id`          bigint       NOT NULL COMMENT '所属任务',
  `phase`            varchar(16)  NOT NULL COMMENT '阶段：full / incr',
  `shard_no`         int          NOT NULL DEFAULT 0 COMMENT '分片编号（全量按 ID 分片，增量恒为 0）',
  `cursor_kind`      varchar(16)  NOT NULL COMMENT '游标类型：id_range / binlog_pos / gtid',
  `cursor_value`     varchar(256) NOT NULL DEFAULT '' COMMENT '游标值 JSON',
  `progress_percent` decimal(5,2) NOT NULL DEFAULT 0 COMMENT '进度 0-100',
  `last_lag_ms`      bigint       NOT NULL DEFAULT 0 COMMENT '最近一次同步延迟（毫秒）',
  `version`          bigint       NOT NULL DEFAULT 0 COMMENT '乐观锁版本号',
  `updated_at`       bigint       NOT NULL DEFAULT 0 COMMENT '更新时间（Unix 毫秒）',
  PRIMARY KEY (`id`) USING BTREE,
  UNIQUE INDEX `uk_checkpoint_task_phase_shard` (`task_id`,`phase`,`shard_no`)
) ENGINE=InnoDB CHARACTER SET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci ROW_FORMAT=Dynamic COMMENT='迁移断点：全量分片游标 + 增量 binlog 位点';
```

### 4.1.3 `validate_log` — 对账日志（仅记差异）

```sql
CREATE TABLE `validate_log` (
  `id`            bigint       NOT NULL AUTO_INCREMENT COMMENT '主键',
  `task_id`       bigint       NOT NULL COMMENT '所属任务',
  `direction`     varchar(16)  NOT NULL COMMENT '方向：src_to_dst / dst_to_src',
  `table_name`    varchar(64)  NOT NULL COMMENT '表名',
  `biz_id`        bigint       NOT NULL COMMENT '业务主键',
  `mismatch_kind` varchar(32)  NOT NULL COMMENT '差异：missing / extra / diff',
  `diff_detail`   text                  COMMENT 'JSON 差异（敏感字段 mask）',
  `repaired`      tinyint      NOT NULL DEFAULT 0 COMMENT '是否已修复 0=否 1=是',
  `created_at`    bigint       NOT NULL DEFAULT 0 COMMENT '创建时间（Unix 毫秒）',
  `repaired_at`   bigint       NOT NULL DEFAULT 0 COMMENT '修复时间（Unix 毫秒）',
  PRIMARY KEY (`id`) USING BTREE,
  INDEX `idx_validate_log_task_repaired` (`task_id`,`repaired`),
  INDEX `idx_validate_log_table_biz` (`table_name`,`biz_id`)
) ENGINE=InnoDB CHARACTER SET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci ROW_FORMAT=Dynamic COMMENT='迁移对账日志：仅记录差异';
```

### 4.1.4 `audit_log` — 操作审计（合规留存 1 年）

```sql
CREATE TABLE `audit_log` (
  `id`         bigint        NOT NULL AUTO_INCREMENT COMMENT '主键',
  `task_id`    bigint        NOT NULL COMMENT '所属任务',
  `actor`      varchar(64)   NOT NULL COMMENT '操作者（用户名 / service account）',
  `action`     varchar(32)   NOT NULL COMMENT '动作（扁平描述字符串）：create/start/pause/set_gray/set_stage_SRC_FIRST/cutover_propose/cutover_approve/rollback/repair',
  `payload`    text                   COMMENT 'JSON 入参（敏感字段 mask）',
  `result`     varchar(16)   NOT NULL COMMENT 'success / fail',
  `error_msg`  varchar(512)           COMMENT '失败原因',
  `client_ip`  varchar(64)            COMMENT '客户端 IP',
  `created_at` bigint        NOT NULL DEFAULT 0 COMMENT '操作时间（Unix 毫秒）',
  PRIMARY KEY (`id`) USING BTREE,
  INDEX `idx_audit_log_task_created` (`task_id`,`created_at`)
) ENGINE=InnoDB CHARACTER SET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci ROW_FORMAT=Dynamic COMMENT='迁移操作审计';
```

### 4.1.5 `dead_letter` — 死信队列（双写失败兜底）

```sql
CREATE TABLE `dead_letter` (
  `id`            bigint        NOT NULL AUTO_INCREMENT COMMENT '主键',
  `task_id`       bigint        NOT NULL                COMMENT '所属任务',
  `op`            varchar(16)   NOT NULL                COMMENT '操作：insert / update / delete',
  `table_name`    varchar(64)   NOT NULL                COMMENT '目标表名',
  `biz_id`        bigint        NOT NULL                COMMENT '业务主键',
  `payload`       text          NOT NULL                COMMENT 'JSON：完整 mutation 数据',
  `last_error`    varchar(1024)                         COMMENT '最后一次失败原因',
  `retry_count`   int           NOT NULL DEFAULT 0      COMMENT '已重试次数',
  `replayed`      tinyint       NOT NULL DEFAULT 0      COMMENT '是否已重放成功 0=否 1=是',
  `replay_failed` tinyint       NOT NULL DEFAULT 0      COMMENT '重放是否仍失败 0=否 1=是（需人工）',
  `created_at`    bigint        NOT NULL DEFAULT 0      COMMENT '入死信时间（Unix 毫秒）',
  `replayed_at`   bigint        NOT NULL DEFAULT 0      COMMENT '重放时间（Unix 毫秒）',
  PRIMARY KEY (`id`) USING BTREE,
  INDEX `idx_dead_letter_task_replayed` (`task_id`, `replayed`),
  INDEX `idx_dead_letter_table_biz`     (`table_name`, `biz_id`),
  INDEX `idx_dead_letter_created`       (`created_at`)
) ENGINE=InnoDB CHARACTER SET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci ROW_FORMAT=Dynamic COMMENT='死信队列：双写失败兜底';
```

用途：`switch_stage=SRC_FIRST` 时 SDK 双写失败（SRC 必成、DST retry 3 次仍败）会写入此表；`POST /tasks/:id/replay-dl` 批量重放。`replay_failed=1` 的需 DBA 人工处理。

### 4.2 缓存设计（统一前缀 `migrator:`，SDK ↔ migrator 服务共享 key 用 taskName 命名）

> v1 实际落地。SDK 内部 key（`internal/migratorsdk/redis.go`）与 migrator 服务侧 `SwitchStateCache` / `ThrottleCache`（经 SwitchStateRepository / ThrottleRepository 供 service 层使用）必须**写读同一 key**——SDK 业务侧只从 yaml 拿 `migrator.sdk.taskName`，不知 taskId，因此 stage/gray 类共享 key 都按 taskName。

| Key 模式 | 值 | TTL | 用途 | v1 状态 |
|---|---|---|---|---|
| `migrator:stage:<taskName>` | `SRC_ONLY \| SRC_FIRST \| DST_FIRST \| DST_ONLY` | 永久 | **switch_stage**：四阶段切流状态；SDK + 服务双向读写源 | ✅ |
| `migrator:gray:<taskName>` | `0-100` | 永久 | 灰度切读比例（控制 `SwitchReader` 路由） | ✅ |
| `migrator:cutover_propose:<taskName>` | `string actor` | 10 分钟 | 双人复核 propose actor，approve 时验证 + 删除 | ✅ |
| `migrator:throttle:<taskId>` | qps_limit JSON | 永久 | 动态调速（仅 migrator 内部，FullEngine `Start` 时读，与 SDK 不共享，taskId 命名即可） | ✅ |

业务库自身的 cache key 不变。SDK 注入的 `SwitchReader` 在 read 路径上读 stage/gray 决定走哪侧；缓存 miss 时降级"全部读 source"作为 safe 默认。

v1 各 Key 前缀常量定义：
- 服务端：`webook/migrator/service/switching/switch.go::{KeyStage,KeyGray,KeyCutoverPropose}` + `webook/migrator/repository/cache/throttle.go::RedisThrottleKeyPrefix`
- SDK 端：`webook/internal/migratorsdk/redis.go::{keyStage,keyGray}`

**持久化兜底**：以上 `永久` TTL 的 key 依赖 Redis AOF / RDB 持久化。**控制库 `task` 表是真值源**，Redis 仅做性能缓存：webook-migrator 启动时从控制库回填 gray / switch 两类 key（启动时 reconciler，单次 batch）；运行时如 Redis 重启丢 key，SDK 读 miss 降级到 safe 默认（SideOld），同时触发回填。

### 4.3 对账数据策略（亿级）

| 模式 | 触发 | 实现 |
|------|------|------|
| 采样对账 | API 触发 | 按 `sampleRate` 随机选 ID 范围，源 / 目标各一查比对 |
| 全量对账 | T+1 / API 触发 | 框架不直接做亿级 JOIN；**外接 Spark/Flink job dump+hash 比对**，结果 sink 回 `validate_log` |

亿级对账绝对不能反复 SQL JOIN，否则会拖垮源库。

---

## 5. 切流状态机

```
task.status        switch_stage     gray%
─────────          ────────────     ─────
created            SRC_ONLY         0
   │ POST /start {phase:full}
   ▼
full_running       SRC_ONLY         0
   │
   ▼
full_done          SRC_ONLY         0
   │ POST /start {phase:incr}
   ▼
incr_running       SRC_ONLY         0
   │ POST /switch {stage:SRC_FIRST}
   ▼
incr_running       SRC_FIRST        0 ◄──────────────┐
   │ POST /gray N%                                    │
   ▼                                                  │
incr_running       SRC_FIRST       N%                 │
   │ POST /gray 100                                   │
   ▼                                                  │ POST /switch {stage:SRC_FIRST, action:rollback}
incr_running       SRC_FIRST      100                 │
   │ verify pass                                      │
   │ POST /switch {stage:DST_ONLY, action:propose}    │
   │ POST /switch {stage:DST_ONLY, action:approve}    │
   ▼                                                  │
switched           DST_FIRST      100                 │
   │ 30s 自动过渡                                      │
   ▼                                                  │
switched           DST_ONLY       100 ────────────────┘
   │ 观察期（监控 + 对账；DST_ONLY 已不可逆）
   ▼
（观察期满 → 运维手动下线 OLD；v1 无 closed 状态/API，task 停在 switched）
```

### 5.1 四阶段切流（switch_stage）+ 读路由（gray% 派生）

**switch_stage 是行业标准四阶段命名**（DTS / gh-ost / DataX 系出此谱）。switch_stage 控制读写主从关系，gray% 控制读分流比例：

| switch_stage | 含义 | 读策略 | 写策略 | 可回滚 |
|--------------|------|--------|--------|---------|
| **`SRC_ONLY`**（默认）| task 创建初始态；读写都在源 | 全读 SRC | 单写 SRC | — |
| **`SRC_FIRST`** | 双写过渡，以源为主；切读由 gray% 控制 | 按 `hash(user_id) % 100 < gray%` 分流 | 双写：SRC 必成 / DST 异步 retry | ✅ gray 调 0 / rollback |
| **`DST_FIRST`** | cutover 启动后的 30 秒过渡期；为避免空窗保持同步双写 | 全读 DST | SRC + DST 同步双写（两侧都必成） | ✅ rollback 回 SRC_FIRST |
| **`DST_ONLY`** | 切流完成；单写目标（**不可逆终态**）| 全读 DST | 单写 DST | ❌ OLD 停滞，point of no return |

**关键澄清**：

- **`rollback`** 不是 switch_stage 值，是 API 指令；rollback 触发后系统把 switch_stage 落到 `SRC_FIRST`
- **`propose / approve`** 是 API 入参 action（需配合 `stage=DST_ONLY` 使用），不是 switch_stage 值；双人审批后系统执行 `SRC_FIRST → DST_FIRST → (30s) → DST_ONLY`
- **`task.status`** 与 switch_stage 是两个维度：v1 终态是 `switched`（进入 `DST_ONLY` 时置位）；设计中的 `closed`（观察期满收工）v1 未实现
- **`mode=dual_write`** 是 task 配置字段（应用层双写 vs CDC 通道），与 switch_stage 也是两个维度

### 5.2 状态转换约束

- 不允许跳级（如 `SRC_ONLY` 直接到 `DST_ONLY`；必须经 `SRC_FIRST → DST_FIRST → DST_ONLY` 顺序）
- 进入 `DST_FIRST`（即 `{stage:DST_ONLY, action:approve}`）前必须 `verify` 通过且 mismatch_rate < 0.001%（详见 `cutover-checklist.md`）
- `{action: rollback}` 仅在 `SRC_FIRST` / `DST_FIRST`（双写期，OLD 有全量数据）可触发，幂等；落库后 switch_stage = `SRC_FIRST`。`DST_ONLY` 单写后 OLD 停滞，**不可 rollback**
- 进入 `DST_ONLY`（status=`switched`）后 stage 不再变化；观察期满下线 OLD 为运维手动步骤（v1 无 `closed` 状态 / `{action:closed}` API / DELETE 端点）

---

## 6. HTTP 接口设计

服务端口：**8030**（service-port，与 webook-core / webook-chat 隔离）。URL 前缀：`/api/migrator/`。

| Method | Path | 请求 | 响应 | 认证 |
|--------|------|---------|------|------|
| POST | `/preflight` | `{sourceDsnRef, tables[]}` | `{code, msg, data:{binlog_format, gtid_mode, tables_with_pk, read_replica_lag, ready: bool}}` | RBAC `migrator:read` |
| POST | `/tasks` | `{name, mode, kind, sourceDsnRef, sinkType, sinkDsnRef, tables[]}` | `{code, msg, data:{taskId}}` | Admin JWT + RBAC `migrator:write` |
| GET | `/tasks` | `?status=&offset=&limit=` | `{code, msg, data:{list, total}}` | RBAC `migrator:read` |
| GET | `/tasks/:id` | path | `{code, msg, data:{task, checkpoints[]}}` | RBAC `migrator:read` |
| **DELETE** *(v1 未实现)* | `/tasks/:id` | path | `{code, msg}` | 设计：仅允许 `task.status=closed` 的 task 软删（设置 `deleted_at`）。**v1 无此端点**（无 `closed` 状态，task 停在 `switched`） |
| POST | `/tasks/:id/start` | `{phase: full\|incr}` | `{code, msg}` | RBAC `migrator:write` |
| POST | `/tasks/:id/pause` | path | `{code, msg}` | RBAC `migrator:write` |
| POST | `/tasks/:id/throttle` | `{qps?: int, shard_workers?: int}` | `{code, msg, data:{applied_on:"next_start"}}` | RBAC `migrator:write` **限速：TaskService.SetThrottle 经 ThrottleRepository 写 Redis，下次 `start` 回读生效（**非实时调速**）；默认 qps=10000, shard_workers=16；最小 qps=100；throttle 存储未装配返 501** |
| POST | `/tasks/:id/gray` | `{percent: 0-100}` | `{code, msg}` | RBAC `migrator:write` |
| POST | `/tasks/:id/switch` | `{stage: SRC_FIRST\|DST_ONLY, action?: propose\|approve\|rollback}` | `{code, msg}` | **RBAC `migrator:switch`；`stage=DST_ONLY,action=approve` 必须双人复核**；`action=propose` 时 body 带 `propose` 发起人 actor，`action=approve` 时 body 带 `approve` 复核人（必须与 propose 不同 actor；服务端校验） |
| GET | `/tasks/:id/lag` | path | `{code, msg, data:{lagMs, srcLagMs, dstLagMs}}` | RBAC `migrator:read` |
| POST | `/tasks/:id/verify` | `{mode: sample\|full, sampleRate}` | `{code, msg, data:{mismatchCount}}` | RBAC `migrator:write` |
| GET | `/tasks/:id/mismatch` | `?repaired=&offset=&limit=` | `{code, msg, data:{list, total}}` | RBAC `migrator:read` |
| POST | `/tasks/:id/repair` | `{strategy: src_overwrite_dst\|dst_overwrite_src\|mark_only, ids[]}` | `{code, msg}` | **RBAC `migrator:repair` + DBA 审批** |
| POST | `/tasks/:id/replay-dl` | `{limit?: int}` | `{code, msg, data:{replayed, failed}}` | RBAC `migrator:write` **批量重放死信队列（默认每次 1000 条）；replay_failed=1 的需人工处理** |

> **v1 认证现状**：仅强制 JWT（`server.http.jwt.disabled` 可关）+ IP 限流；上表 `RBAC migrator:*` scope 为权限设计意图，v1 未挂 scope 校验中间件（接入 webook-core SSO 后挂回，见 §11）。

**repair `strategy` 取值定义**：
- `src_overwrite_dst`：用源数据覆盖目标（常用，源是真值时）
- `dst_overwrite_src`：反向覆盖（少见，仅在 cutover 后期 NEW 是新真值时）
- `mark_only`：不实际修改任一侧，仅把 `validate_log.repaired` 置 1（用于"差异合法、人工已离线处理"的场景；审计字段必填理由）

**写幂等**：不可逆操作（switch / repair）靠 MySQL 唯一索引 + 状态机 + `IsRunning` 防双开；v1 不挂 `Idempotency-Key` 中间件。

**API 入参 → switch_stage 落库映射**（API 指令是动作维度，switch_stage 是状态维度）：

| API 入参 | 落库 switch_stage | 说明 |
|---------|------------------|------|
| `{stage: SRC_FIRST}` | `SRC_ONLY → SRC_FIRST` | 启动业务双写过渡（前置：task.status=incr_running，全量已完成） |
| `{stage: DST_ONLY, action: propose}` | （不变） | 操作人 A 申请最终切流，等待 B |
| `{stage: DST_ONLY, action: approve}` | `SRC_FIRST → DST_FIRST → (30s) → DST_ONLY` | 操作人 B 批准（与 A 不同人），系统执行切流 |
| `{stage: SRC_FIRST, action: rollback}` | 双写期(SRC_FIRST/DST_FIRST) → `SRC_FIRST` | 应急回退；DST_ONLY 单写不可逆、SRC_ONLY 未进双写均拒绝 |

读路由不通过 stage 控制，由 `POST /gray {percent}` 调整 `gray_percent`。详见 §5.1。

**统一响应**：`Result{Code int, Msg string, Data any}`（与 webook-core / webook-chat 一致）。

**认证**：`x-access-token` 请求；`x-refresh-token` 响应；前端协议复用 webook 现有 JWT 双 Token 模式。

### 6.1 错误码

| HTTP | code | 用户消息 |
|------|------|---------|
| 400 | `MIGRATOR_VALIDATION_FAILED` | 请检查参数 |
| 401 | `UNAUTHORIZED` | 请先登录 |
| 403 | `MIGRATOR_FORBIDDEN` | 无权限操作迁移任务 |
| 404 | `MIGRATOR_TASK_NOT_FOUND` | 迁移任务不存在 |
| 409 | `MIGRATOR_STATE_CONFLICT` | 任务状态冲突，无法执行此操作 |
| 409 | `MIGRATOR_VERIFY_FAILED` | 对账未通过，请先修复差异再切流 |
| 409 | `MIGRATOR_CUTOVER_PRECONDITION_FAILED` | cutover 前置条件未满足（详见 cutover-checklist.md） |
| 409 | `MIGRATOR_APPROVAL_SAME_ACTOR` | 双人复核失败：approve 与 propose 的 actor 相同 |
| 429 | `RATE_LIMITED` | 操作过于频繁 |
| 500 | `MIGRATOR_INTERNAL` | 迁移服务内部错误，请联系管理员 |
| 502 | `MIGRATOR_UPSTREAM_DOWN` | 源/目标存储不可达，请检查链路 |

PRD §7.2 的错误码列表以此为权威；任何冲突回退到本表。

---

## 7. 分层与目录结构

按 CLAUDE.md 严格分层（Handler → Service → Repository → DAO/Cache）+ 服务拆分清单 14 项。

**布局原则**：与 `webook/chat/` 平级，复用 webook 主仓 `go.mod`（module `github.com/webook`），可直接 import `github.com/webook/pkg/saramax`、`github.com/webook/pkg/gormx`、`github.com/webook/pkg/logger` 等已有工具。**不重复独立 go.mod**。

```
webook/                                     ← 主仓（module github.com/webook）
├── chat/                                   ← 已有：聊天服务（与 migrator 平级）
├── internal/                               ← 已有：webook-core
├── pkg/                                    ← 可直接复用：saramax / gormx / logger / ratelimit / ginx / ...
└── migrator/                          ★    ← 新增：迁移服务（与 chat 平级）
    ├── web/                                # Handler 层
    │   ├── task.go                         # CRUD
    │   ├── operation.go                    # start/pause/gray/switch/verify/repair
    │   └── jwt/                            # 复用 webook 的 JwtHandler 模式
    ├── service/
    │   ├── task.go                         # 任务编排
    │   ├── full/                           # 全量同步引擎
    │   ├── incr/                           # 增量同步引擎
    │   ├── verify/                         # 对账引擎
    │   └── switching/                      # 切流引擎（switch 是 Go 关键字）
    ├── repository/
    │   ├── task.go                         # cache + dao 协调
    │   ├── checkpoint.go
    │   ├── validate.go
    │   ├── audit.go
    │   ├── dao/                            # 仅访问 webook_migrator 控制库
    │   └── cache/                          # gray/switch/lag/lock/dedup
    ├── domain/                             # 任务模型 + 切流状态机 + Mutation
    ├── pipeline/                           # 通用 Source/Sink/Transform 抽象
    │   ├── source/
    │   │   ├── mysql.go                    # 全量分片 SELECT
    │   │   └── canal.go                    # 增量 binlog 订阅
    │   ├── sink/
    │   │   ├── mysql.go
    │   │   ├── es.go                       # 复用 article_search.go 的 client
    │   │   ├── clickhouse.go
    │   │   ├── mongo.go
    │   │   ├── tidb.go
    │   │   └── kafka.go                    # KafkaSink：sarama SyncProducer，key=PK 保序
    │   └── transform/                      # 字段映射 / 类型转换 / 软删除归一 / 脱敏
    ├── consts/                             # cache key / 错误码 / 状态机常量
    ├── errs/                               # sentinel errors
    ├── ioc/                                # Wire Provider
    ├── config/                             # 五份 yaml（local/dev/staging/prod/test）
    ├── scripts/migrator.sql                # 控制库 schema
    ├── integration/                        # 集成测试（参考 webook/chat/integration）
    ├── wire.go
    ├── wire_gen.go
    ├── Dockerfile
    ├── Makefile
    ├── CLAUDE.md
    └── main.go

# 业务侧 SDK（webook-core / webook-chat 在 schema 演进时引用）
webook/internal/migratorsdk/
├── dual_writer.go                          # 双写包装 DAO
├── switch_reader.go                        # 灰度切读
├── noop.go                                 # 默认 NoOp，未启用时业务零感知
└── consts.go                               # 与控制库共享的 Redis key 模式
```

**import 路径示例**（与 chat 一致）：
- `github.com/webook/migrator/domain`
- `github.com/webook/migrator/repository/dao`
- `github.com/webook/pkg/saramax`（复用）
- `github.com/webook/pkg/logger`（复用）

---

## 8. 核心接口签名

> 设计要点：Source / Sink 解耦是企业级关键，所有引擎依赖接口而非具体实现。

### 8.1 pipeline.Source — 所有读端

```go
// webook/migrator/pipeline/source/source.go
package source

type Source interface {
    FullScan(ctx context.Context, shard ShardSpec, out chan<- Row) error
    IncrSubscribe(ctx context.Context, ckpt domain.Checkpoint, out chan<- ChangeEvent) error
    SaveCheckpoint(ctx context.Context, ckpt domain.Checkpoint) error
    Close() error
}

type ShardSpec struct {
    No       int
    PKMin    int64
    PKMax    int64
    BatchSz  int
    QPSLimit int
}

type Row struct {
    Table string
    PK    int64
    Cols  map[string]any
}

type ChangeEvent struct {
    Op        string  // insert / update / delete
    Table     string
    PK        int64
    Before    map[string]any  // update / delete 时填
    After     map[string]any  // insert / update 时填
    BinlogPos string
    GTID      string
    EventTs   int64           // binlog 事件时间（毫秒），用于 lag 计算
}
```

### 8.2 pipeline.Sink — 所有写端

```go
// webook/migrator/pipeline/sink/sink.go
package sink

type Sink interface {
    Apply(ctx context.Context, batch []Mutation) error  // 实现侧负责幂等
    Close() error
}

type Mutation struct {
    Op      string
    Table   string
    PK      int64
    Cols    map[string]any
    Version int64  // 用于乐观锁防"老 binlog 覆盖新值"
}
```

### 8.3 service 层引擎

```go
// webook/migrator/service/full/full.go
type FullEngine interface {
    Run(ctx context.Context, taskId int64) error
    Pause(taskId int64) error
}

// webook/migrator/service/incr/incr.go
type IncrEngine interface {
    Run(ctx context.Context, taskId int64) error
    Pause(taskId int64) error
    Lag(taskId int64) (time.Duration, error)
}

// webook/migrator/service/verify/verify.go
type VerifyEngine interface {
    Sample(ctx context.Context, taskId int64, rate float64) (mismatch int64, err error)
    Full(ctx context.Context, taskId int64) (mismatch int64, err error)
    Repair(ctx context.Context, taskId int64, strategy domain.RepairStrategy, ids []int64) error
}

// webook/migrator/service/switching/switch.go
type SwitchService interface {
    SetGray(ctx context.Context, taskId int64, percent int) error
    SetStage(ctx context.Context, taskId int64, stage domain.Stage) error
    Cutover(ctx context.Context, taskId int64) error
    Rollback(ctx context.Context, taskId int64) error
}
```

### 8.4 业务侧 SDK

```go
// webook/internal/migratorsdk/sdk.go
package migratorsdk

type SwitchReader interface {
    // ChooseSide 按 user/biz hash + grayPercent 决定本次读走哪侧
    ChooseSide(ctx context.Context, taskName string, hashKey int64) (Side, error)
}

type DualWriter interface {
    // Write 按当前 stage 决定单写 / 双写 / 失败补偿
    Write(ctx context.Context, taskName string, fn func(target Side) error) error
}

type Side string

const (
    SideOld Side = "old"
    SideNew Side = "new"
)

// 默认实现：NoOpSwitchReader / NoOpDualWriter
// 未启用时返回固定 SideOld / 直接调 fn(SideOld)，业务零感知
```

### 8.5 Source / Sink 实现矩阵

| Source | 同构 | 异构 | 备注 |
|--------|:----:|:----:|------|
| `MySQLSource` (GORM 分片 SELECT) | 全量 | 全量 | 走 ReadReplica |
| `CanalSource` (订阅 binlog) | 增量 | 增量 | 主推 |
| `MaxwellSource` | 备 | 备 | 备选 |

| Sink | 用途 |
|------|------|
| `MySQLSink` | 同构（跨机房 / 分库分表 / schema 演进） |
| `ESSink` | 异构搜索（重构现有 `article_search.go`） |
| `ClickHouseSink` | 异构 OLAP（互动 / 点击事件报表） |
| `MongoSink` | 异构文档型 |
| `TiDBSink` | 异构 HTAP |
| `KafkaSink` | 异构事件流（下游订阅） |

**接入新 Sink 只需实现 `Sink` 接口 + ioc 注册**，full/incr/verify/switch 引擎零改动 — 这是"全覆盖通用框架"的核心价值。

---

## 9. 业务侧代码影响（对现在代码的实际改动）

> 用户问：对现在代码的实际影响有哪些，需要改动什么？

### 9.1 未启用时：零影响

**关键设计**：webook-migrator 是独立服务，业务侧 SDK 默认 `NoOpSwitchReader` / `NoOpDualWriter`：

```go
// webook/internal/migratorsdk/noop.go
type NoOpSwitchReader struct{}
func (NoOpSwitchReader) ChooseSide(ctx context.Context, taskName string, hashKey int64) (Side, error) {
    return SideOld, nil  // 永远走旧侧
}

type NoOpDualWriter struct{}
func (NoOpDualWriter) Write(ctx context.Context, taskName string, fn func(Side) error) error {
    return fn(SideOld)  // 直接调一次旧侧的写
}
```

webook-core / webook-chat 即使引入了 SDK，未启用迁移时业务路径与今天一致——同一个 DAO 调用，无额外 RPC、无额外缓存读、无 RT 增量。

启用迁移时把 NoOp 替换成 `RedisSwitchReader` / `RedisDualWriter`，每次写多一次 Redis GET（gray% / switch stage），P99 RT 增量 < 1ms。

### 9.2 场景 A：`article_search.go` CDC 重构（推荐第一个落地）

**驱动**：当前 `article_search.go` 是同步阻塞型 ES Upsert——文章发布主流程必须等 ES 写完才返回。ES 抖一下 P99 就到秒级；ES 故障文章发布完全卡死。重构成异步 CDC 后，业务只写 MySQL，ES 由 webook-migrator 订阅 binlog 异步同步。

### 9.2.1 文件改动清单

| 文件 | 改动 | 行数估计 |
|------|------|---------|
| `webook/internal/repository/dao/article_search.go` | **拆分**：`Search` 留下；`Upsert` / `Delete` 整体迁到 `webook-migrator` | -55 行 / 留 ~60 行 |
| `webook/internal/repository/article.go` | **删除** `articleSearchDAO.Upsert` / `Delete` 调用；保留 `Search` 调用 | -8 行 |
| `webook/internal/service/article.go` | 不再依赖 `ArticleSearchDAO` 的写入接口（只查询） | 调用点小调整 |
| `webook/wire.go` | `ArticleSearchDAO` 不再注入到 `ArticleService` 写入路径，只注入到 `SearchService` | 重生成 wire_gen.go |
| `webook/migrator/pipeline/sink/es.go` | **新增**：复用现有 `ElasticArticleDAO` 的 ES client + ArticleESDoc 结构 | +120 行 |
| `webook/migrator/pipeline/transform/article_to_es_doc.go` | **新增**：从 binlog ChangeEvent 转 ArticleESDoc（含向量化触发） | +60 行 |

### 9.2.2 改动 diff 示意

**改前**（业务侧同步阻塞）：

```go
// webook/internal/repository/article.go
func (r *cachedArticleRepository) Sync(ctx context.Context, article domain.Article) (int64, error) {
    id, err := r.dao.Upsert(ctx, dao.PublishedArticle{...})
    if err != nil {
        return 0, err
    }
    // ⬇️ 同步阻塞：ES 抖一下整个 Sync 卡住
    err = r.searchDAO.Upsert(ctx, dao.ArticleESDoc{...})
    if err != nil {
        r.l.Warn("ES 同步失败，但已写 MySQL", logger.Error(err))
    }
    return id, nil
}
```

**改后**（业务侧只写 MySQL）：

```go
// webook/internal/repository/article.go
func (r *cachedArticleRepository) Sync(ctx context.Context, article domain.Article) (int64, error) {
    id, err := r.dao.Upsert(ctx, dao.PublishedArticle{...})
    if err != nil {
        return 0, err
    }
    // 不再调 searchDAO.Upsert
    // ES 由 webook-migrator 订阅 binlog 异步同步
    return id, nil
}
```

```go
// webook/migrator/pipeline/sink/es.go (新增)
type ESArticleSink struct {
    client *elasticsearch.TypedClient
    embed  embedding.EmbeddingClient
}

func (s *ESArticleSink) Apply(ctx context.Context, batch []sink.Mutation) error {
    bulk := s.client.Bulk()
    for _, m := range batch {
        if m.Op == "delete" {
            bulk.DeleteOp(types.DeleteOperation{
                Index_: ptr("article_v1"),
                Id_:    ptr(strconv.FormatInt(m.PK, 10)),
            })
            continue
        }
        doc := transform.ArticleToESDoc(m.Cols)
        if doc.ContentVec == nil {
            vec, _ := s.embed.Embed(ctx, doc.Title+" "+doc.Abstract)
            doc.ContentVec = vec
        }
        bulk.IndexOp(
            types.IndexOperation{Index_: ptr("article_v1"), Id_: ptr(strconv.FormatInt(m.PK, 10))},
            doc,
        )
    }
    _, err := bulk.Do(ctx)
    return err
}
```

### 9.2.3 迁移步骤

1. **Day 1**：webook-migrator 部署上线，创建 task `article_to_es_v1`，mode=cdc
2. **Day 1**：启动全量 + 增量，让 NEW(ES) 数据等价于 OLD(MySQL)
3. **Day 2**：sample verify 通过 → 改业务代码删 `searchDAO.Upsert` 调用
4. **Day 2**：业务上线，文章发布只写 MySQL；webook-migrator 持续 CDC 同步 ES
5. **Day 3-9**：观察期一周；监控 + 对账（DST_ONLY 已不可逆，cutover 前已充分验证）
6. **Day 10**：收尾 / OLD 下线

> 精确到天的端到端时间线（D-3 ~ D12 + 每步命令 + 监控 + 回滚）见 `./04-cutover-playbook.md` §15.1。

### 9.2.4 微服务依赖变化

**改前**：

```
webook-core → MySQL (article)
webook-core → ES (article_v1)              ← 业务关键路径依赖 ES
```

**改后**：

```
webook-core → MySQL (article)              ← 只剩这条关键路径
webook-core → ES (article_v1) [仅 Search]  ← 读路径仍依赖；写路径解耦

webook-migrator → MySQL.binlog（Canal 订阅）
              → IncrEngine 进程内分发
              → ES (article_v1)            ← 写路径在这里
```

业务关键路径从 2 个外部依赖（MySQL + ES 写）降到 1 个（MySQL）。

### 9.2.5 回滚步骤

如果 CDC 链路出问题，5 分钟回滚：

1. 把删掉的 `searchDAO.Upsert(...)` 调用 git revert 回来
2. 业务重新部署 → 退回同步双写
3. webook-migrator task 暂停，不影响业务

### 9.3 场景 B：`user.nickname` schema 演进（典型同构）

**驱动**：现在 `user.Nickname` 是 `varchar(50)`，业务发现部分用户需要长一点的展示名（如组织名 + 个人）。直接 `ALTER TABLE user MODIFY nickname VARCHAR(255)` 在亿级表会锁表数小时（即使 INSTANT 算法也有元数据锁）。安全做法：新加 `nickname_v2 VARCHAR(255)` 字段，迁移期双写，迁完切读后下线 `nickname`。

### 9.3.1 文件改动清单

| 文件 | 改动 | 备注 |
|------|------|------|
| `webook/scripts/webook.sql` | `user` 表加 `nickname_v2 varchar(255) NOT NULL DEFAULT ''` 列 | DDL 即时（默认值不锁表） |
| `webook/internal/repository/dao/user.go` | `User` struct 加 `NicknameV2 string`；DAO 加 `UpdateLegacy` / `UpdateV2` 方法 | 双方法各写一字段 |
| `webook/internal/repository/user.go` | `Update` 包成 `dualWriter.Write(ctx, "user_nickname_v2", ...)`；`FindById` 包 `switchReader.ChooseSide(...)` 决定读哪个字段 | 改 4 个方法 |
| `webook/internal/domain/user.go` | `Nickname` 字段不变；DAO → Domain 映射在 repository 层处理 | 业务层零感知 |
| `webook/ioc/migrator_sdk.go` | **新增**：启用 `RedisSwitchReader` / `RedisDualWriter` Provider | 默认 NoOp Provider 替换 |
| `webook/wire.go` + `wire ./...` | 重生成 `wire_gen.go` | 必跑 |

### 9.3.2 改动 diff 示意

**改前**：

```go
// webook/internal/repository/user.go
func (r *cachedUserRepository) Update(ctx context.Context, u domain.User) error {
    return r.dao.Update(ctx, dao.User{
        Id:       u.Id,
        Nickname: u.Nickname,
        // ...
    })
}

func (r *cachedUserRepository) FindById(ctx context.Context, id int64) (domain.User, error) {
    u, err := r.dao.FindById(ctx, id)
    return domain.User{Nickname: u.Nickname /* ... */}, err
}
```

**改后**：

```go
// webook/internal/repository/user.go
func (r *cachedUserRepository) Update(ctx context.Context, u domain.User) error {
    return r.dualWriter.Write(ctx, "user_nickname_v2", func(side migratorsdk.Side) error {
        if side == migratorsdk.SideOld {
            return r.dao.UpdateLegacy(ctx, dao.User{Id: u.Id, Nickname: u.Nickname /* ... */})
        }
        return r.dao.UpdateV2(ctx, dao.User{Id: u.Id, NicknameV2: u.Nickname /* ... */})
    })
}

func (r *cachedUserRepository) FindById(ctx context.Context, id int64) (domain.User, error) {
    side, _ := r.switchReader.ChooseSide(ctx, "user_nickname_v2", id)
    u, err := r.dao.FindById(ctx, id)
    if err != nil {
        return domain.User{}, err
    }
    nickname := u.Nickname
    if side == migratorsdk.SideNew {
        nickname = u.NicknameV2  // V2 阶段读新字段
    }
    return domain.User{Nickname: nickname /* ... */}, nil
}
```

### 9.3.3 迁移步骤

1. **Day 1**：DDL 加 `nickname_v2` 列（INSTANT，秒级）
2. **Day 1**：业务接入 SDK，启用 `dualWriter.Write` —— 此时 switch_stage=`SRC_FIRST`（API 调 `POST /switch {stage:SRC_FIRST}`），每次 Update 都写两个字段
3. **Day 1**：webook-migrator 创建 task `user_nickname_v2`，mode=`dual_write`（迁移机制：应用层双写），启动全量回填（把 `nickname` → `nickname_v2`）
4. **Day 2**：verify pass → 切读 50% → 100%
5. **Day 4**：cutover（switch_stage: SRC_FIRST → DST_FIRST → 30s → DST_ONLY），业务侧切到单写 V2（**DST_ONLY 不可逆，切前已充分对账**）
6. **Day 5-D11**：观察期；监控 + 对账，确认 V2 稳定
7. **Day 12**：收尾 / OLD 下线（OLD 列 `nickname` 可下线）
8. **Day 17+**：DDL 删除 `nickname` 列（先 grep 确认无业务读引用）

> 精确到天的端到端时间线（D-3 ~ D12 + 每步命令 + 监控 + 回滚）见 `./04-cutover-playbook.md` §15.2。

### 9.3.4 回滚步骤

任何阶段问题都可回滚：

| 阶段 | 回滚动作 | 业务感知 |
|------|---------|---------|
| 切读阶段（gray > 0，仍 SRC_FIRST 双写） | `POST /tasks/:id/gray {percent: 0}` | 用户读回 `nickname`，秒级 |
| DST_FIRST（30s 过渡，仍双写） | `POST /tasks/:id/switch {stage: SRC_FIRST, action: rollback}` | 回 SRC_FIRST，秒级切回 |
| DST_ONLY 单写后 | **不可回滚**（OLD 已停止更新，point of no return） | 切前必充分对账 |

### 9.4 一次性新增（不动现有代码）

| 新增范围 | 内容 | 启用前业务影响 |
|---------|------|-------------|
| 新服务 | `webook-migrator/` 整个目录（控制台 API + 引擎 + Source/Sink） | 无 |
| 控制库 | `webook_migrator` schema + 5 张表（task / checkpoint / validate_log / audit_log / dead_letter） | 无 |
| 业务 SDK | `webook/internal/migratorsdk/`（接口 + NoOp 默认实现） | 无（NoOp 与今天行为一致） |
| 部署 | `deploy/` 14 项（按 CLAUDE.md 服务拆分清单） | 无（独立 docker-compose 服务） |
| CI | `.github/workflows/migrator-ci.yml` | 无 |

### 9.5 改动量级总结

| 启用范围 | 改动文件数 | 业务关键路径影响 |
|---------|---------|----------------|
| 仅新服务（不接入业务） | ~80（全是新增） | 0 |
| 接入场景 A（article ES CDC） | +5 业务侧改 / +2 新增 | 关键路径少 1 个外部依赖（解耦 ES 写） |
| 接入场景 B（user nickname schema 演进） | +6 业务侧改 / +1 新增 SDK Provider | NoOp 关闭时 0；启用时 P99 RT < 1ms |

---

## 10. 前瞻性设计

| 维度 | 核心问题 | 当前方案 |
|------|---------|---------|
| **扩展性** | 接入新 Sink（如 Doris / Pulsar）改动多大？ | 实现 `Sink` 接口 + ioc 注册即可；task / checkpoint / verify / switch 全复用，**核心引擎零改动** |
| **可用性** | Canal 挂 / 网络分区主流程能跑吗？ | 业务主写 source 不受影响；migrator incr 暂停，binlog pos 持久化在 checkpoint，恢复后续传；`lag>5min` 自动告警 + `gray` 自动归零回退到 read source（safe 默认） |
| **容错性** | 重复消费 / 顺序错乱 / 极端输入安全吗？ | (1) Sink 用 PK + version 乐观锁幂等；(2) dispatcher 按 PK 分进程内 partition 保同 key 顺序；(3) 双写失败入 retry 队列指数退避 3 次再死信 |
| **可观测性** | 5 分钟内能定位落后 / 失败 / 错位吗？ | v1 基础设施 metric（`up` / `webook_http_*` / `webook_db_*` / `go_*`）+ 控制台 API（`/lag` · `/mismatch` · `/tasks/:id`）+ runbook 覆盖；业务级专用 metric 未埋点 |

详细自查（来自 architect skill foresight-checklist）：

- [x] 数据模型面向能力（Source/Sink 抽象，不绑定具体存储）
- [x] 接口参数化（task_id 路由所有操作）
- [x] 新增 Sink 时改动只在 sink 包 + ioc
- [x] 缓存不可用时降级（gray/switch 不可用时退回"全 source 读" safe 默认）
- [x] 外部调用有超时 / 重试（Source/Sink Apply 默认 3s timeout，3 次指数退避）
- [x] 非关键路径失败静默（verify mismatch 落表，不阻塞同步）
- [x] 批量场景合并（incr 默认攒批 1000 条 / 100ms flush，参考 `saramax.BatchConsumer` 模式）
- [x] 幂等性（Sink 侧 PK upsert ON DUPLICATE KEY + Version 乐观锁；at-least-once 靠 checkpoint 重放）
- [x] 并发安全（checkpoint 写入用乐观锁 + version；shard worker 互不重叠 ID 范围）
- [x] 数据边界（gray 0-100 / sample rate 0-1 / batch size 上限 10k）
- [x] 事务边界（control plane DML 短事务；data plane 不跨库事务，靠对账 + 补偿）
- [x] 关键路径结构化日志（task_id / phase / shard / pos / err 全字段）
- [x] 错误日志带业务上下文
- [x] 量化指标（lag / qps / mismatch / progress）

---

## 11. 权限设计

| 接口 | 认证 | RBAC | 备注 |
|------|------|------|------|
| 创建任务 / 启停 / 灰度 | Admin JWT | `migrator:write` | 运维 |
| 切流 (cutover/rollback) | Admin JWT | **`migrator:switch` + 双人复核** | 前端 UI 强制二次确认；最高权限 |
| 修复 (repair) | Admin JWT | **`migrator:repair` + DBA 审批** | 必须先 verify 跑过且 mismatch 低于阈值 |
| 查看（list/detail/lag/mismatch） | Admin JWT | `migrator:read` | 运维 + DBA + SRE |

### 11.1 安全约束

1. **DSN 凭据不入 API body**：只传 `dsnRef`（Vault / K8s Secret 路径），服务端拉密
2. **审计**：所有写操作记 `audit_log`，留存 1 年（GDPR 合规）
3. **敏感字段脱敏**：`tables_json` 配置 `sensitiveColumns`，落对账日志时 mask（手机号 134****5678 / 密码哈希 ***）
4. **网络隔离**：迁移服务全闭网，禁止公网暴露；Nginx 仅放开 `/api/migrator/*` 给运维白名单 IP
5. **rate limit**：写操作 30 req/min/user，避免误触发风暴

---

## 12. DI 变更（Wire）

### 12.1 webook-migrator 三个 Provider Set

```go
// webook/migrator/wire.go
var dataPlaneProviderSet = wire.NewSet(
    source.NewMySQLSource,
    source.NewCanalSource,
    sink.NewMySQLSink,
    sink.NewESSink,
    sink.NewClickHouseSink,
    transform.NewDefaultChain,
)

var controlPlaneProviderSet = wire.NewSet(
    web.NewTaskHandler,
    web.NewOperationHandler,
    service.NewTaskService,
    full.NewFullEngine,
    incr.NewIncrEngine,
    verify.NewVerifyEngine,
    switching.NewSwitchService,
    repository.NewTaskRepository,
    repository.NewCheckpointRepository,
    repository.NewValidateRepository,
    repository.NewAuditRepository,
    dao.NewGormTaskDAO,
    dao.NewGormCheckpointDAO,
    dao.NewGormValidateLogDAO,
    dao.NewGormAuditLogDAO,
    cache.NewRedisThrottleCache,
    cache.NewRedisSwitchStateCache,    // stage/gray/propose 键（SDK 共享）
    // repository：service 层唯一数据入口（task/checkpoint/validate_log/
    // dead_letter/audit_log/throttle/switch_state 各一个仓储 provider）
    repository.NewTaskRepository, // ...等 7 个，见 wire.go
)

var infraProviderSet = wire.NewSet(
    ioc.InitDB,           // 控制库连接
    ioc.InitRedis,
    ioc.InitCanalClient,
    ioc.InitPrometheus,
    ioc.InitLogger,
    ioc.InitOTel,
)
```

### 12.2 业务侧 SDK 注入

```go
// webook/ioc/migrator_sdk.go
package ioc

// 默认 NoOp，业务零感知
func ProvideNoOpSwitchReader() migratorsdk.SwitchReader {
    return migratorsdk.NewNoOpSwitchReader()
}

// 启用模式：从 config / 环境变量切换
func ProvideRedisSwitchReader(cmd redis.Cmdable) migratorsdk.SwitchReader {
    return migratorsdk.NewRedisSwitchReader(cmd)
}
```

接入步骤：
1. 业务侧 wire 注入 `migratorsdk.SwitchReader` / `DualWriter`
2. DAO 调用从 `dao.Insert(...)` 包成 `dualWriter.Write(ctx, "user_migration", func(side) error { return dao.Insert(...) })`
3. 默认 NoOp 不影响主流程；切换 Redis 实现即启用

---

## 13. 服务拆分对照表（CLAUDE.md 14 项）

| # | 维度 | 文件 | 落地内容 |
|---|------|------|---------|
| 1 | 应用配置 | `webook/migrator/config/{local,dev,staging,prod,test}.yaml` | 5 份同构 + `otel.service_name=webook-migrator` + `otel.sample_ratio` 差异 |
| 2 | Wire DI | `webook/migrator/wire.go` + `cd webook && wire ./migrator/...` | 三个 Provider Set + InitWebServer |
| 3 | Prometheus 抓取 | `deploy/prometheus/prometheus.yml` | `job_name: webook-migrator` + targets |
| 4 | Recording rules | `deploy/prometheus/rules/migrator.rules.yml` | full_progress / lag P95 / mismatch_rate |
| 5 | Grafana 告警 | `deploy/grafana/provisioning/alerting/migrator.yml` | up / 5xx / lag>5min / mismatch_rate>0.01% |
| 6 | Grafana 看板 | services-overview 加 migrator 列 + 独立 dashboard | 任务进度 / 增量延迟 / 对账差异 / 切流阶段 4 panel |
| 7 | Docker compose | `deploy/docker-compose.yaml` | 服务定义 + healthcheck (`/health`) + nginx depends_on |
| 8 | Nginx 反代 | `deploy/nginx/conf.d/default.conf` | upstream + `/api/migrator/*` location（白名单 IP） |
| 9 | 部署脚本 | `deploy/deploy.sh` | logs / restart 加 migrator |
| 10 | 部署变量 | `.env.<env>` + `.env.<env>.example` | `MIGRATOR_IMAGE_TAG` / `MIGRATOR_APP_ENV` |
| 11 | CI workflow | `.github/workflows/migrator-ci.yml` | paths 限定 `webook-migrator/**` 互斥 |
| 12 | Dockerfile | `webook/migrator/Dockerfile` | 多阶段构建（context = `webook/` 仓根，与 chat 同模式） |
| 13 | Metric 命名 | builder | `webook_<subsystem>_*`（http / db / go，禁止 `webook_<service>_*`）；service 区分靠 job label |
| 14 | 文档 | `CHANGELOG.md` + `prd/migrator/`（架构 / product / playbook / runbooks）| 拆分原因 + 接入方式 |

review 收尾前 `grep -rn 'webook-migrator'` 全仓扫确认 14 项全部同步。

---

## 14. 任务拆分

| # | 任务 | 依赖 | 粒度 | 阶段 |
|---|------|------|------|------|
| 1 | 服务骨架 + 控制库 5 张表（task / checkpoint / validate_log / audit_log / dead_letter）+ Wire 基础 + main.go + ioc 基础 | 无 | 1 day | M1 |
| 2 | `pipeline.Source` / `Sink` 抽象 + `MySQLSource` + `MySQLSink` + 单元测试 | #1 | 1 day | M1 |
| 3 | `FullEngine`（分片并行 + checkpoint 续传 + 限速）+ 集成测试（千万级模拟） | #2 | 2 day | M1 |
| 4 | `CanalSource` + `IncrEngine`（binlog 订阅 + 攒批 + checkpoint）+ Canal docker compose | #2 | 2 day | M1 |
| 5 | `VerifyEngine`（采样 + 增量 + repair）+ Prometheus 指标 + mismatch 落表 | #3, #4 | 2 day | M1 |
| 6 | `SwitchService` + 切流状态机 + Redis 灰度配置 + 状态机单元测试覆盖 | #3, #4 | 1 day | M1 |
| 7 | 控制台 Handler 11 个 API + RBAC + 审计日志 + swagger / OpenAPI 输出 | #6 | 2 day | M2 |
| 8 | 业务侧 SDK：`DualWriter` + `SwitchReader` + NoOp + article 表 demo | #6 | 1 day | M2 |
| 9 | 异构 Sink：`ESSink`（重构 `article_search.go`）+ `ClickHouseSink` | #4 | 2 day | M3 |
| 10 | 部署 / 监控 / 告警 / CI（CLAUDE.md 14 项对照表落地） | #7 | 1 day | M3 |
| 11 | E2E 演练：`article` 表跑"同构 schema 演进 + 异构 ES CDC"双场景 + 故障注入（Canal 挂 / checkpoint 损坏 / mismatch 重放） | #1-#10 | 2 day | M3 |

合计 **~17 工作日**（不含设计周期）。

### 14.1 阶段交付

| 阶段 | 交付物 |
|---|---|
| **M1 同构核心**（任务 #1-#6，~9 天） | webook-migrator 可独立运行，能跑同构迁移全流程（创建 / 全量 / 增量 / 对账 / 切流） |
| **M2 控制台 + SDK**（任务 #7-#8，~3 天） | 控制台 API + 业务侧 SDK，schema 演进可落地（article 表 demo） |
| **M3 异构 + 验收**（任务 #9-#11，~5 天） | ES/CK Sink + 部署监控 + E2E 演练通过 |

> 任务实施进入实战阶段后，按时间维度的执行手册见 `./04-cutover-playbook.md`：§2 路线图、§3-§9 阶段细节、§13 故障演练（必跑 6 项）、§14 一页纸 bash 命令序列。

---

## 15. 风险点

### 15.1 性能（亿级关键）

| 风险 | 缓解方案 |
|---|---|
| 单线程 SELECT 撑不住源库 | 按 `id` 范围分 16 片，多 worker 并行；走 `ReadReplica`；Sink 攒批 1000 条/100ms；qps 限速默认 10k/s 可调 |
| binlog 追平跟不上 | dispatcher 按 PK hash 分到 16 个进程内 partition，16 个 incr worker 并行 |
| 对账放大（亿级 SQL JOIN 卡死） | verify 用 PK hash 采样（API 触发，不做亿级 JOIN）+ 全量走外部 Spark/Flink dump+hash 比对，结果回 `validate_log` |

### 15.2 并发

| 风险 | 缓解方案 |
|---|---|
| 双写竞态：业务写 OLD 成功 → 还没写 NEW → CDC 已经把 binlog 同步到 NEW → 双写 overwrite 旧值 | Sink 写入加 `WHERE updated_at >= cur` 乐观锁条件，新值不会被旧 binlog 覆盖 |
| 切读不一致：灰度 50% 时同一用户一会读旧一会读新（read-your-write 破坏） | 按 `user_id hash % 100 < grayPercent` 路由，同一用户始终命中同侧 |
| Checkpoint 跨进程并发：多 incr worker 同更新 | MySQL 行锁 + `version` 乐观锁，并发只有 1 个 worker 赢，其余重读 |

### 15.3 安全

| 风险 | 缓解方案 |
|---|---|
| DSN 泄露 | `dsnRef` → Vault/Secret，绝不明文入库 / 入日志 |
| repair 滥用：能直接覆盖业务数据 | 双人审批 + 必须 verify 通过 + mismatch 低于阈值 + 全审计 |
| diff_detail 污染：可能把密码哈希、手机号写进对账日志 | 每表配置 `sensitiveColumns`，落表前 mask |

### 15.4 回归

| 风险 | 缓解方案 |
|---|---|
| 业务 SDK 引入风险：webook-core / webook-chat 引 dual_writer 改 DAO 调用 | SDK 默认 NoOp（grayPercent=0 时透传 source DAO），未启用零感知；先在 dev 走影子流量 |
| Wire 重生成范围：业务侧 wire 重生成可能炸出隐性循环 | SDK 用接口注入，默认 `NoOpSwitchReader` 兜底；只有真接入的服务才注入 Redis 实现 |
| 监控误报：大事务时 lag 飙高但其实正常 | lag 用 `binlog 事件时间` 而非 wallclock，sustained > 5min（窗口 3min 平均）才告警 |

---

## 16. 不在范围（明确排除）

| 不支持 | 原因 | 替代方案 |
|--------|------|---------|
| **强一致迁移（XA / 2PC）** | 双写最终一致 + 对账补偿，不能保证毫秒级强一致 | SAGA / TCC 单独方案 |
| **跨大版本字符集变更**（utf8 → utf8mb4_0900） | 排序规则差异需预处理 | 外挂脚本预处理后再走本方案 |
| **数据脱敏 / 加密迁移** | 作为 Transform 链扩展点，不是默认能力 | 实现 transform plugin |
| **表无主键 / 无 binlog** | CDC 不可行 | 先补主键 / 开 binlog 再迁 |
| **业务直接 raw SQL 绕过 Repository** | 无法拦截双写 | 先重构到 Repository 层再迁 |
| **单实例无副本的源库** | 全量扫描会拖垮 | 先加 ReadReplica |

playbook §0.2 给出每条边界的检测命令与判定标准。

---

## 17. 关键决策（默认选项）

| 选项 | 默认 | 备选 |
|---|---|---|
| CDC 中间件 | **Canal**（国内生态最成熟） | Maxwell / Debezium |
| 全量工具 | **自研 Source/Sink**（与控制面打通最深） | DataX / Spark |
| OpenAPI / Swagger | **是** | — |
