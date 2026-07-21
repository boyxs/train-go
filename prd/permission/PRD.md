# SaaS 权限系统(permission) — 产品需求文档(PRD)

> 定位：**双面完整版多租户 RBAC 权限平台**——①租户侧自助后台(组织/成员/部门/角色/权限矩阵/数据权限/审计)+②平台运营侧超管后台(租户管理/租户套餐/菜单管理/平台审计)。权限点注册制+DB 菜单化管理，任何服务可注册权限点接入鉴权。
> 业界范式：对标 若依-pro / 芋道(yudao) 多租户版标准功能集（租户+套餐+菜单+部门+数据权限），融合 Slack/Notion workspace 自助创建模式（PLG 自助 + 运营管控并存）。
> 原型：`prd/permission/prototypes/*.png` · pen 源：`prd/permission/permission.pen`

---

## 1. 模块概述

为小微书引入组织（租户）维度的完整 SaaS 权限平台：**谁（成员/部门）在哪个空间（租户）以什么身份（角色）在什么范围（数据权限）能做什么（权限点），租户能用什么由套餐决定（plan），一切授权变更可追溯（审计）**。平台运营人员在超管后台管理租户生命周期与功能边界。

### 背景（为什么现在做）
- 现有用户体系是平台全局单角色，无组织概念，无法支撑团队协作与商业化分层（套餐）。
- 管理类操作无权限收口，扩展任何管理面都缺鉴权地基。
- 平台自身也需要运营面（开通/禁用租户、控制功能边界），与租户侧共用一套 RBAC 机制最经济。

### 已完成（相邻能力，本模块复用/共存）
- ✅ 用户注册/登录 + JWT（`x-access-token`/`x-refresh-token`）——权限系统在其上叠加租户上下文
- ✅ 用户资料体系（uid/昵称/头像）——成员列表 core BFF 聚合
- ✅ 统一响应 `{code,msg,data}` + Gin 中间件链——权限中间件按现有模式挂接

### 交付状态（本模块，全部待建）
**租户侧**
- ⬜ 组织（租户）：自助创建（默认 Free 套餐）/ 我的组织 / 切换
- ⬜ 成员管理：列表（含部门）/ 邀请 / 调整角色 / 调整部门 / 禁用 / 移除
- ⬜ 部门管理：部门树 CRUD（≤5 层）+ 成员归属
- ⬜ 角色：系统预置 4 角色 + 自定义角色；**功能权限矩阵 + 数据权限范围**
- ⬜ 审计日志（P1）
**平台运营侧（系统租户）**
- ⬜ 租户管理：开通 / 换套餐续期 / 禁用启用 / 列表检索
- ⬜ 租户套餐：套餐 CRUD + 套餐×权限点矩阵（套餐=租户功能边界）
- ⬜ 菜单/权限点管理：树形管理（目录/菜单可视化管理；按钮型代码注册 + UI 管展示）
- ⬜ 平台审计（复用审计能力，记在系统租户下）

---

## 2. 页面清单

租户侧挂「组织设置」布局（左侧设置导航：成员/部门/角色/审计/组织信息）；平台侧挂「平台管理」布局（仅系统租户成员可见入口，侧导航：租户/套餐/菜单/平台审计）。移动端租户侧收横滑 Tab；平台运营后台桌面优先（移动端 P2）。

| # | 页面 | 路由 | 认证/权限码 | 原型 |
|---|------|------|------|------|
| 1 | 成员管理 | `/org/members` | `member:view` | 01-成员管理.png |
| 2 | 邀请成员（弹窗） | `/org/members`（弹层） | `member:invite` | 02-邀请成员弹窗.png |
| 3 | 角色管理 | `/org/roles` | `role:view` | 03-角色管理.png |
| 4 | 角色权限配置（功能矩阵+**数据权限**） | `/org/roles/[id]` | `role:manage`（系统角色只读） | 04-角色权限配置.png |
| 5 | 审计日志 | `/org/audit` | `audit:view` | 05-审计日志.png |
| 6 | 成员管理（移动端） | 同 1，`< 768px` | 同 1 | 06-成员管理-移动.png |
| 7 | 部门管理 | `/org/depts` | `dept:view` | 07-部门管理.png |
| 8 | 租户管理（平台） | `/platform/tenants` | `platform:tenant:view` | 08-租户管理.png |
| 9 | 租户套餐（平台） | `/platform/plans` | `platform:plan:view` | 09-租户套餐.png |
| 10 | 菜单/权限点管理（平台） | `/platform/menus` | `platform:menu:view` | 10-菜单管理.png |
| — | 平台审计 | `/platform/audit` | `platform:audit:view` | 复用 05 样式，不单独出图 |
| — | 组织切换器（含「平台管理」入口） | Header 全局 | 登录 | 体现在 01/08 |
| — | 403 兜底页 | 越权直访 | — | P1，无独立原型 |

