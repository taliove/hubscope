# 0005 — 多用户认证与按 Hub 隔离

## 背景

当前认证是单管理员口令模型:`ADMIN_PASSWORD` 环经变量 → 明文常驻内存 → stateless session cookie(签名 key 从密码派生,`auth.go:24-27`)。审计 `actor` 硬编码 `"admin"`(`audit.go:35`),无用户表,无角色,无数据隔离。

本 spec 将其迁移为多用户 + 四级角色 + 按 Hub 隔离,解决:团队多用户并发使用、per-actor 审计可追溯、多 Hub 部署的数据隔离。本 spec **修订承重墙 W6**(凭证边界),并新增一个隐式不变量(按 Hub 查询隔离)。配套 ADR 0011。

## 决策总览

1. **四级角色**:super_admin(全局,CLI 创建,不绑 Hub) / admin(Hub 内全权含本 Hub 用户管理) / operator(Hub 内操作无用户管理) / viewer(Hub 内只读管理台)。super_admin 负责建 Hub + 给各 Hub 派首个 admin,解决按 Hub 隔离下跨 Hub 角色的语义裂缝。
2. **Bootstrap**:新增 CLI `hubscope admin create` 创建首个 super_admin;`ADMIN_PASSWORD` 废弃(Phase 1 起记 deprecation warning,Phase 7 硬忽略)。不自动从 env 建用户(密码非用户名,自动建是安全洞)。
3. **按 Hub 隔离**:除 super_admin 外每个用户绑 `hub_id`,角色在该 Hub 内生效,登录后只看/管本 Hub 数据。
4. **Session 签名 key**:独立 `SESSION_SECRET` env,缺失则首启在 `settings` 表自动生成 32 字节 hex 落库;不再从任何密码派生。轮换即全部 session 失效。token 结构扩为 `<userId>.<issuedUnix>.<hmac>`,user 身份随 cookie 携带、由 HMAC 验证。
5. **公开路由语义**(双语义分叉,handler 内按 session 判断):
   - `/api/overview`:匿名=全局聚合(公开状态板语义不变);登录且非 super_admin=本 Hub 聚合;super_admin=全局。
   - `/api/endpoints/{id}`(及 `/series`/`/probes`/`eval-summary`):公开详情保持全局可访问(状态本就公开,跨 Hub 可看)。
6. **全局资源口径**(JOIN-through-model 对这些表不成立,单独定调):
   - `settings`/`classification_rules`/`suites`/`cases`:保持全局,写操作仅 super_admin,Hub 级用户只读。
   - `audit_logs`:**加 `hub_id` 列**(`s.audit` 落库时从 ctx 取;auth/settings/discovery 类无 Hub 归属的动作 `hub_id=NULL`,仅 super_admin 可见)。这是 schema 变更,吸收 architect 纠错——plan 原说"Phase 4 无 schema 变更"在此破功。
   - `tasks`:按 `entity_type` JOIN 过滤(eval_run→campaign→campaign_models→models;hub→直接 entity_id);rollup/retention 类无 Hub 归属的 task 归 super_admin 可见。
   - `hubs`:super_admin 看全部;Hub 级用户只看自己的 Hub(前端 Hub 切换器需要)。
7. **题库全局**:`suites`/`cases` 跨 Hub 一致(W7 评估不可变性的延伸,同套题跨 Hub 才有可比性)。`campaigns`/`eval_runs` 经 `campaign_models`→`models.hub_id` 天然按 Hub 隔离,JOIN 成立。
8. **后台作业不改隔离逻辑**:`discovery`/`prober`/`evaluator`/`alerter`/`scheduler` 按 entity_id 精确写,不经 session、不涉及跨 Hub 聚合读。**后台作业不写 audit_logs**(architect grep 零命中验证),它们用 `tasks.source` 字段;故 `s.audit` 的 30 个调用点全在 HTTP handler 内、全持有 `r`,可直接从 ctx 取 user,无需"回退 system"路径。
9. **结构性隔离强制**(architect 加固建议,替代纯测试护栏):store 层 `List*` 函数**去掉无参形态**,只留 `ListXByHub(hubID int64)` + `ListXAll()`(仅 super_admin 路径可达)。漏传 hub 过滤=编译错误,结构性消除"忘记过滤"。isolation sweep 测试作运行时第二道防线。

