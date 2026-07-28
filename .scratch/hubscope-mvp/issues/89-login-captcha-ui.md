# 89 — LoginView 验证码区 + 设计评审

**What to build:** 按 spec 0012 决策 6,让验证码在登录页真实可用。行为:LoginView 默认渲染现状(用户名 + 密码,品牌区不动);登录响应带 `captcha_required` 标记时**展开验证码区**——图形 img(data URI,**点击图形刷新**重新拉 `GET /api/auth/captcha`)+ el-input 输答案,之后每次提交携带 `captcha_id`/`captcha_answer`;展开后保持至登录成功;「请完成验证码」/「验证码错误或已过期」**内联**展示(§5 表单校验失败内联纪律,不用弹窗);提交期间按钮 loading(§6 即时反馈);图形加载失败给重试入口(§6 三态)。**开工先过设计评审**(frontend skill 调 plan 的 UI 评审子能力):验证码区在登录卡的布局/尺寸/间距走三层令牌,登录页工具风与 §2b 品牌区(BrandMark + Wordmark + display 档)不动。契约以 ticket 88 落地为准(`captcha_required` 标记 + 双字段请求体 + 两条错误消息文案)。

**Blocked by:** 88(登录契约与签发端点先行,前端只消费稳定接口)

**Status:** done(2026-07-28,53dd46f;设计评审有条件通过并完成,§6 两条新约定已回写;check 三维度 PASS;两个决策点(401/429 中文映射)已纳入)

## 执行顺序

1. **设计评审**:frontend skill 调 plan 评审验证码区方案(布局/令牌/三态),结论回写 ui-guidelines.md 若产生新约定
2. **实现**:LoginView 验证码区(展开状态机:隐藏 → captcha_required 展开 → 成功回隐藏;点击刷新;提交携带字段)
3. **验证**:typecheck + build;手动联调(本地起 dev,制造 2 次失败后第 3 次见码)

## 验收清单

- [x] 默认登录页与现状像素级一致(无验证码区残留占位)
- [x] 收到 `captcha_required` 后展开验证码区;「请完成验证码」与「验证码错误或已过期」内联展示不弹窗
- [x] 点击图形刷新(新 id 新图);提交携带 `captcha_id` + `captcha_answer`
- [x] 登录成功后验证码区回到隐藏态(后续会话默认无码)
- [x] 图形加载失败有重试入口;提交期间 loading;无闪烁/布局跳动
- [x] 设计评审结论落地;如涉及新约定已回写 ui-guidelines.md
- [x] typecheck/build 通过(已通过 2026-07-28);check 三维度 PASS(2026-07-28,五分支状态机逐分支走查通过;2 项观察登记不处理)

## 风险登记

1. **契约漂移**:88 的标记名/字段名/文案若有调整,以 88 最终落地为准,本票不另造口径
2. **展开跳动**:验证码区插入不能推挤品牌区与表单主列(登录页是公开门面),布局预留或过渡要评审定论
3. **data URI 与 CSP**:img-src 'self' data: 已允许 data:(ratelimit.go 安全头),若评审改图形呈现方式需复核 CSP
