# 不停机迁移完整方案（Playbook）

> 配套：`./01-product.md`（要做什么）/ `./02-architecture.md`（怎么设计）
> 本文档定位：**端到端实操手册**——每一步具体命令、预期结果、异常处理、回滚动作
> 读者：执行迁移的运维 / DBA / 后端 owner
> **术语权威**：所有 stage / status / 枚举命名以 `architecture.md` 为准，本文档与之冲突时回退到 architecture

---

## 三件套导航

| 文档 | 角色 | 主要回答 |
|------|------|---------|
| `PRD.md` | 业务 PRD | 要做什么 · 为谁 · 验收标准 |
| `architecture.md` | 技术架构（**权威定义**） | 怎么设计 · Source/Sink 抽象 · 状态机 · 控制库 schema · 业务侧代码影响 |
| **`zero-downtime-playbook.md`**（本文档） | 端到端实操 runbook | 怎么一步步执行（D-3 → D12 时间线 + 命令 + 监控 + 演练） |

---

## 目录

- §0 不停机的工程定义与边界
- §1 前置条件（不满足则不能不停机）
- §2 整体路线图（典型 12 天）
- §3 阶段 0：准备（D-3 ~ D-1）
- §4 阶段 1：全量同步（D0 ~ D1）
- §5 阶段 2：增量追平（D1 起）
- §6 阶段 3：对账（D2 ~ D3）
- §7 阶段 4：灰度切读（D3）
- §8 阶段 5：cutover 切写（D4）
- §9 阶段 6：观察期 + 收尾（D5 ~ D12）
- §10 三大坑的完整解法
- §11 关键代码片段
- §12 监控与告警
- §13 故障演练（必跑）
- §14 完整 Runbook（一页纸命令序列）
- §15 两个端到端 Case Study
- §16 FAQ

---

## 0. 不停机的工程定义与边界

「不停机」不是魔法，是一组工程化保证。本方案对**满足前置条件**的迁移作如下承诺：

| 保证 | 含义 | 验证方式 |
|------|------|---------|
| **业务不停服** | 迁移全程业务请求 100% 能处理（含写入） | 监控 `webook_*_http_requests_total` 全程不掉零 |
| **零数据丢失** | 迁移前后源 / 目标行数 + 内容一致 | T+1 全量对账 mismatch_rate < 0.001% |
| **read-your-write** | 任意用户写完后立即能读到 | 灰度按 user_id hash 分流，同用户固定一侧 |
| **双写期可回滚** | SRC_FIRST / DST_FIRST 双写期秒级 rollback 回 OLD；DST_ONLY 单写后不可逆（切前充分对账兜底） | 故障演练验证（§13） |
| **崩溃可恢复** | webook-migrator 任何崩溃恢复后续传 | checkpoint + 测试杀进程恢复 |

### 0.1 适用场景（四类）

| 场景 | 适用 | 章节 |
|------|------|------|
| 同构跨机房（MySQL→MySQL，整库搬家） | ✅ | §15.1 |
| 同构分库分表 | ✅ | §15.3 |
| 同构 schema 演进（字段拆分 / 类型升级 / 软删除补齐） | ✅ | §15.2 |
| 异构同步（MySQL→ES/CK/Mongo/TiDB/Doris） | ✅ | §15.1 |

### 0.2 明确不在不停机范围

| 不支持 | 原因 | 替代 |
|--------|------|------|
| 强一致（支付级 XA / 2PC） | 双写最终一致 + 对账补偿，不能保证毫秒级一致 | SAGA / TCC 单独方案 |
| 跨大版本字符集变更（utf8 → utf8mb4_0900） | 排序规则差异需预处理 | 外挂脚本预处理后再走本方案 |
| 表无主键 / 无 binlog | CDC 不可行 | 先补主键 / 开 binlog 再迁 |
| 业务直接 raw SQL 绕过 Repository | 无法拦截双写 | 先重构到 Repository 层再迁 |
| 单实例无副本的源库 | 全量扫描会拖垮 | 先加 ReadReplica |

---

## 1. 前置条件（硬门槛）

迁移开始前，**全部 ✅ 才允许进入 D0**。任意一项 ❌ 转为先决条件任务，处理完再来。

### 1.1 源库前置

```bash
# 1. binlog_format 必须 ROW
mysql -e "SHOW VARIABLES LIKE 'binlog_format'"
# Value 必须是 ROW，不能是 STATEMENT / MIXED

# 2. binlog_row_image 必须 FULL
mysql -e "SHOW VARIABLES LIKE 'binlog_row_image'"
# Value 必须是 FULL（保证 update 事件能拿到完整 before/after）

# 3. gtid_mode 建议 ON（GTID 比 binlog pos 更稳）
mysql -e "SHOW VARIABLES LIKE 'gtid_mode'"

# 4. binlog 保留时长足够全量耗时 + 安全余量
mysql -e "SHOW VARIABLES LIKE 'binlog_expire_logs_seconds'"
# 建议 ≥ (全量耗时 + 24h)，亿级表至少 86400 * 3 = 3 天

# 5. 表必须有主键
mysql -e "SELECT TABLE_NAME FROM information_schema.tables t LEFT JOIN information_schema.key_column_usage k
ON t.table_name=k.table_name AND k.constraint_name='PRIMARY' WHERE t.table_schema='webook' AND k.table_name IS NULL"
# 输出空表 = OK

# 6. ReadReplica 准备好
mysql -h readonly.host -e "SHOW SLAVE STATUS\G"
# Slave_IO_Running + Slave_SQL_Running = Yes，Seconds_Behind_Master < 5
```

### 1.2 业务前置

```bash
# 1. 业务写入全部经过 Repository 层（无 raw SQL 绕过）
grep -rn "db.Exec\|db.Raw" webook/internal/web/ webook/internal/service/
# 输出空 = OK；任何匹配先重构到 Repository

# 2. webook-migrator SDK 已发布且业务侧已 import（NoOp 模式）
grep -rn "migratorsdk" webook/wire.go
# 至少有 ProvideNoOpSwitchReader / ProvideNoOpDualWriter
```

### 1.3 基础设施前置

| 组件 | 检查 | 期望 |
|------|------|------|
| Canal 集群 | `curl canal:11111/metrics` | 至少 3 节点，无单点 |
| Kafka *(仅 `sinkType=kafka`)* | `kafka-topics.sh --list` 含目标 topic | topic 已建（CDC 传输不经 Kafka，走进程内 channel） |
| Redis | `redis-cli ping` | PONG，5min 不丢连接 |
| webook-migrator | `curl migrator:8083/health` | `{"status":"ok","service":"migrator"}` |
| Prometheus | `curl prom:9090/api/v1/targets` | webook-migrator job up=1 |
| Grafana 告警 | 看板 `migrator-overview` 已加载 | 4 panel 都有数据 |

### 1.4 容量预估

```
全量耗时（小时）≈ total_rows / (shard_count × per_shard_qps × 3600)

示例：1 亿行 article 表
  shard_count = 16
  per_shard_qps = 5000
  耗时 ≈ 100_000_000 / (16 × 5000 × 3600) ≈ 0.35h ≈ 21 min（极快，因为分片并行）
  实际加上限速 + Sink 反压，按 4h 估

  binlog 量 ≈ 日均 10GB
  16 个进程内 partition 并行消费 + Sink 攒批，日常增量轻松追平（峰值看 /lag）
```

| 表规模 | shard | per_shard_qps | 预计全量耗时 | binlog/日 |
|--------|-------|---------------|------------|----------|
| 千万级 | 8 | 5k | 30 min | 1-3 GB |
| 亿级 | 16 | 5k | 4-6 h | 5-15 GB |
| 十亿级 | 64 | 3k（限速） | 24-48 h | 50-100 GB |

### 1.5 SLO 设定

迁移开始前与业务方约定 SLO，写入 `task` 任务备注：

| SLO | 目标 | 触发动作 |
|-----|------|---------|
| 业务 P99 RT 增量 | < 5ms | 超过暂停切流 |
| 业务 5xx 错误率 | < 0.01% | 超过 0.05% 自动 gray=0 |
| 增量 lag P99 | < 30s | 超过 5min 阻断 cutover |
| 对账 mismatch_rate | < 0.01% | 超过自动暂停 + 告警 DBA |

---

## 2. 整体路线图（典型 12 天）

