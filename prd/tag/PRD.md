# 通用标签系统 — 产品需求文档（PRD）

> 定位：**通用（Universal）**——标签不绑定文章、不绑定单一来源。一套多态结构服务多种被标注对象（文章 / 用户 / 对话…）与多个消费方（内容发现 / 搜索筛选 / 未来 Chat 意图路由）。
> 业界范式：多态 taggable 模型（Rails acts-as-taggable / Stack Overflow tag 模型）+ typeahead 打标签 + AI 候选建议 + 标签聚合页 + 搜索 facet。
> 原型：`prd/tag/prototypes/*.png` · pen 源：`prd/tag/tag.pen`

---

## 1. 模块概述

为平台**任意实体**提供多对多、多来源的标签能力，取代"一篇文章只有一个 5 值 `category` 枚举"的粗粒度分类，支撑更细的内容发现、搜索筛选与后续智能路由。

### 背景（为什么现在做）

- **文章侧**当初"标签/分类 + 搜索"需求被**语义搜索（ES BM25+kNN）+ 单值 category** 绕过，没建真正的标签体系（`prd/article/prd.md:24` 标注删除线）。
- **Chat 侧**仍把「文章标签/分类体系」列为**未完成 P1**，因为意图识别 + 路由分发需要比 5 值 category 更丰富的标签能力（`prd/chat/prd.md:20-21`）。
- 两侧口径矛盾的根因：缺一个**通用、细粒度、可多值**的标签层。本模块补齐它。

### 已完成（相邻能力，本模块复用/共存）
- ✅ 单值 `category` 枚举（tech/career/life/ai/other）+ 榜单分区（**保留不动**）
- ✅ 文章语义搜索 ES（BM25 + kNN，`article_v1` 索引）
- ✅ 文本向量化 embedding（`content_vec`，dims=1024）——AI 推荐标签将复用

### 交付状态（本模块）
- ✅ 通用 `tag` + `tagging` + `tag_follow` 多态模型（`biz` + `biz_id` + `source`；硬删风格、无 `deleted_at`）
- ✅ 作者发文章打标签（typeahead 选已有 / 建新）+ AI 推荐候选（kNN 相似文章标签聚合 + 相似度阈值）
- ✅ 标签浏览页 `/tag/[slug]`（内容聚合 + 关注按钮 / 粉丝数 / 本周新增）
- ✅ 搜索结果页按标签 facet 筛选（已改为正式结果页）
- ✅ 标签订阅（关注 / 取关 / 关注态，原 P1）+ tag 详情 Cache-Aside 缓存（原 P1）
- ⬜ 标签广场 `/tags`（P1，未做）
- ⬜ 对话意图标签供 Chat 路由消费（P2，仅预留 `biz='conversation'` + `type='intent'`）

---

## 2. 页面清单

| # | 页面 | 路由 | 认证 | 原型 |
|---|------|------|------|------|
| 1 | 发文章·标签输入（区块） | `/article/edit`（编辑页内） | 需登录 | 01-发文章打标签.png |
| 2 | 标签浏览页 | `/tag/[slug]` | 公开 | 02-标签浏览页.png |
| 3 | 搜索结果页（标签筛选） | `/search?q=&tags=` | 需登录 | 03-搜索结果页.png |
| 4 | 标签广场（P1） | `/tags` | 公开 | — |
| — | 移动端适配（1~3） | 同上 `< 768px` | — | 04~06-*-移动端.png |

---

## 3. API 接口

统一响应 `{code,msg,data}`；路由不带 `/api` 前缀（前端 baseURL `/api`，网关剥前缀）。