---

## 3. API 接口

统一 `{code,msg,data}`；租户上下文经 `x-org-id`；平台侧接口固定作用于系统租户（org_id=1），无需 header。分页 POST。

### 3.1 租户侧

| Method | 路径 | 说明 | 权限码 |
|--------|------|------|------|
| POST | `/org` | 自助创建组织（默认 Free 套餐），创建者成为 Owner | JWT（配额≤3） |
| GET | `/org/list` | 我加入的组织（含 planCode、是否平台成员标识） | JWT |
| POST | `/org/member/list` | 成员分页 `{page,size,q,roleId,deptId,status}` | `member:view` |
| POST | `/org/member/invite` | 批量邮箱邀请 `{emails[]≤20, roleId, deptId?}` | `member:invite` |
| POST | `/org/invite/accept` | 受邀接受 `{token}`（幂等） | JWT |
| PUT | `/org/member/:id/roles` | 调整角色 `{roleIds[]≥1}` | `member:manage` |
| PUT | `/org/member/:id/dept` | 调整部门 `{deptId}`（0=未分配） | `member:manage` |
| PUT | `/org/member/:id/status` | 启用/禁用 | `member:manage` |
| DELETE | `/org/member/:id` | 移除成员 | `member:remove` |
| GET | `/org/dept/tree` | 部门树（含各部门成员数） | `dept:view` |
| POST | `/org/dept` · PUT/DELETE `/org/dept/:id` | 部门 CRUD `{parentId,name≤20,sort}`；有成员/子部门禁删 | `dept:manage` |
| GET | `/org/role/list` | 角色列表（成员数、系统/自定义） | `role:view` |
| POST | `/org/role` · PUT/DELETE `/org/role/:id` | 自定义角色 CRUD（≤20/org；绑定禁删）（P1） | `role:manage` |
| GET | `/org/role/:id/permissions` | 角色配置回显 `{codes[],dataScope,deptIds[]}` | `role:view` |
| PUT | `/org/role/:id/permissions` | 保存 `{codes[],dataScope:1..5,deptIds[]}`（系统角色 403；codes ⊆ 套餐） | `role:manage` |
| GET | `/permission/tree` | 当前租户可见权限树（**已按套餐裁剪**，配矩阵用） | JWT |
| GET | `/org/member/me/permissions` | 我的 `{roleCodes[],permCodes[],dataScope,deptIds[],menus[]}`（前端渲染源） | JWT+x-org-id |
| POST | `/org/audit/list` | 审计分页（P1） | `audit:view` |

### 3.2 平台侧（超管后台）

| Method | 路径 | 说明 | 权限码 |
|--------|------|------|------|
| POST | `/platform/tenant/list` | 租户分页 `{page,size,q,planId,status}` | `platform:tenant:view` |
| POST | `/platform/tenant` | 开通租户 `{name, ownerEmail, planId, expiresAt}`（owner 未注册则发邀请） | `platform:tenant:manage` |
| PUT | `/platform/tenant/:id/plan` | 换套餐/续期 `{planId, expiresAt}`（0=永久） | `platform:tenant:manage` |
| PUT | `/platform/tenant/:id/status` | 禁用/启用租户（禁用=全员即时 403） | `platform:tenant:manage` |
| GET | `/platform/plan/list` | 套餐列表（含绑定租户数） | `platform:plan:view` |
| POST | `/platform/plan` · PUT/DELETE `/platform/plan/:id` | 套餐 CRUD `{name≤20,memberLimit,remark}`；绑定租户禁删 | `platform:plan:manage` |
| GET | `/platform/plan/:id/permissions` | 套餐权限点集合 | `platform:plan:view` |
| PUT | `/platform/plan/:id/permissions` | 保存套餐矩阵 `{codes[]}`（platform:* 点仅 Platform 套餐可含） | `platform:plan:manage` |
| GET | `/platform/menu/tree` | 全量菜单/权限点树（含隐藏） | `platform:menu:view` |
| POST | `/platform/menu` · PUT/DELETE `/platform/menu/:id` | 菜单管理：目录/菜单全字段可管；按钮型仅展示字段（code 语义代码注册）；有子节点/被角色或套餐引用禁删 | `platform:menu:manage` |
| POST | `/platform/audit/list` | 平台审计（org_id=1 维度） | `platform:audit:view` |

