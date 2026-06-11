# CHANGELOG

<!-- 新功能前插在此，日期降序 -->

## [2026-06-11] webook-migrator 分层合规重构 + 命名全链统一 + 业务监控 + 文档对齐

**变更内容**：在干净 v1 基础上做架构质量收口——①**跨层引用清除**：service/web 层不再直引 `repository/dao`·`repository/cache`·`redis.Cmdable`，新增 7 个 repository（checkpoint/validate_log/dead_letter/audit_log/throttle/switch_state + 原 task）+ `service/replay`（ReplayDL 业务从 handler 下沉）+ cache 层 `SwitchStateCache`；②**命名全链统一**：DAO 实体去 `Migration` 前缀（`MigrationTask`→`Task`…，含表名 `migration_*`→`*` 与索引名）、`Id` 风格贯通（`TaskID`/`taskID`→`TaskId`/`taskId`，字段/变量/方法一体）、转换 helper 升维泛型 `pkg/slicex.Map`（删手写复数方法）、删死哨兵 `ErrStateConflict` + validate_log 索引自愈兼容逻辑 + GTID 占位澄清；③**业务监控补齐**：`webook_migration_lag_ms{task_id,side}` + `webook_migration_dead_letter_unreplayed{task_id}`（scrape 实采 Collector）+ grafana 面板恢复真实查询（删此前永远 No-data 的幽灵面板）；④**transform 文件结构对齐** source/sink（接口/registry/identity/mongo 分文件）；⑤三服务 `ioc/config.go` 同构统一。

**影响范围**：`webook/migrator/**`（16 包）+ 新增 `webook/pkg/slicex`；`webook/internal/ioc/config.go`（core 抽出）+ `chat/ioc/config.go` + `internal/migratorsdk/sdk.go` 注释；部署 `deploy/grafana/.../webook-migrator.json`；prd/migrator 全量同步（02-architecture·03-walkthrough·README·postman 等）；`webook/CLAUDE.md` 新增 4 条规则（层间模型转换 / 标识符 Id 风格 / 包内多实现文件组织 / 多服务布局）。全模块 `go build/vet/test ./...` 全绿、goimports 干净；`Migration*`/`migration_*`/`TaskID`/`toDomainList` 全仓 grep 零残留。

**技术决策**：分层修复严守「service 只依赖 repository 接口、横切中间件（audit）可直达仓储」；`Id` 风格选「向仓内既有约定收敛」而非 Go-initialisms（存量几百处 vs 2 离群点）；`slicex.Map` 自写 6 行泛型而非引 samber/lo（标准库无 Map、避免新依赖）；表名前缀剥除因控制库是专用库 `webook_migrator`（前缀无命名空间价值）+ v1 未发版零迁移成本；监控用 scrape-time Collector（无后台 goroutine，2s 超时兜底 DB 聚合查询）。

**待办**：CountUnreplayedByTask 补 sqlmock 单测；full 同步进度指标（v2）；lag/dead_letter 告警规则（alerting yml，可选）；部署侧 dev/staging 旧 `webook_migrator` 库需 DROP 重建（表已改名，AutoMigrate 不迁移）。

**会话**：260606-migrator-删反向同步（延续）

## [2026-06-08] webook-migrator 清扫为干净 v1（删 CLAUDE.md/02b + 清除 idempotency/RBAC/反向同步残留 + 文档同步）

**变更内容**：把 migrator 代码注释与设计文档统一收敛为「干净 v1」——①删 `webook/migrator/CLAUDE.md`（v1/v2/扩展点前瞻框架最集中、且与 prd 文档重复）+ `prd/migrator/02b-task-module-design.md`（交付前 vertical-slice 蓝图，已被实际代码取代）；②清除文档里**已从代码删除的功能**残留：idempotency 中间件 / `Idempotency-Key` header / `migrator:idempotency` 键 / IdempotencyCache、RBAC 中间件假声明（`rbac.go` / `InitRBACBuilder` / 401·403·429 e2e / 挂 14 endpoint）——区分保留 `幂等`（Sink upsert 性质）；③代码注释去兼容性/前瞻措辞（反向同步残留 4 处、向后兼容、GTID/throttle v2 前瞻、Step 路标、失效声明 sink 乐观锁/canal 后续集成）；④文档前瞻指针（"以 CLAUDE.md 设计范围/v2 范围为单一真相源"）全部清掉，v1 能力以 `02-architecture.md`「📌 v1 实现摘要」为权威。

**影响范围**：删 2 文件；代码 12 文件改注释（build + vet + `go test ./migrator/...` 全绿）；prd 文档 `02-architecture.md`·`03-walkthrough.md`·`01-product.md`·`04-cutover-playbook.md`·`04b-cutover-checklist.md`·`adr/0002`·`assets/prometheus-alerting.yml` + `webook/migrator/README.md`·`scripts/postman.json` 同步；全仓 grep 验证 0 残留（idempotency 中间件 / Idempotency-Key / migrator:idempotency / IdempotencyMiddleware / RBAC middleware / requireRBAC / InitRBACBuilder / 反向同步 / webook/migrator/CLAUDE.md）。

**技术决策**：CLAUDE.md 与 prd/migrator 文档职责重叠 + 是 v1/v2 框架残留集中地，删除后 `02-architecture.md`（本就是「权威定义」）单一承载 v1 能力摘要；idempotency/RBAC 中间件早在 v1 收尾已从代码删除（见 `[2026-05-27]` 条），但文档当时未同步干净，本次彻底对齐；`幂等`（Sink ON DUPLICATE KEY + Version 乐观锁）是真实 v1 行为，与「idempotency 中间件」区分保留；反向 repair `dst_overwrite_src`（≠反向同步引擎）保留。

**待办**：无（v1 文档/代码已自洽；`webook/migrator/CLAUDE.md` 已删，先前条目对其「v2 设计范围」的引用随文件移除而失效，v1 文档不再前瞻 v2）。

**会话**：260606-migrator-删反向同步（续接）

**发布**：（未发布）

## [2026-06-06] webook-migrator prd 文档全面对齐 v1（反向同步归 v2 + 业务 metric 归基础设施）

**变更内容**：prd/migrator 设计文档与 v1 代码对齐——①移除反向同步（cutover Plan B），回滚模型改为「DST_ONLY 不可逆，回滚仅双写期 SRC_FIRST/DST_FIRST，切前充分对账兜底」；②42 处 `webook_migrator_*` 业务 metric 改为 v1 实际可观测（基础设施 metric `webook_http_/db_/go_` + 控制台 API `/lag`·`/tasks/:id`·`/mismatch` + mysql 查控制库 + 服务日志），prometheus-alerting 重写为 v1 4 条核心告警 + 业务 SLO；③health `{status:ok,service}`、throttle「next_start 生效」、Redis stage/gray key 用 taskName —— 全部对齐代码。
**影响范围**：`prd/migrator/`（01~04b + 10 runbook + adr/retros/assets）+ `webook/migrator/CLAUDE.md`（v2 设计范围补反向同步）；代码侧反向同步增删抵消（`pipeline/source/factory.go` 等回原样，build/vet/44 单元包全绿）。
**技术决策**：反向同步（NEW→OLD 自动回写）涉及引擎方向化 + 防死循环 + 状态编排，复杂度高，v1 不做、归 v2，靠「切单写不可逆 + cutover 前强制对账（mismatch<0.001%）」兜底；业务 metric 埋点归 v2（落地须合规命名 `webook_<subsystem>_*`），v1 用基础设施 metric + API 可观测。
**待办**：v2 若做反向同步 / 业务 metric，按 `webook/migrator/CLAUDE.md`「v2 设计范围」推进。
**会话**：260606-migrator-删反向同步
**发布**：（未发布）

## [2026-05-28] webook-migrator v1 收尾（异构 verify Mongo dst + 共享 mongo client + README A/B/C + 全量文档同步）

**变更内容**：在 v1 任意源框架（2026-05-27）基础上补齐两件事：①**异构 verify 真支持 Mongo 作目标**（`SourceFactory.BuildDst` mongo 分支复用 `MongoSource` 读 dst collection + `VerifyEngine.diffAndLog` 比对前 `normalizeRows` 对两侧应用表的 transform 归一 + 去 `_id` PK 回显）；②**README 重排为 A/B/C 三组**（A 核心生命周期 / B 异构方向 / C 参考），修 Step 4 空号 + 6b 编号、ES Appendix B 上移成 B3、顶部加目录、内部交叉引用全部对齐。

**影响范围**：
- 代码（前一会话已实现，本日补 e2e + 文档）：`pipeline/source/factory.go`（BuildDst mongo 分支）、`service/verify/verify.go`（`normalizeRows` + transform Registry 注入）、`ioc/engines.go::InitVerifyEngine`、`service/verify/verify_test.go`（异构-transform 用例）。
- 新增 e2e：`integration/verify_mongo_e2e_test.go::TestMySQL_E2E_VerifyMongoDst` —— 真 infra 跑 MySQL→Mongo 全量后构造 VerifyEngine（mysql+mongo 双 builder + transform Registry）跑 Full，断言①干净迁移零假阳性 ②改 dst 一行检出恰好 1 diff。**本机 Mongo 副本集 + MySQL 起着时实跑 PASS**。
- 文档：`webook/migrator/README.md` 整篇重排（1290→1319 行，命令/期望输出零流失；fence count 140=140 校验）；`webook/migrator/CLAUDE.md`「已知功能边界」加「异构 verify Mongo dst ✅」行；`verify.go` 包注释边界更新。
- 顺手修 bug：README L1155 `mongosh` 命令漏 `&authSource=admin` → 用户运行报 `Authentication failed`。补齐后与其他 5 处 mongosh 命令一致。

**技术决策**：
- 异构 verify normalize 选「两侧都过 transform + 去 `_id`」而非「只对源端 transform」—— transform 对已正确迁移的 dst 是幂等的（拍平后再拍平不变），保证同构（Identity + 无 `_id`）路径行为零漂移，单测验证 2 路径（异构 + 同构）。
- README 重排走 Python line-range 切片 + 标题行重写脚本（命令体 byte-for-byte 切片，不是 LLM 重写命令），避免命令文本回写漂移；脚本输出与原文做 `Counter` 比对验证零内容流失。
- 编号方案选 A/B/C 三组但不动 markdown heading 层级（组首和子节都是 `##`），避免大规模层级 demotion 风险；目录用手写嵌套列表，导航靠 Ctrl-F + 目录文字定位（不依赖 anchor 链接，避免中文/标点 anchor 生成歧义）。

**配套收尾**：①`ioc.InitMongoClient` 进程级共享 `*mongo.Client` 注入 wire（sink/source 各 builder 复用同一连接池，driver 内置 pool 管理），关闭原 `buildMongo*` 多 client 模式；②全部迁移相关文档（`migrator/CLAUDE.md` / `migrator/README.md` / `prd/migrator/01-product.md` / `02-architecture.md` / `02b-task-module-design.md` / `03-walkthrough.md` / `04b-cutover-checklist.md`）同步对齐 v1，删除"首版差异 / 待办 / TODO"语气，统一指向 `migrator/CLAUDE.md`「设计范围与扩展点」作为单一真相源。

**会话**：260527-migrator-任意源框架（续接 2，从 2026-05-27 晚 prompt-too-long 中断 → 2026-05-28 收尾）
**发布**：待发布

---

## [2026-05-27] webook-migrator v1（第一版）

**总述**：webook-migrator 第一版 —— 异构数据迁移服务（任意源 MySQL/Mongo ↔ 任意目标 MySQL/ES/…），全量 + 增量（CDC binlog / Mongo Change Stream）+ 对账 + 4 阶段切流。本条目合并三段建设：①完整 review（17 bug + 异构对账闭环 + canal cdc 落地 + 架构简化 + 文档同步，§一~五）；②incr binlogpos→checkpoint 持久化修复（§六）；③任意源框架（MySQL 单源 → 任意源可插拔，Mongo 全量+增量，§七）。**Mongo 全量 + 增量 e2e 已对真副本集 Mongo（2 节点 rs0）验证 PASS**。

**背景**：以 README Step 8 对账数据对不上为入口，一路 review 出来一连串关联 bug + 设计缺陷。最终覆盖：P0 阻塞修复、P1 实现缺失补齐、异构对账闭环、canal cdc 运行环境落地、架构 YAGNI 简化、prd 文档同步。完整 17 个 bug 表见 `webook/migrator/CLAUDE.md`「已知功能边界」段。

### 一、P0 + 关键 bug 修复

