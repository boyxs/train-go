# 功能完成检查

> **铁律**: 没有跑通验证，不声称完成。
> 不接受"应该没问题"、"看起来可以"。只接受验证命令的实际输出。

## 验证关卡（按顺序执行，不可跳过）

### Gate 1: 编译通过
```bash
go build ./...
```
贴出完整输出。有任何错误 → 停止，先修复。

### Gate 2: 静态分析通过
```bash
go vet ./...
```
贴出完整输出。有任何警告 → 停止，先修复。

### Gate 3: 测试通过
```bash
go test ./...
```
贴出完整输出。有任何失败 → 停止，先修复。

### Gate 4: 代码检查清单

- [ ] 无 `fmt.Println`（用 `zap` 日志）
- [ ] 无测试假数据残留（或已用 `// ===== TODO: 测试假数据 START/END =====` 包裹）
- [ ] 方法命名语义明确（`FindXx` / `PageXxs` / `ListXxs` / `CreateXx` / `UpdateXx` / `DeleteXx`）
- [ ] error 全部处理，无 `_ = err`
- [ ] 严格分层：Handler → Service → Repository → DAO/Cache，无跨层调用
- [ ] 新增接口有 JWT 认证（除非明确是公开接口）

### Gate 5: 功能验证
- [ ] 核心路径正常工作
- [ ] 空数据 / nil / 零值场景处理
- [ ] 并发场景（Redis 操作、缓存一致性）
- [ ] 大数据量场景无性能问题

### Gate 6: 文档更新
- [ ] 追加 `DEVLOG.md` 记录（含会话名）
- [ ] API 接口注释已更新（如有新增/修改接口）
- [ ] domain 模型注释已更新（如有数据结构变更）
- [ ] `memory/` 已记录本次踩的坑（如果有）

## 提交信息
生成 commit message，格式：`type(scope): description`
- type: feat / fix / refactor / docs / chore / perf / test
- scope: web / service / repository / dao / cache / ioc / config / pkg / integration

## 完成报告
```
### 变更摘要
一句话描述

### 变更文件
文件列表 + 每个文件改了什么

### 验证结果
- go build: ✅/❌
- go vet: ✅/❌
- go test: ✅/❌ (X passed, Y failed)

### 遗留问题
后续需要跟进的事项（如果有）
```

## 红旗自检
如果你正在想以下任何一条，**停下来跑验证**：
- "应该没问题" → 跑 `go test` 才知道有没有问题
- "刚才跑过了" → "刚才"不算，重新跑
- "只改了一点点" → 一点点也要验证
- "编译通过就行了" → 编译通过 ≠ 逻辑正确
