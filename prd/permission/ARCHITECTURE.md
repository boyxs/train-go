# SaaS 权限系统（多租户 RBAC · 双面完整版） — 架构设计

> 配套：`prd/permission/PRD.md` · 原型 `prd/permission/prototypes/01~10-*.png`
> 决策基线：**permission 独立 gRPC 服务（8090/8091）· 自研 RBAC1+套餐 表驱动（不引 Casbin）· core 作 HTTP BFF · 鉴权 fail-closed · 平台侧=系统租户（org.id=1）+ Platform 套餐，与租户侧共用同一套机制**
> 原则：严格落地 10 屏原型 · KISS（P0/P1 之外不做）· 复用现有 gRPC 服务拆分范式（tag/search/relation）
> 前瞻设计：按 (b) 业务核心启用（权限=安全基础设施 + 多方接入 + 不可逆操作）；「双面完整版」范围经用户选项确认

---

## 1. 需求摘要

为小微书引入完整多租户权限平台：**租户侧**（组织/成员/部门/角色/功能矩阵/数据权限/审计）+ **平台运营侧**（租户管理/租户套餐/菜单管理/平台审计）。做完 = 10 屏原型全部有真实接口支撑；有效权限 = **∪(角色权限) ∩ 套餐权限**（且租户正常未过期）；任何服务「注册权限点 + 挂中间件」即可接入；越权/越界 API 层 403 兜底。本期不给内容域（文章/评论）挂权限码。

---

## 2. 服务拓扑与边界

| 服务 | 端口 HTTP/gRPC | 独占数据 | 对外（gRPC） | 依赖 |
|------|---------------|---------|-------------|------|
| **permission**（新建） | `8090/8091` | MySQL 12 表 + Redis（权限集/权限树缓存） | 租户侧 + 平台侧全量 RPC | MySQL、Redis |
| **core**（BFF） | 8010/8011（不变） | MySQL `user`（昵称聚合，不动） | 新增 HTTP `/org/*`、`/permission/*`、`/platform/*`；服务层持 permission gRPC client | permission gRPC |

- 服务发现 etcd（沿用 comment/interaction/relation/tag 同款）；纯静态本地配置（worker 档）。
- 端口铁律：permission 独占 `809x` 段（下一个新服务 `8100/8101`）。
- Metric：`webook_db_*`/`webook_grpc_*`/`webook_cache_*`，**禁止** `webook_permission_*`；区分靠 `job` label。
- user 信息归属：permission 只存 `uid`；昵称/头像/邮箱由 core BFF `userRepo.FindByIds` 批量聚合（对齐 `GRPCCommentService`）。
- **平台侧零特殊分支**：系统租户 `org.id=1`（种子）绑「Platform 套餐」（唯一含 `platform:*` 点的套餐）；平台运营人员=系统租户成员；平台路由与租户路由走**同一个**鉴权中间件（`x-org-id=1` 由前端平台入口固定携带）。

### 2.1 鉴权链路（热路径）

```
HTTP 请求 → core JWT 中间件(401)
         → RequirePermission("member:invite") 中间件：
             uid=claims · orgId=header[x-org-id]
             → permission.MemberPermissions(orgId, uid)   [gRPC, 超时 500ms]
                 ├─ Redis perm:set:{org}:{uid}:g{gver}:o{over} 命中直返
                 └─ miss → 计算：成员态 + 租户态(status/expires_at/plan)
                           + ∪(user_role→role_permission) ∩ plan_permission（∩ 后落缓存）
                           + data_scope 取最宽 + dept_ids 展开 → 回填(TTL 10min+jitter)
             ├─ 非成员/被禁用      → 403 ORG_FORBIDDEN
             ├─ 租户禁用/过期      → 403 TENANT_DISABLED / TENANT_EXPIRED
             ├─ code ∉ 有效权限集  → 403 PERMISSION_DENIED
             ├─ permission 不可达  → 403（fail-closed，禁止 fail-open）
             └─ 通过 → handler（orgId/dataScope/deptIds 注入 ctx 供下游）
```

- `x-org-id` 是**不可信选择器**：权限永远来自该 (org,uid) 的 DB/缓存事实，伪造 header 拿不到非成员组织的任何权限。
- 一次 RPC 完成 成员+租户+权限 三重校验；**∩ 套餐在写缓存前完成**，热路径零额外计算。
- 数据权限本期只**供数**（scope+deptIds 进 ctx），内容域消费下期接入。

