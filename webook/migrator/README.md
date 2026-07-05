# webook-migrator 手动测试手册

本手册分四块：**0 前置** → **A 核心生命周期（MySQL→MySQL 同构）** → **B 异构方向（任意源框架）** → **C 参考**。A 段按 A1→A12 顺序走一遍即覆盖全部 14 endpoint + 切流状态机 + 故障路径；B 段演示任意源框架的三种异构方向。每步可独立运行。

## 目录

- **0. 前置准备** — 工具 / 起 MySQL+Redis / 建库 / 建业务表 / 起服务（go run + docker）
- **A. 核心生命周期（MySQL → MySQL 同构）** — A1→A12 走完 14 endpoint
  - A1 健康检查 · A2 创建任务 · A3 列表/详情 · A4 Preflight · A5 全量同步 · A6 增量 cdc
  - A7 暂停 · A8 对账 · A9 修复差异 · A10 灰度 · A11 切流状态机 · A12 死信重放
- **B. 异构方向（sourceType × sinkType 自由组合）**
  - B1 Mongo → MySQL（全量 find + 增量 Change Stream）
  - B2 MySQL → Mongo（全量 + 增量 canal）
  - B3 MySQL → Elasticsearch（异构 sink + 真异构对账）
- **C. 参考** — C1 功能边界 · C2 错误码 · C3 清理 · C4 故障排查 · C5 自动化测试入口
- **附录** — 业务侧 SDK 接入自测（webook-core 集成）

---

## 0. 前置准备

### 0.1 工具

需要本机装好：

- Go 1.25+
- `mysql` 客户端
- `redis-cli`
- `curl` 或 [HTTPie](https://httpie.io)（任选）

### 0.2 启动 MySQL + Redis

```bash
cd C:/Go/work/webook
./deploy.sh local
# 等 10 秒，docker 起 MySQL :3306 + Redis :6379
```

### 0.3 建库

```bash
mysql -uroot -p13520 -e "CREATE DATABASE IF NOT EXISTS webook_migrator CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci"
mysql -uroot -p13520 -e "CREATE DATABASE IF NOT EXISTS webook_migrator_test CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci"
```

5 张控制库表（task / checkpoint / validate_log / audit_log / dead_letter）首次启动 migrator 时 GORM AutoMigrate 自动建。

> 注：GORM AutoMigrate 只建/补字段，**不会**修改/删除已有索引；改索引定义需手跑 ALTER（以 `scripts/migrator.sql` 为准）。

### 0.4 建业务表（demo 用）

后面 A5 全量同步 + A8 对账要用：

```bash
mysql -uroot -p13520 webook_migrator <<SQL
SET NAMES utf8mb4;

CREATE TABLE IF NOT EXISTS article (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    title VARCHAR(255) NOT NULL,
    content TEXT
) ENGINE=InnoDB CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS article_v1 (
    id BIGINT PRIMARY KEY,
    title VARCHAR(255) NOT NULL,
    content TEXT
) ENGINE=InnoDB CHARSET=utf8mb4;

INSERT INTO article (id, title, content) VALUES
    (1, '第一篇', '内容 a'),
    (2, '第二篇', '内容 b'),
    (3, '第三篇', '内容 c');
SQL
```

### 0.5 跑自动化测试（baseline）

```bash
cd webook && go test ./migrator/...
# 期望：15 包全 ok（含新增 pipeline/transform；Mongo e2e 无副本集时自动 skip）
```

### 0.6 启动 migrator 服务（go run 开发模式）

```bash
cd webook/migrator
APP_ENV=config/local.yaml go run .
```

期望日志：

```
[migrator] config loaded: config/local.yaml
[migrator] logger config: ...
[migrator] listening on :8030
```

服务监听 `:8030`。下面所有 curl 都打这个端口。

### 0.7 启动 migrator 服务（docker compose 容器模式 — 类生产）

> 适合验收部署链路 + 监控 / nginx / CI 是否串通；中间件 + webook-core + webook-chat + webook-migrator 一键全起。

```bash
cd C:/Go/work/deploy
./deploy.sh local         # 等价 docker compose -p webook-local --env-file .env.local \
                          #     -f docker-compose.yaml -f docker-compose.local.yaml up -d
```

`./deploy.sh local` 模式特点：

- 所有 Go 服务 build 本地镜像（`webook-migrator:local` 等），不拉 ghcr
- 暴露宿主端口（`MIGRATOR_HOST_PORT=8030` 等），方便 IDE 调试
- 中间件容器自动 healthy 后再起业务（compose `depends_on.condition`）

验证容器跑起来：

```bash
docker ps --filter "name=webook-migrator" --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}"
# webook-migrator   Up X minutes (healthy)   0.0.0.0:8030->8030/tcp

docker logs webook-migrator --tail 20 | grep "listening on"
# [migrator] listening on :8030
```

通过 nginx 反代访问（仅内网/白名单 IP；`/api/migrator/*` 剥前缀转 :8030）：

```bash
curl -sS http://localhost/api/migrator/tasks   # → 经 nginx → webook-migrator:8030/migrator/tasks
```

监控 / CI 验收快查：

| 检查 | 命令 / 入口 |
|------|------------|
| Prometheus 抓到 migrator 指标 | `curl -s http://localhost:9090/api/v1/targets \| jq '.data.activeTargets[] \| select(.labels.job=="webook-migrator")'` 期望 `health=up` |
| Grafana 看板 | 浏览器开 `http://localhost:3001` (admin/admin) → Dashboards → "Webook Migrator（服务详情）" |
| Grafana 告警规则 | `http://localhost:3001/alerting/list` 期望看到 `webook-migrator-up / -5xx-rate / -p99 / -goroutines` 4 条 |
| GitHub Actions 触发条件 | 改 `webook/migrator/**` 或 `webook/pkg/**` 推 main / feature 分支自动跑 `.github/workflows/webook-migrator-ci.yml`；推 `webook-migrator-v*.*.*` tag 自动 push ghcr image |

切回 go run 模式调试：先 `docker compose -p webook-local stop webook-migrator` 让容器停掉，再 `go run .` 占用 8030 端口。

---

## A. 核心生命周期（MySQL → MySQL 同构）

> 同构迁移完整生命周期，按 A1 → A12 顺序走一遍即覆盖全部 14 endpoint + 切流状态机 + 故障路径。

---

## A1 健康检查

新开一个终端，先确认服务存活：

```bash
curl -sS http://localhost:8030/health
# 期望：{"service":"migrator","status":"ok"}

curl -sS http://localhost:8030/metrics | head -5
# 期望：Prometheus exposition 格式（# HELP / # TYPE / 指标数值）
```

---

## A2 创建任务（POST /migrator/tasks）

```bash
curl -sS -X POST http://localhost:8030/migrator/tasks \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "art_schema_v1",
    "mode": "cdc",
    "kind": "schema",
    "sourceDsnRef": "vault:src",
    "sinkType": "mysql",
    "sinkDsnRef": "vault:dst",
    "tables": [{"src": "article", "dst": "article_v1", "partitionKey": "id"}]
  }'
```

**期望响应**：

```json
{"code":0,"msg":"","data":{"taskId":1}}
```

**验证落表**：

```bash
mysql -uroot -p13520 webook_migrator -e "SELECT id, name, mode, kind, sink_type, status FROM task"
# 1 | art_schema_v1 | cdc | schema | mysql | 0

mysql -uroot -p13520 webook_migrator -e "SELECT id, task_id, actor, action, result, payload FROM audit_log"
# 1 | 1 | anonymous | create | success | {"name":"art_schema_v1",...,"sourceDsnRef":"***","sinkDsnRef":"***",...}
```

注意 `payload` 中 `sourceDsnRef` 和 `sinkDsnRef` 被 mask 成 `***`（中间件自动脱敏）。

**把上面响应里的 `taskId` 存到 shell 变量供 A3-A12 复用**（首次跑应为 1；如果控制库已有其他 task，会递增到 2/3/...）：

```bash
TID=1        # 用上面 A2 响应里实际的 taskId 替换
```

---

## A3 列表 + 详情（GET）

```bash
# 列表
curl -sS 'http://localhost:8030/migrator/tasks?offset=0&limit=10' | jq
# 期望：{"code":0,"data":{"list":[{TaskVO}],"total":1}}

# 详情
curl -sS http://localhost:8030/migrator/tasks/$TID | jq
# 期望：{"code":0,"data":{"id":1,"name":"art_schema_v1","mode":"cdc",...}}
# 注意：TaskVO 屏蔽了 sourceDsnRef / sinkDsnRef / tables_json（敏感字段不出网）
```

---

## A4 Preflight（飞行前检查）

```bash
curl -sS -X POST http://localhost:8030/migrator/preflight \
  -H 'Content-Type: application/json' \
  -d '{"sourceDsnRef":"vault:src","tables":["article"]}' | jq
```

**期望响应**（本地 docker MySQL,deploy/docker-compose.yaml 已开 `--binlog-format=ROW`）：

```json
{"code":0,"data":{"binlog_format":"ROW","gtid_mode":"OFF","ready":true,"read_replica_lag":0,"tables_with_pk":["article"],"tables_missing_pk":[]}}
```

> **ready 判断**:`binlog_format=ROW` + 每张表有 PK 即 true,**不强制 gtid_mode=ON**。
> 当前 migrator 的 `BinlogClient` 用 binlog file/pos 续订（`Subscribe(ctx, fromPos)`),
> **不依赖 MySQL 服务端 gtid_mode**。`gtid_mode` 字段在响应里是**信息性**(运维感知),不进 ready 判断。
>
> 未来真要走 GTID 续订模式（强一致跨机房场景）需要:
> - 代码侧:`BinlogClient.Subscribe` 加回 `fromGTID` 参数 + `GoMySQLCanalClient` 真实现 GTID 续订
> - 服务端:MySQL `--gtid-mode=ON --enforce-gtid-consistency=ON`
> - 那时 Preflight ready 判断把 gtid_mode=ON 加进去
>
> Preflight 通过 `dsn.Resolver` 连源库查 `@@global.binlog_format` / `@@global.gtid_mode` / `information_schema.STATISTICS` 表 PK,
> v1 默认 `StaticResolver`（控制库自闭环模式）实际查的是控制库（`webook_migrator`)，所以 `tables_with_pk` 看的是该库的 `article` 表 PK（README §0.4 已建）。生产用 `PerTaskResolver` 时按 `task.SourceDsnRef` 解 Vault 拿真源库连接，查的就是真源端的表 PK。

