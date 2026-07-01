# 错误模型重构设计（Kratos 风格的两级错误标识）

> 状态：设计稿（待评审）｜作者：后端｜日期：2026-06-30
> 范围：`pkg/errs`、`pkg/ginx`、`pkg/grpcx`、各服务 `*/errs`、监控、前端 `Result`/错误处理

## 1. 背景与问题

当前错误模型「三码同源」（`webook/CLAUDE.md` 规则 #7）：

```
errs.Error.Code  ≡  ginx.Result.Code  ≡  HTTP status
```

实现现状（已核对源码）：

- `pkg/errs/error.go`：`Error{ Code int; Message string; Metadata map[string]string; cause error }`，`Code` 直接用 HTTP status。
- **`Is` 按 `Code + Message` 比对**（`error.go:61`）→ 注释明确要求「`Message` 必须全局唯一」。
- `ginx/result.go`：`Result{ Code, Msg, Data, Metadata }`，`WriteError` 写 `ctx.JSON(be.Code, …)`，HTTP status = `be.Code`。
- `GRPCStatus()`（`error.go:101`）只带 `Code+Message`，**跨 gRPC 丢 Metadata**（注释自承认）。

### 四个根本约束

1. **业务身份绑死在 HTTP 码上**。HTTP 码是传输层语义、空间小（常用十几个），一个码背所有同类业务错误：润色限流 / 登录限流 / 评论限流 / AI 调用限流**全是 429**，码层面不可区分。
2. **`Message` 同时承担「展示文案」和「身份键」两个职责**（`Is` 按 Code+Message）。后果：① 文案必须全局唯一；② **改展示文案 = 改身份 = 悄悄破坏 `errors.Is`**；③ 前端只能靠 `msg` 字符串猜业务类型（脆）。
3. **监控只能 `sum by (status)`**：看到「429 涨了」却不知是哪个业务在限流，下钻得翻日志。
4. **跨 gRPC 丢上下文**：`GRPCStatus` 不带 details，`Metadata`（如 `retry_after`/`limit`）过一次 gRPC 就没了，多服务架构下业务错误信息保真度差。

> 直接触发本设计的线上现象：润色 `429 {code:429, msg:"润色次数已达上限"}`，前端只能 `message.error(msg)`，想做「引导升级 + 倒计时」只能 `if(msg.includes(...))`；监控看不出是润色限流。

## 2. 目标 / 非目标

**目标**
- 把「业务身份」从 HTTP 码里解耦出来，用稳定的业务原因码 `reason` 表达。
- `Message` 回归纯展示，可自由改文案而不影响错误匹配 / 前端逻辑 / 监控。
- 前端按 `reason` 精确分支；监控按 `reason` 聚合 / 告警。
- 跨 gRPC 保真 `reason + metadata`。
- **增量可迁移**，不破坏现有调用与前端。

**非目标**
- 不全量迁移到 go-kratos 框架（与现有 gin / 自研 `ginx` / 自研 `grpcx` 冲突，成本不成比例）。只借鉴其错误模型。
- 不改 `Code` 仍 = HTTP status 的事实（代理 / 缓存 / 重试 / 前端 401 刷新拦截器都依赖它）。

## 3. 参考：go-kratos 错误模型

`kratos/errors.Error` 四字段，关键是把「分类」和「身份」拆开：

| 字段 | 角色 | 例子 |
|------|------|------|
| `code` (int) | HTTP/gRPC 状态码 = **粗分类** | `429` |
| `reason` (string) | 业务原因码 = **稳定、机器可读、细粒度身份** | `POLISH_RATE_LIMITED` |
| `message` (string) | 人类可读展示文案 | `润色次数已达上限` |
| `metadata` (map) | 附加上下文 | `{retry_after:"60"}` |

- proto 枚举 + `protoc-gen-go-errors` 生成 `IsXxx(err)` / `errors.Reason(err)`。
- HTTP 出口 → `{code, reason, message, metadata}`；gRPC 出口 → `Status.details`（`errdetails.ErrorInfo`）。
- 精髓一句话：**`code` 给传输层和粗筛，`reason` 给业务和监控。**