---

## 3. 数据（permission 独占，DDL 落 `webook/permission/scripts/permission.sql`）

> DDL 铁律：单数表名 · 每列 COMMENT · bigint 毫秒 · utf8mb4_0900_ai_ci · 索引带表前缀。硬删风格 + `status` 位（对齐 tag/relation 先例），管理动作 `audit_log` 留痕即归档。

| 表 | 关键列 | 约束 / 索引 |
|----|--------|------------|
| `org` | name(30) · logo · owner_uid · **plan_id** · **expires_at**(0=永久) · status(1正常/0禁用) · member_count | `idx_org_owner(owner_uid)` · `idx_org_plan(plan_id)` |
| `org_member` | org_id · uid · **dept_id**(0=未分配) · status(1/0) · invited_by | `uk_org_member_edge(org_id,uid)` · `idx_org_member_uid(uid)` · `idx_org_member_dept(org_id,dept_id)` |
| `dept` | org_id · parent_id(0=根) · name(20) · sort · status | `uk_dept_sibling(org_id,parent_id,name)` · `idx_dept_org(org_id)` |
| `role` | org_id(**0=系统模板**) · name(20) · code(32) · type(1系统/2自定义) · **data_scope**(1全部/2自定义/3本部门/4本部门及以下/5仅本人) · description | `uk_role_org_code(org_id,code)` |
| `user_role` | org_id · uid · role_id | `uk_user_role_grant(org_id,uid,role_id)` · `idx_user_role_role(org_id,role_id)` |
| `role_dept` | role_id · dept_id（data_scope=2 自定义集） | `uk_role_dept(role_id,dept_id)` |
| `permission` | code(64) 唯一 · name(30) · **parent_id**(0=根) · type(**dir/menu/button**) · module(32) · **path**(64) · **icon**(32) · **visible**(1/0) · sort · status(1启用/0废弃) | `uni_permission_code(code)` · `idx_permission_parent(parent_id)` |
| `role_permission` | role_id · permission_code | `uk_role_permission(role_id,permission_code)` |
| `plan` | name(20) · code(32) · **member_limit** · status · remark | `uni_plan_code(code)` |
| `plan_permission` | plan_id · permission_code | `uk_plan_permission(plan_id,permission_code)` |
| `org_invite` | org_id · inviter_uid · email(254) · role_id · dept_id · token(32) · status(0pending/1accepted/2revoked) · expires_at | `uni_org_invite_token(token)` · `idx_org_invite_org(org_id,status)` |
| `audit_log`（P1） | org_id · operator_uid · action(64) · target_type(32) · target_id · detail json · ip(45) | `idx_audit_log_org_time(org_id,created_at)` |

**语义要点**
- **有效权限 = ∪(角色) ∩ 套餐**：套餐降级**不物理清** `role_permission`（∩ 时裁剪，升回自动恢复——芋道同款）；角色矩阵配置 UI 只展示套餐内的点（`/permission/tree` 已按套餐裁剪）。
- **菜单树 = permission 表树形化**：**代码管语义**（code/type/module 由 `registry.go` 声明、启动幂等 upsert，button 新增必须走代码），**DB 管展示**（name/path/icon/sort/visible 以 DB 为准、启动同步不覆盖）；dir/menu 可 UI 全管；废弃置 `status=0` 不物理删（保引用）。
- **数据权限展开在 permission 侧**：scope=3 → 本部门 id；scope=4 → 本部门+递归子树（≤5 层，一次查全 org 部门内存建树）；scope=2 → `role_dept` 并集；多角色取最宽（1 > 4 > 3 > 2 > 5）。
- **配额原子性**：接受邀请事务内 `UPDATE org SET member_count=member_count+1 WHERE id=? AND member_count < (SELECT member_limit FROM plan WHERE id=org.plan_id)`…实现按「先读 plan.member_limit 再条件 UPDATE」两步在同事务；affected=0 → 满员拒绝。移除成员 `GREATEST(0,-1)`。创建组织配额 ≤3 `COUNT FOR UPDATE`。
- **种子数据**：`plan`（Free/Pro/Enterprise/Platform + 各自 plan_permission）、`org`（id=1 系统租户，绑 Platform）、系统角色模板（org_id=0）与默认矩阵、初始平台 Owner（uid 走 config 指定）——全部在 `registry.go` 启动幂等同步。