---

## A5 启动全量同步（POST /start phase=full）

```bash
curl -sS -X POST http://localhost:8030/migrator/tasks/$TID/start \
  -H 'Content-Type: application/json' \
  -d '{"phase":"full","shards":[{"no":0,"pkMin":1,"pkMax":100,"batchSz":1000}]}'
# 期望：{"code":0,"data":{"started":"full"}}（立即返回，异步跑）

# 等 2 秒让 goroutine 跑完
sleep 2

# 验证 dst 表数据
mysql -uroot -p13520 webook_migrator -e "SELECT id, title FROM article_v1 ORDER BY id"
# 期望：3 行（1=第一篇 / 2=第二篇 / 3=第三篇），从 article 表同步过来

# 验证 checkpoint
mysql -uroot -p13520 webook_migrator -e "SELECT task_id, phase, shard_no, cursor_kind, cursor_value FROM checkpoint"
# 期望：1 | full | 0 | id_range | 3（lastPK=3 表示已扫到 id=3）
```

---

## A6 增量同步 cdc（POST /start phase=incr）

> **前置**：docker-compose 已起 canal-ready MySQL（开 binlog + 建 canal 用户）；
> `config/local.yaml` 已配 `migrator.canal.{addr,user,password,serverIdBase,flavor}`。
>
> 升级老 MySQL 容器需要让新参数（开 binlog）+ init 脚本（建 canal 用户）生效。**有两种方式,推荐方式 A 不清数据**：
>
> ### 方式 A（推荐,保留所有数据）：手动建 canal 用户 + restart 容器
>
> ```bash
> # 1. 手动跑 canal 用户初始化 SQL（不依赖 init/*.sql,因为 init 只在首次启动跑）
> mysql -h 127.0.0.1 -P 3306 -uroot -p13520 < C:/Go/work/deploy/mysql/init/01-canal-user.sql
>
> # 2. restart MySQL 容器让 docker-compose 的新 command（--log-bin --binlog-format=ROW ...）生效
> docker compose -p webook-local restart webook-mysql
>
> # 3. 验证 binlog 已开 + canal 用户能登
> mysql -h 127.0.0.1 -P 3306 -uroot -p13520 -e "SHOW VARIABLES LIKE 'binlog_format'"  # ROW
> mysql -h 127.0.0.1 -P 3306 -ucanal -pcanal -e "SHOW MASTER STATUS"                   # File / Position 非空
> ```
>
> ### 方式 B（暴力,**会清掉所有库的数据**）：删 volume 让 init 重跑
>
> 🔴 **警告**：`mysql-data` volume 是**整个 MySQL 实例 datadir**,删除会一起清掉:
> - `webook` 库（主仓 article / user 数据）
> - `webook_chat` 库（chat 历史）
> - `webook_migrator` 库（迁移任务 / 控制库）
>
> 如果机器上有调试数据要保留,**先 dump**:
> ```bash
> # 备份所有库（保命操作）
> docker exec webook-mysql mysqldump -uroot -p13520 --all-databases > /tmp/webook-backup-$(date +%Y%m%d).sql
> ```
>
> 然后才执行清 volume：
> ```bash
> cd C:/Go/work/deploy
> docker compose -p webook-local stop webook-mysql
> docker volume rm webook-local_mysql-data  # ⚠️ 清掉所有库
> ./deploy.sh local
> # 重启后 README §0.3 / §0.4 的建库 + 建业务表 SQL 要重跑
> # 想恢复主仓数据:docker exec -i webook-mysql mysql -uroot -p13520 < /tmp/webook-backup-XXXXX.sql
> ```

### A6.1 启动 cdc 增量订阅

```bash
curl -sS -X POST http://localhost:8030/migrator/tasks/$TID/start \
  -H 'Content-Type: application/json' \
  -d '{"phase":"incr"}'
# 期望：{"code":0,"data":{"started":"incr"}}（立即返回；canal 后台订阅 binlog）

# 看 task 状态(IncrEngine 正常退出前保持 incr_running)
curl -sS http://localhost:8030/migrator/tasks/$TID | jq .data.status
# 期望：3 (IncrRunning)
```

### A6.2 写源表触发 binlog → 验证同步到 dst

```bash
# INSERT 一行 article(id=4)
mysql -uroot -p13520 webook_migrator -e \
  "INSERT INTO article (id, title, content) VALUES (4, '第四篇增量', '通过 binlog 同步')"

# 等 binlog event 经 canal 流到 IncrEngine → Sink
sleep 2

# article_v1 应该出现 id=4
mysql -uroot -p13520 webook_migrator -e "SELECT id, title FROM article_v1 WHERE id=4"
# 期望: 4 | 第四篇增量

# UPDATE 一行
mysql -uroot -p13520 webook_migrator -e "UPDATE article SET title='第一篇 v2' WHERE id=1"
sleep 2
mysql -uroot -p13520 webook_migrator -e "SELECT id, title FROM article_v1 WHERE id=1"
# 期望: 1 | 第一篇 v2

# DELETE 一行
mysql -uroot -p13520 webook_migrator -e "DELETE FROM article WHERE id=4"
sleep 2
mysql -uroot -p13520 webook_migrator -e "SELECT id FROM article_v1 WHERE id=4"
# 期望: 空(已删)
```

### A6.3 验证 checkpoint 位点推进

```bash
mysql -uroot -p13520 webook_migrator -e \
  "SELECT task_id, phase, shard_no, cursor_kind, cursor_value FROM checkpoint WHERE phase='incr'"
# 期望: 1 | incr | 0 | binlog_pos | mysql-bin.000001/<不断增长的 pos 值>
```

### A6.4 查看 lag

```bash
curl -sS http://localhost:8030/migrator/tasks/$TID/lag | jq
# 期望: {srcLagMs: <小,通常 < 1000ms>, dstLagMs: <小>, lagMs: <同 srcLagMs>}
```

### A6.5 暂停 cdc

```bash
curl -sS -X POST http://localhost:8030/migrator/tasks/$TID/pause
# 期望: {"code":0,"data":{"paused":true}}
# (与 A7 走的是同一 endpoint;cdc task 在跑时 pause 真触发 ctx cancel)
```

### 故障排查

| 现象 | 原因 | 修法 |
|------|------|------|
| `start incr` 返 500 + log `migrator.canal.addr 未配置` | yaml 没配 canal 段 | 跟 README §0.x 的 yaml 模板对齐 |
| `task.status=-1` 立即变 Failed | canal 连不上 MySQL master | 容器内 `mysql -h webook-mysql -ucanal -pcanal -e "SHOW MASTER STATUS"` 测;无 master status 说明 binlog 没开 |
| binlog stream 起来但 article_v1 没变更 | `binlog_row_image != FULL` 让 update event 缺 before 字段 | `SHOW VARIABLES LIKE 'binlog_row_image'` 应为 FULL;不对则改 my.cnf 重启 |
| 撞 ServerID | 多 canal 实例同 server_id 抢同一 binlog 槽位 | `migrator.canal.serverIdBase` 改成跟其他 canal/replica 不冲突的值 |