```
                业务请求 ── 全程不中断 ──────────────────────────────────────────────────────────────►
                
D-3 ─── D-1   D0 ─────── D1 ─────── D2 ─────── D3 ─────── D4 ─────── D5 ── ── ── ── ── ── ── D11   D12
   │           │           │           │           │           │           │                       │
准备           创建任务     全量完成    采样对账    灰度切读     cutover     观察期 7 天             收尾
+ SDK 接入     + 启动全量   + 启动增量  + verify    + 5%→50%     + 双人      + 监控 + 对账           + OLD
+ NoOp 默认    + binlog                + 修复差异   + 100%       复核        （DST_ONLY 不可逆）      下线
                position                                          + 双写
                持久化                                            过渡 30s
                                                                  + 单写
                                                                  NEW
```

### 2.1 时间线核心动作表

| 日期 | 阶段 | 主要动作 | 风险 | 回滚成本 |
|------|------|---------|------|---------|
| D-3 ~ D-1 | 准备 | 前置检查 / SDK 接入 / 资源准备 | 0 | 0（未启动） |
| D0 早 | 创建任务 | POST /tasks | 0 | 手动废弃（v1 无 DELETE 端点） |
| D0 晚 | 启动全量 | POST /start full | 源库压力 | pause + 限速 |
| D1 | 全量完成 | 等 progress=100 | binlog 失效 | 重启全量 |
| D1 晚 | 启动增量 | POST /start incr | Canal 不通 | 重启 Canal |
| D2 | 采样对账 | POST /verify sample | 差异多 | 走 repair |
| D3 早 | gray=5 | POST /gray 5 | 5xx 上升 | gray=0 秒级 |
| D3 中 | gray=50 | POST /gray 50 | RT 上升 | gray=0 秒级 |
| D3 晚 | gray=100 | POST /gray 100 | NEW 抗不住 | gray=0 秒级 |
| D4 | cutover | POST /switch {action:approve} | 写丢失 | DST_FIRST 过渡期 rollback 秒级；DST_ONLY 后不可逆 |
| D5 ~ D11 | 观察期 | 监控 + 对账（DST_ONLY 已不可逆） | NEW 异常需前向修复 | ❌ 不可回滚 |
| D12 | 收尾 | 手动下线 OLD（v1 无 closed API，task 停在 switched） | OLD 下线 | 不可回滚 |

---

## 3. 阶段 0：准备（D-3 ~ D-1）

### 3.1 D-3：业务侧 SDK 接入

**目标**：业务代码引入 `migratorsdk`，默认 NoOp 模式（业务零感知）。

**操作**：

```bash
# 在 webook/ 工作目录
cd webook

# 1. 引入 SDK 包到要迁移的 Repository（以 user 为例）
# 编辑 internal/repository/user.go（按 §15.2 case study 改造）

# 2. ioc 注入（默认 NoOp）
# 编辑 ioc/migrator_sdk.go：
#   func ProvideSwitchReader() migratorsdk.SwitchReader {
#       return migratorsdk.NewNoOpSwitchReader()
#   }

# 3. 重生成 wire
wire ./...

# 4. 编译验证
go build ./...
go vet ./...

# 5. 跑全量测试
go test ./...

# 6. goimports
goimports -local github.com/webook -w .

# 7. 提交（NoOp 上线，业务零感知）
git add -A
git commit -m "chore(sdk): import migratorsdk in NoOp mode for upcoming migration"
git push
```

**预期**：
- 业务编译通过
- 测试全绿
- 部署到 dev 后 RT P99 与之前 diff < 1ms（实际是 0，NoOp 透传）

**回滚**：直接 git revert 这次 commit，几分钟事。

### 3.2 D-2：webook-migrator 部署

**目标**：迁移控制服务 webook-migrator 上线，控制库 5 张表创建。

**操作**：

```bash
# 1. 创建独立控制库
mysql -h db-host -e "CREATE DATABASE webook_migrator DEFAULT CHARSET utf8mb4 COLLATE utf8mb4_0900_ai_ci"

# 2. DDL 应用 5 张表（事先评审过，按 architecture.md §4.1）
mysql -h db-host webook_migrator < webook/migrator/scripts/migrator.sql

# 3. 部署 webook-migrator（按 deploy/.env.prod 配置 MIGRATOR_IMAGE_TAG）
cd deploy
./deploy.sh prod

# 4. 健康检查
curl http://migrator.internal:8083/health
# {"status":"ok","service":"migrator"}

# 5. Prometheus 指标接入验证
curl http://migrator.internal:8083/metrics | grep -E 'webook_http_|webook_db_|go_goroutines'
# 至少有 webook_http_* / webook_db_* / go_* 基础设施 metric

# 6. Grafana 看板加载验证
# 浏览器打开 grafana.internal/d/migrator-overview，4 个 panel 都能渲染
```

**预期**：
- 服务 up，健康检查全绿
- 控制库 5 张表存在（task / checkpoint / validate_log / audit_log / dead_letter），结构与设计一致
- Prometheus + Grafana 接入

**回滚**：`./deploy.sh prod down webook-migrator`，不影响业务。

### 3.3 D-1：前置条件全量验证

**目标**：执行 §1 全部 checklist，全部 ✅ 才允许进 D0。

**操作**：

```bash
# 跑 webook-migrator 提供的预检脚本
curl -X POST http://migrator.internal:8083/migrator/preflight \
  -H "Content-Type: application/json" \
  -d '{
    "sourceDsnRef": "vault:webook/db/source",
    "tables": ["article", "published_article"]
  }'

# 期望返回：
# {
#   "code": 0,
#   "data": {
#     "binlog_format": "ROW ✓",
#     "binlog_row_image": "FULL ✓",
#     "gtid_mode": "ON ✓",
#     "binlog_expire": "604800s ✓",
#     "tables_with_pk": "all ✓",
#     "read_replica_lag": "1s ✓",
#     "ready": true
#   }
# }
```

**异常处理**：

| 失败项 | 处理 |
|--------|------|
| binlog_format != ROW | 改 my.cnf + 重启从库（不重启主库） |
| 表无主键 | 先 DDL 加 `id BIGINT AUTO_INCREMENT PRIMARY KEY` |
| ReadReplica lag > 5s | 先排查从库慢的原因，不能在这种状态做迁移 |
| binlog 保留 < 全量耗时 | 临时调大 `binlog_expire_logs_seconds` |

---

## 4. 阶段 1：全量同步（D0 ~ D1）

### 4.1 D0 早：创建任务

**目标**：在 `task` 表登记迁移任务，状态 = created。

**操作**（以 article→ES 为例，CDC 模式）：

```bash
TASK_ID=$(curl -sX POST http://migrator.internal:8083/migrator/tasks \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "article_to_es_v1",
    "mode": "cdc",
    "kind": "heterogeneous",
    "sourceDsnRef": "vault:webook/db/source",
    "sinkType": "es",
    "sinkDsnRef": "vault:webook/es/cluster",
    "tables": [
      {
        "src": "article",
        "dst": "article_v1",
        "partitionKey": "id",
        "filter": "deleted_at = 0",
        "transform": "article_to_es_doc",
        "sensitiveColumns": []
      }
    ],
    "consistency": "eventual"
  }' | jq -r '.data.taskId')

echo "TaskId = $TASK_ID"
```

**预期**：
- 返回 `code=0` + taskId
- `task` 表新增一行 status=0（created）
- `audit_log` 记录 actor/action=create

**回滚**：废弃任务

```bash
# v1 无 DELETE 端点：created 态任务直接弃用即可（未启动引擎，不影响业务）；
# 如需清理控制库记录，DBA 手动 UPDATE task SET deleted_at=<ts> WHERE id=$TASK_ID。
```

### 4.2 D0 晚：启动全量

**目标**：分片并行扫源库写 sink，断点续传持久化。

**操作**：

```bash
curl -X POST http://migrator.internal:8083/migrator/tasks/$TASK_ID/start \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"phase": "full"}'

# 立即返回（异步执行）
# 状态变为 status=1 (full_running)
```

### 4.3 全量内部时序