> 鉴权失败语义：401 未登录；`403 ORG_FORBIDDEN` 非成员/被禁用；`403 TENANT_DISABLED / TENANT_EXPIRED` 租户禁用/过期；`403 PERMISSION_DENIED` 缺权限码。接口变更前后端同步。

---

## 4. 用户故事

### P0 — 核心路径
| 角色 | 操作 | 价值 |
|------|------|------|
| 用户 | 自助创建组织（Free 套餐）成为 Owner；多组织切换 | 零门槛开团队空间 |
| Owner/Admin | 邀请成员、调整角色/部门、禁用/移除 | 团队治理 |
| Owner/Admin | 维护部门树，把成员挂到部门 | 组织结构化 |
| Admin | 给角色配功能权限矩阵 + 数据权限范围 | 精细授权 |
| 成员 | 只看到套餐∩角色允许的菜单/按钮；数据按 scope 过滤 | 界面即权限、数据隔离 |
| 平台运营 | 开通租户、指定套餐与有效期 | 商业化管控 |
| 平台运营 | 禁用违规租户（全员即时失效） | 平台治理 |
| 系统 | API 层按 套餐∩角色 权限码拦截（403），fail-closed | 安全兜底 |

### P1 — 首版应有
| 角色 | 操作 | 价值 |
|------|------|------|
| 平台运营 | 新建/编辑套餐并配置功能边界；给租户换套餐/续期 | 套餐化售卖 |
| 平台超管 | 菜单管理：调整目录/菜单可见性、排序、图标 | 运营自助 |
| Admin | 自定义角色 CRUD；审计日志查询 | 精细化+可追溯 |
| Owner | 转让所有权 | 组织生命周期 |
| 成员 | 越权/租户过期看到明确兜底页 | 明确反馈 |

### P2 — 后续迭代
| 角色 | 操作 | 价值 |
|------|------|------|
| Admin | 岗位管理（纯标签，无权限语义）、用户组 | 大团队补充维度 |
| 企业 | SSO（OIDC/SAML）/ SCIM 同步 | 企业集成 |
| 平台 | 套餐计费/订单、租户运营看板、登录日志/在线用户/强制下线 | 商业化闭环+账号安全 |
| 系统 | 敏感操作二人复核 | 高危动作管控 |

---

## 5. 数据模型（通用核心 · 细化见 ARCHITECTURE）

> 时间列一律 bigint 毫秒；硬删风格 + status 位 + 审计留痕。**系统租户 org.id=1（种子）**：平台运营人员=系统租户成员，绑「Platform 套餐」（唯一含 `platform:*` 点的套餐）——平台侧与租户侧**共用同一套 RBAC 机制，零特殊分支**。

| 表 | 职责 | 关键点 |
|----|------|--------|
| `org` | 租户 | + `plan_id`、`expires_at`(0=永久)、`status`、`member_count` |
| `org_member` | 成员关系 | + `dept_id`(0=未分配)；uk(org_id,uid) |
| `dept` | 部门树 | org_id · parent_id · name · sort · 层级≤5；uk(org_id,parent_id,name) |
| `role` | 角色 | org_id(0=系统模板) · code · type · **`data_scope`**(1全部/2自定义/3本部门/4本部门及以下/5仅本人) |
| `user_role` | 成员↔角色 | 多角色并集；uk(org_id,uid,role_id) |
| `role_dept` | 自定义数据范围 | data_scope=2 时的 role↔dept 集合 |
| `permission` | 菜单/权限点树 | code 唯一 · **parent_id** · type(**dir/menu/button**) · path/icon/visible/sort · status(1启用/0废弃)；**代码管语义（code/type/module），DB 管展示（name/icon/sort/visible/path）** |
| `role_permission` | 角色矩阵 | uk(role_id,permission_code) |
| `plan` | 租户套餐 | name · code · member_limit · status · remark；种子 Free/Pro/Enterprise/Platform |
| `plan_permission` | 套餐边界 | uk(plan_id,permission_code) |
| `org_invite` | 邀请 | token 唯一 · 72h · status |
| `audit_log` | 审计 | org_id 维度（平台动作记 org_id=1）（P1） |