| Method | 路径 | 说明 | 认证 |
|--------|------|------|------|
| GET | `/tag/suggest?q=&limit=` | typeahead 前缀补全已有标签（含 `ref_count`） | JWT |
| POST | `/tag/recommend` | AI 基于文章内容推荐候选标签（body: `{title, content}` 或 `{articleId}`） | JWT |
| GET | `/tag/:slug` | 标签详情（`name/slug/description/ref_count/follow_count/weekly_new_count/is_following`） | 公开（可选登录，登录才算 `is_following`） |
| POST | `/tag/:slug/articles` | 标签下文章分页（`{page,size,sort}`，sort=new/hot） | 公开 |
| POST | `/tag/:slug/follow` / DELETE | 关注 / 取关标签（返 `{isFollowing, followCount}`） | JWT |
| POST | `/article`（**扩展**） | 发布文章附带 `tags: string[]`（≤5） | JWT |
| POST | `/search/article`（**扩展**） | 请求体加 `filter: { tags: string[] }`（terms 过滤） | JWT |
| GET | `/search/facets?q=` | 返回结果集的标签 facet（terms agg top N + count） | JWT |
| GET | `/tags/trending`（**P1，未做**） | 标签广场：热门 / 趋势标签 | 公开 |

> 接口变更铁律：前后端同步更新；`POST /article` 与 `POST /search/article` 是**已有接口扩展字段**，需前后端同时改。

---

## 4. 用户故事

### P0 — 核心路径
| 角色 | 操作 | 价值 |
|------|------|------|
| 作者 | 发文章时给文章打多个标签（从已有库选 / 新建） | 内容更易被检索、聚合 |
| 作者 | 看到系统基于正文推荐的候选标签，一键采纳 | 降低打标签成本、提升标签质量与一致性 |
| 读者 | 点文章上的标签，进入标签页看该标签下全部文章 | 顺着主题深入发现内容 |
| 读者 | 搜索结果页按标签 facet 缩小范围（叠加在关键词之上） | 更快定位目标内容 |
| 系统 | 文章标签变更时同步进 ES（`tags` keyword 多值字段） | 搜索 facet / 过滤实时可用 |

### P1 — 首版应有
| 角色 | 操作 | 价值 |
|------|------|------|
| 读者 | ✅ 订阅感兴趣的标签（关注/取关/关注数） | 追踪某主题更新 |
| 读者 | ⬜ 浏览标签广场（热门 / 趋势标签词云或列表） | 发现平台内容版图 |
| 作者 | ✅ 编辑已发布文章的标签（编辑页回显 + TagInput） | 修正 / 补充标签 |

### P2 — 后续迭代
| 角色 | 操作 | 价值 |
|------|------|------|
| 系统（Chat） | 给对话 / query 打意图标签（`type=intent`）用于路由分发 | 打通智能客服意图识别前置依赖 |
| 运营 | 标签治理：合并同义、设别名、禁用低质 | 控制标签爆炸、保持质量 |
| 系统 | 给用户打兴趣标签（`biz=user`）做个性化推荐 | 千人千面 |

---

## 5. 数据模型（通用核心 · 细化留给 architect）

> 对齐项目多态模式：`user_interaction` 的 `biz+biz_id`、`ClickEvent` 的 `source`。领域命名通用化（CLAUDE.md #8）。

**`tag`** — 标签本体，与被标注对象**完全解耦**

| 列 | 类型 | 说明 |
|----|------|------|
| `id` | bigint PK | 主键 |
| `name` | varchar(30) | 展示名（如 `Golang`） |
| `slug` | varchar(30) | URL 友好唯一名（小写、空格转 `-`、去特殊字符；如 `golang`） |
| `type` | varchar(16) | **通用命名空间**：`topic`(内容主题) / `intent`(chat意图) / …默认 `topic` |
| `description` | varchar(255) | 标签简介（标签页展示，可空） |
| `ref_count` | bigint | 内容引用数（多少对象打了此标签；`SyncByBiz` 事务内 `GREATEST` 同步维护） |
| `follow_count` | bigint | 关注数（多少用户关注此标签；`tag_follow` 翻转时事务内 `GREATEST` 维护） |
| 唯一键 `uni_tag_slug_type (slug,type)` | | 同命名空间内 slug 唯一 |
| 索引 `idx_tag_type_refcount (type,ref_count)` | | typeahead / 热门排序 |

**`tagging`** — 通用多态关联（谁 · 被打了什么 · 谁打的）

