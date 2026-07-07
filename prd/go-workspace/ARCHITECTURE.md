# Webook 多模块化 + go.work 架构方案

> 状态：**待实施**（方案已确认，决策见 §9）
> 目标：把 `webook/`（仓库 `github.com/boyxs/train-go` 的子目录）从单模块拆成「每个服务 + `pkg`/`api`/`shared` 各自独立 `go.mod`」的多模块，用 `webook/go.work` 统一本地/CI 跨模块解析；并顺带把历史占位模块名 `github.com/webook` 对齐真实仓库路径 `github.com/boyxs/train-go/webook`。

## 1. 背景与目标

**现状**：`webook/` 是单个 Go module（go 1.25.6），一份 `go.mod`（9.4KB）+ `go.sum`（63KB）承载全仓所有依赖。7 个服务（`internal`(core)、`chat`、`comment`、`interaction`、`migrator`、`relation`、`worker`）+ 3 个共享层（`pkg`、`api`、`shared`）共用同一依赖图。

**两个待解决问题：**

1. **单模块耦合**：任一服务改依赖 → 全仓 `go.sum` 变动；服务间依赖边界靠约定、编译器不拦；CI/Docker 每次解析全仓依赖，缓存粒度粗。
2. **模块名与仓库不符**：当前模块路径 `github.com/webook` 是历史占位名，真实仓库是 `github.com/boyxs/train-go`（`webook/` 是其子目录）。纯本地开发靠 go.work+replace 能跑，但 `go get`、发布、新人认知都会被误导。

**目标**：

- 每个服务和共享层独立成 module，各自 `go.mod` 只列自己真正用到的依赖；
- 用 `go.work` 让本地/CI 无需发布即可解析兄弟模块；用 `replace` 让 `go mod tidy`/Docker/CI 全部离线确定性解析；
- 模块路径前缀对齐真实仓库：`github.com/webook` → **`github.com/boyxs/train-go/webook`**。

**非目标**（本次不做）：不改任何业务逻辑；`internal/` 保持原名不改成 `core`；不发布到远程 registry（纯本地 monorepo，靠 go.work + replace）；不拆分 git 仓库。

## 2. 侦察结论（拆分可行性依据）

全仓 `grep` 得到的**跨目录依赖图异常干净**，无环，拆分风险低：

| 层 | 目录 | 依赖 webook 内部 |
|---|---|---|
| 叶子 | `api`、`shared` | **无**（仅外部包） |
| 中间 | `pkg` | 仅自己的子包 |
| 服务 | `internal`(core)、`chat`、`comment`、`interaction`、`relation`、`worker` | `pkg` + `api` + `shared` |
| 服务 | `migrator` | `pkg` + `shared`（**不用 `api`**） |

**关键事实：**

1. **服务间零耦合**——唯一跨服务 import 是 `worker/consumer/event/contract_test.go` → `.../internal/events/interaction`（契约测试，验证 worker 消费的事件结构与 core 生产端一致）。`internal`(core) 不 import 任何兄弟服务。
2. **根模块可彻底解散**——`webook/` 根目录无任何 `.go` 文件，全仓无人 import 裸模块路径。拆完后 `webook/go.mod` + `go.sum` 直接删除。
3. **`internal/` 保持原名零改写**——给 `internal/` 一个 `go.mod`（`github.com/boyxs/train-go/webook/internal`），所有 `.../internal/...` 引用路径不变。Go 的 internal 可见性规则基于**路径前缀**（`.../internal/...` 只允许 `github.com/boyxs/train-go/webook/` 前缀下的包 import），所有兄弟模块都在这个前缀下，故 worker 契约测试仍合法。
4. **前缀迁移规模**：369 个 `.go` 文件含 `github.com/webook`；6 个 `.proto` 的 `option go_package` 含该前缀（需改+重生成）；另有 `api/Makefile`、根 `go.mod`、`mk/mock.mk`、7 个 CI workflow 的 `MODULE:` env。纯机械前缀替换，`go build` 编译器兜底，风险低但 diff 大。

