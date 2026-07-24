# 61 — 多用户登录闭环（后端 wide refactor）

**What to build:** 用户能用账号+密码登录并换取带身份的 session。新增 CLI `hubscope admin create` 建首个 super_admin（bcrypt 哈希，hub_id=NULL）；users 表落地；session 签名 key 改为独立 `SESSION_SECRET`（env 或 settings 表首启自动生成），不再从密码派生；`server.New` 去掉 adminPassword 参数；`/api/auth/login` 改 `{username,password}`；`/api/auth/me` 返回 `{authenticated, user:{id,username,role,hub_id,hub_name}}`；session token 扩为 `<userId>.<issuedUnix>.<hmac>`。详见 spec 0005。**这是 wide refactor**：session 机制一换，所有依赖旧 password 派生 key 的测试必须同步改，一次必须全绿——无 expand-contract 余地（签名变更无法新旧并存）。爆炸半径集中在 `newTestAPIServer` 单点 + ~24 个直调 `server.New` 的测试文件（逐个改实参）。

**Blocked by:** None — can start immediately

**Status:** ready-for-agent

- [ ] CLI `hubscope admin create --username X --password Y` 在库建 super_admin，`password_hash` 是 bcrypt（非明文），hub_id=NULL；重名建第二次失败
- [ ] `SESSION_SECRET` env 设则用之；未设则首启在 settings 表生成 32 字节 hex 并落库，重启复用
- [ ] `POST /api/auth/login {username,password}` 正确 → 签发 cookie；错密码/禁用用户 → 401
- [ ] `GET /api/auth/me` 返回 `{authenticated:true, user:{id,username,role,hub_id,hub_name}}`
- [ ] `ADMIN_PASSWORD` 仍设时记 deprecation warning 不报错（过渡期，Phase 68 硬删）
- [ ] 既有全部黑盒测试绿（`make test` 后端）；`newTestAPIServer`/`authedClient`/`forgeSessionToken`/`testAdminPassword` 镜像新机制，调用语法不变