## 4. 设计总览

引入 **`Reason`（业务原因码）** 作为第二级标识，形成「`code`（HTTP 粗分类）+ `reason`（业务身份）+ `message`（展示）+ `metadata`（上下文）」四元组，对齐 Kratos，但用本项目既有的手写 sentinel + builder 风格落地（不引 proto 代码生成）。

```
┌── code      = HTTP status   →  传输层、代理、前端拦截器、粗筛
├── reason    = 业务原因码      →  errors.Is 身份、前端分支、监控 label   ← 新增核心
├── message   = 展示文案        →  仅给人看，可随意改
└── metadata  = 上下文键值       →  retry_after / limit / field / resourceId …
```

## 5. 详细设计

### 5.1 `pkg/errs.Error` 扩展

```go
type Error struct {
    Code     int               // HTTP status，粗分类（不变）
    Reason   string            // 业务原因码（SCREAMING_SNAKE，全局唯一）—— 新增
    Message  string            // 人类可读展示文案（不再要求唯一）
    Metadata map[string]string
    cause    error
}

// 新增 builder（与现有 WithCause / WithMetadata 同风格，零破坏）
func (e *Error) WithReason(reason string) *Error { cp := *e; cp.Reason = reason; return &cp }

// Is：优先按 Reason 比对（稳定身份）；双方都无 Reason 时回退 Code+Message（兼容旧 sentinel）
func (e *Error) Is(target error) bool {
    var t *Error
    if !errors.As(target, &t) { return false }
    if e.Reason != "" && t.Reason != "" { return e.Reason == t.Reason }
    return e.Code == t.Code && e.Message == t.Message   // 迁移期回退
}
```

**为何用 builder 而非改 `New(code, reason, message)` 三参**：`errs.New(code,msg)` 调用点遍布 internal/errs、chat/errs、interaction、各 gRPC server 校验……三参签名是一次性破坏所有调用。builder `.WithReason(...)` 让 sentinel 增量加 reason、抛出用法（临时校验错误）保持现状，`Is` 的回退分支保证迁移期混用不出错。

sentinel 写法：

```go
// internal/errs/article.go
var ErrPolishRateLimited = errs.New(429, "润色次数已达上限").
    WithReason("POLISH_RATE_LIMITED")
```

### 5.2 Reason 命名规范 + registry

- 格式：`SCREAMING_SNAKE_CASE`，建议 `<域>_<语义>`，如 `POLISH_RATE_LIMITED`、`ARTICLE_NOT_FOUND`、`USER_NOT_LOGIN`、`COMMENT_CONTENT_SENSITIVE`。
- **各服务自治定义 + 强约束（决策）**：reason 由各服务在自己的 `*/errs/` 下随本域 sentinel 定义（不强行收口到单一文件），但用**自动化强约束**兜底——加一个 `go test`/lint 扫全仓所有 `*errs.Error` sentinel 的 reason：① 重复 reason → fail；② 不符 `SCREAMING_SNAKE`/域前缀规范 → fail；③ 空 reason 允许但计数并告警（提示待补）。比纯人肉 review 硬，又不牺牲各服务自治。
- 空 `reason` 合法（表示「未归类 / 临时校验错误」），监控侧归到 `_UNSPECIFIED` 桶。

### 5.3 `ginx.Result` + `WriteError`

`Result` 加 `Reason`（`Metadata` 已存在）：

```go
type Result struct {
    Code     int               `json:"code"`              // = HTTP status（不变）
    Reason   string            `json:"reason,omitempty"`  // 业务原因码 —— 新增
    Msg      string            `json:"msg"`
    Data     any               `json:"data"`
    Metadata map[string]string `json:"metadata,omitempty"`
}
```

`WriteError`（`wrapper.go:33`）业务错误分支带上 reason：

```go
ctx.JSON(be.Code, Result{Code: be.Code, Reason: be.Reason, Msg: be.Message, Metadata: be.Metadata})
```

成功路径、系统错误（500）不变。handler 仍 `return Result{Data}, err`，无感知。