## 3. 目标模块拓扑

拆成 **10 个 module**，模块路径一律 `github.com/boyxs/train-go/webook/<目录名>`：

```
webook/                              (= github.com/boyxs/train-go/webook 子目录)
├── go.work            # ← 新增：workspace，纳入下列 10 个模块
├── go.work.sum        # ← 新增：go work sync 生成
├── api/      go.mod → github.com/boyxs/train-go/webook/api        (叶子)
├── shared/   go.mod → github.com/boyxs/train-go/webook/shared     (叶子)
├── pkg/      go.mod → github.com/boyxs/train-go/webook/pkg        (叶子)
├── internal/ go.mod → github.com/boyxs/train-go/webook/internal   (core 服务)
├── chat/     go.mod → github.com/boyxs/train-go/webook/chat
├── comment/  go.mod → github.com/boyxs/train-go/webook/comment
├── interaction/ go.mod → github.com/boyxs/train-go/webook/interaction
├── migrator/ go.mod → github.com/boyxs/train-go/webook/migrator
├── relation/ go.mod → github.com/boyxs/train-go/webook/relation
├── worker/   go.mod → github.com/boyxs/train-go/webook/worker
├── Makefile           # ← 改：不再从 go.mod 取 MODULE
├── mk/*.mk            # ← 改：wire/mockgen/tidy 路径 per-module
└── tools/ scripts/    # 无 .go，无需模块
```

依赖方向（全部单向，无环）：

```
        ┌─────────┐   ┌────────┐
        │  api    │   │ shared │      ← 叶子，只依赖外部包
        └────┬────┘   └───┬────┘
             │  ┌────────┐ │
             │  │  pkg   │ │          ← 只依赖自己子包
             │  └───┬────┘ │
   ┌─────────┴──────┴──────┴─────────────────┐
   ▼         ▼         ▼        ▼      ▼      ▼
internal  chat  comment interaction relation worker
                                              │(仅契约测试)
                                              ▼
                                          internal
migrator ──► pkg + shared （不依赖 api）
```

## 4. 模块契约

### 4.1 `webook/go.work`

```
go 1.25.6

use (
	./api
	./shared
	./pkg
	./internal
	./chat
	./comment
	./interaction
	./migrator
	./relation
	./worker
)
```

- **提交进 git**：monorepo（所有模块同仓同步开发），`go.work` + `go.work.sum` 都提交，`.gitignore` 确保**不**忽略它们。（「不要提交 go.work」的建议针对发布型库，不适用本仓。）
- `use` 是相对路径，与模块路径前缀无关——前缀改成 `boyxs/train-go/webook` 后 go.work 内容不变。
- `go.work.sum` 由 `go work sync` 生成。

### 4.2 每个消费模块的 `go.mod`（以 `chat` 为例）

```
module github.com/boyxs/train-go/webook/chat

go 1.25.6

require (
	github.com/boyxs/train-go/webook/api    v0.0.0
	github.com/boyxs/train-go/webook/pkg    v0.0.0
	github.com/boyxs/train-go/webook/shared v0.0.0
	// ... chat 自己的外部依赖，go mod tidy 自动填
)

replace (
	github.com/boyxs/train-go/webook/api    => ../api
	github.com/boyxs/train-go/webook/pkg    => ../pkg
	github.com/boyxs/train-go/webook/shared => ../shared
)
```

- `require` 的 `v0.0.0` 只是占位，`go mod tidy` 会写成伪版本 `v0.0.0-00010101000000-000000000000`；实际解析全走 `replace` 指向的本地路径。
- **replace 行按各模块真实依赖生成**（脚本化，见 §6 Phase 3）：
  - `chat`/`comment`/`interaction`/`relation`：replace `api`、`pkg`、`shared`
  - `migrator`：replace `pkg`、`shared`（**无 `api`**）
  - `worker`：replace `api`、`pkg`、`shared`、**`internal`**（契约测试用）
  - `internal`(core)：replace `api`、`pkg`、`shared`