| Bug | 修法 |
|------|------|
| SourceFactory cdc 任务全量阶段误用 CanalSource | 接口拆 `BuildFullSrc / BuildIncrSrc / BuildDst` 三方法（按读取语义,不再按 task.Mode）;ADR-0002 记录决策 |
| validate_log 唯一索引 `uk_dedup` 含 repaired 列 → mark 时 1062 Duplicate entry | 索引列减为 `(task_id, table_name, biz_id)`;`BatchInsert` upsert 显式重置 `repaired=0`（绕开 GORM `default:0` 零值过滤） |
| `task.status` 死字段 8 状态全停 0 | `TaskService/Repository.UpdateStatus`;FullEngine/IncrEngine.Run 入口/出口 defer 推进;SwitchService DST_ONLY 同步 Switched |
| DSN 字段被忽略（所有迁移在控制库内自闭环） | 新建 `pipeline/dsn/Resolver` 接口 + `StaticResolver` 占位;SourceFactory/SinkFactory 加 `WithDBResolver`;生产前补 Vault PerTaskResolver |
| audit_log.error_msg 永远空 | middleware 解响应 body `{code,msg}` 截 512 字节填入 |
| Preflight 占位返 ready=true | 通过 `dsn.Resolver` 拿源端 db,真查 `@@global.binlog_format / gtid_mode` + `information_schema.STATISTICS` 表 PK |
| `/lag` 只看 src 侧 | 加 `IncrEngine.LagDst` + `dstLastEventTs sync.Map` CAS 更新;handler 返 `{lagMs, srcLagMs, dstLagMs}` |
| GORM AutoMigrate 不改同名索引 | `dao/init_table.go` 加 `ensureValidateLogDedupIndex`:启动时查 information_schema 比对,不符合就去重 + DROP + ADD 自愈 |
| GORM 零值字段过滤让 upsert repaired=0 失效 | DoUpdates 用 `clause.Assignments(map{"repaired": 0})` 字面值绕开 |
| `/start` 重复请求双开 race | `paused.LoadOrStore(taskID, nil)` 原子占位 + handler `IsRunning` 早判 409 双层防护 |
| `/verify sampleRate=0` 不被拦 | SampleRate 改 `*float64` 区分"未传"vs"显式 0";handler 校验 (0,1] |
| `/gray percent=200` 业务消息被吞 | 去掉 binding `min/max` tag,让 service 层 `ErrInvalidGrayPercent` 透传到 HTTP |
| switch `Stage required` 让 rollback `stage:""` 撞 binding 400 | switchReq.Stage 去 required,handler 内仅 rollback 模式跳过 stage.Valid() |
| 状态机错误 sentinel.Message 吞动态 from/to | `WithMetadata` 携带 `from/to/allowed`,前端 `Result.Metadata` 取 |
| README Step 8.1 INSERT id=99 重复跑撞 PK | 改 `REPLACE INTO`,Step 8.1 全部幂等 |
| Step 9 `mark_only` 后再 verify 同差异从列表消失 | DoUpdates 覆盖 repaired=0 让差异重新进列表（符合"当下承认,再发现仍提醒"语义） |
| Windows Git Bash `paste -sd,` \r\n 导致 ids JSON 无效 | README 改用 `tr -d '\r' \| tr '\n' ',' \| sed 's/,$//'` |

### 二、异构对账闭环（MySQL ↔ ES 双向真异构）

- 新建 `pipeline/source/es.go`（~190 行）：ESSource 用 search_after 分页（ES 8 scroll 已 deprecated）+ aggs PKRange + IncrSubscribe 返 err（ES 无 binlog 概念）
- `SourceFactory.BuildDst` 按 `task.SinkType` 分发 MySQL/ES;`WithESSourceBuilder` ioc option
- 与 `buildESSink` 共享 yaml `migrator.es.addrs`（读/写同集群）
- `es_test.go` 用 httptest.Server 作 mock（esapi.SearchRequest.ctx 是 unexported,不抽 ESClient interface,直接传 `*elasticsearch.Client`）;6 case 全 PASS
- README 附录 B 完整 demo：B.1 创建 → B.2 全量同步 → B.3 真对账 → B.4 src_overwrite_dst 真覆盖 ES → B.5 切流

### 三、canal cdc 运行环境落地

**协议层（之前已实现）**：`GoMySQLCanalClient`（go-mysql canal SDK）+ `CanalSource` + `BinlogClient` 接口。

**本轮补齐**：
- `config/local.yaml` + `test.yaml` 加 `migrator.canal.{addr,user,password,serverIdBase,flavor}`
- `deploy/docker-compose.yaml` MySQL `command:` 加 `--log-bin --binlog-format=ROW --binlog-row-image=FULL --server-id=1 --default-authentication-plugin=mysql_native_password`
- 新建 `deploy/mysql/init/01-canal-user.sql`：首次启动建 canal 用户 + REPLICATION SLAVE/CLIENT/SELECT 权限
- 新建 `integration/canal_e2e_test.go`（~150 行）：连真 MySQL,INSERT/UPDATE/DELETE 触发 BinlogEvent 到达;基础设施不可用 t.Skip
- README Step 6b cdc 完整 demo + 故障排查表

**canal 自动重连**（architect review P0 修复）：
- `Subscribe` 包重连循环,指数退避 1s→30s 封顶;`canalEventHandler` 跨重连周期共享 + `OnRotate/OnPosSynced` 实时跟踪最新位点
- `canalSrv` 改 `atomic.Pointer[canal.Canal]`,重连换新实例时 Stop 安全

### 四、架构简化（YAGNI / 删过度抽象）

- 删 `web/middleware/{rbac,idempotency}.go` + `repository/cache/idempotency.go` + `integration/authz_e2e_test.go`：RBAC 是占位 NoOp,Idempotency 调试期反而干扰;不可逆操作（switch/repair）由 MySQL 唯一索引 + 状态机 + IsRunning 兜底
- `ChangeEvent` 改成 `type ChangeEvent = BinlogEvent` 别名（字段完全一致,无业务转换）
- `BinlogClient.Subscribe(ctx, fromPos, fromGTID)` → `Subscribe(ctx, fromPos)`（fromGTID 一直 `_` 忽略,接口诚实优于占位）;CursorKindGTID 改返显式 error
- 删 es_test.go / canal_e2e_test.go 末尾的 `var _ = errors.New` 占位

### 五、prd/migrator 文档同步

- `02-architecture.md` 头部加"📌 与首版实现的差异"段（12 行表格映射目标态 vs 首版实际）
- `02b-task-module-design.md` / `03-walkthrough.md` 顶部加 caveat 指向差异段;`03 §8.2` SourceFactory 代码示例改新三方法接口
- `01-product.md` / `04b-cutover-checklist.md` 加 RBAC/Idempotency 实施差异注释
- `04-cutover-playbook.md` + 7 个 runbooks：批量 sed 删 `-H "Idempotency-Key: $(uuidgen)"` curl header 行;删 `IDEM()` shell helper + 13 处 `-H "$(IDEM)"` 调用
- 新增 `prd/migrator/adr/0002-source-factory-three-methods.md`：详细记录 SourceFactory 三方法决策的备选/理由/后果

### 六、incr binlogpos → checkpoint 持久化修复（合并 2026-05-26）

Canal 端 `buildBinlogEvent` 漏填 `BinlogPos` + `OnRow` 没透传 file/logPos → `BinlogEvent.BinlogPos==""` → `runPartition.flush()` 守卫 `if lastPos!="" && compareBinlogPos>0` 永不命中 → `migration_checkpoint` `phase=incr` 行从未入库 → 重启后从 master 当前位点起订，丢重启窗口事件。修复：`OnRow` 透传位点 + `buildBinlogEvent` 加 binlogPos 参数（`canal_client.go` + `canal_client_test.go` 3 子测试防回归）；首事件 file 空时留空串不写残废格式（守卫逻辑本身正确，从源头修）。

### 七、任意源框架：MySQL 单源 → 任意源可插拔（Mongo 全量+增量，合并 2026-05-27）

4 阶段：①PK 行标识全链路 int64→string（`Row`/`ChangeEvent`/`Mutation.PK` + `validate_log`/`dead_letter.biz_id`→`varchar(64)`），数值源分片键/全量游标保数值语义（**拆两层规避 `"10"<"9"`**）；②可插拔骨架：`domain.SourceType`(mysql/mongo) + `SourceFactory` 按源分发 `BuildFullSrc`/`BuildIncrSrc` + 新 `pipeline/transform`（`Transformer` 接口 + `Registry` 按 `TableMapping.Transform` 名选，空→Identity）；③`MongoSource` 全量（find 单 shard 流式 + ObjectID→hex）+ `MongoToRelational`（嵌套子文档/数组→JSON 列）+ full 引擎接入；④`MongoSource` 增量（Change Stream `Watch` + resume token 经 `BinlogPos` 复用引擎游标）+ incr 引擎接入。关键决策：**全量游标 int64→string「最后发出的 PK」**（源升序 last==max，零 MySQL 漂移 + 通吃非数值 PK）；transform 注册表建在 ioc `InitFullEngine/InitIncrEngine` 内（引擎 provider 签名不变，免 wire 重生成）。`go vet ./migrator/...` 全净（顺手清掉 `runPartition` pre-existing unreachable code）。新增 `pipeline/source/mongo.go` · `pipeline/transform/` · `consts.CursorKindResumeToken` · `deploy` 加 `webook-mongo` 单节点副本集 · `config` mongo 段 · `integration/mongo_{,incr_}e2e_test.go`。**两 e2e 对真副本集 Mongo 验证 PASS**。

### 八、扩展点 / v2 路线图（按生产场景演进）

v1 已交付全集功能；下列项为接口扩展点（v1 已抽象，按生产场景注入实现）或显式 v2 范围（需架构决策）。完整对照见 [`webook/migrator/CLAUDE.md`](webook/migrator/CLAUDE.md)「设计范围与扩展点」。

1. **DSN 真 Vault/K8s Secret 解析**：实现 `pipeline/dsn.PerTaskResolver` 替换 `StaticResolver`，按 `task.SourceDsnRef`/`SinkDsnRef` 解明文 DSN + LRU `*gorm.DB` 连接池；同模式扩展异构 sink（按 `task.SinkDsnRef` 支持多 ES/Kafka/CK/Mongo 集群）。
2. **多 cdc task 共享 binlog stream**（架构演进，per-task → per-process）：单进程一个 canal client + `IncludeTableRegex` 全订；`IncrEngine` 按 `task.tables` 过滤事件。影响 `BinlogClient` 接口，独立立项。
3. **Canal GTID 续订模式**：`BinlogClient.Subscribe` 加 `fromGTID` 参数 + `GoMySQLCanalClient` GTID 实现 + MySQL `--gtid-mode=ON --enforce-gtid-consistency=ON`。
4. **CK / Kafka verify-dst Source**：在 `SourceFactory.BuildDst` 加 switch case 复用对应 Source 实现（参考 mysql / es / mongo 分支模板，~200 行各）。
5. **RBAC scope 中间件回接**：webook-core 起 SSO 签发链路后，挂回 `web/middleware/rbac.go` 走 `migrator:{read,write,switch,repair}` scope 校验。
6. **Throttle 运行时实时调速**：`engine` 暴露 `SetConfig` + atomic 字段实时反映 yaml/Redis 改动。
7. **Canal Prometheus 指标增强**：`connection_status` / `events_total` / lag 平稳态（无事件时返 0 而非 stale）。
8. **dev/staging/prod yaml 按部署目标补完**：local.yaml 已配 `migrator.{canal,mongo,es}.*` 全集；其他环境按部署目标库参数复制即可。
9. **Mongo `_id` 全量游标 mid-shard resume**：`Source.FullScan` 签名扩展（影响所有 source）；v1 走 `MongoSink.ReplaceOne` upsert 幂等重扫兜底，已知设计选择。
10. **2026-05-26 之前的 webook_migrator 库一次性升级**：旧实例如已跑过 cdc，`migration_checkpoint.phase='incr'` 行可能为空（§六 修复前的现象），重启前手工 INSERT 一行兜底位点；见下「部署侧手动迁移 SQL」段。

### 部署侧手动迁移 SQL（已有 webook_migrator 库升级前必跑）

```sql
-- 1. validate_log 去重 + 换索引（去 repaired 字段）
DELETE v1 FROM migration_validate_log v1
  INNER JOIN migration_validate_log v2
  ON v1.task_id=v2.task_id AND v1.table_name=v2.table_name
     AND v1.biz_id=v2.biz_id AND v1.id < v2.id;
ALTER TABLE migration_validate_log
  DROP INDEX uk_migration_validate_log_dedup,
  ADD UNIQUE INDEX uk_migration_validate_log_dedup (task_id, table_name, biz_id);

-- 2. webook-mysql 容器升级让 docker-compose 新参数生效。**推荐方式（保数据）**:
-- 手动跑一次 canal 用户 SQL,然后 restart 容器,不删 volume:
-- mysql -h 127.0.0.1 -uroot -p13520 < deploy/mysql/init/01-canal-user.sql
-- docker compose -p webook-local restart webook-mysql
--
-- ⚠️ 暴力方式(会清 webook / chat / migrator 所有库,**先 dump 备份**):
-- docker exec webook-mysql mysqldump -uroot -p13520 --all-databases > /tmp/backup.sql
-- docker compose -p webook-local stop webook-mysql
-- docker volume rm webook-local_mysql-data
-- ./deploy.sh local

-- 3. 清残留 Idempotency Redis key（可选）
-- redis-cli -a <pass> KEYS 'webook:idem:*' | xargs redis-cli -a <pass> DEL

-- 4. 任意源框架 schema 升级（已有 webook_migrator 库,来自 05-27）：
ALTER TABLE migration_task ADD COLUMN source_type varchar(32) NOT NULL DEFAULT 'mysql' AFTER kind;
ALTER TABLE migration_validate_log MODIFY COLUMN biz_id varchar(64) NOT NULL;
ALTER TABLE dead_letter MODIFY COLUMN biz_id varchar(64) NOT NULL;
```

**会话**: 260525-migrator-Step8-对账调试 + 260526-migrator-binlogpos修复 + 260527-migrator-任意源框架（v1 合并）
**发布**: 待发布

---

## [2026-05-19] webook-migrator 部署 / CI / 监控同步（补齐 CLAUDE.md 服务拆分 14 项）

**变更内容**: 补齐 2026-05-16 migrator 服务初版遗留的 9 项部署/CI/监控同步，使其与 webook-core / webook-chat 处于同一就绪等级，可走 `./deploy.sh <env>` 起服务、被 Prometheus 抓取、Grafana 看板/告警生效、GitHub Actions 自动构建镜像