---

## A7 暂停（POST /pause）

```bash
curl -sS -X POST http://localhost:8030/migrator/tasks/$TID/pause
# 期望（任务已跑完，无活动 goroutine）：{"code":409,"msg":"task not running (no full / incr engine paused)"}
```

如果 A5 还在跑（大数据量场景），这里会成功停掉。

---

## A8 对账（POST /verify + GET /mismatch）

> **机制**：对账分两件事 —— `POST /verify` **只返差异计数** `{mismatchCount}` 作概览；差异明细逐条落
> `validate_log` 表（差异可能成千上万条，不塞进 response）。三件套各司其职：
>
> | 步骤 | 职责 |
> |------|------|
> | `POST /verify` | 对账 → 差异写 `validate_log` → 返 `{mismatchCount}` |
> | `GET /mismatch` | 分页拉 `repaired=0` 的差异明细（默认 50/页，见 §A8.3）|
> | `POST /repair` | 按 `validate_log.id` 修复（见 A9）|
>
> `mismatch_kind` 三种：`missing`（目标缺行）/ `extra`（目标多行）/ `diff`（同 PK 字段值不同）。
> **dst 若还没跑过 full sync，src 全部行都会报 `missing`** —— 这是如实对账，不是 bug。

### A8.1 制造差异

> **幂等设计**：三条 SQL 反复跑都安全。`UPDATE` 天然幂等；`DELETE` 不存在的行返 0 rows
> 不报错；`REPLACE INTO` 替代 `INSERT` 让 id=99 已存在时变成"覆盖"（mark_only 后再跑 8.1 不会撞主键）。

```bash
# 删 dst 一行，造一个 missing
mysql -uroot -p13520 webook_migrator -e "DELETE FROM article_v1 WHERE id=2"

# 改 dst 一行，造一个 diff
mysql -uroot -p13520 webook_migrator -e "UPDATE article_v1 SET title='被篡改' WHERE id=3"

# 加 dst 一行 src 没有的，造一个 extra（REPLACE 让重复跑不撞 PK）
mysql -uroot -p13520 webook_migrator -e "REPLACE INTO article_v1 (id, title) VALUES (99, '幽灵')"
```

### A8.2 全量对账

```bash
curl -sS -X POST http://localhost:8030/migrator/tasks/$TID/verify \
  -H 'Content-Type: application/json' \
  -d '{"mode":"full"}'
# 期望：{"code":0,"data":{"mismatchCount":3}}
```

### A8.3 查差异列表

```bash
curl -sS "http://localhost:8030/migrator/tasks/$TID/mismatch?offset=0&limit=10" | jq
# 期望：list 3 行
#   bizId="2" mismatchKind="missing"
#   bizId="3" mismatchKind="diff"  diffDetail 含 "diff_fields":["title"]
#   bizId="99" mismatchKind="extra"
```

### A8.4 采样对账

```bash
curl -sS -X POST http://localhost:8030/migrator/tasks/$TID/verify \
  -H 'Content-Type: application/json' \
  -d '{"mode":"sample","sampleRate":0.5}'
# 期望：mismatchCount 是 PK hash 落入 50% 采样池的差异数（约 1-2 个）

# 参数越界
curl -sS -X POST http://localhost:8030/migrator/tasks/$TID/verify \
  -H 'Content-Type: application/json' -d '{"mode":"sample","sampleRate":0}'
# 期望：{"code":400,"msg":"采样率必须在 (0, 1] 之间"}
```

---

## A9 修复差异（POST /repair）

```bash
# 拿前面 verify 落下的 validate_log id
MISMATCH_IDS=$(mysql -uroot -p13520 webook_migrator -sNe "SELECT id FROM validate_log WHERE task_id=$TID AND repaired=0"  | tr -d '\r' | tr '\n' ',' | sed 's/,$//')
echo "待修复 ids: $MISMATCH_IDS"

# mark_only：只标记不动数据
curl -sS -X POST http://localhost:8030/migrator/tasks/$TID/repair \
  -H 'Content-Type: application/json' \
  -d "{\"strategy\":\"mark_only\",\"ids\":[$MISMATCH_IDS]}"
# 期望：{"code":0,"data":{"repaired":3}}

# 验证
mysql -uroot -p13520 webook_migrator -e "SELECT id, repaired, repaired_at FROM validate_log WHERE task_id=$TID"
# 期望：所有 repaired=1，repaired_at 有值

# src_overwrite_dst：真用 src 端数据覆盖 dst（修复差异让两侧一致）
# 注：需要先重新 verify 产生新的 repaired=0 行（mark_only 之后旧行已 repaired=1）
curl -sS -X POST http://localhost:8030/migrator/tasks/$TID/verify \
  -H 'Content-Type: application/json' -d '{"mode":"full"}'
NEW_IDS=$(mysql -uroot -p13520 webook_migrator -sNe "SELECT id FROM validate_log WHERE task_id=$TID AND repaired=0"  | tr -d '\r' | tr '\n' ',' | sed 's/,$//')
curl -sS -X POST http://localhost:8030/migrator/tasks/$TID/repair \
  -H 'Content-Type: application/json' \
  -d "{\"strategy\":\"src_overwrite_dst\",\"ids\":[$NEW_IDS]}"
# 期望：{"code":0,"data":{"repaired":3}}
# 验证：dst 表回归 src 一致状态
mysql -uroot -p13520 webook_migrator -e "SELECT id, title FROM article_v1 ORDER BY id"
# 期望：1 第一篇 / 2 第二篇 / 3 第三篇（id=2 回来 / id=3 还原 / id=99 已删）
```

---

## A10 灰度比例（POST /gray）

```bash
# 设 50%
curl -sS -X POST http://localhost:8030/migrator/tasks/$TID/gray \
  -H 'Content-Type: application/json' \
  -d '{"percent":50}'
# 期望：{"code":0,"data":{"gray":50}}

# 验证 Redis（路由决策源）+ MySQL（冗余持久化）
redis-cli -a 13520 GET "migrator:gray:art_schema_v1"
# 期望：50

mysql -uroot -p13520 webook_migrator -e "SELECT gray_percent FROM task WHERE id=1"
# 期望：50

# 越界
curl -sS -X POST http://localhost:8030/migrator/tasks/$TID/gray \
  -H 'Content-Type: application/json' -d '{"percent":200}'
# 期望：{"code":400,"msg":"灰度比例必须在 0-100 之间"}
```

---

## A11 切流状态机（POST /switch）— 核心场景

### A11.1 完整 4 阶段推进

```bash
# 当前 stage 默认 SRC_ONLY（Redis 没存就是 SRC_ONLY）
redis-cli -a 13520 GET "migrator:stage:art_schema_v1"      # nil

# 11.1.1 SRC_ONLY → SRC_FIRST
curl -sS -X POST http://localhost:8030/migrator/tasks/$TID/switch \
  -H 'Content-Type: application/json' \
  -d '{"stage":"SRC_FIRST"}'
# 期望：{"code":0,"data":{"stage":"SRC_FIRST"}}
redis-cli -a 13520 GET "migrator:stage:art_schema_v1"      # SRC_FIRST

# 11.1.2 SRC_FIRST → DST_FIRST
curl -sS -X POST http://localhost:8030/migrator/tasks/$TID/switch \
  -H 'Content-Type: application/json' -d '{"stage":"DST_FIRST"}'
# 期望：{"code":0,"data":{"stage":"DST_FIRST"}}

# 11.1.3 DST_FIRST → DST_ONLY（双人复核）
#   第一步：propose
curl -sS -X POST http://localhost:8030/migrator/tasks/$TID/switch \
  -H 'Content-Type: application/json' \
  -d '{"stage":"DST_ONLY","propose":"alice"}'
# 期望：{"code":412,"msg":"未提议或提议已过期，请先 propose"}
# （propose 已记录到 Redis，等 approve）
redis-cli -a 13520 GET "migrator:cutover_propose:art_schema_v1"   # alice

#   第二步：approve（必须不同 actor）
curl -sS -X POST http://localhost:8030/migrator/tasks/$TID/switch \
  -H 'Content-Type: application/json' \
  -d '{"stage":"DST_ONLY","approve":"bob"}'
# 期望：{"code":0,"data":{"stage":"DST_ONLY"}}
redis-cli -a 13520 GET "migrator:stage:art_schema_v1"              # DST_ONLY
redis-cli -a 13520 GET "migrator:cutover_propose:art_schema_v1"    # nil（已消费）
```

