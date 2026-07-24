# 63 — 角色鉴权 + per-actor 审计

**What to build:** 不同角色有不同写权限，审计日志记录真实操作人而非硬编码 "admin"。

**Blocked by:** 61

**Status:** ready-for-agent

- [ ] `requireSession` 放行后将 `{userID,role,hubID,username}` 注入请求 ctx；新增 `sessionUser(r)` helper
- [ ] 新增 `requireRole(roles ...string)` 中间件叠在 `requireSession` 上，不匹配 → 403
- [ ] 路由分组：写操作 `requireRole(super_admin,admin,operator)`；读任意已登录；用户管理路由预留 `requireRole(super_admin,admin)`（67 落地）
- [ ] `s.audit` 签名不变，内部读 ctx 取 username 作 actor；删 `auditActor="admin"` 常量（ctx 无 user 的理论兜底用 `"system"`，当前无此路径）
- [ ] `share_links.go:77` 直接传 `auditActor` 给 `CreateShareLink` 作 `created_by`（不经 s.audit，ticket 61 影响分析发现）——同步改为取 ctx username，否则 `share_links.created_by` 永远写 "admin"
- [ ] 黑盒测试：viewer 写操作 → 403；operator 命中用户管理类路由 → 403；审计行 `actor` = 登录用户名而非 `"admin"`
- [ ] 既有 audit/security 测试回测绿
