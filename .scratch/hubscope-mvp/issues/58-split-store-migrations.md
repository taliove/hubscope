# 58 — 拆分 store.go 迁移/backfill 段

**What to build:** code-review(53-55 批)LOW-1 遗留:`internal/store/store.go` 已 ~493 行且随每次迁移持续增长。把 schema 迁移与 backfill 段拆到同包独立文件(如 `migrate.go`/`backfill.go`),`store.go` 只留 Open/连接管理。纯重构,行为不变,`make test` 全绿。

**Blocked by:** None — can start immediately

**Status:** 已迁移至 GitHub issue #11(2026-07-28 全面切换 GitHub Issues;本地票只读存档)

- [ ] store.go 拆分后各文件 ≤~400 行
- [ ] 纯移动(逐行 diff 证明),无标识符/行为变更
- [ ] `make test` 全绿