### A11.2 双人同 actor 拦截

```bash
# 全程用 task=$TID。当前在 DST_ONLY → rollback 回 SRC_FIRST，再按顺序推到 DST_FIRST 后测同 actor。
curl -sS -X POST http://localhost:8030/migrator/tasks/$TID/switch \
  -H 'Content-Type: application/json' \
  -d '{"action":"rollback","stage":""}'
# 期望：{"code":0,"data":{"stage":"SRC_FIRST"}}

# 按顺序推到 DST_FIRST（前置条件，否则状态机会先拦）
curl -sS -X POST http://localhost:8030/migrator/tasks/$TID/switch \
  -H 'Content-Type: application/json' \
  -d '{"stage":"DST_FIRST"}'
# 期望：{"code":0,"data":{"stage":"DST_FIRST"}}

# 推 DST_ONLY 但 propose=approve=alice
curl -sS -X POST http://localhost:8030/migrator/tasks/$TID/switch \
  -H 'Content-Type: application/json' \
  -d '{"stage":"DST_ONLY","propose":"alice","approve":"alice"}'
# 期望：{"code":409,"msg":"propose 和 approve 必须是不同用户"}
```

### A11.3 跳级拦截

```bash
# 先 rollback 到 SRC_FIRST（清掉上一步的 DST_FIRST 状态）
curl -sS -X POST http://localhost:8030/migrator/tasks/$TID/switch \
  -H 'Content-Type: application/json' \
  -d '{"action":"rollback","stage":""}'
# 期望：{"code":0,"data":{"stage":"SRC_FIRST"}}

# 跳级 SRC_FIRST → DST_ONLY（跨过 DST_FIRST）
curl -sS -X POST http://localhost:8030/migrator/tasks/$TID/switch \
  -H 'Content-Type: application/json' \
  -d '{"stage":"DST_ONLY","propose":"alice","approve":"bob"}'
# 期望：{"code":409,"msg":"状态机非法转移","metadata":{"from":"SRC_FIRST","to":"DST_ONLY","allowed":"SRC_ONLY→SRC_FIRST→DST_FIRST→DST_ONLY"}}
# Msg 固定 sentinel 文本；动态 from/to 在 metadata 字段，前端可读
```

### A11.4 Rollback 幂等

```bash
# 当前是 SRC_FIRST，再 rollback 一次 → 无变化
curl -sS -X POST http://localhost:8030/migrator/tasks/$TID/switch \
  -H 'Content-Type: application/json' \
  -d '{"action":"rollback","stage":""}'
# 期望：{"code":0,"data":{"stage":"SRC_FIRST"}}（幂等）
```

---

## A12 死信重放（POST /replay-dl）

### A12.1 手插死信

```bash
mysql -uroot -p13520 webook_migrator <<SQL
INSERT INTO dead_letter (task_id, op, table_name, biz_id, payload) VALUES
    ($TID, 'insert', 'article', 100, '{"id":100,"title":"replay-1"}'),
    ($TID, 'insert', 'article', 101, '{"id":101,"title":"replay-2"}'),
    ($TID, 'insert', 'article', 102, 'this-is-not-json');
SQL
# 故意第 3 条 payload 是坏 JSON，验证失败计数
```

> ⚠️ `table_name` 填**源表名** `article`（= task.tables[].src），**不是**目标表 `article_v1`。
> replay 按它反查 tableIdx，目标表名由 sink 自己持有（`MySQLSink` 忽略 `Mutation.Table`）。
> 填成 `article_v1` 会被拒：`last_error: biz_table article_v1 not in task tables`，replayed=0。

### A12.2 重放

```bash
curl -sS -X POST http://localhost:8030/migrator/tasks/$TID/replay-dl \
  -H 'Content-Type: application/json' \
  -d '{"limit":1000}'
# 期望：{"code":0,"data":{"replayed":2,"failed":1}}
```

### A12.3 验证

```bash
# dst 表多了 2 行
mysql -uroot -p13520 webook_migrator -e "SELECT id, title FROM article_v1 WHERE id IN (100, 101)"
# 期望：100 | replay-1     101 | replay-2

# dead_letter 状态：2 条 replayed=1，1 条 retry_count=1 + last_error
mysql -uroot -p13520 webook_migrator -e "SELECT id, biz_id, replayed, retry_count, last_error FROM dead_letter WHERE task_id=$TID"
# 期望：
#   ... | 100 | 1 | 0 | (空)
#   ... | 101 | 1 | 0 | (空)
#   ... | 102 | 0 | 1 | "payload unmarshal: ..."
```

---

## B. 异构方向（任意源框架：sourceType × sinkType 自由组合）

> 方向 = 任务里的 `sourceType` × `sinkType`，**改字段不改代码**。下面三个子节是三种已验证方向。

---

## B1 Mongo 源 → MySQL（任意源框架：全量 + 增量 Change Stream）

> **任意源框架 v1**：`sourceType=mongo` 时，全量走 `find` 单 shard 流式扫描，增量走 Change Stream；
> 表上 `transform=mongo_to_relational` 把文档拍平成关系行——顶层标量同名进列，嵌套子文档/数组整团转 JSON 列。

### B1.0 前置：Mongo 副本集（Change Stream 必需，单机 mongod 不支持）

```bash
# deploy 已带 webook-mongo（单节点副本集 rs0）；首次起容器后初始化一次：
docker exec webook-mongo mongosh --quiet --eval "rs.initiate()"

# 确认 migrator/config/local.yaml 的 migrator.mongo 指向该副本集（改了要重启 migrator 让 go run 重读）：
#   mongo:
#     uri: "mongodb://root:13520@127.0.0.1:27017,127.0.0.1:27018/?replicaSet=rs0"
#     database: "webook"
```

### B1.1 准备 Mongo 源数据（含嵌套子文档 + 数组）

```bash
mongosh "mongodb://root:13520@127.0.0.1:27017,127.0.0.1:27018/webook?replicaSet=rs0&authSource=admin" --quiet <<'JS'
db.user_demo.drop()
db.user_demo.insertMany([
  { _id: "u1", name: "alice", age: 30, profile: { city: "SG", tags: ["go", "db"] } },
  { _id: "u2", name: "bob",   age: 25, profile: { city: "NY" } }
])
JS
# 注：用 string _id 便于断言；若用默认 ObjectID，目标 _id 列会是 24 位 hex（MongoSource 自动 ObjectID→hex）
```

### B1.2 建 MySQL 目标表（列对齐拍平后字段）

```bash
mysql -uroot -p13520 webook_migrator <<'SQL'
CREATE TABLE IF NOT EXISTS user_demo (
    _id     VARCHAR(64) PRIMARY KEY,
    name    VARCHAR(255),
    age     BIGINT,
    profile TEXT          -- 嵌套子文档 → JSON 字符串
) ENGINE=InnoDB CHARSET=utf8mb4;
SQL
```

### B1.3 建迁移任务（sourceType=mongo + 表级 transform）

```bash
curl -sS -X POST http://localhost:8030/migrator/tasks \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "user_mongo_to_mysql",
    "mode": "cdc",
    "kind": "heterogeneous",
    "sourceType": "mongo",
    "sourceDsnRef": "vault:mongo-src",
    "sinkType": "mysql",
    "sinkDsnRef": "vault:dst",
    "tables": [{"src": "user_demo", "dst": "user_demo", "partitionKey": "_id", "transform": "mongo_to_relational"}]
  }'
# 期望：{"code":0,"data":{"taskId":<N>}}
export TID=<上面返回的 taskId>   # 后续步骤用 $TID（art_schema_v1 占了 1，这里通常是 2）
```

### B1.4 全量同步（find 流式 → 拍平 → 落库）

```bash
curl -sS -X POST http://localhost:8030/migrator/tasks/$TID/start \
  -H 'Content-Type: application/json' -d '{"phase":"full"}'
# 期望：{"code":0,"data":{"started":"full"}}（Mongo 无 PKRange，自动单 shard 全扫，无需传 shards）
sleep 2

mysql -uroot -p13520 webook_migrator -e "SELECT _id, name, age, profile FROM user_demo ORDER BY _id"
# 期望（嵌套 profile 成 JSON 列）：
#   u1 | alice | 30 | {"city":"SG","tags":["go","db"]}
#   u2 | bob   | 25 | {"city":"NY"}
```

### B1.5 增量同步（Change Stream：插 / 改 / 删）