```
启动 full
    │
    ▼
1. 锁定 binlog 起始位点（重要！）
   SHOW MASTER STATUS  → (binlog_file=mysql-bin.000123, pos=4567)
   写入 checkpoint(phase=incr, cursor_kind=binlog_pos, cursor_value="...")
   ▼
2. 计算分片
   SELECT MIN(id), MAX(id) FROM article  → 1, 100_000_000
   分 16 片：[1,6_250_000], [6_250_001,12_500_000], ...
   每片写入一行 checkpoint(phase=full, shard_no=N)
   ▼
3. 16 个 worker 并行
   for shard in shards:
     while cursor < shard.max_id:
       rows = SELECT * FROM article WHERE id > cursor AND id <= cursor+1000 AND deleted_at=0 ORDER BY id
       Sink.Apply(transform(rows))
       UPDATE checkpoint SET cursor_value = last_id WHERE shard_no = N
   ▼
4. 全 shard 完成 → status=full_done
```

**为什么先记 binlog 位点再扫全量**：保证全量扫描完成后，从这个位点开始增量，不会漏数据。如果先扫全量再记位点，全量过程中产生的 binlog 就丢了。

### 4.4 监控（全量阶段必看）

```bash
# 实时进度
watch -n 5 'curl -sX GET http://migrator.internal:8083/migrator/tasks/$TASK_ID | jq ".data.checkpoints"'

# Grafana 看板：migrator-overview → 基础设施 panel（HTTP / DB / Redis）
# 全量进度看上面的 API checkpoints（v1 无业务 metric）
```

**关键指标**：

| 指标 | 期望 | 异常处理 |
|------|------|---------|
| `checkpoints[].progress_percent`（API） | 单调递增 | 不动 → 看 worker 日志 |
| checkpoint `cursor_value` / `updated_at`（mysql） | 持续推进 | 不动 → Sink 不通，查日志 |
| `mysql_global_status_threads_running`（源库） | < 50 | > 100 → 限速 |
| `mysql_global_status_innodb_buffer_pool_pct`（源库） | < 95% | > 95% → 限速 |
| `dead_letter` 行数（mysql） | 增长缓慢 | 飙升 → 查 sink 错误日志 |

### 4.5 全量异常处理

| 异常 | 现象 | 处理 |
|------|------|------|
| 源库压力上升 | threads_running 飙到 100+ | `POST /tasks/$ID/throttle {qps: 1000}` 临时限速 |
| Sink 写失败 | apply_qps 跌 0 + errors 飙 | 查 Sink 健康，恢复后自动续传（checkpoint 不动） |
| Worker 崩溃 | 单 shard 进度停滞 | webook-migrator 自动拉起；从 checkpoint 续传，无重复无丢失 |
| 网络断开 | 全部 worker 都停滞 | 等恢复，所有 worker 从 checkpoint 续传 |
| binlog 即将失效 | 监控告警 binlog_expire 余量 < 1h | 紧急加大 `binlog_expire_logs_seconds`；或全量失败重启（罕见） |

### 4.6 全量完成确认

```bash
curl -X GET http://migrator.internal:8083/migrator/tasks/$TASK_ID | jq '.data.task.status'
# 期望：2（full_done）

# checkpoint 表所有 shard.progress=100
mysql webook_migrator -e "SELECT shard_no, progress_percent FROM checkpoint WHERE task_id=$TASK_ID AND phase='full'"
# 16 行全部 progress_percent=100
```

---

## 5. 阶段 2：增量追平（D1 起，持续运行）

### 5.1 启动增量

**目标**：订阅源库 binlog 持续写 sink，lag P99 < 30s。

**操作**：

```bash
curl -X POST http://migrator.internal:8083/migrator/tasks/$TASK_ID/start \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"phase": "incr"}'

# 立即返回（持续后台运行）
# 状态 status=3 (incr_running)
```

### 5.2 增量内部时序

```
启动 incr
    │
    ▼
1. 读取 §4.3 step 1 持久化的 binlog 起始位点
   binlog_file=mysql-bin.000123, pos=4567
   ▼
2. CanalSource 订阅
   canal.connect(filter="webook.article")
   canal.seek(file=mysql-bin.000123, pos=4567)
   ▼
3. binlog 解析 → ChangeEvent → dispatcher 按 FNV(PK) % 16 路由到进程内 partChans[i]
   for event := range canal.events():
     partChans[ FNV(event.PK) % 16 ] <- event
   ▼
4. 16 个 incr worker 各消费自己的 partChan（进程内 channel，非 Kafka）
   for partition in [0..15]:
     batch = []
     for event := range partChans[partition]:
       batch.append(event)
       if len(batch) >= 1000 or 100ms elapsed:
         Sink.Apply(transform(batch))
         updateCheckpointForPartition(partition, lastBinlogPos)  // 持久化本 partition 位点到 MySQL
         batch = []
   ▼
5. 持续运行直到 cutover 切单写（DST_ONLY，status=switched）
```

### 5.3 lag 监控

```bash
curl -X GET http://migrator.internal:8083/migrator/tasks/$TASK_ID/lag

# 期望：
# {
#   "code": 0,
#   "data": {
#     "lagMs": 230,                   # 230 毫秒（健康）
#     "lastSyncAt": 1715234567890     # 最近一次 binlog 事件被 sink 的时刻
#   }
# }
```

**lag 计算公式**（重要）：

```
lag = now() - max(binlog_event_ts of last applied batch)

注意不是：
lag = now() - worker_last_processed_ts
（worker 卡住时这种算法看起来一切正常）
```

### 5.4 增量异常处理

| 异常 | 现象 | 处理 |
|------|------|------|
| Canal 进程崩溃 | apply_qps 跌零 | k8s 自动拉起；从持久化的 binlog pos 续传 |
| Kafka 不可达（仅 sinkType=kafka）| apply_qps 跌零 | 重启 broker；KafkaSink.Apply 失败的行进 dead_letter，恢复后 replay-dl |
| 单 partition lag 飙升 | lag P99 异常 | 看是不是大事务（业务批量删除 / 全表 update）；正常情况会追平 |
| binlog 中断 | 源库 binlog 不再产生 | 检查源库是否在做大事务 / 主从切换 |
| 重复消费 | 同一 PK 多次写 | Sink 用 Version 乐观锁（`VALUES(version) > version` 才覆盖）丢弃旧值，自动幂等 |
| 顺序错乱（极端） | 同一 PK 的 update→delete 错位 | 不会发生：dispatcher 按 PK FNV-hash 固定把同一行路由到同一 partition worker，单行有序 |

### 5.5 双写启动（仅 mode=dual_write 模式 / schema 演进场景）

**异构 / 跨机房 / 分库分表**：跳过这一步，CDC 直接搞定（switch_stage 保持 `SRC_ONLY`，业务无感）。

**schema 演进 / 强一致过渡**：业务侧 SDK 进入 `SRC_FIRST` 阶段：

```bash
curl -X POST http://migrator.internal:8083/migrator/tasks/$TASK_ID/switch \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -d '{"stage": "SRC_FIRST"}'

# 这一步会更新 Redis：migrator:stage:$TASK_NAME = "SRC_FIRST"（stage/gray key 按 taskName，需先 export TASK_NAME）
# 业务侧 SDK 下一次 ChooseSide / Write 调用读到新值，开始双写
```

业务侧效果：

```go
// 改前（NoOp 模式）
dualWriter.Write(ctx, "user_nickname_v2", fn)
// 等价于 fn(SideOld)，单写 OLD

// 改后（switch_stage=SRC_FIRST）
dualWriter.Write(ctx, "user_nickname_v2", fn)
// 等价于：
//   fn(SideOld)         // 同步执行，必成功
//   go fn(SideNew)      // 异步执行，失败 retry → 死信
```

---

## 6. 阶段 3：对账（D2 ~ D3）

### 6.1 增量期间对账（API 触发，可周期跑）

v1 对账是 API 触发的批对账：增量追平期间周期性调 `POST /tasks/:id/verify`（`mode=sample` 低成本巡检，cron 跑即可），引擎扫 src/dst 同 PK 采样池比对，差异落 `validate_log`。

**查 mismatch**：

```bash
curl -X GET "http://migrator.internal:8083/migrator/tasks/$TASK_ID/mismatch?repaired=0&offset=0&limit=50" \
  -H "Authorization: Bearer $ADMIN_TOKEN"

# 返回：
# {
#   "code": 0,
#   "data": {
#     "list": [
#       {
#         "id": 123,
#         "taskId": 1,
#         "direction": "src_to_dst",
#         "bizTable": "article",
#         "bizId": "99887",
#         "mismatchKind": "diff",
#         "diffDetail": "{\"src.title\":\"foo\",\"dst.title\":\"bar\"}",
#         "repaired": 0,
#         "createdAt": 1779953000000,
#         "repairedAt": 0
#       }
#     ],
#     "total": 1
#   }
# }
```