### 3.1 缓存（permission 服务内，`permission/consts/cache.go`）

| key | 值 | TTL | 失效 |
|-----|----|----|------|
| `perm:set:{org}:{uid}:g{gver}:o{over}` | JSON{tenantStatus, roleCodes, permCodes(∩后), dataScope, deptIds} | 10min+jitter | **单人级**：改角色/部门/禁用/移除 → 精确 DEL 当前版本 key；**角色级**：改矩阵/删角色 → `idx_user_role_role` 反查绑定 uid（≤member_limit）pipeline DEL |
| `perm:over:{org}`（org 版本号） | int | 常驻 | **org 级**：换套餐/续期/禁用启用/改套餐矩阵(该套餐所有 org) → INCR，旧 key 靠 TTL 自清 |
| `perm:gver`（全局版本号） | int | 常驻 | **全局级**：菜单/权限点变更（visible/新增/废弃） → INCR |
| `perm:tree`（全量权限树） | JSON | 30min+jitter | 菜单管理写路径 DEL（树含展示字段，DB 可改，不再进程常驻） |

- 版本号读路径：`MGET perm:gver, perm:over:{org}`（1 RTT）→ 拼 set key → GET；miss 回源。**org/全局变更 O(1) 失效全租户**，无 scan、无 key 风暴。
- Redis 故障读回源 DB（性能降级不失能）；写失效失败记 WARN + 计数指标（脏窗 ≤15min）。
- Cache-Aside 全在 repository 层（`RedisPermissionCache`），Service 不碰 Redis。

---

## 4. 接口

### 4.1 gRPC 契约（`api/proto/permission/v1/permission.proto`，新建）

```proto
service PermissionService {
  // 组织（租户侧）
  rpc CreateOrg(CreateOrgReq) returns (Org);
  rpc MyOrgs(MyOrgsReq) returns (OrgList);
  // 成员
  rpc PageMembers(PageMembersReq) returns (PageMembersResp);      // 支持 q/roleId/deptId/status
  rpc InviteMembers(InviteMembersReq) returns (InviteMembersResp);
  rpc AcceptInvite(AcceptInviteReq) returns (AcceptInviteResp);
  rpc ChangeMemberRoles(ChangeMemberRolesReq) returns (google.protobuf.Empty);
  rpc ChangeMemberDept(ChangeMemberDeptReq) returns (google.protobuf.Empty);
  rpc ChangeMemberStatus(ChangeMemberStatusReq) returns (google.protobuf.Empty);
  rpc RemoveMember(RemoveMemberReq) returns (google.protobuf.Empty);
  // 部门
  rpc DeptTree(DeptTreeReq) returns (DeptTreeResp);               // 含各部门成员数
  rpc CreateDept(CreateDeptReq) returns (Dept);
  rpc UpdateDept(UpdateDeptReq) returns (google.protobuf.Empty);
  rpc DeleteDept(DeleteDeptReq) returns (google.protobuf.Empty);  // 有成员/子部门 → FailedPrecondition
  // 角色 & 矩阵（含数据权限）
  rpc ListRoles(ListRolesReq) returns (RoleList);
  rpc CreateRole(CreateRoleReq) returns (Role);                   // P1
  rpc UpdateRole(UpdateRoleReq) returns (google.protobuf.Empty);  // P1
  rpc DeleteRole(DeleteRoleReq) returns (google.protobuf.Empty);  // P1
  rpc RolePermissions(RolePermissionsReq) returns (RolePermissionsResp);      // codes+dataScope+deptIds
  rpc SaveRolePermissions(SaveRolePermissionsReq) returns (google.protobuf.Empty);
  // 鉴权（热路径）+ 权限树
  rpc MemberPermissions(MemberPermissionsReq) returns (MemberPermissionsResp);
  rpc PermissionTree(PermissionTreeReq) returns (PermissionTreeResp);         // scope=tenant 按套餐裁剪 / scope=all 平台全量
  // 平台侧（operator 必须是系统租户成员 + platform:* 码，服务端复核）
  rpc PageTenants(PageTenantsReq) returns (PageTenantsResp);
  rpc CreateTenant(CreateTenantReq) returns (Org);                // ownerEmail 未注册 → 挂邀请
  rpc ChangeTenantPlan(ChangeTenantPlanReq) returns (google.protobuf.Empty);
  rpc ChangeTenantStatus(ChangeTenantStatusReq) returns (google.protobuf.Empty);
  rpc ListPlans(ListPlansReq) returns (PlanList);
  rpc CreatePlan(CreatePlanReq) returns (Plan);
  rpc UpdatePlan(UpdatePlanReq) returns (google.protobuf.Empty);
  rpc DeletePlan(DeletePlanReq) returns (google.protobuf.Empty);  // 绑定租户 → FailedPrecondition
  rpc PlanPermissions(PlanPermissionsReq) returns (PermissionCodes);
  rpc SavePlanPermissions(SavePlanPermissionsReq) returns (google.protobuf.Empty);
  rpc CreateMenu(CreateMenuReq) returns (Permission);             // 仅 dir/menu；button → InvalidArgument
  rpc UpdateMenu(UpdateMenuReq) returns (google.protobuf.Empty);  // 展示字段全类型可改
  rpc DeleteMenu(DeleteMenuReq) returns (google.protobuf.Empty);  // 有子/被引用 → FailedPrecondition
  // 审计（P1）
  rpc WriteAudit(WriteAuditReq) returns (google.protobuf.Empty);
  rpc PageAuditLogs(PageAuditLogsReq) returns (PageAuditLogsResp);
}
message MemberPermissionsReq  { int64 org_id=1; int64 uid=2; }
message MemberPermissionsResp { bool is_member=1; int32 member_status=2; int32 tenant_status=3; // 1正常/2禁用/3过期
                                repeated string role_codes=4; repeated string perm_codes=5;      // ∩套餐后
                                int32 data_scope=6; repeated int64 dept_ids=7; }
```

