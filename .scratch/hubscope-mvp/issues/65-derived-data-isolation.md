# 65 — 派生数据隔离

**What to build:** Hub 级用户看不到其他 Hub 的 campaign / eval / alert / task / share_link。

**Blocked by:** 64

**Status:** done(2026-07-28 核对销账:List*ByHub 系列 + isolation_test.go 均在)

- [ ] `campaigns`/`eval_runs`/`alerts`/`tasks`/`share_links` 各加 `ListXByHub`/`ListXAll` + handler 从 ctx 传 hubID；非 super_admin 强制本 Hub
- [ ] `tasks` 按 `entity_type` 分派 JOIN：`eval_run`→`campaign`→`campaign_models`→`models.hub_id`；`hub`→`entity_id` 直接；rollup/retention 类无 Hub 归属 task 归 super_admin 可见
- [ ] isolation sweep 扩展覆盖 campaign/eval/alert/task/share_link 接口
- [ ] 黑盒测试：Hub-A 用户 `GET /api/campaigns`、`/api/alerts`、`/api/tasks`、`/api/share-links` 不见 Hub-B 数据