### 6.2 采样对账（D2，必跑）

**目标**：在切流前主动验证一致性。

**操作**：

```bash
curl -X POST http://migrator.internal:8083/migrator/tasks/$TASK_ID/verify \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -d '{
    "mode": "sample",
    "sampleRate": 0.01
  }'

# 异步执行；查询进度：
curl -X GET http://migrator.internal:8083/migrator/tasks/$TASK_ID | jq '.data.task.status'
# 对账期间 status 仍是 incr_running（v1 无独立 verifying 状态）
```

**预期**：1 亿表采样 1% = 100 万行，约 5-10 min；mismatch 数 < 100（mismatch_rate < 0.0001%）。

### 6.3 全量对账（T+1，可选但推荐）

亿级表 SQL JOIN 比对会拖死源库。本框架对接外部 Spark/Flink job：

```bash
# 假设你已有 Spark 集群
spark-submit \
  --class com.webook.migrator.FullVerify \
  webook-migrator-verify.jar \
  --task-id $TASK_ID \
  --src-jdbc "jdbc:mysql://source/webook" \
  --dst-jdbc "es://es-cluster/article_v1" \
  --output-jdbc "jdbc:mysql://migrator/webook_migrator"

# Spark job dump 源 + 目标到本地，按 PK 排序后 hash 比对
# 差异结果直接 INSERT 到 validate_log
```

### 6.4 mismatch 处理

| mismatch_rate | 决策 |
|---------------|------|
| < 0.001% | 合理范围（双写竞态 / 网络抖动），可继续 |
| 0.001% ~ 0.01% | **暂停切流**；走 repair 自动修复后重新 verify |
| > 0.01% | **必须停止**：可能 transform 写错或 Sink 漏处理 op；call DBA |

**自动修复**（src 覆盖 dst）：

```bash
curl -X POST http://migrator.internal:8083/migrator/tasks/$TASK_ID/repair \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -d '{
    "strategy": "src_overwrite_dst",
    "ids": [99887, 99892, 99895]
  }'
# strategy 可选：src_overwrite_dst（源覆盖目标）/ dst_overwrite_src（反向）/ mark_only（仅标记不修改）
```

---

## 7. 阶段 4：灰度切读（D3）

**前置条件**：
- ✅ status = incr_running
- ✅ lag P99 < 30s 持续 30 min
- ✅ verify mismatch_rate < 0.001%
- ✅ 业务侧 SDK 已部署且 ChooseSide 可被调用

### 7.1 5% 试水（10:00 ~ 10:30）

```bash
# 改 gray 比例
curl -X POST http://migrator.internal:8083/migrator/tasks/$TASK_ID/gray \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -d '{"percent": 5}'

# 这一步只是改 Redis：migrator:gray:$TASK_NAME = 5
# 业务侧 SwitchReader 下次调用立即生效（无 RPC，纯 Redis GET）
```

**监控（必看）**：

```bash
# 业务侧 5xx 错误率
watch -n 5 'curl -G http://prom:9090/api/v1/query --data-urlencode \
  "query=rate(webook_core_http_requests_total{status=~\"5..\"}[1m])"'

# 业务侧 P99 RT
watch -n 5 'curl -G http://prom:9090/api/v1/query --data-urlencode \
  "query=histogram_quantile(0.99, rate(webook_core_http_request_duration_seconds_bucket[1m]))"'

# Sink 侧延迟（v1 无 apply metric，查 /lag 的 dstLagMs）
watch -n 5 'curl -s http://migrator.internal:8083/migrator/tasks/$TASK_ID/lag | jq ".data.dstLagMs"'
```

**红线（任一触发立即 gray=0 回滚）**：

| 红线 | 阈值 |
|------|------|
| 业务 5xx 错误率 | > 0.05% 持续 1 min |
| 业务 P99 RT 增量 | > 50ms |
| Sink RT P99 | > 1s |
| 用户投诉 | 任何一条相关投诉 |

**回滚**（5 秒内）：

```bash
curl -X POST http://migrator.internal:8083/migrator/tasks/$TASK_ID/gray \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -d '{"percent": 0}'
```

### 7.2 50% 中试（10:30 ~ 11:30）

5% 试水持续 30 min 无异常 → 升 50%：

```bash
curl -X POST http://migrator.internal:8083/migrator/tasks/$TASK_ID/gray \
  -d '{"percent": 50}' \
  ...
```

监控同上。50% 持续 1h 是关键观察期：晚高峰流量足够压测 NEW。

### 7.3 100%（11:30 ~ 14:30）

```bash
curl -X POST http://migrator.internal:8083/migrator/tasks/$TASK_ID/gray \
  -d '{"percent": 100}' \
  ...
```

100% 全部走 NEW 至少 3 小时，覆盖一个完整流量周期（午高峰）。

### 7.4 灰度阶段总回滚

灰度期（SRC_FIRST 双写中）发现问题，单条命令秒级把读路由切回 OLD：

```bash
curl -X POST http://migrator.internal:8083/migrator/tasks/$TASK_ID/gray \
  -d '{"percent": 0}' \
  ...

# 业务下一次 ChooseSide 调用立即返回 SideOld
# 全部读切回 OLD，无数据丢失（双写从未停过）
```

---

## 8. 阶段 5：cutover 切写（D4）

### 8.1 cutover 前置硬门槛

**全部 ✅ 才能 cutover**。任意 ❌ 不允许进。

| 门槛 | 检查 |
|------|------|
| gray=100 持续 ≥ 24h | `task.gray_percent=100 AND updated_at < now()-86400000` |
| 全量对账通过 | 跑过 `verify mode=full`，mismatch_rate < 0.001% |
| 双人复核已签字 | UI 上两个独立 Admin 都点过"批准 cutover" |
| 应急联系人就位 | DBA / 后端 owner / SRE 一定在岗 |
| 当前是工作时间 | 禁止周末 / 节假日 / 凌晨 cutover |
| 业务无大促 / 大流量 | 业务方书面确认非高峰窗口 |

### 8.2 cutover 操作

```bash
# 1. 操作人 A 提请 cutover
curl -X POST http://migrator.internal:8083/migrator/tasks/$TASK_ID/switch \
  -H "Authorization: Bearer $ADMIN_TOKEN_A" \
  -H "X-Cutover-Approver: actor_a" \
  -d '{"stage": "DST_ONLY", "action": "propose"}'

# 这一步只是把 task 标为 pending_cutover，等第二个 admin 批准

# 2. 操作人 B 批准
curl -X POST http://migrator.internal:8083/migrator/tasks/$TASK_ID/switch \
  -H "Authorization: Bearer $ADMIN_TOKEN_B" \
  -H "X-Cutover-Approver: actor_b" \
  -d '{"stage": "DST_ONLY", "action": "approve"}'

# 系统验证 actor_a != actor_b，开始执行切流：SRC_FIRST → DST_FIRST → (30s) → DST_ONLY
```

### 8.3 cutover 内部时序（最关键的 2 分钟）

```
T-30s   预备（系统自动）
        - 锁定 Redis: migrator:stage:$TASK_NAME = "DST_FIRST"
        - 业务侧 SwitchReader 进入 DST_FIRST：读全切到 DST
        - 业务侧 DualWriter 进入 DST_FIRST：SRC + DST 同步双写（两侧都必成，避免空窗）

T-0     cutover 触发
        - 系统记 cutover_start_ts
        - 此时业务仍同步双写 SRC + DST（DST_FIRST 过渡，仍可 rollback）

T+30s   切单写（point of no return）
        - Redis: migrator:stage:$TASK_NAME = "DST_ONLY"
        - 业务侧 DualWriter.Write 改为单写 NEW
        - OLD 转只读、停止更新（业务侧 ChooseSide 永远返回 SideNew）

T+30s+  status = switched
        - 切流完成，DST_ONLY 不可逆
        - 进入观察期：监控 + 对账，确认 NEW 稳定
```

**为什么 30 秒过渡期**：避免「写入空窗」（§3.7 坑 3）。30s = binlog 同步 lag 的 P99 × 2。

### 8.4 cutover 监控

```bash
# 切流过程实时查看 stage
watch -n 1 'redis-cli get migrator:stage:'$TASK_NAME

# 切流后 lag 验证
curl -X GET http://migrator.internal:8083/migrator/tasks/$TASK_ID/lag
# 期望：data 里 srcLagMs / dstLagMs 正常
```