**有效权限 = ∪(成员各角色权限) ∩ 套餐权限**，且租户 status=1 且未过期。套餐降级不物理清角色矩阵（∩ 时被裁剪，升级自动恢复——对齐芋道语义）。多角色数据权限取**最宽**。

### 预置角色 / 套餐（启动同步）
| 角色 | 默认权限 | 数据权限 |
|------|---------|---------|
| Owner | 全部（含 org:manage 转让/解散） | 全部 |
| Admin | 除 org:manage 外全部管理面 | 全部 |
| Editor | 内容域读写（article:*，预留） | 本部门及以下 |
| Viewer | 内容域只读 | 仅本人 |

| 套餐 | 边界 | 成员上限 |
|------|------|---------|
| Free | 成员/角色/组织信息（无部门/审计/自定义角色） | 10 |
| Pro | 租户侧全功能 | 200 |
| Enterprise | 租户侧全功能（+未来 SSO） | 500 |
| Platform | 全功能 + `platform:*`（仅系统租户绑定，不可分配给普通租户） | — |

---

## 6. 用户流程

### 6.1 租户开通（双通道）
```
自助：用户 → 组织切换器「创建组织」 → 默认 Free/10 人 → 成为 Owner
运营：平台后台「开通租户」 → 填名称+Owner 邮箱+套餐+有效期
  ├─ 邮箱已注册 → 直接建 org + 授 Owner；未注册 → 挂邀请，注册接受后生效
  └─ 审计 tenant.create 记系统租户
```

### 6.2 配置角色（功能+数据权限）
```
角色管理 → 角色权限配置页
  ├─ 功能权限：按模块勾选（仅展示套餐内的点；模块全选/半选）
  ├─ 数据权限：单选 全部/自定义/本部门/本部门及以下/仅本人；选「自定义」展开部门树多选
  ├─ 系统角色 → 整页只读锁定
  └─ 保存 → 二次确认（影响 N 名成员）→ 相关成员权限缓存精确失效（≤5s 生效）
```

### 6.3 套餐管控（平台）
```
套餐管理 → 选/建套餐 → 勾选功能边界 → 保存 → 绑定该套餐的所有租户即时生效（∩ 裁剪）
租户管理 → 换套餐/续期/禁用
  ├─ 禁用/过期 → 该租户全员 MemberPermissions 返 tenant 态 → 全接口 403 + 前端兜底页
  └─ 恢复/续期 → 即时恢复
```

### 6.4 鉴权链路（系统）
```
请求 → JWT(401) → RequirePermission(code)：
  uid+x-org-id → permission.MemberPermissions（缓存）
  ├─ 校验：成员&正常 → 租户正常&未过期 → code ∈ (角色∪ ∩ 套餐)
  ├─ 数据权限：resp 附 dataScope+deptIds，业务查询按 scope 拼过滤（内容域接入下期）
  └─ permission 服务不可达 → 403（fail-closed）
```

---

## 7. 设计规范

全局以 `webook-fe/.claude/rules/design-tokens.md` 为真相源；本模块补充沿用初版（§7.1 角色徽章 / §7.2 状态色 / §7.3 管理面组件 / §7.4 交互，见下），新增：

