# 数据迁移框架 PRD（webook-migrator）

> 作用：为 webook 后端提供一套**企业级**数据迁移能力，覆盖**同构**（MySQL → MySQL）与**异构**（MySQL → 任意 sink）两类场景，亿级数据规模下零停机切流。
> 读者：后端 / 运维 / DBA / SRE
> 配套：`./02-architecture.md`（技术架构）/ `./04-cutover-playbook.md`（端到端实操 runbook）
> **v1 实现摘要**：能力全集与关键对齐点见 [`02-architecture.md`](./02-architecture.md) 顶部「📌 v1 实现摘要」。

---

## 三件套导航

| 文档 | 角色 | 主要回答 |
|------|------|---------|
| **`PRD.md`**（本文档） | 业务 PRD | 要做什么 · 为谁 · 验收标准 |
| `architecture.md` | 技术架构 | 怎么设计 · Source/Sink 抽象 · 状态机 · 控制库 schema · 业务侧代码影响 |
| `zero-downtime-playbook.md` | 端到端实操 runbook | 怎么一步步执行（D-3 → D12 时间线 + 命令 + 监控 + 演练） |

术语 / 数字 / 阶段命名以 `architecture.md` 为权威定义；任意冲突以 architecture 为准。

补充：
- [`runbooks/`](./runbooks/) — 应急手册（按事件类型 10 份），oncall 接告警先查 runbook
- [`retros/`](./retros/) — 演练记录（cutover 前 6 项必跑）
- [`README.md`](./README.md) — 文档索引页

---

## 1. 背景与目标

### 1.1 现状

webook 当前已经"自然地"出现了若干迁移雏形，但都是一次性、强耦合的实现：

| 已有迁移现象 | 实现位置 | 局限 |
|---|---|---|
| 文章双库（制作 / 线上） | `dao/article_author.go` + `dao/article_reader.go` | 应用层同步双写，无对账、无切流、无回滚 |
| 文章 ↔ ES 索引 | `dao/article_search.go` 同步 Upsert | 业务侵入；ES 故障会拖垮主流程；无补偿 |
| 互动事件 → Kafka | `events/interaction/` | 仅单向投递，无 sink 通用化 |

未来一定会遇到：
- **跨机房 / 上云**：MySQL 整库搬家
- **容量瓶颈**：单库 → 分库分表
- **schema 演进**：旧表字段拆分 / 类型升级 / 软删除补齐
- **多 sink 扩展**：报表（ClickHouse）/ 文档型（Mongo）/ HTAP（TiDB）

每次都从零写一套迁移工具，成本高、风险大、监控告警全部各搞一套。**需要一个统一框架。**

### 1.2 目标

| # | 目标 | 衡量 |
|---|------|------|
| G1 | 同构迁移通用化 | 跨机房 / 分库分表 / schema 演进 三类都跑同一框架 |
| G2 | 异构迁移通用化 | 接入新 sink（ES/CK/Mongo/TiDB/Doris/...）核心引擎零改动 |
| G3 | 零停机切流 | 业务方无感知，灰度可调，回滚 5 分钟内 |
| G4 | 亿级规模可承载 | 全量分片并行 / 增量按 PK 分区追平 / 对账可采样可全量 |
| G5 | 可观测可审计 | 5 分钟内能定位落后 / 失败 / 错位；所有操作留 1 年审计 |
| G6 | 安全合规 | DSN 走 Vault；敏感字段对账日志 mask；高危操作双人复核 |

### 1.3 非目标

- **强一致迁移（XA / 2PC）**：本框架默认最终一致 + 对账补偿。支付级强一致需要单独走 SAGA / TCC，不在本期范围
- **跨大版本字符集差异**（MySQL 5.7 → 8.0 collation）：需要外挂预处理脚本，不内置
- **数据脱敏 / 加密迁移**：作为 Transform 链扩展点，本期只做接口预留

---

## 2. 用户与角色

| 角色 | 关心什么 | 主要场景 |
|------|----------|---------|
| **运维 / SRE** | 启动迁移、调灰度、看进度 | 创建任务、设置 gray%、观察 lag |
| **DBA** | 数据正确性、对账、修复差异 | 跑 verify、审 mismatch、批准 repair |
| **业务后端** | 自家表能不能跟着切 | 接 SDK：`DualWriter` + `SwitchReader` |
| **架构师 / Tech Lead** | 接入新 sink 成本 | 实现 `Sink` 接口注册 ioc |
| **Leader / 安全** | 操作可审计、不泄密 | 审计日志、双人复核记录 |