```bash
# 启动 Change Stream 订阅（异步后台跑）
curl -sS -X POST http://localhost:8030/migrator/tasks/$TID/start \
  -H 'Content-Type: application/json' -d '{"phase":"incr"}'
sleep 1   # 等 watch 建立（Change Stream 只捕获订阅之后的变更）

# 在 Mongo 写变更
mongosh "mongodb://root:13520@127.0.0.1:27017,127.0.0.1:27018/webook?replicaSet=rs0&authSource=admin" --quiet <<'JS'
db.user_demo.insertOne({ _id: "u3", name: "carol", age: 28, profile: { city: "LA" } })
db.user_demo.updateOne({ _id: "u1" }, { $set: { name: "alice2" } })
db.user_demo.deleteOne({ _id: "u2" })
JS
sleep 2   # 等事件经 Change Stream → 引擎 → Sink

mysql -uroot -p13520 webook_migrator -e "SELECT _id, name FROM user_demo ORDER BY _id"
# 期望：
#   u1 | alice2   （update 全文档 upsert）
#   u3 | carol    （insert）
#   （u2 已删）

# checkpoint：Mongo resume token 复用 binlog_pos 列（cursor_kind 标签沿用，cursor_value 是 resume token hex）
mysql -uroot -p13520 webook_migrator -e "SELECT task_id, phase, cursor_kind, LEFT(cursor_value,16) AS token FROM checkpoint WHERE phase='incr'"

# 暂停
curl -sS -X POST http://localhost:8030/migrator/tasks/$TID/pause
```

### B1.6 自动化 e2e（替代手动，需同一副本集）

```bash
cd webook    # config/test.yaml 的 migrator.mongo.uri 指向副本集后：
go test ./migrator/integration/ -run TestMongo_E2E -v
# 期望：TestMongo_E2E_FullToMySQL PASS + TestMongo_E2E_IncrToMySQL PASS
# 无 Mongo / 非副本集 / 连不上 → 自动 SKIP（不 fail）
```

---

## B2 MySQL 源 → Mongo 目标（反向：任意源框架另一方向）

> 方向纯由 `sourceType` × `sinkType` 决定，**无需改代码**。MySQL→Mongo = `sourceType:"mysql"`（默认）+ `sinkType:"mongo"`，**不带 transform**（关系行天然是平文档；反向"文档→关系"才需 `mongo_to_relational`）。
> `MongoSink` 用 `ReplaceOne {_id: 行PK}` upsert（幂等）：行 PK → 文档 `_id`，列 → 文档字段。

### B2.1 建任务（源 mysql / 目标 mongo，复用 Step 0.4 的 article 表作源）

```bash
curl -sS -X POST http://localhost:8030/migrator/tasks \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "article_to_mongo",
    "mode": "cdc",
    "kind": "heterogeneous",
    "sourceType": "mysql",
    "sourceDsnRef": "vault:src",
    "sinkType": "mongo",
    "sinkDsnRef": "vault:mongo-dst",
    "tables": [{"src": "article", "dst": "article", "partitionKey": "id"}]
  }'
# 期望：{"code":0,"data":{"taskId":<N>}}
export TID2=<返回的 taskId>
```

### B2.2 全量同步 → 验证 Mongo collection

```bash
curl -sS -X POST http://localhost:8030/migrator/tasks/$TID2/start \
  -H 'Content-Type: application/json' -d '{"phase":"full"}'
sleep 2

mongosh "mongodb://root:13520@127.0.0.1:27017,127.0.0.1:27018/webook?replicaSet=rs0&authSource=admin" \
  --quiet --eval 'db.article.find().sort({_id:1}).toArray()'
# 期望 3 个文档（行 PK 进 _id，列进字段）：
#   { _id: "1", id: 1, title: "第一篇", content: "内容 a" }
#   { _id: "2", id: 2, title: "第二篇", content: "内容 b" }
#   { _id: "3", id: 3, title: "第三篇", content: "内容 c" }
```

### B2.3 增量同步（走 canal binlog，**不是** Change Stream）

```bash
# 前置同 A6：MySQL 开 binlog + 建 canal 用户（增量源是 MySQL，机制是 binlog；Change Stream 是 Mongo 源专属）
curl -sS -X POST http://localhost:8030/migrator/tasks/$TID2/start \
  -H 'Content-Type: application/json' -d '{"phase":"incr"}'
sleep 1

# 改 MySQL 源表 → canal 捕获 → MongoSink upsert/delete
mysql -uroot -p13520 webook_migrator -e "INSERT INTO article (id,title,content) VALUES (4,'第四篇','d'); UPDATE article SET title='第一篇 v2' WHERE id=1; DELETE FROM article WHERE id=2"
sleep 2

mongosh "mongodb://root:13520@127.0.0.1:27017,127.0.0.1:27018/webook?replicaSet=rs0&authSource=admin" \
  --quiet --eval 'db.article.find().sort({_id:1}).toArray()'
# 期望：_id="1" title 变 "第一篇 v2"；新增 _id="4"；_id="2" 已删

curl -sS -X POST http://localhost:8030/migrator/tasks/$TID2/pause
```

### B2.4 对账（mongo 作目标的异构 /verify 已支持）

> **机制**：`SourceFactory.BuildDst` 在 `task.SinkType=mongo` 时复用 `MongoSource` 读 dst collection；
> `VerifyEngine.diffAndLog` 比对前对两侧应用 `normalizeRows`（过表的 transform 归一 + 剥 sink 注入元数据）：
> - `_id` —— MongoSink 写入时 `doc["_id"] = m.PK`（PK 回显），行已按 PK 匹配不参与字段比对
> - `version` —— MongoSink 写入时 `doc["version"] = m.Version`（来自 IncrEngine `EventTs`，sink 端乐观锁元数据），src 端 cols 不带，比对会假阳性
>
> 无表级 transform 时 Identity + 剥 `_id`/`version` 即可（MySQL→Mongo 这条路径就是这样）。业务表若需要保留 `_id` / `version` 同名业务列，避开这两个命名即可。
> 单测 + e2e（`TestMySQL_E2E_VerifyMongoDst`）覆盖。

```bash
# 全量对账（干净迁移）—— 零假阳性
curl -sS -X POST http://localhost:8030/migrator/tasks/$TID2/verify \
  -H 'Content-Type: application/json' -d '{"mode":"full"}'
# 期望：{"code":0,"data":{"mismatchCount":0}}

# 改 Mongo dst 一行 → 再 verify 应检出恰好 1 处 diff
mongosh "mongodb://root:13520@127.0.0.1:27017,127.0.0.1:27018/webook?replicaSet=rs0&authSource=admin" \
  --quiet --eval 'db.article.updateOne({_id:"1"}, {$set:{title:"被改脏了"}})'
curl -sS -X POST http://localhost:8030/migrator/tasks/$TID2/verify \
  -H 'Content-Type: application/json' -d '{"mode":"full"}'
# 期望：{"code":0,"data":{"mismatchCount":1}}

# 查差异明细（与同构 A8.3 同形）
curl -sS "http://localhost:8030/migrator/tasks/$TID2/mismatch?offset=0&limit=10" | jq
# 期望：list 1 行 mismatchKind="diff"，diffDetail 含 "diff_fields":["title"]
```

> 反向 Mongo→MySQL（`B1`）`/verify` 同样支持：表级 `transform:"mongo_to_relational"` 被 normalizeRows 应用到两侧，
> 把 src 端嵌套文档拍平到与 dst 同形态后再比对（dst 已是关系行，transform 幂等不变）。

### B2.5 自动化 e2e

```bash
cd webook && go test ./migrator/integration/ -run TestMySQL_E2E_FullToMongo -v
# 期望：PASS（MySQLSource → MongoSink 全量；行 PK→_id，列→字段）
```

---

## B3 MySQL 源 → Elasticsearch 目标（异构 sink 演示）

> 范围：验证 `sinkType=es` 异构同步链路 — FullEngine 全量 article 表 → ES `article_v1` 索引；
> verify 双向对账（MySQL src vs ES dst）；repair 走 ES 端写。
> 前置：本地起一个 ES 7.x/8.x；migrator 配 `migrator.es.addrs` yaml。
>
> **为什么有 B3 异构演示**：主流程（A1-A12）演示同构 MySQL→MySQL（schema 演进 / 同库换表）。
> 异构链路（ES / ClickHouse / Mongo / Kafka）走 `SinkFactory.heteroBuilder` 分支，
> 需要专门的 sink client + dst 端独立查询逻辑。这里只演示 ES 一种，其它 sink 类比扩展。

### 链路总览：三条链路谁读谁写

异构同步三条链路各用不同 Source/Sink，**`ESSource` 只在对账时读 ES，不在数据同步主链路上**：