**影响范围**:
- 新增 `webook/migrator/Dockerfile`（多阶段构建，抄 chat/Dockerfile 改 build target = `./migrator`）
- 新增 `.github/workflows/webook-migrator-ci.yml`（lint-test + build-push，tag 模式 `webook-migrator-v*.*.*`）
- 新增 `deploy/grafana/provisioning/alerting/webook-migrator.yml`（up / 5xx / P99 / goroutines 4 类告警，`{job="webook-migrator"}` 限定）
- 新增 `deploy/grafana/provisioning/dashboards/webook-migrator.json`（8 panel：up + QPS + P50/95/99 + Go runtime 3 + 业务 lag/dead-letter 2）
- 改 `deploy/docker-compose.yaml`：加 `webook-migrator` service 段（depends_on mysql/redis、healthcheck :8083/health、mem_limit MIGRATOR_MEM 默认 384m）
- 改 `deploy/nginx/conf.d/default.conf`：加 `upstream webook_migrator` + `/api/migrator/` location（白名单 IP，剥 /api 前缀转 :8083）
- 改 `deploy/prometheus/prometheus.yml`：加 `job_name: webook-migrator` target `webook-migrator:8083/metrics`
- 改 `deploy/.env.{local,dev,staging,prod}` + 4 份 `.example`：加 `MIGRATOR_IMAGE_TAG` / `MIGRATOR_APP_ENV`（实际部署用最小集 2 个；example 完整含 `MIGRATOR_MEM/GOMEMLIMIT/GOGC` 5 个）
- 收尾延续：改 `deploy/docker-compose.local.yaml` 补 `webook-migrator` local override（chat/core/fe 都有，独缺 migrator → `./deploy.sh local` 会去拉 ghcr 失败）；`.env.local{,.example}` 加 `MIGRATOR_HOST_PORT=8083` 让 local override 暴露宿主端口
- 收尾延续：`webook/migrator/README.md` 加 0.7 docker compose 容器模式段（验证容器 + nginx + prometheus + grafana + CI 入口）+ 附录 A 业务侧 SDK 接入自测 8 步（搬自 `prd/migrator/code-reading-guide.md §16.5`）；§16.5 改成指针段引用 README 附录 A

**技术决策**:
- 不新建 `deploy/prometheus/rules/webook-migrator.rules.yml` — 现有 `webook-services.rules.yml` 已按 `sum by (job)` 自动覆盖任意新 job，migrator 加 prom job 即被记录
- dashboard 业务 metric 命名 `webook_migration_*`（subsystem 前缀），**不用** `webook_migrator_*`（service 前缀）— 遵 CLAUDE.md「禁止 `webook_<service>_*`」
- nginx `/api/migrator/` 默认仅放 docker bridge / 内网 IP（公网默认 deny），符合 architecture.md「迁移服务全闭网」要求
- 4 份 `.env.<env>`（实际部署）只加 IMAGE_TAG + APP_ENV，跟现有 CORE/CHAT 精简风格一致；4 份 `.env.<env>.example` 完整含 MEM/GOMEMLIMIT/GOGC，给部署者参考

**待办**:
- prod 上线前在 `.env.prod` 把 `MIGRATOR_IMAGE_TAG` 同步到推出的 `webook-migrator-v*.*.*` 真实 tag 号（同 CORE_/CHAT_ 规则）
- migrator 服务实装 `webook_migration_lag_seconds` / `webook_migration_dead_letter_total` 指标后 dashboard 业务 panel 自动生效

**会话**: 260519-migrator-部署同步

## [2026-05-16] webook-migrator 服务初版（多表 + Canal + 异构 Sink + 完整鉴权）

**变更内容**: 新增独立服务 `webook-migrator`（与 webook-core / webook-chat 并列），完整数据迁移框架 — 全量 / 增量 / 对账 / 切流 / 死信重放 14 endpoint + 业务侧 SDK 接入 + 三件套 PRD/架构/playbook 文档

**影响范围**:
- 新建 `webook/migrator/`（独立服务，与 chat 平级）：
  - `service/{full,incr,verify,switching}` 五大引擎；IncrEngine 含 partition 并行（FNV hash + errgroup + min-ckpt resume + ckpt 防回退）
  - `pipeline/source/`：`MySQLSource`（全量分页 SELECT）+ `CanalSource`（实现 `GoMySQLCanalClient` 基于 `go-mysql-org/go-mysql/canal` 真订阅 binlog）；`SourceFactory` 按 task.Mode 分发
  - `pipeline/sink/`：`MySQLSink`（INSERT ... ON DUP KEY UPDATE + Version 乐观锁）+ `ESSink` + `ClickHouseSink` + `MongoSink` + `KafkaSink`；`SinkFactory` 按 task.SinkType 分发
  - `domain.Task`：`Tables()` / `PickTable(idx)` 支持任务内多张表；`EncodeShardNo(tableIdx, shardNo)` 解决 checkpoint 表冲突
  - `web/`：14 endpoint（CRUD + Lifecycle 11 + Query）+ middleware（idempotency / audit / RBAC 4 scope）；handler 持 factory 运行时按 task.id 动态构造 Source/Sink
  - `repository/dao/`：5 张控制库表（task / checkpoint / validate_log / audit_log / dead_letter）
  - `config/`：5 份 yaml（local/test/dev/staging/prod），含 partitionCount + Canal / ES / CK / Mongo / Kafka 配置段
  - `ioc/`：8 Provider + InitRBACBuilder + Rate-Limit middleware
  - `integration/`：e2e（CRUD + idempotency + audit + 401/403/429 鉴权 5 子测）
- 新建 `webook/internal/migratorsdk/`：业务侧接入接口
  - `SwitchReader.ChooseSide` 按 stage + gray 决策 OLD/NEW 路由
  - `DualWriter.Write` 按 stage 分阶段双写策略
  - `NoOp` / `Redis` 两套实现 + `FailureRecorder` 双写失败兜底
- 主仓 `webook/internal/wire.go` + `migrator_sdk.go`：yaml flag `migrator.sdk.enabled` 决定注入 NoOp / Redis 实现
- 新建 `prd/migrator/` 文档套件：PRD / architecture / zero-downtime-playbook / task-module-design / code-reading-guide / cutover-checklist / 10 runbooks / drill-records / postmortems
- `webook/go.mod` 加 5 个异构 SDK 依赖：`go-mysql v1.15.0` / `elastic v8.19.6` / `clickhouse-go v2` / `mongo-driver v1.17.9` / `sarama`

**技术决策**:
- IncrEngine partition 并行：单一订阅 → dispatcher FNV-hash → N partition channel → 各 worker 攒批 / Sink.Apply / checkpoint。subscriber / dispatcher / workers 全进 errgroup，任一失败 gctx cancel 全部退出（无 goroutine leak）
- 多 partition resume 正确性：load 全部 partition ckpt → `min(各 partition CursorValue)` 作 IncrSubscribe 起点 + worker 保留 startPos 防 ckpt 回退（fast partition 重放安全靠 Sink 幂等 + Version 乐观锁兜底）
- BinlogPos 比较：先比 file 字典序（zero-padded `mysql-bin.000001` 保证字典序 = 数字序）再比 pos 数字（不能字典序 — `"100" < "99"` 字典序但 100 > 99 数值序）
- 任务内多表分发：checkpoint shard_no 编码 `tableIdx * ShardStride + realShardNo`（ShardStride=10000，每张表最多 1 万 shard，最多 21474 张表）
- factory 模式：引擎 / handler 持 factory，运行时按 task.id Get → BuildSrc/BuildDst 构造对应 Source/Sink；wire 不再注入静态 Source/Sink 实例
- Sink 异构分发：MySQLSinkFactory 默认 MySQL；task.SinkType=es/clickhouse/mongo/kafka → 调 heteroBuilder（ioc 注入，按 yaml 配 ES client / CK conn / Mongo Connect / Kafka SyncProducer）
- ESSink 乐观锁：external version_type + Mutation.Version → ES 自动拒老 version 写入（409 conflict，业务层视为正常跳过）
- ClickHouseSink：insert 走 ReplacingMergeTree(version)，delete 走 `ALTER TABLE ... DELETE`（CK 异步删除）
- KafkaSink：key=PK → HashPartitioner 同 PK 落同 partition 保单行顺序
- RBAC：4 scope（read/write/switch/repair）+ ScopeExtractor 函数式注入（生产 Redis lookup，本地 NoOp 全 scope）；挂到 14 endpoint
- Rate-Limit：复用 `pkg/ginx/middleware/ratelimit` 滑动窗口（默认 1 秒 100 req / IP，yaml 可覆盖）

**待办**:
- 部署到 dev 后跑端到端验证（部署同步已于 2026-05-19 补齐：Dockerfile / docker-compose / nginx / prometheus job + dashboard / grafana alerting + dashboard / .env MIGRATOR_* / GitHub Actions CI 全到位）

**会话**: 260516-migrator-完整版



## [2026-05-12] k8s 部署目录上移到仓库根 + 去冗余前缀

**变更内容**: `webook/k8s/`（10 个 YAML）整体上移到仓库根 `kubernetes/`，与 `deploy/` 并列；文件名去掉冗余的 `k8s-` 前缀
**影响范围**:
- `webook/k8s/k8s-*.yaml`（10 个）→ `kubernetes/*.yaml`（git rename，历史保留）
- `webook/mk/k8s.mk`：4 处路径改为 `../kubernetes/<name>.yaml`；顶部用法注释明确"必须在 webook/ 下执行"
- `webook/mk/infra.mk`：10 处路径改为 `../kubernetes/<name>.yaml`
- `webook/CLAUDE.md` L1 部署层章节：`k8s/`（将来式）改为 `kubernetes/`（已建）

**技术决策**:
- 位置上移：webook/CLAUDE.md 早有规划「K8s 与 `deploy/` 并列于仓库根，不挤压 deploy/」，本次落地
- 目录用 `kubernetes/` 全称：与 `deploy/` 一致采用通用全称，避免与 mk 文件名 `k8s.mk` 重复
- 文件去 `k8s-` 前缀：目录名已表意，`kubernetes/mysql-deployment.yaml` 比 `kubernetes/k8s-mysql-deployment.yaml` 干净
- Makefile 走 `../kubernetes/` 相对路径：调用方式 `make -f mk/k8s.mk` 隐含 working directory = `webook/`，顶部注释明确化避免误用

**会话**: 260512-k8s-目录上移



## [2026-04-28] 代码审查修复：鉴权收敛 + 缓存规则归位 + 配置一致性

**变更内容**: 修复连续三轮 ship review 发现的安全/规则问题 — 1 Critical（SSE ResumeStream 越权窃听他人对话流）+ 4 Important（IsGenerating 缺鉴权、ranking.Archive 与 ai.Dashboard 在 prod 暴露、UpdateContent 写后不清缓存）+ 多处 Suggestion（viper key 与 env 名不匹配致 prod 守卫失效、`// =====` 注释分隔线违规、chat local.yaml otel.env 标错、内部 config 注释漏 staging 维度）

**影响范围**:
- `chat/web/chat.go`：`ResumeStream` 增 UserClaims 提取 + `ListMessages` 归属探测（越权 → 404 阻断 SSE 建立）；`IsGenerating` 路由改 `WrapReqClaims[conversationIdReq, jwtx.UserClaims]`，handler 内同样走 ListMessages 探测
- `chat/repository/chat_message.go`：`UpdateContent` 写 DB 后自清缓存（Cache-Aside 完整）；删除 `DelMsgCache` 接口方法和实现 — 缓存职责回归 repository
- `chat/service/chat.go`：删除 `flushToDB` / `finalizeReply` / `savePartialReply` 中 3 处 `DelMsgCache(...)` 兜底调用
- `internal/web/click_event.go`：`/ai/dashboard` 加 `os.Getenv("DEPLOY_ENV") != "prod"` 路由守卫（与 ranking.Archive 同模式）
- `internal/web/ranking.go`：`/article/ranking/archive` prod 守卫从 `viper.GetString("deployEnv")` 改 `os.Getenv("DEPLOY_ENV")`（修复 gate 永不触发的 bug）
- `internal/web/article.go` / `internal/service/article.go` / `pkg/errs/error_test.go` / `pkg/errs/mapping_test.go` / `pkg/ginx/wrapper_errs_test.go` / `pkg/ginx/wrapper_variants_test.go`：7 处 `// =====` 分隔线改 Makefile 风格 `// ── 区域名 ──`（对齐 CLAUDE.md 注释风格）
- `chat/config/local.yaml`：`otel.env` 由 `"dev"` 改 `"local"`（避免 local trace 与 dev 服务器混存难分）
- `internal/config/local.yaml` + `test.yaml`：环境说明注释补齐 staging.yaml 一档（原文档过时只列 local/dev/prod 三档）

**技术决策**:
- ResumeStream 鉴权用 `service.ListMessages(uid, convId, 0, 1)` 当探测器，不新增 service 方法：复用既有「越权/不存在 → ErrConversationNotFound (404)」路径，不增加 API 表面积
- prod 守卫直读 `os.Getenv("DEPLOY_ENV")` 不走 viper：viper.AutomaticEnv 对非嵌套 key 会 lookup `DEPLOYENV`（uppercase 后无下划线），与 `.env.*` 里的 `DEPLOY_ENV` 对不上 — CLAUDE.md 已记「AutomaticEnv 对嵌套 key 不生效实测验证」，本次确认对扁平 key 同样有此陷阱
- UpdateContent 自清缓存而非保留 `DelMsgCache` 兜底：service 三处调用容易在新加调用点漏写；接口收敛后责任落到 repository 层，CLAUDE.md「写操作后必须清对应缓存」从字面规则变成结构保障
- Dashboard / Archive 用 prod 路由守卫而非 admin role middleware：项目当前没有 admin 角色概念，路由守卫是最小可行方案；二者性质一致（运维/调试用，业务无依赖）

**会话**: 260428-review-修复



**变更内容**: 新增 `pkg/redislockx`（bsm/redislock 底座 + 自研 Watchdog 续约 + OnLost 钩子）+ `pkg/cronx`（任务级 Prometheus 指标 + Wrapper 通用模板）；prometheus 子包对齐 `redisx/gormx` 的 builder 链式风格；`internal/job/ranking.go` 缩到薄壳，service 层完全不感知锁；archive 任务首次纳入分布式锁保护；cron 加 graceful Stop hook