---

## 3. 用户故事（按优先级）

### P0 — 同构核心

**US-1**：作为 SRE，我希望可以**创建一个迁移任务**（指明源库、目标库、涉及表、模式），让系统接管后续工作流。

**US-2**：作为 SRE，我希望可以**启动全量同步**，框架按 ID 分片并行扫源库写目标库，可看到每个分片实时进度。

**US-3**：作为 SRE，我希望可以**启动增量同步**，框架订阅 binlog 持续追平，断点可续传。

**US-4**：作为 SRE，我希望可以**调整灰度比例**（0% → 50% → 100%），按 user_id hash 分流读请求，同一用户始终命中同侧（保 read-your-write）。

**US-5**：作为 SRE，我希望可以一键**切流**（cutover），写也切到新库；切流过程中（双写期 SRC_FIRST / DST_FIRST）发现问题可**秒级回滚**到旧库（切单写 DST_ONLY 后 OLD 停滞、不可回滚，故切前强制充分对账兜底）。

**US-6**：作为 DBA，我希望框架自动**采样对账**（默认 1%），异常时升级为**全量对账**，差异落 `validate_log` 表。

**US-7**：作为 DBA，我希望可以**手动批量修复**差异行（指定方向、白名单 ID），所有修复操作必须经过审计。

### P0 — 异构核心

**US-8**：作为业务后端，我希望接入异构 sink 时**核心引擎零改动**：实现 `Sink` 接口 + ioc 注册即可，复用现有 task / checkpoint / verify / switch 全套能力。

**US-9**：作为业务后端，我希望文章索引同步从"应用层同步 Upsert"重构为"binlog CDC 异步"，业务主流程不被 ES 故障拖垮。

### P1 — 业务集成

**US-10**：作为业务后端，我希望在 schema 演进时**直接调 SDK 包装的 DAO**，迁移期内自动双写，迁移完成后切回单写，业务代码改动可控。

**US-11**：作为运维，我希望迁移期间可观测——基础设施 metric（`webook_http_*` / `webook_db_*` / `go_*`，service 靠 job label 区分）**统一上 Prometheus** + 控制台 API 巡检 lag / mismatch / 进度，看板沿用 services-overview 格式。

**US-12**：作为安全，我希望 DSN 不入 API body 与日志，**只传 `dsnRef`**（Vault 路径），且所有写操作进 `audit_log`。

### P2 — 高级能力

**US-14**：作为架构师，我希望框架预留 **Transform 链扩展点**，支持字段映射 / 类型转换 / 脱敏 / 加密。

**US-15**：作为业务后端，我希望支持**强一致过渡**（应用层双写 + CDC 补偿历史）的组合模式。

---

## 4. 范围矩阵

### 4.1 同构场景（MySQL → MySQL）

| 场景 | 典型驱动 | 框架支持模式 |
|---|---|---|
| 跨机房 / 上云 | 自建机房搬家 / 上云 | CDC + 全量分片 |
| 单库 → 分库分表 | 容量瓶颈 | 双写 + CDC + 分片路由 |
| 旧表 → 新表 schema 演进 | 字段拆分 / 软删除补齐 / 类型升级 | 应用层双写 + 全量回填 |

### 4.2 异构场景（MySQL → 任意 Sink）

| 目标 | 用途 | 实现 Sink |
|---|---|---|
| ES | 搜索 | `ESSink`（重构现有 `article_search.go`） |
| ClickHouse | OLAP / 报表（互动 / 点击） | `ClickHouseSink` |
| MongoDB | 文档型 | `MongoSink` |
| TiDB | HTAP | `TiDBSink` |
| Kafka | 事件流 | `KafkaSink`（下游订阅） |
| Doris / Hive / Pulsar | 数仓 / BI / 消息 | 实现接口扩展 |

### 4.3 数据规模