## 数据模型变更(`internal/store/store.go` migrate)

### 新表 `users`
```
id INTEGER PK, username TEXT UNIQUE NOT NULL,
password_hash TEXT NOT NULL,            -- bcrypt
hub_id INTEGER NULL,                    -- NULL = super_admin; FK hubs(id)
role TEXT NOT NULL,                     -- super_admin|admin|operator|viewer
enabled INTEGER NOT NULL DEFAULT 1,
created_at DATETIME NOT NULL
```
UNIQUE(username)。hub_id 为 NULL 仅 super_admin 可达(应用层 + CHECK 约束)。

### `audit_logs` 加列
`hub_id INTEGER NULL`(via `ensureColumn`);`s.audit` 落库时从 ctx 取 user.hub_id 写入,无 Hub 归属动作(auth.login/auth.logout/settings.update/discovery.*)写 NULL。回填:历史行 hub_id 全 NULL(等价 super_admin 可见,不破坏旧数据可读)。

### `settings` 新 key
`session_secret`(32 字节 hex,首启若 SESSION_SECRET env 缺失则自动生成落库)。

## API 契约变更

### 认证(`/api/auth/*`)
- `POST /api/auth/login`:body 从 `{password}` 改 `{username, password}`;查 users 表( bcrypt.Verify + enabled=1)→ 签发 `<userId>.<unix>.<hmac>` cookie。
- `GET /api/auth/me`:返回 `{authenticated, user:{id, username, role, hub_id, hub_name|null}}`。
- `POST /api/auth/logout`:不变。
- 登录限流沿用(ticket 16,5/min/IP),不新增。

### 用户管理(`/api/users/*`,新增)
- `GET /api/users`:super_admin 看全部;admin 只看本 Hub。
- `POST /api/users`:super_admin 可建任意角色(含 super_admin/admin)+ 指定 hub_id;admin 只能在本 Hub 建 operator/viewer。
- `PATCH /api/users/{id}`:改 role / enabled;admin 只能改本 Hub 用户,且不能把自己降级以防锁死。
- `PUT /api/users/{id}/password`:admin/super_admin 重置他人密码;用户改自己密码另走 `PUT /api/users/me/password`(需旧密码)。
- `DELETE /api/users/{id}`:admin 只能删本 Hub 用户;super_admin 任意;禁止自删。
- 路由 gate:`requireRole("super_admin","admin")`。

### 中间件
- `requireSession` 保留,放行后把 `{userID, role, hubID, username}` 注入 `r.Context()`(新增 `SessionUser` 类型 + `sessionUser(r)` helper)。
- 新增 `requireRole(roles ...string)`:叠在 `requireSession` 之上,读 ctx role,不匹配 403。路由分组:用户管理=super_admin+admin;写操作=super_admin+admin+operator;读=任意已登录;公开 GET 仍走 publicReadPattern。
- `s.audit(r, action, ...)`:签名不变,内部读 `sessionUser(r)` 取 username 作 actor;ctx 无 user(理论兜底)用 `"system"`(防御性,当前无此路径)。

### 按 Hub 过滤的 list 接口
所有 `/api` list handler 从 ctx 取 hubID,非 super_admin 传给 store 强制过滤:
- `GET /api/hubs`:super_admin 全部;Hub 级只看自己 Hub。
- `GET /api/models`/`/api/overview`(登录态):按 ctx hubID 过滤;匿名/super_admin 全局。
- `GET /api/campaigns`/`/api/evals`/`/api/alerts`/`/api/tasks`/`/api/share-links`:按 ctx hubID。
- `GET /api/audit-logs`:按 ctx hubID(super_admin 看 hub_id IS NULL 的全局行 + 全部 Hub 行;Hub 级只看 hub_id=自己 的行)。
- `GET /api/settings`/`/api/classification-rules`/`/api/suites`/`/api/cases`:全局,任意已登录可读;写仅 super_admin。
- 公开详情(`/api/endpoints/{id}` 等)不按 Hub 过滤(见决策 5)。