**影响范围**:
- 新增 `webook/pkg/redislockx/`：`Client`/`Lock` 接口；`redisClient` 用 `bsm/redislock` v0.9.4 提供 Obtain/Refresh/Release，错误统一映射到本包 `ErrLockNotHeld`；`redisLock` 在 bsm 之上自研 Watchdog（redisson 招牌特性 bsm 不带）+ `OnLost` / `OnRefresh` 钩子（续约失败 / 续约成功可观测，回调内 panic 由 `safeOnLost` / `safeOnRefresh` 包 recover 防止拖崩进程）；**watchdog 默认 ttl/3 自动开**（对齐 Redisson `lockWatchdogTimeout/3` 行为，调用方无须显式启用）；`Options`：`WithWatchdog`（覆盖默认 interval）/ `WithoutWatchdog`（显式关闭）/ `WithRetryInterval` / `WithOnLost` / `WithOnRefresh`，`applyOptions(opts, ttl)` 助手统一去重
- 新增 `webook/pkg/redislockx/prometheus/`：builder 风格装饰器，自动给每次 TryLock/Lock 注入默认 OnLost；指标 `webook_lock_acquire_total{result=success/busy/error}` + `held_seconds` + `wait_seconds`（Lock 阻塞实际等待）+ `watchdog_lost_total`（锁中途丢失，幻觉持锁告警）
- 新增 `webook/pkg/cronx/`：`Wrapper` 把"抢锁→跑业务→Unlock + 4 组指标 + panic recover"模板封死，业务 Job 复用；3 个 Option（`WithNow` / `WithLockKeyPrefix` / `WithLockTTL`）；watchdog 由 redislockx 默认接管，wrapper 不重复暴露；recover 加 `runtime/debug.Stack()` 进 panic 日志
- 新增 `webook/pkg/cronx/prometheus/`：builder 链式风格，4 组指标 — `webook_cron_runs_total{task,result=success/failed/skipped/panic}`、`duration_seconds{task}`、`in_flight{task}`、`last_success_timestamp{task}`
- `internal/job/ranking.go`：缩到 ~50 行，只剩 entries 表 + `wrapper.Wrap()` 调用；4 个任务命名加 `ranking_` 前缀（`ranking_hot_recompute` 等）避免未来撞名；archive 任务首次纳入锁保护
- `internal/service/ranking.go`：`RecomputeHot/Best/New` 删除顶部 TryLock 模板，service 不再 import logger 用于锁日志
- `internal/repository/ranking.go` + `cache/ranking.go`：删 `TryLock` 接口方法和实现
- `internal/consts/cache.go`：删 `ArticleRankingLockPattern` + `ArticleRankingLockTTL`
- `ioc/lock.go`（新增）：`InitLockClient` 包 prometheus 装饰器；`ioc/cron.go`：`InitCron` 改返 `(*cron.Cron, func())`，cleanup 调 `<-c.Stop().Done()` 等 in-flight 跑完，进 wire cleanup chain；新增 `InitCronMetrics` + `InitCronWrapper`
- `wire.go` + `wire_gen.go`：注入 `InitLockClient` + `InitCronMetrics` + `InitCronWrapper`
- `mk/mock.mk` + mocks：`pkg/redislockx` 加入 mockgen 矩阵
- 测试新写：`pkg/cronx` 6（success/lockBusy/lockError/businessError/panic-with-stack/unlockIndependentCtx）+ `pkg/redislockx` 13（含 `DefaultWatchdog` / `WithoutWatchdog` / `OnLost`）+ `pkg/redislockx/prometheus` 4（含 watchdog_lost / wait_seconds）；`internal/job/ranking_test.go` 砍到 1 个入口测试（业务无关行为已被 cronx 单测覆盖）

**技术决策**:
- 底座选 `bsm/redislock` v0.9.4 而非自实现 SETNX+Lua：bsm 提供 token 校验、RetryStrategy 等成熟件，自研只剩 Watchdog（redisson 招牌特性 bsm 不带）；将来想换 redsync 多节点也只改 `client.go` 一处，`Client`/`Lock` 接口不动
- prometheus 装饰器靠"prepend WithOnLost"自动注入到每次 TryLock/Lock，**`NewClient` 签名零变更** —— 比加 ClientOption 工厂级默认更轻
- watchdog 默认开（ttl/3）对齐 Redisson：调用方"任何地方都能直接用"，无须每次记得 `WithWatchdog`；短临界区不想要后台 goroutine 用 `WithoutWatchdog()` 关
- service 层完全不感知锁：锁是部署形态决定的，不该污染业务接口
- Wrapper 抽到 `pkg/cronx`：业务无关模板，未来其他 Job 直接复用，无须复制 100 行
- Watchdog 30s TTL + 10s 续：实例 crash 30s 让贤；archive 任务 2min 也能持续续约
- task 名加 `ranking_` 前缀 + 锁 key 前缀 `cronx:lock:`：rolling deploy 容忍一次双跑（业务幂等），换来未来 Grafana 标签清晰
- 跳过 yaml 配置化（cron.lockTTL / watchdog 默认值已合理，无差异化需求时不开口子）
- 跳过 graceful shutdown 单测（chan 异步难真实，人眼审 wire_gen cleanup 链已够）

**监控模板**（同步随本次提交）：
- `deploy/prometheus/rules/webook-jobs.rules.yml`：**8 条 Recording Rules**（cron 成功率 + P50/P95/P99 三档耗时 + last_success_age + lock 错误率 / wait P99 / held P99）；模板版 `examples/recording-rules-example.yml`（与 `alerts-example.yml` 对仗的 config-type 命名，业务域无关）带完整注释。**Prometheus 规则目录从 `alerting/` 重命名为 `rules/`**，同步改 `prometheus.yml` 的 `rule_files: rules/*.rules.yml` + `docker-compose.yaml` 的 volume 挂载，与项目"Grafana Alerting + Prometheus Recording rules"架构对齐。**Dashboard 'Cron Duration' panel 改为读三档 record（perf 优化）**：每次 refresh 不再现算 `histogram_quantile`，CPU 开销由 N×panels 降至常量
- `deploy/grafana/provisioning/alerting/webook-jobs.yml`：6 条告警（CronTaskStale / CronPanicSpike / WatchdogLost / LockAcquireErrorRate / LockWaitP99High / CronInFlightStuck），全 Q→Reduce→Threshold 评估链路；模板版 `examples/alerting/rules-recording-example.yml`
- `deploy/grafana/provisioning/dashboards/webook-jobs.json`：14 panel 专题面板（cron 4 + lock 4 + 概览 4 + 健康 2），含 `$task` 变量过滤；模板版 `examples/dashboards/webook-jobs-example.json` 每 panel 带 description
- `webook/tools/check_monitoring.sh`：YAML/JSON 语法 + 元素数量校验脚本（promtool 不可用环境的替代）

**待办**:
- 部署到 staging 后观察 `webook_cron_*` 与 `webook_lock_*` 指标，配 Grafana 面板和 "X 分钟没成功过" / "watchdog_lost_total > 0" 告警
- transient Refresh error（非 ErrNotObtained）目前 silently retry 无指标，未来若发现 Redis 抖动场景需告警可加 `refresh_error_total` counter 或扩展 OnLost 在连续 N 次失败时触发

**会话**: 260426-定时任务-分布式锁与数据采集
**发布**: -

## [2026-04-23] 榜单移动端适配 + 切 tab 体验 + 后端并发 perf

**变更内容**: 榜单搜索页 5 项移动端响应式 + URL 状态持久化 + 200ms 延迟清空策略 + 后端并发优化 + trend bug fix；三份 CLAUDE.md 补硬规则

**影响范围**:
- 前端 `views/search/RankingBoard.tsx`：Header 堆叠、Tabs flex-wrap、Item meta 两行、Pagination simple + 对齐 `article/list.tsx`；URL 参数驱动（`dim/rcat/rdate/rpage/rsize`）；Spin 冻结 + minHeight 锁高 + 200ms 延迟清空；抽 `RankingItem` + `React.memo` 防 countdown tick 触发全量 re-render
- 全局 `app/globals.css`：`@layer tw-utilities` 全局 `scrollbar-gutter: stable`；`AppLayout.tsx` `<main>` 改 `overflow-y-auto`
- 后端 `repository/ranking.go`：`fallbackFromDAO` 删覆盖 snapshot trend 的 2 行（bug fix，归档 JSON 里原本就有 `trend:"up/down/new"` 被静默覆盖成 same）；`Top()` 用 `errgroup` 并发 `GetDetails` + `GetPrevRanks`
- 后端 `service/ranking.go`：`RecomputeHot` 5 分区 fan-out `errgroup`
- `scripts/webook.sql`：article / published_article 扩到 25 条；prev1 hot 归档扩到 25 条，满足 pageSize=20 分页阈值
- 原型：`prd/chat/chat.pen` 新增 `08-榜单-移动端` frame（tVVgY）；`prd/ranking/ranking-search-page-mobile.png`；PRD.md §6.2 移动端章节
- 规范：根 `CLAUDE.md` 加「侦察优先」「Pencil 原型修改」「动手前先出方案」；`webook-fe/CLAUDE.md` 加「列表分页状态规则」「跨页面模式对齐」「Effect 依赖稳定性」「常量集中」「CSS 规则就近 vs 全局」

**技术决策**:
- 移动端 pen 从 `rSwsj` 复制而非从零搭；硬编码进度条 width 按新 viewport 比例重算（225/175/158/119/99/86）
- URL 参数 `r` 前缀避免和 `/search?page=` 搜索分页撞名
- 切 tab 用"延迟 200ms 清空 + Skeleton 兜底"：快响应直接 commit 无感切换，慢响应才露 Skeleton
- `scrollbar-gutter` 挂在 `<main>` 而非 `html/body`（AppLayout `overflow-hidden` 下 body 不滚）；`@layer tw-utilities` 压过 Tailwind utility
- Pagination 完全对齐 `article/list.tsx`：`showTotal + showSizeChanger + showQuickJumper + size='small' + pageSizeOptions=['10','20','50']`

**待办**:
- 默认值魔数（`'hot'` / `'tech'` / `1` / `20`）可抽 `DEFAULTS` 常量
- `searchParams.get('dim')` 建议加白名单校验
- 748 行的 `RankingBoard.tsx` 可进一步拆 Header/Tabs 子组件

**会话**: 260423-ranking-移动端-规则沉淀

**发布**: 待补

## [2026-04-22] 文章日榜 Top100

**变更内容**: 搜索页落地文章日榜（热度/最新/最佳/分区 4 维度 Top100），支持实时刷新 + 次日归档 + 历史回看

**影响范围**:
- 后端全栈新增：`dao/ranking.go` / `cache/ranking.go` / `repository/ranking.go` / `service/ranking.go` + `ranking_scorer.go` + `ranking_hook.go` / `web/ranking.go`
- cron 业务层新建 `internal/job/` 目录（`ioc/cron.go` 只做生命周期）
- `article` / `published_article` 表加 `category` 列 + 索引；新增 `article_ranking` 表 + 视图 `v_article_ranking`
- 前端 `views/search/RankingBoard.tsx` + `api/ranking.ts` + `types/ranking.ts`；搜索页无 query 时默认落地榜单
- 通用工具：`pkg/ginx/page_result.go`（`PageResult{List any, Total int64}`），同步 `article.go` 两处分页接口复用
- 规范：`CLAUDE.md` 补"Handler/Service/Repo/DAO 构造函数必须返回接口"+"表名单数"

**技术决策**:
- 公式：热度 `(click + 3·like + 5·collect) / (hours+2)^1.5`；最佳 Wilson 下界（`clicks ≥ 50`）；最新按 `publish_ts`（24h + `clicks ≥ 10`）
- 存储：Redis ZSet 实时（1min 重算 + 增量 ZINCRBY）+ MySQL 归档；cron 分布式锁用 Redis SETNX
- cron 库：`robfig/cron/v3`，多实例靠 SETNX 抢占实现分布式
- date 字段：`varchar(10) 'YYYY-MM-DD'` 作日分区键（行业标准，非 int64 毫秒戳）；业务时区 **Asia/Shanghai**，三端对齐（后端 `carbon.Now()` 走全局 TZ、前端 `toLocaleDateString('sv-SE', {timeZone:'Asia/Shanghai'})`、SQL `CONVERT_TZ(UTC_TIMESTAMP(), '+00:00', '+08:00')`）
- 接口命名：泛型接口 `RankingService/Repository/Cache/DAO`，实体实现 `ArticleRankingService` 等（未来 `UserRanking...` 可共享同接口）
- 分页响应：`ginx.PageResult{List any, Total int64}` 对齐前端 `types.PageResult<T>`
- 点击追踪：`ClickEvent.source` 编码为 `"ranking:{dim}:{rank}"`，不改表结构

**待办**:
- cron 仍是测试节奏（10s/15s/5s/10min），上线前切回生产节奏（1min/5min/30s/每日 00:10）
- `rankingCandidates=2000` / `bestMinClicks=50` / `newMinClicks=10` 可配置化
- 归档页面路由 `/ranking/archive?date=...` 未单独实现，通过 Drawer 切换日期达成

**会话**: 260422-ranking-文章榜单

**发布**: 待补

## [2026-04-21] v1.0.0 基础设施增强：端口避让 + ghcr 源切换 + 懒拉镜像

**变更内容**: 围绕 prod 正式发版 (`webook-v1.0.0` / `webook-fe-v1.0.0`) 做了三件部署基建的事：

