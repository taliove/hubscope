# 69 — CLI admin create + 硬删 ADMIN_PASSWORD

**What to build:** `hubscope admin create` 子命令建首个 super_admin(bcrypt 哈希,hub_id=NULL);main.go os.Args 分派;CLI flag `--username --password [--hub]`(不传 --hub 建 super_admin,传则建 Hub 级 admin/operator/viewer);admin_test.go 测试(runAdminCreate 函数级 + bcrypt hash 非明文验证 + 重名失败)。CLI 落地后硬删 main.go 的 ADMIN_PASSWORD env 读取 + deprecation warning(61b 过渡期遗留)。ticket 61 范围遗漏的 CLI 部分在此补齐。

**Blocked by:** 61

**Status:** ready-for-agent

- [ ] `cmd/hubscope/admin.go`(新):`runAdminCreate(args)` 解析 flag、bcrypt.GenerateFromPassword、调 `db.CreateUser(username, hash, hubID, role)`;重名(ErrUsernameTaken)返回友好错误退出;用户名非空 + 密码≥8 + hub_id 存在性校验
- [ ] `cmd/hubscope/main.go`:os.Args 分派(`os.Args[1]=="admin"` → runAdmin(os.Args[2:]))+ 硬删 ADMIN_PASSWORD env 读取 + deprecation warning(57-62 整段)
- [ ] `cmd/hubscope/admin_test.go`(新):runAdminCreate 函数级测试(不启子进程)——建用户后 db.GetUserByUsername 可查 + password_hash 是 bcrypt 非明文 + 重名第二次失败 + 密码<8 拒绝 + --hub 不存在拒绝
- [ ] 全仓无 `ADMIN_PASSWORD` env 引用(grep 确认 testAdminPassword 是种子常量保留不删)
- [ ] `make test` 全绿