### 5.4 跨 gRPC 保真（修 Metadata 丢失 + 带 reason）

当前 `GRPCStatus()` 只 `status.New(code, message)`。改为携带 `errdetails.ErrorInfo`：

```go
func (e *Error) GRPCStatus() *status.Status {
    st := status.New(httpToGRPC(e.Code), e.Message)
    if e.Reason != "" || len(e.Metadata) > 0 {
        if d, err := st.WithDetails(&errdetails.ErrorInfo{
            Reason: e.Reason, Metadata: e.Metadata,
        }); err == nil { st = d }
    }
    return st
}
```

`FromError` 反向：从 status details 里抓 `ErrorInfo` 还原 `Reason + Metadata`。这样 `grpcx` errconv 拦截器两端转换后，业务身份 + 上下文跨服务不丢（worker→interaction、core→interaction、chat→interaction 都受益）。

> 依赖：`google.golang.org/genproto/googleapis/rpc/errdetails`（grpc 生态标准件，已在依赖树）。

### 5.5 监控：按 reason 聚合（决策：给现有 `webook_http_requests_total` 加 `reason` label）

- **给现有 `webook_http_requests_total` 加 `reason` label**（不新增指标）：成功路径 `reason=""`，错误路径 = 业务原因码。中间件/拦截器写错误响应时 reason 已知，打点时带上即可。
- **`reason` 是有限枚举 → 低基数，可安全做 label**；**`message` 高基数，绝不可做 label**（写进 CLAUDE.md 规则）。
- **基数提醒**：该指标已含 `method/pattern/status`，再加 `reason` 会乘**错误路径**的组合数（成功路径 reason="" 不放大）；reason 有限、单个 `(pattern,status)` 错误通常只对应少数 reason，增量可控。若日后基数告急，再拆独立 `webook_biz_errors_total{reason}`（接口预留）。
- Grafana：`sum by (reason) (rate(webook_http_requests_total{reason!=""}[5m]))`，告警精确到 `reason="POLISH_RATE_LIMITED"`。

### 5.6 前端

```ts
// 统一响应类型加 reason
interface ApiResult<T> { code: number; reason?: string; msg: string; data: T; metadata?: Record<string,string> }

// apiErrMsg 升级：reason 给逻辑分支，msg 给展示
export function apiReason(err: unknown): string | undefined {
  return (err as { response?: { data?: { reason?: string } } })?.response?.data?.reason;
}
```

需要精确处理的 handler 按 `reason` 分支（稳定），通用展示仍用 `msg`：

```ts
catch (e) {
  if (apiReason(e) === 'POLISH_RATE_LIMITED') {
    // 引导升级 + 用 metadata.retry_after 做倒计时
  } else {
    message.error(apiErrMsg(e, '润色失败'));
  }
}
```

> 注：前端「在哪 catch / 是否中心化自动弹」是**另一个独立议题**（见附录 A），本设计只负责把 `reason` 暴露出来，让前端有稳定的分支依据。

### 5.7 Grafana

- `prd`/部署侧把业务错误面板 / 告警从 `by (status)` 升级到 `by (reason)`。
- 新增「业务错误 Top（按 reason）」面板 + 关键 reason 的专项告警。

## 6. Reason 初始枚举（示例，随迁移补全）

| 域 | reason | code | message（示例） |
|----|--------|------|----------------|
| user | `USER_NOT_LOGIN` | 401 | 请先登录 |
| user | `USER_PASSWORD_INVALID` | 401 | 账号或密码错误 |
| article | `ARTICLE_NOT_FOUND` | 404 | 文章不存在 |
| article | `POLISH_RATE_LIMITED` | 429 | 润色次数已达上限 |
| comment | `COMMENT_CONTENT_SENSITIVE` | 422 | 内容包含敏感词 |
| comment | `COMMENT_RATE_LIMITED` | 429 | 操作太频繁 |
| interaction | `INTERACTION_BIZ_INVALID` | 400 | biz 不合法 |
| common | `INVALID_ARGUMENT` | 400 | 参数错误 |
| common | `RATE_LIMITED` | 429 | 操作太频繁 |