- `api`/`shared`/`pkg`：**无 replace**（叶子，无 webook 内部依赖）。

### 4.3 为什么 go.work 和 replace 都要

| 场景 | 只 go.work | go.work + replace |
|---|---|---|
| IDE / `go build ./...`（workspace 根） | ✅ | ✅ |
| 单模块 `go mod tidy` | ❌ 联网找兄弟模块失败 | ✅ 读 `../pkg/go.mod` |
| Docker 构建（`GOWORK=off`，确定性） | ❌ 必须带 go.work + workspace 模式 | ✅ go.mod 自描述 |
| CI 单服务 build/test | ✅（带 go.work） | ✅（两种都行） |

结论：**replace 是本地解析的真相源**（tidy/docker/CI 离线确定），**go.work 是 IDE/跨模块测试的便利层**。二者不冲突。

## 5. 配套改造（逐项落地）

### 5.1 Dockerfile（7 个）

构建 context 仍是 `webook/` 仓根（子目录）。改为 **`GOWORK=off` + 依赖 replace** 的 per-module 构建（确定性、无需带 go.work）。Dockerfile 本身**不含模块路径字符串**，前缀迁移不影响它，只需改构建逻辑。以 `chat` 为例：

```dockerfile
FROM golang:1.25.6-alpine AS builder
WORKDIR /src
ENV GOFLAGS=-mod=readonly GOWORK=off GOPROXY=https://goproxy.cn,direct

# 缓存层：先只 COPY chat 及其本地依赖的 go.mod/go.sum，再 download 外部依赖
COPY chat/go.mod chat/go.sum     ./chat/
COPY pkg/go.mod  pkg/go.sum      ./pkg/
COPY api/go.mod  api/go.sum      ./api/
COPY shared/go.mod shared/go.sum ./shared/
RUN cd chat && go mod download

# 再 COPY 源码（本地依赖 + 自身）
COPY pkg/    ./pkg/
COPY api/    ./api/
COPY shared/ ./shared/
COPY chat/   ./chat/

RUN cd chat && CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w -X main.version=${VERSION}" -o /out/chat .
```

- `GOWORK=off`：即使镜像混入 go.work 也忽略，避免 workspace 不确定性。
- 缓存层 COPY 的 go.mod 集合 = 该服务 replace 的模块集（`migrator` 去 api；`worker` 加 internal；`internal` 是 api/pkg/shared）。
- 简化回退方案：`COPY . .` 后 `cd chat && go build .`——丢层缓存，**推荐上面缓存友好版**。

### 5.2 CI workflow（7 个 `.github/workflows/webook-*-ci.yml`）

- `cache-dependency-path: webook/go.sum` → `webook/**/go.sum`（+ 可选 `webook/go.work.sum`）
- `MODULE: github.com/webook` → **`github.com/boyxs/train-go/webook`**（用于 `goimports -local`，前缀迁移必改）
- build/test：从「`working-directory: webook` + `go build ./...`」改为 **per-module**：`cd webook/chat && go build ./... && go test ./...`（go.work 生效，改 `pkg` 会连带用本地新代码）。
  ⚠ workspace 根的 `./...` **不跨模块**，必须进各模块目录跑，否则漏测。
- `paths` 过滤：改 `pkg`/`api`/`shared` 必须触发**所有**下游服务 CI（现状各服务 CI 的 `pull_request.paths` 已含 `webook/api/**`、`webook/pkg/**`，需补 `webook/shared/**` 并确认无遗漏）。

### 5.3 Makefile / mk

- 根 `Makefile`：`MODULE := $(shell head -1 go.mod ...)` 失效（根 go.mod 删）。改为硬编码 `MODULE := github.com/boyxs/train-go/webook`（供 `goimports -local`），或改成分发器调各服务 `Makefile`。
- `mk/mock.mk`：`-source=./internal/service/user.go` 等从根跑会跨模块解析失败，改 per-module（`cd internal && mockgen -source=./service/user.go ...`）。
- `mk/k8s.mk:13` 的 `go mod tidy`：改 per-module。
- `api/Makefile`：含 `github.com/webook` 前缀（protobuf 相关），前缀迁移时一并改。