1. **prometheus 宿主端口重排**：dev/staging/prod 从 9090/9091/9092 → 9090/9190/9290，与 `KAFKA_EXTERNAL_PORT`（9094/9194/9294）百位 tier 对齐；prod 原 9092 撞 Kafka 业界默认端口
2. **ghcr 镜像源可切换**：`.env.*` 加 `GHCR_REGISTRY`（默认 `ghcr.io`），`deploy.sh` 加 `--ghcr <host>` / `--ghcr=<host>` flag 单次覆盖（不改 env 文件）；配合 README 新增的阿里云 ACR 反代教程，为国内 prod 拉取慢提供方案
3. **up 不再自动拉镜像**：compose 17 个 service 全加 `pull_policy: missing`，`deploy.sh` 的 `up` 分支删掉 `$COMPOSE pull`——镜像缺失才拉，已有直接启动；想显式刷新走 `./deploy.sh <env> pull`

**影响范围**:
- `deploy/docker-compose.yaml`（webook/webook-fe image 路径变量化、17 service 加 pull_policy）
- `deploy/deploy.sh`（新增 `--ghcr` flag 解析、删 up 里的 auto pull）
- `deploy/.env.{local,dev,staging,prod}` + 4 份 `.example`（新增 `GHCR_REGISTRY`、改 `PROMETHEUS_PORT`）
- `deploy/README.md`（端口表 + ghcr 切换节 + 阿里云 ACR 反代节 + pull_policy 说明）

**技术决策**:
- **端口**：百位 tier 对齐 kafka 系，dev 保持社区默认 9090；Grafana → Prometheus 走容器内 `webook-prometheus:9090`，不受宿主端口改动影响
- **ghcr 源**：只做 ghcr，不碰 Docker Hub（后者走 daemon `registry-mirrors` 更标准）；CLI flag 用 shell export 覆盖 `--env-file`，优先级天然更高。**实测公共 pull-through 代理（nju 等）拉不了私有 GHCR package**（404 manifest unknown），因此仓内默认官方源；要用加速域需把 package 改 public，或走阿里云 ACR 反代（教程见 README）
- **pull_policy**：默认非强制 pull 提速启动；显式刷新和容器启动分离，心智清晰。local 保持 `$COMPOSE build`，和 `docker-compose.local.yaml` 里 `pull_policy: never` 一起保证永远用本地构建产物

**坑**:
- **同 tag 远端被重推**：`pull_policy: missing` 判定"镜像已存在"不拉，继续用旧镜像。v1.0.0 force-tag 场景必须 `./deploy.sh <env> pull` 显式刷新后再 up
- **防火墙/安全组**：服务器若放通过 9091/9092 入站，需改放通 9190/9290

**用法**:
```bash
# 按需切 ghcr 源（前提：package 已 public）
./deploy.sh prod --ghcr <mirror>
# 或改 .env.prod: GHCR_REGISTRY=<mirror>（永久）

# 同 tag 刷新要显式 pull 再 up
./deploy.sh prod pull && ./deploy.sh prod
```

**会话**: 260421-cicd-L1发版v1.0.0

## [2026-04-20] L1 部署体系：deploy/ 单源 + 一份 compose + 多 env 切换

**变更内容**: 完成 L1 部署 + CI 体系，最终落地的状态：

- `deploy/` 是部署唯一真相源（根目录原 `docker-compose.yaml` / `nginx/` / `prometheus/` / `grafana/` / `otel-collector/` 全部清空）
- `deploy/docker-compose.yaml`：一份 all-in-one（17 服务：webook + webook-fe + 中间件 + 监控栈 + exporters + nginx）
- `deploy/docker-compose.local.yaml`：override 文件，本地从 ghcr 改成本地 build + 暴露宿主端口
- `deploy/.env.{local,dev,staging,prod}(.example)`：四份 env 模板入库（占位 `CHANGE_ME`），真实 `.env.*` gitignored
- `deploy/deploy.sh <env>`：一键切换，`local|dev|staging|prod`，子命令 `up/down/nuke/logs/status/pull/restart`
- `deploy/nginx/conf.d/default.conf`：一份配置（通配 `server_name _`），不再按 env 拆
- `deploy/prometheus/` + `deploy/grafana/` + `deploy/otel-collector/`：完整 provisioning + examples（教学参考）
- `deploy/grafana/Makefile`：Grafana 运维命令（仪表盘导出/导入等）
- `deploy/README.md`：架构图 + 端口表 + 首次部署序列 + 日常操作

**多环境机制**:
- project 名按 env 分（`webook-dev` / `webook-prod`）→ volume 自然隔离 `webook-<env>_*`
- container_name 不带 env 前缀（`webook-app` / `webook-mysql`）→ docker 全局唯一，**同时只能跑一套**（切换前自动 stop）
- `APP_ENV` 由 `.env.<env>` 注入 → webook 加载对应 `config/<env>.yaml`
- 切环境 volume 不清，dev 数据切到 prod 再切回还在；显式清要 `nuke` 子命令

**配置体系（混合方案）**:
- `webook/config/{local,dev,staging,prod}.yaml` 同构按 env 差异化（otel.env / sampleRatio / logger 级别）
- 敏感字段（`mysql.dsn` / `redis.password`）yaml 占位 `OVERRIDE_VIA_ENV`，运行时由 `.env.<env>` 注入
- `webook/main.go` `initViperV2` 加 `viper.AutomaticEnv()` + `SetEnvKeyReplacer('.','_')` —— L2 K8s Secret 注入直接可用

**服务引用 K8s 心智对齐**:
- nginx / prometheus / grafana / otel-collector 跨服务引用全用 service name DNS（不写 IP）
- L2 K8s Service name 一一对应，提前练 K8s 思维

**CI 流水线**:
- `.github/workflows/webook-ci.yml` + `webook-fe-ci.yml`：main/feature push 触发，build → push ghcr，tag 含 `main-latest` 滚动 + 版本 tag
- `webook/Dockerfile` `ARG VERSION` + `LABEL org.opencontainers.image.version`，CI 注入版本元数据
- `paths-ignore '**.md'` 文档改不触发 CI

**影响范围**:
- `deploy/`（新建目录 + 完整文件集 ~25 文件）
- `webook/Dockerfile` / `webook/main.go`（CI 元数据 + AutomaticEnv 钩子）
- `webook/config/{local,dev,staging,prod}.yaml`（命名标准化 + 混合配置）
- `.github/workflows/webook-ci.yml` / `webook-fe-ci.yml`
- 根目录大量删除（旧 `docker-compose.yaml` / `nginx/` / `prometheus/` / `grafana/` / `otel-collector/`）
- `webook/CLAUDE.md` 加「环境说明」+「部署层」章节
- `.gitignore`（加 `!.env.*.example` 允许模板入库）

**技术决策**:
- 一份 compose + env 切换 vs 多 project 拆分 + 共享 infra：选前者，避免跨 project extnetwork 复杂度；故障域简单清晰
- 切 env 不清 volume：保留历史数据可随时切回；显式 nuke 才清
- 监控栈每环境重建：prometheus 数据单 env 独立不混淆，符合 L1「每 project 独立全套」哲学
- 服务引用用 service name 不用固定 IP：对齐 L2 K8s 心智模型（K8s 没人手写 IP）
- yaml 分环境 + env 注 secrets 的混合方案：PR review "改 dev 配置" 直接看 dev.yaml diff；微服务 N×M 不用维护 N×M env 变量
- prod tag 强校验语义化版本（`x.y.z`）：倒逼走 `git tag webook-v*` 流程，不允许 `main-<sha>` 上 prod
- example 用 `CHANGE_ME` 占位：强制部署者主动填密码，比示例写明文更安全

**待办**:
- L2：K3s + ArgoCD + Helm + Secret 分环境（AutomaticEnv 钩子已预埋，Secret 注入链路就绪）
- CI 接入集成测试（需 MySQL + Redis + Kafka + ES services，性能 / 成本另议）
- 实机验证：服务器 192.168.150.101 `git pull` + `./deploy.sh dev` 全 17 服务启动成功

**会话**: 260420-cicd-L1阶段一与命名标准化

## [2026-04-20] GORM tracing 避开 AutoMigrate 启动噪音

**变更内容**: `webook/ioc/db.go` 把 `dao.InitTable(db)` 从 `tracing.NewPlugin` 注册之后调整到之前，让 AutoMigrate 的 `SELECT information_schema.statistics ...` 等系统表查询不再产生 `gorm.Query` / `gorm.Row` span。

**影响范围**:
- `webook/ioc/db.go`（仅调整 InitTable 与 tracing plugin 的注册顺序）

**技术决策**: gorm opentelemetry plugin 不支持按 database/table 过滤 span；`WithDBSystem` 只贴属性、不过滤。评估过 Collector 侧 filter processor（事后丢弃，浪费）、移除 Row callback（治标不治本）、换 uptrace/otelgorm（依赖变更大），最终选顺序调整——最小改动解决"启动期 information_schema 噪音"这个主要痛点。gormprom 保留在 InitTable 前，启动 SQL 仍会进 Prometheus metrics（聚合值影响忽略）。

**会话**: 260420-observability-采样率公式

## [2026-04-20] OTel Collector 升级 + 采样率公式文档化

**变更内容**: otel-collector 镜像 `0.88.0 → 0.105.0`；`webook/config/prod.yaml` 新增 `sampleRatio` 计算公式注释（`min(1.0, TargetTracesPerSec / AvgQPS)`）并在 `docs/opentelemetry/06-best-practices.md` 同步；`dev.yaml` 加公式引用注释。

**影响范围**:
- `docker-compose.yaml`（collector 版本）
- `otel-collector/config.yaml`（版本注释同步）
- `webook/config/prod.yaml` / `webook/config/dev.yaml`（采样率说明块）
- `docs/opentelemetry/06-best-practices.md`（新增「采样率计算公式」小节）

**技术决策**: 未选 0.116.0 因 CentOS 7 kernel 3.10 跑不起（`exec: no such file or directory`，实为新 Go runtime syscall 不兼容）；0.105.0 是经验证可运行的上限。采样率公式两处同步（配置 + 文档），注释明确标注"请勿删除"。

**待办**: 后续内核升级到 ≥5.x 或迁移 Rocky/Alma 9 后，可尝试升到 0.116+。

**会话**: 260420-observability-采样率公式

## [2026-04-19] 服务总览 dashboard + 监控栈自监控（Grafana/OTel/Zipkin）

**变更内容**: 新增 services-overview 大盘覆盖 up/主机/Go/MySQL/Redis/Kafka/监控栈/Zipkin 六大区；Prometheus 加 grafana/otel-collector/zipkin 三个 scrape job；OTel Collector 暴露 :8888 自监控；Zipkin 换 slim→full 以保留 /prometheus 端点

**影响范围**:
- `grafana/provisioning/dashboards/services-overview.json`（新）
- `prometheus/prometheus.yml`（+3 jobs，grafana 带 basic_auth）
- `otel-collector/config.yaml`（telemetry.metrics.address）
- `docker-compose.yaml`（zipkin 镜像换 full）

**技术决策**: grafana 显式带 basic_auth 兜底 metrics endpoint 可能 401；zipkin-slim 砍了 micrometer 所以换 full，+200MB 可接受

**会话**: 260419-ops-服务总览

## [2026-04-19] 配置模板参考体系 + Grafana/Prometheus examples 目录

**变更内容**: 为 `grafana/` 和 `prometheus/` 下每种配置文件建立完整注释的 `-example.yml` 模板 + 两份字段字典文档；Prometheus 启用 `--web.enable-lifecycle` 支持热 reload；Grafana dashboards 开启生产三件套

**影响范围**:
- 新增 `grafana/examples/`（独立目录，不被 Grafana 加载）：
  - `alerting/{contactpoints,policies,rules}-example.yml`（4 种告警范式、Alertmanager 模板语法、路由 + matcher + 静默时段）
  - `dashboards/{dashboards-example.yml, webook-example.json}`（provider 生产三件套 + 8 种 Panel 骨架 + 变量 + gridPos）
  - `datasources/{prometheus,zipkin,mysql,loki}-example.yml`（exemplar 联动、tracesToMetrics/Logs、只读账号、derivedFields）
  - `README.md` 对照表 + 使用流程
- 新增 `prometheus/examples/`：
  - `prometheus-example.yml`（global/scrape_configs/alerting/relabel/remote_write/多种 SD）
  - `alerts-example.yml`（Prom 原生告警规则 + recording rules + 模板变量说明）
  - `README.md` 解释两种告警方式（Prom 原生 vs Grafana Alerting）对比
- 新增文档：
  - `docs/grafana/08-alerting-template-reference.md`（Contact Point 邮件模板全字段字典 + Alertmanager template 语法 + 陷阱 cheatsheet）
  - `docs/grafana/09-provisioning-reference.md`（datasources/dashboards/rules/policies/Makefile 字段字典，定位为参考不是模板）
- 文档索引：`docs/grafana/README.md` + `docs/prometheus/README.md` 加"即开即用 example"板块
- 生产三件套落地：`grafana/provisioning/dashboards/dashboards.yml` 三件套全关（`disableDeletion:true editable:false allowUiUpdates:false`），UI 只读防误改
- Prometheus reload API 开启：`docker-compose.yaml` prometheus 服务加 `--web.enable-lifecycle`，`curl -X POST /-/reload` 即可热加载规则
- 模板修复（上轮未完全收敛）：
  - `grafana/provisioning/alerting/contactpoints.yml` 改成 Alertmanager template 语法（`.SortedPairs` 代替 `range $k,$v`）
  - `grafana/provisioning/alerting/rules.yml` 所有规则补 Reduce 节点，不再 "invalid format of evaluation results"
  - `otel-collector/config.yaml` verbosity 改回 basic（调试结束，detailed 日志量过大）
- 新增 `grafana/mk/grafana.mk`：Makefile 封装 reload / test-email / restart 命令

**技术决策**:
- `-example.yml` 文件单独放 `examples/` 不放 provisioning 下：Grafana 按扩展名加载，避免"测试模板被当真实数据源"；`.example` 后缀也能达到同样效果但 `-example.yml` + 独立目录更直观
- 文档 vs example 双份存在的定位：文档是"字段字典 + 坑速查"，examples 是"复制即用"。都改时以 examples 为准，文档只更新字段释义
- dashboards 生产三件套全关：UI 改动被重启覆盖会产生"诡异现象"（改完又消失），强制 Git 化唯一 source of truth