亿级基线：
- 全量：分 16 片，4-8 worker 并行，预期 12-24h 跑完亿级表
- 增量：日均 GB 级 binlog，按 PK hash 分 16 partition，多 incr worker 并行追平
- 对账：增量事件级实时比对 + T+1 离线全量比对（外接 Spark/Flink dump+hash）

千万级 / 十亿级也可承载，差异在 worker 数与限速参数；十亿级建议外挂分布式计算（Spark / Flink）。

---

## 5. 关键流程

### 5.1 标准迁移生命周期

```
[1] 创建任务            POST /tasks
    ├─ name / mode / kind / 源 / 目标 / 表清单
    └─ status: created
        │
[2] 启动全量            POST /tasks/:id/start {phase: full}
    ├─ 分片并行 SELECT → Sink 攒批写入
    └─ status: full_running → full_done
        │
[3] 启动增量            POST /tasks/:id/start {phase: incr}
    ├─ Canal 订阅 binlog → IncrEngine 进程内分发 → Sink Apply
    └─ status: incr_running
        │
[4] 采样对账            POST /tasks/:id/verify {mode: sample}
    └─ 差异落 validate_log
        │
[5] 灰度切读            POST /tasks/:id/gray {percent: 50}
    └─ 按 user_id hash 分流
        │
[6] 全量对账            POST /tasks/:id/verify {mode: full}
    └─ T+1 离线 dump+hash 比对
        │
[7] 一键切流            POST /tasks/:id/switch {stage: cutover}
    ├─ 双人复核 + 审计
    ├─ 写也切到 sink；source 转只读
    └─ status: switched
        │
[8] 观察期 N 小时
    │
[9] 收尾 / OLD 下线     观察期满 → 运维手动下线 OLD（v1 无 closed 状态/API，task 停在 switched）
    └─ DST_ONLY 单写不可逆，下线前充分确认 NEW 无误
```

### 5.2 异常路径

| 情况 | 处理 |
|---|---|
| 全量某分片失败 | checkpoint 持久化，恢复后续传该分片，不影响其他分片 |
| Canal 挂 / 网络分区 | incr 暂停，binlog pos 保留；恢复后从断点续传；`lag>5min` 触发 P1 告警；`gray` 自动归零回退到 source |
| 对账发现差异超阈值 | 自动暂停 cutover；告警通知 DBA；强制走 repair 流程 |
| 双写期发现问题 | `POST /switch {stage: SRC_FIRST, action: rollback}` 切回 OLD（SRC_FIRST / DST_FIRST 双写期 OLD 有全量数据，秒级回滚）；切到 DST_ONLY 单写后 OLD 停滞、**不可回滚**，故切前必充分对账 |
| 双写失败 | 进 retry 队列指数退避 3 次；仍失败入死信队列，告警 |

---

## 6. 验收标准

### 6.1 功能验收

| # | 验收项 | 通过标准 |
|---|---|---|
| AC-1 | 同构 schema 演进 | 用 `article` 表 demo，新加 `summary` 字段 + 双写 + 全量回填 + 切读，业务无感 |
| AC-2 | 异构 ES 同步 | `article_search.go` 重构成 CDC 异步，业务主流程与 ES 解耦，停 ES 业务不受影响 |
| AC-3 | 切流回滚 | 切流后人为注入异常，rollback 在 5 分钟内完成，业务恢复正常 |
| AC-4 | 故障注入演练 | Canal 挂 / checkpoint 损坏 / mismatch 重放 三类故障可恢复 |
| AC-5 | 双写补偿 | 双写过程注入 Sink 失败，retry 队列正确处理，最终一致 |

### 6.2 性能验收

| # | 验收项 | 通过标准 |
|---|---|---|
| PF-1 | 全量吞吐 | 千万级表 ≤ 30min（单实例 16 worker）；亿级 ≤ 12h |
| PF-2 | 增量延迟 | P95 lag ≤ 5s；P99 ≤ 30s；sustained > 5min 触发告警 |
| PF-3 | 对账采样 | 千万级表采样 1% ≤ 5min |
| PF-4 | 业务侧 SDK 开销 | NoOp 模式（未启用）QPS / RT 完全无影响；启用后 P99 RT 增量 < 1ms |

### 6.3 可观测验收

