# ADR — Architecture Decision Records

> 记录关键架构决策的"为什么"。一份 ADR 一个决策，不可变更：被新 ADR 取代时改状态为"被 NNNN 取代"，不删不改原内容。

## 索引

| # | 决策 | 状态 |
|---|------|------|
| [0001](./0001-redis-as-routing-truth.md) | Redis 作为路由决策的唯一真相源 | 接受 |
| [0002](./0002-source-factory-three-methods.md) | SourceFactory 按"读取语义"拆三方法 | 接受 |

## 写新 ADR

```
adr/NNNN-<decision-title>.md
```

四位数字递增（0001 → 0002 → ...）。

必备章节：背景 / 备选方案（≥2 个）/ 选择 / 理由 / 后果 / 状态。

**硬规则**：
- 理由必须给量化或可验证依据，不写"感觉更好"
- 后果必须写负面影响（没有缺点的选择不存在）
- 一份 ADR 一个决策，不打包