| 列 | 类型 | 说明 |
|----|------|------|
| `id` | bigint PK | 主键 |
| `tag_id` | bigint | → `tag.id` |
| `biz` | varchar(32) | **通用目标类型**：`article` / `user` / `conversation` …零改表扩展 |
| `biz_id` | bigint | 目标实体 id |
| `source` | varchar(16) | **通用来源**：`author` / `ai` / `ops` / `system` |
| 唯一键 `uk_tagging_dedup (biz,biz_id,tag_id)` | | 同对象同标签不重复 |
| 索引 `idx_tagging_tag_biz (tag_id,biz)` | | 反查"某标签下的 article" |
| 索引 `idx_tagging_target (biz,biz_id)` | | 正查"某文章的标签" |

**`tag_follow`** — 用户关注标签边（uid → tag_id）

| 列 | 类型 | 说明 |
|----|------|------|
| `id` | bigint PK | 主键 |
| `uid` | bigint | 关注者 uid |
| `tag_id` | bigint | → `tag.id` |
| `status` | tinyint | 关注状态：1=关注中 0=已取关（翻转维护，不物理删） |
| 唯一键 `uk_tag_follow_edge (uid,tag_id)` | | 一个用户对一个标签一条边（FOR UPDATE 翻转）|
| 索引 `idx_tag_follow_uid (uid,status)` | | 我关注的标签列表 |

时间列一律 `bigint`(Unix 毫秒)。**三表均硬删风格、无 `deleted_at`**：`tagging` untag 物理删、`tag_follow` status 翻转（对齐 relation/interaction 计数表，避免软删幽灵冲突）。DDL 真相源 `tag/scripts/tag.sql`。

**与 category 的关系（本期结论：并存）**
- `category`（5 值枚举）**保留不动**——仍管榜单分区 / 一级大类 / 意图大类。
- `tag` 是新增的细粒度、多值、通用层，正交于 category。
- 未来若要统一，可把 category 收编为 `tag.type='category'` 的保留命名空间——**本期不做**，改动最小、不破坏已有榜单链路。

---

## 6. 用户流程

### 6.1 打标签（作者）
```
写文章页 → 标签输入框聚焦
  ├─ 输入前缀 → GET /tag/suggest → 下拉候选（含 ref_count）→ 选中成 chip
  ├─ 输入库里没有的词 → "创建「foo」" 选项 → 新建 chip（source=author）
  ├─ AI 推荐区：POST /tag/recommend（基于标题+正文/embedding）→ 候选 chips → 点击加入（source=ai）
  ├─ 已选 chips 可 ✕ 删除；上限 5 个（到顶禁用输入 + 提示）
  └─ 发布 → POST /article {..., tags:[...]} → 异步同步 ES tags 字段
```

### 6.2 标签浏览（读者）
```
任意文章卡片上的标签 chip → 跳 /tag/[slug]
  标签头：名称 · ref_count 篇 · N 人关注 · 本周新增 M 篇 · 简介 · [关注按钮]（已实现，详见 CHANGELOG）
  排序 Tab：最新 | 最热
  文章列表（复用文章卡片）→ 分页
  异常：标签不存在 → 404 空状态引导；该标签暂无文章 → 空状态占位
```

### 6.3 搜索 + 标签筛选（读者）
```
/search 搜索框输入关键词回车 → 结果页
  ├─ 顶部标签 facet 条：GET /search/facets → 该结果集 top N 标签 + count（chip 可多选）
  ├─ 选中标签 → URL 加 &tags=a,b → POST /search/article {query, filter:{tags:[a,b]}}
  ├─ 已选 facet 高亮 + 一键清除
  ├─ 结果列表（文章卡片，命中高亮）→ 分页
  ├─ 空 query → 落地榜单（现状保留）
  └─ 无结果 → 空状态 + 放宽筛选建议
```

---

## 7. 设计规范

沿用现有搜索/榜单页视觉，标签 pill 复用 `views/search/RankingBoard.tsx` 的 `CategoryTag` 风格。

