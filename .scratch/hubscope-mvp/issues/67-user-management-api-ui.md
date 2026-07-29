# 67 — 用户管理 API + UI

**What to build:** 管理员在管理台增删改本 Hub 用户。

**Blocked by:** 63, 64

**Status:** done(2026-07-28 核对销账:/api/users CRUD + UserManager.vue 均在)

- [ ] `/api/users` CRUD：GET 列表（super_admin 全部 / admin 只本 Hub）；POST 建（super_admin 可建任意角色+指定 hub_id；admin 只能本 Hub 建 operator/viewer）；PATCH 改 role/enabled（admin 只改本 Hub，禁自我降级防锁死）；PUT 重置密码；DELETE（admin 只删本 Hub，禁自删）
- [ ] `UserManager.vue` 复用 `HubManager.vue` 范式（el-card shadow=never + el-table + el-dialog + 手动校验 + ElMessage/ElMessageBox.confirm；密码框编辑时留空=不改）
- [ ] `AdminView` 新增「用户管理」tab（位置：操作日志与设置之间）；非 admin 角色隐藏该 tab
- [ ] 黑盒测试：admin 建本 Hub viewer → 201；admin 建/改他 Hub 用户 → 403；operator `POST /api/users` → 403；禁用用户登录失败；重置密码后用新密码可登录
- [ ] 前端 typecheck + build 绿
