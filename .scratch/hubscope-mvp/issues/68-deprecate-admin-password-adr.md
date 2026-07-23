# 68 — 废弃清理 + ADR 0011 + W6 修订

**What to build:** 移除单管理员口令遗留，文档定稿新模型。

**Blocked by:** 65, 66, 67

**Status:** done

- [x] ~~删 `cmd/hubscope/main.go` 的 `ADMIN_PASSWORD` 读取与 deprecation 逻辑~~ —— **延后到 ticket 69**(CLI `hubscope admin create` 未实现,硬删将锁死生产 bootstrap;main.go deprecation warning 保留至 69 落地)
- [x] 定稿 `docs/adr/0011-multi-user-auth-per-hub-isolation.md`(users 表 + bcrypt + 四级角色 + SESSION_SECRET 独立 + CLI bootstrap + 按 Hub 隔离 + audit_logs.hub_id + 公开路由双语义 + 全局资源口径;含 65b 漏洞 + isolation sweep 声明)
- [x] 修订 `.claude/rules/load-bearing-walls.md` W6:凭证经 users 表 bcrypt + `SESSION_SECRET` env/settings + CLI `admin create`;`ADMIN_PASSWORD` 废弃(过渡期 deprecation warning 保留至 69)
- [x] 更新部署文档:`hubscope admin create` bootstrap 步骤、`SESSION_SECRET` 说明、移除 `ADMIN_PASSWORD` 作为必填 env、本地 `ADDR=:8080` / 生产自定义端口(CLI 由 69 交付)
- [x] `make test` 全量绿;`testAdminPassword` 保留(种子常量,非 env),全仓无 `ADMIN_PASSWORD` env 代码引用(仅 main.go deprecation warning 与文档)