### 7.1 色彩
| 层级 | 色值 | 用途 |
|------|------|------|
| Primary | `#0D9488` | 品牌主色：logo、主按钮、激活 Tab、已选 facet、订阅按钮 |
| 文字主 | `#1A1A1A` | 标题、正文 |
| 文字次 | `#9CA3AF` | 元信息、placeholder、计数 |
| 边框 | `#E5E7EB` | 卡片/输入框边框；更浅 `#F3F4F6` 分割线 |
| 页底 | `#F5F5F5` | 页面背景 |
| 卡片 | `#FFFFFF` | 内容容器 |
| 标签色 | 浅 teal 底 `#F0FDFA` / 文字 `#0D9488`（token `teal-surface`/`primary`，全站配色收口 `constants/theme.ts` + `globals.css @theme`）；AI 候选紫色区分 |

分类色沿用现状：技术=蓝、职场=琥珀、AI=teal（`CategoryTag` 既有映射）。

### 7.2 排版（Inter）
| 层级 | 字号/字重 | 场景 |
|------|-----------|------|
| H1 | 24-28 / 700 | 页面标题、标签名 |
| H2 | 18-20 / 700 | 区块标题 |
| Body | 14-15 / 400-500 | 正文、卡片标题 |
| Caption | 12-13 / 500 | 元信息、标签文字、计数 |

### 7.3 容器与间距
- 圆角：卡片 12 / 按钮·输入 8 / 标签 pill 12（胶囊）/ 分页 6
- 间距：xs4 · sm8 · md12-16 · lg20-24 · xl32
- 桌面宽 1280，内容区 padding 24；卡片静态无阴影、hover `shadow-md`

### 7.4 响应式
| 断点 | 策略 |
|------|------|
| `< 768px` | 单列；标签 facet 横向滚动（中文禁换行）；标签输入 chips 换行 flow；隐藏次级导航 |
| `≥ 768px` | 多列；facet 平铺 |

### 7.5 交互
- 标签输入：typeahead 下拉、Enter/逗号成 chip、Backspace 删末尾 chip、到 5 上限禁用输入
- AI 候选：加载时 skeleton chips；采纳后从候选区移除、进已选区
- facet 多选即时刷新结果（URL 驱动，可分享/回退）
- 空状态有插图 + 引导；加载显 skeleton；破坏性操作（删标签）二次确认

---

## 8. 边界与约束

| 维度 | 约束 |
|------|------|
| 每篇文章标签数 | **≤ 5**（对齐 Stack Overflow / Medium / dev.to 业界惯例，防标签滥用） |
| 标签名 | ≤ 30 字符；slug 归一化（小写 + 空格转 `-` + 去特殊字符）；`(slug,type)` 唯一，大小写不敏感 |
| 新建标签权限 | 登录用户可建（MVP）；后续可加低质审核 / 运营治理 |
| AI 推荐 | 复用 `content_vec` + ES kNN 取相似文章、聚合其标签作候选（零 LLM）；命中按相似度阈值过滤远邻（不相关不硬推）；无 embedding / 无相似邻居时降级为空、不阻塞发布 |
| 搜索 facet | terms agg top 20；多选默认 **AND**（可 architect 阶段定 OR） |
| ES 同步 | 文章标签变更走现有 `IndexArticle` 异步 goroutine + 30s 超时；失败仅告警不阻塞主流程 |
| 分页 | pageSize ≤ 50；标签下文章默认 20/页 |
| ref_count / follow_count | 冗余计数，写路径**事务内** `GREATEST(0,±1)` 同步维护（打/删标签、关注/取关），防负；用于排序/展示 |
| 权限 | 标签页读公开；打标签 / 订阅需登录；`/search/article` 保持受保护现状 |
| 多端 | PC + 移动响应式 |

---

## 9. 验收标准

**AC1 · 作者打标签**
- Given 作者在写文章页
- When 输入 `go` 前缀
- Then 下拉出已有标签 `Golang(128)`、`Go并发` 等候选；选中成 chip；上限 5 个

**AC2 · AI 推荐**
- Given 文章正文已填写
- When 触发 AI 推荐
- Then 返回 top 5-8 候选标签 chips，点击加入已选（`source=ai`）；无 embedding 时候选区为空且发布不受阻

**AC3 · 新建标签**
- Given 输入库中不存在的词 `WebAssembly`
- When 选择"创建「WebAssembly」"
- Then 发布后新建 `tag{slug:webassembly,type:topic}`，`tagging{biz:article,source:author}` 关联