store 层签名:`ListModelsByHub(hubID)` + `ListModelsAll()`;`ListCampaignsByHub/All`、`ListEvalRunsByHub/All`、`ListAlertEventsByHub/All`、`ListTasksByHub/All`、`ListShareLinksByHub/All`、`ListAuditLogsByHub/All`、`ListHubsForUser(hubID)`(单元素)/`ListHubsAll()`、`GetOverview(hubID *int64)`(nil=全局)。

## 前端变更

- `api/auth.ts`:`login({username,password})`;`fetchAuthStatus()` 返回 user 身份。`api/users.ts`(新):list/create/update/resetPassword/delete。
- `LoginView.vue`:单密码框 → 账号 + 密码双框。
- `AppHeader.vue`:`authed` ref 扩为 user 对象;右栏显示用户名 + 角色 tag;`visibleNavItems` 按角色过滤(viewer 只看状态板 + 评估榜单)。
- `AdminView.vue`:新增「用户管理」tab(位置:操作日志与设置之间),非 admin 角色隐藏该 tab;viewer 隐藏「设置」tab。
- `UserManager.vue`(新,复用 `HubManager.vue` 范式):el-card shadow=never + el-table + el-dialog + 手动校验 + ElMessage/ElMessageBox.confirm;密码框编辑时留空=不改;禁用/启用/改角色/重置密码。
- `router/index.ts`:可选——viewer 访问 `/admin` 重定向到只读页。

## 阶段拆分(ticket)

| Ticket | 目标 | 代表文件 | 承重墙 | 测试 |
|---|---|---|---|---|
| **61a** store 地基 | users 表 + session_secret seed | `store/store.go`、`store/users.go`(新) | — | L1 store |
| **61b** auth 重构 | `New(db,opts...)` 去 adminPassword;session secret;login 查 users;`/auth/me` 返回身份;改 `newTestAPIServer` 单点 + `testAdminPassword`/`forgeSessionToken` + ~24 直调测试文件 | `server/server.go`、`server/auth.go`、`main.go`、`scheduler_test.go`、`auth_test.go` + 24 文件 | W6 | L1+L2 |
| **62** CLI bootstrap | `hubscope admin create` 子命令(bcrypt + super_admin,hub_id=NULL) | `cmd/hubscope/main.go`、`cmd/hubscope/admin.go`(新) | — | L1 exec |
| **63** 角色中间件 + audit actor | `requireRole`;`requireSession` 注入 ctx;`s.audit` 读 ctx 取 username(删 auditActor 常量) | `server/auth.go`、`server/audit.go`、`server/server.go` 路由分组 | W6 | L1+L2 |
| **64a** models+overview 隔离 | `ListModelsByHub/All`、`GetOverview(hubID)`;overview handler 按 session 分叉(匿名全局/Hub 级本 Hub) | `store/models.go`、`store/overview.go`、`server/overview.go` | 新不变量 | L1 isolation |
| **64b** campaigns+evals 隔离 | JOIN campaign_models→models | `store/campaign.go`、`store/eval_run.go` | 同上 | L1 |
| **64c** alerts+tasks+share_links 隔离 | JOIN endpoints/campaigns→models;tasks 按 entity_type 分派 | `store/alerts.go`、`tasks.go`、`share_link.go` | 同上 | L1 |
| **65** audit_logs 加 hub_id + 隔离 | schema 加列;s.audit 落库写 hub_id;`ListAuditLogsByHub/All` | `store/store.go`、`store/audit.go`、`server/audit.go` | W6 子项 | L1+L2 |
| **66a** 用户管理 API | `/api/users` CRUD,gate super_admin+admin;admin 只管本 Hub | `server/users.go`(新)、`store/users.go` | — | L1 |
| **66b** 前端用户管理 | `UserManager.vue` + AdminView tab + AppHeader 用户名/角色 | `web/...`(见上) | — | typecheck+build |
| **67** 前端身份+角色导航 | login 账号密码、nav 按角色、tab 按角色显隐 | `api/auth.ts`、`LoginView.vue`、`AppHeader.vue`、`AdminView.vue`、`router` | — | typecheck+build |
| **68** 废弃清理 + ADR + W6 修订 | 删 ADMIN_PASSWORD 路径;定稿 ADR 0011;修订 W6 文档;更新部署文档 | `main.go`、`docs/adr/0011`、`.claude/rules/load-bearing-walls.md`、`docs/deployment.md` | W6 终态 | L3 全量 |