| 链路 | 读（Source） | 写（Sink） |
|------|-------------|-----------|
| 全量同步（`start phase=full`，§B3.2）| `MySQLSource.FullScan`（读 MySQL `article`）| `ESSink`（写 ES `article_v1`）|
| 增量同步（`start phase=incr`，cdc）| `CanalSource`（读 MySQL binlog）| `ESSink`（写 ES `article_v1`）|
| 对账（`verify`，§B3.3）| src 侧 `MySQLSource` ＋ dst 侧 `ESSource`（`search_after` 扫 ES）| —（`repair` 才回写 ES）|

> **`ESSource` 不支持 `IncrSubscribe` 不影响同步性能**：增量更新走 `CanalSource` 读 MySQL binlog
> → `ESSink` 增量写 ES，是真增量，**不靠反复全量重灌**。`ESSource` 的职责是对账时读 ES dst
> （扫描语义），从 ES 增量读对它无意义，所以"不支持"是合理设计而非短板。全量同步（§B3.2）是
> **一次性**初始化（`FullEngine` 分片并行 + ES bulk 写），跑完即转增量；对账大表可用 `mode=sample`
> 采样降本（见 §A8.4），不必每次全量对账。

### B3.0 前置：起 ES + 配 yaml

#### B3.0.1 docker 起 ES（单节点 dev 模式，无认证）

```bash
docker run -d --name webook-es-dev \
  -p 9200:9200 -p 9300:9300 \
  -e "discovery.type=single-node" \
  -e "xpack.security.enabled=false" \
  -e "ES_JAVA_OPTS=-Xms512m -Xmx512m" \
  docker.elastic.co/elasticsearch/elasticsearch:8.13.0

# 等 ES 起来（15-30s），探活
curl -sS http://localhost:9200/_cluster/health | jq
# 期望：{"cluster_name":"...","status":"green"或"yellow",...}
```

#### B3.0.2 建 ES 索引 + mapping

```bash
# 显式建索引让字段类型可控（不依赖自动推断）
curl -sS -X PUT 'http://localhost:9200/article_v1' \
  -H 'Content-Type: application/json' \
  -d '{
    "mappings": {
      "properties": {
        "id":      {"type": "long"},
        "title":   {"type": "text", "analyzer": "standard"},
        "content": {"type": "text", "analyzer": "standard"}
      }
    }
  }'
# 期望：{"acknowledged":true,"shards_acknowledged":true,"index":"article_v1"}
```

#### B3.0.3 yaml 加 ES 地址

编辑 `webook/migrator/config/local.yaml`，`migrator:` 段下加：

```yaml
migrator:
  full:
    batchSize: 1000
    channelBuf: 4096
  incr:
    batchSize: 100
    channelBuf: 4096
    partitionCount: 1
  verify:
    batchSize: 1000
    channelBuf: 4096
  # ↓ 新加 es 段
  es:
    addrs:
      - "http://localhost:9200"
```

重启 migrator 服务让新配置生效：
```bash
cd webook/migrator && APP_ENV=config/local.yaml go run .
```

### B3.1 创建异构 task

```bash
curl -sS -X POST http://localhost:8030/migrator/tasks \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "art_to_es_v1",
    "mode": "cdc",
    "kind": "heterogeneous",
    "sourceDsnRef": "vault:src",
    "sinkType": "es",
    "sinkDsnRef": "vault:es-dst",
    "tables": [{"src": "article", "dst": "article_v1", "partitionKey": "id"}]
  }'
# 期望：{"code":0,"data":{"taskId":<N>}}（控制库已有 task 时自动递增）
export TID3=<返回的 taskId>

# 看一下任务详情
curl -sS http://localhost:8030/migrator/tasks/$TID3 | jq
# 注意：sink_type=es，kind=heterogeneous
```

### B3.2 启动全量同步（MySQL article → ES article_v1）

```bash
curl -sS -X POST http://localhost:8030/migrator/tasks/$TID3/start \
  -H 'Content-Type: application/json' \
  -d '{"phase":"full"}'
# 期望：{"code":0,"data":{"started":"full"}}（立即返回，异步执行）

# 等 1-2s，验证 ES 已收到 3 行
curl -sS 'http://localhost:9200/article_v1/_search?pretty&size=10'
# 期望：hits.total.value=3，3 个 doc，_source 对应 MySQL article 内容
```

### B3.3 对账（MySQL src vs ES dst — 真异构对账）

> **已实现**：`SourceFactory.BuildDst` 在 `task.SinkType=es` 时返 `ESSource(article_v1)`，verify 真读 ES。
> ESSource 用 `search_after` 分页扫所有 doc，PK 字段名取 `tm.PartitionKey`（默认 `id`）。

```bash
# 全量同步完(B.2),两侧应一致
curl -sS -X POST http://localhost:8030/migrator/tasks/$TID3/verify \
  -H 'Content-Type: application/json' \
  -d '{"mode":"full"}'
# 期望：mismatchCount=0
```

#### 制造差异验证 verify 能发现

> **跨平台注意**：ES 是 raw HTTP API，curl `-d` 的 payload 必须是 UTF-8 字节流。
> Windows Git Bash 默认把命令行字符串按 GBK 编码发出去，ES 收到的中文字符会报
> `Invalid UTF-8 start byte 0xb1`。本节用纯 ASCII title（`ES_TAMPERED` / `ES_GHOST`）规避此问题；
> 真要发中文，把 payload 写文件 `body.json`（保存为 UTF-8）用 `--data-binary @body.json` 发送。

```bash
# 删 ES 一个 doc(造 missing)
curl -sS -X DELETE 'http://localhost:9200/article_v1/_doc/2?refresh=true'

# 改 ES 一个 doc 的 title(造 diff)
curl -sS -X POST 'http://localhost:9200/article_v1/_update/3?refresh=true' \
  -H 'Content-Type: application/json' \
  -d '{"doc":{"title":"ES_TAMPERED"}}'

# 加 ES 一个 src 没有的 doc(造 extra)
curl -sS -X PUT 'http://localhost:9200/article_v1/_doc/99?refresh=true' \
  -H 'Content-Type: application/json' \
  -d '{"id":99,"title":"ES_GHOST","content":"phantom"}'

# 再 verify
curl -sS -X POST http://localhost:8030/migrator/tasks/$TID3/verify \
  -H 'Content-Type: application/json' \
  -d '{"mode":"full"}'
# 期望：mismatchCount=3
```

#### 查差异明细

> 同 A8 §A8.3：`verify` 只返 `{mismatchCount}` 计数，3 条明细逐条落 `validate_log`，
> 走 `GET /mismatch` 分页拉（默认 50/页，返 `{list, total}`）。异构对账明细形态与同构一致
> （明细来自对账比对逻辑，与 sink 是 MySQL 还是 ES 无关）。

```bash
curl -sS "http://localhost:8030/migrator/tasks/$TID3/mismatch?offset=0&limit=10" | jq
# 期望：list 3 行
#   bizId="2"  mismatchKind="missing"
#   bizId="3"  mismatchKind="diff"  diffDetail 含 "diff_fields":["title"]
#   bizId="99" mismatchKind="extra"

# 或直接查表（列名是 mismatch_kind，不是 diff_kind）
mysql -uroot -p13520 webook_migrator -e \
  "SELECT id, table_name, biz_id, mismatch_kind, repaired FROM validate_log WHERE task_id=$TID3"
```

### B3.4 修复差异（用 src 真覆盖 ES dst — 完整闭环）

> verify 真读 ES 看到差异 → repair 拿 src snapshot 走 ESSink 真写回 ES → ES 状态回归。
> 链路两端都是 ES：读取 ESSource、写入 ESSink。

```bash
# 拿 validate_log id
IDS=$(mysql -uroot -p13520 webook_migrator -sNe \
  "SELECT id FROM validate_log WHERE task_id=$TID3 AND repaired=0" \
  | tr -d '\r' | tr '\n' ',' | sed 's/,$//')

curl -sS -X POST http://localhost:8030/migrator/tasks/$TID3/repair \
  -H 'Content-Type: application/json' \
  -d "{\"strategy\":\"src_overwrite_dst\",\"ids\":[$IDS]}"
# 期望：{"code":0,"data":{"repaired":3}}

# 验证 ES 已回归(refresh=wait_for 等到 commit 可见)
curl -sS 'http://localhost:9200/article_v1/_refresh' -X POST
curl -sS 'http://localhost:9200/article_v1/_search?pretty&size=10&sort=id:asc' \
  | grep -E '"_id"|"title"'
# 期望：3 个 doc(_id=1,2,3),无 _id=99,title 都是 "第X篇"

# 再 verify 一遍确认无差异
curl -sS -X POST http://localhost:8030/migrator/tasks/$TID3/verify \
  -H 'Content-Type: application/json' \
  -d '{"mode":"full"}'
# 期望：mismatchCount=0
```

