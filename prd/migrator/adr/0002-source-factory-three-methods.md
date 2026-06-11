# ADR 0002:SourceFactory 按"读取语义"拆为三方法

| | |
|---|---|
| 状态 | 接受（2026-06-06 接口隔离深化，见末尾「演进」段） |
| 日期 | 2026-05-25 |
| 决策者 | webook-migrator review |
| 取代 | 无 |

## 背景

首版 `SourceFactory` 接口签名：

```go
type SourceFactory interface {
    BuildSrc(ctx, task, tableIdx) (Source, error)
    BuildDst(ctx, task, tableIdx) (Source, error)
}
```

`BuildSrc` 内部按 `task.Mode == cdc` 强制返 `CanalSource`。

**Bug 现场**：cdc task 启动 `phase=full`(全量同步)时,FullEngine.Run 调 `BuildSrc` 拿到 CanalSource → CanalSource.FullScan 直接返 `ErrFullScanNotSupported` → article_v1 永远空 → README Step 8 对账数据完全对不上。

根因：`task.Mode` 描述的是"迁移机制"(dual_write / cdc),`BuildSrc` 却用它当"该用 MySQLSource 还是 CanalSource"的开关，**混淆了"机制"和"读取场景"两个维度**。

## 备选方案

### 方案 A:`BuildSrc` 内按 phase 分发(不改接口)

引擎调用时传入 phase 参数:`BuildSrc(ctx, task, tableIdx, phase)`。phase=full → MySQLSource,phase=incr → 按 task.Mode 决定。

- 优点:接口面变化最小
- 缺点:phase 参数在 SinkFactory 没有意义(Sink 不分全量/增量);接口非对称丑

### 方案 B:接口拆成三方法(采用)

```go
type SourceFactory interface {
    BuildFullSrc(ctx, task, tableIdx) (Source, error)  // 全量永远 MySQLSource
    BuildIncrSrc(ctx, task, tableIdx) (Source, error)  // 按 task.Mode 决定
    BuildDst(ctx, task, tableIdx) (Source, error)      // 对账读 dst,按 task.SinkType 决定
}
```

各方法语义明确,调用方按场景选。FullEngine 调 `BuildFullSrc`,IncrEngine 调 `BuildIncrSrc`,VerifyEngine 用 `BuildFullSrc + BuildDst`。

### 方案 C:用两个独立工厂

`FullSourceFactory` / `IncrSourceFactory` 各自接口。

- 优点:语义最清
- 缺点:wire 装两套工厂,ioc 复杂;两套工厂内 PK/table 解析逻辑重复

## 选择

**方案 B**。

## 理由

1. **bug 修复彻底**:cdc task 的全量阶段不会再误走 CanalSource(实测 README Step 6 同步 3 行 + Step 8 mismatchCount=0 通过)
2. **接口扩展自然**:`BuildDst` 后续按 `task.SinkType` 分发 MySQL/ES,跟 BuildFullSrc/BuildIncrSrc 的"按场景分发"是同一种模式
3. **测试覆盖直接**:每个方法有独立返值,测试用例无需考虑 phase 参数耦合
4. **代码量小**:相比方案 C,只多 1 个方法签名,不引入第二个工厂

## 后果

### 正面

- ✅ cdc 任务全量阶段不再失败(P0 bug 修复)
- ✅ 异构对账(ESSource)的接入点天然清晰:`BuildDst` 按 SinkType 分发
- ✅ FullEngine / IncrEngine / VerifyEngine 三处调用点意图明确,代码读起来不需要推断 task.Mode

### 负面

- ⚠️ **接口面变大 50%**(2 方法 → 3 方法),所有 SourceFactory 测试 stub 多写一个方法的覆盖
- ⚠️ **跟 SinkFactory 不对称**:SinkFactory 仍是 `BuildSrc / BuildDst` 二元,如果以后 Sink 也按场景区分,要平等拆三方法
- ⚠️ **migration cost**:已有调用方 13 处需要按场景挑对应方法,review 时要逐个核对

## 实施

- `webook/migrator/pipeline/source/factory.go` 接口拆分 + 实现
- `service/full/full.go` 调 `BuildFullSrc`
- `service/incr/incr.go` 调 `BuildIncrSrc`
- `service/verify/verify.go` src 侧调 `BuildFullSrc`,dst 侧调 `BuildDst`
- `web/task.go resolveShards` 用 PKRange 时调 `BuildFullSrc`

## 演进（2026-06-06：接口隔离）

方案 B 的三方法判断是对的（按读取场景分发），但当时三方法都返回同一个**胖 `Source` 接口**（`FullScan + IncrSubscribe + SaveCheckpoint + Close`），导致每个实现只实现一半、另一半返 `ErrXxxNotSupported`（MySQLSource/ESSource/MongoSource 不支持 IncrSubscribe；CanalSource 不支持 FullScan）——这正是本 ADR 想修的"读取场景混淆"在**接口层**的残留。负面后果里那条「接口面变大、stub 多写方法」也源于此。

**深化**：把 `Source` 按读取语义拆成两个小接口（接口隔离 ISP）：

```go
type FullSource interface { FullScan(...); Close() }       // MySQLSource / ESSource / MongoSource
type IncrSource interface { IncrSubscribe(...); Close() }  // CanalSource / MongoIncrSource
```

三方法返回精确类型：`BuildFullSrc→FullSource` / `BuildIncrSrc→IncrSource` / `BuildDst→FullSource`。`SaveCheckpoint`（死方法，引擎用 CheckpointDAO 持久化）与未使用的 `SrcSource/DstSource` named type 一并删除。

结果：5 个实现各只实现自己能做的，**0 个 NotSupported**；调用方拿到 `FullSource` 就能 `FullScan`，无需试探。`BuildIncrSrc` 的 mysql 分支不再回退 MySQLSource（它已不是 `IncrSource`），mysql 增量必须 cdc+canal，否则**构造期**报错（原来是运行时 `ErrIncrNotSupported`）。

全 webook build/vet/单元测试零回归。

## 关联

- `02-architecture.md` "📌 v1 实现摘要" 段
- CHANGELOG `[2026-05-27]` 条目（v1 第一版总述）