### 8.5 cutover 回滚（仅 DST_FIRST 过渡期内）

cutover 分两步：DST_FIRST（30s 同步双写过渡）→ DST_ONLY（单写）。**仅在 DST_FIRST 过渡期、尚未切单写时可回滚**；一旦进入 DST_ONLY 单写，OLD 停止更新、**不可回滚**（point of no return）：

```bash
curl -X POST http://migrator.internal:8083/migrator/tasks/$TASK_ID/switch \
  -H "Authorization: Bearer $ADMIN_TOKEN_A" \
  -d '{"stage": "SRC_FIRST", "action": "rollback"}'

# 系统执行：
# 1. switch_stage 落回 SRC_FIRST，gray 自动归零
# 2. 业务侧 SwitchReader/DualWriter 切回读 OLD（双写期 OLD 有全量数据）
# 3. 秒级生效，无需追平
```

**回滚耗时**：秒级（双写期 OLD 一直有全量数据）。进 DST_ONLY 单写后不可回滚，故 cutover 前必充分对账。

---

## 9. 阶段 6：观察期 + 收尾（D5 ~ D12）

### 9.1 D5 ~ D11：7 天观察期

DST_ONLY 已不可逆，观察期以**监控 + 对账**为主，确认 NEW 稳定：

| 指标 | 期望 | 异常处理 |
|------|------|---------|
| 业务 5xx 错误率 | < 0.01% | 排查并前向修复 NEW（OLD 已停滞，不可回滚） |
| 业务 P99 RT | 与切流前持平 | 调查 NEW 性能问题 |
| sink 写延迟 P99 | < 100ms | 调 NEW 容量 |
| verify 对账 | mismatch=0 | 走 repair 修复 NEW |

观察期内发现问题只能**前向修复 NEW**（DST_ONLY 后 OLD 停滞、已不可回滚）——这正是 cutover 前必须充分对账的原因。

### 9.2 D12：收尾 / OLD 下线

**前置**：
- 7 天无业务异常
- 业务方书面确认 NEW 已稳定

**操作**（v1 无 `closed` action/API，均为运维手动步骤；task 记录停在 `switched`）：

```bash
# 1. 确认 NEW 稳定、对账 mismatch=0 后，DBA 归档下线 OLD 表（见 §9.3）
# 2. 如需收尾控制库记录：手动 UPDATE task SET deleted_at=<ts> WHERE id=$TASK_ID
# 3. 30 天后清理 validate_log 历史数据（见 §9.3）
```

### 9.3 收尾后的清理（一次性，不影响业务）

```bash
# 1. （schema 演进）DDL 删除旧字段
mysql webook -e "ALTER TABLE user DROP COLUMN nickname"
# 注意：必须确认业务代码已经不再读 nickname（grep -rn 验证）

# 2. （SDK 清理）业务代码移除 dualWriter / switchReader 包装
# 一次 PR 把所有调用从 dualWriter.Write(...) 改回 dao.Insert(...)
# 删除 ioc/migrator_sdk.go 的 RedisSwitchReader 注入，回到 NoOp

# 3. （异构）下线 OLD 库 / OLD 表
mysql webook -e "DROP TABLE article_old"
# 或下线整个 OLD 实例

# 4. 控制库清理（30 天后）
mysql webook_migrator -e "DELETE FROM validate_log WHERE task_id=$TASK_ID"
```

---

## 10. 三大坑的完整解法

### 10.1 坑 1：旧 binlog 覆盖新值

**完整时序**：

```
T0   业务: dualWriter.Write → fn(SideOld)
       OLD.UPDATE article SET title='v1' WHERE id=1
       (binlog event ts=1000ms, content: title=v1)
       
T1   业务: dualWriter.Write → fn(SideNew)
       NEW.UPDATE article SET title='v2' WHERE id=1
       (NEW 当前 title=v2)
       
T2   CDC 把 OLD 的 t=1000ms binlog 同步到 NEW
       NEW Sink 收到: UPDATE article SET title='v1' WHERE id=1
       
       ❌ 错误实现：直接 UPDATE → NEW 退化到 v1
       ✅ 正确实现：UPDATE article SET title='v1' WHERE id=1 AND updated_at <= 1000
                   row_affected=0（NEW.updated_at 已经是 1100），跳过这条 binlog
```

**Sink 实现**：

```go
// pipeline/sink/mysql.go
func (s *MySQLSink) Apply(ctx context.Context, batch []sink.Mutation) error {
    return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
        for _, m := range batch {
            switch m.Op {
            case "insert", "update":
                // 关键：updated_at 乐观锁
                cols := m.Cols
                cols["updated_at"] = m.Cols["updated_at"]
                err := tx.Table(m.Table).
                    Where("id = ? AND updated_at <= ?", m.PK, m.Cols["updated_at"]).
                    Updates(cols).Error
                if err != nil { return err }
                // 如果 row_affected = 0 + 不是 missing，说明 NEW 已有更新值，丢弃这次写入是对的
            case "delete":
                err := tx.Table(m.Table).
                    Where("id = ? AND updated_at <= ?", m.PK, m.Cols["updated_at"]).
                    Delete(nil).Error
                if err != nil { return err }
            }
        }
        return nil
    })
}
```

**ES Sink 等价实现**：用 `if_seq_no` + `if_primary_term` 或 `version_type=external`。

### 10.2 坑 2：read-your-write 破坏

**完整时序**：

```
T0   业务: dualWriter.Write → fn(SideOld)
       OLD.UPDATE user SET nickname='Alice' WHERE id=42
       
T1   CDC: OLD binlog 还没同步到 NEW（lag 200ms）
       NEW.user.id=42 仍是旧 nickname
       
T2   业务: switchReader.ChooseSide(ctx, "user_nickname_v2", 42)
       ❌ 错误实现：随机分流（hash(req_id) % 100）
                  此次落到 SideNew → 读 NEW → nickname 还是旧值
                  用户感受：刚改完名字就消失了
       
       ✅ 正确实现：按 user_id 分流（hash(42) % 100）
                  hash(42) = 0x123abc → 0x123abc % 100 = N
                  if N < gray%: SideNew else SideOld
                  对同一个 user_id=42，hash 永远一样，路由永远固定一侧
```

**SwitchReader 实现**：

```go
// migratorsdk/switch_reader.go
type RedisSwitchReader struct {
    cmd  redis.Cmdable
    hash hash.Hash64
}

func (r *RedisSwitchReader) ChooseSide(ctx context.Context, taskName string, hashKey int64) (Side, error) {
    grayKey := fmt.Sprintf("migrator:gray:%s", taskName)
    grayStr, err := r.cmd.Get(ctx, grayKey).Result()
    if errors.Is(err, redis.Nil) {
        return SideOld, nil  // 缓存 miss 走 safe 默认
    }
    if err != nil {
        return SideOld, nil  // Redis 不可用走 safe 默认
    }
    gray, _ := strconv.Atoi(grayStr)
    
    // 关键：固定 hash 函数 + user_id（不是 req_id / ts）
    h := fnv.New64a()
    h.Write([]byte(fmt.Sprintf("%s:%d", taskName, hashKey)))
    bucket := int(h.Sum64() % 100)
    
    if bucket < gray {
        return SideNew, nil
    }
    return SideOld, nil
}
```

**为什么 hashKey 必须是 user_id 不是 req_id**：req_id 每次都不一样，会导致同一用户随机分流；user_id 是稳定的，保证同一用户固定一侧。

### 10.3 坑 3：cutover 切写空窗

**完整时序**：

```
T-30s  switch_stage: SRC_FIRST → DST_FIRST
       业务侧 DualWriter 切到同步双写：fn(SideOld) + fn(SideNew)（两侧都必成）
       业务侧 SwitchReader 读全切到 DST
       
T-0    DST_FIRST（continued）
       系统记 cutover_start_ts = now()
       业务仍同步双写（switch_stage 仍为 DST_FIRST，仍可 rollback）
       
T+30s  switch_stage: DST_FIRST → DST_ONLY
       业务侧切到单写：只 fn(SideNew)
       OLD 不再接受新写入、转只读
       
T+30s起 DST_ONLY 不可逆
        OLD 停滞，进 DST_ONLY 后不可回滚
        （回滚窗口在 DST_FIRST 过渡期之前）
```

**业务侧 DualWriter 实现**：