## 7. 迁移次序（增量、各步独立可上线）

> **进度（2026-06-30）**：步骤 **1-5 已 TDD 实现并验证**（`pkg/errs` Reason/WithReason/Is、gRPC errdetails 往返、`pkg/ginx` Result+WriteError、http metrics reason label、7 个 `*/errs` 共 50 个 reason + 唯一性 go test、前端 `types/common.ts`+`utils/apiError.ts`）。**步骤 6（收紧 Is 去回退）暂缓**——依赖全量 reason 覆盖；另有约 6 处散落 inline sentinel（`internal/web/interaction.go`、`migrator/web·service`）仍 reason-less，靠 Code+Message 回退安全运行，后续补。

1. **基建**：`errs.Error` 加 `Reason` + `WithReason` + `Is` 改优先 Reason；`ginx.Result` 加 `Reason`、`WriteError` 带出。`reason` 暂为空 → 行为完全不变（纯加字段，向后兼容）。
2. **跨 gRPC 保真**：`GRPCStatus`/`FromError` 带 `errdetails.ErrorInfo`。
3. **逐域补 reason**：从高价值域开始（润色限流、登录、限流类），给 sentinel 加 `.WithReason(...)`。
4. **监控**：加 `webook_biz_errors_total{reason}` + Grafana 面板/告警。
5. **前端**：`Result` 类型加 `reason`；对需要精确处理的场景（限流引导、登录跳转）按 reason 分支。
6. **收尾**：把 `Is` 的 Code+Message 回退分支标记 deprecated（待所有 sentinel 都有 reason 后移除），强约束「sentinel 必须带 reason」。

## 8. 兼容性

- `Code` 仍 = HTTP status：老前端 / 代理 / 重试 / 401 刷新拦截器**零影响**。
- `reason` 是**新增 omitempty 字段**：旧客户端忽略即可。
- `Is` 回退分支：迁移期「有 reason 的新 sentinel」与「无 reason 的旧 sentinel」混用都能正确匹配。
- handler 签名、`ginx.Wrap` 用法不变。

## 9. 风险与取舍

| 风险 | 应对 |
|------|------|
| `reason` 命名散乱 / 不唯一 | 各服务自治定义 + `go test` 强约束（扫全仓重复 reason + 命名规范，违者 CI fail） |
| 监控 label 基数 | 只允许 `reason`（有限枚举）做 label，**禁止 `msg`**；写进 CLAUDE.md 规则 |
| `Is` 语义变化 | 优先 Reason + 回退 Code+Message，迁移期双轨；全量后再收紧 |
| 跨 gRPC details 兼容 | `WithDetails` 失败时降级为仅 Code+Message（不 panic）；老服务端不带 details，新客户端读不到 reason 时回退现状 |
| 不上全套 Kratos 的「半套」感 | 明确只借模型不借框架；本项目 sentinel 是手写常量，proto 生成收益不抵迁移成本 |

## 10. 已定决策（评审拍板 2026-06-30）

1. **reason 定义位置**：各服务 `*/errs/` **自治定义**（不收口单一文件），用 `go test`/lint **强约束**全仓 reason 唯一 + 命名规范（违者 CI fail）。
2. **监控指标**：给现有 `webook_http_requests_total` **加 `reason` label**（不新增指标）；成功路径 reason=""，仅错误路径带值；基数告急再拆独立 `webook_biz_errors_total`。
3. **HTTP / gRPC reason 一致**：**共用同一套 reason 字符串枚举**——gRPC 走 `errdetails.ErrorInfo.Reason`，HTTP 走 body `reason`，同字符串。

---

## 附录 A：与「前端错误处理中心化」议题的关系

前端是否在 axios 拦截器中心化自动弹 `message.error`（vs 每 handler 一处 catch）是**独立议题**，本设计不决定它。但本设计让那个议题更好做：无论中心化与否，前端都能拿到稳定的 `reason` 做分支（如 `USER_NOT_LOGIN` → 跳登录而非弹 toast，`*_RATE_LIMITED` → 弹引导）。建议待本错误模型落地后再回头定前端策略。