- 所有写 RPC 带 `operator_uid`；保护规则（Owner 不可动/仅 Owner 授 Admin/禁自操作/系统角色只读/平台点仅 Platform 套餐）**全部收口 permission service 层**，core 不重复实现。
- gRPC `codes` → core 映射 `*errs.Error`。

### 4.2 HTTP 契约（core BFF）

29 个接口全表见 `PRD.md §3`（租户侧 17 + 平台侧 12），形状即契约单一源；本节只列映射与错误码。

- 路由：`/org/*`、`/permission/tree`、`/platform/*`，**不带 `/api` 前缀**；分页 POST + `ginx.PageResult`；`ginx.WrapReq[T]` + validator 校验。
- 平台路由挂同一 `RequirePermission("platform:*")` 中间件（前端平台入口固定 `x-org-id: 1`）。
- **错误 sentinel**（`internal/errs/permission.go`，全带 `.WithReason`）：

| reason | HTTP | 用户消息 |
|--------|------|---------|
| `ORG_FORBIDDEN` | 403 | 你不在该组织或已被禁用 |
| `TENANT_DISABLED` / `TENANT_EXPIRED` | 403 | 该组织已被停用 / 服务已到期，请联系平台 |
| `PERMISSION_DENIED` | 403 | 无权限执行此操作，请联系管理员 |
| `OWNER_PROTECTED` / `SELF_OPERATION_FORBIDDEN` | 403/400 | 所有者不可被操作 / 不能对自己执行该操作 |
| `ADMIN_GRANT_FORBIDDEN` | 403 | 仅所有者可授予或撤销管理员 |
| `ROLE_SYSTEM_READONLY` / `ROLE_BOUND` | 403/409 | 系统角色不可修改 / 角色仍有成员绑定 |
| `DEPT_NOT_EMPTY` / `DEPT_DEPTH_EXCEEDED` | 409/400 | 部门下仍有成员或子部门 / 部门层级超过 5 级 |
| `ORG_CREATE_LIMIT` / `ORG_MEMBER_LIMIT` | 400 | 创建组织已达上限（3 个）/ 组织成员已达套餐上限 |
| `INVITE_EXPIRED` / `INVITE_INVALID` | 400 | 邀请已过期，请联系管理员重发 / 邀请无效 |
| `PLAN_BOUND` / `PLAN_PLATFORM_RESERVED` | 409/403 | 套餐仍有租户绑定 / platform 权限仅系统套餐可含 |
| `MENU_REFERENCED` / `MENU_CODE_READONLY` | 409/403 | 菜单被角色或套餐引用 / 按钮型权限点需代码注册 |