```go
// migratorsdk/dual_writer.go
type RedisDualWriter struct {
    cmd redis.Cmdable
}

func (w *RedisDualWriter) Write(ctx context.Context, taskName string, fn func(Side) error) error {
    stageKey := fmt.Sprintf("migrator:stage:%s", taskName)
    stage, err := w.cmd.Get(ctx, stageKey).Result()
    if errors.Is(err, redis.Nil) || err != nil {
        return fn(SideOld)  // 默认 / Redis 不可用走 SideOld
    }
    
    switch stage {
    case "SRC_FIRST":
        // 双写过渡：SRC 必成，DST 异步 retry
        if err := fn(SideOld); err != nil {
            return err
        }
        go w.asyncWriteNew(ctx, fn, taskName)
        return nil
        
    case "DST_FIRST":
        // cutover 30s 过渡期：SRC + DST 同步双写，两侧都必成（避免空窗）
        if err := fn(SideOld); err != nil {
            return err
        }
        return fn(SideNew)
        
    case "DST_ONLY":
        // 单写 DST
        return fn(SideNew)
        
    default:
        // SRC_ONLY 或未知 stage 走 safe 默认
        return fn(SideOld)
    }
}

func (w *RedisDualWriter) asyncWriteNew(ctx context.Context, fn func(Side) error, taskName string) {
    backoff := 100 * time.Millisecond
    for i := 0; i < 3; i++ {
        if err := fn(SideNew); err == nil {
            return
        }
        time.Sleep(backoff)
        backoff *= 2
    }
    // 三次失败入死信队列
    w.deadLetter.Send(ctx, DeadLetter{Task: taskName, Func: fn})
}
```

---

## 11. 关键代码片段

### 11.1 业务侧最小接入示例（user repository）

**改前**（无 SDK）：

```go
// webook/internal/repository/user.go
type cachedUserRepository struct {
    dao   dao.UserDAO
    cache cache.UserCache
}

func (r *cachedUserRepository) Update(ctx context.Context, u domain.User) error {
    if err := r.dao.Update(ctx, dao.User{
        Id:       u.Id,
        Nickname: u.Nickname,
    }); err != nil {
        return err
    }
    return r.cache.Del(ctx, u.Id)
}

func (r *cachedUserRepository) FindById(ctx context.Context, id int64) (domain.User, error) {
    if u, err := r.cache.Get(ctx, id); err == nil {
        return u, nil
    }
    u, err := r.dao.FindById(ctx, id)
    if err != nil {
        return domain.User{}, err
    }
    _ = r.cache.Set(ctx, u.ToDomain())
    return u.ToDomain(), nil
}
```

**改后**（接入 SDK，未启用时 NoOp 透传）：

```go
type cachedUserRepository struct {
    dao          dao.UserDAO
    cache        cache.UserCache
    dualWriter   migratorsdk.DualWriter
    switchReader migratorsdk.SwitchReader
}

func (r *cachedUserRepository) Update(ctx context.Context, u domain.User) error {
    err := r.dualWriter.Write(ctx, "user_nickname_v2", func(side migratorsdk.Side) error {
        if side == migratorsdk.SideOld {
            return r.dao.UpdateLegacy(ctx, dao.User{
                Id: u.Id, Nickname: u.Nickname,  // 写 nickname
            })
        }
        return r.dao.UpdateV2(ctx, dao.User{
            Id: u.Id, NicknameV2: u.Nickname,    // 写 nickname_v2
        })
    })
    if err != nil {
        return err
    }
    return r.cache.Del(ctx, u.Id)
}

func (r *cachedUserRepository) FindById(ctx context.Context, id int64) (domain.User, error) {
    if u, err := r.cache.Get(ctx, id); err == nil {
        return u, nil
    }
    u, err := r.dao.FindById(ctx, id)
    if err != nil {
        return domain.User{}, err
    }
    side, _ := r.switchReader.ChooseSide(ctx, "user_nickname_v2", id)
    domainUser := u.ToDomain()
    if side == migratorsdk.SideNew {
        domainUser.Nickname = u.NicknameV2  // 切读后取新字段
    }
    _ = r.cache.Set(ctx, domainUser)
    return domainUser, nil
}
```

### 11.2 ioc 注入

```go
// webook/ioc/migrator_sdk.go
package ioc

import (
    "github.com/redis/go-redis/v9"
    "github.com/webook/internal/migratorsdk"
)

// 默认：NoOp 模式（业务零感知）
func ProvideSwitchReader(cmd redis.Cmdable) migratorsdk.SwitchReader {
    if cmd == nil {  // 没配 Redis 直接 NoOp
        return migratorsdk.NewNoOpSwitchReader()
    }
    // 启用时改这里：
    // return migratorsdk.NewRedisSwitchReader(cmd)
    return migratorsdk.NewNoOpSwitchReader()
}

func ProvideDualWriter(cmd redis.Cmdable, dl migratorsdk.DeadLetter) migratorsdk.DualWriter {
    if cmd == nil {
        return migratorsdk.NewNoOpDualWriter()
    }
    return migratorsdk.NewNoOpDualWriter()  // 启用时改 NewRedisDualWriter
}
```

启用迁移时改这两行返回 Redis 实现，重生成 wire，重新部署即可。

### 11.3 Sink 幂等示例（MySQL → MySQL）

```go
// webook/migrator/pipeline/sink/mysql.go
type MySQLSink struct {
    db *gorm.DB
}

func (s *MySQLSink) Apply(ctx context.Context, batch []sink.Mutation) error {
    if len(batch) == 0 {
        return nil
    }
    return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
        // 按 PK 排序：避免死锁
        sort.Slice(batch, func(i, j int) bool { return batch[i].PK < batch[j].PK })
        
        for _, m := range batch {
            switch m.Op {
            case "insert", "update":
                // 幂等 UPSERT + 乐观锁
                err := tx.Table(m.Table).Clauses(clause.OnConflict{
                    DoUpdates: clause.AssignmentColumns(keysOf(m.Cols)),
                    Where:     clause.Where{Exprs: []clause.Expression{
                        clause.Lte{Column: "updated_at", Value: m.Cols["updated_at"]},
                    }},
                }).Create(m.Cols).Error
                if err != nil { return err }
            case "delete":
                err := tx.Table(m.Table).
                    Where("id = ? AND updated_at <= ?", m.PK, m.Cols["updated_at"]).
                    Delete(nil).Error
                if err != nil { return err }
            }
        }
        return nil
    })
}
```

---

## 12. 监控与告警

### 12.1 v1 实际 metric（基础设施层）

> v1 不埋业务 metric；业务进度 / lag / 对账走控制台 API（`/lag` · `/tasks/:id` · `/mismatch`）。

```
# HTTP（ginx prometheus 中间件）
webook_http_request_duration_seconds{job="webook-migrator"}  histogram  P99 / QPS / 5xx
# DB / Redis（pkg/gormx·redisx/prometheus）
webook_db_*  /  webook_redis_*                               连接池 / 查询 / 命令延迟
# Go runtime
go_goroutines / go_memstats_*                                goroutine 数 / 内存
```

业务进度 / lag / 对账 v1 通过控制台 API 查（非 metric）：
- 全量进度：`GET /tasks/:id` → `data.checkpoints[].progress_percent`
- 增量延迟：`GET /tasks/:id/lag` → `{lagMs, srcLagMs, dstLagMs}`
- 对账差异：`GET /tasks/:id/mismatch` / `verify` 结果
- 死信：`GET /tasks/:id` detail / mysql 查 `dead_letter`

### 12.2 Grafana 看板（migrator-overview）

| Panel | 内容 |
|-------|------|
| 服务存活 | `up{job="webook-migrator"}` |
| HTTP P99 / QPS / 5xx | `webook_http_*` 时序 |
| DB / Redis | `webook_db_*` / `webook_redis_*` |
| Goroutines / 内存 | `go_goroutines` / `go_memstats_*` |

### 12.3 告警规则（migrator.yml）

```yaml
groups:
- name: webook-migrator
  rules:
  
  - alert: MigratorServiceDown
    expr: up{job="webook-migrator"} == 0
    for: 1m
    labels:
      severity: P0
    annotations:
      summary: "webook-migrator 服务不可用"
      
  - alert: Migrator5xxHigh
    expr: rate(webook_http_requests_total{job="webook-migrator",code=~"5.."}[5m]) > 0.05
    for: 3m
    labels:
      severity: P1
    annotations:
      summary: "webook-migrator 5xx 错误率 > 5%"

  - alert: MigratorP99High
    expr: histogram_quantile(0.99, rate(webook_http_request_duration_seconds_bucket{job="webook-migrator"}[5m])) > 1
    for: 5m
    labels:
      severity: P1
    annotations:
      summary: "webook-migrator HTTP P99 > 1s"

  - alert: MigratorGoroutinesHigh
    expr: go_goroutines{job="webook-migrator"} > 10000
    for: 5m
    labels:
      severity: P2
    annotations:
      summary: "webook-migrator goroutines > 10000（疑似泄漏）"
```