**待办**: 无
**会话**: 260419-template-reference-体系建设

## [2026-04-19] OTel 调用链完整串联 + Kafka 连接重试 + Grafana tracing dashboard

**变更内容**: 两个关键 bug 修复让 OTel trace 真正成为一棵完整的树；Kafka producer 启动竞态修复；Grafana 增加 tracing 入口 dashboard + 文档同步

**影响范围**:
- Bug 1：HTTP span 与 DB/Redis span 断成独立 trace
  - 根因：Gin 1.11 `*gin.Context.Value()` 默认不 fallback 到 `c.Request.Context()`
  - 修：`webook/ioc/web.go` `server.ContextWithFallback = true`
  - 测：`webook/ioc/web_ctx_propagation_test.go` 单测复现 true/false 两种行为
- Bug 2：Kafka producer 降级为 NoopProducer
  - 根因：webook 启动早于 Kafka JVM ready，`InitSaramaSyncProducer` 一次性失败返回 nil
  - 修（代码）：`webook/ioc/kafka.go` 新增 `retryConnect` 泛型工具，指数退避重试 6 次（共约 63s）；Producer + Client 都用
  - 修（编排）：`docker-compose.yaml` Kafka 加 healthcheck + webook `depends_on.kafka.condition: service_healthy`
  - 测：`webook/ioc/kafka_test.go` 3 用例（成功/耗尽/指数 backoff）
- Grafana tracing dashboard：`grafana/provisioning/dashboards/webook-tracing.json`
  - 5 个 panel：Text 说明 + QPS/错误率/P99/Goroutine Stat + HTTP QPS by path + 分位趋势
  - 顶部 link 跳 Zipkin UI + Webook/Overview
- Collector 版本：`otel/opentelemetry-collector-contrib:0.116.0` → `0.88.0`（CentOS 7 kernel 3.10 兼容）
- 文档同步：
  - `docs/opentelemetry/05-integration.md`：从"集成蓝图"改成"已落地 + 4 大踩坑记录"
  - `docs/opentelemetry/README.md`：加生产接入状态图（5 层 instrumentation + OTLP → Collector → Zipkin）
  - `docs/opentelemetry/06-best-practices.md`：踩坑清单追加 #13-16（Gin fallback / 启动竞态 / Collector kernel / IBM sarama）
  - `grafana/provisioning/dashboards/README.md`：新增"项目自有 Dashboard"表格

**技术决策**:
- ContextWithFallback=true 是 Gin + OTel 必选，单测保护防回归
- Kafka 重试走双保险：compose 层解决启动顺序，代码层解决运行期抖动
- retryConnect 抽成泛型工具，Producer/Client 共享，单测独立验证
- Collector 降到 0.88.0 兼容老 kernel，直接转发 OTLP → Zipkin，业务代码零改动

**生产验证（SSH）**:
- `Container webook-kafka Healthy` → webook 等 Kafka healthy 才起
- `docker logs webook-app | grep kafka` 空 → 无连接失败
- Zipkin API `?serviceName=webook&spanName=/article/reader/page` → 19 个 span 完整树形
- `[SERVER] /article/reader/page` 根下挂 `eval/get/select published_article×2/set/hgetall×10/select interaction`

**待办**: Service 层手动埋点（Article.Publish / Chat.Send）；Prometheus exemplar（Histogram observe 附 trace_id 实现大盘跳 trace）

**会话**: 260419-otel-fix-ctx-propagation+kafka-retry

## [2026-04-18] OpenTelemetry 全链路接入 + Grafana 邮件告警

**变更内容**: webook 集成 OpenTelemetry（OTLP/gRPC → otel-collector → zipkin），覆盖 HTTP 入口（otelgin）/ SQL（otelgorm）/ Redis（redisotel）；Grafana 接 Zipkin 数据源 + 配置 5 条核心告警 + QQ 邮箱 SMTP 通知

**影响范围**:
- OTel SDK 初始化：`webook/ioc/otel.go`（OTLP/gRPC exporter + Resource + ParentBased(TraceIDRatioBased) sampler + BatchSpanProcessor + W3C TraceContext propagator + wire cleanup）
- Wire 注入：`webook/wire.go` 加 `ioc.InitOTel`，签名改为 `(App, func(), error)`；集成测试 setup wire 加 `noop.NewTracerProvider`
- HTTP：`webook/ioc/web.go` `InitMiddlewares` 加 `tp trace.TracerProvider` 参数，挂 `otelgin.Middleware("webook", WithTracerProvider(tp))` 紧随 Prometheus 之后
- DB：`webook/ioc/db.go` `db.Use(tracing.NewPlugin(WithoutMetrics(), WithoutQueryVariables()))`
- Redis：`webook/ioc/redis.go` `redisotel.InstrumentTracing(client)`
- Kafka：`webook/ioc/kafka.go` 暂保留 TODO 注释（otelsarama 未适配 IBM/sarama）
- main：`webook/main.go` 处理 wire cleanup，进程退出 flush span
- 配置：`webook/config/{dev,prod}.yaml` 加 `otel` 块（dev → `localhost:4317`，prod → `otel-collector:4317`）
- 依赖：`webook/go.mod` 加 OTel SDK v1.32.0 + otlptracegrpc v1.32.0 + otelgin v0.57.0 + gorm.io/plugin/opentelemetry v0.1.16 + redisotel/v9 v9.18.0
- Mock 重生：`make -f mk/mock.mk mockgen`（go-redis 因 redisotel 升级，旧 mock 缺新方法）
- Collector：`otel-collector/config.yaml` 新增（OTLP receiver + memory_limiter + batch processor + zipkin exporter）
- Compose：`docker-compose.yaml` 加 `otel-collector` 服务（image `otel/opentelemetry-collector:0.116.0`，IP 172.21.0.29，port 4317）
- Grafana 数据源：`grafana/provisioning/datasources/zipkin.yml` 新增（uid=zipkin，关联 prometheus）；`prometheus.yml` 加 uid=prometheus 固定
- Grafana 告警：`grafana/provisioning/alerting/{contactpoints,policies,rules}.yml` 三件套
  - rules：5 条核心告警（服务 up / 5xx 率 > 1% / P99 > 500ms / Goroutine > 10000 / MySQL 连接 > 80%）
  - policies：按 alertname+severity 分组，critical 立即发，其它攒批 5m
  - contactpoints：邮件，收件人 `3236447743@qq.com`
- Grafana SMTP：`docker-compose.yaml` grafana 服务加 `GF_SMTP_*` 环境变量（QQ 邮箱 587 STARTTLS），凭证从 `.env` 注入
- 凭证管理：`.env`（本地真实，已 .gitignore）+ `.env.example`（模板，可入库）；`.gitignore` 加 `.env` `.env.*` 规则

**技术决策**:
- Exporter 选 OTLP/gRPC（CNCF 标准）+ otel-collector 中转：换后端只改 collector 出口，业务代码零改动；未来加 Tempo/Datadog 多 fan-out 也只动 collector
- Collector 用 core 版（80MB）而非 contrib（350MB）：仅需 OTLP receiver + zipkin exporter，core 全覆盖
- otelgin 紧随 Prometheus 之后：让所有下游中间件 / handler 都在 root span 上下文里
- Init 走 wire `(T, func(), error)` cleanup pattern：与项目既有 wire 风格一致，BatchSpanProcessor 队列退出时 flush
- Sampler 用 `ParentBased(TraceIDRatioBased)`：跨服务时跟随上游决策，避免一条 trace 一半在一半丢
- Kafka 暂未接 OTel：otelsarama 未适配 IBM/sarama，等社区或自己写包装；当前 producer/consumer span 缺失，TODO 标记
- gormopentelemetry 用 `WithoutMetrics()`：与现有 gormprom 互斥避免重复采集
- gormopentelemetry 用 `WithoutQueryVariables()`：SQL 参数可能含 PII，默认隐藏

**部署步骤（VM 上）**:
1. `git pull` 同步代码
2. `cp .env.example .env && vi .env` 填 SMTP 授权码（或 scp 已填好的 .env 过去）
3. `docker pull otel/opentelemetry-collector:0.116.0`（或国内镜像 retag）
4. `docker compose build webook && docker compose up -d`
5. 验证 trace：`curl http://192.168.150.101/api/user/login` → Zipkin UI `:9411` 看到 root span + DB/Redis 子 span；Grafana `:3001` Explore 选 Zipkin 也能查
6. 验证告警邮件：手动触发（如 stop webook → up == 0）等 1m 后收 QQ 邮箱

**待办**:
- Phase 3：service 层手动埋点（Article.Publish / Chat.Send / User.Login 等关键路径加 attribute / RecordError）
- Kafka OTel：自写 sarama producer interceptor / consumer middleware 注入 trace context 到 Kafka header
- Grafana exemplar：Prometheus Histogram observe 时附 trace_id，大盘高延迟点直跳 Zipkin

**会话**: 260418-otel-Phase1to5全链路接入

## [2026-04-18] 前后端 CI 完善 + GHCR 镜像推送 + nginx 同源部署

**变更内容**: 补齐前端 CI；前后端 CI 均扩展 `build-push` job 推镜像到 GHCR；docker-compose 新增 webook-fe + nginx 同源反代，实现浏览器只见一个域名、零 CORS
**影响范围**:
- 前端 CI 新建：`.github/workflows/webook-fe-ci.yml`（eslint + tsc + next build；main/feature/webook-fe-v 三种 tag 推 `ghcr.io/<user>/webook-fe`）
- 后端 CI 扩展：`.github/workflows/webook-ci.yml` 加 `build-push` job，先 `go build -tags=k8s` 再 docker build（保留简单 Dockerfile 策略）；tag `webook-v*.*.*` 也触发
- 前端 Dockerfile：`webook-fe/Dockerfile` npm registry 改 `ARG NPM_REGISTRY` 可传参（默认官方源，CI 用默认，本地按需切 npmmirror）
- 部署栈：`docker-compose.yaml` 加 `webook-fe`（3000 仅 expose）+ `nginx`（80 对外，同源反代：`/` → 前端，`/api/*` → 后端）
- nginx 配置：`nginx/nginx.conf` + `nginx/conf.d/webook.conf`（upstream + 安全头 + healthz + 代理参数）
- Workflow 完善：两个 workflow 加 `workflow_dispatch`（可手动触发）和顶层 `permissions: contents: read`
**技术决策**:
- 同源部署：前端 `NEXT_PUBLIC_API_BASE_URL=/api`（相对路径）**一次构建多环境通用**，不用再为不同 API 地址构建多个镜像
- Workflow `paths` 过滤移除：否则 tag push 指向的 commit 若 diff 不含目标目录会静默不触发，发版失败
- 后端不改 Dockerfile 走简单单阶段：与项目既有决策一致（commit 36e8dbe 回归精简），构建责任在 CI 外部
- npmmirror 改成构建参数而非硬编码：CI 走官方源避免海外 runner 访问国内镜像站失败
**待办**: GitHub 仓库 Settings → Actions → Workflow permissions 勾选 "Read and write permissions" 否则 GHCR 推送 403；设置分支保护强制 PR + CI 绿
**会话**: 260418-infra-CI完善前端部署

## [2026-04-18] 学习沙箱归档到 sandbox/

**变更内容**: `work/` 顶层散落的 8 个独立 Go 学习模块统一归档到 `sandbox/`，主目录只剩项目核心（webook / webook-fe / 基础设施 / docs）
**影响范围**:
- 目录归并（git mv 保留历史）：`context/` `gin/` `gorm/` `mongodb/` `opentelemetry/` `sarama/` `syntax/` `wire/` → `sandbox/<name>/`
- 文档同步：`docs/opentelemetry/` 共 7 处 `opentelemetry/` 路径引用 → `sandbox/opentelemetry/`
**技术决策**:
- 命名 `sandbox/` 而非 `learning/`：中性专业，不带学生味
- 扁平结构（不按主题分组）：沙箱本来就孤立，分类反而增加查找深度
- 各沙箱保留独立 go.mod，依赖树不互相污染
**待办**: 无
**会话**: 260418-refactor-沙箱归档

## [2026-04-18] 学习沙箱：context + opentelemetry trace

**变更内容**: 新增两个独立 Go 学习沙箱
**影响范围**:
- `sandbox/context/`（独立模块 `context-demo`，演示 WithValue/WithCancel/WithTimeout/父子传导/反向隔离，5 个测试）
- `sandbox/opentelemetry/`（独立模块 `otel-demo`，OTel SDK v1.32.0，stdout + Zipkin 双 exporter 测试）
**技术决策**: 独立 go.mod 与主模块隔离，依赖树不互相污染；与 `sandbox/mongodb/` `sandbox/sarama/` `sandbox/gin/` `sandbox/gorm/` `sandbox/wire/` `sandbox/syntax/` 等已有学习沙箱风格一致
**待办**: 无
**会话**: 260418-learning-context+OTel

## [2026-04-18] 前端 Docker 部署 + Nginx 反代 + 基础设施目录顶层化