### B3.5 切流（与同构一致）

```bash
# 推 SRC_FIRST
curl -sS -X POST http://localhost:8030/migrator/tasks/$TID3/switch \
  -H 'Content-Type: application/json' -d '{"stage":"SRC_FIRST"}'
# 期望：{"code":0,"data":{"stage":"SRC_FIRST"}}

# Redis 验证 stage 已持久化
redis-cli -a 13520 GET 'migrator:stage:art_to_es_v1'
# 期望：SRC_FIRST
```

### B3.6 清理 ES

```bash
# 删索引
curl -sS -X DELETE 'http://localhost:9200/article_v1'
# 期望：{"acknowledged":true}

# 停 ES 容器
docker stop webook-es-dev && docker rm webook-es-dev
```

### B3.7 与同构 demo 的关键差异

| 维度 | 同构（A1-A12） | 异构（B 段） |
|------|------------------|---------------|
| **task.sinkType** | `mysql` | `es` |
| **task.kind** | `schema`（同库 schema 演进） | `heterogeneous` |
| **SinkFactory 走哪个分支** | `MySQLSink` 直接 | `heteroBuilder` → `buildESSink` |
| **dst 端检索方式** | SQL `SELECT * FROM article_v1` | HTTP `GET /article_v1/_search` |
| **dst 端 PK** | MySQL 主键 `id` | ES `_id`（migrator 用 `tm.PartitionKey` 字段值做 _id） |
| **verify dst 读** | `MySQLSource(article_v1)`(SQL `SELECT`) | `ESSource(article_v1)`(`search_after` 分页 + aggs PKRange) |
| **`SourceFactory.BuildDst` 分发** | `task.SinkType="mysql"` → `MySQLSource` | `task.SinkType="es"` → `ESSource`(详见 §B3.3) / `task.SinkType="mongo"` → `MongoSource`(详见 §B2.4 异构对账) |

### B3.8 设计边界

> 异构 ES 链路已**双向闭环**：写端 `ESSink` + 读端 `ESSource` 都真用 ES（§B3.2 写 + §B3.3 对账 + §B3.4 修复全部走真 ES）。Mongo 同样双向闭环（B1 / B2 / B2.4 异构对账）。
> 扩展点 / 设计边界：
>
> - **CK / Kafka verify-dst Source**：v1 异构 verify 支持 mysql / es / mongo 三种 dst；ClickHouse / Kafka 在 `SourceFactory.BuildDst` 上是扩展点（按 ESSource / MongoSource 模式 ~200 行各）。
> - **ESSource 不支持 IncrSubscribe**（设计如此）：ES 无 binlog 概念。CDC 增量同步走 "源端 MySQL Canal → ESSink" 路径；反向（ES 主、其他 sink 副）不在 v1 范围。
> - **多 ES/Kafka/CK 集群（按 task 解 DSN）**：v1 用全局 yaml `migrator.{es,kafka,clickhouse}.*`，单集群通用够用；多 task 多集群场景通过 `dsn.Resolver` 扩展（接 Vault/K8s Secret）。
## C. 参考

---

## C1 功能边界与前提

下列 endpoint **均已实现**，但生效有前提，注意：

| Endpoint | 前提 / 行为 |
|---|---|
| `POST /tasks/:id/start {phase:"incr"}` | cdc 模式 + 注入 CanalClient（`local.yaml` 配 `migrator.canal.*` + docker-compose 起 canal-ready MySQL）→ 走真 `CanalSource` 订 binlog（见 A6）。未配 canal 时 fallback `MySQLSource`，`IncrSubscribe` 返 `ErrIncrNotSupported` |
| `GET /tasks/:id/lag` | 任务在跑且用 `CanalSource` → 返 `{srcLagMs, dstLagMs, lagMs}`。纯 `MySQLSource`（非 cdc）不实现 LagReporter → `503 lag unavailable` |
| `POST /tasks/:id/throttle` | TaskService 经 ThrottleRepository 写 Redis 持久化，**下次 Start** 时回读覆盖 ShardSpec QPS（"重启生效"，非运行时实时调速） |
| `sourceType:"mongo"` 任务 | 全量走 MongoSource（`find` 单 shard 流式）；增量走 Change Stream（**需副本集**，单机 mongod 会让 Watch 失败）。表上需 `transform:"mongo_to_relational"` 才把文档拍平成关系列；`migrator.mongo.{uri,database}` 未配则建源失败。见 B1 |
| `sinkType:"mongo"` 任务 | 迁移（全量 + 增量 canal binlog）OK，行 PK→文档 `_id`、列→字段、无需 transform；**异构 `/verify` 已支持**（`BuildDst` mongo 分支复用 MongoSource 读 dst collection + `verify.normalizeRows` 两侧过 transform 归一 + 去 `_id` PK 回显）。见 B2.4 |

---

## C2 错误码速查

| HTTP | 触发场景 |
|------|---------|
| 400 | 参数不合法 / id 不是数字 / 采样率越界 / 灰度越界 |
| 404 | 任务不存在 |
| 409 | 任务状态冲突（switch 跳级）/ 双人同 actor / pause 时无活动任务 |
| 412 | switch 进 DST_ONLY 时缺 approve（先 propose） |
| 501 | engine not configured（wire 未注入对应引擎 / throttle cache 实现） |
| 503 | Lag 不可用（Source 不支持） |

---

## C3 清理

测完想完全重置：

```bash
# 关闭 migrator 服务（Ctrl+C）

# 清表（保留库 schema）
mysql -uroot -p13520 webook_migrator -e "TRUNCATE task; TRUNCATE checkpoint; TRUNCATE validate_log; TRUNCATE audit_log; TRUNCATE dead_letter; TRUNCATE article; TRUNCATE article_v1; DROP TABLE IF EXISTS user_demo"

# 清 Mongo demo collection（B1 / B2 用）
mongosh "mongodb://root:13520@127.0.0.1:27017,127.0.0.1:27018/webook?replicaSet=rs0&authSource=admin" --quiet --eval "db.user_demo.drop(); db.article.drop()"

# 清 Redis
redis-cli -a 13520 KEYS 'migrator:*' | xargs redis-cli -a 13520 DEL
redis-cli -a 13520 KEYS 'webook:idem:*' | xargs redis-cli -a 13520 DEL

# 彻底删库（如不再用）
mysql -uroot -p13520 -e "DROP DATABASE webook_migrator; DROP DATABASE webook_migrator_test"
```

---

## C4 故障排查

| 现象 | 排查 |
|------|------|
| 启动报 `failed to connect mysql` | 检查 `.deploy/.env.local` 密码与 `config/local.yaml` 是否一致 |
| 启动报 `failed to connect redis` | 同上，密码 |
| 启动报 `init table` | 控制库表已存在但结构不匹配（task / checkpoint 等） — TRUNCATE 或 DROP 重建 |
| curl 返 401 | JWT middleware 启用中。本地 `config/local.yaml` 已默认 `server.http.jwt.disabled: true` 跳过 JWT；如果你改回 `false` 或部署到 staging/prod，需要 webook-core 起着并签发 token，请求带 `Authorization: Bearer <token>` |
| `traces export: context deadline exceeded: dial tcp 127.0.0.1:4317` | 本地没起 OTel collector。`config/local.yaml` 已默认 `otel.disabled: true` 用 noop tracer 跳过；如果你改回 `false` 又没起 collector，会看到这条 log 刷屏（不影响业务） |
| curl 返 ConnRefused | migrator 没起来或端口被占（默认 :8030，看启动日志） |
| audit_log 看不到行 | 异步落表，等 100ms 再查；或检查服务日志有无 `audit insert failed` |
| 全量同步 dst 表为空 | 检查源表是否有数据 + ShardSpec 的 PKMin/PKMax 是否覆盖源表 PK 范围 |
| replay-dl `failed > 0` | 看 `dead_letter.last_error` 字段；常见原因：`table_name` 填成目标表名（应填源表名 src）/ payload 损坏 / dst 表不存在 / 唯一键冲突 |

---

## C5 自动化测试入口

如果只想跑测试不想手工 curl：

```bash
# 全套（含 sqlmock + miniredis 单测）
cd webook && go test ./migrator/...

# 真库集成测试
cd webook && go test -v ./migrator/integration/...

# 单包详细
cd webook && go test ./migrator/web/ -v
```

---

## 附录 · 业务侧 SDK 接入自测（webook-core 集成）