### 5.4 wire / mockgen 命令（同步 `.claude/rules/coding-rules.md`）

- `cd webook && wire ./...` → per-module：`cd webook/internal && wire ./...`、`cd webook/chat && wire ./...` …
- `cd webook && wire ./internal/integration/setup/...` → `cd webook/internal && wire ./integration/setup/...`（`comment`/`interaction` 各有 `integration/setup/wire.go`）
- wire/mockgen 用 `go/packages`，有 go.work 时 workspace 感知，per-module 目录下能解析兄弟模块。

### 5.5 文档

- `CHANGELOG.md`：追加两条（前缀迁移 / 多模块拆分），或合并一条注明两步。
- `webook/CLAUDE.md`：更新「服务拆分同步项」——补「模块边界：新服务需 `go mod init` + replace + 纳入 `go.work` + `go work sync`」。
- 各服务 `CLAUDE.md`：注明本服务是独立 module + 新模块路径前缀。

## 6. 迁移步骤（分阶段，每步 2–5min 粒度，逐步验证）

> **两件独立的事，分开提交**：Phase 0 先在**单模块**状态把前缀对齐真实仓库；Phase 1+ 再做多模块拆分。每 Phase 独立可验证可回退。

**Phase 0 · 基线 + 前缀迁移**（单模块状态，一次机械替换最省事）
1. 确认 Go ≥ 1.25（已 1.25.6 ✓）；记录基线 `go build ./... && go test ./...` 通过，打 git 回退点
2. 改模块行：`cd webook && go mod edit -module=github.com/boyxs/train-go/webook`
3. 改 6 个 `.proto` 的 `option go_package`：`github.com/webook/api/...` → `github.com/boyxs/train-go/webook/api/...`
4. 全仓 import 前缀替换：`github.com/webook` → `github.com/boyxs/train-go/webook`（369 个 `.go` + `api/Makefile` + `mk/mock.mk`）
   - ⚠ coding-rules 禁止 `sed -i` 改源码：此处是**纯前缀重命名**，用 IDE 全局重构 / `gopls rename` / 受控脚本执行，并以 `go build ./...` 编译器兜底（执行方式实施前再定）
5. 重生成派生物：protobuf（`cd api && make ...` 或 protoc）、`wire ./...`、`make -f mk/mock.mk mockgen`
6. `go mod tidy && go build ./... && go vet ./... && go test ./...` 全绿
7. 改 7 个 CI workflow 的 `MODULE:` env → 新前缀
8. **单独提交**：`refactor(module): 模块路径对齐真实仓库 github.com/boyxs/train-go/webook`

**Phase 1 · 叶子模块**（无 webook 内部依赖，tidy 直接成功）
9. `cd api    && go mod init github.com/boyxs/train-go/webook/api    && go mod tidy && go build ./...`
10. `cd shared && go mod init github.com/boyxs/train-go/webook/shared && go mod tidy && go build ./...`
11. `cd pkg    && go mod init github.com/boyxs/train-go/webook/pkg    && go mod tidy && go build ./...`

**Phase 2 · 建 workspace**
12. `cd webook && go work init ./api ./shared ./pkg`

**Phase 3 · 服务模块**（逐个：init → 加 replace → tidy → `go work use` → 验证）
   顺序：`internal` → `chat` → `comment` → `interaction` → `migrator` → `relation` → `worker`
   每个模块：
   - `go mod init github.com/boyxs/train-go/webook/<svc>`
   - 加 replace（脚本：扫该模块 import 的 `.../webook/{api,pkg,shared,internal}` 前缀，命中哪个加哪行）
   - `go mod tidy`
   - 回 `webook/` 执行 `go work use ./<svc>`
   - `cd <svc> && go build ./... && go test ./...`
   - **worker 特例**：replace 额外加 `github.com/boyxs/train-go/webook/internal => ../internal`