- **套餐徽章**：Free `#F3F4F6`/`#6B7280` · Pro `#DBEAFE`/`#2563EB` · Enterprise `#E0E7FF`/`#6366F1` · Platform `#F0FDFA`/`#0D9488`
- **租户状态**：正常/已禁用同成员状态；**已过期** `#FFFBEB`/`#D97706`
- **菜单类型徽章**：目录 `#F3F4F6`/`#6B7280` · 菜单 `#F0FDFA`/`#0D9488` · 按钮 `#E0E7FF`/`#6366F1`
- **树形表格**：子级行左缩进 24px/层 + chevron 展开符；部门树选中项浅 teal 底
- **平台管理布局**：与组织设置同构（左导航 200px），Header 组织切换器处显示「平台管理」标识
- 角色徽章：Owner `#F0FDFA/#0D9488` · Admin `#DBEAFE/#2563EB` · Editor `#FFFBEB/#D97706` · Viewer `#F3F4F6/#6B7280` · 自定义 `#E0E7FF/#6366F1`；成员状态：正常 `#F0FDF4/#22C55E` · 已禁用 `#F3F4F6/#9CA3AF` · 待接受 `#FFFBEB/#D97706`
- Table 白卡 r12、表头 `#F9FAFB` 12/600、行高 56、破坏性操作 Modal 二次确认 danger、系统角色锁 icon、加载 skeleton、空态引导

---

## 8. 边界与约束

| 维度 | 约束 |
|------|------|
| 租户 | 自助创建 ≤3 个/人；成员上限=套餐 `member_limit`（Free10/Pro200/Ent500）；禁用/过期全员即时 403 |
| 套餐 | 绑定租户的套餐禁删；`platform:*` 点仅 Platform 套餐可含；换套餐/改套餐矩阵即时生效（∩ 裁剪，不清角色数据） |
| 部门 | 层级 ≤5；同级同名唯一；有成员/子部门禁删；成员单部门归属（KISS，多部门 P2） |
| 数据权限 | 5 档（若依枚举）；多角色取最宽；本期权限系统只**供数**（scope+deptIds），内容域消费下期 |
| Owner 保护 | Owner 唯一；不可被移除/禁用/改角色；转让 P1；不能移除自己 |
| 分级管理 | Admin 不可操作 Owner；仅 Owner 授/撤 Admin；禁自操作（防锁死） |
| 系统角色/菜单 | 系统角色矩阵只读；菜单树中 button 型新增走代码注册（UI 只管展示字段），dir/menu 可 UI 全管；被引用节点禁删 |
| 自定义角色 | ≤20/org；名称 ≤20 字符；绑定禁删 |
| 邀请 | 单次 ≤20 邮箱；72h 过期；同邮箱 pending 唯一（重发=刷新 token）；接受时套餐满员失败提示 |
| 权限生效 | 角色/矩阵/成员变更 ≤5s（精确失效）；套餐/租户级变更 ≤5s（org 版本号批量失效） |
| 审计 | 只记管理动作；平台动作记系统租户；保留 180 天（P1） |
| 分页/多端 | pageSize ≤50；租户侧响应式，平台后台桌面优先（移动 P2） |

---

## 9. 验收标准

**AC1 · 自助创建**：Given 登录用户 When 创建组织 Then 成为 Owner、套餐=Free、上限 10 人、部门/审计菜单不可见（Free 无此点）

**AC2 · 邀请成员**：Given Admin When 输入 2 邮箱+角色 Editor+部门「内容部」发送 Then 生成 pending（72h）；接受后成员列表出现，角色/部门正确

**AC3 · API 越权兜底**：Given Viewer When 直调 `PUT /org/role/:id/permissions` Then `403 PERMISSION_DENIED` 数据无变更

**AC4 · 权限变更即时生效**：Given Editor 在线 When Admin 改其为 Viewer Then ≤5s 其写入口消失、写接口 403

**AC5 · 功能+数据权限矩阵**：Given Admin 配置「实习编辑」 When 勾 `article:view`+dataScope=仅本人 保存 Then 该角色成员菜单只剩对应项且资源查询仅返回本人数据（内容域接入后）；系统角色页只读

**AC6 · 保护规则**：Given Admin When 对 Owner 操作/移除自己 Then 禁用或 403

**AC7 · 审计（P1）**：Given Admin 移除成员 When 查审计 Then 首条含操作人/`member.remove`/对象/时间/IP

**AC8 · 邀请过期**：Given 邀请超 72h When 打开链接 Then 失效页；列表可重发

