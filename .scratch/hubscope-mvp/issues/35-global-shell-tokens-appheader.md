# 35 — 全局壳:设计 token 体系 + AppHeader + 登录页品牌区

**What to build:** 全站切换至深靛蓝设计 token 体系(集中在 tokens.css,覆盖 Element Plus 变量 + `--hs-*` 自定义);新增 AppHeader 组件(左品牌区 Logo+HubScope、中导航「状态总览/评估中心/任务中心」、右「管理视图」+ 登录/退出),五个页面共用并删除各自手写散链与标题块;登录页不显示导航,改为居中品牌布局;ui-guidelines §2 同步修订为新 token 表。规格:docs/specs/0003-ui-redesign.md §3、§4。

**Blocked by:** None — can start immediately

**Status:** ready-for-agent

- [ ] tokens.css 全量 token 落地,全站无 `#409EFF` 等旧默认色残留
- [ ] 五个页面共享 AppHeader,当前页高亮正确,sticky 置顶
- [ ] 登录/退出全链路可用(复用 fetchAuthStatus,不引入状态库)
- [ ] 登录页无 header、居中品牌布局
- [ ] ui-guidelines §2 修订稿写入
- [ ] typecheck + build 通过;frontend-checker 全项 PASS