- **幂等**：uk 兜底（member_edge/user_role/role_permission/plan_permission/invite_token/dept_sibling）+ AcceptInvite 重复返「已加入」+ token CAS + 矩阵/套餐保存=事务内 diff。

### 4.3 前端消费（webook-fe）

- `api/org.ts` / `api/permission.ts` / `api/platform.ts`；axios 拦截器注入 `x-org-id`（平台路由固定 1）。
- `usePermission()`：`/org/member/me/permissions` 进 context；`<PermissionGuard code>` 控按钮；菜单按 `menus`（menu 型码 ∩ visible）渲染；「平台管理」入口仅当用户是系统租户成员（`/org/list` 返回标识）。
- 页面：`(main)/org/{members,depts,roles,roles/[id],audit}` + `(main)/platform/{tenants,plans,menus,audit}`，严格对齐原型 01~10（Table bordered、URL 分页、移动卡片化）。

---

## 5. 风险

- **安全**：fail-closed；`x-org-id` 不可信仅选择器；平台接口双保险（中间件 `platform:*` 码 + service 层复核 operator 是系统租户成员）；提权规则收口 service；邀请 token 128bit 随机一次性；audit 只 insert。
- **并发**：套餐上限/组织配额原子 UPDATE；并发接受邀请 uk+CAS；矩阵/套餐保存事务 diff；部门删除与成员移入并发 → 删除事务内 `COUNT(member)+COUNT(child) FOR UPDATE` 复核。
- **性能**：热路径 = 1 gRPC + 2 次 Redis RTT（版本号 MGET + set GET）；miss 回源 4 表索引 join（org/user_role/role_permission/plan_permission，(org,uid) 粒度）；部门树全 org 一次拉取内存建树（≤5 层、部门数几十级别）；成员列表昵称 `FindByIds` 批量防 N+1；角色级失效 pipeline DEL ≤ member_limit key。
- **一致性**：单人/角色级精确 DEL ≤1 RTT 生效；org/全局级版本号 INCR O(1) 生效（满足 PRD ≤5s）；DEL/INCR 失败降级 TTL 兜底（≤15min 脏窗，WARN+指标）。
- **回归**：全新服务 + core 纯新增；`article:*` 点只注册不挂路由，内容域行为不变；`org.id=1` 种子与既有业务无交集。

---

## 6. 任务拆分

> 五阶段，粒度 2–5min/项。P1=服务核心（租户侧），P2=平台侧能力，P3=core BFF，P4=前端，P5=部署（14 点见 §D）。

**P1 · permission 服务核心（`webook/permission/`，平铺布局）**
| # | 任务 | 依赖 | 验收 |
|---|------|------|------|
| 1 | 建表脚本 `scripts/permission.sql`（12 表+索引+COMMENT）+ dao model + `TableName()` | 无 | SQL↔struct 对齐 |
| 2 | `registry.go`：权限树（dir/menu/button）+ 4 系统角色矩阵 + 4 套餐 + 系统租户/平台 Owner 种子；启动幂等同步（语义覆盖、展示字段不覆盖） | 1 | 重复启动幂等；废弃点 status=0 |
| 3 | `GormOrgDAO`（Create 配额/MyOrgs/原子 member_count(联动 plan.member_limit)）+ `GormOrgMemberDAO`（Page 含 dept 筛选/状态/部门变更/删除） | 1 | dao_test：配额/套餐上限/uk 全绿 |
| 4 | `GormDeptDAO`（树查/CRUD/层级≤5/同级唯一/删除复核）+ `GormRoleDAO`+`GormUserRoleDAO`+`GormRoleDeptDAO`+`GormRolePermissionDAO`（矩阵 diff+data_scope） | 1 | dao_test：树/矩阵/数据权限全绿 |
| 5 | `GormPlanDAO`（CRUD/绑定禁删/plan_permission diff）+ `GormPermissionDAO`（树/菜单 CRUD/引用禁删）+ `GormOrgInviteDAO` + `GormAuditLogDAO` | 1 | dao_test 全绿 |
| 6 | `RedisPermissionCache`：set(∩后)/tree Get/Set/Del + `perm:gver`/`perm:over:{org}` INCR/MGET + jitter；consts Pattern | 无 | cache_test：版本翻转命中失效全绿 |
| 7 | `CachePermissionRepository`：MemberPermissions（成员+租户态+∪∩+scope 展开）Cache-Aside + 三级失效（单人 DEL/角色批量 DEL/版本 INCR） | 3-6 | repo_test：三级失效链路全绿 |
| 8 | `InternalPermissionService`（租户侧）：组织/成员/邀请/部门/角色矩阵 + 保护规则 + 审计埋点 | 7 | service_test：AC1-AC8/AC11 全覆盖 |
| 9 | `permission.proto` + regen；`grpc/PermissionServer`（租户侧 rpc）；wire + 5 config(:8091) + integration(main.go 锚) | 8 | `wire ./... && go test ./...` 绿 |

