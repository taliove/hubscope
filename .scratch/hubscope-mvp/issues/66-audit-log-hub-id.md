# 66 — audit_logs 加 hub_id + 隔离

**What to build:** 审计日志按 Hub 隔离——Hub 级用户只看本 Hub 审计，super_admin 看全局。

**Blocked by:** 64

**Status:** ready-for-agent

- [ ] `audit_logs` 加 `hub_id INTEGER NULL` 列（`ensureColumn` 幂等）；历史行回填 NULL（等价 super_admin 可见，不破坏旧数据）
- [ ] `s.audit` 落库时从 ctx 取 user.hub_id 写入；`auth.login`/`auth.logout`/`settings.update`/`discovery.*` 类无 Hub 归属动作写 NULL
- [ ] `ListAuditLogsByHub`/`ListAuditLogsAll`；`GET /api/audit-logs`：Hub 级只看 `hub_id=本 Hub`；super_admin 看 NULL + 全部
- [ ] 黑盒测试：Hub-A 用户 `GET /api/audit-logs` 不见 Hub-B 审计；super_admin 见全局；`auth.login` 类行（hub_id NULL）仅 super_admin 可见
- [ ] 既有 audit 测试回测绿（actor 断言已在 63 改）