**AC9 · 套餐裁剪**：Given 租户 Pro→Free When 平台改套餐 Then ≤5s 该租户全员部门/审计菜单消失、对应接口 403；角色矩阵数据未被清除，升回 Pro 自动恢复

**AC10 · 租户禁用/过期**：Given 平台禁用租户（或到期） When 该租户任意成员访问任意组织接口 Then `403 TENANT_DISABLED/EXPIRED` + 前端兜底页；启用/续期后恢复

**AC11 · 部门树**：Given Admin When 在「内容部」下建「编辑组」并把成员移入 Then 树与成员数正确；删除有成员的部门被拒绝

**AC12 · 菜单管理**：Given 平台超管 When 隐藏「审计日志」菜单（visible=0） Then 所有租户前端该菜单消失；按钮型点在 UI 新增被引导走代码注册

---

## 10. 风险与待讨论（留给 architect）

- ✅ **平台侧机制**：定案「系统租户 org.id=1 + Platform 套餐」——平台与租户共用一套 RBAC，零特殊分支
- ✅ **套餐语义**：定案 ∩ 裁剪不清数据（芋道同款）
- ✅ **菜单管理边界**：定案 代码管语义 / DB 管展示；dir/menu 可 UI 全管、button 走代码注册
- ⬜ **org 级批量失效**：套餐/租户变更需失效整租户缓存——版本号方案 vs scan DEL（architect 定）
- ⬜ **数据权限消费方式**：中间件注入 scope 到 ctx？统一 DAO 过滤 helper？（内容域接入期定）
- ⬜ **邀请触达**：无邮件基础设施，MVP「复制邀请链接」，邮件后补
- ⬜ **开通租户 Owner 未注册**：挂邀请 vs 先建占位账号（architect 定，倾向挂邀请）
- ⬜ **岗位（post）**：P2，纯标签无权限语义，表结构预研不建

---

## 11. 文档与原型同步清单

| 产物 | 路径 | pen frame id |
|------|------|------|
| PRD / 架构 | `prd/permission/PRD.md` · `ARCHITECTURE.md` | — |
| 原型 pen 源 | `prd/permission/permission.pen` | — |
| 01 成员管理 | `prototypes/01-成员管理.png` | `C3TJ9` |
| 02 邀请成员弹窗 | `prototypes/02-邀请成员弹窗.png` | `CrEH4` |
| 03 角色管理 | `prototypes/03-角色管理.png` | `A0qEXt` |
| 04 角色权限配置（+数据权限） | `prototypes/04-角色权限配置.png` | `E6yXi` |
| 05 审计日志 | `prototypes/05-审计日志.png` | `bMLL0` |
| 06 成员管理（移动） | `prototypes/06-成员管理-移动.png` | `ZBq7Q` |
| 07 部门管理 | `prototypes/07-部门管理.png` | `Lle6B` |
| 08 租户管理（平台） | `prototypes/08-租户管理.png` | `n7sW5` |
| 09 租户套餐（平台） | `prototypes/09-租户套餐.png` | `Sljt6` |
| 10 菜单管理（平台） | `prototypes/10-菜单管理.png` | `ibUgH` |

> **Pencil 导出注意（实测踩坑）**：① MCP `open_document` 指不存在的 .pen 只建内存文档不落盘；该内存文档会在**其 tab 首次在桌面应用中可见**时写盘一次，此后磁盘加载态下 **MCP 改动不置应用 dirty 位、合成键（SendKeys Ctrl+S）可能被 UIPI 拦截**——改完 pen 必须**人工**在 Pencil 中切到该文档按 Ctrl+S，否则改动只在内存；② `export_nodes` 的 `outputDir` 写 `prd/**` 报误导性错误 `wrong .pen file`——先导会话 scratchpad 再 move 改名；③ 远离视口的 frame 不进渲染缓存、批量导出失败——逐帧移到原点后**可直接单帧导出**（无需 `open_document` 刷新，实测），导完把坐标还原；④ `batch_design` 的 `C()` 复制里 `descendants` 键必须用**上一次调用结果返回的真实节点 id**，同块绑定名会被静默忽略（复制体变成源的原样克隆）。

**同步规则**：pen 改动必须 ①MCP 改 pen ②`export_nodes` 覆盖 PNG ③同步本 PRD，缺一不可。