| # | 验收项 | 通过标准 |
|---|---|---|
| OB-1 | Prometheus 可观测 | 基础设施 metric（`webook_<subsystem>_*`，service 靠 job label）+ 控制台 API + Grafana 看板 |
| OB-2 | 告警 | up / 5xx / lag / mismatch 4 类告警 |
| OB-3 | 审计 | 所有写操作进 `audit_log`，含 actor / action / payload / result / IP |
| OB-4 | 看板 | services-overview 加 migrator 列；独立 dashboard 含任务进度 / 增量延迟 / 对账差异 / 切流阶段 |

### 6.4 安全验收

| # | 验收项 | 通过标准 |
|---|---|---|
| SC-1 | DSN 安全 | API body / 数据库 / 日志均不出现明文 DSN |
| SC-2 | 敏感字段 mask | `diff_detail` 落表前手机号 / 密码哈希等按白名单 mask |
| SC-3 | RBAC | 切流 / 修复需独立角色；前端 UI 强制双人复核 |
| SC-4 | 网络隔离 | Nginx 仅放白名单 IP 访问 `/api/migrator/*` |

---

## 7. 接口契约

### 7.1 HTTP API（11 个）

详见 `./02-architecture.md` 「HTTP 接口设计」章节。统一响应：

```json
{ "code": 0, "msg": "ok", "data": {} }
```

### 7.2 错误码

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

错误码权威定义在 `./02-architecture.md` §6.1。任何冲突回退到 architecture。

---

## 8. 风险与依赖

### 8.1 风险

| 类别 | 风险 | 缓解 |
|---|---|---|
| 性能 | 亿级全量拖垮源库 | ReadReplica + 分片并行 + 限速可调 |
| 性能 | binlog 追平跟不上 | 按 PK hash 分 partition + 多 worker |
| 性能 | 对账放大 | 增量事件级 + T+1 离线 + 不做反复 SQL JOIN |
| 并发 | 双写竞态 + 老 binlog 覆盖新值 | Sink `WHERE updated_at >= cur` 乐观锁 |
| 并发 | 切读不一致破坏 read-your-write | 按 user_id hash 分流，同用户始终命中同侧 |
| 并发 | Checkpoint 跨进程 | 行锁 + version 乐观锁 |
| 安全 | DSN 泄露 | dsnRef → Vault；不入库 / 日志 |
| 安全 | repair 滥用 | 双人审批 + verify 前置 + 全审计 |
| 安全 | diff_detail 泄露敏感字段 | sensitiveColumns 白名单 mask |
| 回归 | 业务 SDK 引入风险 | 默认 NoOp；接口注入；先影子流量 |
| 回归 | Wire 重生成 | 接口隔离；NoOp 兜底 |
| 回归 | 监控误报 | 用 binlog 事件时间，sustained > 5min 才告警 |

### 8.2 依赖

| 依赖 | 用途 | 替代 |
|---|---|---|
| Canal | binlog 订阅 | Maxwell（备）/ Debezium（备）|
| Kafka | 仅作 sink 类型（`sinkType=kafka`）；CDC 不经 Kafka | 已有，不新增 |
| Redis | stage / gray 灰度 / 双人复核 propose / throttle 限速 | 已有，不新增 |
| MySQL | 控制库 + 业务库 | 已有，新建独立控制库 `webook_migrator` |
| Prometheus + Grafana | 指标 + 看板 | 已有，按 CLAUDE.md 服务拆分 14 项接入 |
| Vault / K8s Secret | DSN 凭据 | 暂用 K8s Secret（L2 演进引 Vault） |

---

## 9. 里程碑

| 阶段 | 内容 | 工期 |
|------|------|------|
| **M1 同构核心** | 任务 #1-#6（骨架 / Source / Sink / Full / Incr / Verify / Switch） | ~9 天 |
| **M2 控制台 + SDK** | 任务 #7-#8（API / RBAC / 审计 / 业务 SDK） | ~3 天 |
| **M3 异构 + 验收** | 任务 #9-#11（ES/CK Sink / 14 项部署 / E2E 演练） | ~5 天 |

合计 **~17 工作日**（不含设计周期）。详细任务见 `./02-architecture.md` 「任务拆分」章节。

按时间维度的端到端执行手册见 `./04-cutover-playbook.md`，其 §2 路线图给出 D-3 → D12 的精确日历，§14 提供一页纸 bash 命令序列。