**P2 · 平台侧能力**
| # | 任务 | 依赖 | 验收 |
|---|------|------|------|
| 10 | 租户管理 service+rpc（Page/Create(owner 未注册挂邀请)/ChangePlan/ChangeStatus + over INCR + 平台审计） | 9 | AC10 过；换套餐 ≤5s 生效 |
| 11 | 套餐管理 service+rpc（CRUD/矩阵 diff/platform 点校验/绑定禁删 + 同套餐全 org over INCR） | 9 | AC9 过 |
| 12 | 菜单管理 service+rpc（dir/menu CRUD/button 仅展示字段/引用禁删 + gver INCR + tree 缓存 DEL） | 9 | AC12 过 |

**P3 · core BFF 接线**
| # | 任务 | 依赖 | 验收 |
|---|------|------|------|
| 13 | ioc：etcd 解析 `permissionv1.PermissionServiceClient`；`GRPCPermissionService`（user 聚合、codes→errs 映射、邀请 URL） | 9 | service_test（mock）全绿 |
| 14 | `RequirePermission(code)` 中间件（claims+x-org-id→MemberPermissions→三重校验→fail-closed→ctx 注入 scope）+ sentinel | 13 | middleware_test：401/403 全分支 |
| 15 | `PermissionHandler`+`PlatformHandler`（29 路由，`ginx.WrapReq`+PageResult）+ 路由注册 | 14 | web_test 全绿；无 `/api` 前缀 |
| 16 | core wire regen + `make wire` + `make verify`（GOWORK=off） | 15 | 全绿 |

**P4 · 前端（webook-fe）**
| # | 任务 | 依赖 | 验收 |
|---|------|------|------|
| 17 | `api/org|permission|platform.ts` + axios x-org-id 注入 + org 切换器（含平台入口） | 15 | 创建/切换/进平台可用 |
| 18 | `usePermission` + `<PermissionGuard>` + 组织设置/平台管理双布局（menu 码过滤） | 17 | 无权限菜单/按钮不渲染 |
| 19 | 成员管理+邀请弹窗+移动端（原型 01/02/06）；部门管理页（07：树+成员简表） | 18 | 5 态齐；AC2/AC6/AC11 过 |
| 20 | 角色管理+权限配置页（03/04：矩阵+数据权限 5 档+系统角色只读）+ 审计页（05，P1） | 18 | AC5 过 |
| 21 | 平台侧：租户管理（08）+ 套餐管理（09）+ 菜单管理（10） | 18 | AC9/AC10/AC12 过 |

**P5 · 部署 + 可观测（14 点，收尾 `grep -rn permission` 全仓核对）**
| # | 任务 | 依赖 | 验收 |
|---|------|------|------|
| 22 | Dockerfile + compose（healthcheck/depends_on mysql,redis,etcd）+ deploy.sh + `.env.*(.example)` `PERMISSION_IMAGE_TAG` | 9 | `./deploy.sh local` 起全栈 |
| 23 | prometheus job + rules + grafana 告警/看板 + nginx 确认（HTTP 走 core 无新 upstream） | 22 | 监控见 job=permission |
| 24 | CI workflow（paths 互斥）+ `permission/CLAUDE.md` + webook/CLAUDE.md 端口台账 + CHANGELOG | 22 | CI 触发；14 点无遗漏 |