> 范围：验证主仓 `webook-core` 通过 `internal/migratorsdk/` 接入 migrator 的双写 / 切读路径，不依赖本服务（migrator）跑着。
> 适用：接入完 SDK 想验收 / cutover 前演练 / 怀疑接错时复现。
> 背景：SDK 设计和接入手册见 [`prd/migrator/03-walkthrough.md §16`](../../prd/migrator/03-walkthrough.md)。
>
> **命令风格说明**：本附录以下命令为 **Windows / PowerShell 风格**（业务侧主仓常在 Windows 上 go run 调试，`webook/CLAUDE.md` 已注明）；其他章节（0.1-0.7、A1-A12）为 **Bash / Git Bash 风格**。Linux/Mac 用户把 `cd C:\Go\work\...` 改成 `cd ~/Go/work/...`、反引号续行 改 `\`、`$env:X='y'` 改 `export X=y` 即可。

### A.1 起本地中间件 + 应用 SQL

```powershell
# MySQL + Redis（仓库根 deploy/docker-compose.yaml 本地 profile）
cd C:\Go\work\deploy; .\deploy.sh local

# 应用建表脚本（确保 published_article + published_article_v1 都建好）
mysql -h127.0.0.1 -uroot -p13520 webook < C:\Go\work\webook\scripts\webook.sql
```

### A.2 打开 SDK 开关

编辑 `webook/internal/config/local.yaml`：

```yaml
migrator:
  sdk:
    enabled: true                       # 默认 false → NoOp 零开销；改 true 启用 Redis 实现
    taskName: "published_article_v1"
```

### A.3 启动主服务（webook-core）

```powershell
cd C:\Go\work\webook; $env:APP_ENV='config/local.yaml'; go run main.go
```

### A.4 用 redis-cli 模拟 4 个 stage（替代 migrator 服务下发）

| 阶段 | 命令 | 期望行为 |
|------|------|---------|
| 默认 SRC_ONLY | `DEL migrator:stage:published_article_v1` | 只写 OLD、只读 OLD（等价旧行为） |
| **SRC_FIRST 灰度 100%** | `SET migrator:stage:published_article_v1 SRC_FIRST` + `SET migrator:gray:published_article_v1 100` | 双写 OLD+NEW；读全切 NEW；NEW 写失败仅 log warn 不阻塞业务 |
| **SRC_FIRST 灰度 50%** | `SET ... SRC_FIRST` + `SET ... gray 50` | 双写；读按 `FNV(id)%100 < 50` 分流，同 id 始终落同侧（read-your-write） |
| **DST_FIRST 严格双写** | `SET ... DST_FIRST` | OLD+NEW 都必成；任一失败业务直接报错；读全 NEW |
| **DST_ONLY 切干净** | `SET ... DST_ONLY` | 只写 NEW、只读 NEW；OLD 不再变动 |

```powershell
redis-cli -a 13520 -h 127.0.0.1 -p 6379 SET migrator:stage:published_article_v1 SRC_FIRST
redis-cli -a 13520 -h 127.0.0.1 -p 6379 SET migrator:gray:published_article_v1 100
```

### A.5 调 webook-core API 触发 SDK 路径

> v1 实际路由（来源 `webook/internal/web/article.go::RegisterRoutes`）。所有端点是 **POST**，认证靠 `Authorization: Bearer <token>` header（除 `/article/reader/*`，读者侧不要 token）。

| 用户行为 | 端点 | 触发的 SDK 方法 | 内部链路 |
|---|---|---|---|
| 发表 | `POST /article/publish` | `DualWriter.Write` | `service.Publish → readerRepo.Upsert → dualWriter.Write(side → daoBySide(side).Upsert)` |
| 撤回 | `POST /article/withdraw` | `DualWriter.Write` | `service.Withdraw → readerRepo.Delete → dualWriter.Write` |
| 删除 | `POST /article/delete` | `DualWriter.Write` | `service.Delete → readerRepo.Delete → dualWriter.Write` |
| 读详情（按 id 路由）| `POST /article/reader/detail` | `SwitchReader.ChooseSide` | `readerSvc.Detail → readerRepo.FindById → switchReader.ChooseSide(taskName, id)` |
| 读分页 | `POST /article/reader/page` | **不走 SDK** | Page 跨侧语义不一致，切流期始终走 oldDAO（见 `repository/article_reader.go` 注释）|

```powershell
# 先设一次 token，整段复用（值 = 登录后响应头 x-access-token；跑 scripts/postman.json 的 A0.2 密码登录可自动拿）
$token = "<webook-core JWT>"

# 发表（Upsert → DualWriter.Write 双写 / 单写按 stage 决策）
curl.exe -X POST http://localhost:8010/article/publish `
  -H "Content-Type: application/json" -H "Authorization: Bearer $token" `
  -d '{\"id\":0,\"title\":\"sdk-e2e\",\"content\":\"hello\",\"abstract\":\"abs\"}'
# 返：{"code":0,"data":<新生成的 article id>}

# 详情（FindById → SwitchReader.ChooseSide 按 stage+gray 决定走 OLD/NEW）
curl.exe -X POST http://localhost:8010/article/reader/detail `
  -H "Content-Type: application/json" `
  -d '{\"id\":<上面返的 id>}'

# 撤回（Delete → DualWriter.Write，作者侧 token 必带）
curl.exe -X POST http://localhost:8010/article/withdraw `
  -H "Content-Type: application/json" -H "Authorization: Bearer $token" `
  -d '{\"id\":<id>}'

# 删除（同 Delete 链路，区别于 withdraw 是物理删 vs 状态置 unpublished）
curl.exe -X POST http://localhost:8010/article/delete `
  -H "Content-Type: application/json" -H "Authorization: Bearer $token" `
  -d '{\"id\":<id>}'
```

> Bash / Linux 用户把 `curl.exe` 改 `curl`、反引号 `` ` `` 改 `\` 续行，`$token` 赋值改成 `token="..."`（无 `$`、等号两侧不留空格）。`$token` 是 webook-core JWT，登录态拿——跑 `scripts/postman.json` 的 A0.2 密码登录后从 x-access-token 响应头复制即可。
>
> 如何看到 SDK 真触发：把 `migrator.sdk.enabled` 切 `true` 后，A.4 已 `SET migrator:stage:published_article_v1 SRC_FIRST` → 发表后 A.6 SQL 验证 `published_article_v1` 表（NEW 侧）真出现新行。stage=SRC_ONLY 时 NEW 侧不动（NoOp 等价旧行为）。

### A.6 双侧表对照校验

```sql
-- OLD 侧
SELECT id, title, status, updated_at FROM published_article WHERE id = <id>;
-- NEW 侧（SRC_FIRST/DST_FIRST/DST_ONLY 才有数据；SRC_ONLY 阶段为空）
SELECT id, title, status, updated_at FROM published_article_v1 WHERE id = <id>;
```

期望对照：

| stage | OLD 是否动 | NEW 是否动 |
|-------|----------|----------|
| SRC_ONLY | ✅ | ❌ |
| SRC_FIRST | ✅ | ✅（NEW 失败业务不报错） |
| DST_FIRST | ✅ | ✅（NEW 失败业务报错） |
| DST_ONLY | ❌ | ✅ |

### A.7 故障降级演练

```powershell
# Redis 挂掉，主服务必须仍可用（SwitchReader → SideOld；DualWriter → SRC_ONLY 等价旧行为）
docker stop webook-redis
# 重新调发表 / 详情 API，应正常 200 且只动 OLD 表
docker start webook-redis
```

### A.8 收尾还原 NoOp

```powershell
# 关 SDK 回 NoOp，路径零 Redis 调用
# local.yaml: migrator.sdk.enabled = false

# 清掉残留控制位（避免下次开 SDK 时误切）
redis-cli -a 13520 -h 127.0.0.1 -p 6379 DEL migrator:stage:published_article_v1 migrator:gray:published_article_v1
```

### 自测验收点

跑完 A.1-A.8 后，下列结论必须能站住，否则集成有问题：

- [ ] SRC_ONLY（默认）下双侧表行为等价旧版本，NEW 表 0 写入
- [ ] SRC_FIRST gray=100 下双写成立；redis-cli `KEYS migrator:*` 显示控制位生效
- [ ] SRC_FIRST gray=50 下同一 id 多次调用 FindById 始终落同侧（read-your-write）
- [ ] DST_FIRST 下手工 `DROP` 掉 `published_article_v1` 模拟 NEW 故障 → 业务报错；恢复后业务正常
- [ ] DST_ONLY 下 OLD 表完全不变，新文章只在 NEW 表
- [ ] Redis 挂掉 → 业务 API 仍 200，行为等价 SRC_ONLY
- [ ] yaml `enabled: false` 切回后 `KEYS migrator:*` 已清，应用日志无 SDK 相关 warn

---