**变更内容**: webook-fe 多阶段 Docker 镜像（standalone + 非 root + healthcheck + OCI labels）；Nginx 反向代理（同源部署 `/api` 剥前缀、SSE 关 buffering、登录强限流、静态资源长缓存、安全响应头）；docker-compose.yaml / nginx / prometheus / grafana 全部上移到 `work/` 顶层，`webook/` 目录回归"纯后端代码"
**影响范围**:
- 前端镜像：`webook-fe/Dockerfile`（多阶段 deps→builder→runner，standalone 输出，非 root nextjs:1001，HEALTHCHECK，OCI 标签）+ `webook-fe/.dockerignore` + `webook-fe/next.config.ts`（`output: 'standalone'` + `poweredByHeader: false` + `compress: true`）
- 反代：`nginx/nginx.conf`（JSON access log、gzip、limit_req zone）+ `nginx/conf.d/webook.conf`（upstream + 安全头 + SSE 特殊路径 + metrics 内网白名单 + Next 静态长缓存）
- 编排：`docker-compose.yaml` 加 `webook-fe`（仅 expose 3000）+ `nginx`（80 对外，不挂日志卷以保留 `/dev/stdout` 软链让 docker logs 接管）；启动命令注释更新
- 目录顶层化（git mv，保留历史）：`webook/docker-compose.yaml` → `docker-compose.yaml`、`webook/prometheus/` → `prometheus/`、`webook/grafana/` → `grafana/`
- 文档同步：`docs/grafana/` `docs/prometheus/` 共 4 处路径引用从 `webook/grafana` → `grafana` 等
**技术决策**:
- 同源部署：`NEXT_PUBLIC_API_BASE_URL=/api`，构建一次到处部署，无 CORS，外暴露端口仅 80
- standalone 输出：镜像最小化，仅装运行时 node_modules
- nginx 在容器网络内部通过 upstream `webook:8089` / `webook-fe:3000` 拼接，无需暴露后端端口（保留 8089 暂供调试）
- 顶层化：`webook/` 不再承担"基础设施 + 后端"双重职责，`nginx/prometheus/grafana` 是跨前后端基础设施，与 `webook/` 平级更清晰
**待办**: 后端去掉 8089 公网端口（仅走 nginx）；TLS（443）+ HSTS；webook 接 OTel；Grafana provisioning 切 `editable=false` 三件套
**会话**: 260418-deploy-前端Docker部署

## [2026-04-18] Grafana / OpenTelemetry 文档 + Zipkin 接入

**变更内容**: 新增 Grafana / OpenTelemetry 详细学习文档（共 13 篇），docker-compose 引入 Zipkin 服务（暂用内存存储），原 `docs/monitoring/` 重命名为 `docs/prometheus/` 与新文档对齐
**影响范围**:
- 文档：`docs/opentelemetry/`（概念/Tracing 模型/上手/Exporter 选型/接入 webook/最佳实践，6 篇）
- 文档：`docs/grafana/`（概念/部署/数据源/Dashboard/告警/生产级流程/最佳实践，7 篇）
- 文档重命名：`docs/monitoring/` → `docs/prometheus/`（git mv 保留历史，同步修复 grafana/otel 文档内引用）
- 基础设施：`docker-compose.yaml` 加 `zipkin`（openzipkin/zipkin:3.4，9411，IP 172.21.0.26，mem 存储 + ES 切换注释占位 + healthcheck）
**技术决策**:
- 文档目录三件套对齐（prometheus / grafana / opentelemetry），命名一致便于检索
- Zipkin 暂内存存储，重启丢 trace 数据；切 ES 路径以注释保留（复用 `webook-es`），单节点 ES 必须 `ES_INDEX_REPLICAS=0`
**待办**: webook 接入 OTel（otelgin/otelgorm/otelredis/otelsarama）；接入后切 OTLP exporter；按需切 Zipkin ES 存储
**会话**: 260418-observability-OTel起步

## [2026-04-17] GitHub 迁移 + CI 体系

**变更内容**: 仓库从 gitee 迁至 GitHub（`github.com/boyxs/train-go`），Go 模块路径改为 `github.com/webook`，落地 GitHub Actions CI（goimports + vet + test），handler 测试适配 ginx.WrapReq HTTP 状态码映射
**影响范围**:
- 模块路径：`webook/go.mod` + 132 个 .go 文件 import 全替换（`gitee.com/train-cloud/geektime-basic-go` → `github.com/webook`）
- Import 规范：`goimports -local github.com/webook` 重排 111 个文件，本地包分组到第三组
- Makefile：`webook/Makefile` 加 `fmt` / `fmt-check` target，MODULE 从 go.mod 动态提取
- CI：`.github/workflows/webook-ci.yml`（lint-test：goimports 检查 + go vet + go test -race），actions/checkout@v5、setup-go@v6（消除 Node 20 警告）
- Test 修复：`internal/web/` 5 个 test 函数的 wantCode 按 ginx.WrapReq 状态码映射校正（Code 4→400、Code 5→500）；`internal/service/sms/tencent/` TestSend 无凭证时改 `t.Skip()`
- Remote：origin → GitHub，原 gitee 重命名为 `gitee` remote 保留
**技术决策**:
- 模块路径采用 `github.com/webook` 而非 `github.com/boyxs/train-go/webook`：短路径更简洁，私有项目不需要外部 `go get`
- CI 不依赖 make，直接调 goimports 二进制：降低 runner 环境耦合
- 当前只有 lint-test job，build-push 留到 L1 完整流程时加（需配合 Dockerfile 多阶段）
**待办**: Dockerfile 改多阶段 + CI 加 build-push → GHCR；打开 GitHub 仓库分支保护（main 强制 PR + CI 绿）
**会话**: 260417-infra-GitHub迁移

## [2026-04-14] Prometheus 监控链路

**变更内容**: HTTP 指标中间件 + Prometheus/Grafana/Exporter Docker 栈 + 3 个自定义 Grafana 面板 + PromQL 文档
**影响范围**:
- 中间件：`pkg/ginx/middleware/metrics/`（Builder 接口 + PrometheusBuilder 实现，4 种指标按需启用）
- 集成：`ioc/web.go`（/metrics 端点 + 中间件注册）
- Docker：`docker-compose.yaml`（prometheus/grafana/mysqld-exporter/redis-exporter/kafka-exporter/node-exporter）
- 配置：`prometheus/prometheus.yml`、`prometheus/prometheus.local.yml`、`grafana/provisioning/`
- 面板：`webook-overview.json`（17 面板）、`webook-ops.json`（15 面板）、`linux-host.json`（18 面板）
- 文档：`docs/monitoring/`（架构/部署/PromQL/实战查询/告警/最佳实践，6 篇）
- 目录整理：`docs/` PRD 文件移至 `prd/`，`docs/` 改为技术文档目录
- 配置同步：`config/prod.yaml` 补齐 llm/embedding/ollama 配置
**技术决策**: 中间件用 Builder 接口抽象，Prometheus 为具体实现，后续可换 OpenTelemetry；Histogram + Summary 同时开启便于学习对比，生产建议二选一
**待办**: 生产环境 API Key 移到密钥管理
**会话**: 260414-monitoring-Prometheus监控

## [2026-04-12] AI 回复操作栏（复制 + 点赞/点踩）

**变更内容**: AI 消息气泡底部增加操作栏，支持复制回复内容、点赞/点踩反馈
**影响范围**:
- 后端：`domain/chat.go`（Message +Feedback）· `dao/chat_message.go`（+Feedback 字段 + UpdateFeedback）· `repository/chat_message.go`（UpdateFeedback + 清缓存）· `service/chat.go`（SetFeedback 归属校验）· `web/chat.go`（/chat/message/feedback 路由）
- 前端：`types/chat.ts`（+feedback）· `api/chat.ts`（setMessageFeedback）· `hooks/useChat.ts`（乐观更新 setFeedback）· `views/chat/MessageBubble.tsx`（ActionBar 组件）· `ChatMessages/ChatBubble/index.tsx`（props 传递）
- 原型：`prd/chat/chat.pen`（06-消息反馈画板）· `prd/chat/prototypes/06-消息反馈.png`
- 测试：8 个集成测试（点赞/取消/点踩/无效值/无效ID/非属主/幂等/列表展示）
**技术决策**: feedback 字段直接加在 message 表（1:1 关系），不新建表；前端乐观更新用函数式 setState 避免闭包竞态
**待办**: 无
**会话**: 260412-chat-消息反馈

## [2026-04-11] Chat SSE 断线续传（Redis Stream）

**变更内容**: 用 Redis Stream 实现 SSE 断线续传，刷新/切换对话后从断点继续流式输出
**影响范围**:
- 后端：`pkg/streamer/`（EventStreamer 接口 + Redis Stream 实现）· `service/chat.go`（publishToStream 写入 + ReadStream/BlockReadStream 读取 + isGenerating 内存标记）· `web/chat.go`（ResumeStream GET SSE 端点 + pollStream + formatSSE）· `consts/cache.go`（ChatStreamPattern + ChatStreamTTL）
- 前端：`api/chat.ts`（resumeStream SSE 重连函数）· `hooks/useChat.ts`（buffer 管理 + isStale 守卫 + resumeStream 恢复）
**技术决策**:
- Redis Stream 做消息中转，`XREAD BLOCK` 零空转阻塞读
- Stream 生成完成后 5 分钟 EXPIRE，供断线重连
- 前端 `isStale()` 守卫所有异步回调，防止快速切换对话时数据串
- 有活跃 buffer 时 effect 直接 return，避免与 send() 竞态
**会话**: 260411-SSE-Redis-Stream

## [2026-04-11] 文章润色助手 + 原型配色统一 + Chat 生成不中断 + 阅读量展示

**变更内容**: 文章润色助手（LLM 同步调用 + 对比 Modal）；原型配色全量统一为 teal #0D9488；Chat 生成独立 context 不被刷新/切换中断，支持刷新后轮询恢复；阅读页互动栏对齐原型（独立卡片 + 点赞收藏按钮样式）；文章广场/作者列表新增阅读量展示；首页按钮对齐原型；Header logo 统一主色；代码块语法高亮
**影响范围**:
- 后端：`service/article_polish.go`（润色服务）· `web/article_polish.go`（Handler + 限流）· `service/ai/llm.go`（LLMClient.Chat 同步方法）· `service/ai/openai.go` + `failover.go` + `timeout_failover.go`（Chat 实现）· `service/chat.go`（独立 context + placeholder + onFlush + isGenerating + trySend 非阻塞）· `web/chat.go`（isGenerating 接口）· `web/jwt/handler.go`（CheckSession Redis 容错）· `domain/chat.go`（ArticleCard.Url）· `chat_tools.go`（工具结果带 url）
- 前端：`views/article/read.tsx`（互动栏重构）· `views/article/feed.tsx`（阅读量）· `views/article/list.tsx`（阅读量列 + 移动端卡片重构）· `views/article/edit.tsx`（AI 润色按钮 + PolishModal）· `views/home.tsx`（按钮对齐原型）· `views/chat/index.tsx`（Header 导航重构）· `views/chat/MessageBubble.tsx`（工具卡片下移 + 语法高亮）· `hooks/useChat.ts`（buffer 缓存 + 轮询恢复）· `components/layout/Header.tsx`（logo 主色）
- 原型：3 个 pen 文件配色统一 + PNG 重新导出 + PRD 更新
**技术决策**:
- Chat 生成用 `context.WithTimeout(context.Background(), 2min)` 独立于 HTTP 请求
- placeholder 消息预插入 DB + onFlush 每 2 秒更新，支持刷新后轮询看到进度
- trySend 加 default 分支，channel 满时不阻塞（浏览器断开后生成继续）
- isGenerating 用内存 sync.Map，不依赖 Redis
- 切换对话用 bufferRef 缓存 pending 消息，切回来立即恢复
- CheckSession Redis 出错时容错放行，解决间歇性点赞状态丢失
**待办**:
- Chat SSE 断线续传：用 Redis Stream 替代轮询，支持 Last-Event-ID 断点续传（为微服务拆分做准备）
- P1 文章标签/分类体系 → 意图识别 + 路由分发
- P2 用户反馈（点赞/点踩单条回复）
**会话**: 260411-原型实现-润色-Chat优化

## [2026-04-10] AI 点击埋点 + 数据看板 + Chat 工具调用修复

**变更内容**: 新增 AI 文章点击埋点全链路（记录 + 看板 + 缓存），修复 Chat Function Calling SSE 事件嵌套层级、工具结果刷新丢失问题，代码块语法高亮，命名规范统一
**影响范围**:
- 后端：`web/click_event.go`（Handler）· `service/click_event.go` · `repository/click_event.go` · `dao/click_event.go` · `cache/click_event.go`（全链路新增）· `service/chat.go`（saveReply 持久化 toolResults、buildPrompt 用 ListRecentLite）· `service/ai/openai.go`（finish_reason 兼容）· `service/chat_tools.go` + `chat.go`（命名 AIChatService/AIChatToolExecutor）
- 前端：`api/chat.ts`（SSE data.data 修复）· `hooks/useChat.ts`（历史消息还原 toolStates）· `views/chat/MessageBubble.tsx`（语法高亮 + 链接埋点）· `components/chat/ArticleCardBlock.tsx`（卡片埋点）· `views/dashboard/ai.tsx`（看板页面）· `views/article/read.tsx`（返回按钮兼容新标签）
**技术决策**:
- 命名规范：接口通用名，实现用 `{技术}{业务}{领域}{层}` 组合前缀（如 `CacheAIClickEventRepository`）
- 去重用 `uk_dedup`（user+article+conversation+source），`ON CONFLICT DO NOTHING`
- 看板缓存 10min TTL + jitter，写入后清缓存
- `ListRecentLite` 排除 tool_calls 字段优化 buildPrompt 性能
- 工具结果序列化存入 message.tool_calls，前端加载历史时还原卡片
**待办**: Phase 6 意图识别 + 路由分发（FAQ / RAG / Tool）
**会话**: 260410-chat-埋点看板

## [2026-04-06] Chat Function Calling — 工具调用 + 热门文章精确排行

