# Runbook: 对账 mismatch 飙升

> 触发：`verify` 结果 mismatch_rate > 0.01% / `GET /tasks/:id/mismatch` 行数飙升
> 严重度：**P2**（cutover 前发现是 P1）
> 预期解决时间：**< 2 h**
> 关联：`zero-downtime-playbook.md` §6 / §15.1 风险

---

## 症状

- `verify mode=sample/full` 返回 mismatchCount 高 / `GET /tasks/:id/mismatch` 行数飙升
- `validate_log` 表新增行飙升（mysql 查）
- 切流前的硬门槛失败：cutover 申请被系统拒绝

## 立即动作（5 分钟内）

1. **确认是哪类 mismatch（missing / extra / diff）**

   ```bash
   mysql webook_migrator -e "
     SELECT mismatch_kind, COUNT(*)
     FROM validate_log
     WHERE task_id=$TASK_ID AND created_at >= UNIX_TIMESTAMP(NOW() - INTERVAL 1 HOUR) * 1000
     GROUP BY mismatch_kind"
   ```

2. **如果当前在切流过程中（gray > 0），立即 gray=0**

3. **冻结 cutover**：禁止任何切流操作（`{stage:DST_ONLY, action:propose}` / `approve`）

## 诊断（30 分钟内）

按 mismatch_kind 分类排查：

### `missing`（目标缺）

```bash
# 抽样几条 missing 看现象
mysql webook_migrator -e "
  SELECT biz_id FROM validate_log
  WHERE task_id=$TASK_ID AND mismatch_kind='missing' LIMIT 5"

# 对应 ID 在源 / 目标各查一次
mysql -h source -e "SELECT id, updated_at FROM article WHERE id IN (...)"
# Sink 侧查询（ES / CK / MySQL_NEW）
```

可能原因：
- Sink 写入失败但没进死信（漏告警）
- Transform 把这些行 filter 掉了
- 全量 scan 把这些行 skip 了

### `extra`（目标多）

```bash
# 看 extra 的 ID 是否在源已被软删
mysql -h source -e "SELECT id, deleted_at FROM article WHERE id IN (...)"
```

可能原因：
- 源软删，sink 没同步删除（CDC 跳过 delete event）

### `diff`（字段不一致）

```bash
mysql webook_migrator -e "
  SELECT biz_id, diff_detail FROM validate_log
  WHERE task_id=$TASK_ID AND mismatch_kind='diff' LIMIT 5"
```

可能原因：
- 旧 binlog 覆盖新值（Sink 乐观锁失效）
- Transform 字段映射写错（如时区转换）
- 字段类型不一致（VARCHAR(50) → VARCHAR(255) 截断）

## 永久修复

| 根因 | 处理 |
|------|------|
| Sink 漏写 missing | 修死信告警；走 repair 补回 |
| Sink delete 漏处理 | 修 Sink 实现，处理 op=delete；走 repair 删除 extra |
| 旧 binlog 覆盖（diff） | 强制 Sink 加乐观锁条件；review 实现；repair 用 src_overwrite_dst |
| Transform 错 | 修 transform 函数；下线 sink 重做全量 |
| 字段类型不兼容 | 修 schema；走 schema 演进流程 |

## 修复操作

```bash
# 自动修复（src 覆盖 dst）
curl -X POST http://migrator.internal:8083/migrator/tasks/$TASK_ID/repair \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -d '{
    "strategy": "src_overwrite_dst",
    "ids": [99887, 99892, 99895]
  }'

# repair 完成后重新跑 verify
curl -X POST .../tasks/$TASK_ID/verify -d '{"mode":"sample","sampleRate":0.01}'
```

## 验证

- mismatch_rate < 0.001% 持续 1h
- 抽样 `validate_log WHERE repaired=0` 行数 < 100

## 事后

- [ ] 自动 repair 阈值评估（多少差异自动修，多少需要人工）
- [ ] 对账采样率提升（0.01% → 0.1%）
- [ ] postmortem，归因到具体 transform / sink 实现 bug