---

## A. 前瞻性设计（(b) 业务核心）

| 维度 | 问题 | 当前方案 |
|------|------|---------|
| 扩展性 | 第二个服务/新菜单接入改动多大？ | 面向能力：接入=registry 注册权限点+挂中间件，核心/存储零改动；套餐机制天然支持功能分层售卖；`permission.code` 无业务硬编码 |
| 可用性 | permission 挂了主流程能跑吗？ | 管理面 fail-closed 403（安全>可用，显式取舍）；内容域不受影响（未挂码）；Redis 挂回源 DB；gRPC 500ms 超时 |
| 容错性 | 重复/并发/极端输入安全吗？ | 全写路径 uk 兜底+原子 UPDATE 配额+token CAS+矩阵 diff 幂等+`GREATEST(0,·)` 防负+部门层级/同级唯一约束 |
| 可观测性 | 5 分钟能定位吗？ | audit_log 双面留痕；拒绝路径结构化日志（who/org/code/reason）；指标：鉴权 QPS/拒绝数/缓存命中率/版本号 INCR 次数/失效失败计数 |

## B. 分层设计（permission 平铺布局，不套 internal/）

`grpc/`（PermissionServer：toPb+参数校验）→ `service/`（InternalPermissionService：保护规则/配额/∩ 语义/审计埋点/事务边界）→ `repository/`（CachePermissionRepository + Org/Dept/Role/Plan/Menu/Invite Repository：Cache-Aside+三级失效）→ `dao/`（9 个 GormXxxDAO，事务在 DAO）+ `cache/`（RedisPermissionCache）；`ioc/`；`consts/`；`registry.go`。构造函数返回接口；`toDomain`/`toEntity` 单条 + `slicex.Map`。

**core**：`web/PermissionHandler`+`PlatformHandler`（只调 service+toVO）→ `service/GRPCPermissionService`（持 client+userRepo 聚合，镜像 `GRPCCommentService`）→ 下游 gRPC；中间件 `internal/web/middleware/permission.go`（依赖 service 接口）。

## C. Wire / DI 变更

- **permission** wire：mysql+redis+9 DAO+cache+repos+service+PermissionServer+grpcx server+registry 同步 hook（`permissionProviderSet`）；`integration/setup` wire 装配 + `integration/main.go` 锚。
- **core** wire：`permissionv1.PermissionServiceClient`（etcd）、`GRPCPermissionService`、两个 Handler、`RequirePermission` provider；`wire ./...`+`make wire`+`make verify`。

## D. 服务拆分 14 点检查表（permission，8090/8091）

| 维度 | 落点 |
|------|------|
| 应用配置 | `permission/config/{local,dev,staging,prod,test}.yaml`：mysql+redis+grpc :8091+otel=permission |
| Wire / Dockerfile / CI | `wire.go`+regen；多阶段构建 context=webook/；`permission-ci.yml`（paths 互斥） |
| Prometheus / Grafana | job=permission :8090 + rules；`alerting/permission.yml`（up/5xx/P99/goroutines）+ services-overview |
| Compose / Nginx | 服务定义+healthcheck+core depends_on；HTTP 走 core BFF 无新 upstream |
| 部署脚本/变量 | deploy.sh + `.env.*` `PERMISSION_IMAGE_TAG`/APP_ENV |
| Metric / 文档 | `webook_*` 通用命名；`permission/CLAUDE.md`+CHANGELOG+端口台账 |

> 发版：`webook-permission-v*` tag + `deploy/.env.prod.example` 同步 `PERMISSION_IMAGE_TAG`。

---

## 附：本轮不做（KISS 边界）

内容域挂权限码（下期）· 岗位 post/用户组（P2，纯标签无权限语义）· 数据权限的内容域消费（本期只供数）· SSO/SCIM/计费订单（P2）· 登录日志/在线用户/强制下线（P2，属 core 认证域）· 多部门归属（单部门 KISS）· 前端动态路由（Next.js 静态路由+码过滤，不做若依式 component 动态加载）· 多角色 UI（模型支持，UI 单选）· core 本地二级缓存 · Casbin。