**变更内容**: Chat 接入 OpenAI Function Calling，实现三个工具（文章搜索、热门推荐、我的收藏）；热门文章改为按互动加权分（read×1 + like×3 + collect×5）真实排行；前端渲染工具执行状态和文章卡片
**影响范围**: `service/ai/llm.go` · `service/ai/openai.go`（streaming tool_call 拼接）· `service/chat.go`（runStream 多轮工具循环）· `service/chat_tools.go`（新增 ToolExecutor）· `repository/dao/interaction.go`（ListHotBizIds / ListCollectedBizIds）· `domain/chat.go`（ArticleCard / ToolResultData）· `wire.go` · `hooks/useChat.ts` · `components/chat/ArticleCardBlock.tsx` · `views/chat/MessageBubble.tsx`
**技术决策**: 工具调用循环最多 5 轮防无限循环；executor 为 nil 时降级（兼容无工具场景）；热门排行加权而非最新，`stream_options include_usage=true` 修复 token 用量归零问题；system prompt 规则 8 强制每次重新调用动态数据工具
**待办**: Phase 6 意图识别 + 路由分发（FAQ / RAG / Tool）

## [2026-04-06] Chat RAG — 基于文章内容的知识问答

**变更内容**: 聊天接入 RAG，用户提问时自动检索平台文章，将相关内容注入 prompt，AI 基于文章回答并附带可点击链接
**影响范围**: `service/chat.go`（RAG 逻辑）· `wire.go`（DI 注入 ArticleSearchService）· `views/chat/MessageBubble.tsx`（链接新标签页打开）· `views/article/read.tsx`（返回按钮兼容新标签页）· `.gitignore`（排除 .next 构建产物）
**技术决策**: 复用已有 ArticleSearchService（hybrid BM25 + 向量），top-3 召回注入 system message；直接提供完整 Markdown 链接防止 LLM 编造 ID；检索失败静默降级不阻塞对话
**待办**: 阶段 B Function Calling（search_articles / list_favorites / hot_articles）
**会话**: 260406-chat-RAG

## [2026-04-05] Embedding 分包 + 分页状态保持

**变更内容**: ai/embedding 拆为独立子包，缓存移到 cache 层；文章列表分页同步 URL，编辑返回不丢页码
**影响范围**: `service/ai/embedding/`（新子包）· `cache/embedding.go`（缓存归位）· `views/article/list.tsx` · `views/article/edit.tsx`
**技术决策**: 子包按能力拆分（LLM vs Embedding），接口用完整名 `EmbeddingClient` 避免歧义；分页用 URL searchParams 而非 state，支持刷新保留

## [2026-04-05] 本地 Ollama Embedding + 收费模型降级

**变更内容**: 向量化优先走本地 Ollama bge-m3，失败自动降级到阿里百炼收费 API
**影响范围**: `ai/ollama_embedding.go` · `ai/failover_embedding.go` · `ioc/es.go` · `config/`
**技术决策**: 复用 `EmbeddingClient` 接口 + `FailoverEmbeddingClient` 顺序尝试，外层 `CachedEmbeddingClient` 不变
**会话**: 260405-embedding-ollama-failover

## [2026-04-05] 搜索优化 + 时间 int64 统一 + 性能修复

**变更内容**: 搜索功能全链路优化、时间字段统一为 int64 毫秒时间戳、多项性能和缓存修复
**影响范围**: DAO/domain/repository/service/web 全层 + 前端 types/views + ES mapping + SQL schema
**技术决策**:
- 时间用 int64 而非 time.Time：消除时区歧义，全链路零转换，前端 dayjs 格式化
- 搜索用 script_score 替代顶层 kNN：避免 kNN OR 合并返回全部文档
- Embedding 加 Redis 缓存（装饰器模式）：相同查询不重复调 API
- Wire/Mock 禁止手动改生成文件，统一用命令生成
**待办**: FindByBizIds 批量缓存、ES kNN+RRF 混合搜索（需 ES 8.8+）
**会话**: 250404-搜索优化-时间统一

## [2026-04-04] 安全修复：Chat 越权删除消息 + 越权取消生成

**变更内容**: 修复 3 处安全 / 架构问题
**影响范围**: `dao/chat_conversation.go` · `service/chat.go` · `service/chat_test.go`
**技术决策**:
1. `Delete` 事务先删 Conversation（uid 校验 + RowsAffected）再删 Messages，防越权删他人消息
2. `cancel` map 改用 `uid:convId` 复合 key，防任意用户 `/chat/stop` 取消他人生成
3. `isNotFound` 改用 `repository.ErrRecordNotFound`，service 层不再直接依赖 GORM
**会话**: 260404-chat-security-fix

## [2026-04-04] AI 客服多 LLM 故障转移 + 项目治理

**变更内容**: 新增 Kimi LLM 提供方，实现 FailoverClient（轮询）+ TimeoutFailoverClient（粘性超时切换）双策略故障转移；前端 SSE 超时断开 + 业务错误即断；consts 按领域拆分；CLAUDE.md 精简为导航+规则
**影响范围**: `service/ai/`（新增 4 文件）、`service/chat.go`（forwardStream 防泄漏重写）、`config/types.go`、`ioc/chat.go`、`consts/`（拆 3 文件）、`api/chat.ts`、`hooks/useChat.ts`、`CLAUDE.md` × 3
**技术决策**: 参考 SMS failover 拆为两种独立策略（单一职责），不合并；超时计入故障但不轮询（网络差时备用也超时）；业务错误不计入 cnt（切 provider 也没用）；forwardStream 所有 channel 写入改为 select ctx.Done 防 goroutine 泄漏
**待办**: 集成测试 SSE 用例需要有效 API Key 才能跑通
**会话**: 240404-全栈-AI客服故障转移

## 2026-03-29

### 读者端首页分页缓存 + 代码质量修复

**变更内容**:
- 读者端分页首页缓存（方案 C：只缓存第一页，TTL 3min）
- DAO 层 Insert/Update/Upsert 自动补充 abstract（`ensureAbstract`）
- 互动模块：logger 注入替代 `zap.L()`、事务包裹、错误码补充
- 前端 `Modal.confirm` → `modal.confirm`（antd 规范）
- SQL 补充 abstract 列和数据
- mock.mk 修复 article mock 生成路径

**影响范围**: `repository/cache/article.go`、`repository/article_reader.go`、`dao/article_author.go`、`dao/article_reader.go`、`dao/interaction.go`、`repository/interaction.go`、`web/interaction.go`、`webook-fe/views/article/list.tsx`、`scripts/webook.sql`

### 点赞/收藏功能（第2期互动模块）

**变更内容**: 新增文章互动功能，包含阅读量上报、点赞、收藏，前后端全栈实现
**影响范围**:
- 后端：domain/interaction.go（新），dao/interaction.go（新，`interaction`+`user_interaction` 两张表），repository/interaction.go（新），service/interaction.go（新），web/interaction.go（新），ioc/web.go，wire.go，wire_gen.go，init_table.go
- 前端：types/interaction.ts（新），api/interaction.ts（新），views/article/read.tsx（更新），types/index.ts（更新）

**新增接口**:
- `POST /interaction/read` — 阅读上报（公开，无需登录）
- `POST /interaction/get` — 获取聚合计数+用户状态（公开，登录后有 liked/collected 状态）
- `POST /interaction/like` — 点赞/取消点赞（需登录）
- `POST /interaction/collect` — 收藏/取消收藏（需登录）

**技术决策**:
- 聚合计数独立表 `interaction`（article_id unique），避免污染 published_article
- 用户操作记录独立表 `user_interaction`（user_id+article_id 联合唯一索引）
- 计数用 `GREATEST(0, count + delta)` 防止下界越零
- 前端乐观更新：点赞/收藏立即更新本地状态，失败时显示错误提示
- 阅读上报在文章内容加载完成后触发，失败静默处理不影响阅读体验
- `/interaction/read` 和 `/interaction/get` 加入 JWT 白名单（允许匿名访问）

**会话**: 260329-like-collect

---

## 2026-03-26

### 文章发布/撤回功能

**变更内容**: 实现文章发布（制作库+线上库双表事务）和撤回（幂等，只对已发布文章改状态）功能
**影响范围**: domain/article.go, dao/article_author.go, dao/article_reader.go, repository/article_author.go, service/article.go, web/article.go, wire.go, integration/article_test.go
**技术决策**:
- 制作库/线上库用 author/reader 命名，发布事务合并到 ArticleAuthorDAO（作者的操作）
- 撤回采用幂等设计：已发布→Private，草稿/已撤回→不改状态，线上库 DELETE 幂等
- DAO 层不依赖 domain 包，状态常量只在 domain 定义一份，通过 Repository 传参
- ArticleReaderDAO 暂时只保留 PublishedArticle 模型，接口和实现待未来读取功能时添加
**待办**: Publish 接口缺少"发布不存在文章"测试用例
**会话**: 260326-article-publish

---

## 2026-03-25

### 学习笔记：第01-03周

生成专业学习笔记，覆盖 Go 基本语法、Gin/GORM 入门、Session/JWT 认证、Kubernetes 部署入门。按知识体系重新组织，补充底层原理、最佳实践、对比表格和面试要点。

**文件:** `notes/_sections/sec_01_03.md`（2400+ 行）

---

## 2026-03-08

### 微信 OAuth2 登录

微信扫码登录完整流程：AuthURL 生成授权链接 → 回调获取 code → 换取 access_token + openid → 查找/创建用户 → 签发双 Token

**文件:** `internal/web/wechat.go` / `internal/service/oauth2/wechat/service.go` / `internal/domain/wechat.go` / `ioc/wechat.go`

### JWT 双 Token 认证（长短 Token）

access token 30min + refresh token 7天，refresh 时 ssid 不变只重签 access token，logout 时 Redis 存 ssid 使会话失效

**文件:** `internal/web/jwt/handler.go` / `internal/web/jwt/types.go` / `internal/web/user.go`

### SMS 装饰器：权限控制 / 限流 / 故障转移

SMS 服务支持多种装饰器组合：JWT 权限校验(`sms/auth/`) / Redis 滑动窗口限流(`sms/ratelimit/`) / 轮询故障转移(`sms/failover/`) / 连续超时切换(`sms/failover/timeout_failover.go`)

### 集成测试框架

Wire 独立 setup + MySQL/Redis 容器化测试环境，覆盖用户注册/登录/微信登录等核心流程

**文件:** `internal/integration/` / `internal/integration/setup/`

## 2026-03-15

### Docker Compose 本地开发环境

从 K8s deployment 文件中提取 MySQL 8.0、Redis 7.0、etcd 3.5.17 三个基础设施服务，生成 `docker-compose.yaml`，方便本地 `docker compose up -d` 一键启动开发依赖

**文件:** `docker-compose.yaml`

### Logger 中间件

请求日志中间件，Builder 模式构建，记录 Path/Query/Method/ClientIP/UserAgent/ReqBody/ResBody/Status/Duration，ReqBody 和 ResBody 截断 2048 字节

**文件:** `internal/web/middleware/logger.go`

### Logger 配置模块

基于 viper 配置文件路径判断环境（dev → DevelopmentConfig，k8s → ProductionConfig），通过 `LoggerConfig` 中间结构体将 yaml 配置逐字段覆盖到 `zap.Config`，解决 `viper.UnmarshalKey` 无法反序列化 `AtomicLevel`/`EncoderConfig` 的问题

**文件:** `ioc/logger.go` / `config/dev.yaml` / `config/k8s.yaml`

### Docker Compose webook 服务

webook 服务加入 docker-compose，传递 `APP_ENV=config/k8s.yaml` 环境变量，Dockerfile 增加 COPY config 目录

**文件:** `docker-compose.yaml` / `Dockerfile`

### 全局日志替换

项目中所有 `log.Printf`/`log.Println`/`fmt.Printf` 调试打印替换为注入式 `logger.LoggerX`，通过构造函数注入而非全局变量，涉及 user handler、failover SMS、memory SMS、ratelimit builder、login middleware。补充 `logger.Int32`/`Uint64`/`Strings` 辅助函数

**文件:** `internal/web/user.go` / `internal/service/sms/failover/` / `internal/service/sms/memory/` / `pkg/ginx/middleware/ratelimit/builder.go` / `internal/web/middleware/login.go` / `pkg/logger/fields.go`

### 集成测试 viper 初始化

添加 `TestMain` 入口加载 `config/dev.yaml`，修复集成测试无法连接数据库的问题

**文件:** `internal/integration/init_test.go` / `internal/integration/setup/wire.go`

## 2026-03-17

### 时间类型改造：int64 → time.Time

DAO 层和 Domain 层的 `Birthday`/`CreatedAt`/`UpdatedAt` 从 `int64` 毫秒时间戳改为 `time.Time`，MySQL 存储从 `BIGINT` 改为 `DATETIME`，和 MongoDB BSON Date 行为一致

**策略:** 全链路 UTC 存储 + carbon 东八区查询

**配置:**
- DSN: `parseTime=true&loc=UTC` — MySQL 驱动按 UTC 解析 datetime
- `ioc/time.go`: `carbon.SetTimezone("Asia/Shanghai")` — carbon 全局东八区
- `main.go`: 启动时调用 `ioc.InitTimezone()`

**时区处理规则:**
- 存储: 前端传毫秒时间戳 → `time.UnixMilli(ts).UTC()` → 存入 MySQL datetime（UTC）
- 查询: `carbon.Now().StartOfDay().StdTime()` 自动按东八区算一天的开始/结束
- 展示: 前端 `dayjs()` 自动将 UTC 转本地时区显示

**踩坑:**
1. `time.Time` JSON 反序列化只认 RFC3339，前端传时间戳必须用 `int64` 接收
2. `dayjs(value, 'YYYY-MM-DD')` 带格式参数会丢时区信息导致日期少一天，应用 `dayjs(value)`
3. carbon 默认用 UTC，必须 `SetTimezone` 后 `StartOfDay()` 才是东八区零点

**文件:** `ioc/time.go` / `ioc/db.go` / `config/dev.yaml` / `config/prd.yaml` / `internal/repository/dao/user.go` / `internal/domain/user.go` / `internal/domain/article.go` / `internal/repository/user.go` / `internal/web/user.go`