**AC4 · 标签浏览页**
- Given 标签 `golang` 下有 128 篇文章
- When 访问 `/tag/golang`
- Then 展示标签头（名称 + 128 篇 + 简介）+ 文章列表（默认最新，可切最热）+ 分页

**AC5 · 搜索标签筛选**
- Given 搜索"并发"返回 40 条结果
- When 点击 facet 标签 `Golang`
- Then URL 变 `?q=并发&tags=golang`，结果收窄为同时命中"并发"关键词且带 `Golang` 标签的文章

**AC6 · 空/异常**
- Given 访问不存在的 `/tag/nope`
- Then 展示 404 空状态 + 返回引导；标签下无文章时展示"暂无内容"占位

---

## 10. 风险与待讨论（留给 architect）

- ✅ **facet 多选语义** → 定案 **AND**（单条 ES 查询 `post_filter` 收窄 + `aggs` 恒基于关键词命中集）
- ✅ **ref_count 维护策略** → 定案 **事务内 `GREATEST(0,±1)` 同步维护**（对齐 relation/interaction）
- ✅ **slug 国际化** → 定案 **保留中文（UTF-8）**：`NormalizeSlug` 小写/trim/空白折叠/剥 path 不安全字符，免拼音库
- ✅ **AI 推荐召回源** → 定案 **embedding kNN 近邻聚合相似文章标签 + 相似度阈值**（零 LLM；阈值生产需实测微调）
- ⬜ **标签合并 / 别名**：仍 P2，未预留 `alias_of`（需要时再加，不提前抽象）
- ⬜ **category 收编时机**：本期并存不动；未来是否统一为 `type='category'` 待定
- ⬜ **Chat 意图标签**：`type=intent` 产生与消费链路（P2，模型已就绪、路由未实现）

---

## 11. 文档与原型同步清单

| 产物 | 路径 | pen frame id |
|------|------|------|
| PRD 文档 | `prd/tag/PRD.md` | — |
| 原型 pen 源 | `prd/tag/tag.pen` | — |
| 01 发文章打标签（桌面） | `prd/tag/prototypes/01-发文章打标签.png` | `u02MB` |
| 02 标签浏览页（桌面） | `prd/tag/prototypes/02-标签浏览页.png` | `r0ZrHH` |
| 03 搜索结果页（桌面） | `prd/tag/prototypes/03-搜索结果页.png` | `XeYLF` |
| 04 发文章打标签（移动） | `prd/tag/prototypes/04-发文章打标签-移动.png` | `HZgj3` |
| 05 标签浏览页（移动） | `prd/tag/prototypes/05-标签浏览页-移动.png` | `otpOs` |
| 06 搜索结果页（移动） | `prd/tag/prototypes/06-搜索结果页-移动.png` | `PVOAk` |
| 服务文档 | `webook/tag/CLAUDE.md`（拆分服务文档）· `webook/search/CLAUDE.md`（检索服务） | — |
| 建表脚本 | `webook/tag/scripts/tag.sql`（tag/tagging/tag_follow DDL） | — |
| CHANGELOG 追加 | ✅ 已追加（2026-07-11 ~ 07-12 多条：⑤关注/⑥本周新增/详情缓存/AI 推荐修复/配色收口/SQL 脚本） | — |

> tag.pen 当前仅含 6 张标签交付原型（桌面 3 @y1389 + 移动 3 @y0）；复制基底 `rSwsj`/`tVVgY` 已在完成复制后删除（其源仍在 `chat.pen`，如需再建可重新复制）。
> **Pencil 导出注意**：本文件较大，远离画布视口（原点附近）的 frame 不进渲染缓存、`export_nodes` 会导出空白 body（也会误报 `partially clipped`）。改法：把目标 frame 移到原点附近（y≈0）再 `open_document` 刷新后导出。
> **移动 facet 约束**：390px 下标签 facet 一行最多约 3 个带计数 chip，多出的靠横滑（PRD §7.4），原型只展示可见部分，勿硬塞导致溢出。

**同步规则**：pen 原型任何改动必须①Pencil MCP 改 pen ②`export_nodes` 覆盖 PNG ③同步更新本 PRD 第 6/7 节描述，缺一不可。