> 业务级告警（lag / mismatch / dead_letter）依赖 v2 业务 metric；v1 靠 §13 演练 + oncall 巡检 API（`/lag`、`/mismatch`、`/tasks/:id`）+ runbook 覆盖。

---

## 13. 故障演练（D-1 必跑，cutover 前必跑）

每次切流前一周内必须跑过这 7 项演练（第 7 项见 `retros/README.md` 配套），缺一不可。每项演练记录到 [`./retros/`](./retros/)，按 [TEMPLATE.md](./retros/TEMPLATE.md) 填写；填好的示例见 [2026-04-canal-failure.md](./retros/2026-04-canal-failure.md)。

演练时按对应 runbook 走恢复流程：

| # | 演练 | 关联 runbook |
|---|------|------------|
| 1 | Canal 故障 | [runbooks/canal-failure.md](./runbooks/canal-failure.md) |
| 2 | Kafka 故障（仅 sinkType=kafka）| [runbooks/kafka-broker-down.md](./runbooks/kafka-broker-down.md) |
| 3 | Sink 故障 | [runbooks/sink-unreachable.md](./runbooks/sink-unreachable.md) |
| 4 | webook-migrator 崩溃 | [runbooks/migrator-service-down.md](./runbooks/migrator-service-down.md) |
| 5 | 切读回滚 | （无单独 runbook，直接 gray=0） |
| 6 | cutover 回滚 | [runbooks/cutover-rollback.md](./runbooks/cutover-rollback.md) |

### 13.1 Canal 故障演练

```bash
# 1. 当前 lag 基线
curl -X GET .../tasks/$TASK_ID/lag  # 期望 lag < 1s

# 2. 杀 Canal 主节点
docker stop canal-master

# 3. 观察告警 5 min 后触发
# Grafana 看板：lag 曲线开始上升

# 4. 重启 Canal
docker start canal-master

# 5. 验证从 checkpoint 续传
curl -X GET .../tasks/$TASK_ID/lag  # 几分钟内回到 < 1s

# 6. 跑一次 verify 确认无丢数据
curl -X POST .../tasks/$TASK_ID/verify -d '{"mode":"sample","sampleRate":0.01}'
# 期望：mismatch 增量 < 100
```

**通过标准**：lag 恢复 < 5 min，无数据丢失。

### 13.2 Kafka 故障演练（仅 sinkType=kafka）

> v1 CDC 传输是 canal → 进程内 channel partition，**不经 Kafka**——一般迁移（MySQL→MySQL / ES / Mongo）无需此演练。
> 仅当 `sinkType=kafka`（Kafka 作目标）时，broker 挂 = 一次 Sink 故障：

```bash
# 杀一个 Kafka broker（Kafka 作 sink 时）
docker stop kafka-broker-2

# 观察：KafkaSink.Apply 失败 → 失败行进 dead_letter（与 §13.3 同兜底）
# 重启 broker 后用 replay-dl 重放死信
docker start kafka-broker-2
```

### 13.3 Sink 故障演练

```bash
# 模拟 ES 集群故障
docker stop elasticsearch

# 观察 dead_letter 表行数开始累积（mysql 查）
# webook-migrator 日志：sink apply failed: connection refused 持续刷

# 恢复 ES
docker start elasticsearch

# 验证 dead_letter 自动重放
curl -X POST .../tasks/$TASK_ID/replay-dl
```

### 13.4 webook-migrator 进程崩溃演练

```bash
# 在 cutover 关键路径上 kill webook-migrator
docker kill webook-migrator

# k8s / docker-compose 自动拉起
# 验证从 checkpoint 续传
# 验证 cutover 流程能继续完成
```

### 13.5 切读回滚演练

```bash
# gray=50%
curl -X POST .../tasks/$TASK_ID/gray -d '{"percent":50}'

# 等 5 min

# 立刻回滚
curl -X POST .../tasks/$TASK_ID/gray -d '{"percent":0}'

# 验证业务侧 ChooseSide 立即返回 SideOld
# 验证业务 5xx 错误率无突增
```

### 13.6 cutover 回滚演练（最关键）

```bash
# 在 staging 环境的真实 task 上演练（不能在 prod）
# cutover
curl -X POST .../tasks/$TASK_ID/switch -d '{"stage":"cutover"}'

# 在 DST_FIRST 30s 过渡期内（尚未切单写）触发 rollback
curl -X POST .../tasks/$TASK_ID/switch -d '{"stage":"SRC_FIRST","action":"rollback"}'

# 验证：
# 1. switch_stage 回到 SRC_FIRST，gray 归零
# 2. 业务侧切回读 OLD（双写期 OLD 有全量数据）
# 3. 数据一致性：跑全量 verify
```

**通过标准**：DST_FIRST 过渡期 rollback 秒级回到 SRC_FIRST，OLD/NEW 一致。

---

## 14. 完整 Runbook（一页纸命令序列）

```bash
#!/bin/bash
# 不停机迁移完整命令序列
# 任务：article 表同步到 ES（CDC 异构）
# 用法：按章节顺序执行，每节命令不要跨节合并

set -e
TASK_NAME="article_to_es_v1"
BASE="http://migrator.internal:8083/migrator"
AUTH="Authorization: Bearer $ADMIN_TOKEN"


# ============ D-1 前置检查 ============
curl -X POST $BASE/preflight -H "$AUTH" \
  -d '{"sourceDsnRef":"vault:webook/db/source","tables":["article"]}'

# ============ D0 创建任务 ============
TASK_ID=$(curl -sX POST $BASE/tasks -H "$AUTH" \
  -d '{"name":"'$TASK_NAME'","mode":"cdc","kind":"heterogeneous","sourceDsnRef":"vault:webook/db/source","sinkType":"es","sinkDsnRef":"vault:webook/es/cluster","tables":[{"src":"article","dst":"article_v1","partitionKey":"id","filter":"deleted_at = 0","transform":"article_to_es_doc"}]}' \
  | jq -r '.data.taskId')
echo "TASK_ID=$TASK_ID"

# ============ D0 启动全量 ============
curl -X POST $BASE/tasks/$TASK_ID/start -H "$AUTH" -d '{"phase":"full"}'

# ============ 等全量完成（监控）============
while true; do
  STATUS=$(curl -sX GET $BASE/tasks/$TASK_ID -H "$AUTH" | jq -r '.data.task.status')
  echo "status=$STATUS"
  [ "$STATUS" = "2" ] && break  # full_done
  sleep 60
done

# ============ D1 启动增量 ============
curl -X POST $BASE/tasks/$TASK_ID/start -H "$AUTH" -d '{"phase":"incr"}'

# ============ D2 采样对账 ============
curl -X POST $BASE/tasks/$TASK_ID/verify -H "$AUTH" -d '{"mode":"sample","sampleRate":0.01}'

# ============ D2 全量对账（外接 Spark）============
spark-submit --class com.webook.migrator.FullVerify webook-migrator-verify.jar \
  --task-id $TASK_ID

# ============ D3 灰度切读 ============
curl -X POST $BASE/tasks/$TASK_ID/gray -H "$AUTH" -d '{"percent":5}'
sleep 1800   # 30 min

curl -X POST $BASE/tasks/$TASK_ID/gray -H "$AUTH" -d '{"percent":50}'
sleep 3600   # 1 h

curl -X POST $BASE/tasks/$TASK_ID/gray -H "$AUTH" -d '{"percent":100}'
sleep 86400  # 24 h

# ============ D4 cutover（双人复核） ============
# 操作人 A
curl -X POST $BASE/tasks/$TASK_ID/switch \
  -H "Authorization: Bearer $ADMIN_TOKEN_A" \
  -d '{"stage":"DST_ONLY","action":"propose"}'

# 操作人 B
curl -X POST $BASE/tasks/$TASK_ID/switch \
  -H "Authorization: Bearer $ADMIN_TOKEN_B" \
  -d '{"stage":"DST_ONLY","action":"approve"}'

# ============ D5-D11 观察期（无操作，仅监控）============

# ============ D12 收尾 / OLD 下线（v1 无 closed API，手动）============
# 确认 NEW 稳定后 DBA 归档下线 OLD 表；task 停在 switched（见 §9.2/§9.3）

echo "迁移完成"

# ============ 应急回滚（仅双写期 SRC_FIRST/DST_FIRST）============
# gray rollback（读切回 OLD，双写期任意 gray 比例可用）：
# curl -X POST $BASE/tasks/$TASK_ID/gray -H "$AUTH" -d '{"percent":0}'

# cutover rollback（DST_FIRST → SRC_FIRST；DST_ONLY 单写后不可逆，会被拒）：
# curl -X POST $BASE/tasks/$TASK_ID/switch -H "$AUTH" -d '{"stage":"SRC_FIRST","action":"rollback"}'
```

