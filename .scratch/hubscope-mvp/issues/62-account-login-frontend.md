# 62 — 前端账号密码登录 + 身份显示

**What to build:** 浏览器用户能用账号+密码登录并在顶栏看到自己的身份。

**Blocked by:** 61

**Status:** done(2026-07-28 核对销账:LoginView 双框 + AppHeader 角色 tag 均在)

- [ ] `LoginView` 单密码框 → 账号 + 密码双框，提交 `POST {username,password}`，成功跳 `?redirect` 目标
- [ ] `fetchAuthStatus()` 返回 user 身份对象（不只是 `{authenticated}`）
- [ ] `AppHeader` 右栏显示当前用户名 + 角色 tag；退出按钮行为不变，退出后跳首页
- [ ] 前端 typecheck + build 绿（`make test` 前端部分）