**Phase 4 · 解散根模块 + sync**
13. 删除 `webook/go.mod`、`webook/go.sum`
14. `cd webook && go work sync`（生成 `go.work.sum`）
15. 全量验证：逐模块 `go build ./... && go vet ./...`

**Phase 5 · 配套改造**（逐文件 Edit + 验证，§5 全部）
16. 7 Dockerfile → 17. 7 CI workflow（cache-path + per-module build/test）→ 18. 根 Makefile + mk/*.mk → 19. coding-rules.md → 20. .gitignore 确认

**Phase 6 · 端到端验证 + 文档**（见 §7）

## 7. 验证清单（每 Phase 完成必须全绿）

- [ ] Phase 0：改完前缀后 `go build ./... && go test ./...` 全绿，生成物无残留 `github.com/webook`
- [ ] 每个模块 `cd <m> && go build ./... && go vet ./...` 通过
- [ ] 每个模块 `go test ./...` 通过（尤其 `worker` 契约测试跨模块解析成功）
- [ ] `cd webook && go work sync` 无报错，`go.work.sum` 已生成并提交
- [ ] 每个服务 `docker build -f <svc>/Dockerfile .`（context=webook/）成功产出镜像
- [ ] 单模块 `cd chat && go mod tidy` **离线**成功（不联网找兄弟模块）
- [ ] `wire ./...`、`mockgen` per-module 跑通，生成物无 diff
- [ ] 全仓 `grep -rn 'github.com/webook'` 只剩本文档/CHANGELOG 的历史说明，无代码残留
- [ ] CI 干跑：改 `pkg` 触发全下游服务 CI 且通过

## 8. 风险清单

| 风险 | 缓解 |
|---|---|
| 前缀替换漏改（如生成物、字符串拼接的路径） | `go build ./...` 编译器兜底 + 全仓 grep 校验；protobuf/wire/mock 全部重生成 |
| coding-rules 禁 `sed -i` 改源码 | 前缀重命名走 IDE 全局重构 / `gopls` / 受控脚本，实施前确认执行方式 |
| `go.work.sum` 未 sync/未提交 → `-mod=readonly` 报缺校验和 | Phase 4 `go work sync` 后提交 |
| 某模块漏加 replace → tidy 联网拉兄弟模块失败 | replace 脚本化，按 import 前缀统一生成 |
| Docker COPY 顺序错 → 缓存失效 / 缺 replace 目标 go.mod | 缓存层先 COPY 所有本地依赖的 go.mod（§5.1） |
| workspace 根 `./...` 不跨模块 → CI 漏测 | CI per-module 循环 build/test |
| worker 契约测试跨模块 → 缺 internal replace | Phase 3 worker 特例显式加 |
| mockgen/wire 从根跑跨模块解析失败 | per-module 目录下跑（§5.4） |
| `internal` 作模块名引发困惑 | 本文档说明：合法，可见性基于路径前缀，非模块边界 |
| 两大改动叠加难 bisect | Phase 0（前缀）与 Phase 1+（拆分）分开提交 |

## 9. 决策记录（已确认）

1. **模块路径前缀**：`github.com/webook` → **`github.com/boyxs/train-go/webook`**，对齐真实仓库 + `webook/` 子目录布局。作为**拆分前的独立第一步**在单模块状态完成（Phase 0）。
2. **`internal/`(core) 命名**：保持 `internal/`（`.../webook/internal`），零 import 改写。放弃改名 `core`。
3. **本地解析策略**：`go.work` + 各 `go.mod` 加 `replace`。tidy/Docker/CI 全离线确定性解析。放弃「只 go.work」。
4. **根模块**：彻底解散（删 `webook/go.mod`/`go.sum`），根目录无 Go 包。
5. **go.work/go.work.sum**：提交进 git（monorepo 实践）。

---

**下一步**：本方案确认后进入实施——建议 **Phase 0（前缀迁移）单独一个提交**先落地并验证，再逐 Phase 做多模块拆分。实施走逐文件/逐模块 Edit + 每步 `go build` 验证流程。