---

## 15. 两个端到端 Case Study

### 15.1 Case A：article 表同步到 ES（异构 CDC）

**驱动**：当前 `article_search.go` 同步阻塞业务，ES 故障会拖垮发布。

**目标**：业务只写 MySQL，ES 由 webook-migrator CDC 异步同步。

**完整时间线**：

| 日 | 动作 | 验证 |
|----|------|------|
| D-3 | webook 接入 SDK（NoOp） | RT P99 不变 |
| D-2 | webook-migrator 部署 | health up |
| D-1 | preflight + drill 全跑 | 7 项演练全过 |
| D0 早 | 创建 task `article_to_es_v1`（mode=cdc） | task 表新增 |
| D0 晚 | 启动 full | progress 开始递增 |
| D1 上午 | full_done | 16 shard 全 100% |
| D1 下午 | 启动 incr | lag P99 < 1s |
| D2 | sample verify | mismatch_rate < 0.0001% |
| D3 | gray=5→50→100 | 业务监控全绿 |
| **此处特殊** | **业务侧 PR：删除 `article_search.go.Upsert` 调用** | go test 全绿；NoOp DualWriter 不参与 |
| D4 | cutover | DST_ONLY 不可逆 |
| D5-D11 | 观察期 | 业务 RT P99 ↓ 30ms（解耦 ES） |
| D12 | 收尾 | OLD 表下线 |
| D12+ | 清理：删除 webook 主仓的 `searchDAO.Upsert` 路径相关代码 | 一次性 PR |

**业务收益**：
- 文章发布 P99 RT 50ms → 10ms
- ES 故障不再拖垮文章发布
- 关键路径外部依赖 2 → 1

### 15.2 Case B：user.nickname 字段从 varchar(50) 拆到 nickname_v2 varchar(255)（同构 schema 演进）

**驱动**：业务发现 nickname 长度不够；亿级表直接 ALTER 锁元数据。

**目标**：双写过渡 + 切读 + 下线旧字段。

**完整时间线**：

| 日 | 动作 | 验证 |
|----|------|------|
| D-3 | DDL: `ALTER TABLE user ADD COLUMN nickname_v2 VARCHAR(255) NOT NULL DEFAULT ''`（INSTANT 算法，秒级） | 0 锁表 |
| D-3 | webook 接入 SDK（NoOp）+ DAO 加 `UpdateLegacy` / `UpdateV2` 双方法 | RT 不变 |
| D-2 | webook-migrator 部署 | health up |
| D-1 | preflight + drill | 全过 |
| D0 早 | 创建 task `user_nickname_v2`（mode=`dual_write`，即应用层双写机制） | task 表新增 |
| D0 晚 | 启动 full（把 nickname → nickname_v2） | full 跑完 |
| D1 上午 | full_done | nickname_v2 已等价 nickname |
| D1 下午 | 启动 incr（同时把后续 nickname 更新同步到 nickname_v2） | lag < 1s |
| D1 晚 | 启动 `SRC_FIRST` 阶段（业务开始双写两字段） | 业务监控正常 |
| D2 | sample verify | < 0.0001% |
| D3 | gray=5→50→100（读 nickname_v2） | 监控正常 |
| D4 | cutover（业务单写 nickname_v2，DST_ONLY 不可逆） | switched |
| D5-D11 | 观察期 | 无异常 |
| D12 | 收尾 | OLD 列 nickname 可下线 |
| D12+ | 清理 PR：业务代码删除 `UpdateLegacy` / dualWriter 包装；DDL `DROP COLUMN nickname` | 一次性 |

**关键 DDL 时机**：
- D-3 加 `nickname_v2`：INSTANT，0 锁
- D12+ 删 `nickname`：必须确认无业务读引用（grep 验证），不允许误删

---

## 16. FAQ

### Q1: 全量过程中业务还在写，会丢数据吗？

A: 不会。全量启动前已经记录了 binlog 起始位点（§4.3 step 1）。全量结束后启动增量，从这个位点开始，所有全量过程中产生的写都被 binlog 重放到 sink，不会丢。

### Q2: 增量 lag 一直在 5 min 以上，能切流吗？

A: 不能。lag 高说明：(1) Sink 写入跟不上；(2) Canal 订阅吞吐瓶颈；(3) 大事务卡住。先排查再切流。

### Q3: 双写时 NEW 失败了，业务会卡住吗？

A: 不会。OLD 必成功后 NEW 异步 retry，最多 3 次失败入死信队列。业务可见性只看 OLD 是否成功。

### Q4: 切读时如果某个用户读到了 NEW，但 NEW 还没收到他刚才的更新，怎么办？

A: 不会发生。按 `hash(user_id) % 100` 路由保证同一用户固定一侧。如果他在 SideNew，他写也走 NEW（走双写策略），读自然能读到自己的写。

### Q5: cutover 切单写前（DST_FIRST 过渡期）发现问题能回滚吗？

A: 能，秒级回 SRC_FIRST（双写期 OLD 有全量数据）。但切到 DST_ONLY 单写后不可回滚（见 Q6）。

### Q6: cutover（DST_ONLY）之后还能回滚吗？

A: 不能。DST_ONLY 单写后 OLD 停止更新，不可回滚（point of no return）。回滚窗口只在双写期（SRC_FIRST / DST_FIRST 过渡）。这就是为什么 cutover 前要充分对账 + 双人复核。

### Q7: 两个 task 同时跑会冲突吗？

A: 不会。每个 task 独立 Source/Sink/Redis key（sinkType=kafka 时各用独立 topic）。但要注意 Canal 集群带宽。

### Q8: 如何处理大事务（如批量删除 100 万行）？

A: binlog 仍然按 row 解析，会产生 100 万个 ChangeEvent。dispatcher 按 PK 分到 16 个进程内 partition，每 partition 6.25 万事件。Sink 攒批 1000 条/100ms 处理，约 1-2 min 追平。这期间 lag 短暂上升。

### Q9: 切流时业务发版会不会冲突？

A: 会。切流期间禁止业务发版，避免 SDK 行为不一致。窗口期约定写入"业务封板期 D-1 ~ D+12"。

### Q10: 强一致场景（订单、支付）能用吗？

A: 不能。本方案默认最终一致。强一致场景需要 SAGA / TCC + 同步 2PC，不在本框架范围（§16）。

### Q11: 千万级表能用同样的流程吗？

A: 能，但参数调整：shard 数 4-8 即可，per_shard_qps 10k；总耗时约 10-30 min。整体周期可压缩到 3-5 天。

### Q12: 十亿级表呢？

A: 全量 worker 内置容量不够，建议外挂 Spark/Flink 跑全量，框架对接 sink 部分。增量仍走 Canal。整体周期 4-6 周。

### Q13: 没有 Redis 怎么切流？

A: 用 etcd / consul 等 KV store 替换。SwitchReader 接口的 Redis 实现换成 etcd 实现即可，业务侧无感知。

### Q14: 如果 webook-migrator 自身挂了，业务受影响吗？

A: 不影响主流程。SDK 默认 safe 行为：Redis 不可用时 ChooseSide 返回 SideOld，DualWriter 退化为单写 OLD。webook-migrator 拉起后从 checkpoint 续传，不丢数据但有 lag。

### Q15: cutover 切单写（DST_ONLY）后发现 NEW 有问题怎么办？

A: 只能**前向修复 NEW**（DST_ONLY 后 OLD 已停滞、不可回滚）。所以进 DST_ONLY 前必须充分对账（mismatch=0）+ 双人复核，把风险挡在双写期。双写期（SRC_FIRST / DST_FIRST 过渡）才是回滚窗口。