**TDD 起点:61a**(加表 + settings key,不改既有行为,最小爆炸半径)。61b 是真正触 W6 的 commit,爆炸半径集中在 `newTestAPIServer` 单点 + ~24 直调文件(逐个改实参)。

## 承重墙修订

### W6(凭证边界)修订
- 旧:管理员口令只经 `ADMIN_PASSWORD` env 注入,代码/配置/测试用假值。
- 新:凭证经 users 表(bcrypt 哈希)注入;session 签名 key 经 `SESSION_SECRET` env 或 settings 表自动生成(独立 secret,不从密码派生);首个 super_admin 经 CLI `hubscope admin create` 创建;`ADMIN_PASSWORD` 废弃。
- 配套 ADR 0011。

### 新隐式不变量:按 Hub 查询隔离
Phase 64 起,每个新的 list/query handler 必须按 session user 的 hub_id 过滤(super_admin 传 nil/走 `*All` 变体)。**结构性强制**:store 层 `List*` 无无参形态,漏传 hub 过滤=编译错误。运行时护栏:isolation sweep 测试(`internal/server/isolation_test.go`),seed 两 Hub,以 Hub-A 用户登录断言所有 list 接口不返回 Hub B 数据,super_admin 断言两边可见;新增 list 接口必须加对应断言行。CLAUDE.md「开工纪律」第 1 条权限与数据隔离风险项增列此不变量。

## Testing Decisions

- **W1 接缝不变**:httptest + stub hub + 假时钟 + 真 temp SQLite;不 mock 内部、不断言内部状态。
- **测试 helper 重构**:`newTestAPIServer`(`scheduler_test.go:154`)改签名去 password、seed 一个 test super_admin;`testAdminPassword` 常量废弃;`forgeSessionToken` 镜像新 token 格式(`<userId>.<unix>.<hmac>`);`authedClient` 语法不变但内部 POST body 改 `{username,password}`。改这三个单点 + `newTestAPIServer`,多数测试自动跟随;~24 个直调 `server.New` 的测试文件逐个改实参。
- **L1 新增**:`auth_test.go` 扩 login(账号+密码)、`role_test.go`(四级角色 403/200 矩阵)、`isolation_test.go`(两 Hub 隔离 sweep + super_admin 全局)、`users_test.go`(admin 只管本 Hub、operator 403、禁用用户登录失败、重置密码后可登录)。
- **L2 回测**:既有 `auth_test.go`/`audit_test.go`(actor 从 `"admin"` 变 username,断言改)/`security_test.go`/`scheduler_test.go`/`eval_weekly_test.go`/`discovery_test.go` 全绿。
- **L3 闭环**:`make test` 全量(后端 + 前端 typecheck + build)。后台作业走 DB 直连,不受 auth 改动影响。

## 迁移

1. 停服 → 升级二进制。
2. `hubscope admin create --username admin --password <新口令>`(无 --hub → super_admin)。
3. 设 `SESSION_SECRET`(或省略自动生成)。
4. 移除 `ADMIN_PASSWORD`(Phase 7 后硬忽略;过渡期 Phase 1 起若仍设,记 deprecation warning 不报错)。
5. 启动(`ADDR=:18080` 本地 / 生产自定义)。
6. super_admin 登录 → 建/核 Hub → 给各 Hub 派 admin。

## Out of Scope

- 多租户计费/配额。
- Hub 间数据共享/复制(每 Hub 独立数据集)。
- SSO/OIDC 外部身份源(本期纯本地账号)。
- `endpoints` 表冗余 `hub_id`(denormalize 优化,仅当 JOIN-through 实测性能不足再做)。
- API token(服务间鉴权),沿用 session cookie 即可。

## 对 spec 0001 的修订

- 0001「数据模型均带 hub_id」理想未完全落地(实际仅 `models` 表有 hub_id,architect 验证)。本 spec 基于**现状**设计隔离,不假设下游表已有 hub_id。
- 0001「认证与管理」单管理员口令语义被本 spec 取代。
