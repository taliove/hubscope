# 48 — make lint 依赖 ensure-dist

**What to build:** `make lint`(go vet)在全新 clone/worktree 直接运行时失败:`web/embed.go` 的 `go:embed all:dist` 找不到 `web/dist`(W8 的 embed 前置)。`make test` 因 backend-test 依赖 ensure-dist 而正常,但 lint 单独跑会炸。让 lint 目标同样依赖 ensure-dist。

**Blocked by:** None — can start immediately

**Status:** done

- [ ] `make lint` 在全新 worktree(无 web/dist)直接成功
